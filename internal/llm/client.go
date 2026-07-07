package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client is an OpenAI-compatible LLM client
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// Config for LLM client
type Config struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	return &Client{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Chat sends messages and returns the assistant's reply
func (c *Client) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	return c.chat(ctx, messages)
}

// ChatJSON sends a prompt and unmarshals the response into result (must be a pointer)
func (c *Client) ChatJSON(ctx context.Context, systemPrompt, userPrompt string, result any) error {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	reply, err := c.chat(ctx, messages)
	if err != nil {
		return err
	}
	// Try to extract JSON from markdown code blocks, then parse
	extracted := strings.TrimSpace(extractJSON(reply))

	// Try direct parse first
	if err := json.Unmarshal([]byte(extracted), result); err == nil {
		return nil
	}

	// Try truncating trailing characters (for objects and arrays)
	if idx := strings.LastIndex(extracted, "}"); idx > 0 {
		if err := json.Unmarshal([]byte(extracted[:idx+1]), result); err == nil {
			return nil
		}
	}
	if idx := strings.LastIndex(extracted, "]"); idx > 0 {
		if err := json.Unmarshal([]byte(extracted[:idx+1]), result); err == nil {
			return nil
		}
	}

	// Try unescaping JSON string escapes (LLM may return \" instead of ")
	if strings.Contains(extracted, `\"`) {
		unescaped := strings.ReplaceAll(extracted, `\"`, `"`)
		unescaped = strings.ReplaceAll(unescaped, `\\n`, `\n`)
		unescaped = strings.ReplaceAll(unescaped, `\\t`, `\t`)
		if err := json.Unmarshal([]byte(unescaped), result); err == nil {
			return nil
		}
		// Try truncating after unescape too
		if idx := strings.LastIndex(unescaped, "}"); idx > 0 {
			if err := json.Unmarshal([]byte(unescaped[:idx+1]), result); err == nil {
				return nil
			}
		}
	}

	// Try interpreting as a JSON-encoded string (double-encoded)
	if strings.HasPrefix(extracted, `"`) && strings.HasSuffix(extracted, `"`) {
		var inner string
		if err := json.Unmarshal([]byte(extracted), &inner); err == nil {
			if err := json.Unmarshal([]byte(inner), result); err == nil {
				return nil
			}
			// Try unescape on inner too
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			if err := json.Unmarshal([]byte(inner), result); err == nil {
				return nil
			}
		}
	}

	// Last resort: try to fix common LLM JSON issues
	fixed := fixLLMJSON(extracted)
	if fixed != extracted {
		if err := json.Unmarshal([]byte(fixed), result); err == nil {
			return nil
		}
	}

	if len(reply) > 1000 {
		return fmt.Errorf("%w\n\n-- RAW (last 500) --\n%s\n-- EXTRACTED --\n%s", json.Unmarshal([]byte(extracted), result),
			reply[len(reply)-min(500, len(reply)):], extracted[:min(500, len(extracted))])
	}
	return fmt.Errorf("%w\n\n-- RAW --\n%s\n-- EXTRACTED --\n%s", json.Unmarshal([]byte(extracted), result),
		reply[:min(500, len(reply))], extracted[:min(500, len(extracted))])
}

// fixLLMJSON attempts to fix common LLM JSON output issues
func fixLLMJSON(s string) string {
	// Fix: trailing commas before ] or }
	s = strings.ReplaceAll(s, ",]", "]")
	s = strings.ReplaceAll(s, ",}", "}")
	// Fix: unclosed strings at end
	if strings.Count(s, `"`)%2 != 0 {
		s += `"`
	}
	return s
}

func (c *Client) chat(ctx context.Context, messages []chatMessage) (string, error) {
	reqBody := chatRequest{
		Model:               c.model,
		Messages:            messages,
		Temperature:         0.3,
		MaxTokens:           32768,
		MaxCompletionTokens: 32768,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("llm error %d: %s", resp.StatusCode, string(body))
	}

	// Some LLM APIs return SSE-formatted data even for non-streaming requests.
	// Try to detect and parse SSE stream format by collecting all chunks.
	content := string(body)

	if strings.Contains(content, "data: ") && strings.Contains(content, "\n\n") {
		// Looks like SSE — collect all data chunks
		accumulated := collectSSEContent(content)
		if accumulated != "" {
			return accumulated, nil
		}
	}

	var result chatResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// Last resort: try to extract content field directly with bracket matching
		if extracted := extractJSONField(content, "content"); extracted != "" {
			return extracted, nil
		}
		// If nothing worked, log and return raw for caller to handle
		log.Printf("⚠️  LLM unparseable response (%d bytes): %.200s", len(body), content)
		return content, nil
	}

	if len(result.Choices) == 0 {
		log.Printf("⚠️  LLM response has no choices (%d bytes): %.200s", len(body), content)
		return "", fmt.Errorf("no choices in llm response (%d bytes body)", len(body))
	}

	reply := result.Choices[0].Message.Content
	if reply == "" {
		log.Printf("⚠️  LLM response has empty content (%d bytes): %.300s", len(body), content)
	} else if result.Choices[0].FinishReason != "" && result.Choices[0].FinishReason != "stop" {
		log.Printf("⚠️  LLM finish_reason=%s — response may be truncated (%d bytes, %d chars)",
			result.Choices[0].FinishReason, len(body), len(reply))
	}
	return reply, nil
}

// collectSSEContent parses SSE stream data and accumulates delta.content values
func collectSSEContent(body string) string {
	var result strings.Builder
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}

		// Try to parse as stream chunk with delta.content
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Text string `json:"text"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			// Prefer delta.content (streaming), fallback to message.content
			if choice.Delta.Content != "" {
				result.WriteString(choice.Delta.Content)
			} else if choice.Message.Content != "" {
				result.WriteString(choice.Message.Content)
			} else if choice.Text != "" {
				result.WriteString(choice.Text)
			}
		}
	}

	return result.String()
}

// extractJSONField extracts a specific JSON field value from raw JSON
func extractJSONField(raw, field string) string {
	// Look for "field":" or "field": "
	search := fmt.Sprintf(`"%s":"`, field)
	idx := strings.Index(raw, search)
	if idx < 0 {
		search = fmt.Sprintf(`"%s": "`, field)
		idx = strings.Index(raw, search)
	}
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	// Find the closing unescaped quote
	for i := start; i < len(raw); i++ {
		if raw[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if raw[i] == '"' {
			return raw[start:i]
		}
	}
	return ""
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	Temperature         float64       `json:"temperature"`
	MaxTokens           int           `json:"max_tokens,omitempty"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// extractJSON extracts JSON from markdown code fences or raw text
func extractJSON(text string) string {
	// Try ```json ... ``` block
	for _, fence := range []string{"```json", "```"} {
		start := indexOf(text, fence)
		if start >= 0 {
			start += len(fence)
			rest := text[start:]
			end := indexOf(rest, "```")
			if end >= 0 {
				return strings.TrimSpace(rest[:end])
			}
		}
	}
	// Try to find first [ or { with bracket counting (support both arrays and objects)
	startArr := indexOf(text, "[")
	startObj := indexOf(text, "{")

	// Use whichever comes first
	start := -1
	if startArr >= 0 && (startObj < 0 || startArr < startObj) {
		start = startArr
	} else if startObj >= 0 {
		start = startObj
	}

	if start >= 0 {
		depth := 0
		// Count all brackets: {} and [] to handle nested arrays/objects
		for i := start; i < len(text); i++ {
			ch := text[i]
			if ch == '\\' {
				i++ // skip escaped char
				continue
			}
			switch ch {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return text[start : i+1]
				}
			}
		}
	}
	return text
}

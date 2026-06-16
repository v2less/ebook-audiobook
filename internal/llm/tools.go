package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Tool definition for function calling
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents a function call requested by the LLM
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResult is the result of executing a tool call
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ChatWithTools sends messages with tool definitions and returns tool calls
func (c *Client) ChatWithTools(ctx context.Context, systemPrompt, userPrompt string, tools []Tool) ([]ToolCall, error) {
	messages := []toolMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reqBody := toolChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
		Tools:       tools,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal tool request: %w", err)
	}

	return c.sendToolRequest(ctx, payload)
}

// ChatWithToolResults continues a conversation with tool results
func (c *Client) ChatWithToolResults(ctx context.Context, systemPrompt string, history []toolMessage, toolResults []ToolResult) (string, error) {
	// Build messages: system + history + tool results as role="tool" messages
	messages := []toolMessage{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, history...)
	for _, tr := range toolResults {
		messages = append(messages, toolMessage{
			Role:       "tool",
			Content:    tr.Content,
			ToolCallID: tr.ToolCallID,
		})
	}

	reqBody := toolChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal tool follow-up: %w", err)
	}

	resp, err := c.sendToolRequest(ctx, payload)
	if err != nil {
		return "", err
	}
	if len(resp) > 0 {
		return resp[0].Arguments, nil
	}
	return "", nil
}

func (c *Client) sendToolRequest(ctx context.Context, payload []byte) ([]ToolCall, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tool chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llm tool error %d: %s", resp.StatusCode, string(body))
	}

	var result toolChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse tool response: %w", err)
	}

	var toolCalls []ToolCall
	for _, choice := range result.Choices {
		if choice.Message.ToolCalls != nil {
			for _, tc := range choice.Message.ToolCalls {
				toolCalls = append(toolCalls, ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}
	}

	return toolCalls, nil
}

// AudiobookTools returns the standard tools for audiobook production
func AudiobookTools() []Tool {
	return []Tool{
		{
			Name:        "search_sfx",
			Description: "搜索本地音效库中匹配关键词的音效文件",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{
						"type":        "string",
						"description": "搜索关键词，如：开门声、雨声、脚步声",
					},
				},
				"required": []string{"keyword"},
			},
		},
		{
			Name:        "search_bgm",
			Description: "搜索本地背景音乐库中匹配情绪的音乐文件",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mood": map[string]any{
						"type":        "string",
						"description": "情绪描述，如：紧张、温馨、悲伤、欢快",
					},
				},
				"required": []string{"mood"},
			},
		},
		{
			Name:        "get_character_voice",
			Description: "获取指定角色已配置的语音音色信息",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"character_name": map[string]any{
						"type":        "string",
						"description": "角色名称",
					},
				},
				"required": []string{"character_name"},
			},
		},
		{
			Name:        "suggest_audio_filter",
			Description: "根据场景类型推荐合适的音频滤镜效果",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scene_type": map[string]any{
						"type":        "string",
						"description": "场景类型：phone_call, underwater, broadcast, inner_monologue, mecha_voice, cave, stadium",
					},
				},
				"required": []string{"scene_type"},
			},
		},
	}
}

type toolMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls,omitempty"`
}

type toolChatRequest struct {
	Model       string        `json:"model"`
	Messages    []toolMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Tools       []Tool        `json:"tools,omitempty"`
}

type toolChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

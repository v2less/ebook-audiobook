package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ebook-audiobook/internal/model"
)

// CustomEngine supports any OpenAI-compatible TTS API endpoint
type CustomEngine struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	voices  []model.VoiceProfile
	client  *http.Client
}

// CustomEngineConfig configures a user-defined TTS engine
type CustomEngineConfig struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	Voices  []model.VoiceProfile `json:"voices"`
}

func NewCustomEngine(cfg CustomEngineConfig) *CustomEngine {
	if cfg.Model == "" {
		cfg.Model = "tts-1"
	}
	return &CustomEngine{
		name:    cfg.Name,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		voices:  cfg.Voices,
		client:  &http.Client{},
	}
}

func (e *CustomEngine) Name() string             { return e.name }
func (e *CustomEngine) SupportsVoiceClone() bool  { return false }
func (e *CustomEngine) SupportsVoiceDesign() bool { return false }

func (e *CustomEngine) ListVoices() ([]model.VoiceProfile, error) {
	return e.voices, nil
}

// Synthesize via OpenAI /audio/speech endpoint
func (e *CustomEngine) Synthesize(ctx context.Context, text string, opts model.TTSOptions) ([]byte, string, error) {
	format := opts.Format
	if format == "" {
		format = "wav"
	}

	model := opts.Model
	if model == "" {
		model = e.model
	}
	voice := opts.VoiceID
	if voice == "" && len(e.voices) > 0 {
		voice = e.voices[0].VoiceID
	}

	// Try OpenAI /audio/speech first
	audio, err := e.tryOpenAISpeech(ctx, text, model, voice, format)
	if err == nil {
		return audio, format, nil
	}

	// Fallback: try MiMo-style /chat/completions
	return e.tryMiMoStyle(ctx, text, model, voice, format, opts.StyleDirective)
}

// tryOpenAISpeech tries the standard OpenAI TTS endpoint
func (e *CustomEngine) tryOpenAISpeech(ctx context.Context, text, model, voice, format string) ([]byte, error) {
	reqBody := map[string]any{
		"model": model,
		"input": text,
		"voice": voice,
		"response_format": format,
	}

	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts api %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// tryMiMoStyle tries MiMo-compatible /chat/completions endpoint
func (e *CustomEngine) tryMiMoStyle(ctx context.Context, text, model, voice, format, style string) ([]byte, string, error) {
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": style},
			{"role": "assistant", "content": text},
		},
		"audio": map[string]any{
			"format": format,
			"voice":  voice,
		},
	}

	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("tts api %d: %s", resp.StatusCode, string(body))
	}

	var result chatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("parse tts response: %w", err)
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Audio.Data == "" {
		return nil, "", fmt.Errorf("no audio in tts response")
	}

	audio, err := base64.StdEncoding.DecodeString(result.Choices[0].Message.Audio.Data)
	return audio, format, err
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Audio struct {
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
}

// SynthesizeStream for custom engines supports true SSE streaming
func (e *CustomEngine) SynthesizeStream(ctx context.Context, text string, opts model.TTSOptions, callback func(chunk []byte) error) error {
	if opts.Model == "" {
		opts.Model = e.model
	}
	if opts.Format == "" {
		opts.Format = "wav"
	}
	voice := opts.VoiceID
	if voice == "" && len(e.voices) > 0 {
		voice = e.voices[0].VoiceID
	}

	// Try OpenAI streaming TTS first
	err := e.tryStreamOpenAISpeech(ctx, text, opts.Model, voice, opts.Format, callback)
	if err == nil {
		return nil
	}

	// Fallback: try MiMo-style streaming via /chat/completions
	err = e.tryStreamMiMoStyle(ctx, text, opts.Model, voice, opts.Format, opts.StyleDirective, callback)
	if err == nil {
		return nil
	}

	// Last resort: non-streaming then callback once
	audio, _, err := e.Synthesize(ctx, text, opts)
	if err != nil {
		return err
	}
	return callback(audio)
}

// tryStreamOpenAISpeech attempts streaming from /audio/speech endpoint
func (e *CustomEngine) tryStreamOpenAISpeech(ctx context.Context, text, model, voice, format string, callback func(chunk []byte) error) error {
	reqBody := map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voice,
		"response_format": format,
	}

	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Accept", "audio/"+format)

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tts stream api %d: %s", resp.StatusCode, string(body))
	}

	// If the response is chunked, stream it
	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := callback(chunk); err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// tryStreamMiMoStyle attempts SSE streaming via /chat/completions
func (e *CustomEngine) tryStreamMiMoStyle(ctx context.Context, text, model, voice, format, style string, callback func(chunk []byte) error) error {
	userContent := style
	if userContent == "" {
		userContent = "请用自然的语气朗读以下内容"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": userContent},
			{"role": "assistant", "content": text},
		},
		"audio": map[string]any{
			"format": "pcm16",
			"voice":  voice,
		},
		"stream": true,
	}

	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tts stream api %d: %s", resp.StatusCode, string(body))
	}

	// Parse SSE stream
	buf := make([]byte, 4096)
	var leftover string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			leftover += string(buf[:n])
			for {
				idx := strings.Index(leftover, "\n\n")
				if idx < 0 {
					break
				}
				line := strings.TrimSpace(leftover[:idx])
				leftover = leftover[idx+2:]

				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						return nil
					}
					// Try to extract audio from delta
					var chunk struct {
						Choices []struct {
							Delta struct {
								Audio struct {
									Data string `json:"data"`
								} `json:"audio"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if err := json.Unmarshal([]byte(data), &chunk); err != nil {
						continue
					}
					if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Audio.Data != "" {
						audioChunk, err := base64.StdEncoding.DecodeString(chunk.Choices[0].Delta.Audio.Data)
						if err != nil {
							continue
						}
						if err := callback(audioChunk); err != nil {
							return err
						}
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}

// ---- Multi-Engine Voice Aggregation ----

// AllVoices aggregates voices from all registered engines
func AllVoices(reg *EngineRegistry) []model.VoiceProfile {
	var all []model.VoiceProfile
	for _, name := range []string{"mimo", "rtvc"} {
		engine, err := reg.Get(name)
		if err != nil {
			continue
		}
		voices, err := engine.ListVoices()
		if err != nil {
			continue
		}
		all = append(all, voices...)
	}
	// Add custom engines later
	return all
}

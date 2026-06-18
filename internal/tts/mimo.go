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

// MiMoEngine implements TTS synthesis via Xiaomi MiMo API (OpenAI-compatible)
type MiMoEngine struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// MiMo preset voices
var MiMoPresetVoices = []model.VoiceProfile{
	{ID: "mimo_default", Name: "MiMo-默认", Source: "preset", Engine: "mimo", VoiceID: "mimo_default", Language: "zh-CN", Gender: "female"},
	{ID: "bingtang", Name: "冰糖", Source: "preset", Engine: "mimo", VoiceID: "冰糖", Language: "zh-CN", Gender: "female"},
	{ID: "moli", Name: "茉莉", Source: "preset", Engine: "mimo", VoiceID: "茉莉", Language: "zh-CN", Gender: "female"},
	{ID: "suda", Name: "苏打", Source: "preset", Engine: "mimo", VoiceID: "苏打", Language: "zh-CN", Gender: "male"},
	{ID: "baihua", Name: "白桦", Source: "preset", Engine: "mimo", VoiceID: "白桦", Language: "zh-CN", Gender: "male"},
	{ID: "mia", Name: "Mia", Source: "preset", Engine: "mimo", VoiceID: "Mia", Language: "en-US", Gender: "female"},
	{ID: "chloe", Name: "Chloe", Source: "preset", Engine: "mimo", VoiceID: "Chloe", Language: "en-US", Gender: "female"},
	{ID: "milo", Name: "Milo", Source: "preset", Engine: "mimo", VoiceID: "Milo", Language: "en-US", Gender: "male"},
	{ID: "dean", Name: "Dean", Source: "preset", Engine: "mimo", VoiceID: "Dean", Language: "en-US", Gender: "male"},
}

func NewMiMoEngine(apiKey, baseURL string) *MiMoEngine {
	if baseURL == "" {
		baseURL = "https://api.xiaomimimo.com/v1"
	}
	return &MiMoEngine{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (e *MiMoEngine) Name() string { return "MiMo" }

func (e *MiMoEngine) SupportsVoiceClone() bool { return true }
func (e *MiMoEngine) SupportsVoiceDesign() bool { return true }

// ListVoices returns preset MiMo voices
func (e *MiMoEngine) ListVoices() ([]model.VoiceProfile, error) {
	return MiMoPresetVoices, nil
}

// Synthesize non-streaming TTS
func (e *MiMoEngine) Synthesize(ctx context.Context, text string, opts model.TTSOptions) ([]byte, string, error) {
	if opts.Model == "" {
		opts.Model = "mimo-v2.5-tts"
	}
	if opts.Format == "" {
		opts.Format = "wav"
	}

	reqBody := map[string]any{
		"model": opts.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": opts.StyleDirective,
			},
			{
				"role":    "assistant",
				"content": text,
			},
		},
		"audio": map[string]any{
			"format": opts.Format,
			"voice":  opts.VoiceID,
		},
	}

	return e.doRequest(ctx, reqBody, opts.Format)
}

// SynthesizeClone 音色复刻合成
func (e *MiMoEngine) SynthesizeClone(ctx context.Context, text string, voiceAudio []byte, voiceFormat string, opts model.TTSOptions) ([]byte, string, error) {
	// Map file extension to MIME type
	mimeType := "audio/mpeg" // default for mp3
	switch strings.ToLower(voiceFormat) {
	case "wav":
		mimeType = "audio/wav"
	case "m4a", "aac":
		mimeType = "audio/mp4"
	case "ogg":
		mimeType = "audio/ogg"
	case "flac":
		mimeType = "audio/flac"
	case "mp3":
		mimeType = "audio/mpeg"
	}
	voiceB64 := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(voiceAudio))

	reqBody := map[string]any{
		"model": "mimo-v2.5-tts-voiceclone",
		"messages": []map[string]string{
			{"role": "user", "content": opts.StyleDirective},
			{"role": "assistant", "content": text},
		},
		"audio": map[string]any{
			"format": opts.Format,
			"voice":  voiceB64,
		},
	}

	return e.doRequest(ctx, reqBody, opts.Format)
}

// SynthesizeDesign 音色设计合成
func (e *MiMoEngine) SynthesizeDesign(ctx context.Context, text string, designPrompt string, opts model.TTSOptions) ([]byte, string, error) {
	reqBody := map[string]any{
		"model": "mimo-v2.5-tts-voicedesign",
		"messages": []map[string]string{
			{"role": "user", "content": designPrompt},
			{"role": "assistant", "content": text},
		},
		"audio": map[string]any{
			"format":               opts.Format,
			"optimize_text_preview": true,
		},
	}

	return e.doRequest(ctx, reqBody, opts.Format)
}

// SynthesizeStream streaming TTS
func (e *MiMoEngine) SynthesizeStream(ctx context.Context, text string, opts model.TTSOptions, callback func(chunk []byte) error) error {
	if opts.Model == "" {
		opts.Model = "mimo-v2.5-tts"
	}
	opts.Format = "pcm16" // streaming requires PCM

	reqBody := map[string]any{
		"model": opts.Model,
		"messages": []map[string]string{
			{"role": "user", "content": opts.StyleDirective},
			{"role": "assistant", "content": text},
		},
		"audio": map[string]any{
			"format": "pcm16",
			"voice":  opts.VoiceID,
		},
		"stream": true,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	// Parse SSE stream for audio chunks
	return e.parseStream(ctx, resp.Body, callback)
}

func (e *MiMoEngine) doRequest(ctx context.Context, reqBody map[string]any, format string) ([]byte, string, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	// Parse OpenAI-compatible chat completion response
	var result chatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Audio.Data == "" {
		return nil, "", fmt.Errorf("no audio data in response")
	}

	audioBytes, err := base64.StdEncoding.DecodeString(result.Choices[0].Message.Audio.Data)
	if err != nil {
		return nil, "", fmt.Errorf("decode audio: %w", err)
	}

	return audioBytes, format, nil
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Audio struct {
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
}

// parseStream parses SSE streaming response for audio chunks
func (e *MiMoEngine) parseStream(ctx context.Context, body io.Reader, callback func(chunk []byte) error) error {
	buf := make([]byte, 4096)
	var leftover string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := body.Read(buf)
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
					chunk, err := extractAudioChunk(data)
					if err != nil {
						continue
					}
					if chunk != nil {
						if err := callback(chunk); err != nil {
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

func extractAudioChunk(data string) ([]byte, error) {
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
		return nil, err
	}
	if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Audio.Data == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(chunk.Choices[0].Delta.Audio.Data)
}

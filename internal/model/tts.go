package model

import "time"

// VoiceProfile 音色配置
type VoiceProfile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Source      string    `json:"source"`       // "preset" | "clone" | "design"
	Engine      string    `json:"engine"`        // "mimo" | "rtvc"
	VoiceID     string    `json:"voice_id"`      // MiMo preset voice id, or custom id
	SamplePath  string    `json:"sample_path"`   // reference audio file path (clone)
	DesignPrompt string   `json:"design_prompt"` // voice design text description
	Description string    `json:"description"`
	Language    string    `json:"language"`      // zh-CN, en-US, etc.
	Gender      string    `json:"gender"`        // male, female, neutral
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TTSOptions TTS 合成选项
type TTSOptions struct {
	VoiceID         string  `json:"voice_id"`
	Model           string  `json:"model"`             // mimo-v2.5-tts, mimo-v2.5-tts-voiceclone, etc.
	Format          string  `json:"format"`            // wav, pcm16, mp3
	Speed           float64 `json:"speed"`             // 0.5 - 2.0
	StyleDirective  string  `json:"style_directive"`   // natural language style prompt
	UseStreaming    bool    `json:"use_streaming"`     // stream vs non-stream
	SampleRate      int     `json:"sample_rate"`       // default 24000
}

// TTSEngine 接口在 tts 包中定义

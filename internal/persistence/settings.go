package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// RuntimeSettings holds in-memory settings that survive restart
type RuntimeSettings struct {
	MiMoAPIKey     string            `json:"mimo_api_key"`
	LLMBaseURL     string            `json:"llm_base_url"`
	LLMAPIKey      string            `json:"llm_api_key"`
	LLMModel       string            `json:"llm_model"`
	CustomTTS      []TTSConfig       `json:"custom_tts"`
}

type TTSConfig struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Key    string `json:"key"`
	Model  string `json:"model"`
	Voices string `json:"voices"`
}

var (
	mu       sync.Mutex
	settings RuntimeSettings
	filePath string
)

// Load reads persisted settings from disk
func Load(path string) (*RuntimeSettings, error) {
	mu.Lock()
	defer mu.Unlock()

	filePath = filepath.Join(path, "runtime_settings.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &RuntimeSettings{}, nil
		}
		return nil, err
	}
	var s RuntimeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return &RuntimeSettings{}, nil
	}
	settings = s
	return &s, nil
}

// Save persists current settings to disk
func Save(s *RuntimeSettings) error {
	mu.Lock()
	defer mu.Unlock()

	if s != nil {
		settings = *s
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(filePath), 0755)
	return os.WriteFile(filePath, data, 0644)
}

// Current returns the current settings
func Current() RuntimeSettings {
	mu.Lock()
	defer mu.Unlock()
	return settings
}

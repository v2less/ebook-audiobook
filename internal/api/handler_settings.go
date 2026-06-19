package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"ebook-audiobook/internal/llm"
	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/tts"
)

// ---- Settings handlers ----

// settingsResponse is the full settings payload returned to the frontend
type settingsResponse struct {
	MiMoAPIKeySet  bool   `json:"mimo_api_key_set"`
	MiMoAPIKeyMask string `json:"mimo_api_key_mask"`
	LLMBaseURL     string `json:"llm_base_url"`
	LLMAPIKeySet   bool   `json:"llm_api_key_set"`
	LLMAPIKeyMask  string `json:"llm_api_key_mask"`
	LLMModel       string `json:"llm_model"`
	CustomTTS      []customTTSInfo `json:"custom_tts"`
}

type customTTSInfo struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	KeySet     bool   `json:"key_set"`
	KeyMask    string `json:"key_mask"`
	RawKey     string `json:"-"`
	Model      string `json:"model"`
	Voices     string `json:"voices"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	// Support ?import=1 to trigger library import
	if r.URL.Query().Get("import") == "1" {
		projPath := s.getProjectJSONPath()
		if projPath == "" {
			writeJSON(w, map[string]any{
				"sfx_count": 0, "bgm_count": 0, "voice_count": 0,
				"errors_count": 1, "errors": []string{"project json path not configured, set library.project_path in config.yaml"},
			})
			return
		}
		result, err := s.importProjectJSON(projPath)
		if err != nil {
			writeJSON(w, map[string]any{
				"sfx_count": 0, "bgm_count": 0, "voice_count": 0,
				"errors_count": 1, "errors": []string{err.Error()},
			})
			return
		}
		writeJSON(w, result)
		return
	}

	s.llmMu.Lock()
	llmSet := s.llmCfg != nil && s.llmCfg.APIKey != ""
	llmBaseURL := ""
	llmModel := ""
	llmKeyMask := ""
	if s.llmCfg != nil {
		llmBaseURL = s.llmCfg.BaseURL
		llmModel = s.llmCfg.Model
		llmKeyMask = maskKey(s.llmCfg.APIKey)
	}
	s.llmMu.Unlock()

	// Collect custom TTS engine info
	var customTTS []customTTSInfo
	for name := range s.customEngines {
		info := s.lookupCustomTTSInfo(name)
		if info.Name == "" {
			info.Name = name
		}
		customTTS = append(customTTS, info)
	}

	writeJSON(w, settingsResponse{
		MiMoAPIKeySet:  s.cfg.MiMo.APIKey != "",
		MiMoAPIKeyMask: maskKey(s.cfg.MiMo.APIKey),
		LLMBaseURL:     llmBaseURL,
		LLMAPIKeySet:   llmSet,
		LLMAPIKeyMask:  llmKeyMask,
		LLMModel:       llmModel,
		CustomTTS:      customTTS,
	})
}

// lookupCustomTTSInfo reads custom TTS config from persisted file
func (s *Server) lookupCustomTTSInfo(name string) customTTSInfo {
	data, err := os.ReadFile("./data/runtime_settings.json")
	if err != nil {
		return customTTSInfo{}
	}
	var rs struct {
		CustomTTS []struct {
			Name   string `json:"name"`
			URL    string `json:"url"`
			Key    string `json:"key"`
			Model  string `json:"model"`
			Voices string `json:"voices"`
		} `json:"custom_tts"`
	}
	if err := json.Unmarshal(data, &rs); err != nil {
		return customTTSInfo{}
	}
	for _, c := range rs.CustomTTS {
		if c.Name == name {
			return customTTSInfo{
				Name:    c.Name,
				URL:     c.URL,
				KeySet:  c.Key != "",
				KeyMask: maskKey(c.Key),
				RawKey:  c.Key,
				Model:   c.Model,
				Voices:  c.Voices,
			}
		}
	}
	return customTTSInfo{}
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ApiKey    string `json:"api_key"`
		LLMURL    string `json:"llm_url"`
		LLMKey    string `json:"llm_key"`
		LLMModel  string `json:"llm_model"`
		TTSName   string `json:"tts_name"`
		TTSURL    string `json:"tts_url"`
		TTSKey    string `json:"tts_key"`
		TTSModel  string `json:"tts_model"`
		TTSVoices string `json:"tts_voices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.ApiKey != "" {
		s.cfg.MiMo.APIKey = req.ApiKey
		s.engineReg.Register("mimo", tts.NewMiMoEngine(req.ApiKey, s.cfg.MiMo.BaseURL))
	}

	s.llmMu.Lock()
	if req.LLMKey != "" {
		if s.llmCfg == nil {
			s.llmCfg = &llm.Config{}
		}
		s.llmCfg.APIKey = req.LLMKey
	}
	if req.LLMURL != "" {
		if s.llmCfg == nil {
			s.llmCfg = &llm.Config{}
		}
		s.llmCfg.BaseURL = req.LLMURL
	}
	if req.LLMModel != "" {
		if s.llmCfg == nil {
			s.llmCfg = &llm.Config{}
		}
		s.llmCfg.Model = req.LLMModel
	}
	s.llmMu.Unlock()

	if req.TTSURL != "" && req.TTSKey != "" {
		var voices []model.VoiceProfile
		if req.TTSVoices != "" {
			for _, pair := range strings.Split(req.TTSVoices, ",") {
				parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
				if len(parts) == 2 {
					voices = append(voices, model.VoiceProfile{
						ID: strings.TrimSpace(parts[1]), Name: strings.TrimSpace(parts[0]),
						VoiceID: strings.TrimSpace(parts[1]), Engine: req.TTSName, Source: "preset",
					})
				}
			}
		}
		if len(voices) == 0 {
			voices = append(voices, model.VoiceProfile{
				ID: "default", Name: "Default", VoiceID: "alloy", Engine: req.TTSName, Source: "preset",
			})
		}
		eng := tts.NewCustomEngine(tts.CustomEngineConfig{
			Name: req.TTSName, BaseURL: req.TTSURL, APIKey: req.TTSKey,
			Model: req.TTSModel, Voices: voices,
		})
		s.customEngines[req.TTSName] = eng
		s.engineReg.Register(req.TTSName, eng)
	}

	// Persist full settings to disk
	s.persistSettings(req)
	log.Printf("💾 Settings saved to data/runtime_settings.json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// persistSettings writes runtime settings to disk with full custom TTS details
func (s *Server) persistSettings(req struct {
	ApiKey    string `json:"api_key"`
	LLMURL    string `json:"llm_url"`
	LLMKey    string `json:"llm_key"`
	LLMModel  string `json:"llm_model"`
	TTSName   string `json:"tts_name"`
	TTSURL    string `json:"tts_url"`
	TTSKey    string `json:"tts_key"`
	TTSModel  string `json:"tts_model"`
	TTSVoices string `json:"tts_voices"`
}) {
	s.llmMu.Lock()
	llmCfg := s.llmCfg
	s.llmMu.Unlock()

	llmBaseURL := ""
	llmAPIKey := ""
	llmModel := ""
	if llmCfg != nil {
		llmBaseURL = llmCfg.BaseURL
		llmAPIKey = llmCfg.APIKey
		llmModel = llmCfg.Model
	}

	// Build custom TTS array with full config
	var customTTS []map[string]string
	for name := range s.customEngines {
		cfg := map[string]string{"name": name}

		// Preserve previously saved config for engines not in the current request
		if name == req.TTSName {
			cfg["url"] = req.TTSURL
			cfg["key"] = req.TTSKey
			cfg["model"] = req.TTSModel
			cfg["voices"] = req.TTSVoices
		} else {
			// Look up from existing persisted data
			existing := s.lookupCustomTTSInfo(name)
			cfg["url"] = existing.URL
			cfg["model"] = existing.Model
			cfg["voices"] = existing.Voices
			cfg["key"] = existing.RawKey
			// Don't overwrite key with empty — keep existing if not being updated
		}
		customTTS = append(customTTS, cfg)
	}

	rtData, _ := json.Marshal(map[string]any{
		"mimo_api_key": s.cfg.MiMo.APIKey,
		"llm_base_url": llmBaseURL,
		"llm_api_key":  llmAPIKey,
		"llm_model":    llmModel,
		"custom_tts":   customTTS,
	})
	os.WriteFile("./data/runtime_settings.json", rtData, 0644)
}

// maskKey returns a masked version of an API key for display
func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

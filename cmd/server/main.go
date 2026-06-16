package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ebook-audiobook/internal/ai"
	"ebook-audiobook/internal/api"
	"ebook-audiobook/internal/config"
	"ebook-audiobook/internal/llm"
	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/parser"
	"ebook-audiobook/internal/storage"
	"ebook-audiobook/internal/synthesis"
	"ebook-audiobook/internal/tts"
)

func parsePairs(s string) [][2]string {
	var out [][2]string
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			out = append(out, [2]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])})
		}
	}
	return out
}

// runtimeSettings is loaded from data/runtime_settings.json
type runtimeSettings struct {
	MiMoAPIKey string   `json:"mimo_api_key"`
	LLMBaseURL string   `json:"llm_base_url"`
	LLMAPIKey  string   `json:"llm_api_key"`
	LLMModel   string   `json:"llm_model"`
	CustomTTS  []ttsCfg `json:"custom_tts"`
}
type ttsCfg struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Key    string `json:"key"`
	Model  string `json:"model"`
	Voices string `json:"voices"`
}

func loadRuntimeSettings(path string) *runtimeSettings {
	s := &runtimeSettings{}
	data, err := os.ReadFile(filepath.Join(path, "runtime_settings.json"))
	if err != nil {
		return s
	}
	json.Unmarshal(data, s)
	return s
}

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if key := os.Getenv("MIMO_API_KEY"); key != "" {
		cfg.MiMo.APIKey = key
	}
	for _, dir := range []string{cfg.Storage.UploadDir, cfg.Storage.OutputDir, filepath.Dir(cfg.Storage.DBPath)} {
		os.MkdirAll(dir, 0755)
	}

	store, err := storage.New(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	parserReg := parser.NewRegistry()
	engineReg := tts.NewEngineRegistry()
	mimoEngine := tts.NewMiMoEngine(cfg.MiMo.APIKey, cfg.MiMo.BaseURL)
	engineReg.Register("mimo", mimoEngine)
	rtvcEngine := tts.NewRTVCEngine(cfg.RTVC.PythonPath, cfg.RTVC.ProjectDir, cfg.RTVC.Enabled)
	engineReg.Register("rtvc", rtvcEngine)

	synthMgr := synthesis.NewManager(store, engineReg, cfg.Storage.OutputDir,
		cfg.Synthesis.MaxConcurrency, cfg.Synthesis.DefaultFormat,
		cfg.Synthesis.SampleRate, cfg.Synthesis.ChapterGap)

	// Restore persisted settings
	rs := loadRuntimeSettings("./data")
	var llmCfg *llm.Config
	var llmClient *llm.Client
	if rs.MiMoAPIKey != "" {
		cfg.MiMo.APIKey = rs.MiMoAPIKey
		mimoEngine = tts.NewMiMoEngine(rs.MiMoAPIKey, cfg.MiMo.BaseURL)
		engineReg.Register("mimo", mimoEngine)
		log.Printf("🔑 Restored MiMo API key from disk")
	}
	if rs.LLMAPIKey != "" {
		llmCfg = &llm.Config{BaseURL: rs.LLMBaseURL, APIKey: rs.LLMAPIKey, Model: rs.LLMModel}
		llmClient = llm.NewClient(*llmCfg)
		log.Printf("🧠 Restored LLM config: %s (model: %s)", rs.LLMBaseURL, rs.LLMModel)
	}
	for _, ttc := range rs.CustomTTS {
		var voices []model.VoiceProfile
		for _, pair := range parsePairs(ttc.Voices) {
			voices = append(voices, model.VoiceProfile{
				ID: pair[1], Name: pair[0], VoiceID: pair[1],
				Engine: ttc.Name, Source: "preset",
			})
		}
		if len(voices) == 0 {
			voices = append(voices, model.VoiceProfile{ID: "default", Name: "Default", VoiceID: "alloy", Engine: ttc.Name, Source: "preset"})
		}
		engineReg.Register(ttc.Name, tts.NewCustomEngine(tts.CustomEngineConfig{
			Name: ttc.Name, BaseURL: ttc.URL, APIKey: ttc.Key, Model: ttc.Model, Voices: voices,
		}))
	}

	aiProd := ai.NewOrchestrator(store, llmClient, engineReg, cfg.Storage.OutputDir, cfg.Synthesis.SampleRate)
	server := api.NewServer(cfg, store, parserReg, engineReg, synthMgr, aiProd, llmCfg)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	// Create HTTP server with timeouts
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.Handler(),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 300 * time.Second, // long timeout for file downloads
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("🛑 Received signal: %v, shutting down gracefully...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("⚠️  HTTP server forced to shutdown: %v", err)
		}
		log.Println("✅ Server stopped")
	}()

	log.Printf("🚀 Audiobook Factory starting on %s", addr)
	log.Printf("📖 Supported formats: %v", parserReg.SupportedFormats())
	log.Printf("🎤 TTS engines: mimo, rtvc")
	log.Printf("🧠 AI production: ready (configure LLM in Settings)")
	log.Printf("💡 Health check: http://%s/health", addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

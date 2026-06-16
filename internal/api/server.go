package api

import (
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"ebook-audiobook/internal/ai"
	"ebook-audiobook/internal/config"
	"ebook-audiobook/internal/llm"
	"ebook-audiobook/internal/parser"
	"ebook-audiobook/internal/storage"
	"ebook-audiobook/internal/synthesis"
	"ebook-audiobook/internal/tts"
)

// Server wraps the HTTP server and all its dependencies
type Server struct {
	cfg       *config.Config
	store     *storage.Store
	parserReg *parser.Registry
	engineReg *tts.EngineRegistry
	synthMgr  *synthesis.Manager
	aiProd    *ai.Orchestrator
	llmCfg    *llm.Config
	llmMu     sync.Mutex

	customEngines map[string]*tts.CustomEngine
	router        chi.Router
}

// NewServer creates a new API server with all dependencies
func NewServer(cfg *config.Config, store *storage.Store, parserReg *parser.Registry,
	engineReg *tts.EngineRegistry, synthMgr *synthesis.Manager,
	aiProd *ai.Orchestrator, llmCfg *llm.Config) *Server {

	s := &Server{
		cfg:           cfg,
		store:         store,
		parserReg:     parserReg,
		engineReg:     engineReg,
		synthMgr:      synthMgr,
		aiProd:        aiProd,
		llmCfg:        llmCfg,
		customEngines: make(map[string]*tts.CustomEngine),
	}
	s.setupRouter()
	return s
}

// Handler returns the HTTP handler for the server
func (s *Server) Handler() http.Handler {
	return s.router
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() {
	log.Println("🛑 Server shutting down...")
	// Cancel all running jobs
}

package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func (s *Server) setupRouter() {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(chimw.Timeout(5 * time.Minute))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Readiness check (includes dependency status)
	r.Get("/health/ready", s.healthReady)

	// Direct import endpoint (from configured path)
	r.Get("/import-now", s.directImportNow)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Settings
		r.Get("/settings", s.getSettings)
		r.Put("/settings", s.updateSettings)

		// Books
		r.Route("/books", func(r chi.Router) {
			r.Get("/", s.listBooks)
			r.Post("/upload", s.uploadBook)
			r.Get("/{id}", s.getBook)
			r.Put("/{id}/chapters", s.updateBookChapters)
			r.Delete("/{id}", s.deleteBook)
		})

		// Voices
		r.Route("/voices", func(r chi.Router) {
			r.Get("/", s.listVoices)
			r.Get("/presets", s.listPresetVoices)
			r.Post("/", s.createVoice)
			r.Get("/{id}", s.getVoice)
			r.Delete("/{id}", s.deleteVoice)
			r.Post("/{id}/preview", s.previewVoice)
			r.Post("/{id}/evaluate", s.evaluateVoice)
		})

		// Synthesis jobs
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/", s.listJobs)
			r.Post("/", s.createJob)
			r.Get("/{id}", s.getJob)
			r.Delete("/{id}", s.deleteJob)
			r.Get("/{id}/progress", s.streamJobProgress)
			r.Get("/{id}/download", s.downloadOutput)
		})

		// Quick synthesis
		r.Post("/synthesis/single", s.synthesizeSingle)
		r.Post("/synthesis/mix", s.mixAudio)

		// AI Production
		r.Post("/ai/analyze/{id}", s.aiAnalyzeBook)
		r.Post("/ai/produce/{id}", s.aiProduceBook)

		// LLM proxy
		r.Post("/ai/llm-proxy", s.llmProxy)

		// Formats
		r.Get("/formats", s.supportedFormats)

		// PDF classification
		r.Post("/pdf/classify", s.classifyPDF)

		// Library import
		r.Post("/library/import", s.importLibrary)
	})

	s.router = r
}

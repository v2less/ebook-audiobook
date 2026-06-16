package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"ebook-audiobook/internal/ai"
	"ebook-audiobook/internal/llm"
)

// ---- AI Production handlers ----

// llmProxy proxies LLM requests from browser to LLM API
func (s *Server) llmProxy(w http.ResponseWriter, r *http.Request) {
	s.llmMu.Lock()
	if s.llmCfg == nil || s.llmCfg.APIKey == "" {
		s.llmMu.Unlock()
		writeError(w, http.StatusBadRequest, fmt.Errorf("LLM not configured"))
		return
	}
	cfg := *s.llmCfg
	s.llmMu.Unlock()

	var req struct {
		SystemPrompt string `json:"system_prompt"`
		UserPrompt   string `json:"user_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	client := llm.NewClient(cfg)
	reply, err := client.Chat(r.Context(), req.SystemPrompt, req.UserPrompt)
	if err != nil {
		log.Printf("❌ LLM proxy error: %v", err)
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if len(reply) == 0 || strings.TrimSpace(reply) == "" {
		log.Printf("⚠️  LLM returned empty response (model: %s)", cfg.Model)
		writeError(w, http.StatusBadGateway, fmt.Errorf("LLM returned empty response — check API key, model availability, or prompt content"))
		return
	}

	log.Printf("📨 LLM response: %d bytes, preview: %.200s", len(reply), reply)
	writeJSON(w, map[string]string{"reply": reply})
}

func (s *Server) aiAnalyzeBook(w http.ResponseWriter, r *http.Request) {
	s.llmMu.Lock()
	if s.llmCfg == nil || s.llmCfg.APIKey == "" {
		s.llmMu.Unlock()
		writeError(w, http.StatusBadRequest, fmt.Errorf("请先在设置中配置 LLM API Key 和 Base URL"))
		return
	}
	s.llmMu.Unlock()

	bookID := chi.URLParam(r, "id")
	book, err := s.store.GetBook(bookID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	if len(book.Chapters) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("book has no chapters"))
		return
	}

	// Analyze first chapter to demonstrate
	analyzer := ai.NewAnalyzer(llm.NewClient(*s.llmCfg))
	analysis, err := analyzer.AnalyzeChapter(r.Context(), book.Chapters[0], nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, analysis)
}

func (s *Server) aiProduceBook(w http.ResponseWriter, r *http.Request) {
	s.llmMu.Lock()
	if s.llmCfg == nil || s.llmCfg.APIKey == "" {
		s.llmMu.Unlock()
		writeError(w, http.StatusBadRequest, fmt.Errorf("请先在设置中配置 LLM API Key"))
		return
	}
	if s.cfg.MiMo.APIKey == "" {
		s.llmMu.Unlock()
		writeError(w, http.StatusBadRequest, fmt.Errorf("请先在设置中配置 MiMo API Key"))
		return
	}
	s.llmMu.Unlock()

	bookID := chi.URLParam(r, "id")

	// Create synthesis job
	jobID := fmt.Sprintf("ai-%s", bookID)

	go func() {
		_, err := s.aiProd.ProduceAudiobook(context.Background(), bookID, jobID)
		if err != nil {
			log.Printf("AI production error: %v", err)
		}
	}()

	writeJSON(w, map[string]string{
		"status": "started",
		"job_id": jobID,
	})
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"ebook-audiobook/internal/model"
)

// ---- Job handlers ----

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if jobs == nil {
		jobs = []*model.SynthesisJob{}
	}
	writeJSON(w, jobs)
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookID string          `json:"book_id"`
		Config model.JobConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.Config.DefaultVoiceID == "" {
		req.Config.DefaultVoiceID = "mimo_default"
	}
	if req.Config.OutputFormat == "" {
		req.Config.OutputFormat = s.cfg.Synthesis.DefaultFormat
	}

	job, err := s.synthMgr.StartJob(req.BookID, req.Config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, job)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	segs, _ := s.store.GetJobSegments(job.ID)
	for _, seg := range segs {
		if seg.Status == "failed" && job.Error == "" {
			job.Error = seg.Error
			break
		}
	}
	writeJSON(w, job)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.store.GetJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if job.Status == model.JobRunning || job.Status == model.JobMerging {
		s.synthMgr.CancelJob(id)
	}
	// Cancel first then delete
	s.store.UpdateJobStatus(id, model.JobCancelled, "deleted")
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) streamJobProgress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	ch := s.synthMgr.SubscribeProgress(id)
	defer s.synthMgr.UnsubscribeProgress(id, ch)

	// Send current progress immediately
	if job, err := s.store.GetJob(id); err == nil {
		data, _ := json.Marshal(job.Progress)
		fmt.Fprintf(w, "data: {\"progress\":%s}\n\n", data)
		flusher.Flush()
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case progress, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(progress)
			fmt.Fprintf(w, "data: {\"progress\":%s}\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) downloadOutput(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if job.Status != model.JobCompleted {
		writeError(w, http.StatusBadRequest, fmt.Errorf("job not completed"))
		return
	}

	w.Header().Set("Content-Type", "audio/"+job.OutputFormat)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"audiobook.%s\"", job.OutputFormat))
	http.ServeFile(w, r, job.OutputPath)
}

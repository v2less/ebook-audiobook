package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/tts"
)

// ---- Quick Synthesis handlers ----

func (s *Server) synthesizeSingle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text        string `json:"text"`
		VoiceID     string `json:"voice_id"`
		EmotionHint string `json:"emotion_hint"`
		Format      string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Format == "" {
		req.Format = "wav"
	}
	if req.VoiceID == "" {
		req.VoiceID = "mimo_default"
	}

	// Try DB first
	vp, err := s.store.GetVoiceProfile(req.VoiceID)
	if err != nil {
		// Try preset voices
		for _, p := range tts.MiMoPresetVoices {
			if p.ID == req.VoiceID || p.VoiceID == req.VoiceID {
				c := p
				vp = &c
				break
			}
		}
	}

	if vp == nil {
		// Fallback
		vp = &model.VoiceProfile{ID: req.VoiceID, Engine: "mimo", Source: "preset", VoiceID: req.VoiceID}
	}

	opts := model.TTSOptions{
		VoiceID:        vp.VoiceID,
		Format:         req.Format,
		StyleDirective: req.EmotionHint,
	}
	audio, format, err := s.engineReg.SynthesizeWithEngine(r.Context(), req.Text, vp, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "audio/"+format)
	w.Write(audio)
}

func (s *Server) mixAudio(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Script []struct {
			Index     int    `json:"index"`
			Type      string `json:"type"`
			Speaker   string `json:"speaker"`
			Text      string `json:"text"`
			AudioURL  string `json:"audio_url"`
			AudioData string `json:"audio_data"` // base64-encoded audio (preferred over audio_url)
			Emotion   string `json:"emotion"`
			SFX       []struct {
				Keyword  string  `json:"keyword"`
				Name     string  `json:"name"`
				Position float64 `json:"position"`
			} `json:"sfx"`
		} `json:"script"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.Format == "" {
		req.Format = "wav"
	}

	tmpDir, err := os.MkdirTemp("", "mix-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(tmpDir)

	var ffmpegInputs []string
	var filterParts []string
	inputIdx := 0

	for i, seg := range req.Script {
		if seg.Type != "dialogue" {
			continue
		}

		fpath := filepath.Join(tmpDir, fmt.Sprintf("seg_%04d.wav", i))

		// Try base64 audio_data first (browser-friendly)
		if seg.AudioData != "" {
			audioBytes, err := base64.StdEncoding.DecodeString(seg.AudioData)
			if err != nil {
				continue // skip corrupted data
			}
			if err := os.WriteFile(fpath, audioBytes, 0644); err != nil {
				continue
			}
		} else if seg.AudioURL != "" {
			// Fallback: try fetching from URL (for backward compatibility)
			resp, err := http.Get(seg.AudioURL)
			if err != nil {
				continue
			}
			f, _ := os.Create(fpath)
			io.Copy(f, resp.Body)
			f.Close()
			resp.Body.Close()
		} else {
			continue // no audio data
		}

		ffmpegInputs = append(ffmpegInputs, "-i", fpath)
		filterParts = append(filterParts, fmt.Sprintf("[%d:a]", inputIdx))
		inputIdx++
	}

	// Build concat filter
	if len(filterParts) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no valid audio segments — ensure each line has generated audio"))
		return
	}
	concatFilter := strings.Join(filterParts, "") +
		fmt.Sprintf("concat=n=%d:v=0:a=1[out]", len(filterParts))

	outputPath := filepath.Join(tmpDir, "output."+req.Format)
	args := append(ffmpegInputs, "-filter_complex", concatFilter, "-map", "[out]", "-y", outputPath)
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("ffmpeg: %s", string(out)))
		return
	}

	data, _ := os.ReadFile(outputPath)
	w.Header().Set("Content-Type", "audio/"+req.Format)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"audiobook.%s\"", req.Format))
	w.Write(data)
}

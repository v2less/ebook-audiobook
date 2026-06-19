package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/tts"
)

// ---- Voice handlers ----

func (s *Server) listVoices(w http.ResponseWriter, r *http.Request) {
	custom, _ := s.store.ListVoiceProfiles()
	if custom == nil {
		custom = []*model.VoiceProfile{}
	}
	writeJSON(w, custom)
}

func (s *Server) listPresetVoices(w http.ResponseWriter, r *http.Request) {
	all := make([]model.VoiceProfile, len(tts.MiMoPresetVoices))
	copy(all, tts.MiMoPresetVoices)

	// Add voices from custom engines
	for _, engine := range s.customEngines {
		voices, _ := engine.ListVoices()
		for i := range voices {
			all = append(all, voices[i])
		}
	}
	writeJSON(w, all)
}

func (s *Server) createVoice(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (for file upload)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse form: %w", err))
		return
	}

	vp := model.VoiceProfile{
		Name:         r.FormValue("name"),
		Source:       r.FormValue("source"),
		Engine:       r.FormValue("engine"),
		VoiceID:      r.FormValue("voice_id"),
		DesignPrompt: r.FormValue("design_prompt"),
		Description:  r.FormValue("description"),
		Language:     "zh-CN",
	}

	if vp.Source == "" {
		vp.Source = "preset"
	}
	if vp.Engine == "" {
		vp.Engine = "mimo"
	}

	// Handle voice file upload (for clone)
	file, header, ferr := r.FormFile("voice_file")
	if ferr == nil {
		defer file.Close()
		uploadDir := s.cfg.Storage.UploadDir
		os.MkdirAll(uploadDir, 0755)
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".mp3"
		}
		samplePath := filepath.Join(uploadDir, "voice-"+header.Filename)
		dst, err := os.Create(samplePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if _, err := io.Copy(dst, file); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		dst.Close()
		vp.SamplePath = samplePath
	}

	if err := s.store.SaveVoiceProfile(&vp); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	log.Printf("🎤 Voice created: name=%s source=%s engine=%s sample=%s", vp.Name, vp.Source, vp.Engine, vp.SamplePath)
	writeJSON(w, vp)
}

func (s *Server) getVoice(w http.ResponseWriter, r *http.Request) {
	vp, err := s.store.GetVoiceProfile(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, vp)
}

func (s *Server) deleteVoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	vp, _ := s.store.GetVoiceProfile(id)
	if err := s.store.DeleteVoiceProfile(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if vp != nil && vp.SamplePath != "" {
		os.Remove(vp.SamplePath)
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) previewVoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Look up: custom voices in DB first, then preset voices
	vp, err := s.store.GetVoiceProfile(id)
	if err != nil {
		// Try preset voices
		for _, p := range tts.MiMoPresetVoices {
			if p.ID == id || p.VoiceID == id {
				c := p
				vp = &c
				break
			}
		}
	}
	if vp == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("voice not found: %s", id))
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Text == "" {
		req.Text = "你好，这是音色预览。"
	}

	// Validate API key for MiMo engine
	if vp.Engine == "mimo" && s.cfg.MiMo.APIKey == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请设置环境变量 MIMO_API_KEY（小米 MiMo API Key）"))
		return
	}

	audio, format, err := s.engineReg.SynthesizeWithEngine(r.Context(), req.Text, vp, model.TTSOptions{Format: "wav"})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "429") || strings.Contains(msg, "Too many") {
			msg = "MiMo API 限流，请稍等几秒后重试"
		}
		writeError(w, http.StatusTooManyRequests, fmt.Errorf(msg))
		return
	}

	w.Header().Set("Content-Type", "audio/"+format)
	w.Write(audio)
}

// evaluateVoice runs voice quality evaluation
func (s *Server) evaluateVoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	vp, err := s.store.GetVoiceProfile(id)
	if err != nil {
		// Try preset voices
		for _, p := range tts.MiMoPresetVoices {
			if p.ID == id || p.VoiceID == id {
				c := p
				vp = &c
				break
			}
		}
	}
	if vp == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("voice not found: %s", id))
		return
	}

	// Validate API key for MiMo engine
	if vp.Engine == "mimo" && s.cfg.MiMo.APIKey == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请先设置 MiMo API Key"))
		return
	}

	evaluator := tts.NewVoiceEvaluator(s.engineReg)
	report, err := evaluator.Evaluate(r.Context(), vp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, report)
}

// generateVoiceDesign 根据文字描述（voice_design prompt）自动生成音色
// 两步策略：先用 voicedesign 生成参考音频保存为 sample，后续合成走 voiceclone
func (s *Server) generateVoiceDesign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DesignPrompt string `json:"design_prompt"`
		PreviewText  string `json:"preview_text"`
		SaveAsName   string `json:"save_as_name"`
		Gender       string `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.DesignPrompt == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("design_prompt 不能为空"))
		return
	}
	if req.PreviewText == "" {
		req.PreviewText = "你好，这是自动生成的角色音色预览。"
	}
	if req.SaveAsName == "" {
		req.SaveAsName = "auto-voice"
	}

	// Validate MiMo API key
	if s.cfg.MiMo.APIKey == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请先设置 MiMo API Key"))
		return
	}

	mimoEngine, err := s.engineReg.Get("mimo")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("MiMo 引擎未注册"))
		return
	}
	mimo, ok := mimoEngine.(*tts.MiMoEngine)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("MiMo 引擎类型错误"))
		return
	}

	// Step 1: 调用 voicedesign 生成参考音频
	audioData, format, err := mimo.SynthesizeDesign(r.Context(), req.PreviewText, req.DesignPrompt, model.TTSOptions{Format: "wav"})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "429") || strings.Contains(msg, "Too many") {
			msg = "MiMo API 限流，请稍等几秒后重试"
		}
		writeError(w, http.StatusTooManyRequests, fmt.Errorf(msg))
		return
	}

	// Step 2: 保存参考音频到磁盘
	uploadDir := s.cfg.Storage.UploadDir
	os.MkdirAll(uploadDir, 0755)
	sampleFileName := fmt.Sprintf("generated-%s-%d.%s", req.SaveAsName, time.Now().UnixMilli(), format)
	samplePath := filepath.Join(uploadDir, sampleFileName)
	if err := os.WriteFile(samplePath, audioData, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存参考音频失败: %w", err))
		return
	}

	// Step 3: 创建 source="generated" 的 VoiceProfile
	vp := model.VoiceProfile{
		Name:         req.SaveAsName,
		Source:       "generated",
		Engine:       "mimo",
		SamplePath:   samplePath,
		DesignPrompt: req.DesignPrompt,
		Description:  fmt.Sprintf("自动生成: %s", req.DesignPrompt),
		Language:     "zh-CN",
		Gender:       req.Gender,
	}
	if err := s.store.SaveVoiceProfile(&vp); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	log.Printf("🎤 Voice generated: name=%s id=%s sample=%s", vp.Name, vp.ID, samplePath)

	// 返回 profile + base64 audio 供前端预览
	audioB64 := base64.StdEncoding.EncodeToString(audioData)
	writeJSON(w, map[string]any{
		"voice_profile": vp,
		"audio_data":    audioB64,
		"audio_format":  format,
	})
}


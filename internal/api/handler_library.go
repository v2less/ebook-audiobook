package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/model"
)

// ---- Library & Format handlers ----

func (s *Server) supportedFormats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"input_formats":  s.parserReg.SupportedFormats(),
		"output_formats": []string{"mp3", "wav", "m4b", "ogg", "flac", "opus"},
	})
}

func (s *Server) importLibrary(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse form: %w", err))
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read file: %w", err))
		return
	}
	defer file.Close()

	// Save project JSON
	projPath := filepath.Join(s.cfg.Storage.UploadDir, "_import_project.json")
	dst, err := os.Create(projPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	io.Copy(dst, file)
	dst.Close()

	result, err := s.importProjectJSON(projPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, result)
}

// getProjectJSONPath returns the configured project JSON path with fallbacks
func (s *Server) getProjectJSONPath() string {
	// 1. Config file
	if s.cfg.Library.ProjectPath != "" {
		if _, err := os.Stat(s.cfg.Library.ProjectPath); err == nil {
			return s.cfg.Library.ProjectPath
		}
	}
	// 2. Environment variable
	if envPath := os.Getenv("PROJECT_JSON_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}
	// 3. Default known locations (backward compatibility)
	candidates := []string{
		"./初始音效工程文件【加载音效音色】.json",
		filepath.Join(s.cfg.Storage.UploadDir, "_import_project.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// importProjectJSON parses Unitale project JSON and extracts SFX/BGM/voices inline
func (s *Server) importProjectJSON(jsonPath string) (map[string]any, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read project file: %w", err)
	}

	var proj struct {
		Libraries struct {
			SFX     []json.RawMessage `json:"sfx"`
			BGM     []json.RawMessage `json:"bgm"`
			Voices  []json.RawMessage `json:"voices"`
			Timbres []json.RawMessage `json:"timbres"` // Unitale uses "timbres" for voices
		} `json:"libraries"`
	}
	if err := json.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parse project: %w", err)
	}

	// Merge timbres into voices (Unitale uses "timbres" key)
	if len(proj.Libraries.Timbres) > 0 && len(proj.Libraries.Voices) == 0 {
		proj.Libraries.Voices = proj.Libraries.Timbres
	}

	result := map[string]any{
		"sfx_count":    0,
		"bgm_count":    0,
		"voice_count":  0,
		"errors_count": 0,
	}

	extract := func(items []json.RawMessage, dir string, isVoice bool) int {
		os.MkdirAll(dir, 0755)
		count := 0
		for _, item := range items {
			var res struct {
				Name     string `json:"name"`
				Filename string `json:"filename"`
				RefPath  string `json:"refPath"`
				FileData string `json:"_fileData"`
			}
			json.Unmarshal(item, &res)

			// Use refPath as fallback for filename (Unitale timbres use refPath)
			filename := res.Filename
			if filename == "" {
				filename = res.RefPath
			}
			if filename == "" || res.FileData == "" {
				continue
			}

			outPath := filepath.Join(dir, filename)
			if _, err := os.Stat(outPath); err == nil {
				count++
				continue
			}
			// Decode base64 data URI
			dataStr := res.FileData
			if idx := strings.Index(dataStr, "base64,"); idx >= 0 {
				dataStr = dataStr[idx+7:]
			}
			audioBytes, err := base64.StdEncoding.DecodeString(dataStr)
			if err != nil {
				continue
			}
			if os.WriteFile(outPath, audioBytes, 0644) == nil {
				count++
				if isVoice && s.store != nil {
					name := res.Name
					if name == "" {
						name = filename
					}
					// Remove extension for the name
					name = strings.TrimSuffix(name, filepath.Ext(name))
					vp := &model.VoiceProfile{
						Name:       name,
						Source:     "clone",
						Engine:     "mimo",
						SamplePath: outPath,
						Language:   "zh-CN",
					}
					s.store.SaveVoiceProfile(vp)
				}
			}
		}
		return count
	}

	result["sfx_count"] = extract(proj.Libraries.SFX, "./data/sfx", false)
	result["bgm_count"] = extract(proj.Libraries.BGM, "./data/bgm", false)
	result["voice_count"] = extract(proj.Libraries.Voices, "./data/voices", true)

	return result, nil
}

func (s *Server) directImportNow(w http.ResponseWriter, r *http.Request) {
	projPath := s.getProjectJSONPath()
	if projPath == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("project json path not configured, set library.project_path in config.yaml or PROJECT_JSON_PATH env"))
		return
	}
	// Check file exists
	if _, err := os.Stat(projPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Errorf("project file not found: %s (skipping import)", projPath))
		return
	}
	result, err := s.importProjectJSON(projPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, result)
}

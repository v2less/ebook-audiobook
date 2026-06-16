package library

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectFile is the Unitale project JSON structure
type ProjectFile struct {
	Version   string       `json:"version"`
	Timestamp string       `json:"timestamp"`
	Libraries Libraries    `json:"libraries"`
}

// Libraries contains SFX, BGM, and Voice resources
type Libraries struct {
	SFX    []Resource `json:"sfx"`
	BGM    []Resource `json:"bgm"`
	Voices []Resource `json:"voices"`
}

// Resource is a single audio resource entry
type Resource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Filename    string `json:"filename"`
	Enabled     bool   `json:"enabled"`
	FileData    string `json:"_fileData"` // data:audio/mpeg;base64,...
}

// ImportResult contains counts of imported resources
type ImportResult struct {
	SFXCount    int `json:"sfx_count"`
	BGMCount    int `json:"bgm_count"`
	VoiceCount  int `json:"voice_count"`
	ErrorsCount int `json:"errors_count"`
	Errors      []string `json:"errors,omitempty"`
}

// ImportProject imports a Unitale project JSON file and extracts all resources
func ImportProject(jsonPath, baseDir string) (*ImportResult, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read project file: %w", err)
	}

	var proj ProjectFile
	if err := json.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parse project JSON: %w", err)
	}

	result := &ImportResult{}

	// Import SFX
	sfxDir := filepath.Join(baseDir, "data", "sfx")
	os.MkdirAll(sfxDir, 0755)
	for _, r := range proj.Libraries.SFX {
		if err := extractResource(r, sfxDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("SFX %s: %v", r.Name, err))
			result.ErrorsCount++
		} else {
			result.SFXCount++
		}
	}

	// Import BGM
	bgmDir := filepath.Join(baseDir, "data", "bgm")
	os.MkdirAll(bgmDir, 0755)
	for _, r := range proj.Libraries.BGM {
		if err := extractResource(r, bgmDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("BGM %s: %v", r.Name, err))
			result.ErrorsCount++
		} else {
			result.BGMCount++
		}
	}

	// Import Voices
	voiceDir := filepath.Join(baseDir, "data", "voices")
	os.MkdirAll(voiceDir, 0755)
	for _, r := range proj.Libraries.Voices {
		if err := extractResource(r, voiceDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Voice %s: %v", r.Name, err))
			result.ErrorsCount++
		} else {
			result.VoiceCount++
		}
	}

	// Write a metadata index for SFX/BGM lookup
	writeIndex(sfxDir, proj.Libraries.SFX, "sfx_index.json")
	writeIndex(bgmDir, proj.Libraries.BGM, "bgm_index.json")

	return result, nil
}

// extractResource decodes a base64 data URI and saves to disk
func extractResource(r Resource, dir string) error {
	if r.Filename == "" {
		return fmt.Errorf("no filename")
	}

	outPath := filepath.Join(dir, r.Filename)
	if _, err := os.Stat(outPath); err == nil {
		return nil // already exists
	}

	if r.FileData == "" {
		return fmt.Errorf("no audio data")
	}

	// Parse data URI: data:audio/mpeg;base64,XXXXX
	dataStr := r.FileData
	if idx := strings.Index(dataStr, "base64,"); idx >= 0 {
		dataStr = dataStr[idx+7:]
	}

	audioBytes, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}

	return os.WriteFile(outPath, audioBytes, 0644)
}

// writeIndex writes a JSON index file for quick keyword lookup
func writeIndex(dir string, resources []Resource, filename string) {
	type indexEntry struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Filename    string `json:"filename"`
	}
	var entries []indexEntry
	for _, r := range resources {
		entries = append(entries, indexEntry{
			Name:        r.Name,
			Description: r.Description,
			Filename:    r.Filename,
		})
	}
	data, _ := json.Marshal(entries)
	os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

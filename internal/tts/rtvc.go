package tts

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/model"
)

// RTVCEngine wraps Real-Time-Voice-Cloning as a local TTS fallback
type RTVCEngine struct {
	pythonPath string
	projectDir string
	enabled    bool
}

func NewRTVCEngine(pythonPath, projectDir string, enabled bool) *RTVCEngine {
	return &RTVCEngine{
		pythonPath: pythonPath,
		projectDir: projectDir,
		enabled:    enabled,
	}
}

func (e *RTVCEngine) Name() string             { return "RTVC" }
func (e *RTVCEngine) SupportsVoiceClone() bool  { return true }
func (e *RTVCEngine) SupportsVoiceDesign() bool { return false }

func (e *RTVCEngine) ListVoices() ([]model.VoiceProfile, error) {
	if !e.enabled {
		return nil, nil
	}

	// Scan for saved encoder outputs (.npy files)
	var profiles []model.VoiceProfile
	encoderDir := filepath.Join(e.projectDir, "encoder", "saved_models")
	entries, err := os.ReadDir(encoderDir)
	if err != nil {
		return nil, nil
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "default" {
			profiles = append(profiles, model.VoiceProfile{
				ID:       "rtvc-" + entry.Name(),
				Name:     entry.Name(),
				Source:   "clone",
				Engine:   "rtvc",
				Language: "en-US",
			})
		}
	}
	return profiles, nil
}

// Synthesize via RTVC CLI
func (e *RTVCEngine) Synthesize(ctx context.Context, text string, opts model.TTSOptions) ([]byte, string, error) {
	if !e.enabled {
		return nil, "", fmt.Errorf("RTVC engine is disabled")
	}

	tmpDir, err := os.MkdirTemp("", "rtvc-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(tmpDir)

	// Write text to a file (RTVC CLI reads from args but long text is better via file)
	// Actually demo_cli.py takes inline text - we call it directly
	outputPath := filepath.Join(tmpDir, "output.wav")

	// Build command
	args := []string{
		filepath.Join(e.projectDir, "demo_cli.py"),
		"--text", text,
		"--out_path", outputPath,
	}
	if opts.VoiceID != "" {
		// VoiceID corresponds to a saved encoder embedding name
		args = append(args, "--voice", strings.TrimPrefix(opts.VoiceID, "rtvc-"))
	}

	cmd := exec.CommandContext(ctx, e.pythonPath, args...)
	cmd.Dir = e.projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", fmt.Errorf("rtvc synthesize: %s: %w", string(out), err)
	}

	audioData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, "", fmt.Errorf("read rtvc output: %w", err)
	}

	return audioData, "wav", nil
}

// SynthesizeStream is not supported by RTVC in this wrapper
func (e *RTVCEngine) SynthesizeStream(ctx context.Context, text string, opts model.TTSOptions, callback func(chunk []byte) error) error {
	audio, _, err := e.Synthesize(ctx, text, opts)
	if err != nil {
		return err
	}
	return callback(audio)
}

// CloneVoice preprocesses a reference audio file for RTVC
func (e *RTVCEngine) CloneVoice(audioPath string, voiceName string) (*model.VoiceProfile, error) {
	if !e.enabled {
		return nil, fmt.Errorf("RTVC engine is disabled")
	}

	// Run encoder on the audio sample
	cmd := exec.Command(e.pythonPath,
		filepath.Join(e.projectDir, "encoder_preprocess.py"),
		"--in_dir", filepath.Dir(audioPath),
		"--out_dir", filepath.Join(e.projectDir, "encoder", "saved_models", voiceName),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("rtvc encoder: %s: %w", string(out), err)
	}

	return &model.VoiceProfile{
		ID:         "rtvc-" + voiceName,
		Name:       voiceName,
		Source:     "clone",
		Engine:     "rtvc",
		SamplePath: audioPath,
		Language:   "en-US",
	}, nil
}

// ---- Engine Registry ----

// Registry routes TTS requests to the appropriate engine
type EngineRegistry struct {
	engines map[string]Engine
}

func NewEngineRegistry() *EngineRegistry {
	return &EngineRegistry{engines: make(map[string]Engine)}
}

func (r *EngineRegistry) Register(name string, engine Engine) {
	r.engines[name] = engine
}

func (r *EngineRegistry) Get(name string) (Engine, error) {
	engine, ok := r.engines[name]
	if !ok {
		return nil, fmt.Errorf("unknown TTS engine: %s", name)
	}
	return engine, nil
}

// SynthesizeWithEngine smart synthesis: picks engine based on voice profile
func (r *EngineRegistry) SynthesizeWithEngine(ctx context.Context, text string, vp *model.VoiceProfile, opts model.TTSOptions) ([]byte, string, error) {
	engine, err := r.Get(vp.Engine)
	if err != nil {
		return nil, "", err
	}

	log.Printf("🎵 Synthesize: profile=%s(%s) source=%s engine=%s voiceID=%s sample=%s design=%s",
		vp.ID, vp.Name, vp.Source, vp.Engine, vp.VoiceID, vp.SamplePath, vp.DesignPrompt)

	switch vp.Source {
	case "preset":
		opts.VoiceID = vp.VoiceID
	case "clone":
		if engine.Name() == "MiMo" {
			// Read reference audio
			voiceAudio, err := os.ReadFile(vp.SamplePath)
			if err != nil {
				return nil, "", fmt.Errorf("read voice sample: %w", err)
			}
			voiceFormat := strings.TrimPrefix(filepath.Ext(vp.SamplePath), ".")

			// MiMo voiceclone only supports MP3 and WAV — convert other formats via ffmpeg
			lowerFmt := strings.ToLower(voiceFormat)
			if lowerFmt != "mp3" && lowerFmt != "wav" {
				log.Printf("🔄 Converting voice sample from %s to mp3 (MiMo only supports mp3/wav)", voiceFormat)
				converted, err := convertAudioToMP3(vp.SamplePath)
				if err != nil {
					return nil, "", fmt.Errorf("convert voice sample to mp3: %w", err)
				}
				voiceAudio = converted
				voiceFormat = "mp3"
			}

			if _, ok := engine.(*MiMoEngine); ok {
				return engine.(*MiMoEngine).SynthesizeClone(ctx, text, voiceAudio, voiceFormat, opts)
			}
		}
		opts.VoiceID = vp.VoiceID
	case "design":
		if _, ok := engine.(*MiMoEngine); ok {
			return engine.(*MiMoEngine).SynthesizeDesign(ctx, text, vp.DesignPrompt, opts)
		}
	}

	return engine.Synthesize(ctx, text, opts)
}

// convertAudioToMP3 converts an audio file to MP3 format using ffmpeg.
// This is needed because MiMo voiceclone only accepts MP3 and WAV reference audio.
func convertAudioToMP3(inputPath string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "voice-convert-*.mp3")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-acodec", "libmp3lame", "-q:a", "2", tmpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg convert: %s: %w", string(out), err)
	}

	return os.ReadFile(tmpPath)
}

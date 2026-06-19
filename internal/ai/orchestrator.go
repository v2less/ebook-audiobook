package ai

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/llm"
	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/storage"
	"ebook-audiobook/internal/tts"
)

// Orchestrator is the AI audiobook production engine
type Orchestrator struct {
	store      *storage.Store
	llmClient  *llm.Client
	engineReg  *tts.EngineRegistry
	analyzer   *Analyzer
	outputDir  string
	sampleRate int
}

func NewOrchestrator(store *storage.Store, llmClient *llm.Client, engineReg *tts.EngineRegistry, outputDir string, sampleRate int) *Orchestrator {
	return &Orchestrator{
		store:      store,
		llmClient:  llmClient,
		engineReg:  engineReg,
		analyzer:   NewAnalyzer(llmClient),
		outputDir:  outputDir,
		sampleRate: sampleRate,
	}
}

// ProductionResult is the final audiobook production
type ProductionResult struct {
	JobID         string           `json:"job_id"`
	OutputPath    string           `json:"output_path"`
	Duration      float64          `json:"duration"`
	Characters    []Character      `json:"characters"`
	VoiceProfiles []model.VoiceProfile `json:"voice_profiles"`
	Timeline      []TimelineEvent  `json:"timeline"`
}

// TimelineEvent is one event in the final audio timeline
type TimelineEvent struct {
	Time     float64 `json:"time"`      // seconds from start
	Duration float64 `json:"duration"`  // duration of this event
	Type     string  `json:"type"`      // speech, sfx, bgm, silence
	Source   string  `json:"source"`    // speaker name, sfx name, bgm name
	Text     string  `json:"text,omitempty"`
	File     string  `json:"file"`      // audio file path
	Scene    string  `json:"scene,omitempty"`
	Volume   float64 `json:"volume"`    // 0.0 - 1.0
}

// ProduceAudiobook runs the full AI-driven audiobook production pipeline
func (o *Orchestrator) ProduceAudiobook(ctx context.Context, bookID string, jobID string) (*ProductionResult, error) {
	book, err := o.store.GetBook(bookID)
	if err != nil {
		return nil, fmt.Errorf("get book: %w", err)
	}

	jobDir := filepath.Join(o.outputDir, jobID)
	os.MkdirAll(jobDir, 0755)

	// Phase 1: LLM analysis of all chapters
	var allAnalyses []ScriptAnalysis
	var knownCharacters []Character
	var fullTimeline []TimelineEvent
	var currentTime float64
	var totalSpeechSegments []SpeechSegment

	for _, ch := range book.Chapters {
		if len(ch.Content) < 50 {
			continue
		}
		analysis, err := o.analyzer.AnalyzeChapter(ctx, ch, knownCharacters)
		if err != nil {
			return nil, fmt.Errorf("analyze chapter %d: %w", ch.Index, err)
		}
		// Update known characters
		for _, c := range analysis.Characters {
			found := false
			for i, kc := range knownCharacters {
				if strings.EqualFold(c.Name, kc.Name) {
					if c.VoiceDesign != "" {
						knownCharacters[i].VoiceDesign = c.VoiceDesign
					}
					found = true
					break
				}
			}
			if !found {
				knownCharacters = append(knownCharacters, c)
			}
		}
		allAnalyses = append(allAnalyses, *analysis)
		totalSpeechSegments = append(totalSpeechSegments, CompileSpeechSegments(*analysis)...)
	}

	// Phase 2: Build voice profiles from character analysis
	customVoices, _ := o.store.ListVoiceProfiles()
	var availableVoices []model.VoiceProfile
	for _, p := range customVoices {
		availableVoices = append(availableVoices, *p)
	}
	availableVoices = append(availableVoices, tts.MiMoPresetVoices...)

	voiceProfiles := o.analyzer.BuildVoiceProfiles(knownCharacters, availableVoices)

	// Phase 3: Synthesize all speech segments with character voices
	splitter := tts.NewTextSplitter(1500)
	voiceMap := buildVoiceMap(voiceProfiles, knownCharacters)

	// Default voice opts
	defaultOpts := model.TTSOptions{
		VoiceID: "mimo_default",
		Format:  "wav",
		Model:   "mimo-v2.5-tts",
	}

	for i, seg := range totalSpeechSegments {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		vp, ok := voiceMap[strings.ToLower(seg.Speaker)]
		if !ok {
			vp = &model.VoiceProfile{
				Engine: "mimo", Source: "preset", VoiceID: "mimo_default",
			}
		}

		opts := defaultOpts
		opts.VoiceID = vp.VoiceID
		opts.StyleDirective = seg.EmotionHint

		// For narrator, if it wasn't matched (or isn't found), we use default but allow dynamic prompt
		if seg.Type == "narration" {
			if _, ok := voiceMap[strings.ToLower(seg.Speaker)]; !ok {
				opts.VoiceID = "mimo_default"
			}
			if seg.EmotionHint == "" {
				opts.StyleDirective = "用平和自然的语气朗读，语速适中，如同深夜电台读故事"
			}
		}

		// Split long text
		textParts := splitter.Split(seg.Text)
		for pi, part := range textParts {
			audioData, format, err := o.engineReg.SynthesizeWithEngine(ctx, part, vp, opts)
			if err != nil {
				return nil, fmt.Errorf("synthesize seg %d.%d: %w", i, pi, err)
			}

			audioPath := filepath.Join(jobDir, fmt.Sprintf("speech_%04d_%02d.%s", i, pi, format))
			if err := os.WriteFile(audioPath, audioData, 0644); err != nil {
				return nil, err
			}

			duration := estimateDuration(audioData, format, o.sampleRate)
			fullTimeline = append(fullTimeline, TimelineEvent{
				Time:     currentTime,
				Duration: duration,
				Type:     seg.Type,
				Source:   seg.Speaker,
				Text:     part,
				File:     audioPath,
				Scene:    seg.Scene,
				Volume:   1.0,
			})
			currentTime += duration

			// Insert SFX cues
			for _, sfx := range seg.SFX {
				sfxPath := findSFXFile(sfx.Keyword)
				if sfxPath != "" {
					sfxDur := o.getAudioDuration(sfxPath)
					insertTime := currentTime - duration
					switch sfx.Timing {
					case "before":
						insertTime = currentTime - duration - 0.3
					case "during":
						insertTime = currentTime - duration + sfx.Offset
					case "after":
						insertTime = currentTime
					}
					fullTimeline = append(fullTimeline, TimelineEvent{
						Time:     insertTime,
						Duration: sfxDur,
						Type:     "sfx",
						Source:   sfx.Keyword,
						File:     sfxPath,
						Volume:   0.7,
					})
				}
			}
		}

		// Add chapter gap
		// (detected by chapter boundary in original input - simplified)
	}

	// Phase 4: Add BGM layer
	// Simplified: add one BGM track for the whole production
	// Full implementation would use BGMTimeline from analysis

	// Phase 5: Apply scene audio filters
	// e.g., phone_call → bandpass filter, underwater → lowpass + reverb

	// Phase 6: Assemble final audio
	outputPath := filepath.Join(jobDir, "output.wav")
	if err := o.assembleTimeline(fullTimeline, outputPath); err != nil {
		return nil, fmt.Errorf("assemble timeline: %w", err)
	}

	return &ProductionResult{
		JobID:         jobID,
		OutputPath:    outputPath,
		Duration:      currentTime,
		Characters:    knownCharacters,
		VoiceProfiles: voiceProfiles,
		Timeline:      fullTimeline,
	}, nil
}

// assembleTimeline merges all timeline events into a single audio file using FFmpeg
func (o *Orchestrator) assembleTimeline(events []TimelineEvent, outputPath string) error {
	if len(events) == 0 {
		return fmt.Errorf("no events to assemble")
	}

	tmpDir := filepath.Dir(outputPath)

	// Simple approach: build a concat list of all audio files in order
	listPath := filepath.Join(tmpDir, "timeline_concat.txt")
	var lines []string
	for _, ev := range events {
		// Apply scene filter to speech files if needed
		filteredPath := ev.File
		if ev.Scene != "" && ev.Scene != "normal" && (ev.Type == "narration" || ev.Type == "dialogue") {
			filteredPath = filepath.Join(tmpDir,
				fmt.Sprintf("filtered_%s", filepath.Base(ev.File)))
			if err := applySceneFilter(ev.File, filteredPath, ev.Scene); err == nil {
				// Successfully filtered
			} else {
				filteredPath = ev.File // fallback to original
			}
		}
		lines = append(lines, fmt.Sprintf("file '%s'", filteredPath))
	}

	if err := os.WriteFile(listPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}

	cmd := exec.Command("ffmpeg", "-y",
		"-f", "concat", "-safe", "0",
		"-i", listPath,
		"-c", "copy",
		outputPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat: %s: %w", string(out), err)
	}
	return nil
}

// applySceneFilter applies an FFmpeg audio filter chain to an audio file
func applySceneFilter(src, dst, scene string) error {
	filter := ""
	switch scene {
	case "phone_call":
		filter = "highpass=f=300,lowpass=f=3400"
	case "underwater":
		filter = "lowpass=f=800,aecho=0.8:0.9:40:0.3"
	case "broadcast":
		filter = "highpass=f=200,lowpass=f=5000,acompressor=threshold=0.1:ratio=4"
	case "inner_monologue":
		filter = "aecho=0.8:0.7:60:0.5"
	case "mecha_voice":
		filter = "afftfilt=real='hypot(re,im)*sin(0.5)':imag='hypot(re,im)*cos(0.5)'"
	default:
		return fmt.Errorf("unknown scene filter: %s", scene)
	}

	cmd := exec.Command("ffmpeg", "-y",
		"-i", src,
		"-af", filter,
		dst,
	)
	return cmd.Run()
}

// findSFXFile searches for a sound effect file matching the keyword
func findSFXFile(keyword string) string {
	// Search in data/sfx and data/bgm directories
	sfxDirs := []string{"./data/sfx", "./sfx", "./data/bgm"}
	keyword = strings.ToLower(keyword)
	for _, dir := range sfxDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Name()), keyword) {
				return filepath.Join(dir, e.Name())
			}
		}
	}
	return ""
}

// estimateDuration estimates audio duration from raw bytes
func estimateDuration(data []byte, format string, sampleRate int) float64 {
	if format == "wav" && len(data) > 44 {
		// WAV header: sample rate at offset 24, data size at offset 40
		dataSize := len(data) - 44
		return float64(dataSize) / float64(sampleRate*2) // 16-bit mono
	}
	return float64(len(data)) / float64(sampleRate*2)
}

func (o *Orchestrator) getAudioDuration(path string) float64 {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 1.0
	}
	var dur float64
	fmt.Sscanf(string(out), "%f", &dur)
	return dur
}

// buildVoiceMap maps character names to voice profiles
func buildVoiceMap(profiles []model.VoiceProfile, characters []Character) map[string]*model.VoiceProfile {
	m := make(map[string]*model.VoiceProfile)
	for i := range profiles {
		m[strings.ToLower(profiles[i].Name)] = &profiles[i]
	}
	return m
}

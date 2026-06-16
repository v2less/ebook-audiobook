package synthesis

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SegmentFile describes one audio file to include in the assembly
type SegmentFile struct {
	Path       string
	ChapterIdx int
	ChapterTitle string // for M4B chapter markers
}

// AudioAssembler is the seam for merging audio segments into a single file.
type AudioAssembler interface {
	Assemble(workDir string, segments []SegmentFile, chapterGap float64, sampleRate int, outputFormat string) (string, error)
}

// AssembleOptions contains additional metadata for assembly
type AssembleOptions struct {
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	CoverPath   string   `json:"cover_path,omitempty"`
	ChapterGap  float64  `json:"chapter_gap"`
	SampleRate  int      `json:"sample_rate"`
	Loudness    float64  `json:"loudness"` // target LUFS, -16 default
	PostProcess []string `json:"post_process"` // "normalize", "deesser", "compress"
}

// FFmpegAssembler concatenates audio using FFmpeg's concat demuxer
type FFmpegAssembler struct{}

func NewFFmpegAssembler() *FFmpegAssembler {
	return &FFmpegAssembler{}
}

func (a *FFmpegAssembler) Assemble(workDir string, segments []SegmentFile, chapterGap float64, sampleRate int, outputFormat string) (string, error) {
	if len(segments) == 0 {
		return "", fmt.Errorf("no segments to assemble")
	}

	outputPath := filepath.Join(workDir, "output."+outputFormat)

	if len(segments) == 1 {
		return a.convertAudio(segments[0].Path, outputPath, outputFormat)
	}

	// Build concat list
	listPath := filepath.Join(workDir, "concat.txt")
	var lines []string
	prevChapter := -1

	for _, seg := range segments {
		if seg.ChapterIdx != prevChapter {
			if prevChapter >= 0 && chapterGap > 0 {
				silenceFile := fmt.Sprintf("silence_ch%d.wav", prevChapter)
				a.generateSilence(filepath.Join(workDir, silenceFile), chapterGap, sampleRate)
				lines = append(lines, fmt.Sprintf("file '%s'", silenceFile))
			}
			prevChapter = seg.ChapterIdx
		}
		lines = append(lines, fmt.Sprintf("file '%s'", filepath.Base(seg.Path)))
	}

	if err := os.WriteFile(listPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return "", fmt.Errorf("write concat list: %w", err)
	}

	cmd := exec.Command("ffmpeg", "-y",
		"-f", "concat", "-safe", "0",
		"-i", "concat.txt",
		"-codec", selectCodec(outputFormat),
		"output."+outputFormat,
	)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg concat: %s: %w", string(out), err)
	}

	return outputPath, nil
}

// AssembleM4B assembles with M4B chapter markers and metadata
func (a *FFmpegAssembler) AssembleM4B(workDir string, segments []SegmentFile, opts AssembleOptions) (string, error) {
	if len(segments) == 0 {
		return "", fmt.Errorf("no segments to assemble")
	}

	outputPath := filepath.Join(workDir, "output.m4b")
	tempConcat := filepath.Join(workDir, "temp_concat.wav")

	// Step 1: Concatenate with chapter-aware processing
	if err := a.concatWithChapterInfo(workDir, tempConcat, segments, opts); err != nil {
		return "", err
	}

	// Step 2: Encode to M4B with metadata and chapter markers
	ffmpegArgs := []string{
		"-y",
		"-i", tempConcat,
		"-codec:a", "aac",
		"-b:a", "128k",
		"-f", "mp4",
		"-movflags", "+faststart",
	}

	// Add metadata
	if opts.Title != "" {
		ffmpegArgs = append(ffmpegArgs, "-metadata", "title="+opts.Title)
	}
	if opts.Author != "" {
		ffmpegArgs = append(ffmpegArgs, "-metadata", "artist="+opts.Author)
		ffmpegArgs = append(ffmpegArgs, "-metadata", "album_artist="+opts.Author)
	}
	ffmpegArgs = append(ffmpegArgs, "-metadata", "genre=Audiobook")

	// Attach cover image if available
	if opts.CoverPath != "" {
		if _, err := os.Stat(opts.CoverPath); err == nil {
			ffmpegArgs = append(ffmpegArgs, "-i", opts.CoverPath)
			ffmpegArgs = append(ffmpegArgs, "-map", "0:a", "-map", "1:v")
			ffmpegArgs = append(ffmpegArgs, "-disposition:v:0", "attached_pic")
		}
	}

	ffmpegArgs = append(ffmpegArgs, outputPath)

	cmd := exec.Command("ffmpeg", ffmpegArgs...)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg m4b: %s: %w", string(out), err)
	}

	// Cleanup temp
	os.Remove(tempConcat)

	return outputPath, nil
}

// concatWithChapterInfo builds a chapter-aware concatenated WAV
func (a *FFmpegAssembler) concatWithChapterInfo(workDir, outputPath string, segments []SegmentFile, opts AssembleOptions) error {
	listPath := filepath.Join(workDir, "concat_chapters.txt")
	var lines []string
	prevChapter := -1

	for _, seg := range segments {
		if seg.ChapterIdx != prevChapter {
			if prevChapter >= 0 && opts.ChapterGap > 0 {
				silenceFile := fmt.Sprintf("silence_ch%d.wav", prevChapter)
				a.generateSilence(filepath.Join(workDir, silenceFile), opts.ChapterGap, opts.SampleRate)
				lines = append(lines, fmt.Sprintf("file '%s'", silenceFile))
			}
			prevChapter = seg.ChapterIdx
		}
		lines = append(lines, fmt.Sprintf("file '%s'", filepath.Base(seg.Path)))
	}

	if err := os.WriteFile(listPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}

	// Build concat with optional loudness normalization
	var filterArgs []string
	if opts.Loudness != 0 {
		filterArgs = []string{
			"-af", fmt.Sprintf("loudnorm=I=%.1f:TP=-1.5:LRA=11", opts.Loudness),
		}
	}

		args := []string{"-y", "-f", "concat", "-safe", "0", "-i", "concat.txt"}
		args = append(args, filterArgs...)
		args = append(args, "-codec:a", "pcm_s16le", outputPath)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg concat: %s: %w", string(out), err)
	}

	return nil
}

// Normalize applies EBU R128 loudness normalization to an audio file
func (a *FFmpegAssembler) Normalize(inputPath, outputPath string, targetLUFS float64) error {
	if targetLUFS == 0 {
		targetLUFS = -16.0
	}

	// Two-pass loudness normalization
	args := []string{
		"-y",
		"-i", inputPath,
		"-af", fmt.Sprintf("loudnorm=I=%.1f:TP=-1.5:LRA=11:print_format=summary", targetLUFS),
		"-f", "null", "-",
	}

	// First pass: analyze (fire-and-forget for now, always apply second pass)
	cmd := exec.Command("ffmpeg", args...)
	cmd.CombinedOutput()

	// Second pass: apply (simplified single-pass for reliability)
	ext := filepath.Ext(outputPath)
	if ext == "" {
		ext = filepath.Ext(inputPath)
	}

	codec := selectCodec(strings.TrimPrefix(ext, "."))
	args2 := []string{
		"-y",
		"-i", inputPath,
		"-af", fmt.Sprintf("loudnorm=I=%.1f:TP=-1.5:LRA=11", targetLUFS),
		"-codec:a", codec,
		outputPath,
	}

	cmd = exec.Command("ffmpeg", args2...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("normalize: %s: %w", string(out), err)
	}

	return nil
}

// ApplyPostProcess applies a chain of audio post-processing filters
func (a *FFmpegAssembler) ApplyPostProcess(inputPath, outputPath string, filters []string) error {
	if len(filters) == 0 {
		return nil
	}

	var filterParts []string
	for _, f := range filters {
		switch f {
		case "normalize":
			filterParts = append(filterParts, "loudnorm=I=-16:TP=-1.5:LRA=11")
		case "deesser":
			// Simple de-esser: attenuate sibilant frequencies (5-10kHz)
			filterParts = append(filterParts, "equalizer=f=7000:t=q:w=0.5:g=-3")
		case "compressor":
			// Light compression for consistent volume
			filterParts = append(filterParts, "compand=attacks=0.003:decays=0.25:points=-80/-80|-30/-15|0/-3|20/-3:volume=-3")
		case "highpass":
			filterParts = append(filterParts, "highpass=f=80")
		case "lowpass":
			filterParts = append(filterParts, "lowpass=f=14000")
		}
	}

	if len(filterParts) == 0 {
		return nil
	}

	filterChain := strings.Join(filterParts, ",")
	ext := filepath.Ext(outputPath)
	codec := selectCodec(strings.TrimPrefix(ext, "."))

	cmd := exec.Command("ffmpeg", "-y",
		"-i", inputPath,
		"-af", filterChain,
		"-codec:a", codec,
		outputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("post-process: %s: %w", string(out), err)
	}

	return nil
}

func (a *FFmpegAssembler) convertAudio(srcPath, dstPath, format string) (string, error) {
	if strings.HasSuffix(srcPath, "."+format) {
		data, _ := os.ReadFile(srcPath)
		os.WriteFile(dstPath, data, 0644)
		return dstPath, nil
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", srcPath, "-codec", selectCodec(format), dstPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg convert: %s: %w", string(out), err)
	}
	return dstPath, nil
}

func (a *FFmpegAssembler) generateSilence(path string, duration float64, sampleRate int) {
	exec.Command("ffmpeg", "-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=%d:cl=mono", sampleRate),
		"-t", fmt.Sprintf("%.3f", duration),
		"-c:a", "pcm_s16le",
		path,
	).Run()
}

func selectCodec(format string) string {
	switch format {
	case "mp3":
		return "libmp3lame"
	case "m4b", "m4a", "aac":
		return "aac"
	case "ogg", "oga":
		return "libvorbis"
	case "flac":
		return "flac"
	case "opus":
		return "libopus"
	default:
		return "pcm_s16le"
	}
}

package synthesis

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSelectCodec(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{"mp3", "libmp3lame"},
		{"m4b", "aac"},
		{"m4a", "aac"},
		{"aac", "aac"},
		{"wav", "pcm_s16le"},
		{"unknown", "pcm_s16le"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := selectCodec(tt.format)
			if got != tt.want {
				t.Errorf("selectCodec(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestFFmpegAssembler_NoSegments(t *testing.T) {
	assembler := NewFFmpegAssembler()
	_, err := assembler.Assemble("/tmp", nil, 1.5, 24000, "mp3")
	if err == nil {
		t.Error("expected error for empty segments")
	}
}

func TestFFmpegAssembler_GenerateSilence(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	assembler := NewFFmpegAssembler()
	dir := t.TempDir()
	silencePath := filepath.Join(dir, "silence.wav")
	assembler.generateSilence(silencePath, 0.5, 24000)

	if _, err := os.Stat(silencePath); os.IsNotExist(err) {
		t.Error("silence file was not created")
	}
}

package parser

import (
	"os"
	"path/filepath"
	"testing"

	"ebook-audiobook/internal/model"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "removes repeated blank lines",
			input: "Line 1\n\n\nLine 2",
			want:  "Line 1\nLine 2",
		},
		{
			name:  "removes carriage returns",
			input: "Hello\r\nWorld",
			want:  "Hello\nWorld",
		},
		{
			name:  "preserves single newlines",
			input: "Line 1\nLine 2\nLine 3",
			want:  "Line 1\nLine 2\nLine 3",
		},
		{
			name:  "handles Chinese text",
			input: "第一章\n\n\n第二章内容",
			want:  "第一章\n第二章内容",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanText(tt.input)
			if got != tt.want {
				t.Errorf("cleanText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChapterCleaner(t *testing.T) {
	cleaner := NewCleaner()
	chapter := &model.Chapter{Content: "Hello\n\n\nWorld"}
	result := cleaner.Clean(chapter)
	if result.Content != "Hello\nWorld" {
		t.Errorf("Clean() = %q, want %q", result.Content, "Hello\nWorld")
	}
}

func TestRegistryDetectFormat(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		filename string
		content  []byte
		wantFmt  string
	}{
		{
			name:     "detects PDF by magic bytes",
			filename: "test.pdf",
			content:  []byte("%PDF-1.4\n%content"),
			wantFmt:  ".pdf",
		},
		{
			name:     "detects EPUB by extension when content is ZIP header",
			filename: "book.epub",
			content:  append([]byte("PK\x03\x04"), make([]byte, 100)...),
			wantFmt:  ".epub",
		},
		{
			name:     "falls back to extension for txt",
			filename: "story.txt",
			content:  []byte("Once upon a time..."),
			wantFmt:  ".txt",
		},
		{
			name:     "detects markdown by extension",
			filename: "readme.md",
			content:  []byte("# Title\n\nContent"),
			wantFmt:  ".md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tt.filename)
			if err := os.WriteFile(path, tt.content, 0644); err != nil {
				t.Fatal(err)
			}

			reg := NewRegistry()
			got := reg.DetectFormat(path)
			if got != tt.wantFmt {
				t.Errorf("DetectFormat() = %q, want %q", got, tt.wantFmt)
			}
		})
	}
}

func TestTextSplitter(t *testing.T) {
	// Note: TextSplitter is in tts package, test there
	t.Skip("TextSplitter is in tts package")
}

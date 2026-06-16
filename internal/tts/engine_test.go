package tts

import (
	"context"
	"testing"

	"ebook-audiobook/internal/model"
)

// mockEngine is a simple TTS engine for testing
type mockEngine struct {
	name            string
	supportsClone   bool
	supportsDesign  bool
	voices          []model.VoiceProfile
	synthesizeErr   error
	synthesizeAudio []byte
}

func (m *mockEngine) Name() string                           { return m.name }
func (m *mockEngine) SupportsVoiceClone() bool               { return m.supportsClone }
func (m *mockEngine) SupportsVoiceDesign() bool              { return m.supportsDesign }
func (m *mockEngine) ListVoices() ([]model.VoiceProfile, error) { return m.voices, nil }

func (m *mockEngine) Synthesize(ctx context.Context, text string, opts model.TTSOptions) ([]byte, string, error) {
	if m.synthesizeErr != nil {
		return nil, "", m.synthesizeErr
	}
	return m.synthesizeAudio, "wav", nil
}

func (m *mockEngine) SynthesizeStream(ctx context.Context, text string, opts model.TTSOptions, callback func(chunk []byte) error) error {
	audio, _, err := m.Synthesize(ctx, text, opts)
	if err != nil {
		return err
	}
	return callback(audio)
}

func TestEngineRegistry(t *testing.T) {
	reg := NewEngineRegistry()

	mock := &mockEngine{
		name:           "test-engine",
		supportsClone:  true,
		supportsDesign: false,
		voices: []model.VoiceProfile{
			{ID: "v1", Name: "Voice 1", VoiceID: "v1"},
		},
	}

	reg.Register("test-engine", mock)

	// Test Get
	eng, err := reg.Get("test-engine")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if eng.Name() != "test-engine" {
		t.Errorf("engine name = %q, want %q", eng.Name(), "test-engine")
	}

	// Test missing engine
	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent engine")
	}

	// Test voices
	voices, err := eng.ListVoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) != 1 || voices[0].ID != "v1" {
		t.Errorf("voices = %v, want 1 voice with ID v1", voices)
	}
}

func TestTextSplitter(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxChars int
		wantMin  int // minimum expected segments
	}{
		{
			name:     "short text is not split",
			text:     "Hello world.",
			maxChars: 1500,
			wantMin:  1,
		},
		{
			name:     "splits at sentence boundary",
			text:     "Short sentence. Another sentence. Third one here.",
			maxChars: 20,
			wantMin:  2,
		},
		{
			name:     "splits Chinese at period",
			text:     "第一句话。第二句话。第三句话。第四句话。",
			maxChars: 8,
			wantMin:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			splitter := NewTextSplitter(tt.maxChars)
			got := splitter.Split(tt.text)
			if len(got) < tt.wantMin {
				t.Errorf("Split() produced %d segments, want at least %d: %v", len(got), tt.wantMin, got)
			}

			// Verify all segments combined equal original
			combined := ""
			for _, s := range got {
				combined += s
			}
			if combined != tt.text {
				t.Errorf("combined segments = %q, want %q", combined, tt.text)
			}
		})
	}
}

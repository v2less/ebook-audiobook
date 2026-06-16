package tts

import (
	"context"
	"fmt"
	"log"

	"ebook-audiobook/internal/model"
)

// FallbackEngine wraps multiple TTS engines with automatic fallback on failure.
type FallbackEngine struct {
	engines []Engine
}

// NewFallbackEngine creates a fallback chain of TTS engines.
// Engines are tried in order; the first successful result is returned.
func NewFallbackEngine(engines ...Engine) *FallbackEngine {
	return &FallbackEngine{engines: engines}
}

func (f *FallbackEngine) Name() string {
	if len(f.engines) > 0 {
		return fmt.Sprintf("Fallback[%s]", f.engines[0].Name())
	}
	return "Fallback[empty]"
}

func (f *FallbackEngine) SupportsVoiceClone() bool {
	for _, e := range f.engines {
		if e.SupportsVoiceClone() {
			return true
		}
	}
	return false
}

func (f *FallbackEngine) SupportsVoiceDesign() bool {
	for _, e := range f.engines {
		if e.SupportsVoiceDesign() {
			return true
		}
	}
	return false
}

func (f *FallbackEngine) ListVoices() ([]model.VoiceProfile, error) {
	var all []model.VoiceProfile
	for _, e := range f.engines {
		voices, err := e.ListVoices()
		if err != nil {
			continue
		}
		all = append(all, voices...)
	}
	return all, nil
}

// Synthesize tries each engine in sequence, returning the first successful result
func (f *FallbackEngine) Synthesize(ctx context.Context, text string, opts model.TTSOptions) ([]byte, string, error) {
	if len(f.engines) == 0 {
		return nil, "", fmt.Errorf("no engines in fallback chain")
	}

	var lastErr error
	for i, engine := range f.engines {
		if i > 0 {
			log.Printf("⚠️  TTS fallback: switching from %s to %s (error: %v)", f.engines[i-1].Name(), engine.Name(), lastErr)
		}
		audio, format, err := engine.Synthesize(ctx, text, opts)
		if err == nil {
			return audio, format, nil
		}
		lastErr = err
	}

	return nil, "", fmt.Errorf("all TTS engines failed: %w", lastErr)
}

// SynthesizeStream tries streaming each engine in sequence
func (f *FallbackEngine) SynthesizeStream(ctx context.Context, text string, opts model.TTSOptions, callback func(chunk []byte) error) error {
	if len(f.engines) == 0 {
		return fmt.Errorf("no engines in fallback chain")
	}

	var lastErr error
	for i, engine := range f.engines {
		if i > 0 {
			log.Printf("⚠️  TTS stream fallback: switching from %s to %s", f.engines[i-1].Name(), engine.Name())
		}
		err := engine.SynthesizeStream(ctx, text, opts, callback)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("all TTS engines failed streaming: %w", lastErr)
}

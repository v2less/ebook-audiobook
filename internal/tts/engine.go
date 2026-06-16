package tts

import (
	"context"

	"ebook-audiobook/internal/model"
)

// Engine TTS 合成引擎接口
type Engine interface {
	// Name returns engine name for display
	Name() string

	// Synthesize 合成一段文本为音频文件
	Synthesize(ctx context.Context, text string, opts model.TTSOptions) (audioData []byte, format string, err error)

	// SynthesizeStream 流式合成，通过回调返回增量音频
	SynthesizeStream(ctx context.Context, text string, opts model.TTSOptions, callback func(chunk []byte) error) error

	// ListVoices 列出可用音色
	ListVoices() ([]model.VoiceProfile, error)

	// SupportsVoiceClone 是否支持音色复刻
	SupportsVoiceClone() bool

	// SupportsVoiceDesign 是否支持文本设计音色
	SupportsVoiceDesign() bool
}

// TextSplitter 智能文本分句
type TextSplitter struct {
	MaxChars int // max chars per segment, default 1500
}

func NewTextSplitter(maxChars int) *TextSplitter {
	if maxChars <= 0 {
		maxChars = 1500
	}
	return &TextSplitter{MaxChars: maxChars}
}

// Split 将文本按句子边界智能分段
func (s *TextSplitter) Split(text string) []string {
	if len(text) <= s.MaxChars {
		return []string{text}
	}

	var segments []string
	var current []rune
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		current = append(current, runes[i])

		if len(current) >= s.MaxChars {
			// Find the best split point: end of sentence
			cutPoint := s.findSentenceEnd(current)
			if cutPoint > 0 {
				segments = append(segments, string(current[:cutPoint]))
				current = current[cutPoint:]
			} else {
				// Force split at max chars
				segments = append(segments, string(current))
				current = nil
			}
		}
	}

	if len(current) > 0 {
		segments = append(segments, string(current))
	}

	return segments
}

// findSentenceEnd finds the best sentence boundary near the end
func (s *TextSplitter) findSentenceEnd(runes []rune) int {
	// Look in the last 20% for sentence terminators
	startSearch := len(runes) * 4 / 5
	if startSearch < 1 {
		startSearch = 1
	}

	sentenceEnds := []rune{'.', '。', '!', '！', '?', '？', '\n', '…'}
	for i := len(runes) - 1; i >= startSearch; i-- {
		for _, end := range sentenceEnds {
			if runes[i] == end {
				if i+1 < len(runes) {
					return i + 1 // include the terminator
				}
				return i
			}
		}
	}
	return 0
}

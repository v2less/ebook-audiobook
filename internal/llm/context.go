package llm

import (
	"strings"
	"unicode/utf8"
)

// ContextWindow manages long text for LLM input, keeping within token limits.
type ContextWindow struct {
	MaxTokens int
	Overlap   int // overlapping tokens between chunks for continuity
}

// NewContextWindow creates a context window manager.
// maxTokens: maximum tokens per chunk (default 4000)
// overlap: tokens overlapping between chunks (default 200)
func NewContextWindow(maxTokens, overlap int) *ContextWindow {
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	if overlap < 0 {
		overlap = 200
	}
	return &ContextWindow{MaxTokens: maxTokens, Overlap: overlap}
}

// ChunkText splits text into chunks that fit within the token limit.
// Uses a rough heuristic: ~1.5 characters per token for Chinese, ~4 for English.
func (cw *ContextWindow) ChunkText(text string) []string {
	maxChars := cw.MaxTokens * 3 // rough estimate: ~0.7-4 chars per token, use 3 as average
	if maxChars > len(text) {
		return []string{text}
	}

	var chunks []string
	runes := []rune(text)
	overlapRunes := cw.Overlap * 3

	for start := 0; start < len(runes); start += maxChars - overlapRunes {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}

		// Try to find a natural break point (sentence end) near the chunk boundary
		if end < len(runes) {
			searchStart := end - min(overlapRunes, end-start)
			for i := end; i >= searchStart; i-- {
				if i < len(runes) && isSentenceEnd(runes[i]) {
					end = i + 1
					break
				}
			}
		}

		chunk := string(runes[start:end])
		chunk = strings.TrimSpace(chunk)
		if len(chunk) > 0 {
			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

// ChunkTextWithSummary creates chunks and generates summaries for previous chunks
// to maintain context across chunk boundaries.
func (cw *ContextWindow) ChunkTextWithSummary(text string) []ChunkWithSummary {
	chunks := cw.ChunkText(text)
	result := make([]ChunkWithSummary, len(chunks))

	for i, chunk := range chunks {
		result[i] = ChunkWithSummary{
			Index:   i,
			Content: chunk,
		}
	}
	return result
}

// ChunkWithSummary holds a text chunk with its context summary
type ChunkWithSummary struct {
	Index          int    `json:"index"`
	Content        string `json:"content"`
	PreviousSummary string `json:"previous_summary,omitempty"`
}

// isSentenceEnd checks if a rune is a sentence terminator
func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '。', '!', '！', '?', '？', '\n', '…', '”', '』':
		return true
	}
	return false
}

// EstimateTokens provides a rough token count estimate
// Chinese: ~1.5 chars/token | English: ~4 chars/token | Mixed: ~3 chars/token
func EstimateTokens(text string) int {
	charCount := utf8.RuneCountInString(text)
	// Use 3 as safe average for mixed Chinese/English text
	return charCount / 3
}

// SlidingWindow provides a sliding window of context for streaming text processing
type SlidingWindow struct {
	maxTokens int
	buffer    []string
}

// NewSlidingWindow creates a sliding window
func NewSlidingWindow(maxTokens int) *SlidingWindow {
	return &SlidingWindow{maxTokens: maxTokens}
}

// Add adds a chunk to the window and returns the current context
func (sw *SlidingWindow) Add(chunk string) string {
	sw.buffer = append(sw.buffer, chunk)
	totalTokens := 0
	for _, s := range sw.buffer {
		totalTokens += EstimateTokens(s)
	}

	// Trim from front if over limit
	for totalTokens > sw.maxTokens && len(sw.buffer) > 1 {
		totalTokens -= EstimateTokens(sw.buffer[0])
		sw.buffer = sw.buffer[1:]
	}

	return sw.Context()
}

// Context returns the current window content
func (sw *SlidingWindow) Context() string {
	return strings.Join(sw.buffer, "\n")
}

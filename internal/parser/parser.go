package parser

import (
	"ebook-audiobook/internal/model"
)

// Parser 电子书解析器接口
type Parser interface {
	// Parse 从文件路径解析电子书，返回统一的 Book 结构
	Parse(filePath string) (*model.Book, error)
	// SupportedFormats 返回支持的扩展名列表
	SupportedFormats() []string
}

// ChapterCleaner 章节内容清洗
type ChapterCleaner struct{}

// NewCleaner 创建清洗器
func NewCleaner() *ChapterCleaner {
	return &ChapterCleaner{}
}

// Clean 清洗章节内容
func (c *ChapterCleaner) Clean(chapter *model.Chapter) *model.Chapter {
	cleaned := *chapter
	cleaned.Content = cleanText(chapter.Content)
	return &cleaned
}

// cleanText 清洗文本：去除多余空白、统一换行
func cleanText(text string) string {
	// Remove repeated blank lines (keep at most one)
	out := make([]byte, 0, len(text))
	prevBlank := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == '\n' {
			if !prevBlank {
				out = append(out, ch)
				prevBlank = true
			}
			// Check if next line is blank too
			j := i + 1
			for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
				j++
			}
			if j < len(text) && text[j] == '\n' {
				// Skip this newline (will be handled by next iteration)
				prevBlank = true
			} else {
				prevBlank = false
			}
		} else if ch == '\r' {
			// skip
		} else {
			out = append(out, ch)
			prevBlank = false
		}
	}
	return string(out)
}

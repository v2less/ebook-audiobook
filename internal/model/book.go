package model

import (
	"time"
)

// Book 解析后的电子书
type Book struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Format    string    `json:"format"` // epub, pdf, txt, mobi, markdown
	FileName  string    `json:"file_name"`
	Chapters  []Chapter `json:"chapters"`
	Meta      BookMeta  `json:"meta"`
	CreatedAt time.Time `json:"created_at"`
}

// BookMeta 书籍元信息
type BookMeta struct {
	Language    string `json:"language"`
	Publisher   string `json:"publisher,omitempty"`
	Description string `json:"description,omitempty"`
	CoverImage  string `json:"cover_image,omitempty"` // base64 or file path
	ISBN        string `json:"isbn,omitempty"`
	PageCount   int    `json:"page_count,omitempty"`
}

// Chapter 章节
type Chapter struct {
	Index    int    `json:"index"`
	Title    string `json:"title"`
	Content  string `json:"content"`   // markdown text
	RawHTML  string `json:"raw_html,omitempty"`
	Images   []ImageRef `json:"images,omitempty"`
}

// ImageRef 图片引用
type ImageRef struct {
	ID       string `json:"id"`
	Alt      string `json:"alt"`
	Path     string `json:"path"`      // local file path
	MimeType string `json:"mime_type"`
	Data     []byte `json:"-"`         // in-memory if small
}

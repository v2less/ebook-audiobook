package parser

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/model"
)

// Registry manages all parsers and routes to the right one
type Registry struct {
	parsers map[string]Parser
}

func NewRegistry() *Registry {
	log.Printf("[registry] NewRegistry: initializing parsers")
	r := &Registry{parsers: make(map[string]Parser)}

	// Register built-in parsers
	epub := NewEPUBParser()
	for _, ext := range epub.SupportedFormats() {
		r.parsers[ext] = epub
	}

	pdf := NewPDFParser()
	for _, ext := range pdf.SupportedFormats() {
		r.parsers[ext] = pdf
	}

	txt := NewTXTParser()
	for _, ext := range txt.SupportedFormats() {
		r.parsers[ext] = txt
	}

		mobi := NewMOBIParser()
		for _, ext := range mobi.SupportedFormats() {
			r.parsers[ext] = mobi
		}

		docx := NewDOCXParser()
		for _, ext := range docx.SupportedFormats() {
			r.parsers[ext] = docx
		}

		return r
}

// Parse auto-detects format and parses the ebook
func (r *Registry) Parse(filePath string) (*model.Book, error) {
	log.Printf("[registry] Parse called: %s", filepath.Base(filePath))
	ext := strings.ToLower(filepath.Ext(filePath))
	parser, ok := r.parsers[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", ext)
	}

	book, err := parser.Parse(filePath)
	if err != nil {
		log.Printf("[registry] Parse FAILED: %v", err)
		return nil, fmt.Errorf("parse %s: %w", ext, err)
	}
	log.Printf("[registry] Parse OK: %d chapters, title=%q", len(book.Chapters), book.Title)

	// Post-process: clean all chapters
	cleaner := NewCleaner()
	for i := range book.Chapters {
		cleaned := cleaner.Clean(&book.Chapters[i])
		book.Chapters[i] = *cleaned
	}

	// Safety net: ensure all chapters have non-empty titles
	for i := range book.Chapters {
		if strings.TrimSpace(book.Chapters[i].Title) == "" {
			book.Chapters[i].Title = fmt.Sprintf("Chapter %d", i+1)
		}
	}

	return book, nil
}

// SupportedFormats returns all supported file extensions
func (r *Registry) SupportedFormats() []string {
	var formats []string
	seen := make(map[string]bool)
	for ext := range r.parsers {
		if !seen[ext] {
			formats = append(formats, ext)
			seen[ext] = true
		}
	}
	return formats
}

// DetectFormat detects the ebook format by reading file magic bytes,
// falling back to file extension.
func (r *Registry) DetectFormat(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return filepath.Ext(filePath)
	}
	defer f.Close()

	header := make([]byte, 8)
	n, _ := f.Read(header)
	if n >= 4 {
		// PDF: %PDF
		if string(header[:4]) == "%PDF" {
			return ".pdf"
		}
		// ZIP-based: EPUB, DOCX (PK\x03\x04)
		if header[0] == 0x50 && header[1] == 0x4B && header[2] == 0x03 && header[3] == 0x04 {
			ext := strings.ToLower(filepath.Ext(filePath))
			if ext == ".epub" {
				return ext
			}
			if ext == ".docx" {
				return ext
			}
		}
	}

	// Fallback to extension
	return strings.ToLower(filepath.Ext(filePath))
}

// ---- MOBI/AZW parser ----

type MOBIParser struct{}

func NewMOBIParser() *MOBIParser {
	return &MOBIParser{}
}

func (p *MOBIParser) SupportedFormats() []string {
	return []string{".mobi", ".azw", ".azw3"}
}

func (p *MOBIParser) Parse(filePath string) (*model.Book, error) {
	// Convert to EPUB via Calibre's ebook-convert
	epubPath := filePath + ".epub"
	defer os.Remove(epubPath)

	cmd := exec.Command("ebook-convert", filePath, epubPath)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ebook-convert (install calibre): %w", err)
	}

	// Reuse EPUB parser
	epubParser := NewEPUBParser()
	book, err := epubParser.Parse(epubPath)
	if err != nil {
		return nil, fmt.Errorf("parse converted epub: %w", err)
	}
	book.Format = strings.TrimPrefix(filepath.Ext(filePath), ".")
	book.FileName = filepath.Base(filePath)
	return book, nil
}

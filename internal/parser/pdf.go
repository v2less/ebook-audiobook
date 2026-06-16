package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/model"
)

// PDFParser 解析 PDF 文件（优先用 opendataloader-pdf 子进程）
type PDFParser struct{}

func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

func (p *PDFParser) SupportedFormats() []string {
	return []string{".pdf"}
}

func (p *PDFParser) Parse(filePath string) (*model.Book, error) {
	// Try opendataloader-pdf first
	book, err := p.parseViaOpenDataLoader(filePath)
	if err == nil && len(book.Chapters) > 0 {
		return book, nil
	}
	// Fallback: extract text directly
	return p.parseSimple(filePath)
}

// parseViaOpenDataLoader 通过 opendataloader-pdf 解析（使用 JSON+Markdown 双输出）
func (p *PDFParser) parseViaOpenDataLoader(filePath string) (*model.Book, error) {
	// Check if opendataloader-pdf is available
	if _, err := exec.LookPath("opendataloader-pdf"); err != nil {
		return nil, fmt.Errorf("opendataloader-pdf not available: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "odl-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	absPath, _ := filepath.Abs(filePath)
	// Use dual format: JSON for structure + Markdown for readable text
	cmd := exec.Command("opendataloader-pdf", absPath, "-o", tmpDir, "-f", "json,markdown")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("opendataloader-pdf failed: %w", err)
	}

	// Try JSON first (best structure)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, err
	}

	// Prefer JSON output for structured parsing
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			book, err := p.buildBookFromODLJSON(filepath.Join(tmpDir, entry.Name()), filePath)
			if err == nil && len(book.Chapters) > 0 {
				return book, nil
			}
		}
	}

	// Fallback to Markdown output
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return p.buildBookFromMarkdown(filepath.Join(tmpDir, entry.Name()), filePath)
		}
	}

	return nil, fmt.Errorf("no usable output from opendataloader-pdf")
}

type odlElement struct {
	Type    string  `json:"type"`
	Content string  `json:"content"`
	Page    int     `json:"page_number"`
	Level   int     `json:"heading_level,omitempty"`
	BBox    []float64 `json:"bounding_box,omitempty"`
}

type odlOutput struct {
	Elements []odlElement `json:"elements"`
	Pages    []struct {
		PageNumber int    `json:"page_number"`
		Text       string `json:"text"`
	} `json:"pages"`
}

func (p *PDFParser) buildBookFromODLJSON(jsonPath, srcPath string) (*model.Book, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var output odlOutput
	if err := json.Unmarshal(data, &output); err != nil {
		// Try as raw elements array
		var elements []odlElement
		if err2 := json.Unmarshal(data, &elements); err2 != nil {
			return nil, fmt.Errorf("parse opendataloader JSON: %w", err)
		}
		output.Elements = elements
	}

	book := &model.Book{
		Title:    strings.TrimSuffix(filepath.Base(srcPath), ".pdf"),
		Author:   "Unknown",
		Format:   "pdf",
		FileName: filepath.Base(srcPath),
	}

	// Group elements by heading to form chapters
	var chapters []model.Chapter
	var currentLines []string
	currentTitle := "Start"
	chIdx := 0

	for _, el := range output.Elements {
		if el.Type == "heading" && el.Level <= 2 {
			if len(currentLines) > 0 {
				chapters = append(chapters, model.Chapter{
					Index:   chIdx,
					Title:   currentTitle,
					Content: strings.TrimSpace(strings.Join(currentLines, "\n")),
				})
				chIdx++
				currentLines = nil
			}
			currentTitle = el.Content
			continue
		}
		currentLines = append(currentLines, el.Content)
	}
	// Flush last chapter
	if len(currentLines) > 0 {
		chapters = append(chapters, model.Chapter{
			Index:   chIdx,
			Title:   currentTitle,
			Content: strings.TrimSpace(strings.Join(currentLines, "\n")),
		})
	}

	if len(chapters) == 0 && len(output.Elements) > 0 {
		// Single chapter from all text
		var allText []string
		for _, el := range output.Elements {
			allText = append(allText, el.Content)
		}
		chapters = append(chapters, model.Chapter{
			Index:   0,
			Title:   "Full Text",
			Content: strings.TrimSpace(strings.Join(allText, "\n")),
		})
	}

	book.Chapters = chapters
	return book, nil
}

// buildBookFromMarkdown creates a Book from markdown output
func (p *PDFParser) buildBookFromMarkdown(mdPath, srcPath string) (*model.Book, error) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, err
	}

	text := string(data)
	title := strings.TrimSuffix(filepath.Base(srcPath), ".pdf")

	chapters := splitByHeadings(text, title)
	return &model.Book{
		Title:    title,
		Author:   "Unknown",
		Format:   "pdf",
		FileName: filepath.Base(srcPath),
		Chapters: chapters,
	}, nil
}

// parseSimple extracts text using pdftotext as fallback
func (p *PDFParser) parseSimple(filePath string) (*model.Book, error) {
	// Try pdftotext
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil, fmt.Errorf("no PDF parser available (install pdftotext or opendataloader-pdf)")
	}

	cmd := exec.Command("pdftotext", "-layout", filePath, "-")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pdftotext: %w", err)
	}

	text := string(out)
	title := strings.TrimSuffix(filepath.Base(filePath), ".pdf")

	// Split by form feeds (page breaks) into chapters
	pages := strings.Split(text, "\f")
	var chapters []model.Chapter
	for i, page := range pages {
		page = strings.TrimSpace(page)
		if len(page) < 20 {
			continue
		}
		// Group pages into chapters (~10 pages each)
		if i%10 == 0 {
			chapters = append(chapters, model.Chapter{
				Index:   len(chapters),
				Title:   fmt.Sprintf("Section %d", len(chapters)+1),
				Content: page,
			})
		} else if len(chapters) > 0 {
			chapters[len(chapters)-1].Content += "\n\n" + page
		}
	}

	return &model.Book{
		Title:    title,
		Author:   "Unknown",
		Format:   "pdf",
		FileName: filepath.Base(filePath),
		Chapters: chapters,
	}, nil
}

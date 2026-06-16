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

// PDFParser 解析 PDF 文件。
// 解析优先级：pdf-inspector > opendataloader-pdf > pdftotext
type PDFParser struct{}

func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

func (p *PDFParser) SupportedFormats() []string {
	return []string{".pdf"}
}

// PDFClassification result from pdf-inspector detect-pdf
type PDFClassification struct {
	PDFType           string  `json:"pdf_type"`           // "text_based", "scanned", "image_based", "mixed"
	Confidence        float64 `json:"confidence"`
	PagesNeedingOCR   []int   `json:"pages_needing_ocr,omitempty"`
}

func (p *PDFParser) Parse(filePath string) (*model.Book, error) {
	// 1. Try pdf-inspector (pdf2md) — fastest, best quality
	book, err := p.parseViaPdfInspector(filePath)
	if err == nil && len(book.Chapters) > 0 {
		return book, nil
	}

	// 2. Try opendataloader-pdf — good structure, Python-based
	book, err = p.parseViaOpenDataLoader(filePath)
	if err == nil && len(book.Chapters) > 0 {
		return book, nil
	}

	// 3. Fallback: pdftotext plain text extraction
	return p.parseSimple(filePath)
}

// ClassifyPDF runs pdf-inspector's detect to determine PDF type
func (p *PDFParser) ClassifyPDF(filePath string) (*PDFClassification, error) {
	// Try pdf-inspector detect, then detect-pdf (older name)
	bin := "pdf-inspector"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "detect-pdf"
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("pdf-inspector not available (install @firecrawl/pdf-inspector)")
		}
	}

	absPath, _ := filepath.Abs(filePath)

	var cmd *exec.Cmd
	if bin == "detect-pdf" {
		cmd = exec.Command(bin, absPath, "--json")
	} else {
		cmd = exec.Command(bin, "detect", absPath, "--json")
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}

	var result PDFClassification
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse detect output: %w", err)
	}
	return &result, nil
}

// ========================
// pdf-inspector (pdf2md)
// ========================

func (p *PDFParser) parseViaPdfInspector(filePath string) (*model.Book, error) {
	// Try pdf-inspector first, then pdf2md (older CLI name)
	bin := "pdf-inspector"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "pdf2md"
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("pdf-inspector not available (install @firecrawl/pdf-inspector)")
		}
	}

	tmpDir, err := os.MkdirTemp("", "pdf-inspector-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	absPath, _ := filepath.Abs(filePath)
	mdPath := filepath.Join(tmpDir, "output.md")

	// pdf-inspector with --json flag for structured output
	cmd := exec.Command(bin, absPath, "--json", "-o", mdPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%s failed: %s: %w", bin, string(out), err)
	}

	return p.buildBookFromMarkdown(mdPath, filePath)
}

// ========================
// opendataloader-pdf
// ========================

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
	Type    string    `json:"type"`
	Content string    `json:"content"`
	Page    int       `json:"page_number"`
	Level   int       `json:"heading_level,omitempty"`
	BBox    []float64 `json:"bounding_box,omitempty"`
}

type odlOutput struct {
	Elements []odlElement `json:"elements"`
	Kids     []odlElement `json:"kids"` // opendataloader-pdf uses "kids" in newer versions
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

	// Merge kids into elements (opendataloader-pdf uses "kids" key)
	if len(output.Kids) > 0 && len(output.Elements) == 0 {
		output.Elements = output.Kids
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

// buildBookFromMarkdown creates a Book from Markdown output.
// Handles both plain markdown and pdf-inspector JSON-wrapped output.
func (p *PDFParser) buildBookFromMarkdown(mdPath, srcPath string) (*model.Book, error) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, err
	}

	text := string(data)

	// pdf-inspector --json wraps markdown in a JSON object
	// Try to extract the "markdown" field from JSON wrapper
	if strings.HasPrefix(text, "{") {
		var wrapper struct {
			Markdown string `json:"markdown"`
			PDFType  string `json:"pdf_type"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Markdown != "" {
			text = wrapper.Markdown
		}
	}

	title := strings.TrimSuffix(filepath.Base(srcPath), ".pdf")
	chapters := p.splitPDFMarkdown(text, title)
	return &model.Book{
		Title:    title,
		Author:   "Unknown",
		Format:   "pdf",
		FileName: filepath.Base(srcPath),
		Chapters: chapters,
	}, nil
}

// splitPDFMarkdown splits PDF markdown into chapters.
// If it has <!-- Page N --> markers, groups pages into ~10-page chapters.
// Otherwise falls back to heading-based splitting.
func (p *PDFParser) splitPDFMarkdown(text, defaultTitle string) []model.Chapter {
	// Check for page break markers from pdf-inspector
	if strings.Contains(text, "<!-- Page ") {
		pages := strings.Split(text, "<!-- Page ")
		var merged []string
		for _, page := range pages {
			page = strings.TrimSpace(page)
			if page == "" {
				continue
			}
			// Remove trailing --> if present
			if idx := strings.Index(page, "-->"); idx >= 0 {
				page = page[idx+3:]
			}
			page = strings.TrimSpace(page)
			if len(page) > 20 {
				merged = append(merged, page)
			}
		}

		// Group pages into chapters (~10 pages each)
		pagesPerChapter := 10
		var chapters []model.Chapter
		for i := 0; i < len(merged); i += pagesPerChapter {
			end := i + pagesPerChapter
			if end > len(merged) {
				end = len(merged)
			}
			chapters = append(chapters, model.Chapter{
				Index:   len(chapters),
				Title:   fmt.Sprintf("Chapter %d", len(chapters)+1),
				Content: strings.TrimSpace(strings.Join(merged[i:end], "\n\n")),
			})
		}
		if len(chapters) > 0 {
			return chapters
		}
	}

	// Fallback to heading-based splitting
	chapters := splitByHeadings(text, defaultTitle)
	if len(chapters) > 0 && len(chapters[0].Content) > 20 {
		return chapters
	}

	// Last resort: single chapter
	return []model.Chapter{{
		Index:   0,
		Title:   defaultTitle,
		Content: strings.TrimSpace(text),
	}}
}

// ========================
// pdftotext fallback
// ========================

// parseSimple extracts text using pdftotext as fallback
func (p *PDFParser) parseSimple(filePath string) (*model.Book, error) {
	// Try pdftotext
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil, fmt.Errorf("no PDF parser available (install pdf-inspector, opendataloader-pdf, or pdftotext)")
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

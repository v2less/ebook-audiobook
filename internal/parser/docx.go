package parser

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"ebook-audiobook/internal/model"
)

// DOCXParser 解析 DOCX 文件（优先 pandoc，回退纯 Go 解析）
type DOCXParser struct{}

func NewDOCXParser() *DOCXParser {
	return &DOCXParser{}
}

func (p *DOCXParser) SupportedFormats() []string {
	return []string{".docx", ".doc"}
}

func (p *DOCXParser) Parse(filePath string) (*model.Book, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	// For .doc, try LibreOffice conversion first
	if ext == ".doc" {
		return p.parseDoc(filePath)
	}

	// Try pandoc first (best quality)
	book, err := p.parseViaPandoc(filePath)
	if err == nil && len(book.Chapters) > 0 {
		return book, nil
	}

	// Fallback: pure Go DOCX parsing
	return p.parseNative(filePath)
}

// parseViaPandoc 通过 pandoc 转换 DOCX 为 Markdown
func (p *DOCXParser) parseViaPandoc(filePath string) (*model.Book, error) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		return nil, fmt.Errorf("pandoc not available: %w", err)
	}

	cmd := exec.Command("pandoc", filePath, "-f", "docx", "-t", "markdown", "--wrap=none")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pandoc conversion: %w", err)
	}

	text := string(out)
	title := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	// Split by markdown headings to form chapters
	chapters := splitByHeadings(text, title)
	return &model.Book{
		Title:    title,
		Author:   "Unknown",
		Format:   "docx",
		FileName: filepath.Base(filePath),
		Chapters: chapters,
	}, nil
}

// parseNative 纯 Go DOCX 解析（ZIP 读取 + XML 解析）
func (p *DOCXParser) parseNative(filePath string) (*model.Book, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer reader.Close()

	// Find document.xml
	var docFile *zip.File
	for _, f := range reader.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return nil, fmt.Errorf("document.xml not found in docx")
	}

	rc, err := docFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	// Parse Word XML
	text := extractTextFromDocxXML(data)
	if text == "" {
		return nil, fmt.Errorf("no text extracted from docx")
	}

	title := strings.TrimSuffix(filepath.Base(filePath), ".docx")
	chapters := splitByHeadings(text, title)

	return &model.Book{
		Title:    title,
		Author:   "Unknown",
		Format:   "docx",
		FileName: filepath.Base(filePath),
		Chapters: chapters,
	}, nil
}

// parseDoc handles .doc files via LibreOffice conversion
func (p *DOCXParser) parseDoc(filePath string) (*model.Book, error) {
	// Try LibreOffice headless conversion to DOCX
	if _, err := exec.LookPath("libreoffice"); err != nil {
		return nil, fmt.Errorf("libreoffice not available for .doc conversion (install libreoffice-headless)")
	}

	tmpDir, err := os.MkdirTemp("", "doc-convert-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("libreoffice", "--headless", "--convert-to", "docx",
		"--outdir", tmpDir, filePath)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("libreoffice conversion: %w", err)
	}

	// Find converted file
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".docx") {
			return p.Parse(filepath.Join(tmpDir, entry.Name()))
		}
	}
	return nil, fmt.Errorf("no docx output from libreoffice")
}

// extractTextFromDocxXML parses Word XML to extract plain text
func extractTextFromDocxXML(data []byte) string {
	// Simple regex-based XML text extraction from <w:t> elements
	// Word stores text in <w:p> (paragraphs) containing <w:r> (runs) containing <w:t> (text)
	re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := re.FindAllSubmatch(data, -1)

	var texts []string
	for _, m := range matches {
		if len(m) > 1 {
			texts = append(texts, string(m[1]))
		}
	}

	// Also handle paragraph breaks
	// Check for <w:p> or <w:p > tags to insert newlines
	result := strings.Join(texts, "")

	// Add paragraph breaks
	paraRe := regexp.MustCompile(`</w:p>`)
	result = paraRe.ReplaceAllString(result, "\n")

	// Clean up XML entities
	result = xmlEntitiesDecode(result)

	return strings.TrimSpace(result)
}

// xmlEntitiesDecode handles common XML entities
func xmlEntitiesDecode(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
		"&#10;", "\n",
		"&#13;", "\r",
		"&#9;", "\t",
	)
	return replacer.Replace(s)
}

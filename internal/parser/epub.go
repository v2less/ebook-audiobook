package parser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ebook-audiobook/internal/model"
)

// EPUBParser 解析 EPUB 文件（优先用 epub2md 子进程，降级纯 Go 解析）
type EPUBParser struct{}

func NewEPUBParser() *EPUBParser {
	return &EPUBParser{}
}

func (p *EPUBParser) SupportedFormats() []string {
	return []string{".epub"}
}

func (p *EPUBParser) Parse(filePath string) (*model.Book, error) {
	// Try epub2md CLI first
	book, err := p.parseViaEpub2MD(filePath)
	if err == nil {
		return book, nil
	}
	// Fallback: parse directly
	return p.parseDirect(filePath)
}

// parseViaEpub2MD 通过 epub2md CLI 解析
func (p *EPUBParser) parseViaEpub2MD(filePath string) (*model.Book, error) {
	// Check if epub2md is available
	if _, err := exec.LookPath("epub2md"); err != nil {
		// Try npx
		if _, err2 := exec.LookPath("npx"); err2 != nil {
			return nil, fmt.Errorf("epub2md not found, fallback needed: %w", err)
		}
	}

	tmpDir, err := os.MkdirTemp("", "epub2md-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	absPath, _ := filepath.Abs(filePath)
	cmd := exec.Command("epub2md", absPath)
	cmd.Dir = tmpDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Try npx fallback
		cmd2 := exec.Command("npx", "epub2md", absPath)
		cmd2.Dir = tmpDir
		cmd2.Stderr = &stderr
		if err2 := cmd2.Run(); err2 != nil {
			return nil, fmt.Errorf("epub2md failed: %s: %s", err2, stderr.String())
		}
	}

	// Read generated markdown files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("read output dir: %w", err)
	}

	return p.buildBookFromMarkdownFiles(tmpDir, entries, filePath)
}

// buildBookFromMarkdownFiles builds a Book from markdown files output by epub2md
func (p *EPUBParser) buildBookFromMarkdownFiles(dir string, entries []os.DirEntry, srcPath string) (*model.Book, error) {
	book := &model.Book{
		Format:   "epub",
		FileName: filepath.Base(srcPath),
	}

	const minContentLen = 100 // 跳过封面/纯图片页等无效章节

	logf := func(format string, args ...interface{}) {
		fmt.Printf("[epub] "+format+"\n", args...)
	}

	var chapters []model.Chapter
	idx := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			logf("skip %s: read error: %v", entry.Name(), err)
			continue
		}
		content := string(data)

		// 跳过过短的文件（封面图片页、空白页等）
		if len(content) < minContentLen {
			logf("skip %s: too short (%d bytes)", entry.Name(), len(content))
			continue
		}

		// 跳过目录页（全是 [标题](./xxx.md#xxx) 链接，无正文内容）
		if isTOCPage(content) {
			logf("skip %s: TOC page", entry.Name())
			continue
		}

		// 如果文件内容包含标题标记，按标题拆分为多个子章节
		// （epub2md 经常将多个章节合并到一个 .md 文件中）
		if strings.Contains(content, "\n#") || strings.HasPrefix(content, "#") {
			subChapters := splitByHeadings(content, strings.TrimSuffix(entry.Name(), ".md"))
			logf("%s: split into %d sub-chapters", entry.Name(), len(subChapters))
			for _, ch := range subChapters {
				if len(ch.Content) < minContentLen {
					logf("  skip sub-chapter [%s]: too short (%d chars)", ch.Title, len(ch.Content))
					continue
				}
				ch.Index = idx
				chapters = append(chapters, ch)
				idx++
			}
		} else {
			chapters = append(chapters, model.Chapter{
				Index:   idx,
				Title:   strings.TrimSuffix(entry.Name(), ".md"),
				Content: content,
			})
			idx++
		}
	}

	if len(chapters) == 0 {
		return nil, fmt.Errorf("no markdown files found in epub2md output")
	}

	// Try to extract book title from first chapter
	book.Title = chapters[0].Title
	book.Chapters = chapters
	return book, nil
}

// isTOCPage 检测内容是否为目录页（全是 [标题](./xxx.md#xxx) 链接格式，无正文）
func isTOCPage(content string) bool {
	// 移除空行后检查：如果大部分行都是 markdown 链接格式 [text](url)
	lines := strings.Split(content, "\n")
	linkLines := 0
	nonEmptyLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmptyLines++
		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "](./") {
			linkLines++
		}
	}
	// 如果非空行中 80% 以上是链接行，判定为目录页
	return nonEmptyLines > 2 && float64(linkLines)/float64(nonEmptyLines) > 0.8
}

// parseDirect is a simplified direct EPUB parser using unzip + XML parsing
func (p *EPUBParser) parseDirect(filePath string) (*model.Book, error) {
	tmpDir, err := os.MkdirTemp("", "epub-direct-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Unzip EPUB
	cmd := exec.Command("unzip", "-q", "-o", filePath, "-d", tmpDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("unzip epub: %w", err)
	}

	// Parse container.xml to find OPF
	opfPath, err := findOPF(tmpDir)
	if err != nil {
		return nil, err
	}

	opfDir := filepath.Dir(opfPath)
	book, err := parseOPF(opfPath)
	if err != nil {
		return nil, err
	}
	book.Format = "epub"
	book.FileName = filepath.Base(filePath)

	// Read each spine item as a chapter
	var chapters []model.Chapter
	for i, spine := range bookSpine {
		htmlPath := filepath.Join(opfDir, spine.Href)
		data, err := os.ReadFile(htmlPath)
		if err != nil {
			continue
		}
		htmlContent := string(data)
		text := stripHTMLTags(htmlContent)

		// Determine chapter title: spine.Title > HTML heading > fallback
		title := spine.Title
		if title == "" {
			title = extractChapterTitleFromHTML(htmlContent)
		}
		
		// If extracted title is identical to the book's title (often hardcoded by Calibre in <title>),
		// or is a generic placeholder, discard it to allow for a better fallback.
		if title == book.Title || title == "未知" || strings.ToLower(title) == "unknown" {
			title = ""
		}
		
		// Intelligent fallback: use the first non-empty line of the chapter text.
		if title == "" && len(text) > 0 {
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) > 0 {
					runes := []rune(line)
					if len(runes) > 50 {
						title = string(runes[:50]) + "..."
					} else {
						title = line
					}
					break
				}
			}
		}

		if title == "" {
			title = fmt.Sprintf("Chapter %d", i+1)
		}

		chapters = append(chapters, model.Chapter{
			Index:   i,
			Title:   title,
			Content: text,
		})
	}
	book.Chapters = chapters
	return book, nil
}

type spineItem struct {
	ID   string
	Href string
	Title string
}

var bookSpine []spineItem

func findOPF(dir string) (string, error) {
	containerPath := filepath.Join(dir, "META-INF", "container.xml")
	data, err := os.ReadFile(containerPath)
	if err != nil {
		return "", fmt.Errorf("read container.xml: %w", err)
	}
	// Simple regex-free extraction
	content := string(data)
	start := strings.Index(content, `full-path="`)
	if start < 0 {
		return "", fmt.Errorf("no full-path in container.xml")
	}
	start += len(`full-path="`)
	end := strings.Index(content[start:], `"`)
	if end < 0 {
		return "", fmt.Errorf("malformed container.xml")
	}
	return filepath.Join(dir, content[start:start+end]), nil
}

func parseOPF(opfPath string) (*model.Book, error) {
	data, err := os.ReadFile(opfPath)
	if err != nil {
		return nil, err
	}
	content := string(data)

	book := &model.Book{}

	// Extract title
	if t := extractTag(content, "dc:title"); t != "" {
		book.Title = t
	}
	if a := extractTag(content, "dc:creator"); a != "" {
		book.Author = a
	}
	if lang := extractTag(content, "dc:language"); lang != "" {
		book.Meta.Language = lang
	}

	// Extract spine items
	spineStart := strings.Index(content, "<spine")
	spineEnd := strings.Index(content[spineStart:], "</spine>")
	if spineStart >= 0 && spineEnd >= 0 {
		spineContent := content[spineStart : spineStart+spineEnd+len("</spine>")]
		bookSpine = extractSpineItems(spineContent, content)
	}

	return book, nil
}

func extractTag(html, tag string) string {
	start := strings.Index(html, "<"+tag)
	if start < 0 {
		return ""
	}
	end := strings.Index(html[start:], ">")
	if end < 0 {
		return ""
	}
	closeTag := "</" + tag + ">"
	end2 := strings.Index(html[start:], closeTag)
	if end2 < 0 {
		return ""
	}
	return strings.TrimSpace(html[start+end+1 : start+end2])
}

func extractSpineItems(spineContent, fullContent string) []spineItem {
	var items []spineItem
	// Look for idref="..." in spine
	for {
		idx := strings.Index(spineContent, `idref="`)
		if idx < 0 {
			break
		}
		spineContent = spineContent[idx+len(`idref="`):]
		end := strings.Index(spineContent, `"`)
		if end < 0 {
			break
		}
		id := spineContent[:end]
		spineContent = spineContent[end:]

		// Find href in manifest
		href := findManifestHref(fullContent, id)
		items = append(items, spineItem{ID: id, Href: href})
	}
	return items
}

func findManifestHref(content, id string) string {
	search := `id="` + id + `"`
	idx := strings.Index(content, search)
	if idx < 0 {
		return id + ".html"
	}
	
	tagStart := strings.LastIndex(content[:idx], "<item")
	if tagStart < 0 {
		return id + ".html"
	}
	tagEnd := strings.Index(content[idx:], ">")
	if tagEnd < 0 {
		return id + ".html"
	}
	
	itemTag := content[tagStart : idx+tagEnd+1]
	
	hrefIdx := strings.Index(itemTag, `href="`)
	if hrefIdx < 0 {
		return id + ".html"
	}
	hrefStart := hrefIdx + len(`href="`)
	hrefEnd := strings.Index(itemTag[hrefStart:], `"`)
	if hrefEnd < 0 {
		return id + ".html"
	}
	return itemTag[hrefStart : hrefStart+hrefEnd]
}

// extractChapterTitleFromHTML extracts a title from HTML content.
// Priority: first <h1>-<h3> heading > <title> tag.
func extractChapterTitleFromHTML(html string) string {
	lower := strings.ToLower(html)

	// Try heading tags h1, h2, h3
	for _, tag := range []string{"h1", "h2", "h3"} {
		openTag := "<" + tag
		closeTag := "</" + tag + ">"
		start := strings.Index(lower, openTag)
		if start < 0 {
			continue
		}
		// Find the end of the opening tag
		gtIdx := strings.Index(lower[start:], ">")
		if gtIdx < 0 {
			continue
		}
		contentStart := start + gtIdx + 1
		end := strings.Index(lower[contentStart:], closeTag)
		if end < 0 {
			continue
		}
		titleHTML := html[contentStart : contentStart+end]
		// Strip any nested tags inside the heading
		title := strings.TrimSpace(stripHTMLTags(titleHTML))
		if title != "" {
			return title
		}
	}

	// Fallback: try <title> tag
	titleStart := strings.Index(lower, "<title")
	if titleStart >= 0 {
		gtIdx := strings.Index(lower[titleStart:], ">")
		if gtIdx >= 0 {
			contentStart := titleStart + gtIdx + 1
			end := strings.Index(lower[contentStart:], "</title>")
			if end > 0 {
				title := strings.TrimSpace(html[contentStart : contentStart+end])
				if title != "" {
					return title
				}
			}
		}
	}

	return ""
}

func stripHTMLTags(html string) string {
	// Remove <style>...</style>, <script>...</script>, and <head>...</head> blocks entirely
	for _, tag := range []string{"style", "script", "head"} {
		for {
			lower := strings.ToLower(html)
			start := strings.Index(lower, "<"+tag)
			if start < 0 {
				break
			}
			end := strings.Index(lower[start:], "</"+tag+">")
			if end < 0 {
				// No closing tag — remove from start tag to end of string
				html = html[:start]
				break
			}
			html = html[:start] + html[start+end+len("</"+tag+">"):]
		}
	}

	// Strip remaining HTML tags
	inTag := false
	var out []byte
	for i := 0; i < len(html); i++ {
		if html[i] == '<' {
			inTag = true
			continue
		}
		if html[i] == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out = append(out, html[i])
		}
	}

	// Decode common HTML entities
	result := string(out)
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")

	// Collapse whitespace
	result = strings.Join(strings.Fields(result), " ")
	return result
}


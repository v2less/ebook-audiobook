package parser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"ebook-audiobook/internal/model"
)

// TXTParser 解析 TXT 和 Markdown 文件
type TXTParser struct{}

func NewTXTParser() *TXTParser {
	return &TXTParser{}
}

func (p *TXTParser) SupportedFormats() []string {
	return []string{".txt", ".md", ".markdown"}
}

func (p *TXTParser) Parse(filePath string) (*model.Book, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	title := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	book := &model.Book{
		Title:    title,
		Author:   "Unknown",
		Format:   strings.TrimPrefix(ext, "."),
		FileName: filepath.Base(filePath),
		Meta:     model.BookMeta{},
		Chapters: splitIntoChapters(string(data), ext == ".md" || ext == ".markdown"),
	}
	return book, nil
}

// splitIntoChapters splits text by blank lines into chapters (or by markdown headings)
func splitIntoChapters(text string, isMarkdown bool) []model.Chapter {
	if isMarkdown {
		return splitByHeadings(text, "Document")
	}
	return splitByBlankLines(text)
}

func splitByHeadings(text, defaultTitle string) []model.Chapter {
	lines := strings.Split(text, "\n")
	var chapters []model.Chapter
	var currentLines []string
	currentTitle := ""
	chIdx := 0

	flushChapter := func() {
		if len(currentLines) > 0 || currentTitle != "" {
			title := currentTitle
			if title == "" {
				title = defaultTitle
			}
			chapters = append(chapters, model.Chapter{
				Index:   chIdx,
				Title:   title,
				Content: strings.TrimSpace(strings.Join(currentLines, "\n")),
			})
			chIdx++
			currentLines = nil
			currentTitle = ""
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			title := strings.TrimLeft(trimmed, "# ")
			// 跳过装饰性分隔线标题（OCR 将 PDF 分隔符误识别为标题）
			// 如 ++++++, ------, ====== 等，不创建新的章节边界
			if isDecorativeTitle(title) {
				continue
			}
			flushChapter()
			currentTitle = title
			continue
		}
		currentLines = append(currentLines, line)
	}
	flushChapter()

	if len(chapters) == 0 {
		return []model.Chapter{{
			Index:   0,
			Title:   defaultTitle,
			Content: strings.TrimSpace(text),
		}}
	}
	return chapters
}

func splitByBlankLines(text string) []model.Chapter {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	paragraphs := strings.Split(text, "\n\n")

	// Group paragraphs into chapters (~50 paragraphs each)
	var chapters []model.Chapter
	const paragraphsPerChapter = 50
	for i := 0; i < len(paragraphs); i += paragraphsPerChapter {
		end := i + paragraphsPerChapter
		if end > len(paragraphs) {
			end = len(paragraphs)
		}
		chapters = append(chapters, model.Chapter{
			Index:   len(chapters),
			Title:   fmt.Sprintf("Part %d", len(chapters)+1),
			Content: strings.TrimSpace(strings.Join(paragraphs[i:end], "\n\n")),
		})
	}
	return chapters
}

// detectEncoding tries to detect if content is GBK and convert to UTF-8
func detectEncoding(data []byte) ([]byte, error) {
	// Simple heuristic: if high bytes present, try GBK->UTF8
	for _, b := range data {
		if b > 0x80 {
			return gbkToUTF8(data)
		}
	}
	return data, nil
}

func gbkToUTF8(data []byte) ([]byte, error) {
	// Use iconv or python for GBK conversion
	cmd := exec.Command("python3", "-c", `
import sys
data = sys.stdin.buffer.read()
try:
    print(data.decode('gbk').encode('utf-8').decode('utf-8'), end='')
except:
    sys.stdout.buffer.write(data)
`)
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.Output()
	if err != nil {
		return data, nil // fallback to raw
	}
	return out, nil
}

func init() {
	_ = detectEncoding
}

// isDecorativeTitle 检测标题是否为装饰性符号（OCR 将 PDF 分隔线误识别为标题）。
// 例如: ++++++, ------, ======, + + + + + 等。
func isDecorativeTitle(title string) bool {
	compact := strings.ReplaceAll(title, " ", "")
	if len(compact) < 2 {
		return false
	}
	first := rune(compact[0])
	// 如果是字母、数字或 CJK 字符，不是装饰性符号
	if unicode.IsLetter(first) || unicode.IsDigit(first) || unicode.Is(unicode.Han, first) {
		return false
	}
	for _, r := range compact {
		if r != first {
			return false
		}
	}
	return true
}

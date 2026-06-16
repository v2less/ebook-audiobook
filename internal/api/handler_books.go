package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"ebook-audiobook/internal/deps"
	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/parser"
)

// ---- Book handlers ----

func (s *Server) listBooks(w http.ResponseWriter, r *http.Request) {
	books, err := s.store.ListBooks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if books == nil {
		books = []*model.Book{}
	}
	writeJSON(w, books)
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request) {
	book, err := s.store.GetBook(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, book)
}

func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteBook(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) uploadBook(w http.ResponseWriter, r *http.Request) {
	// Limit upload to 100MB
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("file too large: %w", err))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read file: %w", err))
		return
	}
	defer file.Close()

	// Save uploaded file
	uploadDir := s.cfg.Storage.UploadDir
	os.MkdirAll(uploadDir, 0755)
	dstPath := filepath.Join(uploadDir, header.Filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dst.Close()

	// Detect format by content (magic bytes), fallback to extension
	format := s.detectFormat(dstPath, header.Filename)

	// Parse
	book, err := s.parserReg.Parse(dstPath)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("parse failed [detected format: %s]: %w", format, err))
		return
	}

	// Save to database
	if err := s.store.SaveBook(book); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	log.Printf("📖 Book uploaded: %s (%d chapters, format: %s)", book.Title, len(book.Chapters), book.Format)
	writeJSON(w, book)
}

// detectFormat performs content-based format detection with extension fallback
func (s *Server) detectFormat(filePath, fileName string) string {
	// Try magic-byte detection first
	f, err := os.Open(filePath)
	if err != nil {
		return filepath.Ext(fileName)
	}
	defer f.Close()

	header := make([]byte, 8)
	n, _ := f.Read(header)
	if n >= 4 {
		// PDF: %PDF
		if string(header[:4]) == "%PDF" {
			return "pdf"
		}
		// ZIP-based: EPUB, DOCX (PK\x03\x04)
		if header[0] == 0x50 && header[1] == 0x4B && header[2] == 0x03 && header[3] == 0x04 {
			// Further check for EPUB (mimetype file inside ZIP)
			if isEPUB(filePath) {
				return "epub"
			}
			// Could also be DOCX - extension takes priority
			ext := filepath.Ext(fileName)
			if ext == ".docx" {
				return "docx"
			}
			return "zip"
		}
	}
	// Fallback to extension
	ext := filepath.Ext(fileName)
	switch ext {
	case ".epub":
		return "epub"
	case ".pdf":
		return "pdf"
	case ".txt", ".md", ".markdown":
		return "txt"
	case ".mobi", ".azw", ".azw3":
		return "mobi"
	case ".docx":
		return "docx"
	case ".doc":
		return "doc"
	}
	return ext
}

// isEPUB checks if a ZIP file is an EPUB by looking for the mimetype entry
func isEPUB(path string) bool {
	// Quick check: look for "mimetype" as first entry in ZIP
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// EPUBs have "mimetypeapplication/epub+zip" at offset 30 in the ZIP local file header
	// Simpler: check extension
	return filepath.Ext(path) == ".epub"
}

// classifyPDF runs pdf-inspector classification on an uploaded PDF file
func (s *Server) classifyPDF(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse form: %w", err))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read file: %w", err))
		return
	}
	defer file.Close()

	// Save temporarily
	tmpDir, err := os.MkdirTemp("", "pdf-classify-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, header.Filename)
	dst, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dst.Close()

	// Run classification
	pdfParser := parser.NewPDFParser()
	classification, err := pdfParser.ClassifyPDF(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("classify PDF: %w", err))
		return
	}

	writeJSON(w, classification)
}

// healthReady checks all dependency status for readiness probes
func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	result := deps.CheckAll(false) // don't auto-install on health check
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":       "ok",
		"dependencies": result,
	})
}

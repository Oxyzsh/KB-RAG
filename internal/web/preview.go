package web

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"kb-rag/internal/store"
)

func (s *Server) handleDocumentPreview(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	info, ok, err := s.store.GetDocumentByID(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取文档失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	}

	name := filepath.Base(info.Path)
	kind := documentKind(name)
	rawAvailable := s.isManagedPath(info.Path)
	if rawAvailable {
		if _, err := os.Stat(info.Path); err != nil {
			rawAvailable = false
		}
	}

	text := ""
	textFormat := "plain"
	if rawAvailable && (kind == "markdown" || kind == "text") {
		if raw, err := os.ReadFile(info.Path); err == nil {
			text = string(raw)
			if kind == "markdown" {
				textFormat = "markdown"
			}
		} else {
			rawAvailable = false
		}
	}
	if strings.TrimSpace(text) == "" {
		chunks, err := s.store.ListDocumentChunks(r.Context(), p.TenantID, id)
		if err != nil {
			s.logger.Error("读取文档片段失败", "error", err, "id", id)
			writeError(w, http.StatusInternalServerError, "读取文档内容失败")
			return
		}
		text = chunksText(chunks)
		textFormat = "plain"
	}

	writeJSON(w, http.StatusOK, documentPreviewDTO{
		ID:           info.ID,
		Title:        info.Title,
		Name:         name,
		Source:       s.sourceOf(info.Path),
		Kind:         kind,
		Text:         text,
		TextFormat:   textFormat,
		RawAvailable: rawAvailable,
	})
}

func (s *Server) handleDocumentRaw(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	info, ok, err := s.store.GetDocumentByID(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取文档失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	}
	if !s.isManagedPath(info.Path) {
		writeError(w, http.StatusForbidden, "仅支持查看上传目录中的原文件")
		return
	}
	file, err := os.Open(info.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "源文件不存在")
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取源文件失败")
		return
	}

	name := filepath.Base(info.Path)
	contentType := documentContentType(name)
	disposition := "attachment"
	if strings.HasPrefix(contentType, "text/") || contentType == "application/pdf" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", contentType)
	if value := mime.FormatMediaType(disposition, map[string]string{"filename": name}); value != "" {
		w.Header().Set("Content-Disposition", value)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, stat.ModTime(), file)
}

func documentKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "markdown"
	case ".txt":
		return "text"
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	default:
		return "other"
	}
}

func documentContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

func chunksText(chunks []store.ChunkText) string {
	var b strings.Builder
	for _, chunk := range chunks {
		if heading := strings.TrimSpace(chunk.Heading); heading != "" {
			b.WriteString(heading)
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(chunk.Text))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

package loader

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Block struct {
	Level int
	Text  string
}

type Document struct {
	Path   string
	Title  string
	Format string
	Blocks []Block
}

func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt", ".docx", ".pdf":
		return true
	}
	return false
}

func Load(path string) (Document, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return loadMarkdown(path)
	case ".txt":
		return loadText(path)
	case ".docx":
		return loadDOCX(path)
	case ".pdf":
		return loadPDF(path)
	default:
		return Document{}, fmt.Errorf("暂不支持的文档格式: %s", filepath.Ext(path))
	}
}

func fallbackTitle(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func firstTitle(blocks []Block, path string) string {
	for _, b := range blocks {
		if b.Level == 1 && strings.TrimSpace(b.Text) != "" {
			return strings.TrimSpace(b.Text)
		}
	}
	return fallbackTitle(path)
}

package web

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func safeUploadName(name string) string {
	base := filepath.Base(name)
	base = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		if r < 32 {
			return '_'
		}
		return r
	}, base)
	base = strings.TrimSpace(base)
	base = strings.Trim(base, ". ")
	if base == "" {
		return "upload"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "upload"
	}
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		stem = "_" + stem
	}
	runes := []rune(stem)
	if len(runes) > 100 {
		stem = string(runes[:100])
	}
	return stem + ext
}

func (s *Server) saveUpload(src io.Reader, original string) (string, error) {
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(s.uploadDir, "upload-")
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, safeUploadName(original))
	f, err := os.Create(target)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		_ = os.RemoveAll(dir)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return target, nil
}

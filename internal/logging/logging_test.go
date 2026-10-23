package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kb-rag/internal/config"
)

func TestNewWritesToStderrAndFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "logs", "app.log")
	var stderr bytes.Buffer
	logger, closer, err := New(config.LogConfig{File: logFile}, "ingest", &stderr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("文档入库", "path", "docs/a.md")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strings.Contains(stderr.String(), "文档入库") {
		t.Errorf("stderr 缺少日志: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "command=ingest") {
		t.Errorf("stderr 缺少 command 字段: %q", stderr.String())
	}
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取日志: %v", err)
	}
	if !strings.Contains(string(raw), "docs/a.md") {
		t.Errorf("文件日志缺少字段: %q", string(raw))
	}
	if !strings.Contains(string(raw), "command=ingest") {
		t.Errorf("文件日志缺少 command: %q", string(raw))
	}
}

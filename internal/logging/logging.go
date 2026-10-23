package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"kb-rag/internal/config"
)

func New(cfg config.LogConfig, command string, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil {
		return nil, nil, err
	}
	rotator := &lumberjack.Logger{
		Filename:   cfg.File,
		MaxSize:    10,
		MaxBackups: 5,
		Compress:   false,
	}
	handler := slog.NewTextHandler(io.MultiWriter(stderr, rotator), &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler).With("command", command), rotator, nil
}

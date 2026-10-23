package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kb-rag/internal/ingest/chunker"
	"kb-rag/internal/ingest/loader"
	"kb-rag/internal/store"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Store interface {
	InitMeta(model string, chunkSize int) error
	GetDocument(path string) (store.DocumentInfo, bool, error)
	SetDim(dim int) error
	ReplaceDocument(ctx context.Context, path, title, hash string, chunks []store.ChunkInput) error
	MarkFailed(ctx context.Context, path, title, hash, msg string) error
}

type Options struct {
	Model     string
	ChunkSize int
	Overlap   int
	Logger    *slog.Logger
}

type Summary struct {
	Succeeded   int
	Skipped     int
	Failed      int
	FailedPaths []string
}

type fileResult struct {
	outcome string
	chunks  int
	err     error
	fatal   error
}

const (
	outcomeOK      = "ok"
	outcomeSkipped = "skipped"
	outcomeFailed  = "failed"
)

func Run(ctx context.Context, root string, st Store, emb Embedder, opt Options) (Summary, error) {
	var summary Summary
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return summary, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return summary, fmt.Errorf("访问目录 %s: %w", root, err)
	}
	if !info.IsDir() {
		return summary, fmt.Errorf("%s 不是目录", root)
	}
	if err := st.InitMeta(opt.Model, opt.ChunkSize); err != nil {
		return summary, err
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			opt.Logger.Warn("访问路径失败", "path", displayPath(absRoot, path), "error", walkErr)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if path != absRoot && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !loader.Supported(path) {
			return nil
		}
		rel := displayPath(absRoot, path)
		start := time.Now()
		res := processFile(ctx, st, emb, opt, path)
		if res.fatal != nil {
			return res.fatal
		}
		switch res.outcome {
		case outcomeOK:
			summary.Succeeded++
			opt.Logger.Info("文档入库", "path", rel, "chunks", res.chunks, "duration_ms", time.Since(start).Milliseconds())
		case outcomeSkipped:
			summary.Skipped++
			opt.Logger.Info("文档未变化，跳过", "path", rel)
		case outcomeFailed:
			summary.Failed++
			summary.FailedPaths = append(summary.FailedPaths, rel)
			opt.Logger.Error("文档入库失败", "path", rel, "error", res.err)
		}
		return nil
	})
	if err != nil {
		return summary, err
	}
	opt.Logger.Info("入库完成", "succeeded", summary.Succeeded, "skipped", summary.Skipped, "failed", summary.Failed)
	return summary, nil
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func processFile(ctx context.Context, st Store, emb Embedder, opt Options, path string) fileResult {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileResult{outcome: outcomeFailed, err: err}
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])

	existing, exists, err := st.GetDocument(path)
	if err != nil {
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if exists && existing.Status == "ok" && existing.Hash == hash {
		return fileResult{outcome: outcomeSkipped}
	}

	doc, loadErr := loader.Load(path)
	title := ""
	if loadErr == nil {
		title = doc.Title
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if loadErr != nil {
		markFailed(ctx, st, opt, path, title, hash, loadErr.Error())
		return fileResult{outcome: outcomeFailed, err: loadErr}
	}

	chunks := chunker.Split(doc, opt.ChunkSize, opt.Overlap)
	if len(chunks) == 0 {
		err := fmt.Errorf("未切分出有效片段")
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}
	vectors, err := emb.Embed(ctx, texts)
	if err != nil {
		if ctx.Err() != nil {
			return fileResult{outcome: outcomeFailed, err: ctx.Err(), fatal: ctx.Err()}
		}
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if len(vectors) != len(chunks) {
		err := fmt.Errorf("嵌入返回 %d 个向量，期望 %d 个", len(vectors), len(chunks))
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if err := validateVectors(vectors); err != nil {
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if err := st.SetDim(len(vectors[0])); err != nil {
		return fileResult{outcome: outcomeFailed, err: err, fatal: err}
	}

	inputs := make([]store.ChunkInput, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = store.ChunkInput{Seq: chunk.Seq, Heading: chunk.Heading, Text: chunk.Text, Vector: vectors[i]}
	}
	if err := st.ReplaceDocument(ctx, path, title, hash, inputs); err != nil {
		if ctx.Err() != nil {
			return fileResult{outcome: outcomeFailed, err: ctx.Err(), fatal: ctx.Err()}
		}
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	return fileResult{outcome: outcomeOK, chunks: len(chunks)}
}

func validateVectors(vectors [][]float32) error {
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return fmt.Errorf("嵌入服务返回空向量")
	}
	dim := len(vectors[0])
	for i, vector := range vectors {
		if len(vector) != dim {
			return fmt.Errorf("第 %d 个向量维度为 %d，期望 %d", i+1, len(vector), dim)
		}
	}
	return nil
}

func markFailed(ctx context.Context, st Store, opt Options, path, title, hash, msg string) {
	if err := st.MarkFailed(ctx, path, title, hash, msg); err != nil {
		opt.Logger.Warn("记录失败状态失败", "path", path, "error", err)
	}
}

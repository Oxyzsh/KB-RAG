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
	GetDocument(ctx context.Context, tenantID int64, path string) (store.DocumentInfo, bool, error)
	SetDim(dim int) error
	ReplaceDocument(ctx context.Context, tenantID int64, path, title, hash string, chunks []store.ChunkInput) error
	MarkFailed(ctx context.Context, tenantID int64, path, title, hash, msg string) error
}

type Options struct {
	TenantID  int64
	Model     string
	ChunkSize int
	Overlap   int
	Logger    *slog.Logger
	// Progress 可选：报告当前处理阶段，用于异步任务展示。
	Progress func(stage string)
}

type Summary struct {
	Succeeded   int
	Skipped     int
	Failed      int
	FailedPaths []string
}

type FileResult struct {
	Outcome string
	Chunks  int
	Err     error
	Fatal   bool
}

const (
	OutcomeOK      = "ok"
	OutcomeSkipped = "skipped"
	OutcomeFailed  = "failed"
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
			rel := displayPath(absRoot, path)
			summary.Failed++
			summary.FailedPaths = append(summary.FailedPaths, rel)
			opt.Logger.Warn("访问路径失败", "path", rel, "error", walkErr)
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
		res := processFile(ctx, st, emb, opt, path, rel)
		if res.Fatal {
			return res.Err
		}
		switch res.Outcome {
		case OutcomeOK:
			summary.Succeeded++
			opt.Logger.Info("文档入库", "path", rel, "chunks", res.Chunks, "duration_ms", time.Since(start).Milliseconds())
		case OutcomeSkipped:
			summary.Skipped++
			opt.Logger.Info("文档未变化，跳过", "path", rel)
		case OutcomeFailed:
			summary.Failed++
			summary.FailedPaths = append(summary.FailedPaths, rel)
			opt.Logger.Error("文档入库失败", "path", rel, "error", res.Err)
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

// IngestFile 对单个文件执行与 Run 相同的解析、切分、嵌入与事务写入。
// 调用方必须先通过 Store.InitMeta 初始化模型与 chunk_size。
func IngestFile(ctx context.Context, path string, st Store, emb Embedder, opt Options) FileResult {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	return processFile(ctx, st, emb, opt, path, filepath.Base(path))
}

func processFile(ctx context.Context, st Store, emb Embedder, opt Options, path, rel string) FileResult {
	reportProgress(opt, "读取文件")
	raw, err := os.ReadFile(path)
	if err != nil {
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])

	existing, exists, err := st.GetDocument(ctx, opt.TenantID, path)
	if err != nil {
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	if exists && existing.Status == "ok" && existing.Hash == hash {
		return FileResult{Outcome: OutcomeSkipped}
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
		markFailed(ctx, st, opt, path, rel, title, hash, loadErr.Error())
		return FileResult{Outcome: OutcomeFailed, Err: loadErr}
	}
	reportProgress(opt, "切分文档")

	chunks := chunker.Split(doc, opt.ChunkSize, opt.Overlap)
	if len(chunks) == 0 {
		err := fmt.Errorf("未切分出有效片段")
		markFailed(ctx, st, opt, path, rel, title, hash, err.Error())
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}
	reportProgress(opt, "生成向量")
	vectors, err := emb.Embed(ctx, texts)
	if err != nil {
		if ctx.Err() != nil {
			return FileResult{Outcome: OutcomeFailed, Err: ctx.Err(), Fatal: true}
		}
		markFailed(ctx, st, opt, path, rel, title, hash, err.Error())
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	if len(vectors) != len(chunks) {
		err := fmt.Errorf("嵌入返回 %d 个向量，期望 %d 个", len(vectors), len(chunks))
		markFailed(ctx, st, opt, path, rel, title, hash, err.Error())
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	if err := validateVectors(vectors); err != nil {
		markFailed(ctx, st, opt, path, rel, title, hash, err.Error())
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	if err := st.SetDim(len(vectors[0])); err != nil {
		return FileResult{Outcome: OutcomeFailed, Err: err, Fatal: true}
	}

	inputs := make([]store.ChunkInput, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = store.ChunkInput{Seq: chunk.Seq, Heading: chunk.Heading, Text: chunk.Text, Vector: vectors[i]}
	}
	reportProgress(opt, "写入索引")
	if err := st.ReplaceDocument(ctx, opt.TenantID, path, title, hash, inputs); err != nil {
		if ctx.Err() != nil {
			return FileResult{Outcome: OutcomeFailed, Err: ctx.Err(), Fatal: true}
		}
		markFailed(ctx, st, opt, path, rel, title, hash, err.Error())
		return FileResult{Outcome: OutcomeFailed, Err: err}
	}
	return FileResult{Outcome: OutcomeOK, Chunks: len(chunks)}
}

func reportProgress(opt Options, stage string) {
	if opt.Progress != nil {
		opt.Progress(stage)
	}
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

func markFailed(ctx context.Context, st Store, opt Options, path, rel, title, hash, msg string) {
	if err := st.MarkFailed(ctx, opt.TenantID, path, title, hash, msg); err != nil {
		opt.Logger.Warn("记录失败状态失败", "path", rel, "error", err)
	}
}

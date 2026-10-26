package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"kb-rag/internal/config"
	"kb-rag/internal/ingest"
	"kb-rag/internal/ingest/embedder"
	"kb-rag/internal/llm"
	"kb-rag/internal/logging"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

const (
	exitOK          = 0
	exitRuntime     = 1
	exitConfig      = 2
	exitInterrupted = 130
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitConfig
	}
	switch args[0] {
	case "ingest":
		return runIngest(ctx, args[1:], stdout, stderr)
	case "ask":
		return runAsk(ctx, args[1:], stdout, stderr)
	case "stats":
		return runStats(ctx, args[1:], stdout, stderr)
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "未知命令 %q\n\n", args[0])
		usage(stderr)
		return exitConfig
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "用法：")
	fmt.Fprintln(w, "  KB-RAG ingest [--config 路径] <目录>")
	fmt.Fprintln(w, "  KB-RAG ask [--config 路径] [--top-k N] \"问题\"")
	fmt.Fprintln(w, "  KB-RAG stats [--config 路径]")
	fmt.Fprintln(w, "  KB-RAG serve [--config 路径]")
}

func newFlagSet(name string, stderr io.Writer, usageLine string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./config.yaml", "配置文件路径")
	fs.Usage = func() {
		fmt.Fprintln(stderr, usageLine)
		fs.PrintDefaults()
	}
	return fs, cfgPath
}

func flagExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitConfig
}

func interrupted(ctx context.Context) bool {
	return ctx.Err() != nil
}

func runIngest(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("ingest", stderr, "用法: KB-RAG ingest [--config 路径] <目录>")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitConfig
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	if err := cfg.Validate("ingest"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	logger, closer, err := logging.New(cfg.Log, "ingest", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "日志初始化失败: %v\n", err)
		return exitRuntime
	}
	defer closer.Close()

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		logger.Error("打开知识库失败", "error", err)
		return exitRuntime
	}
	defer st.Close()

	emb := embedder.New(cfg.Embedding.BaseURL, cfg.Embedding.Model, cfg.Embedding.BatchSize)
	summary, err := ingest.Run(ctx, fs.Arg(0), st, emb, ingest.Options{
		Model:     cfg.Embedding.Model,
		ChunkSize: cfg.Chunk.Size,
		Overlap:   cfg.Chunk.Overlap,
		Logger:    logger,
	})
	if err != nil {
		logger.Error("入库中断", "error", err)
		if interrupted(ctx) {
			return exitInterrupted
		}
		return exitRuntime
	}
	fmt.Fprintf(stdout, "入库完成：成功 %d，跳过 %d，失败 %d\n", summary.Succeeded, summary.Skipped, summary.Failed)
	if summary.Failed > 0 {
		fmt.Fprintln(stdout, "失败路径：")
		for _, path := range summary.FailedPaths {
			fmt.Fprintf(stdout, "  - %s\n", path)
		}
		return exitRuntime
	}
	return exitOK
}

func runAsk(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("ask", stderr, `用法: KB-RAG ask [--config 路径] [--top-k N] "问题"`)
	topK := fs.Int("top-k", 8, "检索片段数量")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		return exitConfig
	}
	if *topK <= 0 {
		fmt.Fprintln(stderr, "参数错误: --top-k 必须大于 0")
		return exitConfig
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	if err := cfg.Validate("ask"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	logger, closer, err := logging.New(cfg.Log, "ask", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "日志初始化失败: %v\n", err)
		return exitRuntime
	}
	defer closer.Close()

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		logger.Error("打开知识库失败", "error", err)
		return exitRuntime
	}
	defer st.Close()

	baseDir, err := os.Getwd()
	if err != nil {
		logger.Error("读取当前目录失败", "error", err)
		return exitRuntime
	}
	service := &query.Service{
		Embedder:   embedder.New(cfg.Embedding.BaseURL, cfg.Embedding.Model, cfg.Embedding.BatchSize),
		Store:      st,
		LLM:        llm.New(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.APIKey),
		EmbedModel: cfg.Embedding.Model,
		TopK:       *topK,
		BaseDir:    baseDir,
		Logger:     logger,
	}
	result, err := service.Ask(ctx, fs.Arg(0))
	if err != nil {
		if len(result.Hits) > 0 {
			fmt.Fprintln(stdout, query.FormatHits(result.Hits, baseDir))
			logger.Error("生成失败，已输出检索片段", "error", err)
		} else {
			logger.Error("问答失败", "error", err)
		}
		if interrupted(ctx) {
			return exitInterrupted
		}
		return exitRuntime
	}
	fmt.Fprintln(stdout, query.FormatResult(result, baseDir))
	return exitOK
}

func runStats(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("stats", stderr, "用法: KB-RAG stats [--config 路径]")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitConfig
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	if err := cfg.Validate("ingest"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	logger, closer, err := logging.New(cfg.Log, "stats", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "日志初始化失败: %v\n", err)
		return exitRuntime
	}
	defer closer.Close()

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		logger.Error("打开知识库失败", "error", err)
		return exitRuntime
	}
	defer st.Close()

	stats, err := st.Stats(ctx)
	if err != nil {
		logger.Error("读取统计失败", "error", err)
		if interrupted(ctx) {
			return exitInterrupted
		}
		return exitRuntime
	}
	fmt.Fprintf(stdout, "文档：%d（失败 %d）\n", stats.Documents, stats.DocumentsFailed)
	fmt.Fprintf(stdout, "片段：%d\n", stats.Chunks)
	if stats.Meta.Model == "" {
		fmt.Fprintln(stdout, "嵌入模型：未初始化")
	} else {
		fmt.Fprintf(stdout, "嵌入模型：%s（%d 维）\n", stats.Meta.Model, stats.Meta.Dim)
	}
	if stats.Meta.ChunkSize > 0 {
		fmt.Fprintf(stdout, "切分：%d 字符\n", stats.Meta.ChunkSize)
	}
	fmt.Fprintf(stdout, "库大小：%.1f MB\n", float64(stats.SizeBytes)/(1024*1024))
	if stats.DocumentsFailed > 0 {
		baseDir, _ := os.Getwd()
		fmt.Fprintln(stdout, "失败文档：")
		for _, path := range stats.FailedPaths {
			fmt.Fprintf(stdout, "  - %s\n", query.DisplayPath(path, baseDir))
		}
	}
	if stats.Chunks > 200000 {
		fmt.Fprintln(stdout, "提示：片段数超过 20 万，建议迁移到 Qdrant。")
	}
	return exitOK
}

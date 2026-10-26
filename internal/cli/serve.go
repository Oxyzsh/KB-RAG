package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"kb-rag/internal/auth"
	"kb-rag/internal/config"
	"kb-rag/internal/ingest/embedder"
	"kb-rag/internal/llm"
	"kb-rag/internal/logging"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
	"kb-rag/internal/web"
)

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("serve", stderr, "用法: KB-RAG serve [--config 路径]")
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
	if err := cfg.Validate("serve"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}

	logger, closer, err := logging.New(cfg.Log, "serve", stderr)
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
	if err := st.InitMeta(cfg.Embedding.Model, cfg.Chunk.Size); err != nil {
		logger.Error("知识库初始化失败", "error", err)
		return exitRuntime
	}
	if err := st.DeleteExpiredSessions(ctx, time.Now()); err != nil {
		logger.Warn("清理过期会话失败", "error", err)
	}

	userCount, err := st.CountUsers(ctx)
	if err != nil {
		logger.Error("读取用户数量失败", "error", err)
		return exitRuntime
	}
	if userCount == 0 {
		if len([]rune(cfg.AdminPassword)) < 12 {
			fmt.Fprintln(stderr, "配置错误: 首次启动需设置环境变量 KB_RAG_ADMIN_PASSWORD，且至少 12 个字符")
			return exitConfig
		}
		hash, err := auth.HashPassword(cfg.AdminPassword)
		if err != nil {
			fmt.Fprintf(stderr, "配置错误: %v\n", err)
			return exitConfig
		}
		admin, created, err := st.EnsureBootstrapAdmin(ctx, cfg.AdminUser, hash)
		if err != nil {
			logger.Error("创建初始管理员失败", "error", err)
			return exitRuntime
		}
		if created {
			logger.Info("已创建初始管理员", "username", admin.Username, "tenant", "default")
			fmt.Fprintf(stdout, "已创建初始管理员：%s（默认租户 default）\n", admin.Username)
		}
	}

	baseDir, err := os.Getwd()
	if err != nil {
		logger.Error("读取当前目录失败", "error", err)
		return exitRuntime
	}
	emb := embedder.New(cfg.Embedding.BaseURL, cfg.Embedding.Model, cfg.Embedding.BatchSize)
	service := &query.Service{
		Embedder:   emb,
		Store:      st,
		LLM:        llm.New(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.APIKey),
		EmbedModel: cfg.Embedding.Model,
		TopK:       8,
		BaseDir:    baseDir,
		Logger:     logger,
	}
	srv := web.New(web.Options{
		Store:          st,
		Embedder:       emb,
		Query:          service,
		Logger:         logger,
		UploadDir:      cfg.Web.UploadDir,
		MaxUploadBytes: int64(cfg.Web.MaxUploadMB) << 20,
		FrontendDir:    cfg.Web.FrontendDir,
		IngestModel:    cfg.Embedding.Model,
		ChunkSize:      cfg.Chunk.Size,
		Overlap:        cfg.Chunk.Overlap,
	})

	fmt.Fprintf(stdout, "Web 服务已启动：http://%s（Ctrl+C 停止）\n", cfg.Web.Addr)
	if err := srv.ListenAndServe(ctx, cfg.Web.Addr); err != nil {
		if interrupted(ctx) {
			return exitInterrupted
		}
		logger.Error("Web 服务退出", "error", err)
		return exitRuntime
	}
	return exitOK
}

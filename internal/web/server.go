package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kb-rag/internal/ingest"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

const defaultSessionTTL = 12 * time.Hour

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Options struct {
	Store          *store.Store
	Embedder       Embedder
	Query          *query.Service
	Logger         *slog.Logger
	UploadDir      string
	MaxUploadBytes int64
	FrontendDir    string
	IngestModel    string
	ChunkSize      int
	Overlap        int
	SessionTTL     time.Duration
}

type Server struct {
	store          *store.Store
	embedder       Embedder
	query          *query.Service
	logger         *slog.Logger
	uploadDir      string
	maxUploadBytes int64
	frontendDir    string
	ingestOptions  ingest.Options
	ingestMu       sync.Mutex
	sessionTTL     time.Duration
	limiter        *loginLimiter
	hashSem        chan struct{}
}

func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = defaultSessionTTL
	}
	return &Server{
		store:          opts.Store,
		embedder:       opts.Embedder,
		query:          opts.Query,
		logger:         opts.Logger,
		uploadDir:      opts.UploadDir,
		maxUploadBytes: opts.MaxUploadBytes,
		frontendDir:    opts.FrontendDir,
		ingestOptions: ingest.Options{
			Model:     opts.IngestModel,
			ChunkSize: opts.ChunkSize,
			Overlap:   opts.Overlap,
			Logger:    opts.Logger,
		},
		sessionTTL: opts.SessionTTL,
		limiter:    newLoginLimiter(5, 5*time.Minute),
		hashSem:    make(chan struct{}, 4),
	}
}

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /session", s.handleSession)
	api.HandleFunc("POST /logout", s.handleLogout)
	api.HandleFunc("GET /stats", s.handleStats)
	api.HandleFunc("GET /documents", s.handleListDocuments)
	api.HandleFunc("POST /documents", s.requireRoles(s.handleUploadDocument, "admin", "editor"))
	api.HandleFunc("DELETE /documents/{id}", s.requireRoles(s.handleDeleteDocument, "admin", "editor"))
	api.HandleFunc("POST /documents/{id}/reingest", s.requireRoles(s.handleReingestDocument, "admin", "editor"))
	api.HandleFunc("POST /ask", s.handleAsk)
	api.HandleFunc("GET /users", s.requireRoles(s.handleListUsers, "admin"))
	api.HandleFunc("POST /users", s.requireRoles(s.handleCreateUser, "admin"))
	api.HandleFunc("PATCH /users/{id}", s.requireRoles(s.handleUpdateUser, "admin"))
	api.HandleFunc("DELETE /users/{id}", s.requireRoles(s.handleDeleteUser, "admin"))
	api.HandleFunc("GET /tenants", s.requireRoles(s.handleListTenants, "admin"))
	api.HandleFunc("POST /tenants", s.requireSuper(s.handleCreateTenant))
	api.HandleFunc("POST /tenants/{slug}/admin/reset", s.requireSuper(s.handleResetTenantAdmin))
	api.HandleFunc("GET /audit", s.requireRoles(s.handleListAudit, "admin"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.Handle("/api/", s.withSession(http.StripPrefix("/api", api)))
	mux.Handle("/", s.staticHandler())
	return s.withSecurityHeaders(s.withLogging(mux))
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      30 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) requireRoles(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalFrom(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		if !slices.Contains(roles, p.Role) {
			writeError(w, http.StatusForbidden, "权限不足")
			return
		}
		next(w, r)
	}
}

func (s *Server) requireSuper(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalFrom(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		if !p.IsSuper {
			writeError(w, http.StatusForbidden, "仅超级管理员可操作")
			return
		}
		next(w, r)
	}
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		s.logger.Info("http 请求", "method", r.Method, "path", r.URL.Path, "status", sw.status, "bytes", sw.bytes, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServer(http.Dir(s.frontendDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	stats, err := s.store.Stats(r.Context(), p.TenantID)
	if err != nil {
		s.logger.Error("读取统计失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取统计失败")
		return
	}
	writeJSON(w, http.StatusOK, statsDTO{
		Documents:       stats.Documents,
		DocumentsFailed: stats.DocumentsFailed,
		Chunks:          stats.Chunks,
		SizeBytes:       stats.SizeBytes,
		TenantSizeBytes: stats.TenantSizeBytes,
		Embedding: embeddingDTO{
			Model:     stats.Meta.Model,
			Dim:       stats.Meta.Dim,
			ChunkSize: stats.Meta.ChunkSize,
		},
	})
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	list, err := s.store.ListDocuments(r.Context(), p.TenantID)
	if err != nil {
		s.logger.Error("读取文档列表失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取文档列表失败")
		return
	}
	out := make([]documentDTO, 0, len(list))
	for _, d := range list {
		out = append(out, s.documentDTO(d))
	}
	writeJSON(w, http.StatusOK, listDocumentsResponse{Documents: out})
}

func (s *Server) documentDTO(d store.DocumentSummary) documentDTO {
	return documentDTO{
		ID:        d.ID,
		Title:     d.Title,
		Name:      filepath.Base(d.Path),
		Status:    d.Status,
		Chunks:    d.Chunks,
		Source:    s.sourceOf(d.Path),
		Error:     d.Error,
		UpdatedAt: d.UpdatedAt,
	}
}

func (s *Server) sourceOf(path string) string {
	if s.isManagedPath(path) {
		return "upload"
	}
	return "external"
}

func (s *Server) isManagedPath(path string) bool {
	root, err := filepath.Abs(s.uploadDir)
	if err != nil {
		return false
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

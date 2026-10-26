package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kb-rag/internal/ingest"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

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
	Token          string
	IngestModel    string
	ChunkSize      int
	Overlap        int
}

type Server struct {
	store          *store.Store
	embedder       Embedder
	query          *query.Service
	logger         *slog.Logger
	uploadDir      string
	maxUploadBytes int64
	frontendDir    string
	token          string
	ingestOptions  ingest.Options
	ingestMu       sync.Mutex
}

func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{
		store:          opts.Store,
		embedder:       opts.Embedder,
		query:          opts.Query,
		logger:         opts.Logger,
		uploadDir:      opts.UploadDir,
		maxUploadBytes: opts.MaxUploadBytes,
		frontendDir:    opts.FrontendDir,
		token:          opts.Token,
		ingestOptions: ingest.Options{
			Model:     opts.IngestModel,
			ChunkSize: opts.ChunkSize,
			Overlap:   opts.Overlap,
			Logger:    opts.Logger,
		},
	}
}

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /health", s.handleHealth)
	api.HandleFunc("GET /session", s.handleSession)
	api.HandleFunc("GET /stats", s.handleStats)
	api.HandleFunc("GET /documents", s.handleListDocuments)
	api.HandleFunc("POST /documents", s.handleUploadDocument)

	mux := http.NewServeMux()
	mux.Handle("/api/", s.withAuth(http.StripPrefix("/api", api)))
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

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "
		provided := ""
		if strings.HasPrefix(header, prefix) {
			provided = strings.TrimSpace(header[len(prefix):])
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
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

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
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
		Embedding: embeddingDTO{
			Model:     stats.Meta.Model,
			Dim:       stats.Meta.Dim,
			ChunkSize: stats.Meta.ChunkSize,
		},
	})
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDocuments(r.Context())
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

func (s *Server) summaryByID(ctx context.Context, id int64) (store.DocumentSummary, bool, error) {
	list, err := s.store.ListDocuments(ctx)
	if err != nil {
		return store.DocumentSummary{}, false, err
	}
	for _, d := range list {
		if d.ID == id {
			return d, true, nil
		}
	}
	return store.DocumentSummary{}, false, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

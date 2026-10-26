package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kb-rag/internal/llm"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

type fakeEmbedder struct{}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if strings.Contains(text, "年假") {
			out[i] = []float32{1, 0, 0}
		} else {
			out[i] = []float32{0, 1, 0}
		}
	}
	return out, nil
}

type fakeLLM struct {
	fail bool
}

func (f *fakeLLM) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	if f.fail {
		return "", errors.New("llm down")
	}
	return "员工满1年享5天年假[1]。", nil
}

func newTestServer(t *testing.T) (*Server, *fakeEmbedder, *fakeLLM, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.InitMeta("test-model", 100); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDim(3); err != nil {
		t.Fatal(err)
	}
	emb := &fakeEmbedder{}
	llmFake := &fakeLLM{}
	q := &query.Service{
		Embedder:   emb,
		Store:      st,
		LLM:        llmFake,
		EmbedModel: "test-model",
		TopK:       8,
		BaseDir:    dir,
	}
	frontend := filepath.Join(dir, "frontend")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontend, "index.html"), []byte("<html><body>kb-rag</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	uploads := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatal(err)
	}
	token := "0123456789abcdef"
	srv := New(Options{
		Store:          st,
		Embedder:       emb,
		Query:          q,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		UploadDir:      uploads,
		MaxUploadBytes: 1 << 20,
		FrontendDir:    frontend,
		Token:          token,
		IngestModel:    "test-model",
		ChunkSize:      100,
		Overlap:        20,
	})
	return srv, emb, llmFake, token
}

func call(t *testing.T, h http.Handler, method, path string, body []byte, token, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthIsPublic(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	rec := call(t, srv.Handler(), "GET", "/api/health", nil, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v", body)
	}
}

func TestAPIRequiresToken(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	if rec := call(t, srv.Handler(), "GET", "/api/session", nil, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("无令牌 status = %d", rec.Code)
	}
	if rec := call(t, srv.Handler(), "GET", "/api/session", nil, "wrong-token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误令牌 status = %d", rec.Code)
	}
	if rec := call(t, srv.Handler(), "GET", "/api/session", nil, token, ""); rec.Code != http.StatusOK {
		t.Fatalf("正确令牌 status = %d", rec.Code)
	}
}

func TestStatsAndEmptyDocuments(t *testing.T) {
	srv, _, _, token := newTestServer(t)

	rec := call(t, srv.Handler(), "GET", "/api/stats", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d", rec.Code)
	}
	var stats statsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 0 || stats.Chunks != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.Embedding.Model != "test-model" || stats.Embedding.Dim != 3 || stats.Embedding.ChunkSize != 100 {
		t.Fatalf("embedding = %+v", stats.Embedding)
	}

	rec = call(t, srv.Handler(), "GET", "/api/documents", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("documents status = %d", rec.Code)
	}
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 0 {
		t.Fatalf("documents = %+v", docs.Documents)
	}
}

func TestStaticIndex(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	rec := call(t, srv.Handler(), "GET", "/", nil, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("static status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "kb-rag") {
		t.Fatalf("static body = %q", rec.Body.String())
	}
}

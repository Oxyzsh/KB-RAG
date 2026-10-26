package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kb-rag/internal/auth"
	"kb-rag/internal/llm"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

const testPassword = "test-password-123"

var testPasswordHash = sync.OnceValue(func() string {
	hash, _ := auth.HashPassword(testPassword)
	return hash
})

type fakeEmbedder struct {
	gate <-chan struct{}
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
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

func (f *fakeLLM) ChatStream(ctx context.Context, messages []llm.Message, onDelta func(string) error) error {
	if f.fail {
		return errors.New("llm down")
	}
	for _, part := range []string{"员工满1年", "享5天年假[1]。"} {
		if err := onDelta(part); err != nil {
			return err
		}
	}
	return nil
}

type testEnv struct {
	srv         *Server
	store       *store.Store
	emb         *fakeEmbedder
	llmFake     *fakeLLM
	defaultT    store.Tenant
	acmeT       store.Tenant
	users       map[string]store.User
	tokens      map[string]string
	dir         string
	uploadDir   string
	frontendDir string
}

func newTestServer(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
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
	defaultT, ok, err := st.TenantBySlug(ctx, "default")
	if err != nil || !ok {
		t.Fatalf("default tenant: ok=%v err=%v", ok, err)
	}

	users := map[string]store.User{}
	root, _, err := st.EnsureBootstrapAdmin(ctx, "root", testPasswordHash())
	if err != nil {
		t.Fatal(err)
	}
	users["root"] = root
	for _, u := range []struct {
		name string
		role string
	}{
		{"admin1", "admin"},
		{"editor1", "editor"},
		{"viewer1", "viewer"},
	} {
		user, err := st.CreateUser(ctx, defaultT.ID, u.name, testPasswordHash(), u.role)
		if err != nil {
			t.Fatal(err)
		}
		users[u.name] = user
	}
	acmeT, acmeAdmin, err := st.CreateTenantWithAdmin(ctx, "acme", "Acme", "acme-admin", testPasswordHash())
	if err != nil {
		t.Fatal(err)
	}
	users["acme-admin"] = acmeAdmin

	tokens := map[string]string{}
	for name, user := range users {
		tokens[name] = issueToken(t, st, user.ID)
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
	srv := New(Options{
		Store:          st,
		Embedder:       emb,
		Query:          q,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		UploadDir:      uploads,
		MaxUploadBytes: 1 << 20,
		FrontendDir:    frontend,
		IngestModel:    "test-model",
		ChunkSize:      100,
		Overlap:        20,
	})
	return &testEnv{
		srv: srv, store: st, emb: emb, llmFake: llmFake,
		defaultT: defaultT, acmeT: acmeT, users: users, tokens: tokens,
		dir: dir, uploadDir: uploads, frontendDir: frontend,
	}
}

func issueToken(t *testing.T, st *store.Store, userID int64) string {
	t.Helper()
	token, err := auth.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(context.Background(), auth.HashToken(token), userID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return token
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

func (e *testEnv) callAs(t *testing.T, who, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, e.srv.Handler(), method, path, body, e.tokens[who], contentType)
}

func (e *testEnv) callAsTenant(t *testing.T, who, tenant, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token := e.tokens[who]; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenant != "" {
		req.Header.Set("X-Tenant", tenant)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func login(t *testing.T, e *testEnv, tenant, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(loginRequest{Tenant: tenant, Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	return call(t, e.srv.Handler(), "POST", "/api/login", body, "", "application/json")
}

func decodeLogin(t *testing.T, rec *httptest.ResponseRecorder) loginResponse {
	t.Helper()
	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login: %v body=%s", err, rec.Body.String())
	}
	return resp
}

func multipartBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &b, mw.FormDataContentType()
}

func uploadDoc(t *testing.T, e *testEnv, who, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, "file", filename, content)
	return e.callAs(t, who, "POST", "/api/documents", body.Bytes(), ct)
}

func uploadedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func askBody(t *testing.T, question string) []byte {
	t.Helper()
	b, err := json.Marshal(askRequest{Question: question})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func userBody(t *testing.T, username, password, role string) []byte {
	t.Helper()
	b, err := json.Marshal(createUserRequest{Username: username, Password: password, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func storeAuditFilter(tenantID int64) store.AuditFilter {
	return store.AuditFilter{TenantID: &tenantID}
}

func storeAuditFilterAll() store.AuditFilter {
	return store.AuditFilter{}
}

func fmtID(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

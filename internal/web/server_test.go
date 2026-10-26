package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHealthIsPublic(t *testing.T) {
	env := newTestServer(t)
	rec := call(t, env.srv.Handler(), "GET", "/api/health", nil, "", "")
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
	env := newTestServer(t)
	if rec := call(t, env.srv.Handler(), "GET", "/api/session", nil, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("无令牌 status = %d", rec.Code)
	}
	if rec := call(t, env.srv.Handler(), "GET", "/api/session", nil, "wrong-token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误令牌 status = %d", rec.Code)
	}
	if rec := call(t, env.srv.Handler(), "GET", "/api/session", nil, env.tokens["viewer1"], ""); rec.Code != http.StatusOK {
		t.Fatalf("正确令牌 status = %d", rec.Code)
	}
}

func TestLoginAndLogout(t *testing.T) {
	env := newTestServer(t)
	rec := login(t, env, "default", "viewer1", testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeLogin(t, rec)
	if resp.Token == "" || resp.User.Role != "viewer" || resp.User.TenantSlug != "default" {
		t.Fatalf("login = %+v", resp)
	}

	rec = call(t, env.srv.Handler(), "GET", "/api/session", nil, resp.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d", rec.Code)
	}
	var session sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.User.Username != "viewer1" || session.User.IsSuper {
		t.Fatalf("session = %+v", session)
	}

	rec = call(t, env.srv.Handler(), "POST", "/api/logout", nil, resp.Token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", rec.Code)
	}
	if rec := call(t, env.srv.Handler(), "GET", "/api/session", nil, resp.Token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("登出后 status = %d", rec.Code)
	}

	items, _, err := env.store.ListAudit(t.Context(), storeAuditFilter(env.defaultT.ID))
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, item := range items {
		actions[item.Action]++
	}
	if actions["login.success"] != 1 || actions["logout"] != 1 {
		t.Fatalf("audit actions = %+v", actions)
	}
}

func TestLoginFailure(t *testing.T) {
	env := newTestServer(t)
	cases := []struct {
		tenant, username, password string
	}{
		{"default", "viewer1", "wrong-password-1"},
		{"default", "ghost", testPassword},
		{"missing", "viewer1", testPassword},
	}
	for _, c := range cases {
		if rec := login(t, env, c.tenant, c.username, c.password); rec.Code != http.StatusUnauthorized {
			t.Fatalf("login(%+v) status = %d", c, rec.Code)
		}
	}
	items, _, err := env.store.ListAudit(t.Context(), storeAuditFilter(env.defaultT.ID))
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, item := range items {
		if item.Action == "login.failure" {
			failures++
		}
	}
	if failures != 2 {
		t.Fatalf("租户内登录失败审计数 = %d，items=%+v", failures, items)
	}
	all, _, err := env.store.ListAudit(t.Context(), storeAuditFilterAll())
	if err != nil {
		t.Fatal(err)
	}
	nullTenant := 0
	for _, item := range all {
		if item.Action == "login.failure" && item.TenantID == nil {
			nullTenant++
		}
	}
	if nullTenant != 1 {
		t.Fatalf("未知租户登录失败应为 NULL tenant: %+v", all)
	}
}

func TestLoginRateLimit(t *testing.T) {
	env := newTestServer(t)
	for i := 0; i < 5; i++ {
		if rec := login(t, env, "default", "viewer1", "wrong-password-1"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次 status = %d", i+1, rec.Code)
		}
	}
	if rec := login(t, env, "default", "viewer1", "wrong-password-1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超限 status = %d", rec.Code)
	}
}

func TestStatsAndEmptyDocuments(t *testing.T) {
	env := newTestServer(t)
	rec := env.callAs(t, "viewer1", "GET", "/api/stats", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d body=%s", rec.Code, rec.Body.String())
	}
	var stats statsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 0 || stats.Chunks != 0 || stats.TenantSizeBytes != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.Embedding.Model != "test-model" || stats.Embedding.Dim != 3 || stats.Embedding.ChunkSize != 100 {
		t.Fatalf("embedding = %+v", stats.Embedding)
	}

	rec = env.callAs(t, "viewer1", "GET", "/api/documents", nil, "")
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
	env := newTestServer(t)
	rec := call(t, env.srv.Handler(), "GET", "/", nil, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("static status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "kb-rag") {
		t.Fatalf("static body = %q", rec.Body.String())
	}
}

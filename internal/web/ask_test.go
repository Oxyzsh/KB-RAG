package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAsk(t *testing.T) {
	env := newTestServer(t)
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工累计工作满1年不满10年的，年休假5天。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}

	rec = env.callAs(t, "viewer1", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Generated || resp.Answer == "" || len(resp.Citations) == 0 {
		t.Fatalf("ask = %+v", resp)
	}
	if resp.Citations[0].N != 1 || resp.Citations[0].Source != "年假.md" {
		t.Fatalf("citation = %+v", resp.Citations[0])
	}
}

func TestAskGenerationFailureReturnsCitations(t *testing.T) {
	env := newTestServer(t)
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	env.llmFake.fail = true

	rec = env.callAs(t, "viewer1", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Generated || resp.Warning == "" || len(resp.Citations) == 0 {
		t.Fatalf("ask = %+v", resp)
	}
}

func TestAskEmptyKnowledgeBase(t *testing.T) {
	env := newTestServer(t)
	rec := env.callAs(t, "viewer1", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAskValidation(t *testing.T) {
	env := newTestServer(t)
	cases := [][]byte{
		[]byte(`{"question":"  "}`),
		[]byte(`{"question":"q","top_k":21}`),
		[]byte(`not-json`),
	}
	for _, body := range cases {
		rec := env.callAs(t, "viewer1", "POST", "/api/ask", body, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d", body, rec.Code)
		}
	}
}

func TestAskStream(t *testing.T) {
	env := newTestServer(t)
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}

	rec = env.callAs(t, "viewer1", "POST", "/api/ask/stream", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("stream status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: citations", "event: delta", "event: done", "员工满1年", "享5天年假[1]。", "年假.md"} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE body 缺少 %q:\n%s", want, body)
		}
	}
}

func TestAskStreamValidation(t *testing.T) {
	env := newTestServer(t)
	rec := env.callAs(t, "viewer1", "POST", "/api/ask/stream", []byte(`{"question":"  "}`), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAskStreamEmptyKnowledgeBase(t *testing.T) {
	env := newTestServer(t)
	rec := env.callAs(t, "viewer1", "POST", "/api/ask/stream", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAskStreamGenerationFailure(t *testing.T) {
	env := newTestServer(t)
	if rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n")); rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	env.llmFake.fail = true

	rec := env.callAs(t, "viewer1", "POST", "/api/ask/stream", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("stream status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: citations") || !strings.Contains(body, "event: error") {
		t.Fatalf("SSE body = %s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("生成失败不应发送 done: %s", body)
	}
}

func TestAskTenantIsolation(t *testing.T) {
	env := newTestServer(t)
	if rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n")); rec.Code != http.StatusCreated {
		t.Fatalf("default upload = %d", rec.Code)
	}
	if rec := uploadDoc(t, env, "acme-admin", "食堂.md", []byte("# 食堂\n\n食堂中午十二点开放。\n")); rec.Code != http.StatusCreated {
		t.Fatalf("acme upload = %d", rec.Code)
	}

	rec := env.callAs(t, "viewer1", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json")
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].Source != "年假.md" {
		t.Fatalf("default citations = %+v", resp.Citations)
	}

	rec = env.callAs(t, "acme-admin", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].Source != "食堂.md" {
		t.Fatalf("acme citations = %+v", resp.Citations)
	}

	statsDefault := statsOf(t, env, "viewer1")
	statsAcme := statsOf(t, env, "acme-admin")
	if statsDefault.Documents != 1 || statsAcme.Documents != 1 {
		t.Fatalf("stats default=%+v acme=%+v", statsDefault, statsAcme)
	}
	if statsDefault.TenantSizeBytes == 0 || statsAcme.TenantSizeBytes == 0 {
		t.Fatalf("tenant size not computed: %+v %+v", statsDefault, statsAcme)
	}
}

func TestTenantIsolationDocuments(t *testing.T) {
	env := newTestServer(t)
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	rec = env.callAs(t, "acme-admin", "GET", "/api/documents", nil, "")
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 0 {
		t.Fatalf("acme 不应看到 default 文档: %+v", docs.Documents)
	}
	rec = env.callAs(t, "acme-admin", "DELETE", fmtID("/api/documents/%d", up.Document.ID), nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("跨租户删除 status = %d", rec.Code)
	}
	rec = env.callAs(t, "acme-admin", "POST", fmtID("/api/documents/%d/reingest", up.Document.ID), nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("跨租户重试 status = %d", rec.Code)
	}
}

func statsOf(t *testing.T, env *testEnv, who string) statsDTO {
	t.Helper()
	rec := env.callAs(t, who, "GET", "/api/stats", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d", rec.Code)
	}
	var stats statsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	return stats
}

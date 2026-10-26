package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kb-rag/internal/store"
)

func TestDocumentPreviewAndRaw(t *testing.T) {
	env := newTestServer(t)
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	id := up.Document.ID

	rec = env.callAs(t, "viewer1", "GET", fmtID("/api/documents/%d/preview", id), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	var preview documentPreviewDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "markdown" || preview.TextFormat != "markdown" || !preview.RawAvailable {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Name != "年假.md" || preview.Source != "upload" || !strings.Contains(preview.Text, "员工满1年") {
		t.Fatalf("preview = %+v", preview)
	}

	rec = env.callAs(t, "viewer1", "GET", fmtID("/api/documents/%d/raw", id), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("raw status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("raw content-type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "员工满1年") {
		t.Fatalf("raw body = %q", rec.Body.String())
	}

	if rec := env.callAs(t, "acme-admin", "GET", fmtID("/api/documents/%d/preview", id), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("跨租户 preview status = %d", rec.Code)
	}
	if rec := env.callAs(t, "acme-admin", "GET", fmtID("/api/documents/%d/raw", id), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("跨租户 raw status = %d", rec.Code)
	}
}

func TestExternalDocumentPreview(t *testing.T) {
	env := newTestServer(t)
	path := filepath.Join(env.dir, "external.md")
	if err := os.WriteFile(path, []byte("# 外部\n\n外部内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := env.store.ReplaceDocument(ctx, env.defaultT.ID, path, "外部文档", "hash-external", []store.ChunkInput{
		{Seq: 0, Heading: "外部", Text: "外部内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	info, ok, err := env.store.GetDocument(ctx, env.defaultT.ID, path)
	if err != nil || !ok {
		t.Fatalf("document: ok=%v err=%v", ok, err)
	}

	rec := env.callAs(t, "viewer1", "GET", fmtID("/api/documents/%d/preview", info.ID), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	var preview documentPreviewDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Source != "external" || preview.RawAvailable || preview.TextFormat != "plain" {
		t.Fatalf("preview = %+v", preview)
	}
	if !strings.Contains(preview.Text, "外部内容") {
		t.Fatalf("preview text = %q", preview.Text)
	}

	if rec := env.callAs(t, "viewer1", "GET", fmtID("/api/documents/%d/raw", info.ID), nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("external raw status = %d", rec.Code)
	}
}

func TestDocumentPreviewNotFound(t *testing.T) {
	env := newTestServer(t)
	if rec := env.callAs(t, "viewer1", "GET", "/api/documents/999/preview", nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

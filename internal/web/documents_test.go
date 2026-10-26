package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func uploadDoc(t *testing.T, srv *Server, token, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, "file", filename, content)
	return call(t, srv.Handler(), "POST", "/api/documents", body.Bytes(), token, ct)
}

func uploadedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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

func TestUploadAndList(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年不满10年的，年休假5天。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.Outcome != "ok" || up.Document.Status != "ok" || up.Document.Chunks == 0 {
		t.Fatalf("upload = %+v", up)
	}
	if up.Document.Name != "年假.md" || up.Document.Source != "upload" {
		t.Fatalf("document = %+v", up.Document)
	}

	rec = call(t, srv.Handler(), "GET", "/api/documents", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 1 || docs.Documents[0].ID != up.Document.ID {
		t.Fatalf("documents = %+v", docs.Documents)
	}
}

func TestUploadRejectsUnsupportedType(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "image.png", []byte("not an image"))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUploadRejectsTooLarge(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	content := bytes.Repeat([]byte("a"), 2<<20)
	rec := uploadDoc(t, srv, token, "big.md", content)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUploadSanitizesFilename(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "../../evil.md", []byte("# 标题\n\n一段足够长的正文内容。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.Document.Name != "evil.md" {
		t.Fatalf("name = %q", up.Document.Name)
	}
	files := uploadedFiles(t, srv.uploadDir)
	if len(files) != 1 {
		t.Fatalf("uploaded files = %v", files)
	}
	rel, err := filepath.Rel(srv.uploadDir, files[0])
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("文件逃逸上传目录: %s rel=%s err=%v", files[0], rel, err)
	}
}

func TestSafeUploadName(t *testing.T) {
	cases := map[string]string{
		"../../evil.md": "evil.md",
		"CON.md":        "_CON.md",
		"a*b?.md":       "a_b_.md",
		"":              "upload",
		"..":            "upload",
	}
	for in, want := range cases {
		if got := safeUploadName(in); got != want {
			t.Errorf("safeUploadName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeleteKeepsSourceFile(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	filesBefore := uploadedFiles(t, srv.uploadDir)
	if len(filesBefore) != 1 {
		t.Fatalf("files before = %v", filesBefore)
	}

	rec = call(t, srv.Handler(), "DELETE", fmt.Sprintf("/api/documents/%d", up.Document.ID), nil, token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = call(t, srv.Handler(), "GET", "/api/documents", nil, token, "")
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 0 {
		t.Fatalf("删除后文档 = %+v", docs.Documents)
	}
	if _, err := os.Stat(filesBefore[0]); err != nil {
		t.Fatalf("源文件被删除: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := call(t, srv.Handler(), "DELETE", "/api/documents/999", nil, token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReingestSkipped(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}

	rec = call(t, srv.Handler(), "POST", fmt.Sprintf("/api/documents/%d/reingest", up.Document.ID), nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reingest status = %d body=%s", rec.Code, rec.Body.String())
	}
	var again uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again.Outcome != "skipped" || again.Document.Chunks == 0 {
		t.Fatalf("reingest = %+v", again)
	}
}

func TestReingestMissingSource(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	files := uploadedFiles(t, srv.uploadDir)
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}

	rec = call(t, srv.Handler(), "POST", fmt.Sprintf("/api/documents/%d/reingest", up.Document.ID), nil, token, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

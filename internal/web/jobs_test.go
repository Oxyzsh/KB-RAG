package web

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"kb-rag/internal/store"
)

func uploadJob(t *testing.T, env *testEnv, who, filename string, content []byte) jobDTO {
	t.Helper()
	body, ct := multipartBody(t, "file", filename, content)
	rec := env.callAs(t, who, "POST", "/api/jobs/documents", body.Bytes(), ct)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload job status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Job
}

func getJob(t *testing.T, env *testEnv, who string, id int64) jobDTO {
	t.Helper()
	rec := env.callAs(t, who, "GET", fmtID("/api/jobs/%d", id), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get job status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Job
}

func waitJob(t *testing.T, env *testEnv, who string, id int64, want string) jobDTO {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job := getJob(t, env, who, id)
		if job.Status == want {
			return job
		}
		if job.Status == store.JobFailed || job.Status == store.JobCanceled {
			if want == store.JobDone {
				t.Fatalf("任务提前结束: %+v", job)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	job := getJob(t, env, who, id)
	t.Fatalf("等待任务 %s 超时: %+v", want, job)
	return jobDTO{}
}

func TestAsyncUploadJob(t *testing.T) {
	env := newTestServer(t)
	env.srv.Start(t.Context())

	job := uploadJob(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	if job.Status != store.JobQueued || job.Name != "年假.md" {
		t.Fatalf("job = %+v", job)
	}
	done := waitJob(t, env, "editor1", job.ID, store.JobDone)
	if done.Chunks == 0 || done.Stage != "完成" {
		t.Fatalf("done = %+v", done)
	}

	rec := env.callAs(t, "viewer1", "GET", "/api/documents", nil, "")
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 1 || docs.Documents[0].Name != "年假.md" {
		t.Fatalf("documents = %+v", docs.Documents)
	}

	rec = env.callAs(t, "admin1", "GET", "/api/audit?action=document.upload", nil, "")
	var audit auditListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &audit); err != nil {
		t.Fatal(err)
	}
	if len(audit.Items) != 1 || audit.Items[0].Username != "editor1" {
		t.Fatalf("audit = %+v", audit.Items)
	}
}

func TestAsyncUploadJobInvalidType(t *testing.T) {
	env := newTestServer(t)
	body, ct := multipartBody(t, "file", "image.png", []byte("not an image"))
	rec := env.callAs(t, "editor1", "POST", "/api/jobs/documents", body.Bytes(), ct)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAsyncReingestJob(t *testing.T) {
	env := newTestServer(t)
	env.srv.Start(t.Context())
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}

	rec = env.callAs(t, "editor1", "POST", fmtID("/api/jobs/documents/%d/reingest", up.Document.ID), nil, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("reingest job status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, env, "editor1", resp.Job.ID, store.JobDone)
	if done.Stage != "已是最新" || done.Chunks == 0 {
		t.Fatalf("reingest job = %+v", done)
	}
}

func TestCancelRunningJob(t *testing.T) {
	env := newTestServer(t)
	gate := make(chan struct{})
	env.emb.gate = gate
	defer close(gate)
	env.srv.Start(t.Context())

	job := uploadJob(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	waitJob(t, env, "editor1", job.ID, store.JobRunning)

	rec := env.callAs(t, "editor1", "DELETE", fmtID("/api/jobs/%d", job.ID), nil, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d body=%s", rec.Code, rec.Body.String())
	}
	canceled := waitJob(t, env, "editor1", job.ID, store.JobCanceled)
	if canceled.Error == "" {
		t.Fatalf("canceled job = %+v", canceled)
	}
}

func TestJobTenantIsolation(t *testing.T) {
	env := newTestServer(t)
	env.srv.Start(t.Context())
	job := uploadJob(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	waitJob(t, env, "editor1", job.ID, store.JobDone)

	if rec := env.callAs(t, "acme-admin", "GET", fmtID("/api/jobs/%d", job.ID), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("跨租户读取任务 status = %d", rec.Code)
	}
	if rec := env.callAs(t, "acme-admin", "DELETE", fmtID("/api/jobs/%d", job.ID), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("跨租户取消任务 status = %d", rec.Code)
	}
	if rec := env.callAs(t, "viewer1", "POST", "/api/jobs/documents", nil, "multipart/form-data"); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer 创建任务 status = %d", rec.Code)
	}
}

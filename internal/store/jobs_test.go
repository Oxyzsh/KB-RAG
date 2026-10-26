package store

import (
	"context"
	"testing"
	"time"
)

func TestJobLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tenantID := testTenant(t, s)

	job, err := s.CreateJob(ctx, Job{
		TenantID: tenantID,
		UserID:   1,
		Username: "admin",
		Action:   "document.upload",
		Path:     "/tmp/a.md",
		Name:     "a.md",
		IP:       "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobQueued || job.ID == 0 {
		t.Fatalf("job = %+v", job)
	}
	got, ok, err := s.GetJob(ctx, tenantID, job.ID)
	if err != nil || !ok || got.Name != "a.md" {
		t.Fatalf("GetJob = %+v, ok=%v, err=%v", got, ok, err)
	}
	if _, ok, err := s.GetJob(ctx, tenantID+1, job.ID); err != nil || ok {
		t.Fatalf("跨租户任务应不可见: ok=%v err=%v", ok, err)
	}

	running, ok, err := s.NextQueuedJob(ctx)
	if err != nil || !ok || running.ID != job.ID || running.Status != JobRunning {
		t.Fatalf("NextQueuedJob = %+v, ok=%v, err=%v", running, ok, err)
	}
	if err := s.UpdateJobStage(ctx, job.ID, "生成向量"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.GetJob(ctx, tenantID, job.ID)
	if got.Stage != "生成向量" || got.Status != JobRunning {
		t.Fatalf("stage = %+v", got)
	}
	if err := s.UpdateJob(ctx, job.ID, JobDone, "完成", 3, ""); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.GetJob(ctx, tenantID, job.ID)
	if got.Status != JobDone || got.Chunks != 3 || got.Stage != "完成" {
		t.Fatalf("done = %+v", got)
	}
	if canceled, err := s.CancelQueuedJob(ctx, tenantID, job.ID); err != nil || canceled {
		t.Fatalf("已完成任务不应可取消: canceled=%v err=%v", canceled, err)
	}
}

func TestCancelQueuedJobAndInterrupted(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tenantID := testTenant(t, s)
	create := func(name string) Job {
		job, err := s.CreateJob(ctx, Job{
			TenantID: tenantID, UserID: 1, Username: "admin",
			Action: "document.upload", Path: "/tmp/" + name, Name: name,
		})
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	queued := create("queued.md")
	canceled, err := s.CancelQueuedJob(ctx, tenantID, queued.ID)
	if err != nil || !canceled {
		t.Fatalf("cancel = %v, %v", canceled, err)
	}
	got, _, _ := s.GetJob(ctx, tenantID, queued.ID)
	if got.Status != JobCanceled {
		t.Fatalf("canceled job = %+v", got)
	}

	runningJob := create("running.md")
	_ = create("pending.md")
	if _, ok, err := s.NextQueuedJob(ctx); err != nil || !ok {
		t.Fatalf("NextQueuedJob: ok=%v err=%v", ok, err)
	}
	n, err := s.MarkInterruptedJobs(ctx)
	if err != nil || n != 1 {
		t.Fatalf("MarkInterruptedJobs = %d, %v", n, err)
	}
	got, _, _ = s.GetJob(ctx, tenantID, runningJob.ID)
	if got.Status != JobFailed {
		t.Fatalf("interrupted job = %+v", got)
	}
	next, ok, err := s.NextQueuedJob(ctx)
	if err != nil || !ok || next.Name != "pending.md" {
		t.Fatalf("排队任务应保留: next=%+v ok=%v err=%v", next, ok, err)
	}

	if _, err := s.DeleteOldJobs(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

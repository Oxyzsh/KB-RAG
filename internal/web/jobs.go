package web

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"kb-rag/internal/ingest"
	"kb-rag/internal/store"
)

func jobDTOFromStore(job store.Job) jobDTO {
	return jobDTO{
		ID:        job.ID,
		Name:      job.Name,
		Action:    job.Action,
		Status:    job.Status,
		Stage:     job.Stage,
		Chunks:    job.Chunks,
		Error:     job.Error,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	}
}

// Start 启动异步入库 worker。重复调用是安全的；ctx 取消时 worker 停止。
func (s *Server) Start(ctx context.Context) {
	s.workerMu.Lock()
	if s.workerCancel != nil {
		s.workerMu.Unlock()
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	s.workerCancel = cancel
	s.workerMu.Unlock()

	if n, err := s.store.MarkInterruptedJobs(workerCtx); err != nil {
		s.logger.Warn("标记中断任务失败", "error", err)
	} else if n > 0 {
		s.logger.Warn("有任务在服务重启时中断", "count", n)
	}
	if n, err := s.store.DeleteOldJobs(workerCtx, time.Now().Add(-7*24*time.Hour)); err != nil {
		s.logger.Warn("清理旧任务失败", "error", err)
	} else if n > 0 {
		s.logger.Info("清理旧任务", "count", n)
	}
	go s.runJobWorker(workerCtx)
}

func (s *Server) runJobWorker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if s.processNextJob(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) processNextJob(ctx context.Context) bool {
	job, ok, err := s.store.NextQueuedJob(ctx)
	if err != nil {
		s.logger.Error("读取排队任务失败", "error", err)
		return false
	}
	if !ok {
		return false
	}

	jobCtx, cancel := context.WithCancel(ctx)
	s.workerMu.Lock()
	s.jobCancels[job.ID] = cancel
	s.workerMu.Unlock()
	defer func() {
		s.workerMu.Lock()
		delete(s.jobCancels, job.ID)
		s.workerMu.Unlock()
		cancel()
	}()

	opts := s.ingestOptions
	opts.TenantID = job.TenantID
	opts.Progress = func(stage string) {
		_ = s.store.UpdateJobStage(jobCtx, job.ID, stage)
	}
	s.ingestMu.Lock()
	res := ingest.IngestFile(jobCtx, job.Path, s.store, s.embedder, opts)
	s.ingestMu.Unlock()

	status, stage, errMsg := jobOutcome(jobCtx, res)
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	chunks := res.Chunks
	if info, ok, err := s.store.GetDocument(finalCtx, job.TenantID, job.Path); err == nil && ok {
		if summary, ok, err := s.store.GetDocumentSummaryByID(finalCtx, job.TenantID, info.ID); err == nil && ok {
			chunks = summary.Chunks
		}
	}
	if err := s.store.UpdateJob(finalCtx, job.ID, status, stage, chunks, errMsg); err != nil {
		s.logger.Error("更新任务状态失败", "error", err, "id", job.ID)
	}
	detail, _ := json.Marshal(map[string]any{
		"outcome": status,
		"chunks":  chunks,
		"error":   errMsg,
	})
	s.writeAudit(store.AuditEntry{
		TenantID: &job.TenantID,
		UserID:   &job.UserID,
		Username: job.Username,
		Action:   job.Action,
		Target:   job.Name,
		Detail:   string(detail),
		IP:       job.IP,
	})
	return true
}

func jobOutcome(ctx context.Context, res ingest.FileResult) (status, stage, errMsg string) {
	if ctx.Err() != nil || errors.Is(res.Err, context.Canceled) {
		return store.JobCanceled, "已取消", "已取消"
	}
	switch {
	case res.Fatal:
		return store.JobFailed, "失败", errorText(res.Err)
	case res.Outcome == ingest.OutcomeOK:
		return store.JobDone, "完成", ""
	case res.Outcome == ingest.OutcomeSkipped:
		return store.JobDone, "已是最新", ""
	default:
		return store.JobFailed, "失败", errorText(res.Err)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Server) cancelRunningJob(id int64) bool {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	cancel, ok := s.jobCancels[id]
	if ok {
		cancel()
	}
	return ok
}

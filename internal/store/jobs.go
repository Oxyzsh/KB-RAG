package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	JobQueued   = "queued"
	JobRunning  = "running"
	JobDone     = "done"
	JobFailed   = "failed"
	JobCanceled = "canceled"
)

type Job struct {
	ID        int64
	TenantID  int64
	UserID    int64
	Username  string
	Action    string
	Path      string
	Name      string
	Status    string
	Stage     string
	Chunks    int
	Error     string
	IP        string
	CreatedAt string
	UpdatedAt string
}

func (s *Store) CreateJob(ctx context.Context, job Job) (Job, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `INSERT INTO jobs(tenant_id,user_id,username,action,path,name,status,stage,chunks,error,ip,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,0,'',?,?,?)`,
		job.TenantID, job.UserID, job.Username, job.Action, job.Path, job.Name, JobQueued, "排队中", job.IP, now, now)
	if err != nil {
		return Job{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Job{}, err
	}
	job.ID = id
	job.Status = JobQueued
	job.Stage = "排队中"
	job.CreatedAt = now
	job.UpdatedAt = now
	return job, nil
}

func (s *Store) GetJob(ctx context.Context, tenantID, id int64) (Job, bool, error) {
	job, err := scanJob(s.db.QueryRowContext(ctx, jobSelect+` WHERE id=? AND tenant_id=?`, id, tenantID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (s *Store) NextQueuedJob(ctx context.Context) (Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()

	job, err := scanJob(tx.QueryRowContext(ctx, jobSelect+` WHERE status=? ORDER BY id LIMIT 1`, JobQueued))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `UPDATE jobs SET status=?, stage=?, updated_at=? WHERE id=? AND status=?`,
		JobRunning, "准备中", now, job.ID, JobQueued)
	if err != nil {
		return Job{}, false, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		if err != nil {
			return Job{}, false, err
		}
		return Job{}, false, nil
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	job.Status = JobRunning
	job.Stage = "准备中"
	job.UpdatedAt = now
	return job, true, nil
}

func (s *Store) UpdateJobStage(ctx context.Context, id int64, stage string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET stage=?, updated_at=? WHERE id=? AND status=?`,
		stage, now, id, JobRunning)
	return err
}

func (s *Store) UpdateJob(ctx context.Context, id int64, status, stage string, chunks int, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status=?, stage=?, chunks=?, error=?, updated_at=? WHERE id=?`,
		status, stage, chunks, errMsg, now, id)
	return err
}

func (s *Store) CancelQueuedJob(ctx context.Context, tenantID, id int64) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status=?, stage=?, error=?, updated_at=? WHERE id=? AND tenant_id=? AND status=?`,
		JobCanceled, "已取消", "已取消", now, id, tenantID, JobQueued)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// MarkInterruptedJobs 将上次进程退出时仍在运行的任务标记失败；排队任务保留，由 worker 重新处理。
func (s *Store) MarkInterruptedJobs(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status=?, stage=?, error=?, updated_at=? WHERE status=?`,
		JobFailed, "已中断", "服务重启导致任务中断", now, JobRunning)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) DeleteOldJobs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE status IN (?,?,?) AND updated_at < ?`,
		JobDone, JobFailed, JobCanceled, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

const jobSelect = `SELECT id,tenant_id,user_id,username,action,path,name,status,stage,chunks,error,ip,created_at,updated_at FROM jobs`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (Job, error) {
	var job Job
	err := row.Scan(&job.ID, &job.TenantID, &job.UserID, &job.Username, &job.Action, &job.Path, &job.Name,
		&job.Status, &job.Stage, &job.Chunks, &job.Error, &job.IP, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}

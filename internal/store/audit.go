package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AuditEntry struct {
	ID         int64
	TenantID   *int64
	TenantSlug string
	UserID     *int64
	Username   string
	Action     string
	Target     string
	Detail     string
	IP         string
	CreatedAt  string
}

type AuditFilter struct {
	TenantID *int64
	Action   string
	From     string
	To       string
	Page     int
	PageSize int
}

func (s *Store) InsertAudit(ctx context.Context, e AuditEntry) error {
	created := e.CreatedAt
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs(tenant_id,user_id,username,action,target,detail,ip,created_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		nullableInt64(e.TenantID), nullableInt64(e.UserID), e.Username, e.Action, e.Target, e.Detail, e.IP, created)
	return err
}

func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, int, error) {
	page := f.Page
	if page <= 0 {
		page = 1
	}
	size := f.PageSize
	if size <= 0 {
		size = 50
	}
	if size > 200 {
		size = 200
	}

	var where []string
	var args []any
	if f.TenantID != nil {
		where = append(where, "a.tenant_id=?")
		args = append(args, *f.TenantID)
	}
	if f.Action != "" {
		where = append(where, "a.action=?")
		args = append(args, f.Action)
	}
	if f.From != "" {
		where = append(where, "a.created_at>=?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "a.created_at<=?")
		args = append(args, f.To)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs a`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT a.id,a.tenant_id,COALESCE(t.slug,''),a.user_id,a.username,a.action,a.target,a.detail,a.ip,a.created_at
		FROM audit_logs a LEFT JOIN tenants t ON t.id=a.tenant_id%s ORDER BY a.id DESC LIMIT ? OFFSET ?`, clause)
	args = append(args, size, (page-1)*size)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]AuditEntry, 0, size)
	for rows.Next() {
		var e AuditEntry
		var tenantID, userID *int64
		if err := rows.Scan(&e.ID, &tenantID, &e.TenantSlug, &userID, &e.Username, &e.Action, &e.Target, &e.Detail, &e.IP, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		e.TenantID = tenantID
		e.UserID = userID
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

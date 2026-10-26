package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Tenant struct {
	ID        int64
	Slug      string
	Name      string
	CreatedAt string
}

type User struct {
	ID           int64
	TenantID     int64
	Username     string
	PasswordHash string
	Role         string
	IsSuper      bool
	CreatedAt    string
	UpdatedAt    string
}

type Principal struct {
	UserID     int64
	Username   string
	Role       string
	IsSuper    bool
	TenantID   int64
	TenantSlug string
	TenantName string
	// HomeTenant* 是用户所属租户；Tenant* 是本次请求的生效租户，可通过切换覆盖。
	HomeTenantID   int64
	HomeTenantSlug string
	HomeTenantName string
	TokenHash      string
}

var (
	ErrTenantNotFound   = errors.New("租户不存在")
	ErrUserNotFound     = errors.New("用户不存在")
	ErrUsernameTaken    = errors.New("用户名已存在")
	ErrTenantSlugTaken  = errors.New("租户标识已存在")
	ErrLastAdmin        = errors.New("不能删除或降级租户最后一个管理员")
	ErrProtectedUser    = errors.New("不能修改或删除超级管理员")
	ErrSelfDelete       = errors.New("不能删除当前登录用户")
	ErrSessionNotFound  = errors.New("会话不存在或已过期")
	ErrTenantNameExists = errors.New("租户名称已存在")
)

func ValidRole(role string) bool {
	return role == "admin" || role == "editor" || role == "viewer"
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) TenantBySlug(ctx context.Context, slug string) (Tenant, bool, error) {
	var t Tenant
	err := s.db.QueryRowContext(ctx, `SELECT id,slug,name,created_at FROM tenants WHERE slug=?`, slug).
		Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, false, nil
	}
	if err != nil {
		return Tenant{}, false, err
	}
	return t, true, nil
}

func (s *Store) GetTenant(ctx context.Context, id int64) (Tenant, bool, error) {
	var t Tenant
	err := s.db.QueryRowContext(ctx, `SELECT id,slug,name,created_at FROM tenants WHERE id=?`, id).
		Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, false, nil
	}
	if err != nil {
		return Tenant{}, false, err
	}
	return t, true, nil
}

func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,slug,name,created_at FROM tenants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// EnsureBootstrapAdmin 仅在 users 表为空时创建默认租户的超级管理员。
// 返回 created=false 表示已有用户，本次不创建。
func (s *Store) EnsureBootstrapAdmin(ctx context.Context, username, passwordHash string) (User, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, false, err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return User{}, false, err
	}
	if count > 0 {
		return User{}, false, nil
	}

	var tenantID int64
	now := time.Now().UTC().Format(time.RFC3339)
	err = tx.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug=?`, defaultTenantSlug).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `INSERT INTO tenants(slug,name,created_at) VALUES(?,?,?)`, defaultTenantSlug, defaultTenantName, now)
		if err != nil {
			return User{}, false, err
		}
		tenantID, err = res.LastInsertId()
		if err != nil {
			return User{}, false, err
		}
	} else if err != nil {
		return User{}, false, err
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO users(tenant_id,username,password_hash,role,is_super,created_at,updated_at)
		VALUES(?,?,?,'admin',1,?,?)`, tenantID, username, passwordHash, now, now)
	if err != nil {
		return User{}, false, fmt.Errorf("创建初始管理员: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, false, err
	}
	return User{ID: id, TenantID: tenantID, Username: username, PasswordHash: passwordHash, Role: "admin", IsSuper: true, CreatedAt: now, UpdatedAt: now}, true, nil
}

func (s *Store) CreateTenantWithAdmin(ctx context.Context, slug, name, username, passwordHash string) (Tenant, User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tenant{}, User{}, err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE slug=?`, slug).Scan(&n); err != nil {
		return Tenant{}, User{}, err
	}
	if n > 0 {
		return Tenant{}, User{}, ErrTenantSlugTaken
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE name=?`, name).Scan(&n); err != nil {
		return Tenant{}, User{}, err
	}
	if n > 0 {
		return Tenant{}, User{}, ErrTenantNameExists
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `INSERT INTO tenants(slug,name,created_at) VALUES(?,?,?)`, slug, name, now)
	if err != nil {
		return Tenant{}, User{}, err
	}
	tenantID, err := res.LastInsertId()
	if err != nil {
		return Tenant{}, User{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id=? AND username=?`, tenantID, username).Scan(&n); err != nil {
		return Tenant{}, User{}, err
	}
	if n > 0 {
		return Tenant{}, User{}, ErrUsernameTaken
	}
	ures, err := tx.ExecContext(ctx, `INSERT INTO users(tenant_id,username,password_hash,role,is_super,created_at,updated_at)
		VALUES(?,?,?,'admin',0,?,?)`, tenantID, username, passwordHash, now, now)
	if err != nil {
		return Tenant{}, User{}, err
	}
	userID, err := ures.LastInsertId()
	if err != nil {
		return Tenant{}, User{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tenant{}, User{}, err
	}
	return Tenant{ID: tenantID, Slug: slug, Name: name, CreatedAt: now},
		User{ID: userID, TenantID: tenantID, Username: username, PasswordHash: passwordHash, Role: "admin", CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, tenantID int64, username string) (User, bool, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id,tenant_id,username,password_hash,role,is_super,created_at,updated_at
		FROM users WHERE tenant_id=? AND username=?`, tenantID, username).
		Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Role, &u.IsSuper, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

func (s *Store) GetUser(ctx context.Context, tenantID, id int64) (User, bool, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id,tenant_id,username,password_hash,role,is_super,created_at,updated_at
		FROM users WHERE id=? AND tenant_id=?`, id, tenantID).
		Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Role, &u.IsSuper, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

func (s *Store) ListUsers(ctx context.Context, tenantID int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,tenant_id,username,password_hash,role,is_super,created_at,updated_at
		FROM users WHERE tenant_id=? ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Role, &u.IsSuper, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, tenantID int64, username, passwordHash, role string) (User, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id=? AND username=?`, tenantID, username).Scan(&n); err != nil {
		return User{}, err
	}
	if n > 0 {
		return User{}, ErrUsernameTaken
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `INSERT INTO users(tenant_id,username,password_hash,role,is_super,created_at,updated_at)
		VALUES(?,?,?,?,0,?,?)`, tenantID, username, passwordHash, role, now, now)
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, TenantID: tenantID, Username: username, PasswordHash: passwordHash, Role: role, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) UpdateUserRole(ctx context.Context, tenantID, id int64, role string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	u, ok, err := getUserTx(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUserNotFound
	}
	if u.IsSuper {
		return ErrProtectedUser
	}
	if u.Role == "admin" && role != "admin" {
		n, err := countAdminsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role=?, updated_at=? WHERE id=? AND tenant_id=?`, role, now, id, tenantID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ResetUserPassword(ctx context.Context, tenantID, id int64, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	u, ok, err := getUserTx(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUserNotFound
	}
	if u.IsSuper {
		return ErrProtectedUser
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=? AND tenant_id=?`, passwordHash, now, id, tenantID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetTenantAdminPassword 供超级管理员重置任一租户的管理员密码。
func (s *Store) ResetTenantAdminPassword(ctx context.Context, tenantID int64, username, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	var role string
	var isSuper bool
	err = tx.QueryRowContext(ctx, `SELECT id,role,is_super FROM users WHERE tenant_id=? AND username=?`, tenantID, username).
		Scan(&id, &role, &isSuper)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if role != "admin" || isSuper {
		return ErrUserNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, passwordHash, now, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteUser(ctx context.Context, tenantID, id, actorID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	u, ok, err := getUserTx(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUserNotFound
	}
	if u.IsSuper {
		return ErrProtectedUser
	}
	if id == actorID {
		return ErrSelfDelete
	}
	if u.Role == "admin" {
		n, err := countAdminsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id=? AND tenant_id=?`, id, tenantID); err != nil {
		return err
	}
	return tx.Commit()
}

func getUserTx(ctx context.Context, tx *sql.Tx, tenantID, id int64) (User, bool, error) {
	var u User
	err := tx.QueryRowContext(ctx, `SELECT id,tenant_id,username,password_hash,role,is_super,created_at,updated_at
		FROM users WHERE id=? AND tenant_id=?`, id, tenantID).
		Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Role, &u.IsSuper, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

func countAdminsTx(ctx context.Context, tx *sql.Tx, tenantID int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id=? AND role='admin'`, tenantID).Scan(&n)
	return n, err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)`,
		tokenHash, userID, now, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) PrincipalByTokenHash(ctx context.Context, tokenHash string, now time.Time) (Principal, bool, error) {
	var p Principal
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.role,u.is_super,t.id,t.slug,t.name,s.token_hash
		FROM sessions s JOIN users u ON u.id=s.user_id JOIN tenants t ON t.id=u.tenant_id
		WHERE s.token_hash=? AND s.expires_at>?`, tokenHash, now.UTC().Format(time.RFC3339)).
		Scan(&p.UserID, &p.Username, &p.Role, &p.IsSuper, &p.TenantID, &p.TenantSlug, &p.TenantName, &p.TokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, err
	}
	p.HomeTenantID = p.TenantID
	p.HomeTenantSlug = p.TenantSlug
	p.HomeTenantName = p.TenantName
	return p, true, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at<=?`, now.UTC().Format(time.RFC3339))
	return err
}

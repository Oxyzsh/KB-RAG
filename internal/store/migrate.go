package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	defaultTenantSlug = "default"
	defaultTenantName = "默认租户"
)

const schemaV3Statements = `
CREATE TABLE IF NOT EXISTS tenants(
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER NOT NULL REFERENCES tenants(id),
	username TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL CHECK(role IN ('admin','editor','viewer')),
	is_super INTEGER NOT NULL DEFAULT 0 CHECK(is_super IN (0,1)),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(tenant_id, username),
	CHECK(is_super = 0 OR role = 'admin')
);
CREATE TABLE IF NOT EXISTS sessions(
	token_hash TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_logs(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER,
	user_id INTEGER,
	username TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL,
	target TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	ip TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS documents(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER NOT NULL REFERENCES tenants(id),
	path TEXT NOT NULL,
	path_key TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'ok',
	error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL,
	UNIQUE(tenant_id, path_key)
);
CREATE TABLE IF NOT EXISTS chunks(
	id INTEGER PRIMARY KEY,
	doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
	seq INTEGER NOT NULL,
	heading TEXT NOT NULL DEFAULT '',
	text TEXT NOT NULL,
	vector BLOB NOT NULL,
	dim INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chunks_doc ON chunks(doc_id);
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS jobs(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	action TEXT NOT NULL,
	path TEXT NOT NULL,
	name TEXT NOT NULL,
	status TEXT NOT NULL,
	stage TEXT NOT NULL DEFAULT '',
	chunks INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	ip TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, id);
CREATE INDEX IF NOT EXISTS idx_jobs_tenant ON jobs(tenant_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_documents_tenant_updated ON documents(tenant_id, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_documents_tenant_status ON documents(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_created ON audit_logs(tenant_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at DESC, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_single_super ON users(is_super) WHERE is_super = 1;
`

const migrationTables = `
CREATE TABLE tenants(
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE users(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER NOT NULL REFERENCES tenants(id),
	username TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL CHECK(role IN ('admin','editor','viewer')),
	is_super INTEGER NOT NULL DEFAULT 0 CHECK(is_super IN (0,1)),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(tenant_id, username),
	CHECK(is_super = 0 OR role = 'admin')
);
CREATE TABLE sessions(
	token_hash TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE audit_logs(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER,
	user_id INTEGER,
	username TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL,
	target TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	ip TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE documents_v3(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER NOT NULL REFERENCES tenants(id),
	path TEXT NOT NULL,
	path_key TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'ok',
	error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL,
	UNIQUE(tenant_id, path_key)
);
`

const migrationIndexes = `
CREATE INDEX idx_documents_tenant_updated ON documents(tenant_id, updated_at DESC, id DESC);
CREATE INDEX idx_documents_tenant_status ON documents(tenant_id, status);
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);
CREATE INDEX idx_audit_tenant_created ON audit_logs(tenant_id, created_at DESC, id DESC);
CREATE INDEX idx_audit_created ON audit_logs(created_at DESC, id DESC);
CREATE UNIQUE INDEX idx_users_single_super ON users(is_super) WHERE is_super = 1;
`

const schemaV4Jobs = `
CREATE TABLE jobs(
	id INTEGER PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	action TEXT NOT NULL,
	path TEXT NOT NULL,
	name TEXT NOT NULL,
	status TEXT NOT NULL,
	stage TEXT NOT NULL DEFAULT '',
	chunks INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	ip TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX idx_jobs_status ON jobs(status, id);
CREATE INDEX idx_jobs_tenant ON jobs(tenant_id, id DESC);
`

func (s *Store) initFreshV4(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaV3Statements); err != nil {
		return fmt.Errorf("初始化数据库: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO tenants(slug,name,created_at) VALUES(?,?,?)
		ON CONFLICT(slug) DO NOTHING`, defaultTenantSlug, defaultTenantName, now); err != nil {
		return fmt.Errorf("创建默认租户: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, SchemaVersion); err != nil {
		return fmt.Errorf("写入 schema 版本: %w", err)
	}
	return tx.Commit()
}

func (s *Store) migrateV2ToV3(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("关闭外键: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
	}()
	var fk int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return err
	}
	if fk != 0 {
		return fmt.Errorf("迁移需要先关闭外键，当前 foreign_keys=%d", fk)
	}

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("开始迁移事务: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	var docsBefore, chunksBefore int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&docsBefore); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&chunksBefore); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx, migrationTables); err != nil {
		return fmt.Errorf("创建 v3 表: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenants(slug,name,created_at) VALUES(?,?,?)`,
		defaultTenantSlug, defaultTenantName, now); err != nil {
		return fmt.Errorf("创建默认租户: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO documents_v3(id,tenant_id,path,path_key,title,content_hash,status,error,updated_at)
		SELECT id,(SELECT id FROM tenants WHERE slug=?),path,path_key,title,content_hash,status,error,updated_at FROM documents`,
		defaultTenantSlug); err != nil {
		return fmt.Errorf("迁移文档数据: %w", err)
	}
	var docsCopied int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents_v3`).Scan(&docsCopied); err != nil {
		return err
	}
	if docsCopied != docsBefore {
		return fmt.Errorf("文档数量不一致：迁移前 %d，迁移后 %d", docsBefore, docsCopied)
	}
	if _, err := conn.ExecContext(ctx, `DROP TABLE documents`); err != nil {
		return fmt.Errorf("删除旧 documents 表: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `ALTER TABLE documents_v3 RENAME TO documents`); err != nil {
		return fmt.Errorf("重命名 documents 表: %w", err)
	}
	if _, err := conn.ExecContext(ctx, migrationIndexes); err != nil {
		return fmt.Errorf("创建索引: %w", err)
	}

	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("外键检查: %w", err)
	}
	hasOrphan := rows.Next()
	rows.Close()
	if hasOrphan {
		return fmt.Errorf("迁移后发现外键孤儿数据，已回滚")
	}

	var chunksAfter int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&chunksAfter); err != nil {
		return err
	}
	if chunksAfter != chunksBefore {
		return fmt.Errorf("片段数量不一致：迁移前 %d，迁移后 %d", chunksBefore, chunksAfter)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version','3')
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		return fmt.Errorf("更新 schema 版本: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return fmt.Errorf("提交迁移事务: %w", err)
	}
	committed = true

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		return fmt.Errorf("恢复外键: %w", err)
	}
	var fkAfter int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fkAfter); err != nil {
		return err
	}
	if fkAfter != 1 {
		return fmt.Errorf("迁移后外键未恢复（foreign_keys=%d）", fkAfter)
	}
	return nil
}

func (s *Store) migrateV3ToV4(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaV4Jobs); err != nil {
		return fmt.Errorf("创建任务表: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version','4')
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		return fmt.Errorf("更新 schema 版本: %w", err)
	}
	return tx.Commit()
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0, err
}

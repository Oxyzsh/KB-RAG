package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func createV2DB(t *testing.T, path string, withExtraTable bool) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE documents(
			id INTEGER PRIMARY KEY,
			path TEXT NOT NULL,
			path_key TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE chunks(
			id INTEGER PRIMARY KEY,
			doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			heading TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			vector BLOB NOT NULL,
			dim INTEGER NOT NULL
		)`,
		`CREATE INDEX idx_chunks_doc ON chunks(doc_id)`,
		`CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO meta(key,value) VALUES('schema_version','2')`,
		`INSERT INTO meta(key,value) VALUES('embedding_model','bge-m3')`,
		`INSERT INTO meta(key,value) VALUES('chunk_size','500')`,
		`INSERT INTO meta(key,value) VALUES('dim','3')`,
		`INSERT INTO documents(id,path,path_key,title,content_hash,status,error,updated_at)
			VALUES(7,'/docs/a.md','/docs/a.md','A','h1','ok','','2026-10-26T00:00:00Z')`,
		`INSERT INTO documents(id,path,path_key,title,content_hash,status,error,updated_at)
			VALUES(8,'/docs/b.md','/docs/b.md','B','h2','failed','boom','2026-10-26T00:00:01Z')`,
	}
	if withExtraTable {
		statements = append(statements, `CREATE TABLE tenants(id INTEGER PRIMARY KEY)`)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	vecA := encodeVector([]float32{1, 0, 0})
	vecB := encodeVector([]float32{0, 1, 0})
	if _, err := db.Exec(`INSERT INTO chunks(id,doc_id,seq,heading,text,vector,dim) VALUES(1,7,0,'标题','甲',?,3)`, vecA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chunks(id,doc_id,seq,heading,text,vector,dim) VALUES(2,8,0,'标题','乙',?,3)`, vecB); err != nil {
		t.Fatal(err)
	}
}

func TestOpenFreshCreatesV4(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	meta, err := s.Meta()
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaVersion != "4" {
		t.Fatalf("schema = %q", meta.SchemaVersion)
	}
	tenant, ok, err := s.TenantBySlug(context.Background(), "default")
	if err != nil || !ok || tenant.Name == "" {
		t.Fatalf("default tenant = %+v, ok=%v, err=%v", tenant, ok, err)
	}
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, err=%v", fk, err)
	}
	var jobs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='jobs'`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("jobs table = %d, err=%v", jobs, err)
	}
}

func TestMigrateV2ToV4PreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	createV2DB(t, path, false)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	meta, err := s.Meta()
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaVersion != "4" || meta.Model != "bge-m3" || meta.Dim != 3 {
		t.Fatalf("meta = %+v", meta)
	}
	tenant, ok, err := s.TenantBySlug(context.Background(), "default")
	if err != nil || !ok {
		t.Fatalf("default tenant: ok=%v err=%v", ok, err)
	}
	list, err := s.ListDocuments(context.Background(), tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != 8 || list[1].ID != 7 {
		t.Fatalf("documents = %+v", list)
	}
	hits, err := s.Search(context.Background(), tenant.ID, []float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Text != "甲" {
		t.Fatalf("hits = %+v", hits)
	}
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, err=%v", fk, err)
	}

	if err := s.DeleteDocument(context.Background(), tenant.ID, 8); err != nil {
		t.Fatal(err)
	}
	var chunks int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if chunks != 1 {
		t.Fatalf("删除后级联失败，chunks = %d", chunks)
	}
}

func TestMigrateV2ToV3Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	createV2DB(t, path, false)
	for i := 0; i < 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("第 %d 次 Open: %v", i+1, err)
		}
		if _, err := s.CountUsers(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateV2ToV3RollsBackOnConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	createV2DB(t, path, true)
	if _, err := Open(path); err == nil {
		t.Fatal("存在冲突 tenants 表时应迁移失败")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "2" {
		t.Fatalf("失败后 schema_version = %q", version)
	}
	var docs, chunks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM documents`).Scan(&docs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if docs != 2 || chunks != 2 {
		t.Fatalf("失败回滚后 docs=%d chunks=%d", docs, chunks)
	}
}

func TestMigrateUnknownVersionFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v5.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE meta SET value='5' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("未知版本应报错")
	}
}

func TestMigrateV3ToV4AddsJobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v3.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InitMeta("bge-m3", 500); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDim(2); err != nil {
		t.Fatal(err)
	}
	tenant, ok, err := s.TenantBySlug(context.Background(), "default")
	if err != nil || !ok {
		t.Fatalf("default tenant: ok=%v err=%v", ok, err)
	}
	if err := s.ReplaceDocument(context.Background(), tenant.ID, "/docs/a.md", "A", "h", []ChunkInput{
		{Seq: 0, Text: "内容", Vector: []float32{1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE jobs`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE meta SET value='3' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open v3: %v", err)
	}
	defer s2.Close()
	meta, err := s2.Meta()
	if err != nil || meta.SchemaVersion != "4" {
		t.Fatalf("meta = %+v, err=%v", meta, err)
	}
	var jobs int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='jobs'`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("jobs table = %d, err=%v", jobs, err)
	}
	list, err := s2.ListDocuments(context.Background(), tenant.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("documents = %+v, err=%v", list, err)
	}
}

func TestTenantIsolation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	defaultID := testTenant(t, s)
	tenant, _, err := s.CreateTenantWithAdmin(ctx, "acme", "Acme", "acme-admin", "hash")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ReplaceDocument(ctx, defaultID, "/docs/a.md", "A", "h1", []ChunkInput{
		{Seq: 0, Text: "默认租户内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDocument(ctx, tenant.ID, "/docs/a.md", "A2", "h2", []ChunkInput{
		{Seq: 0, Text: "acme 内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search(ctx, defaultID, []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Text != "默认租户内容" {
		t.Fatalf("默认租户 hits = %+v", hits)
	}
	hits, err = s.Search(ctx, tenant.ID, []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Text != "acme 内容" {
		t.Fatalf("acme hits = %+v", hits)
	}

	stats, err := s.Stats(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 1 || stats.Chunks != 1 || stats.TenantSizeBytes <= 0 {
		t.Fatalf("acme stats = %+v", stats)
	}
	list, err := s.ListDocuments(ctx, defaultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("默认租户文档数 = %d", len(list))
	}
	if _, ok, err := s.GetDocumentByID(ctx, tenant.ID, list[0].ID); err != nil || ok {
		t.Fatalf("跨租户查询应 404: ok=%v err=%v", ok, err)
	}
	if err := s.DeleteDocument(ctx, tenant.ID, list[0].ID); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("跨租户删除错误 = %v", err)
	}
	if err := s.DeleteDocument(ctx, defaultID, list[0].ID); err != nil {
		t.Fatal(err)
	}
	hits, err = s.Search(ctx, tenant.ID, []float32{1, 0, 0}, 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("删除默认租户文档不应影响 acme: hits=%+v err=%v", hits, err)
	}
}

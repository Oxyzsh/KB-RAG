package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.InitMeta("bge-m3", 500); err != nil {
		t.Fatalf("InitMeta: %v", err)
	}
	if err := s.SetDim(3); err != nil {
		t.Fatalf("SetDim: %v", err)
	}
	return s
}

func TestReplaceAndSearch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A", "hash-a", []ChunkInput{
		{Seq: 0, Heading: "休假", Text: "年假规定", Vector: []float32{1, 0, 0}},
		{Seq: 1, Heading: "食堂", Text: "食堂时间", Vector: []float32{0, 1, 0}},
	}); err != nil {
		t.Fatalf("ReplaceDocument a: %v", err)
	}
	if err := s.ReplaceDocument(ctx, "/docs/b.md", "B", "hash-b", []ChunkInput{
		{Seq: 0, Text: "混合内容", Vector: []float32{0.6, 0.8, 0}},
	}); err != nil {
		t.Fatalf("ReplaceDocument b: %v", err)
	}

	hits, err := s.Search(ctx, []float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
	if hits[0].Text != "年假规定" || hits[0].Path != "/docs/a.md" || hits[0].Heading != "休假" {
		t.Errorf("hits[0] = %+v", hits[0])
	}
	if hits[1].Text != "混合内容" {
		t.Errorf("hits[1] = %+v", hits[1])
	}

	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A2", "hash-a2", []ChunkInput{
		{Seq: 0, Heading: "休假", Text: "新年假规定", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatalf("ReplaceDocument a2: %v", err)
	}
	hits, err = s.Search(ctx, []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("替换后 hits = %+v", hits)
	}
	for _, hit := range hits {
		if hit.Text == "食堂时间" {
			t.Fatal("旧片段未被删除")
		}
	}
	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Chunks != 2 {
		t.Errorf("Chunks = %d", stats.Chunks)
	}
}

func TestDocumentLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, ok, err := s.GetDocument("/docs/x.md"); err != nil || ok {
		t.Fatalf("不存在文档查询异常: ok=%v err=%v", ok, err)
	}
	if err := s.ReplaceDocument(ctx, "/docs/x.md", "X", "hash-1", []ChunkInput{
		{Seq: 0, Text: "旧版本内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	info, ok, err := s.GetDocument("/docs/x.md")
	if err != nil || !ok || info.Status != "ok" || info.Hash != "hash-1" {
		t.Fatalf("info = %+v, ok=%v, err=%v", info, ok, err)
	}

	if err := s.MarkFailed(ctx, "/docs/x.md", "X", "hash-2", "embed down"); err != nil {
		t.Fatal(err)
	}
	info, _, err = s.GetDocument("/docs/x.md")
	if err != nil || info.Status != "failed" || info.Hash != "hash-2" || info.Error != "embed down" {
		t.Fatalf("失败状态 = %+v, err=%v", info, err)
	}
	hits, err := s.Search(ctx, []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("失败文档不应参与检索: %+v", hits)
	}
	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Chunks != 1 {
		t.Fatalf("旧片段应保留在库中等待重试: %+v", stats)
	}

	if err := s.ReplaceDocument(ctx, "/docs/x.md", "X", "hash-2", []ChunkInput{
		{Seq: 0, Text: "新版本内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	info, _, _ = s.GetDocument("/docs/x.md")
	if info.Status != "ok" || info.Hash != "hash-2" {
		t.Fatalf("重试成功后状态 = %+v", info)
	}
}

func TestMetaConsistency(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.InitMeta("other-model", 500); err == nil {
		t.Fatal("嵌入模型不一致应报错")
	}
	if err := s.InitMeta("bge-m3", 300); err == nil {
		t.Fatal("chunk_size 不一致应报错")
	}
	if err := s.SetDim(4); err == nil {
		t.Fatal("维度不一致应报错")
	}
	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A", "h", []ChunkInput{
		{Seq: 0, Text: "x", Vector: []float32{1, 0}},
	}); err == nil {
		t.Fatal("片段维度不一致应报错")
	}
	if _, err := s.Search(ctx, []float32{1, 0}, 5); err == nil {
		t.Fatal("查询维度不一致应报错")
	}
}

func TestSchemaVersionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.db")
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
	if _, err := db.Exec(`UPDATE meta SET value='2' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("schema 版本不一致应报错")
	}
}

func TestStats(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A", "h1", []ChunkInput{
		{Seq: 0, Text: "内容一", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDocument(ctx, "/docs/b.md", "B", "h2", []ChunkInput{
		{Seq: 0, Text: "内容二", Vector: []float32{0, 1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkFailed(ctx, "/docs/b.md", "B", "h3", "boom"); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 2 || stats.DocumentsFailed != 1 || stats.Chunks != 2 {
		t.Errorf("stats = %+v", stats)
	}
	if len(stats.FailedPaths) != 1 || stats.FailedPaths[0] != "/docs/b.md" {
		t.Errorf("FailedPaths = %+v", stats.FailedPaths)
	}
	if stats.SizeBytes <= 0 {
		t.Errorf("SizeBytes = %d", stats.SizeBytes)
	}
	if stats.Meta.Model != "bge-m3" || stats.Meta.Dim != 3 || stats.Meta.ChunkSize != 500 {
		t.Errorf("Meta = %+v", stats.Meta)
	}
}

func TestSearchOnEmptyStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Search(context.Background(), []float32{1, 0, 0}, 5); err == nil {
		t.Fatal("空库检索应报错")
	}
}

func TestReplaceDocumentRollsBackOnInsertError(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A", "hash-old", []ChunkInput{
		{Seq: 0, Text: "旧内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_insert BEFORE INSERT ON chunks
		WHEN NEW.text='boom' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A2", "hash-new", []ChunkInput{
		{Seq: 0, Text: "新内容", Vector: []float32{1, 0, 0}},
		{Seq: 1, Text: "boom", Vector: []float32{0, 1, 0}},
	}); err == nil {
		t.Fatal("期望插入失败")
	}
	if _, err := s.db.Exec(`DROP TRIGGER fail_insert`); err != nil {
		t.Fatal(err)
	}
	info, ok, err := s.GetDocument("/docs/a.md")
	if err != nil || !ok || info.Status != "ok" || info.Hash != "hash-old" || info.Title != "A" {
		t.Fatalf("事务未完整回滚: %+v, ok=%v, err=%v", info, ok, err)
	}
	hits, err := s.Search(ctx, []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Text != "旧内容" {
		t.Fatalf("旧片段丢失: %+v", hits)
	}
}

func TestPathKeyCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 路径大小写不敏感")
	}
	s := testStore(t)
	ctx := context.Background()
	if err := s.ReplaceDocument(ctx, "/docs/Case.md", "Case", "h", []ChunkInput{
		{Seq: 0, Text: "内容内容内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	info, ok, err := s.GetDocument("/DOCS/case.md")
	if err != nil || !ok {
		t.Fatalf("大小写不同应命中同一文档: ok=%v err=%v", ok, err)
	}
	if info.Path != "/docs/Case.md" {
		t.Errorf("展示路径被意外改写: %q", info.Path)
	}
	if err := s.ReplaceDocument(ctx, "/DOCS/CASE.MD", "Case2", "h2", []ChunkInput{
		{Seq: 0, Text: "更新后的内容", Vector: []float32{1, 0, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 1 {
		t.Fatalf("同一文件不应重复入库: %+v", stats)
	}
}

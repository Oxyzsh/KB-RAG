package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"kb-rag/internal/store"
)

type fakeEmbedder struct {
	fail bool
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.fail {
		return nil, errors.New("embed down")
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 0, 0}
	}
	return out, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setup(t *testing.T) (*store.Store, string, *fakeEmbedder) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root := t.TempDir()
	return st, root, &fakeEmbedder{}
}

func writeDoc(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunIncremental(t *testing.T) {
	st, root, emb := setup(t)
	a := filepath.Join(root, "docs", "年假.md")
	b := filepath.Join(root, "docs", "食堂.txt")
	writeDoc(t, a, "# 休假\n\n员工满1年可享5天年假。\n")
	writeDoc(t, b, "食堂开放时间为中午十二点。\n")
	opt := Options{Model: "bge-m3", ChunkSize: 500, Overlap: 80, Logger: discardLogger()}
	ctx := context.Background()

	summary, err := Run(ctx, root, st, emb, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Succeeded != 2 || summary.Skipped != 0 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 2 || stats.Chunks == 0 {
		t.Fatalf("stats = %+v", stats)
	}

	summary, err = Run(ctx, root, st, emb, opt)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Succeeded != 0 || summary.Skipped != 2 {
		t.Fatalf("第二次 summary = %+v", summary)
	}

	writeDoc(t, a, "# 休假\n\n员工满1年可享5天年假，满10年享10天。\n")
	summary, err = Run(ctx, root, st, emb, opt)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Succeeded != 1 || summary.Skipped != 1 {
		t.Fatalf("修改后 summary = %+v", summary)
	}
}

func TestRunEmbedFailureRetries(t *testing.T) {
	st, root, emb := setup(t)
	writeDoc(t, filepath.Join(root, "a.md"), "# 标题\n\n一段足够长的正文内容。\n")
	opt := Options{Model: "bge-m3", ChunkSize: 500, Overlap: 80, Logger: discardLogger()}
	ctx := context.Background()

	emb.fail = true
	summary, err := Run(ctx, root, st, emb, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Failed != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	info, ok, err := st.GetDocument(filepath.Join(root, "a.md"))
	if err != nil || !ok || info.Status != "failed" {
		t.Fatalf("info = %+v, ok=%v, err=%v", info, ok, err)
	}

	emb.fail = false
	summary, err = Run(ctx, root, st, emb, opt)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Succeeded != 1 || summary.Failed != 0 {
		t.Fatalf("重试 summary = %+v", summary)
	}
}

func TestRunBadDocumentDoesNotBlockOthers(t *testing.T) {
	st, root, emb := setup(t)
	writeDoc(t, filepath.Join(root, "good.md"), "# 标题\n\n一段足够长的正文内容。\n")
	writeDoc(t, filepath.Join(root, "bad.docx"), "not a docx")
	opt := Options{Model: "bge-m3", ChunkSize: 500, Overlap: 80, Logger: discardLogger()}

	summary, err := Run(context.Background(), root, st, emb, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Succeeded != 1 || summary.Failed != 1 || len(summary.FailedPaths) != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestIngestFile(t *testing.T) {
	st, root, emb := setup(t)
	if err := st.InitMeta("bge-m3", 500); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "# 标题\n\n一段足够长的正文内容。\n")
	opt := Options{Model: "bge-m3", ChunkSize: 500, Overlap: 80, Logger: discardLogger()}
	ctx := context.Background()

	res := IngestFile(ctx, path, st, emb, opt)
	if res.Outcome != OutcomeOK || res.Chunks == 0 || res.Err != nil {
		t.Fatalf("首次 IngestFile = %+v", res)
	}
	res = IngestFile(ctx, path, st, emb, opt)
	if res.Outcome != OutcomeSkipped {
		t.Fatalf("第二次 IngestFile = %+v", res)
	}

	writeDoc(t, path, "# 标题\n\n修改后的正文内容，足够长。\n")
	emb.fail = true
	res = IngestFile(ctx, path, st, emb, opt)
	if res.Outcome != OutcomeFailed || res.Err == nil {
		t.Fatalf("失败 IngestFile = %+v", res)
	}
	info, ok, err := st.GetDocument(path)
	if err != nil || !ok || info.Status != "failed" {
		t.Fatalf("失败状态 = %+v, ok=%v, err=%v", info, ok, err)
	}
}

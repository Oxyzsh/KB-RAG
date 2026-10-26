package query

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"kb-rag/internal/llm"
	"kb-rag/internal/store"
)

type fakeEmbedder struct {
	vector []float32
	err    error
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return [][]float32{f.vector}, nil
}

type fakeStore struct {
	meta    store.EmbeddingMeta
	hits    []store.Hit
	err     error
	gotTopK int
}

func (f *fakeStore) Meta() (store.EmbeddingMeta, error) { return f.meta, nil }
func (f *fakeStore) Search(ctx context.Context, query []float32, topK int) ([]store.Hit, error) {
	f.gotTopK = topK
	return f.hits, f.err
}

type fakeLLM struct {
	answer   string
	err      error
	messages []llm.Message
}

func (f *fakeLLM) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	f.messages = messages
	return f.answer, f.err
}

func TestAskBuildsPromptAndReturnsAnswer(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "docs", "人事制度.md")
	llmFake := &fakeLLM{answer: "年假为5天[1]。"}
	svc := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{Model: "bge-m3", Dim: 3}, hits: []store.Hit{{Path: path, Heading: "人事制度 > 休假", Text: "满1年享5天年假。"}}},
		LLM:        llmFake,
		EmbedModel: "bge-m3",
		TopK:       8,
		BaseDir:    baseDir,
	}
	result, err := svc.Ask(context.Background(), "年假怎么请")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !result.Generated || result.Answer != "年假为5天[1]。" || len(result.Hits) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(llmFake.messages) != 2 || llmFake.messages[0].Role != "system" {
		t.Fatalf("messages = %+v", llmFake.messages)
	}
	user := llmFake.messages[1].Content
	for _, want := range []string{"资料：", "[1] 来源：docs/人事制度.md", "标题：人事制度 > 休假", "内容：满1年享5天年假。", "问题：年假怎么请"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt 缺少 %q:\n%s", want, user)
		}
	}
}

func TestAskModelMismatch(t *testing.T) {
	svc := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{Model: "other", Dim: 3}},
		LLM:        &fakeLLM{answer: "x"},
		EmbedModel: "bge-m3",
		TopK:       8,
	}
	if _, err := svc.Ask(context.Background(), "q"); err == nil {
		t.Fatal("期望模型不一致报错")
	}
}

func TestAskDimensionMismatch(t *testing.T) {
	svc := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{Model: "bge-m3", Dim: 3}},
		LLM:        &fakeLLM{answer: "x"},
		EmbedModel: "bge-m3",
		TopK:       8,
	}
	if _, err := svc.Ask(context.Background(), "q"); err == nil {
		t.Fatal("期望维度不一致报错")
	}
}

func TestAskEmptyStore(t *testing.T) {
	svc := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{}},
		LLM:        &fakeLLM{answer: "x"},
		EmbedModel: "bge-m3",
		TopK:       8,
	}
	if _, err := svc.Ask(context.Background(), "q"); err == nil {
		t.Fatal("期望空库报错")
	}
}

func TestAskLLMFailureReturnsHits(t *testing.T) {
	hits := []store.Hit{{Path: "/docs/a.md", Text: "内容"}}
	svc := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{Model: "bge-m3", Dim: 3}, hits: hits},
		LLM:        &fakeLLM{err: errors.New("down")},
		EmbedModel: "bge-m3",
		TopK:       8,
	}
	result, err := svc.Ask(context.Background(), "q")
	if err == nil {
		t.Fatal("期望生成失败")
	}
	if len(result.Hits) != 1 || result.Generated {
		t.Fatalf("result = %+v", result)
	}
}

func TestFormatResult(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "base")
	path := filepath.Join(baseDir, "docs", "a.md")
	result := Result{
		Answer: "答案[1]。",
		Hits: []store.Hit{
			{Path: path, Heading: "人事制度 > 休假"},
			{Path: path, Heading: ""},
		},
	}
	out := FormatResult(result, baseDir)
	for _, want := range []string{"答案[1]。", "引用来源：", "[1] docs/a.md > 人事制度 > 休假", "[2] docs/a.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q:\n%s", want, out)
		}
	}
	hitsOut := FormatHits(result.Hits, baseDir)
	if !strings.Contains(hitsOut, "检索片段：") || !strings.Contains(hitsOut, "docs/a.md") {
		t.Errorf("FormatHits = %q", hitsOut)
	}
}

func TestAskWithTopK(t *testing.T) {
	st := &fakeStore{meta: store.EmbeddingMeta{Model: "bge-m3", Dim: 3}, hits: []store.Hit{{Path: "/docs/a.md", Text: "内容"}}}
	svc := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      st,
		LLM:        &fakeLLM{answer: "答案"},
		EmbedModel: "bge-m3",
		TopK:       8,
	}
	if _, err := svc.AskWithTopK(context.Background(), "q", 3); err != nil {
		t.Fatalf("AskWithTopK: %v", err)
	}
	if st.gotTopK != 3 {
		t.Fatalf("topK = %d, want 3", st.gotTopK)
	}
}

func TestAskSentinelErrors(t *testing.T) {
	empty := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{}},
		LLM:        &fakeLLM{answer: "x"},
		EmbedModel: "bge-m3",
	}
	if _, err := empty.Ask(context.Background(), "q"); !errors.Is(err, ErrEmptyStore) {
		t.Fatalf("空库错误 = %v", err)
	}

	mismatch := &Service{
		Embedder:   &fakeEmbedder{vector: []float32{1, 0, 0}},
		Store:      &fakeStore{meta: store.EmbeddingMeta{Model: "other", Dim: 3}},
		LLM:        &fakeLLM{answer: "x"},
		EmbedModel: "bge-m3",
	}
	if _, err := mismatch.Ask(context.Background(), "q"); !errors.Is(err, ErrModelMismatch) {
		t.Fatalf("模型不一致错误 = %v", err)
	}
}

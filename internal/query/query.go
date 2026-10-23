package query

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"kb-rag/internal/llm"
	"kb-rag/internal/store"
)

const systemPrompt = `你是企业知识库问答助手。只能依据用户提供的“资料”回答，禁止使用资料之外的信息。
如果资料中没有能回答问题的依据，只回答：知识库中未找到相关依据。
每个结论后用 [n] 标注对应的资料编号。用中文回答，简洁准确。`

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Store interface {
	Meta() (store.EmbeddingMeta, error)
	Search(ctx context.Context, query []float32, topK int) ([]store.Hit, error)
}

type LLM interface {
	Chat(ctx context.Context, messages []llm.Message) (string, error)
}

type Service struct {
	Embedder   Embedder
	Store      Store
	LLM        LLM
	EmbedModel string
	TopK       int
	BaseDir    string
	Logger     *slog.Logger
}

type Result struct {
	Answer    string
	Hits      []store.Hit
	Generated bool
}

func (s *Service) Ask(ctx context.Context, question string) (Result, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return Result{}, fmt.Errorf("问题不能为空")
	}
	start := time.Now()
	meta, err := s.Store.Meta()
	if err != nil {
		return Result{}, fmt.Errorf("读取库元信息: %w", err)
	}
	if meta.Model == "" || meta.Dim <= 0 {
		return Result{}, fmt.Errorf("知识库为空，请先执行 ingest")
	}
	if s.EmbedModel != meta.Model {
		return Result{}, fmt.Errorf("嵌入模型不一致：库内为 %q，配置为 %q", meta.Model, s.EmbedModel)
	}
	vectors, err := s.Embedder.Embed(ctx, []string{question})
	if err != nil {
		return Result{}, fmt.Errorf("问题嵌入失败: %w", err)
	}
	if len(vectors) != 1 {
		return Result{}, fmt.Errorf("问题嵌入返回 %d 个向量，期望 1 个", len(vectors))
	}
	if len(vectors[0]) != meta.Dim {
		return Result{}, fmt.Errorf("问题嵌入维度为 %d，库内为 %d", len(vectors[0]), meta.Dim)
	}
	topK := s.TopK
	if topK <= 0 {
		topK = 8
	}
	hits, err := s.Store.Search(ctx, vectors[0], topK)
	if err != nil {
		return Result{}, fmt.Errorf("检索失败: %w", err)
	}
	if len(hits) == 0 {
		return Result{}, fmt.Errorf("知识库中未找到相关片段，请先执行 ingest")
	}
	if s.Logger != nil {
		ids := make([]int64, len(hits))
		scores := make([]float64, len(hits))
		for i, hit := range hits {
			ids[i] = hit.ChunkID
			scores[i] = hit.Score
		}
		s.Logger.Info("检索完成", "question_len", utf8.RuneCountInString(question), "top_k", len(hits), "chunk_ids", ids, "scores", scores, "duration_ms", time.Since(start).Milliseconds())
	}
	answer, err := s.LLM.Chat(ctx, []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: s.buildUserPrompt(question, hits)},
	})
	if err != nil {
		return Result{Hits: hits}, fmt.Errorf("生成失败: %w", err)
	}
	return Result{Answer: answer, Hits: hits, Generated: true}, nil
}

func (s *Service) buildUserPrompt(question string, hits []store.Hit) string {
	var b strings.Builder
	b.WriteString("资料：\n")
	for i, hit := range hits {
		fmt.Fprintf(&b, "[%d] 来源：%s\n", i+1, DisplayPath(hit.Path, s.BaseDir))
		heading := strings.TrimSpace(hit.Heading)
		if heading == "" {
			heading = "（无标题）"
		}
		fmt.Fprintf(&b, "    标题：%s\n", heading)
		fmt.Fprintf(&b, "    内容：%s\n\n", strings.TrimSpace(hit.Text))
	}
	fmt.Fprintf(&b, "问题：%s", question)
	return b.String()
}

func FormatResult(result Result, baseDir string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(result.Answer))
	b.WriteString("\n\n引用来源：\n")
	for i, hit := range result.Hits {
		fmt.Fprintf(&b, "[%d] %s", i+1, DisplayPath(hit.Path, baseDir))
		if heading := strings.TrimSpace(hit.Heading); heading != "" {
			b.WriteString(" > " + heading)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatHits(hits []store.Hit, baseDir string) string {
	var b strings.Builder
	b.WriteString("检索片段：\n")
	for i, hit := range hits {
		fmt.Fprintf(&b, "[%d] %s", i+1, DisplayPath(hit.Path, baseDir))
		if heading := strings.TrimSpace(hit.Heading); heading != "" {
			b.WriteString(" > " + heading)
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(hit.Text))
	}
	return strings.TrimRight(b.String(), "\n")
}

func DisplayPath(path, baseDir string) string {
	if baseDir == "" {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

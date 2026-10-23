package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Ollama struct {
	BaseURL    string
	Model      string
	BatchSize  int
	Client     *http.Client
	retryDelay time.Duration
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func New(baseURL, model string, batchSize int) *Ollama {
	return &Ollama{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Model:      model,
		BatchSize:  batchSize,
		Client:     &http.Client{Timeout: 120 * time.Second},
		retryDelay: 500 * time.Millisecond,
	}
}

func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	batchSize := o.BatchSize
	if batchSize <= 0 {
		batchSize = 16
	}
	var out [][]float32
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vectors, err := o.embedBatchWithRetry(ctx, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("嵌入第 %d-%d 条失败: %w", start+1, end, err)
		}
		out = append(out, vectors...)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		return nil, errors.New("嵌入服务返回空向量")
	}
	dim := len(out[0])
	for i, vector := range out {
		if len(vector) != dim {
			return nil, fmt.Errorf("嵌入维度不一致：第 %d 条为 %d，期望 %d", i+1, len(vector), dim)
		}
	}
	return out, nil
}

func (o *Ollama) embedBatchWithRetry(ctx context.Context, batch []string) ([][]float32, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		vectors, err := o.embedBatch(ctx, batch)
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		if attempt == 3 {
			break
		}
		delay := o.retryDelay * time.Duration(1<<(attempt-1))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func (o *Ollama) embedBatch(ctx context.Context, batch []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Model: o.Model, Input: batch})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("Ollama 返回 %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	var decoded embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("解析 Ollama 响应: %w", err)
	}
	if len(decoded.Embeddings) != len(batch) {
		return nil, fmt.Errorf("Ollama 返回 %d 个向量，期望 %d 个", len(decoded.Embeddings), len(batch))
	}
	return decoded.Embeddings, nil
}

package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbedBatches(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&requests, 1)
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		resp := embedResponse{}
		for range req.Input {
			resp.Embeddings = append(resp.Embeddings, []float32{1, 2, 3})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	o := New(server.URL, "bge-m3", 2)
	vectors, err := o.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != 3 {
		t.Fatalf("vectors = %d", len(vectors))
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("请求次数 = %d, want 2", got)
	}
}

func TestEmbedRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: [][]float32{{1, 0}}})
	}))
	defer server.Close()

	o := New(server.URL, "m", 1)
	o.retryDelay = time.Millisecond
	if _, err := o.Embed(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("尝试次数 = %d, want 3", got)
	}
}

func TestEmbedRejectsCountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: [][]float32{{1, 0}}})
	}))
	defer server.Close()

	o := New(server.URL, "m", 2)
	o.retryDelay = time.Millisecond
	if _, err := o.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("期望向量数量不一致报错")
	}
}

func TestEmbedRejectsDimensionMismatch(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		vec := []float32{1, 2, 3}
		if n == 2 {
			vec = []float32{1, 2, 3, 4}
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: [][]float32{vec}})
	}))
	defer server.Close()

	o := New(server.URL, "m", 1)
	o.retryDelay = time.Millisecond
	if _, err := o.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("期望维度不一致报错")
	}
}

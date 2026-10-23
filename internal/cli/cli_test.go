package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"kb-rag/internal/cli"
)

type fakeEmbedServer struct{}

func (fakeEmbedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	vectors := make([][]float32, 0, len(req.Input))
	for _, text := range req.Input {
		if strings.Contains(text, "年假") || strings.Contains(text, "年休") {
			vectors = append(vectors, []float32{1, 0, 0, 0})
		} else {
			vectors = append(vectors, []float32{0, 1, 0, 0})
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
}

type fakeLLMServer struct {
	fail atomic.Bool
	mu   sync.Mutex
	last string
}

func (f *fakeLLMServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.fail.Load() {
		http.Error(w, "llm down", http.StatusInternalServerError)
		return
	}
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.Messages) > 0 {
		f.mu.Lock()
		f.last = req.Messages[len(req.Messages)-1].Content
		f.mu.Unlock()
	}
	_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"员工满1年享5天年假[1]。"}}]}`))
}

func (f *fakeLLMServer) lastPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEndToEnd(t *testing.T) {
	t.Setenv("KB_RAG_LLM_API_KEY", "test-key")

	embedServer := httptest.NewServer(fakeEmbedServer{})
	defer embedServer.Close()
	llmHandler := &fakeLLMServer{}
	llmServer := httptest.NewServer(llmHandler)
	defer llmServer.Close()

	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	writeFile(t, filepath.Join(docs, "年假.md"), "# 休假\n\n员工累计工作满1年不满10年的，年休假5天。\n")
	writeFile(t, filepath.Join(docs, "食堂.txt"), "公司食堂开放时间为中午十二点。\n")

	cfgPath := filepath.Join(dir, "config.yaml")
	cfgBody := fmt.Sprintf(`embedding:
  base_url: %s
  model: test-embed
  batch_size: 2
llm:
  base_url: %s
  model: test-llm
chunk:
  size: 100
  overlap: 20
store:
  path: %s
log:
  file: %s
`, embedServer.URL, llmServer.URL,
		filepath.ToSlash(filepath.Join(dir, "data", "kb.db")),
		filepath.ToSlash(filepath.Join(dir, "logs", "app.log")))
	writeFile(t, cfgPath, cfgBody)

	run := func(args ...string) (int, string, string) {
		var stdout, stderr strings.Builder
		code := cli.Run(context.Background(), args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	code, out, errOut := run("ingest", "--config", cfgPath, docs)
	if code != 0 {
		t.Fatalf("ingest code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.Contains(out, "成功 2") {
		t.Fatalf("ingest 输出 = %q", out)
	}

	code, out, errOut = run("ingest", "--config", cfgPath, docs)
	if code != 0 || !strings.Contains(out, "跳过 2") {
		t.Fatalf("重复 ingest code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	code, out, errOut = run("ask", "--config", cfgPath, "--top-k", "2", "年假怎么请")
	if code != 0 {
		t.Fatalf("ask code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	for _, want := range []string{"员工满1年享5天年假[1]。", "引用来源：", "年假.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("ask 输出缺少 %q:\n%s", want, out)
		}
	}
	prompt := llmHandler.lastPrompt()
	for _, want := range []string{"资料：", "[1] 来源：", "年假怎么请"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt 缺少 %q:\n%s", want, prompt)
		}
	}
	if strings.Index(prompt, "年假.md") > strings.Index(prompt, "食堂.txt") {
		t.Errorf("年假文档应排在检索上下文前面:\n%s", prompt)
	}

	code, out, errOut = run("stats", "--config", cfgPath)
	if code != 0 || !strings.Contains(out, "文档：2（失败 0）") || !strings.Contains(out, "片段：") {
		t.Fatalf("stats code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	llmHandler.fail.Store(true)
	code, out, errOut = run("ask", "--config", cfgPath, "年假怎么请")
	if code != 1 {
		t.Fatalf("生成失败 ask code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.Contains(out, "检索片段：") {
		t.Errorf("生成失败时应输出检索片段:\n%s", out)
	}
}

func TestAskRequiresAPIKey(t *testing.T) {
	t.Setenv("KB_RAG_LLM_API_KEY", "")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	writeFile(t, cfgPath, fmt.Sprintf(`llm:
  base_url: http://127.0.0.1:1
  model: test
store:
  path: %s
log:
  file: %s
`, filepath.ToSlash(filepath.Join(dir, "kb.db")), filepath.ToSlash(filepath.Join(dir, "app.log"))))

	var stdout, stderr strings.Builder
	code := cli.Run(context.Background(), []string{"ask", "--config", cfgPath, "问题"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "KB_RAG_LLM_API_KEY") {
		t.Errorf("错误信息未指出缺少 API Key: %q", stderr.String())
	}
}

func TestUsage(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := cli.Run(context.Background(), nil, &stdout, &stderr); code != 2 {
		t.Fatalf("无参数 code = %d", code)
	}
	if code := cli.Run(context.Background(), []string{"unknown"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("未知命令 code = %d", code)
	}
}

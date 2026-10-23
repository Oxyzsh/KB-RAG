# KB-RAG 第一期 CLI 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现一个可本地运行的 KB-RAG CLI：`ingest` 把 Markdown/TXT/DOCX/PDF 解析、切分、经 Ollama 嵌入后存入 SQLite；`ask` 检索 top-k 并调用 OpenAI 兼容接口生成带引用的中文答案；`stats` 展示知识库状态。

**Architecture:** 单进程 CLI，按设计文档的管道式分层：loader → chunker → embedder → store → query → llm。每个组件通过窄接口连接，存储层用 SQLite + 内存点积，第一期不做任何服务化。

**Tech Stack:** Go 1.26.5、`modernc.org/sqlite`（无 CGO）、`goldmark`、`yaml.v3`、`lumberjack.v2`、`golang.org/x/text`、`github.com/ledongthuc/pdf`。

## Global Constraints

- Go 版本：1.26.5。
- 第一期只做 CLI：`ingest`、`ask`、`stats`；不做 Web、权限、多用户、IM、文件监听、prune、rerank、混合检索、query rewrite、多轮对话、流式输出、OCR、Docker、自动化评测。
- 支持格式：`.md`、`.markdown`、`.txt`、`.docx`、文本型 `.pdf`，扩展名不区分大小写。
- 嵌入：Ollama `/api/embed`，默认 `bge-m3`，默认批量 16，总失败前重试 3 次（指数退避）；单批超时 120 秒。
- 生成：OpenAI 兼容 `/chat/completions`；`temperature=0.2`、`max_tokens=1024`；请求超时 90 秒。
- 检索：默认 top-k 8；向量归一化后 float32 小端 BLOB 存储，点积计算；不做相似度阈值。
- 切分：默认 500 字符、重叠 80 字符；不跨标题；碎片（<10 字符）丢弃。
- API Key 只从环境变量 `KB_RAG_LLM_API_KEY` 读取，不写入配置文件，不打印，不写日志。
- 日志同时写 stderr 与文件；按 10MB 轮转，保留 5 份；**文档正文与问题原文不落盘**，只记录长度、耗时、片段 ID 与分数。
- 退出码：配置/参数错误 2；部分文件失败、服务不可达、库为空、生成失败 1；成功 0；Ctrl+C 130。
- 存储一致性：`meta` 记录 `schema_version`、`embedding_model`、`dim`、`chunk_size`；不一致时直接报错，要求删除库重建，禁止新旧向量混存。
- 所有源文件 UTF-8、LF 换行；路径输出相对当前工作目录，统一使用 `/`。
- 当前机器未安装 Ollama，真实嵌入冒烟测试在实现完成后单独执行；自动化测试全部使用假 HTTP 服务。

---

## 文件结构

```text
cmd/KB-RAG/main.go                 CLI 入口，只负责信号与退出码
internal/cli/cli.go                子命令解析、依赖装配、用户输出
internal/cli/cli_test.go           全链路集成测试（假 Ollama + 假 LLM）
internal/config/config.go          YAML 配置、默认值、环境变量、按命令校验
internal/config/config_test.go
internal/logging/logging.go        slog + 文件轮转 + stderr
internal/logging/logging_test.go
internal/ingest/loader/loader.go   文档模型、格式分发
internal/ingest/loader/text.go     TXT / Markdown 解析（UTF-8 回退 GB18030）
internal/ingest/loader/docx.go     DOCX 解析（zip + encoding/xml）
internal/ingest/loader/pdf.go      PDF 解析（ledongthuc/pdf）
internal/ingest/loader/*_test.go
internal/ingest/loader/testdata/sample_zh.pdf
internal/ingest/chunker/chunker.go 结构感知切分
internal/ingest/chunker/chunker_test.go
internal/ingest/embedder/ollama.go Ollama 嵌入客户端（批量 + 重试）
internal/ingest/embedder/ollama_test.go
internal/ingest/ingest.go          扫描目录、增量判断、入库事务
internal/ingest/ingest_test.go
internal/store/store.go            SQLite 架构、元信息、文档/片段、检索、统计
internal/store/store_test.go
internal/llm/client.go             OpenAI 兼容 chat/completions 客户端
internal/llm/client_test.go
internal/query/query.go            检索 → 上下文 → 生成 → 引用格式化
internal/query/query_test.go
config.example.yaml
README.md
docs/eval/questions.md
```

## 关键接口约定

```go
// loader
type Block struct { Level int; Text string }
type Document struct { Path, Title, Format string; Blocks []Block }
func Load(path string) (Document, error)
func Supported(path string) bool

// chunker
type Chunk struct { Seq int; Heading, Text string }
func Split(doc loader.Document, size, overlap int) []Chunk

// embedder
type Embedder interface { Embed(ctx context.Context, texts []string) ([][]float32, error) }

// store
func Open(path string) (*Store, error)
func (s *Store) InitMeta(model string, chunkSize int) error
func (s *Store) Meta() (EmbeddingMeta, error)
func (s *Store) SetDim(dim int) error
func (s *Store) GetDocument(path string) (DocumentInfo, bool, error)
func (s *Store) ReplaceDocument(ctx context.Context, path, title, hash string, chunks []ChunkInput) error
func (s *Store) MarkFailed(ctx context.Context, path, title, hash, msg string) error
func (s *Store) Search(ctx context.Context, query []float32, topK int) ([]Hit, error)
func (s *Store) Stats(ctx context.Context) (Stats, error)

// llm
type Message struct { Role, Content string }
func New(baseURL, model, apiKey string) *Client
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error)

// query
func (s *Service) Ask(ctx context.Context, question string) (Result, error)
func FormatResult(r Result, baseDir string) string
func FormatHits(hits []store.Hit, baseDir string) string
func DisplayPath(path, baseDir string) string
```

---

### Task 1: 项目骨架与配置模块

**Files:**
- Create: `go.mod`, `.gitignore`, `.gitattributes`, `config.example.yaml`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `config.Config`、`config.Load(path) (*Config, error)`、`(*Config).Validate(command string) error`、`Config.APIKey`

- [ ] **Step 1: 初始化仓库与模块**

```bash
cd /c/workspace/go/src/KB-RAG
git init
go mod init kb-rag
go get gopkg.in/yaml.v3@v3.0.1
```

Expected: `go.mod` 出现 `module kb-rag` 和 `go 1.26.5`。

- [ ] **Step 2: 写 `.gitignore` 与 `.gitattributes`**

`.gitignore`：

```gitignore
/data/
/config.yaml
*.exe
coverage.out
```

`.gitattributes`：

```gitattributes
* text=auto eol=lf
```

- [ ] **Step 3: 写失败测试 `internal/config/config_test.go`**

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaultsAndEnv(t *testing.T) {
	t.Setenv("KB_RAG_LLM_API_KEY", "secret")
	path := writeConfig(t, "llm:\n  base_url: http://localhost:9999\n  model: test-llm\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Embedding.BaseURL != "http://localhost:11434" {
		t.Errorf("embedding.base_url = %q", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.Model != "bge-m3" {
		t.Errorf("embedding.model = %q", cfg.Embedding.Model)
	}
	if cfg.Embedding.BatchSize != 16 {
		t.Errorf("embedding.batch_size = %d", cfg.Embedding.BatchSize)
	}
	if cfg.Chunk.Size != 500 || cfg.Chunk.Overlap != 80 {
		t.Errorf("chunk = %+v", cfg.Chunk)
	}
	if cfg.Store.Path != "./data/KB-RAG.db" {
		t.Errorf("store.path = %q", cfg.Store.Path)
	}
	if cfg.Log.File != "./data/logs/KB-RAG.log" {
		t.Errorf("log.file = %q", cfg.Log.File)
	}
	if cfg.LLM.BaseURL != "http://localhost:9999" || cfg.LLM.Model != "test-llm" {
		t.Errorf("llm = %+v", cfg.LLM)
	}
	if cfg.APIKey != "secret" {
		t.Errorf("APIKey 未从环境变量读取")
	}
}

func validConfig(dir string) *Config {
	return &Config{
		Embedding: EmbeddingConfig{BaseURL: "http://localhost:11434", Model: "bge-m3", BatchSize: 16},
		LLM:       LLMConfig{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat"},
		Chunk:     ChunkConfig{Size: 500, Overlap: 80},
		Store:     StoreConfig{Path: filepath.Join(dir, "data", "KB-RAG.db")},
		Log:       LogConfig{File: filepath.Join(dir, "logs", "KB-RAG.log")},
		APIKey:    "key",
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		cmd    string
		mutate func(*Config)
	}{
		{"bad embedding url", "ingest", func(c *Config) { c.Embedding.BaseURL = "localhost:11434" }},
		{"empty embedding model", "ingest", func(c *Config) { c.Embedding.Model = "" }},
		{"bad batch size", "ingest", func(c *Config) { c.Embedding.BatchSize = 0 }},
		{"overlap equals size", "ingest", func(c *Config) { c.Chunk.Overlap = 500 }},
		{"empty store path", "ingest", func(c *Config) { c.Store.Path = "" }},
		{"empty log file", "ingest", func(c *Config) { c.Log.File = "" }},
		{"ask bad llm url", "ask", func(c *Config) { c.LLM.BaseURL = "ftp://example.com" }},
		{"ask empty llm model", "ask", func(c *Config) { c.LLM.Model = "" }},
		{"ask empty key", "ask", func(c *Config) { c.APIKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(t.TempDir())
			tc.mutate(cfg)
			if err := cfg.Validate(tc.cmd); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

func TestValidateOK(t *testing.T) {
	cfg := validConfig(t.TempDir())
	if err := cfg.Validate("ask"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(cfg.Embedding.BaseURL, "http") {
		t.Fatal("测试前置数据异常")
	}
}
```

- [ ] **Step 4: 运行测试确认失败**

```bash
go test ./internal/config -run TestLoadDefaultsAndEnv -v
```

Expected: FAIL（`undefined: Load`）。

- [ ] **Step 5: 实现 `internal/config/config.go`**

```go
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Embedding EmbeddingConfig `yaml:"embedding"`
	LLM       LLMConfig       `yaml:"llm"`
	Chunk     ChunkConfig     `yaml:"chunk"`
	Store     StoreConfig     `yaml:"store"`
	Log       LogConfig       `yaml:"log"`
	APIKey    string          `yaml:"-"`
}

type EmbeddingConfig struct {
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	BatchSize int    `yaml:"batch_size"`
}

type LLMConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type ChunkConfig struct {
	Size    int `yaml:"size"`
	Overlap int `yaml:"overlap"`
}

type StoreConfig struct {
	Path string `yaml:"path"`
}

type LogConfig struct {
	File string `yaml:"file"`
}

func defaults() Config {
	return Config{
		Embedding: EmbeddingConfig{BaseURL: "http://localhost:11434", Model: "bge-m3", BatchSize: 16},
		Chunk:     ChunkConfig{Size: 500, Overlap: 80},
		Store:     StoreConfig{Path: "./data/KB-RAG.db"},
		Log:       LogConfig{File: "./data/logs/KB-RAG.log"},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}
	cfg.APIKey = os.Getenv("KB_RAG_LLM_API_KEY")
	return &cfg, nil
}

func (c *Config) Validate(command string) error {
	if err := validateURL("embedding.base_url", c.Embedding.BaseURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Embedding.Model) == "" {
		return fmt.Errorf("embedding.model 不能为空")
	}
	if c.Embedding.BatchSize <= 0 {
		return fmt.Errorf("embedding.batch_size 必须大于 0")
	}
	if c.Chunk.Size <= 0 || c.Chunk.Overlap <= 0 || c.Chunk.Size <= c.Chunk.Overlap {
		return fmt.Errorf("chunk 配置必须满足 size > overlap > 0（当前 size=%d, overlap=%d）", c.Chunk.Size, c.Chunk.Overlap)
	}
	if strings.TrimSpace(c.Store.Path) == "" {
		return fmt.Errorf("store.path 不能为空")
	}
	if err := ensureDir(filepath.Dir(c.Store.Path)); err != nil {
		return err
	}
	if strings.TrimSpace(c.Log.File) == "" {
		return fmt.Errorf("log.file 不能为空")
	}
	if err := ensureDir(filepath.Dir(c.Log.File)); err != nil {
		return err
	}
	if command == "ask" {
		if err := validateURL("llm.base_url", c.LLM.BaseURL); err != nil {
			return err
		}
		if strings.TrimSpace(c.LLM.Model) == "" {
			return fmt.Errorf("llm.model 不能为空")
		}
		if c.APIKey == "" {
			return fmt.Errorf("环境变量 KB_RAG_LLM_API_KEY 不能为空")
		}
	}
	return nil
}

func validateURL(field, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s 必须是合法的 http(s) URL（当前为 %q）", field, raw)
	}
	return nil
}

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}
	return nil
}
```

- [ ] **Step 6: 运行测试确认通过**

```bash
go test ./internal/config -v
```

Expected: PASS（3 个测试函数全部通过）。

- [ ] **Step 7: 写 `config.example.yaml`**

```yaml
embedding:
  base_url: http://localhost:11434
  model: bge-m3
  batch_size: 16

llm:
  base_url: https://api.deepseek.com/v1
  model: deepseek-chat

chunk:
  size: 500        # 字符数
  overlap: 80

store:
  path: ./data/KB-RAG.db

log:
  file: ./data/logs/KB-RAG.log
```

- [ ] **Step 8: 提交**

```bash
gofmt -w internal/config
go vet ./...
git add go.mod go.sum .gitignore .gitattributes config.example.yaml internal/config
git commit -m "chore: 初始化 Go 模块与配置加载"
```

---

### Task 2: 日志模块

**Files:**
- Create: `internal/logging/logging.go`
- Test: `internal/logging/logging_test.go`

**Interfaces:**
- Consumes: `config.LogConfig`
- Produces: `logging.New(cfg config.LogConfig, command string, stderr io.Writer) (*slog.Logger, io.Closer, error)`

- [ ] **Step 1: 添加依赖**

```bash
go get gopkg.in/natefinch/lumberjack.v2@v2.2.1
```

- [ ] **Step 2: 写失败测试 `internal/logging/logging_test.go`**

```go
package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kb-rag/internal/config"
)

func TestNewWritesToStderrAndFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "logs", "app.log")
	var stderr bytes.Buffer
	logger, closer, err := New(config.LogConfig{File: logFile}, "ingest", &stderr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("文档入库", "path", "docs/a.md")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strings.Contains(stderr.String(), "文档入库") {
		t.Errorf("stderr 缺少日志: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "command=ingest") {
		t.Errorf("stderr 缺少 command 字段: %q", stderr.String())
	}
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取日志: %v", err)
	}
	if !strings.Contains(string(raw), "docs/a.md") {
		t.Errorf("文件日志缺少字段: %q", string(raw))
	}
	if !strings.Contains(string(raw), "command=ingest") {
		t.Errorf("文件日志缺少 command: %q", string(raw))
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./internal/logging -v
```

Expected: FAIL（`undefined: New`）。

- [ ] **Step 4: 实现 `internal/logging/logging.go`**

```go
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"kb-rag/internal/config"
)

func New(cfg config.LogConfig, command string, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil {
		return nil, nil, err
	}
	rotator := &lumberjack.Logger{
		Filename:   cfg.File,
		MaxSize:    10,
		MaxBackups: 5,
		Compress:   false,
	}
	handler := slog.NewTextHandler(io.MultiWriter(stderr, rotator), &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler).With("command", command), rotator, nil
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/logging -v
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/logging
go vet ./...
git add internal/logging
git commit -m "feat: 增加日志落盘与轮转"
```

---

### Task 3: 文档模型与 TXT/Markdown 解析

**Files:**
- Create: `internal/ingest/loader/loader.go`
- Create: `internal/ingest/loader/text.go`
- Test: `internal/ingest/loader/text_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `loader.Block{Level int; Text string}`、`loader.Document{Path, Title, Format string; Blocks []Block}`、`loader.Load(path) (Document, error)`、`loader.Supported(path) bool`、`loader.fallbackTitle(path) string`、`loader.firstTitle(blocks, path) string`、`loader.splitPlainText(s string) []string`

- [ ] **Step 1: 添加依赖**

```bash
go get github.com/yuin/goldmark golang.org/x/text
```

- [ ] **Step 2: 写失败测试 `internal/ingest/loader/text_test.go`**

```go
package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSupported(t *testing.T) {
	cases := map[string]bool{
		"a.md": true, "a.MARKDOWN": true, "a.txt": true, "a.docx": true,
		"a.PDF": true, "a.doc": false, "a.png": false,
	}
	for name, want := range cases {
		if got := Supported(name); got != want {
			t.Errorf("Supported(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestLoadMarkdown(t *testing.T) {
	body := strings.Join([]string{
		"# 人事制度",
		"",
		"## 休假",
		"",
		"年假规定：员工累计工作满1年不满10年的，年休假5天。",
		"",
		"## 其他",
		"",
		"食堂中午开放。",
	}, "\n")
	path := writeFile(t, "制度.md", []byte(body))

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "md" {
		t.Errorf("Format = %q", doc.Format)
	}
	if doc.Title != "人事制度" {
		t.Errorf("Title = %q", doc.Title)
	}
	want := []Block{
		{Level: 1, Text: "人事制度"},
		{Level: 2, Text: "休假"},
		{Text: "年假规定：员工累计工作满1年不满10年的，年休假5天。"},
		{Level: 2, Text: "其他"},
		{Text: "食堂中午开放。"},
	}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("Blocks 数量 = %d, want %d: %+v", len(doc.Blocks), len(want), doc.Blocks)
	}
	for i := range want {
		if doc.Blocks[i] != want[i] {
			t.Errorf("Blocks[%d] = %+v, want %+v", i, doc.Blocks[i], want[i])
		}
	}
}

func TestLoadTXT(t *testing.T) {
	path := writeFile(t, "说明.txt", []byte("第一段内容。\r\n\r\n第二段内容。\r\n"))
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "txt" || doc.Title != "说明" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("Blocks = %+v", doc.Blocks)
	}
	if doc.Blocks[0].Text != "第一段内容。" || doc.Blocks[1].Text != "第二段内容。" {
		t.Errorf("Blocks = %+v", doc.Blocks)
	}
}

func TestLoadGB18030(t *testing.T) {
	raw, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("年假规定：满一年五天。"))
	if err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, "gbk.txt", raw)
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Text != "年假规定：满一年五天。" {
		t.Errorf("Blocks = %+v", doc.Blocks)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	path := writeFile(t, "a.doc", []byte("x"))
	if _, err := Load(path); err == nil {
		t.Fatal("期望不支持格式报错")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./internal/ingest/loader -v
```

Expected: FAIL（`undefined: Load`）。

- [ ] **Step 4: 实现 `internal/ingest/loader/loader.go`**

```go
package loader

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Block struct {
	Level int
	Text  string
}

type Document struct {
	Path   string
	Title  string
	Format string
	Blocks []Block
}

func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt", ".docx", ".pdf":
		return true
	}
	return false
}

func Load(path string) (Document, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return loadMarkdown(path)
	case ".txt":
		return loadText(path)
	default:
		return Document{}, fmt.Errorf("暂不支持的文档格式: %s", filepath.Ext(path))
	}
}

func fallbackTitle(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func firstTitle(blocks []Block, path string) string {
	for _, b := range blocks {
		if b.Level == 1 && strings.TrimSpace(b.Text) != "" {
			return strings.TrimSpace(b.Text)
		}
	}
	return fallbackTitle(path)
}
```

- [ ] **Step 5: 实现 `internal/ingest/loader/text.go`**

```go
package loader

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var blankLine = regexp.MustCompile(`\n[ \t]*\n`)

func decodeTextFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件: %w", err)
	}
	if utf8.Valid(raw) {
		return string(raw), nil
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
	if err != nil {
		return "", fmt.Errorf("文本编码既不是 UTF-8 也不是 GB18030: %w", err)
	}
	return string(decoded), nil
}

func splitPlainText(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var out []string
	for _, part := range blankLine.Split(s, -1) {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func loadText(path string) (Document, error) {
	content, err := decodeTextFile(path)
	if err != nil {
		return Document{}, err
	}
	var blocks []Block
	for _, p := range splitPlainText(content) {
		blocks = append(blocks, Block{Text: p})
	}
	return Document{Path: path, Title: fallbackTitle(path), Format: "txt", Blocks: blocks}, nil
}

func loadMarkdown(path string) (Document, error) {
	content, err := decodeTextFile(path)
	if err != nil {
		return Document{}, err
	}
	src := []byte(content)
	root := goldmark.New().Parser().Parse(text.NewReader(src))

	var blocks []Block
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			if title := strings.TrimSpace(string(node.Text(src))); title != "" {
				blocks = append(blocks, Block{Level: node.Level, Text: title})
			}
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.TextBlock:
			if body := strings.TrimSpace(string(n.Text(src))); body != "" {
				blocks = append(blocks, Block{Text: body})
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			blocks = appendCodeBlock(blocks, node.Lines(), src)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			blocks = appendCodeBlock(blocks, node.Lines(), src)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return Document{Path: path, Title: firstTitle(blocks, path), Format: "md", Blocks: blocks}, nil
}

func appendCodeBlock(blocks []Block, lines *text.Segments, src []byte) []Block {
	if lines == nil || lines.Len() == 0 {
		return blocks
	}
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(src))
	}
	if code := strings.TrimSpace(b.String()); code != "" {
		blocks = append(blocks, Block{Text: code})
	}
	return blocks
}
```

- [ ] **Step 6: 运行测试确认通过**

```bash
go test ./internal/ingest/loader -run 'TestSupported|TestLoadMarkdown|TestLoadTXT|TestLoadGB18030|TestUnsupportedFormat' -v
```

Expected: PASS（5 个测试）。Task 4 与 Task 5 会分别向 `Load` 的 switch 增加 `.docx`、`.pdf` 分支。

- [ ] **Step 7: 提交**

```bash
gofmt -w internal/ingest/loader
git add internal/ingest/loader go.mod go.sum
git commit -m "feat: 增加 TXT/Markdown 解析"
```

---

### Task 4: DOCX 解析

**Files:**
- Create: `internal/ingest/loader/docx.go`
- Modify: `internal/ingest/loader/loader.go`
- Test: `internal/ingest/loader/docx_test.go`

**Interfaces:**
- Consumes: `Block`、`Document`、`firstTitle`
- Produces: `loadDOCX(path string) (Document, error)`

- [ ] **Step 1: 写失败测试 `internal/ingest/loader/docx_test.go`**

```go
package loader

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const sampleDocxXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>人事制度</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>休假</w:t></w:r></w:p>
    <w:p><w:pPr><w:outlineLvl w:val="2"/></w:pPr><w:r><w:t>年假条款</w:t></w:r></w:p>
    <w:p><w:r><w:t>正文第一段。</w:t></w:r><w:r><w:tab/><w:t>制表符</w:t></w:r></w:p>
    <w:tbl>
      <w:tr><w:tc><w:p><w:r><w:t>姓名</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>天数</w:t></w:r></w:p></w:tc></w:tr>
      <w:tr><w:tc><w:p><w:r><w:t>张三</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>5</w:t></w:r></w:p></w:tc></w:tr>
    </w:tbl>
  </w:body>
</w:document>`

func writeDOCX(t *testing.T, documentXML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, documentXML); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDOCX(t *testing.T) {
	doc, err := Load(writeDOCX(t, sampleDocxXML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "docx" || doc.Title != "人事制度" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Blocks) < 5 {
		t.Fatalf("Blocks = %+v", doc.Blocks)
	}
	if doc.Blocks[0] != (Block{Level: 1, Text: "人事制度"}) {
		t.Errorf("Blocks[0] = %+v", doc.Blocks[0])
	}
	if doc.Blocks[1] != (Block{Level: 2, Text: "休假"}) {
		t.Errorf("Blocks[1] = %+v", doc.Blocks[1])
	}
	if doc.Blocks[2] != (Block{Level: 3, Text: "年假条款"}) {
		t.Errorf("Blocks[2] = %+v", doc.Blocks[2])
	}
	if doc.Blocks[3].Text != "正文第一段。\t制表符" {
		t.Errorf("Blocks[3] = %q", doc.Blocks[3].Text)
	}
	if doc.Blocks[4].Text != "姓名 | 天数\n张三 | 5" {
		t.Errorf("表格 Blocks[4] = %q", doc.Blocks[4].Text)
	}
}

func TestLoadDOCXInvalidZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.docx")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("期望损坏 DOCX 报错")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/ingest/loader -run TestLoadDOCX -v
```

Expected: FAIL（`undefined: loadDOCX` 或编译失败）。

- [ ] **Step 3: 实现 `internal/ingest/loader/docx.go`**

```go
package loader

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type xmlNode struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Text     string
	Children []*xmlNode
}

func parseXMLTree(r io.Reader) (*xmlNode, error) {
	decoder := xml.NewDecoder(r)
	var root *xmlNode
	var stack []*xmlNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: t.Name, Attrs: t.Attr}
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("XML 为空")
	}
	return root, nil
}

func firstChild(n *xmlNode, local string) *xmlNode {
	for _, child := range n.Children {
		if child.Name.Local == local {
			return child
		}
	}
	return nil
}

func childrenOf(n *xmlNode, local string) []*xmlNode {
	var out []*xmlNode
	for _, child := range n.Children {
		if child.Name.Local == local {
			out = append(out, child)
		}
	}
	return out
}

func attrVal(n *xmlNode, local string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

var docxHeadingStyle = regexp.MustCompile(`(?i)^(?:heading|标题)[\s_-]?([1-6])$`)

func headingLevel(style string) int {
	style = strings.TrimSpace(style)
	if style == "" {
		return 0
	}
	if strings.EqualFold(style, "Title") || style == "标题" {
		return 1
	}
	if m := docxHeadingStyle.FindStringSubmatch(style); m != nil {
		level, _ := strconv.Atoi(m[1])
		return level
	}
	if len(style) == 1 && style[0] >= '1' && style[0] <= '6' {
		return int(style[0] - '0')
	}
	return 0
}

func collectRuns(n *xmlNode, b *strings.Builder) {
	for _, child := range n.Children {
		switch child.Name.Local {
		case "t":
			b.WriteString(child.Text)
		case "tab":
			b.WriteString("\t")
		case "br", "cr":
			b.WriteString("\n")
		case "del", "instrText":
			continue
		default:
			collectRuns(child, b)
		}
	}
}

func docxParagraph(p *xmlNode) (Block, bool) {
	level := 0
	if pPr := firstChild(p, "pPr"); pPr != nil {
		if outline := firstChild(pPr, "outlineLvl"); outline != nil {
			if v, err := strconv.Atoi(attrVal(outline, "val")); err == nil && v >= 0 && v <= 8 {
				level = v + 1
			}
		}
		if style := firstChild(pPr, "pStyle"); style != nil {
			if l := headingLevel(attrVal(style, "val")); l > 0 {
				level = l
			}
		}
	}
	var b strings.Builder
	collectRuns(p, &b)
	text := strings.TrimSpace(b.String())
	if text == "" {
		return Block{}, false
	}
	return Block{Level: level, Text: text}, true
}

func docxTable(t *xmlNode) Block {
	var rows []string
	for _, tr := range childrenOf(t, "tr") {
		var cells []string
		for _, tc := range childrenOf(tr, "tc") {
			var parts []string
			for _, p := range childrenOf(tc, "p") {
				if block, ok := docxParagraph(p); ok {
					parts = append(parts, block.Text)
				}
			}
			cells = append(cells, strings.Join(parts, " "))
		}
		if row := strings.TrimSpace(strings.Join(cells, " | ")); row != "" {
			rows = append(rows, row)
		}
	}
	return Block{Text: strings.Join(rows, "\n")}
}

func walkBody(n *xmlNode, blocks *[]Block) {
	for _, child := range n.Children {
		switch child.Name.Local {
		case "p":
			if block, ok := docxParagraph(child); ok {
				*blocks = append(*blocks, block)
			}
		case "tbl":
			if block := docxTable(child); block.Text != "" {
				*blocks = append(*blocks, block)
			}
		case "sdt":
			if content := firstChild(child, "sdtContent"); content != nil {
				walkBody(content, blocks)
			}
		case "body":
			walkBody(child, blocks)
		}
	}
}

func loadDOCX(path string) (Document, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Document{}, fmt.Errorf("打开 DOCX: %w", err)
	}
	defer zr.Close()

	var documentFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			documentFile = f
			break
		}
	}
	if documentFile == nil {
		return Document{}, fmt.Errorf("DOCX 缺少 word/document.xml")
	}
	rc, err := documentFile.Open()
	if err != nil {
		return Document{}, fmt.Errorf("读取 word/document.xml: %w", err)
	}
	defer rc.Close()
	root, err := parseXMLTree(rc)
	if err != nil {
		return Document{}, fmt.Errorf("解析 word/document.xml: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return Document{}, fmt.Errorf("DOCX 缺少 body")
	}
	var blocks []Block
	walkBody(body, &blocks)
	return Document{Path: path, Title: firstTitle(blocks, path), Format: "docx", Blocks: blocks}, nil
}
```

- [ ] **Step 4: 在 `loader.go` 注册 DOCX 分支**

把 `Load` 的 switch 改为：

```go
func Load(path string) (Document, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return loadMarkdown(path)
	case ".txt":
		return loadText(path)
	case ".docx":
		return loadDOCX(path)
	default:
		return Document{}, fmt.Errorf("暂不支持的文档格式: %s", filepath.Ext(path))
	}
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/ingest/loader -run TestLoadDOCX -v
```

Expected: PASS（两个 DOCX 测试通过）。

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/ingest/loader
go test ./internal/ingest/loader -run 'TestLoadDOCX|TestLoadMarkdown|TestLoadTXT' -v
git add internal/ingest/loader
git commit -m "feat: 增加 DOCX 解析"
```

---

### Task 5: PDF 解析（依赖已定：ledongthuc/pdf）

**设计决策依据（已于 2026-10-23 实测）：** 用 `simhei.ttf` 经 `go-pdf/fpdf v0.9.0` 生成中文 PDF 后，`github.com/ledongthuc/pdf` 可正确提取“人事制度 > 休假 > 年假”等中文；`github.com/dslipak/pdf v0.0.2` 输出乱码。因此第一期采用 `ledongthuc/pdf`。设计文档中的 `pdftotext` 兜底仅在后续用真实业务 PDF 验证失败时启用，本次不实现。

**Files:**
- Create: `internal/ingest/loader/pdf.go`
- Modify: `internal/ingest/loader/loader.go`
- Create: `internal/ingest/loader/testdata/sample_zh.pdf`
- Test: `internal/ingest/loader/pdf_test.go`

**Interfaces:**
- Consumes: `Block`、`Document`、`firstTitle`、`splitPlainText`
- Produces: `loadPDF(path string) (Document, error)`

- [ ] **Step 1: 添加依赖**

```bash
go get github.com/ledongthuc/pdf@v0.0.0-20261020125853-5e46805f718e
```

- [ ] **Step 2: 生成中文 PDF 测试夹具**

在项目外的临时目录生成，随后删除临时模块：

```bash
mkdir -p /c/workspace/go/tmp/kbrag-pdfgen
cd /c/workspace/go/tmp/kbrag-pdfgen
go mod init kbrag-pdfgen
go get github.com/go-pdf/fpdf@v0.9.0
```

写入 `gen.go`：

```go
package main

import (
	"fmt"
	"os"

	"github.com/go-pdf/fpdf"
)

func main() {
	out := os.Args[1]
	p := fpdf.New("P", "mm", "A4", "")
	p.AddUTF8Font("simhei", "", "C:/Windows/Fonts/simhei.ttf")
	p.AddPage()
	p.SetFont("simhei", "", 16)
	p.MultiCell(0, 10, "人事制度 > 休假 > 年假", "", "L", false)
	p.SetFont("simhei", "", 12)
	p.MultiCell(0, 8, "员工累计工作满1年不满10年的，年休假5天；满10年不满20年的，年休假10天。", "", "L", false)
	p.MultiCell(0, 8, "年假申请需提前三个工作日提交，由直属主管审批。", "", "L", false)
	if err := p.OutputFileAndClose(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

生成夹具并清理临时模块：

```bash
mkdir -p /c/workspace/go/src/KB-RAG/internal/ingest/loader/testdata
go run gen.go /c/workspace/go/src/KB-RAG/internal/ingest/loader/testdata/sample_zh.pdf
cd /c/workspace/go/src/KB-RAG
rm -rf /c/workspace/go/tmp/kbrag-pdfgen
ls -l internal/ingest/loader/testdata/sample_zh.pdf
```

Expected: 文件存在，大小约 10KB。

- [ ] **Step 3: 写失败测试 `internal/ingest/loader/pdf_test.go`**

```go
package loader

import (
	"strings"
	"testing"
)

func TestLoadPDFChinese(t *testing.T) {
	doc, err := Load("testdata/sample_zh.pdf")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "pdf" || doc.Title != "sample_zh" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Blocks) == 0 || doc.Blocks[0] != (Block{Level: 1, Text: "第 1 页"}) {
		t.Fatalf("Blocks = %+v", doc.Blocks)
	}
	var all strings.Builder
	for _, b := range doc.Blocks {
		all.WriteString(b.Text)
	}
	text := all.String()
	for _, want := range []string{"人事制度 > 休假 > 年假", "年休假5天", "年假申请需提前三个工作日提交"} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF 文本缺少 %q，实际为 %q", want, text)
		}
	}
}
```

- [ ] **Step 4: 运行测试确认失败**

```bash
go test ./internal/ingest/loader -run TestLoadPDFChinese -v
```

Expected: FAIL（`undefined: loadPDF`）。

- [ ] **Step 5: 实现 `internal/ingest/loader/pdf.go`**

```go
package loader

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

func loadPDF(path string) (Document, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("打开 PDF: %w", err)
	}
	defer f.Close()

	var blocks []Block
	for i := 1; i <= reader.NumPage(); i++ {
		pageText, err := reader.Page(i).GetPlainText(nil)
		if err != nil {
			return Document{}, fmt.Errorf("提取 PDF 第 %d 页失败: %w", i, err)
		}
		pageText = strings.TrimSpace(strings.ReplaceAll(pageText, "\r\n", "\n"))
		if pageText == "" {
			continue
		}
		blocks = append(blocks, Block{Level: 1, Text: fmt.Sprintf("第 %d 页", i)})
		for _, paragraph := range splitPlainText(pageText) {
			blocks = append(blocks, Block{Text: paragraph})
		}
	}
	if len(blocks) == 0 {
		return Document{}, fmt.Errorf("PDF 未提取到文本（可能是扫描件，第一期不支持 OCR）")
	}
	// PDF 没有可靠标题，按设计使用文件名，页码只作为切分结构标记。
	return Document{Path: path, Title: fallbackTitle(path), Format: "pdf", Blocks: blocks}, nil
}
```

- [ ] **Step 6: 在 `loader.go` 注册 PDF 分支**

把 `Load` 的 switch 改为：

```go
func Load(path string) (Document, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return loadMarkdown(path)
	case ".txt":
		return loadText(path)
	case ".docx":
		return loadDOCX(path)
	case ".pdf":
		return loadPDF(path)
	default:
		return Document{}, fmt.Errorf("暂不支持的文档格式: %s", filepath.Ext(path))
	}
}
```

- [ ] **Step 7: 运行测试确认通过**

```bash
go test ./internal/ingest/loader -run TestLoadPDFChinese -v
```

Expected: PASS。若出现乱码或断言失败，停止并报告提取结果，不继续后续任务；根据用户提供的真实 PDF 样本再决定是否新增 `pdftotext` loader。

- [ ] **Step 8: 运行 loader 全部测试**

```bash
go test ./internal/ingest/loader -v
```

Expected: 全部 PASS（含 Task 3、Task 4 的测试）。

- [ ] **Step 9: 提交**

```bash
gofmt -w internal/ingest/loader
go test ./internal/ingest/loader -v
git add internal/ingest/loader go.mod go.sum
git commit -m "feat: 增加 PDF 中文解析"
```

---

### Task 6: 结构感知切分

**Files:**
- Create: `internal/ingest/chunker/chunker.go`
- Test: `internal/ingest/chunker/chunker_test.go`

**Interfaces:**
- Consumes: `loader.Document`、`loader.Block`
- Produces: `chunker.Chunk{Seq int; Heading, Text string}`、`chunker.Split(doc loader.Document, size, overlap int) []Chunk`

- [ ] **Step 1: 写失败测试 `internal/ingest/chunker/chunker_test.go`**

```go
package chunker

import (
	"strings"
	"testing"

	"kb-rag/internal/ingest/loader"
)

func doc(blocks ...loader.Block) loader.Document {
	return loader.Document{Title: "test", Blocks: blocks}
}

func TestHeadingBoundaryAndPath(t *testing.T) {
	chunks := Split(doc(
		loader.Block{Level: 1, Text: "人事制度"},
		loader.Block{Level: 2, Text: "休假"},
		loader.Block{Text: "年假规定：员工累计工作满1年不满10年的，年休假5天。"},
		loader.Block{Level: 2, Text: "食堂"},
		loader.Block{Text: "食堂开放时间为中午十二点至下午一点。"},
	), 500, 80)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %+v", chunks)
	}
	if chunks[0].Heading != "人事制度 > 休假" {
		t.Errorf("Heading[0] = %q", chunks[0].Heading)
	}
	if chunks[1].Heading != "人事制度 > 食堂" {
		t.Errorf("Heading[1] = %q", chunks[1].Heading)
	}
	if strings.Contains(chunks[0].Text, "食堂") || strings.Contains(chunks[1].Text, "年假") {
		t.Errorf("片段跨越标题边界: %+v", chunks)
	}
	if chunks[0].Seq != 0 || chunks[1].Seq != 1 {
		t.Errorf("Seq 不连续: %+v", chunks)
	}
}

func TestPackingParagraphs(t *testing.T) {
	chunks := Split(doc(
		loader.Block{Text: strings.Repeat("甲", 60) + "。"},
		loader.Block{Text: strings.Repeat("乙", 60) + "。"},
	), 500, 80)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v", chunks)
	}
}

func TestLongParagraphSizeAndOverlap(t *testing.T) {
	text := strings.Repeat("甲乙丙丁戊己庚辛壬癸。", 80)
	chunks := Split(doc(loader.Block{Text: text}), 100, 20)
	if len(chunks) < 2 {
		t.Fatalf("期望多个片段，实际 %+v", chunks)
	}
	for _, c := range chunks {
		if n := len([]rune(c.Text)); n > 100 {
			t.Errorf("片段长度 %d 超过上限: %q", n, c.Text)
		}
	}
	for i := 0; i+1 < len(chunks); i++ {
		prev := []rune(chunks[i].Text)
		next := []rune(chunks[i+1].Text)
		if len(prev) < 20 || len(next) < 20 {
			continue
		}
		if string(prev[len(prev)-20:]) != string(next[:20]) {
			t.Errorf("片段 %d 与 %d 的重叠不是 20 字符:\n%q\n%q", i, i+1, chunks[i].Text, chunks[i+1].Text)
		}
	}
}

func TestDropFragments(t *testing.T) {
	chunks := Split(doc(loader.Block{Text: "短"}), 500, 80)
	if len(chunks) != 0 {
		t.Fatalf("碎片未被丢弃: %+v", chunks)
	}
}

func TestInvalidParams(t *testing.T) {
	if chunks := Split(doc(loader.Block{Text: "足够长的内容内容内容。"}), 100, 100); len(chunks) != 0 {
		t.Fatalf("非法参数应返回空: %+v", chunks)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/ingest/chunker -v
```

Expected: FAIL（`undefined: Split`）。

- [ ] **Step 3: 实现 `internal/ingest/chunker/chunker.go`**

```go
package chunker

import (
	"strings"
	"unicode/utf8"

	"kb-rag/internal/ingest/loader"
)

const minChunkRunes = 10

type Chunk struct {
	Seq     int
	Heading string
	Text    string
}

type heading struct {
	level int
	text  string
}

func Split(doc loader.Document, size, overlap int) []Chunk {
	if size <= 0 || overlap < 0 || overlap >= size {
		return nil
	}
	var chunks []Chunk
	var stack []heading
	var buf []string
	bufLen := 0

	path := func() string {
		parts := make([]string, 0, len(stack))
		for _, h := range stack {
			parts = append(parts, h.text)
		}
		return strings.Join(parts, " > ")
	}
	flush := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = buf[:0]
		bufLen = 0
		if utf8.RuneCountInString(text) < minChunkRunes {
			return
		}
		chunks = append(chunks, Chunk{Seq: len(chunks), Heading: path(), Text: text})
	}

	for _, block := range doc.Blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		if block.Level > 0 {
			flush()
			for len(stack) > 0 && stack[len(stack)-1].level >= block.Level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{level: block.Level, text: text})
			continue
		}

		n := utf8.RuneCountInString(text)
		if n > size {
			flush()
			for _, piece := range splitLong(text, size, overlap) {
				chunks = append(chunks, Chunk{Seq: len(chunks), Heading: path(), Text: piece})
			}
			continue
		}
		if bufLen > 0 && bufLen+1+n > size {
			flush()
		}
		if bufLen > 0 {
			bufLen++
		}
		buf = append(buf, text)
		bufLen += n
	}
	flush()
	return chunks
}

func splitLong(text string, size, overlap int) []string {
	runes := []rune(text)
	var out []string
	for start := 0; start < len(runes); {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			if cut := lastSentenceCut(runes, start, end); cut > 0 {
				end = cut
			}
		}
		piece := strings.TrimSpace(string(runes[start:end]))
		if utf8.RuneCountInString(piece) >= minChunkRunes {
			out = append(out, piece)
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

func lastSentenceCut(runes []rune, start, end int) int {
	minCut := start + (end-start)/2
	for i := end - 1; i >= minCut; i-- {
		switch runes[i] {
		case '。', '！', '？', '；', '\n':
			return i + 1
		}
	}
	return 0
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/ingest/chunker -v
```

Expected: PASS（5 个测试函数）。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/ingest/chunker
go vet ./internal/ingest/chunker
git add internal/ingest/chunker
git commit -m "feat: 增加结构感知切分"
```

---

### Task 7: Ollama 嵌入客户端

**Files:**
- Create: `internal/ingest/embedder/ollama.go`
- Test: `internal/ingest/embedder/ollama_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `embedder.Embedder` 接口、`embedder.New(baseURL, model string, batchSize int) *Ollama`、`(*Ollama).Embed(ctx, texts []string) ([][]float32, error)`

- [ ] **Step 1: 写失败测试 `internal/ingest/embedder/ollama_test.go`**

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/ingest/embedder -v
```

Expected: FAIL（`undefined: New`）。

- [ ] **Step 3: 实现 `internal/ingest/embedder/ollama.go`**

```go
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
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/ingest/embedder -v
```

Expected: PASS（4 个测试）。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/ingest/embedder
go vet ./internal/ingest/embedder
git add internal/ingest/embedder
git commit -m "feat: 增加 Ollama 嵌入客户端"
```

---

### Task 8: SQLite 存储与检索

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `store.Open(path string) (*Store, error)`
  - `(*Store).Close() error`
  - `(*Store).InitMeta(model string, chunkSize int) error`
  - `(*Store).Meta() (EmbeddingMeta, error)`
  - `(*Store).SetDim(dim int) error`
  - `(*Store).GetDocument(path string) (DocumentInfo, bool, error)`
  - `(*Store).ReplaceDocument(ctx, path, title, hash string, chunks []ChunkInput) error`
  - `(*Store).MarkFailed(ctx, path, title, hash, msg string) error`
  - `(*Store).Search(ctx, query []float32, topK int) ([]Hit, error)`
  - `(*Store).Stats(ctx) (Stats, error)`
  - 类型：`EmbeddingMeta`、`DocumentInfo`、`ChunkInput`、`Hit`、`Stats`

- [ ] **Step 1: 添加依赖并写失败测试 `internal/store/store_test.go`**

```bash
go get modernc.org/sqlite@v1.47.0
```

```go
package store

import (
	"context"
	"database/sql"
	"path/filepath"
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
	if len(hits) != 1 || hits[0].Text != "旧版本内容" {
		t.Fatalf("更新失败后应保留旧版本可检索: %+v", hits)
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/store -v
```

Expected: FAIL（`undefined: Open`）。

- [ ] **Step 3: 实现 `internal/store/store.go`**

```go
package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = "1"

type Store struct {
	db   *sql.DB
	path string
}

type EmbeddingMeta struct {
	SchemaVersion string
	Model         string
	Dim           int
	ChunkSize     int
}

type DocumentInfo struct {
	ID        int64
	Path      string
	Title     string
	Hash      string
	Status    string
	Error     string
	UpdatedAt string
}

type ChunkInput struct {
	Seq     int
	Heading string
	Text    string
	Vector  []float32
}

type Hit struct {
	ChunkID int64
	DocID   int64
	Path    string
	Title   string
	Heading string
	Text    string
	Score   float64
}

type Stats struct {
	Documents       int
	DocumentsFailed int
	Chunks          int
	SizeBytes       int64
	FailedPaths     []string
	Meta            EmbeddingMeta
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建库目录 %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用 WAL: %w", err)
	}
	for _, pragma := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS documents(
			id INTEGER PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS chunks(
			id INTEGER PRIMARY KEY,
			doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			heading TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			vector BLOB NOT NULL,
			dim INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_doc ON chunks(doc_id)`,
		`CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化数据库: %w", err)
		}
	}
	version, ok, err := s.metaGet("schema_version")
	if err != nil {
		return err
	}
	if !ok {
		return s.metaSet("schema_version", SchemaVersion)
	}
	if version != SchemaVersion {
		return fmt.Errorf("数据库 schema 版本 %s 与程序 %s 不一致，请删除库文件后重建", version, SchemaVersion)
	}
	return nil
}

func (s *Store) metaGet(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) metaSet(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) InitMeta(model string, chunkSize int) error {
	if model == "" || chunkSize <= 0 {
		return errors.New("嵌入模型和 chunk_size 必须有效")
	}
	currentModel, ok, err := s.metaGet("embedding_model")
	if err != nil {
		return err
	}
	if ok && currentModel != model {
		return fmt.Errorf("嵌入模型不一致：库内为 %q，配置为 %q；请删除库文件后重新 ingest", currentModel, model)
	}
	if !ok {
		if err := s.metaSet("embedding_model", model); err != nil {
			return err
		}
	}
	currentSize, ok, err := s.metaGet("chunk_size")
	if err != nil {
		return err
	}
	if ok {
		size, err := strconv.Atoi(currentSize)
		if err != nil {
			return fmt.Errorf("库内 chunk_size 值 %q 非法", currentSize)
		}
		if size != chunkSize {
			return fmt.Errorf("切分参数不一致：库内为 %d，配置为 %d；请删除库文件后重新 ingest", size, chunkSize)
		}
	} else if err := s.metaSet("chunk_size", strconv.Itoa(chunkSize)); err != nil {
		return err
	}
	return nil
}

func (s *Store) Meta() (EmbeddingMeta, error) {
	var meta EmbeddingMeta
	rows, err := s.db.Query(`SELECT key,value FROM meta`)
	if err != nil {
		return meta, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return meta, err
		}
		switch key {
		case "schema_version":
			meta.SchemaVersion = value
		case "embedding_model":
			meta.Model = value
		case "dim":
			dim, err := strconv.Atoi(value)
			if err != nil {
				return meta, fmt.Errorf("meta.dim 值 %q 非法", value)
			}
			meta.Dim = dim
		case "chunk_size":
			size, err := strconv.Atoi(value)
			if err != nil {
				return meta, fmt.Errorf("meta.chunk_size 值 %q 非法", value)
			}
			meta.ChunkSize = size
		}
	}
	return meta, rows.Err()
}

func (s *Store) SetDim(dim int) error {
	if dim <= 0 {
		return fmt.Errorf("向量维度必须大于 0")
	}
	current, ok, err := s.metaGet("dim")
	if err != nil {
		return err
	}
	if !ok {
		return s.metaSet("dim", strconv.Itoa(dim))
	}
	n, err := strconv.Atoi(current)
	if err != nil {
		return fmt.Errorf("meta.dim 值 %q 非法", current)
	}
	if n != dim {
		return fmt.Errorf("嵌入维度不一致：库内为 %d，本次为 %d；请删除库文件后重新 ingest", n, dim)
	}
	return nil
}

func (s *Store) GetDocument(path string) (DocumentInfo, bool, error) {
	var info DocumentInfo
	err := s.db.QueryRow(`SELECT id,path,title,content_hash,status,error,updated_at FROM documents WHERE path=?`, path).
		Scan(&info.ID, &info.Path, &info.Title, &info.Hash, &info.Status, &info.Error, &info.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentInfo{}, false, nil
	}
	if err != nil {
		return DocumentInfo{}, false, err
	}
	return info, true, nil
}

func (s *Store) ReplaceDocument(ctx context.Context, path, title, hash string, chunks []ChunkInput) error {
	dim, err := s.dim()
	if err != nil {
		return err
	}
	if dim <= 0 {
		return errors.New("嵌入维度尚未初始化")
	}
	for i, chunk := range chunks {
		if len(chunk.Vector) != dim {
			return fmt.Errorf("第 %d 个片段维度为 %d，期望 %d", i+1, len(chunk.Vector), dim)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO documents(path,title,content_hash,status,error,updated_at)
		VALUES(?,?,?,'ok','',?)
		ON CONFLICT(path) DO UPDATE SET title=excluded.title, content_hash=excluded.content_hash,
			status='ok', error='', updated_at=excluded.updated_at`, path, title, hash, now); err != nil {
		return err
	}
	var docID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM documents WHERE path=?`, path).Scan(&docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE doc_id=?`, docID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks(doc_id,seq,heading,text,vector,dim) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, chunk := range chunks {
		if _, err := stmt.ExecContext(ctx, docID, chunk.Seq, chunk.Heading, chunk.Text, encodeVector(normalize(chunk.Vector)), dim); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MarkFailed(ctx context.Context, path, title, hash, msg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO documents(path,title,content_hash,status,error,updated_at)
		VALUES(?,?,?,'failed',?,?)
		ON CONFLICT(path) DO UPDATE SET title=excluded.title, content_hash=excluded.content_hash,
			status='failed', error=excluded.error, updated_at=excluded.updated_at`, path, title, hash, msg, now)
	return err
}

func (s *Store) Search(ctx context.Context, query []float32, topK int) ([]Hit, error) {
	if topK <= 0 {
		return nil, fmt.Errorf("top-k 必须大于 0")
	}
	dim, err := s.dim()
	if err != nil {
		return nil, err
	}
	if dim <= 0 {
		return nil, errors.New("知识库尚未完成初始化，请先执行 ingest")
	}
	if len(query) != dim {
		return nil, fmt.Errorf("查询向量维度为 %d，库内为 %d", len(query), dim)
	}
	q := normalize(query)
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.doc_id,c.heading,c.text,c.vector,c.dim,d.path,d.title
		FROM chunks c JOIN documents d ON d.id=c.doc_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var (
			hit      Hit
			vector   []byte
			chunkDim int
		)
		if err := rows.Scan(&hit.ChunkID, &hit.DocID, &hit.Heading, &hit.Text, &vector, &chunkDim, &hit.Path, &hit.Title); err != nil {
			return nil, err
		}
		if chunkDim != dim {
			return nil, fmt.Errorf("片段 %d 维度为 %d，库内为 %d", hit.ChunkID, chunkDim, dim)
		}
		decoded, err := decodeVector(vector)
		if err != nil {
			return nil, err
		}
		if len(decoded) != dim {
			return nil, fmt.Errorf("片段 %d 向量长度为 %d，库内维度为 %d", hit.ChunkID, len(decoded), dim)
		}
		var score float64
		for i := range q {
			score += float64(q[i]) * float64(decoded[i])
		}
		hit.Score = score
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&stats.Documents); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents WHERE status='failed'`).Scan(&stats.DocumentsFailed); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&stats.Chunks); err != nil {
		return stats, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM documents WHERE status='failed' ORDER BY path`)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return stats, err
		}
		stats.FailedPaths = append(stats.FailedPaths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return stats, err
	}
	rows.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(s.path + suffix); err == nil {
			stats.SizeBytes += info.Size()
		}
	}
	meta, err := s.Meta()
	if err != nil {
		return stats, err
	}
	stats.Meta = meta
	return stats, nil
}

func (s *Store) dim() (int, error) {
	meta, err := s.Meta()
	if err != nil {
		return 0, err
	}
	return meta.Dim, nil
}

func encodeVector(vector []float32) []byte {
	encoded := make([]byte, 4*len(vector))
	for i, value := range vector {
		binary.LittleEndian.PutUint32(encoded[i*4:], math.Float32bits(value))
	}
	return encoded
}

func decodeVector(encoded []byte) ([]float32, error) {
	if len(encoded)%4 != 0 {
		return nil, fmt.Errorf("向量字节长度 %d 不是 4 的倍数", len(encoded))
	}
	vector := make([]float32, len(encoded)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[i*4:]))
	}
	return vector, nil
}

func normalize(vector []float32) []float32 {
	out := make([]float32, len(vector))
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	if sum == 0 {
		copy(out, vector)
		return out
	}
	scale := float32(1 / math.Sqrt(sum))
	for i, value := range vector {
		out[i] = value * scale
	}
	return out
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/store -v
```

Expected: PASS（6 个测试）。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/store
go vet ./internal/store
git add internal/store go.mod go.sum
git commit -m "feat: 增加 SQLite 存储与向量检索"
```

---

### Task 9: OpenAI 兼容生成客户端

**Files:**
- Create: `internal/llm/client.go`
- Test: `internal/llm/client_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `llm.Message{Role, Content string}`、`llm.New(baseURL, model, apiKey string) *Client`、`(*Client).Chat(ctx, messages []Message) (string, error)`

- [ ] **Step 1: 写失败测试 `internal/llm/client_test.go`**

```go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatSendsExpectedRequest(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "答案[1]。"}}},
		})
	}))
	defer server.Close()

	client := New(server.URL+"/v1", "deepseek-chat", "secret")
	answer, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "规则"},
		{Role: "user", Content: "问题"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "答案[1]。" {
		t.Errorf("answer = %q", answer)
	}
	if got.Model != "deepseek-chat" || got.Temperature != 0.2 || got.MaxTokens != 1024 {
		t.Errorf("请求参数 = %+v", got)
	}
	if len(got.Messages) != 2 || got.Messages[1].Content != "问题" {
		t.Errorf("messages = %+v", got.Messages)
	}
}

func TestChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	if _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}}); err == nil {
		t.Fatal("期望 HTTP 错误")
	}
}

func TestChatEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	if _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}}); err == nil {
		t.Fatal("期望空 choices 报错")
	}
}

func TestChatErrorMessageDoesNotLeakKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()
	client := New(server.URL, "m", "super-secret-key")
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}})
	if err == nil {
		t.Fatal("期望错误")
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Fatal("错误信息泄露 API Key")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/llm -v
```

Expected: FAIL（`undefined: New`）。

- [ ] **Step 3: 实现 `internal/llm/client.go`**

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	BaseURL string
	Model   string
	APIKey  string
	HTTP    *http.Client
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func New(baseURL, model, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: 0.2,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("生成服务返回 %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("解析生成服务响应: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("生成服务返回空 choices")
	}
	answer := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if answer == "" {
		return "", fmt.Errorf("生成服务返回空答案")
	}
	return answer, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/llm -v
```

Expected: PASS（4 个测试）。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/llm
go vet ./internal/llm
git add internal/llm
git commit -m "feat: 增加 OpenAI 兼容生成客户端"
```

---

### Task 10: 查询编排与引用格式化

**Files:**
- Create: `internal/query/query.go`
- Test: `internal/query/query_test.go`

**Interfaces:**
- Consumes: `store`、`llm`
- Produces: `query.Service`、`(*Service).Ask(ctx, question) (Result, error)`、`query.FormatResult`、`query.FormatHits`、`query.DisplayPath`

- [ ] **Step 1: 写失败测试 `internal/query/query_test.go`**

```go
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
	meta store.EmbeddingMeta
	hits []store.Hit
	err  error
}

func (f *fakeStore) Meta() (store.EmbeddingMeta, error) { return f.meta, nil }
func (f *fakeStore) Search(ctx context.Context, query []float32, topK int) ([]store.Hit, error) {
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/query -v
```

Expected: FAIL（`undefined: Service`）。

- [ ] **Step 3: 实现 `internal/query/query.go`**

```go
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
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/query -v
```

Expected: PASS（6 个测试）。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/query
go vet ./internal/query
git add internal/query
git commit -m "feat: 增加检索与引用编排"
```

---

### Task 11: 入库管道

**Files:**
- Create: `internal/ingest/ingest.go`
- Test: `internal/ingest/ingest_test.go`

**Interfaces:**
- Consumes: `loader`、`chunker`、`store`
- Produces: `ingest.Run(ctx, root string, st Store, emb Embedder, opt Options) (Summary, error)`、`ingest.Options`、`ingest.Summary`

- [ ] **Step 1: 写失败测试 `internal/ingest/ingest_test.go`**

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/ingest -run TestRun -v
```

Expected: FAIL（`undefined: Run`）。

- [ ] **Step 3: 实现 `internal/ingest/ingest.go`**

```go
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kb-rag/internal/ingest/chunker"
	"kb-rag/internal/ingest/loader"
	"kb-rag/internal/store"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Store interface {
	InitMeta(model string, chunkSize int) error
	GetDocument(path string) (store.DocumentInfo, bool, error)
	SetDim(dim int) error
	ReplaceDocument(ctx context.Context, path, title, hash string, chunks []store.ChunkInput) error
	MarkFailed(ctx context.Context, path, title, hash, msg string) error
}

type Options struct {
	Model     string
	ChunkSize int
	Overlap   int
	Logger    *slog.Logger
}

type Summary struct {
	Succeeded   int
	Skipped     int
	Failed      int
	FailedPaths []string
}

type fileResult struct {
	outcome string
	chunks  int
	err     error
	fatal   error
}

const (
	outcomeOK      = "ok"
	outcomeSkipped = "skipped"
	outcomeFailed  = "failed"
)

func Run(ctx context.Context, root string, st Store, emb Embedder, opt Options) (Summary, error) {
	var summary Summary
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return summary, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return summary, fmt.Errorf("访问目录 %s: %w", root, err)
	}
	if !info.IsDir() {
		return summary, fmt.Errorf("%s 不是目录", root)
	}
	if err := st.InitMeta(opt.Model, opt.ChunkSize); err != nil {
		return summary, err
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			opt.Logger.Warn("访问路径失败", "path", displayPath(absRoot, path), "error", walkErr)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if path != absRoot && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !loader.Supported(path) {
			return nil
		}
		rel := displayPath(absRoot, path)
		start := time.Now()
		res := processFile(ctx, st, emb, opt, path)
		if res.fatal != nil {
			return res.fatal
		}
		switch res.outcome {
		case outcomeOK:
			summary.Succeeded++
			opt.Logger.Info("文档入库", "path", rel, "chunks", res.chunks, "duration_ms", time.Since(start).Milliseconds())
		case outcomeSkipped:
			summary.Skipped++
			opt.Logger.Info("文档未变化，跳过", "path", rel)
		case outcomeFailed:
			summary.Failed++
			summary.FailedPaths = append(summary.FailedPaths, rel)
			opt.Logger.Error("文档入库失败", "path", rel, "error", res.err)
		}
		return nil
	})
	if err != nil {
		return summary, err
	}
	opt.Logger.Info("入库完成", "succeeded", summary.Succeeded, "skipped", summary.Skipped, "failed", summary.Failed)
	return summary, nil
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func processFile(ctx context.Context, st Store, emb Embedder, opt Options, path string) fileResult {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileResult{outcome: outcomeFailed, err: err}
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])

	existing, exists, err := st.GetDocument(path)
	if err != nil {
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if exists && existing.Status == "ok" && existing.Hash == hash {
		return fileResult{outcome: outcomeSkipped}
	}

	doc, loadErr := loader.Load(path)
	title := ""
	if loadErr == nil {
		title = doc.Title
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if loadErr != nil {
		markFailed(ctx, st, opt, path, title, hash, loadErr.Error())
		return fileResult{outcome: outcomeFailed, err: loadErr}
	}

	chunks := chunker.Split(doc, opt.ChunkSize, opt.Overlap)
	if len(chunks) == 0 {
		err := fmt.Errorf("未切分出有效片段")
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}
	vectors, err := emb.Embed(ctx, texts)
	if err != nil {
		if ctx.Err() != nil {
			return fileResult{outcome: outcomeFailed, err: ctx.Err(), fatal: ctx.Err()}
		}
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if len(vectors) != len(chunks) {
		err := fmt.Errorf("嵌入返回 %d 个向量，期望 %d 个", len(vectors), len(chunks))
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if err := validateVectors(vectors); err != nil {
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	if err := st.SetDim(len(vectors[0])); err != nil {
		return fileResult{outcome: outcomeFailed, err: err, fatal: err}
	}

	inputs := make([]store.ChunkInput, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = store.ChunkInput{Seq: chunk.Seq, Heading: chunk.Heading, Text: chunk.Text, Vector: vectors[i]}
	}
	if err := st.ReplaceDocument(ctx, path, title, hash, inputs); err != nil {
		if ctx.Err() != nil {
			return fileResult{outcome: outcomeFailed, err: ctx.Err(), fatal: ctx.Err()}
		}
		markFailed(ctx, st, opt, path, title, hash, err.Error())
		return fileResult{outcome: outcomeFailed, err: err}
	}
	return fileResult{outcome: outcomeOK, chunks: len(chunks)}
}

func validateVectors(vectors [][]float32) error {
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return fmt.Errorf("嵌入服务返回空向量")
	}
	dim := len(vectors[0])
	for i, vector := range vectors {
		if len(vector) != dim {
			return fmt.Errorf("第 %d 个向量维度为 %d，期望 %d", i+1, len(vector), dim)
		}
	}
	return nil
}

func markFailed(ctx context.Context, st Store, opt Options, path, title, hash, msg string) {
	if err := st.MarkFailed(ctx, path, title, hash, msg); err != nil {
		opt.Logger.Warn("记录失败状态失败", "path", path, "error", err)
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/ingest -v
```

Expected: PASS（3 个测试）。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/ingest
go vet ./internal/ingest
git add internal/ingest/ingest.go internal/ingest/ingest_test.go
git commit -m "feat: 增加增量入库管道"
```

---

### Task 12: CLI 与全链路集成测试

**Files:**
- Create: `cmd/KB-RAG/main.go`
- Create: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: 上述全部组件
- Produces: `cli.Run(ctx context.Context, args []string, stdout, stderr io.Writer) int`、可执行文件 `KB-RAG`

- [ ] **Step 1: 实现 `cmd/KB-RAG/main.go`**

```go
package main

import (
	"context"
	"os"
	"os/signal"

	"kb-rag/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 2: 实现 `internal/cli/cli.go`**

```go
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"kb-rag/internal/config"
	"kb-rag/internal/ingest"
	"kb-rag/internal/ingest/embedder"
	"kb-rag/internal/llm"
	"kb-rag/internal/logging"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

const (
	exitOK          = 0
	exitRuntime     = 1
	exitConfig      = 2
	exitInterrupted = 130
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitConfig
	}
	switch args[0] {
	case "ingest":
		return runIngest(ctx, args[1:], stdout, stderr)
	case "ask":
		return runAsk(ctx, args[1:], stdout, stderr)
	case "stats":
		return runStats(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "未知命令 %q\n\n", args[0])
		usage(stderr)
		return exitConfig
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "用法：")
	fmt.Fprintln(w, "  KB-RAG ingest [--config 路径] <目录>")
	fmt.Fprintln(w, "  KB-RAG ask [--config 路径] [--top-k N] \"问题\"")
	fmt.Fprintln(w, "  KB-RAG stats [--config 路径]")
}

func newFlagSet(name string, stderr io.Writer, usageLine string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./config.yaml", "配置文件路径")
	fs.Usage = func() {
		fmt.Fprintln(stderr, usageLine)
		fs.PrintDefaults()
	}
	return fs, cfgPath
}

func flagExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitConfig
}

func interrupted(ctx context.Context) bool {
	return ctx.Err() != nil
}

func runIngest(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("ingest", stderr, "用法: KB-RAG ingest [--config 路径] <目录>")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitConfig
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	if err := cfg.Validate("ingest"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	logger, closer, err := logging.New(cfg.Log, "ingest", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "日志初始化失败: %v\n", err)
		return exitRuntime
	}
	defer closer.Close()

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		logger.Error("打开知识库失败", "error", err)
		return exitRuntime
	}
	defer st.Close()

	emb := embedder.New(cfg.Embedding.BaseURL, cfg.Embedding.Model, cfg.Embedding.BatchSize)
	summary, err := ingest.Run(ctx, fs.Arg(0), st, emb, ingest.Options{
		Model:     cfg.Embedding.Model,
		ChunkSize: cfg.Chunk.Size,
		Overlap:   cfg.Chunk.Overlap,
		Logger:    logger,
	})
	if err != nil {
		logger.Error("入库中断", "error", err)
		if interrupted(ctx) {
			return exitInterrupted
		}
		return exitRuntime
	}
	fmt.Fprintf(stdout, "入库完成：成功 %d，跳过 %d，失败 %d\n", summary.Succeeded, summary.Skipped, summary.Failed)
	if summary.Failed > 0 {
		fmt.Fprintln(stdout, "失败文件：")
		for _, path := range summary.FailedPaths {
			fmt.Fprintf(stdout, "  - %s\n", path)
		}
		return exitRuntime
	}
	return exitOK
}

func runAsk(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("ask", stderr, `用法: KB-RAG ask [--config 路径] [--top-k N] "问题"`)
	topK := fs.Int("top-k", 8, "检索片段数量")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		return exitConfig
	}
	if *topK <= 0 {
		fmt.Fprintln(stderr, "参数错误: --top-k 必须大于 0")
		return exitConfig
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	if err := cfg.Validate("ask"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	logger, closer, err := logging.New(cfg.Log, "ask", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "日志初始化失败: %v\n", err)
		return exitRuntime
	}
	defer closer.Close()

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		logger.Error("打开知识库失败", "error", err)
		return exitRuntime
	}
	defer st.Close()

	baseDir, err := os.Getwd()
	if err != nil {
		logger.Error("读取当前目录失败", "error", err)
		return exitRuntime
	}
	service := &query.Service{
		Embedder:   embedder.New(cfg.Embedding.BaseURL, cfg.Embedding.Model, cfg.Embedding.BatchSize),
		Store:      st,
		LLM:        llm.New(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.APIKey),
		EmbedModel: cfg.Embedding.Model,
		TopK:       *topK,
		BaseDir:    baseDir,
		Logger:     logger,
	}
	result, err := service.Ask(ctx, fs.Arg(0))
	if err != nil {
		if len(result.Hits) > 0 {
			fmt.Fprintln(stdout, query.FormatHits(result.Hits, baseDir))
			logger.Error("生成失败，已输出检索片段", "error", err)
		} else {
			logger.Error("问答失败", "error", err)
		}
		if interrupted(ctx) {
			return exitInterrupted
		}
		return exitRuntime
	}
	fmt.Fprintln(stdout, query.FormatResult(result, baseDir))
	return exitOK
}

func runStats(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("stats", stderr, "用法: KB-RAG stats [--config 路径]")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitConfig
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	if err := cfg.Validate("ingest"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}
	logger, closer, err := logging.New(cfg.Log, "stats", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "日志初始化失败: %v\n", err)
		return exitRuntime
	}
	defer closer.Close()

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		logger.Error("打开知识库失败", "error", err)
		return exitRuntime
	}
	defer st.Close()

	stats, err := st.Stats(ctx)
	if err != nil {
		logger.Error("读取统计失败", "error", err)
		return exitRuntime
	}
	fmt.Fprintf(stdout, "文档：%d（失败 %d）\n", stats.Documents, stats.DocumentsFailed)
	fmt.Fprintf(stdout, "片段：%d\n", stats.Chunks)
	if stats.Meta.Model == "" {
		fmt.Fprintln(stdout, "嵌入模型：未初始化")
	} else {
		fmt.Fprintf(stdout, "嵌入模型：%s（%d 维）\n", stats.Meta.Model, stats.Meta.Dim)
	}
	if stats.Meta.ChunkSize > 0 {
		fmt.Fprintf(stdout, "切分：%d 字符\n", stats.Meta.ChunkSize)
	}
	fmt.Fprintf(stdout, "库大小：%.1f MB\n", float64(stats.SizeBytes)/(1024*1024))
	if stats.DocumentsFailed > 0 {
		baseDir, _ := os.Getwd()
		fmt.Fprintln(stdout, "失败文档：")
		for _, path := range stats.FailedPaths {
			fmt.Fprintf(stdout, "  - %s\n", query.DisplayPath(path, baseDir))
		}
	}
	if stats.Chunks > 200000 {
		fmt.Fprintln(stdout, "提示：片段数超过 20 万，建议迁移到 Qdrant。")
	}
	return exitOK
}
```

- [ ] **Step 3: 写集成测试 `internal/cli/cli_test.go`**

```go
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
```

- [ ] **Step 4: 运行集成测试**

```bash
go test ./internal/cli -v
```

Expected: PASS（3 个测试）。若失败，先修实现，不得跳过。

- [ ] **Step 5: 构建并验证 CLI 帮助**

```bash
go build -o KB-RAG.exe ./cmd/KB-RAG
./KB-RAG.exe help
```

Expected: 打印三条用法，退出码 0。

- [ ] **Step 6: 提交**

```bash
gofmt -w cmd internal/cli
go vet ./...
git add cmd internal/cli
git commit -m "feat: 完成 ingest/ask/stats CLI"
```

---

### Task 13: 文档、评测模板与最终验证

**Files:**
- Create: `README.md`
- Create: `docs/eval/questions.md`

**Interfaces:**
- Consumes: 全部
- Produces: 使用说明与评测模板

- [ ] **Step 1: 写 `README.md`**

````markdown
# KB-RAG

企业知识库 RAG 命令行工具。文档在本地解析、切分并存储，嵌入通过本地 Ollama 完成，答案由云端 OpenAI 兼容接口生成，并附带原文引用。

## 前置条件

- Go 1.26.5
- 本地 Ollama，已拉取 `bge-m3`：

```bash
ollama pull bge-m3
```

- 一个 OpenAI 兼容生成服务的 API Key。

## 配置

```bash
cp config.example.yaml config.yaml
```

API Key 只从环境变量读取：

```bash
# Linux/macOS
export KB_RAG_LLM_API_KEY=your-key

# Windows PowerShell
$env:KB_RAG_LLM_API_KEY="your-key"
```

## 构建与使用

```bash
go build -o KB-RAG.exe ./cmd/KB-RAG

# 增量入库；重复执行会跳过未变化文件
./KB-RAG.exe ingest ./docs

# 检索并生成带引用的答案
./KB-RAG.exe ask "年假怎么请"
./KB-RAG.exe ask --top-k 4 "年假怎么请"

# 查看状态
./KB-RAG.exe stats
```

运行时数据默认写入 `./data/`，不提交到版本库。

## 已知限制

- 扫描件 PDF 不做 OCR。
- PDF 使用纯 Go 库提取文本；若真实业务 PDF 出现乱码，可改用 `pdftotext` 兜底（需安装 poppler）。
- 检索为内存暴力点积，适合小规模知识库；片段超过 20 万时 `stats` 会提示迁移。
- 第一期不做重排、混合检索、query rewrite、多轮对话与流式输出。

## 测试

```bash
go test ./...
go vet ./...
```

集成测试使用假 Ollama 与假生成服务，不需要真实网络或模型。
````

- [ ] **Step 2: 写 `docs/eval/questions.md`**

```markdown
# KB-RAG 人工评测

使用 20-30 个真实业务问题，另加 3-5 个知识库中无答案的问题。每条记录：

- 正确片段是否进入 top-8
- 答案是否正确
- 引用是否准确

## 验收线

- 正确片段进入 top-8：≥ 85%
- 答案正确且引用无误：≥ 80%
- 无答案问题拒答：≥ 80%

## 有答案问题

| # | 问题 | 预期答案要点 | 正确片段进 top-8 | 答案正确 | 引用准确 | 备注 |
|---|------|--------------|------------------|----------|----------|------|
| 1 | | | ☐ | ☐ | ☐ | |
| 2 | | | ☐ | ☐ | ☐ | |
| 3 | | | ☐ | ☐ | ☐ | |
| 4 | | | ☐ | ☐ | ☐ | |
| 5 | | | ☐ | ☐ | ☐ | |
| 6 | | | ☐ | ☐ | ☐ | |
| 7 | | | ☐ | ☐ | ☐ | |
| 8 | | | ☐ | ☐ | ☐ | |
| 9 | | | ☐ | ☐ | ☐ | |
| 10 | | | ☐ | ☐ | ☐ | |
| 11 | | | ☐ | ☐ | ☐ | |
| 12 | | | ☐ | ☐ | ☐ | |
| 13 | | | ☐ | ☐ | ☐ | |
| 14 | | | ☐ | ☐ | ☐ | |
| 15 | | | ☐ | ☐ | ☐ | |
| 16 | | | ☐ | ☐ | ☐ | |
| 17 | | | ☐ | ☐ | ☐ | |
| 18 | | | ☐ | ☐ | ☐ | |
| 19 | | | ☐ | ☐ | ☐ | |
| 20 | | | ☐ | ☐ | ☐ | |

## 无答案问题（验证拒答）

| # | 问题 | 是否拒答 | 备注 |
|---|------|----------|------|
| 1 | | ☐ | |
| 2 | | ☐ | |
| 3 | | ☐ | |
| 4 | | ☐ | |
| 5 | | ☐ | |

## 环境冒烟记录

- 日期：
- Ollama 版本：
- `bge-m3` 维度：
- 入库样例文档数 / 片段数：
- 备注：
```

- [ ] **Step 3: 全量验证**

```bash
go mod tidy
gofmt -l .
go vet ./...
go test ./...
go build -o KB-RAG.exe ./cmd/KB-RAG
```

Expected:
- `gofmt -l .` 无输出
- `go vet ./...` 无输出
- `go test ./...` 全部 PASS
- 构建成功

- [ ] **Step 4: 清理临时产物**

```bash
rm -rf /c/workspace/go/tmp/kbrag-spike
git status --short
```

Expected: 临时目录不存在；`git status` 只包含本任务的 README、评测模板与计划文档（计划文档在 Task 1 之前已存在，可单独提交）。

- [ ] **Step 5: 提交文档**

```bash
git add README.md docs/eval/questions.md docs/superpowers/plans/2026-10-23-KB-RAG-implementation.md go.mod go.sum
git commit -m "docs: 增加使用说明与人工评测模板"
```

---

## 实施后的真实环境冒烟（需用户环境，未完成前不得声称第一期验收通过）

当前机器未安装 Ollama，因此以下步骤在实现完成后单独执行：

```bash
ollama --version
ollama pull bge-m3
curl -s http://localhost:11434/api/embed -d '{"model":"bge-m3","input":["测试"]}' | head -c 200
```

预期返回 `embeddings` 且维度为 1024。

然后：

```bash
cp config.example.yaml config.yaml
export KB_RAG_LLM_API_KEY=...
./KB-RAG.exe ingest <真实文档目录>
./KB-RAG.exe stats
./KB-RAG.exe ask "一个真实业务问题"
```

需要用户提供：
1. 每种格式各几份有代表性的真实文档（可脱敏），用于验证 PDF/DOCX 中文提取与切分效果。
2. 20-30 个真实问题与预期答案，填入 `docs/eval/questions.md`。

---

## 自检结果

**1. Spec coverage**

| 设计文档要求 | 覆盖任务 |
|--------------|----------|
| CLI `ingest` / `ask` / `stats` | Task 12 |
| MD/TXT/DOCX/PDF 解析 | Task 3、4、5 |
| 结构感知切分、500/80、标题路径、碎片丢弃 | Task 6 |
| Ollama bge-m3、批量、重试、120s | Task 7 |
| SQLite、WAL、busy_timeout、事务、增量、meta 一致性 | Task 8、11 |
| 内存点积 top-k、归一化、float32 BLOB | Task 8 |
| OpenAI 兼容、温度 0.2、max_tokens 1024、90s | Task 9 |
| Prompt 约束、引用格式、生成失败兜底 | Task 10、12 |
| 配置校验、环境变量 Key、退出码 | Task 1、12 |
| 日志轮转、不落正文、不泄露 Key | Task 2、9、10 |
| 单元测试、假服务集成测试 | Task 1-12 |
| 评测模板与验收线 | Task 13 |
| 统计、20 万提示、失败文档 | Task 8、12 |

**2. Placeholder scan:** 无 TBD/TODO/“稍后实现”；所有代码步骤均为完整文件。

**3. Type consistency:** `store.ChunkInput`、`store.Hit`、`store.DocumentInfo`、`store.EmbeddingMeta`、`store.Stats` 在 Task 8 定义，Task 10、11、12 均按同一签名使用；`loader.Block` 在 Task 3 定义，Task 4-6 一致；`llm.Message` 在 Task 9 定义，Task 10 一致。

**已知解释性决策（实施时保留）：**
- `meta.chunk_size` 不一致时 `ingest` 直接报错，避免 hash 跳过后新旧切分混存；设计文档只显式要求模型/维度一致，这是为保证增量正确性增加的校验。
- 文档更新失败时保留旧片段且仍可被检索（设计文档“保留旧版本”的落地方式）；文档状态标为 `failed`，下次 ingest 因状态非 `ok` 而重试。
- `stats` 的“文档”统计总数，失败数单独展示。
- PDF 采用 `ledongthuc/pdf`（已用真实中文 PDF 验证）；`pdftotext` 兜底仅保留为后续真实文档验证失败时的预案，第一期不实现。
- 本项目不是 git 仓库，Task 1 会执行 `git init`；如用户不希望初始化仓库，执行前需确认。


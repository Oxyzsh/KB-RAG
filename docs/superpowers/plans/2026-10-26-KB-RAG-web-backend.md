# KB-RAG Web 后端实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 CLI 之上新增 `KB-RAG serve`、HTTP API、共享令牌认证、文档上传/列表/重试/删除，为 `fronted/` 的 Vue 3 界面提供后端。

**Architecture:** 复用现有 `config`、`store`、`ingest`、`query`、`embedder`、`llm`；新增 `internal/web` 提供 `net/http` API 与静态文件托管。上传文件保存在 `data/uploads/<随机目录>/<安全文件名>`，同步调用单文件入库；删除只删 SQLite 索引，源文件保留。

**Tech Stack:** Go 1.26.5、标准库 `net/http`、现有 `modernc.org/sqlite`、`log/slog`。

**设计依据：** [`docs/superpowers/specs/2026-10-26-KB-RAG-web-design.md`](../specs/2026-10-26-KB-RAG-web-design.md)

## Global Constraints

- Go 版本：1.26.5；不新增 Go 第三方依赖。
- Web Token 只从环境变量 `KB_RAG_WEB_TOKEN` 读取，至少 16 个字符；API Key 只从 `KB_RAG_LLM_API_KEY` 读取。
- 除 `/api/health` 外，所有 `/api/*` 必须校验 `Authorization: Bearer <token>`，使用常量时间比较。
- 默认绑定 `127.0.0.1:8080`；局域网通过 `web.addr` 显式改为 `0.0.0.0:8080`。
- 上传允许 `.md` `.markdown` `.txt` `.docx` `.pdf`；默认上限 20 MB。
- 删除只删除 `documents` 与 `chunks` 索引，永不删除源文件。
- API 不返回服务器绝对路径；不记录问题原文、答案、请求体、Token 或 API Key。
- 前后端同源，不启用 CORS。
- 前端数据契约由本计划的 DTO 固定；前端计划依赖这些字段名。
- 所有新增/修改文档使用中文、UTF-8、LF。

## File Structure

```text
internal/config/config.go        + WebConfig、WebToken、serve 校验
internal/config/config_test.go
internal/store/store.go          + ListDocuments/GetDocumentByID/DeleteDocument
internal/store/store_test.go
internal/ingest/ingest.go        + 导出 FileResult/IngestFile
internal/ingest/ingest_test.go
internal/query/query.go          + AskWithTopK、错误哨兵
internal/query/query_test.go
internal/web/types.go            API DTO
internal/web/server.go           Server、认证、核心 API、静态文件
internal/web/documents.go        上传/删除/重试
internal/web/upload.go           文件名安全化、保存
internal/web/ask.go              问答 API
internal/web/server_test.go      核心 API 测试
internal/web/documents_test.go   上传/删除/重试测试
internal/web/ask_test.go         问答测试
internal/web/README.md
internal/cli/cli.go              + serve 分发
internal/cli/serve.go            组装依赖并启动 Web
config.example.yaml              + web 段
README.md                        （由前端计划 Task 4 更新 Web 使用说明）
```

---

### Task 1: 配置 Web 段与 `serve` 校验

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `config.example.yaml`

**Interfaces:**
- Produces: `config.Config.Web`、`config.Config.WebToken`、`Config.Validate("serve")`。
- Later tasks read `cfg.Web.Addr`、`cfg.Web.UploadDir`、`cfg.Web.MaxUploadMB`、`cfg.Web.FrontendDir`、`cfg.WebToken`。

- [ ] **Step 1: 写失败测试**

在 `internal/config/config_test.go` 追加：

```go
func TestLoadWebDefaultsAndToken(t *testing.T) {
	t.Setenv("KB_RAG_WEB_TOKEN", "0123456789abcdef")
	path := writeConfig(t, "llm:\n  base_url: http://localhost:9999\n  model: test-llm\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Web.Addr != "127.0.0.1:8080" {
		t.Errorf("web.addr = %q", cfg.Web.Addr)
	}
	if cfg.Web.UploadDir != "./data/uploads" {
		t.Errorf("web.upload_dir = %q", cfg.Web.UploadDir)
	}
	if cfg.Web.MaxUploadMB != 20 {
		t.Errorf("web.max_upload_mb = %d", cfg.Web.MaxUploadMB)
	}
	if cfg.Web.FrontendDir != "./fronted/dist" {
		t.Errorf("web.frontend_dir = %q", cfg.Web.FrontendDir)
	}
	if cfg.WebToken != "0123456789abcdef" {
		t.Errorf("WebToken = %q", cfg.WebToken)
	}
}

func TestValidateServe(t *testing.T) {
	dir := t.TempDir()
	frontend := filepath.Join(dir, "frontend")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontend, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := func() *Config {
		c := validConfig(dir)
		c.Web = WebConfig{
			Addr:        "127.0.0.1:8080",
			UploadDir:   filepath.Join(dir, "uploads"),
			MaxUploadMB: 20,
			FrontendDir: frontend,
		}
		c.WebToken = "0123456789abcdef"
		return c
	}
	if err := base().Validate("serve"); err != nil {
		t.Fatalf("合法 serve 配置被拒绝: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"bad addr", func(c *Config) { c.Web.Addr = "localhost" }},
		{"bad port", func(c *Config) { c.Web.Addr = "127.0.0.1:99999" }},
		{"zero upload limit", func(c *Config) { c.Web.MaxUploadMB = 0 }},
		{"missing frontend index", func(c *Config) { c.Web.FrontendDir = filepath.Join(dir, "missing") }},
		{"short token", func(c *Config) { c.WebToken = "short" }},
		{"missing api key", func(c *Config) { c.APIKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(cfg)
			if err := cfg.Validate("serve"); err == nil {
				t.Fatal("期望校验失败")
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/config -run 'TestLoadWebDefaultsAndToken|TestValidateServe' -v
```

Expected: FAIL（`undefined: WebConfig` 或 Web 字段不存在）。

- [ ] **Step 3: 实现配置扩展**

`internal/config/config.go` 增加导入：

```go
import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)
```

在 `Config` 中增加：

```go
	Web      WebConfig `yaml:"web"`
	WebToken string    `yaml:"-"`
```

在 `LogConfig` 后增加：

```go
type WebConfig struct {
	Addr        string `yaml:"addr"`
	UploadDir   string `yaml:"upload_dir"`
	MaxUploadMB int    `yaml:"max_upload_mb"`
	FrontendDir string `yaml:"frontend_dir"`
}
```

`defaults()` 增加：

```go
		Web: WebConfig{
			Addr:        "127.0.0.1:8080",
			UploadDir:   "./data/uploads",
			MaxUploadMB: 20,
			FrontendDir: "./fronted/dist",
		},
```

`Load` 在设置 `APIKey` 后增加：

```go
	cfg.WebToken = os.Getenv("KB_RAG_WEB_TOKEN")
```

`Validate` 把原 `if command == "ask"` 改为：

```go
	if command == "ask" || command == "serve" {
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
	if command == "serve" {
		return c.validateServe()
	}
	return nil
```

在 `validateURL` 后增加：

```go
func (c *Config) validateServe() error {
	if err := validateAddr(c.Web.Addr); err != nil {
		return err
	}
	if strings.TrimSpace(c.Web.UploadDir) == "" {
		return fmt.Errorf("web.upload_dir 不能为空")
	}
	if err := ensureDir(c.Web.UploadDir); err != nil {
		return err
	}
	if c.Web.MaxUploadMB <= 0 {
		return fmt.Errorf("web.max_upload_mb 必须大于 0")
	}
	if strings.TrimSpace(c.Web.FrontendDir) == "" {
		return fmt.Errorf("web.frontend_dir 不能为空")
	}
	index := filepath.Join(c.Web.FrontendDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		return fmt.Errorf("前端未构建：缺少 %s，请先在 fronted 执行 npm run build", index)
	}
	if len(c.WebToken) < 16 {
		return fmt.Errorf("环境变量 KB_RAG_WEB_TOKEN 至少需要 16 个字符")
	}
	return nil
}

func validateAddr(raw string) error {
	_, port, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("web.addr 必须是 host:port（当前为 %q）", raw)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("web.addr 端口非法（当前为 %q）", raw)
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/config -v
```

Expected: PASS。

- [ ] **Step 5: 更新 `config.example.yaml`**

在 `log` 段前增加：

```yaml
web:
  addr: 127.0.0.1:8080
  upload_dir: ./data/uploads
  max_upload_mb: 20
  frontend_dir: ./fronted/dist
```

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/config
go test ./internal/config
git add internal/config config.example.yaml
git commit -m "feat: 增加 Web 配置与 serve 校验"
```

---

### Task 2: Store 文档列表、按 ID 查询与删除

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`
- Modify: `internal/store/README.md`

**Interfaces:**
- Produces:
  - `store.ErrDocumentNotFound`
  - `store.DocumentSummary{ID, Path, Title, Status, Error, UpdatedAt, Chunks}`
  - `(*Store).ListDocuments(ctx) ([]DocumentSummary, error)`
  - `(*Store).GetDocumentByID(ctx, id) (DocumentInfo, bool, error)`
  - `(*Store).DeleteDocument(ctx, id) error`

- [ ] **Step 1: 写失败测试**

在 `internal/store/store_test.go` 的 import 中加入 `"errors"`，并追加：

```go
func TestListDocumentsAndDelete(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.ReplaceDocument(ctx, "/docs/a.md", "A", "h1", []ChunkInput{
		{Seq: 0, Text: "a1", Vector: []float32{1, 0, 0}},
		{Seq: 1, Text: "a2", Vector: []float32{0, 1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDocument(ctx, "/docs/b.md", "B", "h2", []ChunkInput{
		{Seq: 0, Text: "b1", Vector: []float32{0, 0, 1}},
	}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListDocuments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("documents = %+v", list)
	}
	if list[0].Path != "/docs/b.md" || list[0].Chunks != 1 {
		t.Errorf("list[0] = %+v", list[0])
	}
	if list[1].Path != "/docs/a.md" || list[1].Chunks != 2 {
		t.Errorf("list[1] = %+v", list[1])
	}

	info, ok, err := s.GetDocumentByID(ctx, list[1].ID)
	if err != nil || !ok || info.Path != "/docs/a.md" {
		t.Fatalf("GetDocumentByID = %+v, ok=%v, err=%v", info, ok, err)
	}

	if err := s.DeleteDocument(ctx, list[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.GetDocument("/docs/a.md"); err != nil || ok {
		t.Fatalf("删除后仍能查到文档: ok=%v err=%v", ok, err)
	}
	if err := s.DeleteDocument(ctx, list[1].ID); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("重复删除错误 = %v", err)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Chunks != 1 {
		t.Fatalf("级联删除后 chunks = %d", stats.Chunks)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/store -run TestListDocumentsAndDelete -v
```

Expected: FAIL（`undefined: ListDocuments`）。

- [ ] **Step 3: 实现 Store 方法**

在 `store.go` 的 `Stats` 类型后增加：

```go
type DocumentSummary struct {
	ID        int64
	Path      string
	Title     string
	Status    string
	Error     string
	UpdatedAt string
	Chunks    int
}

var ErrDocumentNotFound = errors.New("文档不存在")
```

在 `Stats` 方法后增加：

```go
func (s *Store) ListDocuments(ctx context.Context) ([]DocumentSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,d.path,d.title,d.status,d.error,d.updated_at,COUNT(c.id)
		FROM documents d LEFT JOIN chunks c ON c.doc_id=d.id
		GROUP BY d.id,d.path,d.title,d.status,d.error,d.updated_at
		ORDER BY d.updated_at DESC, d.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DocumentSummary
	for rows.Next() {
		var d DocumentSummary
		if err := rows.Scan(&d.ID, &d.Path, &d.Title, &d.Status, &d.Error, &d.UpdatedAt, &d.Chunks); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDocumentByID(ctx context.Context, id int64) (DocumentInfo, bool, error) {
	var info DocumentInfo
	err := s.db.QueryRowContext(ctx, `SELECT id,path,title,content_hash,status,error,updated_at
		FROM documents WHERE id=?`, id).
		Scan(&info.ID, &info.Path, &info.Title, &info.Hash, &info.Status, &info.Error, &info.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentInfo{}, false, nil
	}
	if err != nil {
		return DocumentInfo{}, false, err
	}
	return info, true, nil
}

func (s *Store) DeleteDocument(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDocumentNotFound
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/store -v
```

Expected: PASS（新增测试与原有测试都通过）。

- [ ] **Step 5: 更新模块文档**

在 `internal/store/README.md` 的“文档与片段”节追加：

```markdown
- `ListDocuments`：按 `updated_at DESC, id DESC` 返回文档摘要，并统计每个文档的片段数。
- `GetDocumentByID` / `DeleteDocument`：按 ID 查询与删除。删除只移除数据库记录，`chunks` 通过外键级联删除；源文件不受影响。
```

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/store
go test ./internal/store
git add internal/store
git commit -m "feat: 增加文档列表、按 ID 查询与删除"
```

---

### Task 3: 导出单文件入库 `IngestFile`

**Files:**
- Modify: `internal/ingest/ingest.go`
- Test: `internal/ingest/ingest_test.go`
- Modify: `internal/ingest/README.md`

**Interfaces:**
- Produces:
  - `ingest.FileResult{Outcome string; Chunks int; Err error; Fatal bool}`
  - `ingest.OutcomeOK`、`OutcomeSkipped`、`OutcomeFailed`
  - `ingest.IngestFile(ctx, path, st, emb, opt) FileResult`
- 调用方必须先调用 `Store.InitMeta`；Web 服务在启动时调用。

- [ ] **Step 1: 写失败测试**

在 `internal/ingest/ingest_test.go` 追加：

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/ingest -run TestIngestFile -v
```

Expected: FAIL（`undefined: IngestFile`）。

- [ ] **Step 3: 导出结果并增加单文件入口**

把 `internal/ingest/ingest.go` 的 `fileResult` 与常量替换为：

```go
type FileResult struct {
	Outcome string
	Chunks  int
	Err     error
	Fatal   bool
}

const (
	OutcomeOK      = "ok"
	OutcomeSkipped = "skipped"
	OutcomeFailed  = "failed"
)
```

`Run` 中所有 `res.outcome`、`res.chunks`、`res.err`、`res.fatal` 改为 `res.Outcome`、`res.Chunks`、`res.Err`、`res.Fatal`；switch 的常量改为 `OutcomeOK`、`OutcomeSkipped`、`OutcomeFailed`。

把 `processFile` 的签名与返回值改为：

```go
func processFile(ctx context.Context, st Store, emb Embedder, opt Options, path, rel string) FileResult {
```

函数内所有 `fileResult{outcome: ..., err: ..., fatal: ...}` 改为：

```go
FileResult{Outcome: ..., Err: ..., Fatal: ...}
```

在 `processFile` 前增加：

```go
// IngestFile 对单个文件执行与 Run 相同的解析、切分、嵌入与事务写入。
// 调用方必须先通过 Store.InitMeta 初始化模型与 chunk_size。
func IngestFile(ctx context.Context, path string, st Store, emb Embedder, opt Options) FileResult {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	return processFile(ctx, st, emb, opt, path, filepath.Base(path))
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/ingest -v
```

Expected: PASS（`TestIngestFile` 与原有 `Run` 测试都通过）。

- [ ] **Step 5: 更新模块文档**

在 `internal/ingest/README.md` 的“入口”节补充：

````markdown
单文件入口：

```go
type FileResult struct {
    Outcome string // ok / skipped / failed
    Chunks  int
    Err     error
    Fatal   bool
}

func IngestFile(ctx context.Context, path string, st Store, emb Embedder, opt Options) FileResult
```

调用前必须已执行 `Store.InitMeta`。Web 上传与重试使用该函数；`Run` 对目录扫描也复用同一内部逻辑。
````

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/ingest
go test ./internal/ingest
git add internal/ingest
git commit -m "feat: 导出单文件入库入口"
```

---

### Task 4: Query 支持按请求指定 top-k 与可识别错误

**Files:**
- Modify: `internal/query/query.go`
- Test: `internal/query/query_test.go`
- Modify: `internal/query/README.md`

**Interfaces:**
- Produces:
  - `query.ErrEmptyStore`、`query.ErrNoResults`、`query.ErrModelMismatch`
  - `(*Service).AskWithTopK(ctx, question string, topK int) (Result, error)`
  - `(*Service).Ask` 保持原签名，内部调用 `AskWithTopK(ctx, question, s.TopK)`。

- [ ] **Step 1: 写失败测试**

在 `internal/query/query_test.go` 给 `fakeStore` 增加字段：

```go
type fakeStore struct {
	meta store.EmbeddingMeta
	hits []store.Hit
	err  error
	gotTopK int
}

func (f *fakeStore) Search(ctx context.Context, query []float32, topK int) ([]store.Hit, error) {
	f.gotTopK = topK
	return f.hits, f.err
}
```

追加：

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/query -run 'TestAskWithTopK|TestAskSentinelErrors' -v
```

Expected: FAIL（`undefined: AskWithTopK` 等）。

- [ ] **Step 3: 实现 Query 改动**

`query.go` 增加 `errors` 导入，并在 `Service` 前增加：

```go
var (
	ErrEmptyStore    = errors.New("知识库为空")
	ErrNoResults     = errors.New("知识库中未找到相关片段")
	ErrModelMismatch = errors.New("嵌入模型不一致")
)
```

把 `Ask` 的开头改为：

```go
func (s *Service) Ask(ctx context.Context, question string) (Result, error) {
	return s.AskWithTopK(ctx, question, s.TopK)
}

func (s *Service) AskWithTopK(ctx context.Context, question string, topK int) (Result, error) {
```

在 `AskWithTopK` 内：

- 空库错误改为：

```go
		return Result{}, fmt.Errorf("%w，请先执行 ingest", ErrEmptyStore)
```

- 模型不一致改为：

```go
		return Result{}, fmt.Errorf("%w：库内为 %q，配置为 %q", ErrModelMismatch, meta.Model, s.EmbedModel)
```

- 删除原来的：

```go
	topK := s.TopK
	if topK <= 0 {
		topK = 8
	}
```

替换为：

```go
	if topK <= 0 {
		topK = 8
	}
```

- 无命中错误改为：

```go
		return Result{}, fmt.Errorf("%w，请先执行 ingest", ErrNoResults)
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/query -v
```

Expected: PASS（原有测试 + 新测试）。

- [ ] **Step 5: 更新模块文档**

在 `internal/query/README.md` 的“入口”节补充：

```markdown
- `Ask` 使用 Service 的固定 `TopK`，供 CLI 使用；
- `AskWithTopK(ctx, question, topK)` 允许按请求指定 top-k，供 Web API 使用；
- `ErrEmptyStore` / `ErrNoResults` / `ErrModelMismatch` 用于 API 区分 409 响应。
```

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/query
go test ./internal/query
git add internal/query
git commit -m "feat: 支持按请求指定 top-k 并暴露错误类型"
```

---

### Task 5: Web 服务核心（认证、统计、文档列表、静态文件）

**Files:**
- Create: `internal/web/types.go`
- Create: `internal/web/server.go`
- Test: `internal/web/server_test.go`

**Interfaces:**
- Produces:
  - `web.Options`、`web.New(opts) *Server`
  - `(*Server).Handler() http.Handler`
  - `(*Server).ListenAndServe(ctx, addr) error`
  - 核心 DTO：`statsDTO`、`documentDTO`、`listDocumentsResponse`、`errorResponse`
- 认证规则：`/api/health` 公开；其余 `/api/*` 需要 Bearer Token。

- [ ] **Step 1: 写失败测试**

创建 `internal/web/server_test.go`：

```go
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kb-rag/internal/llm"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

type fakeEmbedder struct{}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if strings.Contains(text, "年假") {
			out[i] = []float32{1, 0, 0}
		} else {
			out[i] = []float32{0, 1, 0}
		}
	}
	return out, nil
}

type fakeLLM struct {
	fail bool
}

func (f *fakeLLM) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	if f.fail {
		return "", errors.New("llm down")
	}
	return "员工满1年享5天年假[1]。", nil
}

func newTestServer(t *testing.T) (*Server, *fakeEmbedder, *fakeLLM, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.InitMeta("test-model", 100); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDim(3); err != nil {
		t.Fatal(err)
	}
	emb := &fakeEmbedder{}
	llmFake := &fakeLLM{}
	q := &query.Service{
		Embedder:   emb,
		Store:      st,
		LLM:        llmFake,
		EmbedModel: "test-model",
		TopK:       8,
		BaseDir:    dir,
	}
	frontend := filepath.Join(dir, "frontend")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontend, "index.html"), []byte("<html><body>kb-rag</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	uploads := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatal(err)
	}
	token := "0123456789abcdef"
	srv := New(Options{
		Store:          st,
		Embedder:       emb,
		Query:          q,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		UploadDir:      uploads,
		MaxUploadBytes: 1 << 20,
		FrontendDir:    frontend,
		Token:          token,
		IngestModel:    "test-model",
		ChunkSize:      100,
		Overlap:        20,
	})
	return srv, emb, llmFake, token
}

func call(t *testing.T, h http.Handler, method, path string, body []byte, token, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthIsPublic(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	rec := call(t, srv.Handler(), "GET", "/api/health", nil, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v", body)
	}
}

func TestAPIRequiresToken(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	if rec := call(t, srv.Handler(), "GET", "/api/session", nil, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("无令牌 status = %d", rec.Code)
	}
	if rec := call(t, srv.Handler(), "GET", "/api/session", nil, "wrong-token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误令牌 status = %d", rec.Code)
	}
	if rec := call(t, srv.Handler(), "GET", "/api/session", nil, token, ""); rec.Code != http.StatusOK {
		t.Fatalf("正确令牌 status = %d", rec.Code)
	}
}

func TestStatsAndEmptyDocuments(t *testing.T) {
	srv, _, _, token := newTestServer(t)

	rec := call(t, srv.Handler(), "GET", "/api/stats", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d", rec.Code)
	}
	var stats statsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Documents != 0 || stats.Chunks != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.Embedding.Model != "test-model" || stats.Embedding.Dim != 3 || stats.Embedding.ChunkSize != 100 {
		t.Fatalf("embedding = %+v", stats.Embedding)
	}

	rec = call(t, srv.Handler(), "GET", "/api/documents", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("documents status = %d", rec.Code)
	}
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 0 {
		t.Fatalf("documents = %+v", docs.Documents)
	}
}

func TestStaticIndex(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	rec := call(t, srv.Handler(), "GET", "/", nil, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("static status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "kb-rag") {
		t.Fatalf("static body = %q", rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/web -run 'TestHealth|TestAPIRequires|TestStats|TestStatic' -v
```

Expected: FAIL（`undefined: New` / `Server`）。

- [ ] **Step 3: 实现 DTO 与服务器**

创建 `internal/web/types.go`：

```go
package web

type embeddingDTO struct {
	Model     string `json:"model"`
	Dim       int    `json:"dim"`
	ChunkSize int    `json:"chunk_size"`
}

type statsDTO struct {
	Documents       int          `json:"documents"`
	DocumentsFailed int          `json:"documents_failed"`
	Chunks          int          `json:"chunks"`
	SizeBytes       int64        `json:"size_bytes"`
	Embedding       embeddingDTO `json:"embedding"`
}

type documentDTO struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Chunks    int    `json:"chunks"`
	Source    string `json:"source"`
	Error     string `json:"error"`
	UpdatedAt string `json:"updated_at"`
}

type listDocumentsResponse struct {
	Documents []documentDTO `json:"documents"`
}

type citationDTO struct {
	N       int     `json:"n"`
	Source  string  `json:"source"`
	Heading string  `json:"heading"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

type askRequest struct {
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
}

type askResponse struct {
	Answer    string        `json:"answer"`
	Generated bool          `json:"generated"`
	Warning   string        `json:"warning"`
	Citations []citationDTO `json:"citations"`
}

type uploadResponse struct {
	Document documentDTO `json:"document"`
	Outcome  string      `json:"outcome"`
}

type errorResponse struct {
	Error string `json:"error"`
}
```

创建 `internal/web/server.go`：

```go
package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kb-rag/internal/ingest"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Options struct {
	Store          *store.Store
	Embedder       Embedder
	Query          *query.Service
	Logger         *slog.Logger
	UploadDir      string
	MaxUploadBytes int64
	FrontendDir    string
	Token          string
	IngestModel    string
	ChunkSize      int
	Overlap        int
}

type Server struct {
	store          *store.Store
	embedder       Embedder
	query          *query.Service
	logger         *slog.Logger
	uploadDir      string
	maxUploadBytes int64
	frontendDir    string
	token          string
	ingestOptions  ingest.Options
	ingestMu       sync.Mutex
}

func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{
		store:          opts.Store,
		embedder:       opts.Embedder,
		query:          opts.Query,
		logger:         opts.Logger,
		uploadDir:      opts.UploadDir,
		maxUploadBytes: opts.MaxUploadBytes,
		frontendDir:    opts.FrontendDir,
		token:          opts.Token,
		ingestOptions: ingest.Options{
			Model:     opts.IngestModel,
			ChunkSize: opts.ChunkSize,
			Overlap:   opts.Overlap,
			Logger:    opts.Logger,
		},
	}
}

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /health", s.handleHealth)
	api.HandleFunc("GET /session", s.handleSession)
	api.HandleFunc("GET /stats", s.handleStats)
	api.HandleFunc("GET /documents", s.handleListDocuments)

	mux := http.NewServeMux()
	mux.Handle("/api/", s.withAuth(http.StripPrefix("/api", api)))
	mux.Handle("/", s.staticHandler())
	return s.withSecurityHeaders(s.withLogging(mux))
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      30 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "
		provided := ""
		if strings.HasPrefix(header, prefix) {
			provided = strings.TrimSpace(header[len(prefix):])
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		s.logger.Info("http 请求", "method", r.Method, "path", r.URL.Path, "status", sw.status, "bytes", sw.bytes, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServer(http.Dir(s.frontendDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
	if err != nil {
		s.logger.Error("读取统计失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取统计失败")
		return
	}
	writeJSON(w, http.StatusOK, statsDTO{
		Documents:       stats.Documents,
		DocumentsFailed: stats.DocumentsFailed,
		Chunks:          stats.Chunks,
		SizeBytes:       stats.SizeBytes,
		Embedding: embeddingDTO{
			Model:     stats.Meta.Model,
			Dim:       stats.Meta.Dim,
			ChunkSize: stats.Meta.ChunkSize,
		},
	})
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDocuments(r.Context())
	if err != nil {
		s.logger.Error("读取文档列表失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取文档列表失败")
		return
	}
	out := make([]documentDTO, 0, len(list))
	for _, d := range list {
		out = append(out, s.documentDTO(d))
	}
	writeJSON(w, http.StatusOK, listDocumentsResponse{Documents: out})
}

func (s *Server) documentDTO(d store.DocumentSummary) documentDTO {
	return documentDTO{
		ID:        d.ID,
		Title:     d.Title,
		Name:      filepath.Base(d.Path),
		Status:    d.Status,
		Chunks:    d.Chunks,
		Source:    s.sourceOf(d.Path),
		Error:     d.Error,
		UpdatedAt: d.UpdatedAt,
	}
}

func (s *Server) sourceOf(path string) string {
	if s.isManagedPath(path) {
		return "upload"
	}
	return "external"
}

func (s *Server) isManagedPath(path string) bool {
	root, err := filepath.Abs(s.uploadDir)
	if err != nil {
		return false
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func (s *Server) summaryByID(ctx context.Context, id int64) (store.DocumentSummary, bool, error) {
	list, err := s.store.ListDocuments(ctx)
	if err != nil {
		return store.DocumentSummary{}, false, err
	}
	for _, d := range list {
		if d.ID == id {
			return d, true, nil
		}
	}
	return store.DocumentSummary{}, false, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
```

- [ ] **Step 4: 运行核心测试确认通过**

```bash
go test ./internal/web -run 'TestHealth|TestAPIRequires|TestStats|TestStatic' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
gofmt -w internal/web
go test ./internal/web
git add internal/web
git commit -m "feat: 增加 Web API 核心与令牌认证"
```

---

### Task 6: 上传文档并同步入库

**Files:**
- Create: `internal/web/upload.go`
- Create: `internal/web/documents.go`
- Modify: `internal/web/server.go`（注册路由）
- Test: `internal/web/documents_test.go`

**Interfaces:**
- Consumes: `ingest.IngestFile`、`loader.Supported`、`store.GetDocument`、`Server.documentDTO`。
- Produces: `POST /api/documents`，返回 `uploadResponse`。

- [ ] **Step 1: 写失败测试**

创建 `internal/web/documents_test.go`：

```go
package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func multipartBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &b, mw.FormDataContentType()
}

func uploadDoc(t *testing.T, srv *Server, token, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, "file", filename, content)
	return call(t, srv.Handler(), "POST", "/api/documents", body.Bytes(), token, ct)
}

func uploadedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUploadAndList(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年不满10年的，年休假5天。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.Outcome != "ok" || up.Document.Status != "ok" || up.Document.Chunks == 0 {
		t.Fatalf("upload = %+v", up)
	}
	if up.Document.Name != "年假.md" || up.Document.Source != "upload" {
		t.Fatalf("document = %+v", up.Document)
	}

	rec = call(t, srv.Handler(), "GET", "/api/documents", nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 1 || docs.Documents[0].ID != up.Document.ID {
		t.Fatalf("documents = %+v", docs.Documents)
	}
}

func TestUploadRejectsUnsupportedType(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "image.png", []byte("not an image"))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUploadRejectsTooLarge(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	content := bytes.Repeat([]byte("a"), 2<<20)
	rec := uploadDoc(t, srv, token, "big.md", content)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUploadSanitizesFilename(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "../../evil.md", []byte("# 标题\n\n一段足够长的正文内容。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.Document.Name != "evil.md" {
		t.Fatalf("name = %q", up.Document.Name)
	}
	files := uploadedFiles(t, srv.uploadDir)
	if len(files) != 1 {
		t.Fatalf("uploaded files = %v", files)
	}
	rel, err := filepath.Rel(srv.uploadDir, files[0])
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("文件逃逸上传目录: %s rel=%s err=%v", files[0], rel, err)
	}
}

func TestSafeUploadName(t *testing.T) {
	cases := map[string]string{
		"../../evil.md": "evil.md",
		"CON.md":        "_CON.md",
		"a*b?.md":       "a_b_.md",
		"":              "upload",
		"..":            "upload",
	}
	for in, want := range cases {
		if got := safeUploadName(in); got != want {
			t.Errorf("safeUploadName(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/web -run 'TestUpload|TestSafeUploadName' -v
```

Expected: FAIL（`undefined: safeUploadName` / 上传路由不存在）。

- [ ] **Step 3: 实现文件名安全化与保存**

创建 `internal/web/upload.go`：

```go
package web

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func safeUploadName(name string) string {
	base := filepath.Base(name)
	base = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		if r < 32 {
			return '_'
		}
		return r
	}, base)
	base = strings.TrimSpace(base)
	base = strings.Trim(base, ". ")
	if base == "" {
		return "upload"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "upload"
	}
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		stem = "_" + stem
	}
	runes := []rune(stem)
	if len(runes) > 100 {
		stem = string(runes[:100])
	}
	return stem + ext
}

func (s *Server) saveUpload(src io.Reader, original string) (string, error) {
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(s.uploadDir, "upload-")
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, safeUploadName(original))
	f, err := os.Create(target)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		_ = os.RemoveAll(dir)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return target, nil
}
```

- [ ] **Step 4: 实现上传 handler**

创建 `internal/web/documents.go`：

```go
package web

import (
	"errors"
	"net/http"

	"kb-rag/internal/ingest"
	"kb-rag/internal/ingest/loader"
)

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
			return
		}
		writeError(w, http.StatusBadRequest, "上传表单格式错误")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "缺少文件字段 file")
		return
	}
	defer file.Close()

	if !loader.Supported(header.Filename) {
		writeError(w, http.StatusUnsupportedMediaType, "不支持的文件类型")
		return
	}
	if header.Size <= 0 {
		writeError(w, http.StatusBadRequest, "文件为空")
		return
	}
	if header.Size > s.maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
		return
	}

	target, err := s.saveUpload(file, header.Filename)
	if err != nil {
		s.logger.Error("保存上传文件失败", "error", err)
		writeError(w, http.StatusInternalServerError, "保存上传文件失败")
		return
	}

	s.ingestMu.Lock()
	res := ingest.IngestFile(r.Context(), target, s.store, s.embedder, s.ingestOptions)
	s.ingestMu.Unlock()
	if res.Fatal {
		s.logger.Error("上传入库失败", "error", res.Err)
		writeError(w, http.StatusInternalServerError, "入库失败")
		return
	}

	info, ok, err := s.store.GetDocument(target)
	if err != nil || !ok {
		s.logger.Error("读取上传文档失败", "error", err, "path", target)
		writeError(w, http.StatusInternalServerError, "读取上传文档失败")
		return
	}
	summary, ok, err := s.summaryByID(r.Context(), info.ID)
	if err != nil || !ok {
		s.logger.Error("读取上传文档摘要失败", "error", err, "id", info.ID)
		writeError(w, http.StatusInternalServerError, "读取上传文档失败")
		return
	}

	writeJSON(w, http.StatusCreated, uploadResponse{
		Document: s.documentDTO(summary),
		Outcome:  res.Outcome,
	})
}
```

- [ ] **Step 5: 在 `server.go` 注册上传路由**

在 `Handler()` 的 `GET /documents` 后增加：

```go
	api.HandleFunc("POST /documents", s.handleUploadDocument)
```

- [ ] **Step 6: 运行测试确认通过**

```bash
go test ./internal/web -v
```

Expected: PASS（核心测试 + 上传测试）。

- [ ] **Step 7: 提交**

```bash
gofmt -w internal/web
go test ./internal/web
git add internal/web
git commit -m "feat: 支持上传文档并同步入库"
```

---

### Task 7: 删除索引与重试文档

**Files:**
- Modify: `internal/web/documents.go`
- Modify: `internal/web/server.go`（注册路由）
- Test: `internal/web/documents_test.go`

**Interfaces:**
- Consumes: `store.GetDocumentByID`、`store.DeleteDocument`、`store.ErrDocumentNotFound`、`ingest.IngestFile`。
- Produces: `DELETE /api/documents/{id}`、`POST /api/documents/{id}/reingest`。

- [ ] **Step 1: 写失败测试**

在 `internal/web/documents_test.go` 的 import 中加入 `"fmt"` 与 `"os"`，并追加：

```go
func TestDeleteKeepsSourceFile(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	filesBefore := uploadedFiles(t, srv.uploadDir)
	if len(filesBefore) != 1 {
		t.Fatalf("files before = %v", filesBefore)
	}

	rec = call(t, srv.Handler(), "DELETE", fmt.Sprintf("/api/documents/%d", up.Document.ID), nil, token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = call(t, srv.Handler(), "GET", "/api/documents", nil, token, "")
	var docs listDocumentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 0 {
		t.Fatalf("删除后文档 = %+v", docs.Documents)
	}
	if _, err := os.Stat(filesBefore[0]); err != nil {
		t.Fatalf("源文件被删除: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := call(t, srv.Handler(), "DELETE", "/api/documents/999", nil, token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReingestSkipped(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}

	rec = call(t, srv.Handler(), "POST", fmt.Sprintf("/api/documents/%d/reingest", up.Document.ID), nil, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reingest status = %d body=%s", rec.Code, rec.Body.String())
	}
	var again uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again.Outcome != "skipped" || again.Document.Chunks == 0 {
		t.Fatalf("reingest = %+v", again)
	}
}

func TestReingestMissingSource(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	files := uploadedFiles(t, srv.uploadDir)
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}

	rec = call(t, srv.Handler(), "POST", fmt.Sprintf("/api/documents/%d/reingest", up.Document.ID), nil, token, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/web -run 'TestDelete|TestReingest' -v
```

Expected: FAIL（路由不存在，返回 404/405）。

- [ ] **Step 3: 实现删除与重试**

在 `internal/web/documents.go` 增加导入：

```go
import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"kb-rag/internal/ingest"
	"kb-rag/internal/ingest/loader"
	"kb-rag/internal/store"
)
```

在文件末尾增加：

```go
func parseDocumentID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	if err := s.store.DeleteDocument(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "文档不存在")
			return
		}
		s.logger.Error("删除文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "删除文档失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReingestDocument(w http.ResponseWriter, r *http.Request) {
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	info, ok, err := s.store.GetDocumentByID(r.Context(), id)
	if err != nil {
		s.logger.Error("读取文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取文档失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	}
	if _, err := os.Stat(info.Path); err != nil {
		writeError(w, http.StatusConflict, "源文件不存在，无法重试")
		return
	}

	s.ingestMu.Lock()
	res := ingest.IngestFile(r.Context(), info.Path, s.store, s.embedder, s.ingestOptions)
	s.ingestMu.Unlock()
	if res.Fatal {
		s.logger.Error("重试入库失败", "error", res.Err, "id", id)
		writeError(w, http.StatusInternalServerError, "重试入库失败")
		return
	}

	summary, ok, err := s.summaryByID(r.Context(), id)
	if err != nil || !ok {
		s.logger.Error("读取重试结果失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取重试结果失败")
		return
	}
	writeJSON(w, http.StatusOK, uploadResponse{
		Document: s.documentDTO(summary),
		Outcome:  res.Outcome,
	})
}
```

- [ ] **Step 4: 注册路由**

在 `server.go` 的 `Handler()` 中增加：

```go
	api.HandleFunc("DELETE /documents/{id}", s.handleDeleteDocument)
	api.HandleFunc("POST /documents/{id}/reingest", s.handleReingestDocument)
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/web -v
```

Expected: PASS（上传、删除、重试与核心测试）。

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/web
go test ./internal/web
git add internal/web
git commit -m "feat: 支持删除索引与重试文档"
```

---

### Task 8: 问答 API

**Files:**
- Create: `internal/web/ask.go`
- Modify: `internal/web/server.go`（注册路由）
- Test: `internal/web/ask_test.go`

**Interfaces:**
- Consumes: `query.AskWithTopK`、`query.ErrEmptyStore`、`query.ErrNoResults`、`query.ErrModelMismatch`。
- Produces: `POST /api/ask`，返回 `askResponse`。

- [ ] **Step 1: 写失败测试**

创建 `internal/web/ask_test.go`：

```go
package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAsk(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年不满10年的，年休假5天。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}

	body := []byte(`{"question":"年假怎么请","top_k":8}`)
	rec = call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Generated || resp.Answer == "" || len(resp.Citations) == 0 {
		t.Fatalf("ask = %+v", resp)
	}
	if resp.Citations[0].N != 1 || resp.Citations[0].Source != "年假.md" {
		t.Fatalf("citation = %+v", resp.Citations[0])
	}
}

func TestAskGenerationFailureReturnsCitations(t *testing.T) {
	srv, _, llmFake, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	llmFake.fail = true

	body := []byte(`{"question":"年假怎么请"}`)
	rec = call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Generated || resp.Warning == "" || len(resp.Citations) == 0 {
		t.Fatalf("ask = %+v", resp)
	}
}

func TestAskEmptyKnowledgeBase(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	body := []byte(`{"question":"年假怎么请"}`)
	rec := call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAskValidation(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	cases := [][]byte{
		[]byte(`{"question":"  "}`),
		[]byte(`{"question":"q","top_k":21}`),
		[]byte(`not-json`),
	}
	for _, body := range cases {
		rec := call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d", body, rec.Code)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/web -run TestAsk -v
```

Expected: FAIL（`/api/ask` 路由不存在，返回 404/405）。

- [ ] **Step 3: 实现问答 handler**

创建 `internal/web/ask.go`：

```go
package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req askRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "问题不能为空")
		return
	}
	topK := req.TopK
	if topK == 0 {
		topK = 8
	}
	if topK < 1 || topK > 20 {
		writeError(w, http.StatusBadRequest, "top_k 必须在 1 到 20 之间")
		return
	}

	res, err := s.query.AskWithTopK(r.Context(), question, topK)
	citations := citationsFromHits(res.Hits)
	if err != nil {
		if len(citations) > 0 {
			writeJSON(w, http.StatusOK, askResponse{
				Generated: false,
				Warning:   err.Error(),
				Citations: citations,
			})
			return
		}
		if errors.Is(err, query.ErrEmptyStore) || errors.Is(err, query.ErrNoResults) || errors.Is(err, query.ErrModelMismatch) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.logger.Error("问答失败", "error", err)
		writeError(w, http.StatusBadGateway, "问答服务暂时不可用")
		return
	}
	writeJSON(w, http.StatusOK, askResponse{
		Answer:    res.Answer,
		Generated: true,
		Citations: citations,
	})
}

func citationsFromHits(hits []store.Hit) []citationDTO {
	out := make([]citationDTO, 0, len(hits))
	for i, hit := range hits {
		out = append(out, citationDTO{
			N:       i + 1,
			Source:  filepath.Base(hit.Path),
			Heading: hit.Heading,
			Text:    hit.Text,
			Score:   hit.Score,
		})
	}
	return out
}
```

- [ ] **Step 4: 注册路由**

在 `server.go` 的 `Handler()` 中增加：

```go
	api.HandleFunc("POST /ask", s.handleAsk)
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/web -v
```

Expected: PASS（上传、删除、重试、问答与核心测试）。

- [ ] **Step 6: 提交**

```bash
gofmt -w internal/web
go test ./internal/web
git add internal/web
git commit -m "feat: 增加 Web 问答 API"
```

---

### Task 9: CLI `serve` 命令与模块文档

**Files:**
- Modify: `internal/cli/cli.go`
- Create: `internal/cli/serve.go`
- Create: `internal/web/README.md`

**Interfaces:**
- Consumes: `web.New`、`web.Options`、`(*Server).ListenAndServe`、`config.Validate("serve")`。
- Produces: `KB-RAG serve [--config 路径]`。

- [ ] **Step 1: 增加子命令分发**

在 `internal/cli/cli.go` 的 `Run` switch 中增加：

```go
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
```

在 `usage` 中增加：

```go
	fmt.Fprintln(w, "  KB-RAG serve [--config 路径]")
```

- [ ] **Step 2: 实现 `runServe`**

创建 `internal/cli/serve.go`：

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"kb-rag/internal/config"
	"kb-rag/internal/ingest/embedder"
	"kb-rag/internal/llm"
	"kb-rag/internal/logging"
	"kb-rag/internal/query"
	"kb-rag/internal/store"
	"kb-rag/internal/web"
)

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, cfgPath := newFlagSet("serve", stderr, "用法: KB-RAG serve [--config 路径]")
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
	if err := cfg.Validate("serve"); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return exitConfig
	}

	logger, closer, err := logging.New(cfg.Log, "serve", stderr)
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
	if err := st.InitMeta(cfg.Embedding.Model, cfg.Chunk.Size); err != nil {
		logger.Error("知识库初始化失败", "error", err)
		return exitRuntime
	}

	baseDir, err := os.Getwd()
	if err != nil {
		logger.Error("读取当前目录失败", "error", err)
		return exitRuntime
	}
	emb := embedder.New(cfg.Embedding.BaseURL, cfg.Embedding.Model, cfg.Embedding.BatchSize)
	service := &query.Service{
		Embedder:   emb,
		Store:      st,
		LLM:        llm.New(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.APIKey),
		EmbedModel: cfg.Embedding.Model,
		TopK:       8,
		BaseDir:    baseDir,
		Logger:     logger,
	}
	srv := web.New(web.Options{
		Store:          st,
		Embedder:       emb,
		Query:          service,
		Logger:         logger,
		UploadDir:      cfg.Web.UploadDir,
		MaxUploadBytes: int64(cfg.Web.MaxUploadMB) << 20,
		FrontendDir:    cfg.Web.FrontendDir,
		Token:          cfg.WebToken,
		IngestModel:    cfg.Embedding.Model,
		ChunkSize:      cfg.Chunk.Size,
		Overlap:        cfg.Chunk.Overlap,
	})

	fmt.Fprintf(stdout, "Web 服务已启动：http://%s（Ctrl+C 停止）\n", cfg.Web.Addr)
	if err := srv.ListenAndServe(ctx, cfg.Web.Addr); err != nil {
		if interrupted(ctx) {
			return exitInterrupted
		}
		logger.Error("Web 服务退出", "error", err)
		return exitRuntime
	}
	return exitOK
}
```

- [ ] **Step 3: 写 Web 模块文档**

创建 `internal/web/README.md`：

````markdown
# internal/web

KB-RAG 的 HTTP API 与静态文件服务，由 `KB-RAG serve` 启动。

## 认证

除 `GET /api/health` 外，所有 API 要求：

```http
Authorization: Bearer <KB_RAG_WEB_TOKEN>
```

令牌只从环境变量读取，至少 16 个字符；使用常量时间比较，不写日志。

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 公开健康检查 |
| GET | `/api/session` | 校验令牌 |
| GET | `/api/stats` | 文档/片段/模型/库大小 |
| GET | `/api/documents` | 文档列表 |
| POST | `/api/documents` | multipart 字段 `file`，上传并入库 |
| DELETE | `/api/documents/{id}` | 只删索引，源文件保留 |
| POST | `/api/documents/{id}/reingest` | 重试/重新入库 |
| POST | `/api/ask` | `{"question":"...","top_k":8}` |

## 上传

- 允许 `.md` `.markdown` `.txt` `.docx` `.pdf`；
- 保存到 `web.upload_dir/<随机目录>/<安全文件名>`；
- 同步入库；解析/嵌入失败时文档标记 `failed`，文件保留，可重试；
- 删除文档不删除源文件。

## 测试

`server_test.go`、`documents_test.go`、`ask_test.go` 使用临时 SQLite、临时上传目录、假嵌入与假 LLM 覆盖认证、上传、列表、删除、重试与问答。
````

- [ ] **Step 4: 运行全量 Go 测试与构建**

```bash
gofmt -w internal/cli internal/web
go vet ./...
go test ./...
go build -o KB-RAG.exe ./cmd/KB-RAG
```

Expected: 全部 PASS；构建成功。

- [ ] **Step 5: 提交**

```bash
git add internal/cli internal/web config.example.yaml
git commit -m "feat: 增加 serve 命令与 Web API 文档"
```

---

### Task 10: 后端最终验证

**Files:**
- 无新增文件；确认后端构建、测试与配置文档。

- [ ] **Step 1: 全量验证**

```bash
gofmt -l .
go vet ./...
go test ./...
go build -o KB-RAG.exe ./cmd/KB-RAG
```

Expected:
- `gofmt -l .` 无输出；
- `go vet` 无输出；
- `go test ./...` 全部 PASS；
- 构建成功。

- [ ] **Step 2: 检查接口契约**

确认以下文件中的 JSON 字段与设计文档一致：

- `internal/web/types.go`：`documents`、`documents_failed`、`chunks`、`size_bytes`、`embedding`、`documents`、`id`、`title`、`name`、`status`、`chunks`、`source`、`error`、`updated_at`、`answer`、`generated`、`warning`、`citations`、`n`、`text`、`score`；
- `POST /api/documents` 使用 multipart 字段 `file`；
- `/api/ask` 接受 `question` 与 `top_k`。

- [ ] **Step 3: 记录后端完成状态**

在 `internal/web/README.md` 末尾追加：

```markdown
## 状态

- 后端 API、认证、上传、列表、删除、重试、问答已实现；
- 前端构建与手工冒烟在 `docs/superpowers/plans/2026-10-26-KB-RAG-web-frontend.md` 中执行。
```

- [ ] **Step 4: 提交**

```bash
git add internal/web/README.md
git commit -m "docs: 记录 Web 后端完成状态"
```

---

## 自检

**Spec coverage**

| 设计文档要求 | 覆盖任务 |
|--------------|----------|
| `serve` 命令、配置校验 | Task 1、Task 9 |
| 共享令牌认证 | Task 1、Task 5 |
| stats / documents / ask API | Task 5、Task 8 |
| 上传、入库、失败重试 | Task 3、Task 6、Task 7 |
| 删除只删索引 | Task 2、Task 7 |
| 静态文件托管、缓存与安全头 | Task 5 |
| 单文件入库复用 | Task 3 |
| 按请求 top-k | Task 4、Task 8 |
| 错误码 401/404/409/413/415/502 | Task 5-8 |
| Go API 测试 | Task 5-8、Task 10 |
| 前端构建与联调 | 独立计划 `2026-10-26-KB-RAG-web-frontend.md` |

**Placeholder scan:** 无 TBD/TODO/“稍后实现”；所有代码步骤给出完整实现。

**Type consistency:** `store.DocumentSummary`、`store.ErrDocumentNotFound`、`ingest.FileResult`、`ingest.IngestFile`、`query.AskWithTopK`、`query.ErrEmptyStore/ErrNoResults/ErrModelMismatch`、`web.Options/Server` 在前后任务中使用一致。



# KB-RAG Web 界面设计文档

> **后续变更（2026-10-26）**：本文记录的共享令牌认证已被账号体系取代。当前实现支持用户名+密码登录、租户隔离、admin/editor/viewer 角色与审计日志，详见根 [README](../../../README.md) 与 [internal/web/README.md](../../../internal/web/README.md)。本文其余部分保留为当时的设计记录。

- 日期：2026-10-26
- 状态：已确认，待实施
- 范围：在现有 CLI 之上新增本地/局域网 Web 界面
- 前端目录：`fronted/`（沿用用户指定名称）
- 技术栈：Go `net/http` + Vue 3 + TypeScript + Vite + Element Plus

## 1. 背景与目标

第一期 CLI 已实现 `ingest`、`ask`、`stats`，并通过真实 Ollama、云端 OpenAI 兼容网关与 30 条人工评测验证。本期在 CLI 之上增加浏览器界面，让使用者可以：

1. 在浏览器中提问，查看答案与可展开的引用片段；
2. 查看知识库统计；
3. 上传文档并入库；
4. 查看文档状态、重试失败文档、删除索引。

Web 与 CLI 共用同一套配置、SQLite 库和业务包，不复制 RAG 逻辑。

## 2. 范围

### 2.1 做

- 新增 `KB-RAG serve` 子命令，提供 HTTP API 并托管 `fronted/dist`；
- Vue 3 SPA：登录、问答、知识库（统计/上传/文档管理）；
- 局域网共享令牌认证；
- 上传 Markdown/TXT/DOCX/PDF，保存到托管上传目录，同步入库；
- 文档列表、失败重试、只删索引的删除；
- 配置、README、测试与本地冒烟。

### 2.2 不做

- 用户账号、角色、多租户、审计日志；
- HTTPS 终止（需要时放在反向代理后）；
- 流式输出、多轮 LLM 上下文、对话持久化；
- 目录扫描入库（继续使用 CLI `ingest`）；
- 异步任务队列、后台重建索引；
- 上传文件自动清理 / 回收站；
- 文档预览、片段编辑、OCR；
- 对外开放 API 的 CORS。

## 3. 总体架构

```text
浏览器 (Vue 3 SPA)
   │  /api/* + Authorization: Bearer <web-token>
   ▼
KB-RAG serve (Go net/http)
   ├─ 认证中间件
   ├─ /api/ask      → query.Service → Ollama / LLM
   ├─ /api/documents→ ingest / store
   ├─ /api/stats    → store
   └─ /               → frontend_dir 静态文件
```

- 前端与 API 同源部署，生产环境不需要 CORS。
- Web 与 CLI 共用同一个 `data/KB-RAG.db`；Web 内部串行化入库，避免多个上传同时争抢 CPU。
- `fronted/dist` 由 Go 从磁盘读取；前端可独立构建和更新，不嵌入 Go 二进制。
- 前端开发时由 Vite 提供 HMR，Vite 代理 `/api` 到 `127.0.0.1:8080`。

## 4. 后端设计

### 4.1 `serve` 命令

`KB-RAG serve [--config 路径]`：

1. 加载并校验配置（embedding、chunk、store、log、llm、web、API Key、Web Token）；
2. 打开 Store，调用 `InitMeta` 校验模型/chunk_size；
3. 创建 Embedder、Query Service；
4. 校验 `frontend_dir/index.html` 存在；
5. 启动 HTTP 服务，直到 `ctx` 取消（Ctrl+C），优雅关闭。

`serve` 阻塞运行。API Key 与 Web Token 只从环境变量读取，不写入 YAML、不打印、不落日志。

### 4.2 API

所有响应为 JSON（静态文件除外），错误响应为：

```json
{"error": "面向用户的中文错误信息"}
```

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/health` | 否 | `{"status":"ok"}` |
| GET | `/api/session` | 是 | `{"ok":true}`，登录校验 |
| GET | `/api/stats` | 是 | 统计信息 |
| GET | `/api/documents` | 是 | 文档列表 |
| POST | `/api/documents` | 是 | multipart 上传并入库 |
| DELETE | `/api/documents/{id}` | 是 | 只删索引 |
| POST | `/api/documents/{id}/reingest` | 是 | 重试/重新入库 |
| POST | `/api/ask` | 是 | 问答 |

#### `GET /api/stats`

```json
{
  "documents": 2,
  "documents_failed": 0,
  "chunks": 109,
  "size_bytes": 614400,
  "embedding": {
    "model": "bge-m3",
    "dim": 1024,
    "chunk_size": 500
  }
}
```

#### `GET /api/documents`

```json
{
  "documents": [
    {
      "id": 1,
      "title": "大动环架构总览",
      "name": "ARCHITECTURE.md",
      "status": "ok",
      "chunks": 59,
      "source": "upload",
      "error": "",
      "updated_at": "2026-10-26T10:00:00Z"
    }
  ]
}
```

- `title` 来自 Loader（Markdown/DOCX 首个一级标题；TXT/PDF 文件名）；
- `name` 是源文件名；
- `source` 为 `upload`（位于托管上传目录）或 `external`（CLI 从其他路径入库）；
- 不返回服务器绝对路径。

#### `POST /api/documents`

- `multipart/form-data`，字段名 `file`，每次一个文件；前端多文件时串行调用。
- 允许扩展名：`.md`、`.markdown`、`.txt`、`.docx`、`.pdf`（大小写不敏感）。
- 大小上限由 `web.max_upload_mb` 控制，默认 20 MB。
- 成功（无论文档最终状态）返回 201：

```json
{
  "document": {
    "id": 3,
    "title": "休假制度",
    "name": "年假.md",
    "status": "ok",
    "chunks": 2,
    "source": "upload",
    "error": "",
    "updated_at": "2026-10-26T10:00:00Z"
  },
  "outcome": "ok"
}
```

- `outcome`：`ok` / `failed`；解析或嵌入失败时文档仍会保存为 `failed`，上传文件保留，返回 201 并在 `document.error` 说明原因，前端可重试。
- 未通过类型/大小校验时返回 415/413，不保存文件。

#### `DELETE /api/documents/{id}`

- 删除 `documents` 记录，`chunks` 依赖外键级联删除；
- **不删除任何源文件**，包括 Web 上传的文件；
- 成功 204；不存在 404。

#### `POST /api/documents/{id}/reingest`

- 重新对源路径执行单文件入库；
- 已是最新（hash 未变且状态 ok）时返回 `outcome: "skipped"`；
- 失败文档重试成功后变为 `ok`；
- 源文件不存在时返回 409。

#### `POST /api/ask`

请求：

```json
{"question": "告警规则是如何产生的？", "top_k": 8}
```

- `question` 去首尾空白后不能为空；
- `top_k` 可选，默认 8，允许 1–20。

成功：

```json
{
  "answer": "……[1]",
  "generated": true,
  "warning": "",
  "citations": [
    {
      "n": 1,
      "source": "告警规则与告警产生机制.md",
      "heading": "5. 告警事件发出后",
      "text": "……",
      "score": 0.72
    }
  ]
}
```

- `citations` 与答案中的 `[n]` 编号一致；
- `text` 是检索片段原文，供前端展开查看；
- 生成失败但检索成功：返回 200，`generated=false`，`answer` 为空，`warning` 说明生成失败，`citations` 仍然返回；
- 受控错误使用 4xx/5xx，不泄露内部路径与堆栈。

### 4.3 认证

- 共享令牌只从环境变量 `KB_RAG_WEB_TOKEN` 读取，要求至少 16 个字符；
- 除 `/api/health` 外，所有 `/api/*` 要求：

```http
Authorization: Bearer <token>
```

- 使用 `crypto/subtle.ConstantTimeCompare` 比较；
- 失败返回 401 `{"error":"未认证"}`；不区分“缺少令牌”和“令牌错误”；
- 前端把令牌保存在 `sessionStorage`，关闭标签页即失效；登录页通过 `GET /api/session` 校验；
- 不实现用户体系；所有使用者共享同一个令牌；
- 不启用 CORS；状态变更接口依赖 Authorization 头，降低简单 CSRF 风险；
- 令牌不写入日志、不回显到页面。

### 4.4 上传与文件存储

- 上传根目录：`web.upload_dir`，默认 `./data/uploads`。
- 每个文件保存为：

```text
<upload_dir>/<随机 16 字节 hex>/<安全化后的原文件名>
```

- 原文件名只保留 basename；去除路径分隔符、控制字符和 Windows 保留名风险；保留扩展名。
- 随机目录避免同名覆盖；同一文件重复上传会生成独立文档。
- 文件先写入磁盘，再调用单文件入库；入库失败时保留文件，文档标记 `failed`。
- 上传接口用互斥锁串行执行入库，避免多个大文档同时占用 CPU。
- 上传请求受 `http.MaxBytesReader` 限制；不信任客户端 Content-Type，按扩展名选择 Loader。

### 4.5 存储与入库改动

Store 新增：

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

func (s *Store) ListDocuments(ctx context.Context) ([]DocumentSummary, error)
func (s *Store) GetDocumentByID(ctx context.Context, id int64) (DocumentInfo, bool, error)
func (s *Store) DeleteDocument(ctx context.Context, id int64) error
```

- `DeleteDocument` 删除 `documents` 行；`chunks` 通过 `ON DELETE CASCADE` 删除；
- 列表按 `updated_at DESC, id DESC` 排序；
- 源文件路径仅在后端使用，不返回给前端。

Ingest 新增可复用单文件函数：

```go
type FileResult struct {
    Outcome string // ok / skipped / failed
    Chunks  int
    Err     error
}

func IngestFile(ctx context.Context, path string, st Store, emb Embedder, opt Options) FileResult
```

- `ingest.Run` 与 Web 上传/重试共用该函数；
- 服务启动时调用 `Store.InitMeta`；单文件函数不重复初始化元信息；
- 失败文件保留旧片段（如果存在）并标记 `failed`，重试逻辑与 CLI 一致。

### 4.6 错误码

| 状态码 | 场景 |
|--------|------|
| 400 | JSON 格式错误、问题为空、top_k 越界 |
| 401 | 缺少或错误的 Bearer Token |
| 404 | 文档 ID 不存在 |
| 409 | 重试时源文件不存在、知识库为空 |
| 413 | 上传超过大小上限 |
| 415 | 不支持的文件类型 |
| 500 | 存储或内部错误 |
| 502 | 嵌入/生成服务不可用且没有可用的检索结果 |

### 4.7 服务实现边界

新增包 `internal/web`：

- `Server` 持有 Store、Embedder、Query Service、入库配置、上传目录、前端目录、令牌、日志；
- `Handler()` 返回 `http.Handler`，便于 httptest；
- `ListenAndServe(ctx)` 创建 `http.Server`，设置 `ReadHeaderTimeout=10s`、`IdleTimeout=120s`、`WriteTimeout=30m`，在 ctx 取消时 `Shutdown`；
- 静态文件：`http.FileServer` 服务 `frontend_dir`；`/assets/` 长缓存，`index.html` 不缓存；设置 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`；
- 前端使用 hash 路由，不需要 SPA fallback；
- 请求日志只记录方法、路径、状态、耗时、字节数，不记录问题、答案、令牌或请求体。

## 5. 配置与部署

新增配置段：

```yaml
web:
  addr: 127.0.0.1:8080
  upload_dir: ./data/uploads
  max_upload_mb: 20
  frontend_dir: ./fronted/dist
```

环境变量：

| 变量 | 必填 | 说明 |
|------|------|------|
| `KB_RAG_LLM_API_KEY` | `serve` 必填 | 云端生成服务 Key |
| `KB_RAG_WEB_TOKEN` | `serve` 必填 | 局域网共享令牌，至少 16 字符 |

- 默认只绑定本机；局域网使用时把 `web.addr` 改为 `0.0.0.0:8080` 或指定网卡 IP。
- `validate("serve")` 检查：URL、chunk 参数、目录可创建、`frontend_dir/index.html` 存在、`addr` 合法 host:port、`max_upload_mb > 0`、令牌长度、LLM Key。
- `serve` 校验失败退出码 2；启动后运行期错误退出码 1；Ctrl+C 优雅关闭后退出码 130。
- HTTP 明文传输令牌，仅适用于可信局域网；需要更强安全时放在 HTTPS 反向代理后。
- 构建与启动：

```bash
cd fronted
npm install
npm run build

cd ..
go build -o KB-RAG.exe ./cmd/KB-RAG

export KB_RAG_WEB_TOKEN=...       # PowerShell: $env:KB_RAG_WEB_TOKEN="..."
export KB_RAG_LLM_API_KEY=...
./KB-RAG.exe serve --config config.yaml
```

- 浏览器访问 `http://127.0.0.1:8080`（局域网用对应 IP）。
- `fronted/node_modules/` 与 `fronted/dist/` 不提交；`package-lock.json` 提交。

## 6. 前端设计

### 6.1 技术栈与目录

Vue 3 + TypeScript + Vite + Vue Router + Element Plus + markdown-it。

```text
fronted/
  src/
    api/
      client.ts        fetch 封装、令牌注入、401 处理
      types.ts          API DTO
    components/
      AppShell.vue     侧栏/布局/退出
      CitationList.vue 引用卡片
      UploadPanel.vue  上传与逐文件状态
      DocumentTable.vue 文档列表与操作
    views/
      LoginView.vue
      ChatView.vue
      DocumentsView.vue
    router/index.ts
    markdown.ts
    main.ts
    styles.css
  package.json
  vite.config.ts
  tsconfig.json
  index.html
```

### 6.2 路由与登录

- hash 路由：`#/login`、`#/chat`、`#/documents`；默认重定向到 `#/chat`。
- 除登录页外，路由守卫检查 `sessionStorage` 中的令牌；没有则跳登录。
- 登录页：
  - 令牌输入框（password 类型）；
  - 调用 `GET /api/session` 校验；
  - 成功写入 `sessionStorage` 并进入问答页；
  - 失败显示“令牌无效”；
  - 提示令牌由管理员/部署者提供。
- 退出登录清除令牌并回到登录页。

### 6.3 API 客户端

- 统一 `request<T>()`：自动加 `Authorization: Bearer`，JSON 解析，非 2xx 抛出带 `error` 的异常；
- 401：清除令牌并跳转登录；
- 提供方法：`verifySession`、`getStats`、`listDocuments`、`uploadDocument`、`deleteDocument`、`reingestDocument`、`ask`；
- 上传使用 `FormData`，不手动设置 Content-Type；
- 不在前端保存 API Key；只保存 Web Token。

### 6.4 页面

**问答页**

- 问题输入框、发送按钮、top-k 选择（4/8/12，默认 8）；
- 发送中禁用并显示加载；
- 用户问题与助手答案按消息列表展示（仅浏览器内存，不持久化，不传给 LLM）；
- 答案使用 markdown-it 渲染，`html:false`，避免原始 HTML/XSS；
- `CitationList` 显示编号、来源名、标题路径、相似度、片段原文（可折叠）；
- 生成失败但检索成功：显示警告和引用片段；
- “清空对话”只清空浏览器消息。

**知识库页**

- 统计卡片：文档数、失败数、片段数、嵌入模型/维度、库大小；
- `UploadPanel`：
  - 拖拽/选择多文件，accept 与后端一致；
  - 前端预检扩展名与大小；
  - 串行上传，逐文件显示“上传/处理中/成功/失败”；
  - 完成后刷新文档列表与统计；
- `DocumentTable`：
  - 列：标题、文件名、状态、片段数、来源、更新时间、操作；
  - 失败文档显示“重试”；
  - 删除使用确认对话框，明确提示“只删除索引，源文件保留”；
  - 操作完成后刷新列表/统计；
- 空状态与错误提示使用 Element Plus 组件。

### 6.5 安全与渲染

- 令牌仅 sessionStorage；不使用 localStorage；
- 页面不渲染 LLM 返回的原始 HTML（markdown-it `html:false`）；
- 引用片段的 `text` 用文本插值，不当作 HTML；
- 不启用 CORS，不开放第三方 API 调用；
- 静态资源全部本地构建，不依赖 CDN。

## 7. 测试与验收

### 7.1 后端自动化

- Store：`ListDocuments` 排序与片段计数、`DeleteDocument` 级联删除、未知 ID。
- Ingest：`IngestFile` 成功、跳过、失败、重试。
- Web API（httptest，临时 Store/上传目录/前端目录）：
  - 无令牌 401、正确令牌 200；
  - `GET /api/health` 公开；
  - 上传 Markdown 成功并出现在列表；
  - 不支持的扩展名 415、超限 413；
  - 文件名路径穿越被安全化；
  - 删除后索引消失、源文件仍存在；
  - 失败文档重试；
  - `ask` 的答案、引用编号与片段；
  - 生成失败但检索成功返回 200 + `generated=false`；
  - 静态 index.html 可访问。

### 7.2 前端自动化

- `npm run type-check`（vue-tsc --noEmit）通过；
- `npm run build` 通过；
- 不强制引入前端单元测试框架；核心交互用后端 API 测试 + 手工冒烟覆盖。

### 7.3 手工冒烟

1. 启动 Ollama、Go API 与 Vite 或构建后的 SPA；
2. 输入令牌登录；
3. 上传 Markdown 文档，确认状态 ok、片段数合理；
4. 提问，确认答案、引用编号与片段可展开；
5. 删除文档，确认列表消失且上传文件仍在；
6. 从另一台局域网设备访问，未带令牌 401，带令牌可用；
7. 运行 `go test ./...`，确认 CLI 无回归。

### 7.4 验收标准

- 上述 API 与前端功能可用；
- 无令牌无法调用除 health 外的 API；
- 删除只影响索引，源文件保留；
- 现有 CLI `ingest`、`ask`、`stats` 行为不变；
- `go test ./...` 与前端构建通过；
- README 包含构建、令牌、局域网与安全说明。

## 8. 风险与取舍

| 风险/取舍 | 说明 |
|-----------|------|
| 共享令牌 | 无用户隔离与审计；满足可信局域网的基本访问控制 |
| HTTP 明文 | 令牌可能被嗅探；仅在可信网络使用，必要时加 HTTPS 反向代理 |
| 同步上传 | 大文件等待时间长；限制 20 MB，建议大文件走 CLI |
| 上传目录累积 | 删除只删索引；本期不做自动清理 |
| SQLite 单连接 | Web 入库串行化；问答与入库可能短暂互相等待，适合小规模局域网 |
| CLI/Web 并发入库 | 二者共用 SQLite；Web 内部串行，但不阻止 CLI 同时 ingest，部署时应避免同时操作 |
| 上传中断 | 浏览器取消请求可能留下未入库文件；本期不做清理，重新上传或走 CLI 即可 |
| 前端磁盘托管 | 需要先构建 `fronted/dist`；Go 构建不依赖前端产物 |
| 无多轮上下文 | 每次问答独立；对话历史只在浏览器展示 |

## 9. 实施顺序（高层）

1. 配置扩展与 `serve` 命令骨架；
2. Store 列表/删除、Ingest 单文件函数；
3. Web API、认证、上传/删除/重试；
4. API 测试；
5. Vue 3 脚手架与登录/问答/知识库页面；
6. 构建与静态托管联调；
7. 文档、冒烟与回归。

详细任务拆分由后续实施计划给出。

# KB-RAG

企业知识库 RAG 命令行工具。文档在本地解析、切分并存储，嵌入通过本地 Ollama 完成，答案由云端 OpenAI 兼容接口生成，并附带原文引用。

## 状态

- 第一期 CLI 已实现；真实 Ollama 与云端 OpenAI 兼容网关冒烟通过（2026-10-23，`ingest` / `ask` / `stats` 手动验证）。
- 第二期 Web 界面已实现，并扩展为账号登录、admin/editor/viewer 角色、多租户与审计日志（2026-10-26）。
- 30 条人工评测已完成（25 条可答 + 5 条无答案），在当前样例集上达到验收线：top-8 92%、答案正确且引用无误 88%、拒答 100%；详见 [评测结果](docs/eval/questions.md)。

## 模块文档

每个模块的职责、接口、行为约定与测试说明见对应 README：

| 模块 | 职责 |
|------|------|
| [cmd/KB-RAG](cmd/KB-RAG/README.md) | 进程入口、信号与退出码 |
| [internal/cli](internal/cli/README.md) | 子命令解析、依赖装配、输出 |
| [internal/config](internal/config/README.md) | YAML 配置、默认值、按命令校验 |
| [internal/logging](internal/logging/README.md) | slog、文件轮转、日志隐私 |
| [internal/ingest](internal/ingest/README.md) | 入库编排、增量与失败语义 |
| [internal/ingest/loader](internal/ingest/loader/README.md) | Markdown/TXT/DOCX/PDF 解析 |
| [internal/ingest/chunker](internal/ingest/chunker/README.md) | 结构感知切分 |
| [internal/ingest/embedder](internal/ingest/embedder/README.md) | Ollama 嵌入客户端 |
| [internal/store](internal/store/README.md) | SQLite 存储、事务与向量检索 |
| [internal/llm](internal/llm/README.md) | OpenAI 兼容生成客户端 |
| [internal/query](internal/query/README.md) | 检索编排、prompt 与引用格式化 |
| [internal/web](internal/web/README.md) | HTTP API、会话认证、角色与多租户 |
| [internal/auth](internal/auth/password.go) | 密码哈希与会话令牌原语 |
| [fronted](fronted/README.md) | Vue 3 Web 界面 |

## 设计文档

- [设计文档](docs/superpowers/specs/2026-10-23-KB-RAG-design.md)
- [文档切分规则](docs/文档切分规则.md)
- [实施计划](docs/superpowers/plans/2026-10-23-KB-RAG-implementation.md)
- [人工评测](docs/eval/questions.md)

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

# 指定租户（默认 default；需先在 Web 中创建该租户）
./KB-RAG.exe ingest --tenant acme ./docs

# 检索并生成带引用的答案
./KB-RAG.exe ask "年假怎么请"
./KB-RAG.exe ask --tenant acme --top-k 4 "年假怎么请"

# 查看状态
./KB-RAG.exe stats
./KB-RAG.exe stats --tenant acme
```

运行时数据默认写入 `./data/`，不提交到版本库。

`--top-k` 等 flag 必须写在问题之前（Go `flag` 在第一个位置参数处停止解析）。

## Web 界面

Web 界面由 Go `serve` 命令托管，前端源码在 `fronted/`，支持账号登录、角色权限、多租户与审计日志。

```powershell
# 构建前端
cd fronted
npm install
npm run build
cd ..

# 构建 Go
go build -o KB-RAG.exe ./cmd/KB-RAG

# 首次启动：设置初始超级管理员（仅 users 表为空时生效）
$env:KB_RAG_ADMIN_USER = "admin"          # 可选，默认 admin
$env:KB_RAG_ADMIN_PASSWORD = "至少12个字符"
$env:KB_RAG_LLM_API_KEY = "你的生成服务Key"

# 启动 Web 服务（默认 http://127.0.0.1:8080）
./KB-RAG.exe serve --config config.yaml
```

浏览器打开 `http://127.0.0.1:8080`，输入租户（默认 `default`）、用户名和密码登录。`KB_RAG_ADMIN_PASSWORD` 只在首次创建管理员时读取，之后不会覆盖已有密码；后续改密请用系统管理页或 CLI 之外的恢复流程。

### 账号与角色

| 角色 | 权限 |
|------|------|
| `admin` | 本租户用户管理、审计查看、文档上传/删除/重试、问答 |
| `editor` | 文档上传/删除/重试、问答与查看 |
| `viewer` | 只能问答与查看文档列表/统计 |

首个管理员是超级管理员（`is_super`），额外可以创建租户、重置任一租户管理员密码、查看全局审计（`scope=all`）。用户名在同一租户内唯一，登录必须带租户标识。

### 多租户

- 每个租户拥有独立的文档与片段；跨租户访问统一返回 404。
- 现有数据升级后自动归入 `default` 租户。
- 新租户由超级管理员在“系统管理 → 租户管理”中创建，创建时需同时指定该租户的首个管理员。
- CLI 用 `--tenant <slug>` 操作指定租户；租户不存在时退出码 2，不会自动创建。
- 嵌入模型与切分参数仍是全库共享，多租户只隔离数据，不隔离模型。

### 审计日志

记录登录成功/失败、文档上传/删除/重试、问答元数据（问题原文、top_k、引用数、是否生成、耗时）、用户与租户管理操作。**不记录答案、引用原文、密码或会话令牌**。审计在 SQLite 中保留，当前不做自动清理；租户管理员查看本租户，超级管理员可用“全局”查看未归属事件。

### 升级与备份

从旧版本（schema v2）升级时会自动迁移到 v3：文档与片段保留并归入 `default` 租户。升级前请停止所有 KB-RAG 进程，并备份 `KB-RAG.db`、`-wal`、`-shm`；旧版本二进制无法打开 v3 库。首次升级启动后按提示设置 `KB_RAG_ADMIN_PASSWORD` 创建管理员。

### 局域网与安全

局域网访问：把 `config.yaml` 的 `web.addr` 改为 `0.0.0.0:8080`（或指定网卡 IP），并从其他设备访问 `http://<主机IP>:8080`。密码和会话令牌使用明文 HTTP 传输，请只在可信局域网使用；需要更强安全时放在 HTTPS 反向代理后。会话固定 12 小时，前端只把令牌保存在 `sessionStorage`。

超级管理员密码无法通过管理 API 重置。若遗失，请停服并备份后手动执行 `DELETE FROM users; DELETE FROM sessions;`，再按首次启动流程设置 `KB_RAG_ADMIN_PASSWORD` 重新创建（文档与租户数据保留）。

前端开发模式：

```bash
cd fronted
npm run dev
```

Vite 会把 `/api` 代理到 `127.0.0.1:8080`，需要同时运行 `KB-RAG serve`。

删除文档只删除知识库索引，任何源文件都会保留；上传目录不会自动清理。

详见 [Web 设计文档](docs/superpowers/specs/2026-10-26-KB-RAG-web-design.md)。

## 已知限制

- 扫描件 PDF 不做 OCR。
- PDF 使用纯 Go 库提取文本。第一期未实现 `pdftotext` 兜底；若真实业务 PDF 出现乱码，需要后续增加该 loader（依赖 poppler）。
- 检索为内存暴力点积，适合小规模知识库；片段超过 20 万时 `stats` 会提示迁移。
- 第一期不做重排、混合检索、query rewrite、多轮对话与流式输出。

## 测试

```bash
go test ./...
go vet ./...
```

集成测试使用假 Ollama 与假生成服务，不需要真实网络或模型。

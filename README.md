# KB-RAG

企业知识库 RAG 命令行工具。文档在本地解析、切分并存储，嵌入通过本地 Ollama 完成，答案由云端 OpenAI 兼容接口生成，并附带原文引用。

## 状态

- 第一期 CLI 已实现；真实 Ollama 与云端 OpenAI 兼容网关冒烟通过（2026-10-23，`ingest` / `ask` / `stats` 手动验证）。
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

# 检索并生成带引用的答案
./KB-RAG.exe ask "年假怎么请"
./KB-RAG.exe ask --top-k 4 "年假怎么请"

# 查看状态
./KB-RAG.exe stats
```

运行时数据默认写入 `./data/`，不提交到版本库。

`--top-k` 等 flag 必须写在问题之前（Go `flag` 在第一个位置参数处停止解析）。

## Web 界面

Web 界面由 Go `serve` 命令托管，前端源码在 `fronted/`。

```powershell
# 构建前端
cd fronted
npm install
npm run build
cd ..

# 构建 Go
go build -o KB-RAG.exe ./cmd/KB-RAG

# 设置两个密钥（PowerShell 示例）
$env:KB_RAG_WEB_TOKEN = "至少16个字符的共享令牌"
$env:KB_RAG_LLM_API_KEY = "你的生成服务Key"

# 启动 Web 服务（默认 http://127.0.0.1:8080）
./KB-RAG.exe serve --config config.yaml
```

浏览器打开 `http://127.0.0.1:8080`，输入共享令牌登录。

局域网访问：把 `config.yaml` 的 `web.addr` 改为 `0.0.0.0:8080`（或指定网卡 IP），并从其他设备访问 `http://<主机IP>:8080`。令牌使用明文 HTTP 传输，请只在可信局域网使用；需要更强安全时放在 HTTPS 反向代理后。

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

# KB-RAG

企业知识库 RAG 命令行工具。文档在本地解析、切分并存储，嵌入通过本地 Ollama 完成，答案由云端 OpenAI 兼容接口生成，并附带原文引用。

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
- [实施计划](docs/superpowers/plans/2026-10-23-KB-RAG-implementation.md)
- [人工评测模板](docs/eval/questions.md)

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

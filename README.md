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

# internal/config

读取 YAML 配置、应用默认值、从环境变量读取 API Key，并按命令做校验。

## 配置结构

```yaml
embedding:
  base_url: http://localhost:11434
  model: bge-m3
  batch_size: 16

llm:
  base_url: https://api.deepseek.com/v1
  model: deepseek-chat

chunk:
  size: 500
  overlap: 80

store:
  path: ./data/KB-RAG.db

log:
  file: ./data/logs/KB-RAG.log
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `embedding.base_url` | `http://localhost:11434` | Ollama 地址 |
| `embedding.model` | `bge-m3` | 嵌入模型名，必须与库内 `meta.embedding_model` 一致 |
| `embedding.batch_size` | `16` | 每次请求的文本条数 |
| `chunk.size` | `500` | 片段字符上限 |
| `chunk.overlap` | `80` | 长段落切分时的重叠字符数 |
| `store.path` | `./data/KB-RAG.db` | SQLite 文件 |
| `log.file` | `./data/logs/KB-RAG.log` | 日志文件 |

`llm.base_url` / `llm.model` 没有默认值；`ask` 必须提供。

API Key 只从环境变量 `KB_RAG_LLM_API_KEY` 读取，不写入 YAML、不打印、不落日志。

## 校验

```go
func Load(path string) (*Config, error)
func (c *Config) Validate(command string) error
```

- `ingest` / `stats`：校验 embedding、chunk、store、log；不要求 LLM 配置与 API Key。
- `ask`：额外校验 `llm.base_url`、`llm.model` 与 API Key。
- 通用规则：URL 必须是 `http(s)`、必填项非空、`chunk.size > chunk.overlap > 0`、库目录与日志目录可创建。
- 校验会 `MkdirAll` 存储目录和日志目录；失败时 CLI 返回退出码 2。

## 测试

[config_test.go](config_test.go) 覆盖默认值合并、环境变量读取，以及非法 URL、空模型、非法 batch、`overlap >= size`、空路径、ask 缺 Key 等校验分支。

## 相关文档

- [CLI 模块](../cli/README.md)
- 示例配置：[config.example.yaml](../../config.example.yaml)

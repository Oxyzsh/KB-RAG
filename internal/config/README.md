# internal/config

读取 YAML 配置、应用默认值、从环境变量读取密钥与首次管理员信息，并按命令做校验。

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

web:
  addr: 127.0.0.1:8080
  upload_dir: ./data/uploads
  max_upload_mb: 20
  frontend_dir: ./fronted/dist

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
| `web.addr` | `127.0.0.1:8080` | 监听地址 |
| `web.upload_dir` | `./data/uploads` | 上传文件根目录 |
| `web.max_upload_mb` | `20` | 单文件上限 |
| `web.frontend_dir` | `./fronted/dist` | 前端构建产物目录 |
| `log.file` | `./data/logs/KB-RAG.log` | 日志文件 |

`llm.base_url` / `llm.model` 没有默认值；`ask` 与 `serve` 必须提供。

## 环境变量

| 变量 | 说明 |
|------|------|
| `KB_RAG_LLM_API_KEY` | 生成服务 Key，`ask` / `serve` 必填 |
| `KB_RAG_ADMIN_USER` | 首次启动管理员用户名，默认 `admin` |
| `KB_RAG_ADMIN_PASSWORD` | 首次启动管理员密码，至少 12 字符；仅在 `users` 为空时使用 |

这些值都不写入 YAML、不打印、不落日志。

## 校验

```go
func Load(path string) (*Config, error)
func (c *Config) Validate(command string) error
```

- `ingest` / `stats`：校验 embedding、chunk、store、log；不要求 LLM 配置与 API Key。
- `ask`：额外校验 `llm.base_url`、`llm.model` 与 API Key。
- `serve`：在 `ask` 基础上校验 `web.addr`（host:port）、上传目录可创建、`max_upload_mb > 0`、`frontend_dir/index.html` 存在。
- 通用规则：URL 必须是 `http(s)`、必填项非空、`chunk.size > chunk.overlap > 0`、库目录与日志目录可创建。
- 校验会 `MkdirAll` 存储目录、日志目录和上传目录；失败时 CLI 返回退出码 2。

## 测试

[config_test.go](config_test.go) 覆盖默认值合并、管理员环境变量读取，以及非法 URL、空模型、非法 batch、`overlap >= size`、空路径、ask 缺 Key、serve 非法地址/端口/上传上限/缺少前端产物等校验分支。

## 相关文档

- [CLI 模块](../cli/README.md)
- 示例配置：[config.example.yaml](../../config.example.yaml)

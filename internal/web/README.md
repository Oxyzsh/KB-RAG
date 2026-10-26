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

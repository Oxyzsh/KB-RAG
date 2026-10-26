# internal/web

KB-RAG 的 HTTP API 与静态文件服务，由 `KB-RAG serve` 启动。

## 认证

- `POST /api/login` 公开，请求 `{"tenant":"default","username":"...","password":"..."}`，返回会话令牌与用户信息。
- 除 `GET /api/health` 与 `POST /api/login` 外，所有 API 要求：

```http
Authorization: Bearer <会话令牌>
```

- 密码使用 PBKDF2-SHA256（600k 次、随机盐）哈希；令牌 32 字节随机生成，数据库只存 SHA-256 摘要，有效期 12 小时。
- 登录按 IP + 租户 + 用户名限流（5 次 / 5 分钟），并对未知用户执行固定成本校验以降低时序差异。
- 令牌不写日志、不回显。

## 角色

| 角色 | 权限 |
|------|------|
| `admin` | 本租户用户管理、审计查看、文档读写 |
| `editor` | 文档上传/删除/重试、问答与查看 |
| `viewer` | 问答、文档列表与统计 |

超级管理员（`is_super`）额外可创建租户、重置任一租户管理员密码、查看全局审计。

### 超级管理员切换租户

超级管理员可在请求中携带：

```http
X-Tenant: <租户 slug>
```

服务端校验通过后，本次请求的有效租户替换为目标租户，文档、问答、用户管理与审计均按目标租户执行。非超级管理员携带该头返回 403，租户不存在返回 400。未携带时使用用户所属租户。前端通过在 `sessionStorage` 保存所选租户并在侧栏提供切换入口实现。

## API

| 方法 | 路径 | 角色 | 说明 |
|------|------|------|------|
| GET | `/api/health` | 公开 | 健康检查 |
| POST | `/api/login` | 公开 | 登录并返回令牌 |
| POST | `/api/logout` | 登录 | 删除当前会话 |
| GET | `/api/session` | 登录 | 当前用户/租户/角色 |
| GET | `/api/stats` | 登录 | 本租户文档/片段/模型/数据量 |
| GET | `/api/documents` | 登录 | 本租户文档列表 |
| GET | `/api/documents/{id}/preview` | 登录 | 引用来源预览（原文或片段） |
| GET | `/api/documents/{id}/raw` | 登录 | 托管上传文件原文（PDF 内联、DOCX 下载） |
| POST | `/api/documents` | admin/editor | multipart 字段 `file`，同步上传并入库 |
| DELETE | `/api/documents/{id}` | admin/editor | 只删索引，源文件保留 |
| POST | `/api/documents/{id}/reingest` | admin/editor | 同步重试/重新入库 |
| POST | `/api/ask` | 登录 | `{"question":"...","top_k":8}` |
| POST | `/api/ask/stream` | 登录 | SSE 流式问答 |
| POST | `/api/jobs/documents` | admin/editor | 异步上传，返回 202 与任务 |
| POST | `/api/jobs/documents/{id}/reingest` | admin/editor | 异步重试，返回 202 |
| GET | `/api/jobs/{id}` | admin/editor | 查询任务状态与阶段 |
| DELETE | `/api/jobs/{id}` | admin/editor | 取消排队/运行中的任务 |
| GET | `/api/users` | admin | 本租户用户列表 |
| POST | `/api/users` | admin | 创建用户 |
| PATCH | `/api/users/{id}` | admin | 修改角色或重置密码 |
| DELETE | `/api/users/{id}` | admin | 删除用户 |
| GET | `/api/tenants` | admin | 本租户；超级管理员返回全部 |
| POST | `/api/tenants` | super | 创建租户及首个管理员 |
| POST | `/api/tenants/{slug}/admin/reset` | super | 重置指定租户管理员密码 |
| GET | `/api/audit` | admin | 本租户审计；super 可加 `scope=all` |

- 跨租户资源统一返回 404；角色不足返回 403。
- 所有 `/api/*` 响应带 `Cache-Control: no-store`。
- 审计为 best-effort：写入失败只记应用日志，不阻断业务。

## 流式问答

`POST /api/ask/stream` 在检索成功后返回 `text/event-stream`：

```text
event: citations
data: [{"n":1,"source":"a.md",...}]

event: delta
data: {"text":"增量文本"}

event: done
data: {"answer":"完整答案","generated":true}
```

生成失败时发送 `event: error`；客户端中断会取消上游生成。检索阶段失败仍返回普通 JSON 错误（400/409/502）。

## 引用来源预览

引用 DTO 带 `document_id`，前端可调用：

- `GET /api/documents/{id}/preview`：返回标题、文件名、来源、类型、文本与 `raw_available`；
  - Markdown/TXT 且文件在托管上传目录时返回原文；
  - PDF/DOCX 或 CLI 外部文件返回已索引片段拼成的文本；
  - 不返回服务器绝对路径。
- `GET /api/documents/{id}/raw`：仅允许 `web.upload_dir` 下的文件；PDF/Markdown/TXT 内联，DOCX 下载；外部 CLI 文件返回 403。

所有预览接口按租户隔离，跨租户统一 404。

## 上传

- 允许 `.md` `.markdown` `.txt` `.docx` `.pdf`；
- 同步接口保存到 `web.upload_dir/<随机目录>/<安全文件名>` 并立即入库；
- 异步接口 `POST /api/jobs/documents` 返回 202，由单 worker 串行处理，阶段写入 `jobs.stage`；
- 任务状态：`queued` / `running` / `done` / `failed` / `canceled`；服务重启时将 `running` 标记失败，`queued` 会继续处理；
- 解析/嵌入失败时文档标记 `failed`，文件保留，可重试；
- 删除文档不删除源文件；任务记录保留 7 天后清理。

## 测试

`server_test.go`、`documents_test.go`、`ask_test.go`、`roles_test.go`、`users_test.go`、`tenants_test.go`、`audit_test.go` 使用临时 SQLite、临时上传目录、假嵌入与假 LLM 覆盖登录、会话、角色矩阵、租户隔离、上传、删除、重试、问答、用户/租户管理与审计。

## 状态

- 账号、角色、多租户、审计日志已实现；
- v2→v3 自动迁移在 `internal/store` 中实现。

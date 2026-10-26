# internal/store

SQLite 持久化与向量检索层。使用纯 Go 驱动 `modernc.org/sqlite`，无需 CGO。

## 打开与模式

```go
func Open(path string) (*Store, error)
```

- 自动创建库目录；
- 启用 WAL、`busy_timeout=5000`、外键；
- 单连接（`SetMaxOpenConns(1)`），避免进程内写锁竞争；
- 校验 `meta.schema_version`，当前为 `4`。v2/v3 库会自动迁移到 v4（v2 先迁到 v3，再新增 `jobs` 表）；其他版本直接报错，提示恢复备份。

## 表结构

```sql
tenants(id, slug UNIQUE, name, created_at)

users(
  id, tenant_id REFERENCES tenants(id),
  username, password_hash, role CHECK(admin/editor/viewer),
  is_super CHECK(0/1), created_at, updated_at,
  UNIQUE(tenant_id, username)
)

sessions(
  token_hash PRIMARY KEY, user_id REFERENCES users(id) ON DELETE CASCADE,
  created_at, expires_at
)

audit_logs(
  id, tenant_id NULL, user_id NULL, username,
  action, target, detail, ip, created_at
)

documents(
  id, tenant_id REFERENCES tenants(id),
  path, path_key,
  title, content_hash, status, error, updated_at,
  UNIQUE(tenant_id, path_key)
)

chunks(
  id, doc_id REFERENCES documents(id) ON DELETE CASCADE,
  seq, heading, text, vector BLOB, dim
)

jobs(
  id, tenant_id, user_id, username, action, path, name,
  status, stage, chunks, error, ip, created_at, updated_at
)

meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)
-- schema_version / embedding_model / dim / chunk_size
```

- `chunks` 不单独存 `tenant_id`，通过 `doc_id` 关联文档的租户。
- `audit_logs` 刻意不加外键：用户/租户删除后仍保留历史用户名与租户快照；`tenant_id IS NULL` 表示无法归属的系统级事件（如未知租户登录失败）。

## 元信息一致性

| 方法 | 作用 |
|------|------|
| `InitMeta(model, chunkSize)` | 写入或校验嵌入模型与切分参数；不一致报错 |
| `Meta()` | 返回 schema 版本、模型、维度、chunk_size |
| `SetDim(dim)` | 首次写入维度；之后维度不一致直接报错 |

模型、维度或 `chunk_size` 变化都不允许新旧向量混存，需删除库文件重新 ingest。

## 文档与片段

```go
func (s *Store) GetDocument(ctx, tenantID, path string) (DocumentInfo, bool, error)
func (s *Store) ReplaceDocument(ctx, tenantID, path, title, hash string, chunks []ChunkInput) error
func (s *Store) MarkFailed(ctx, tenantID, path, title, hash, msg string) error
```

- `GetDocument` 用 `(tenant_id, path_key)` 查找，Windows 下路径大小写不同也能命中同一文档。
- `ReplaceDocument` 在一个事务内：upsert 文档、删除旧片段、插入新片段；任意一步失败整体回滚。
- `MarkFailed` 只更新文档状态与错误，**保留旧片段**；`status=failed` 的文档不参与检索，但会在下次 ingest 时重试。
- 源文件删除不会自动清理记录（第一期没有 prune）。
- `ListDocuments`：按 `updated_at DESC, id DESC` 返回本租户文档摘要，并统计每个文档的片段数。
- `GetDocumentByID` / `DeleteDocument`：按 `(tenant_id, id)` 查询与删除。删除只移除数据库记录，`chunks` 通过外键级联删除；源文件不受影响。跨租户统一表现为“不存在”。

## 账号、租户与会话

```go
func (s *Store) EnsureBootstrapAdmin(ctx, username, passwordHash string) (User, bool, error)
func (s *Store) CreateTenantWithAdmin(ctx, slug, name, username, passwordHash string) (Tenant, User, error)
func (s *Store) CreateUser(ctx, tenantID, username, passwordHash, role string) (User, error)
func (s *Store) UpdateUserRole(ctx, tenantID, id, role string) error
func (s *Store) ResetUserPassword(ctx, tenantID, id, passwordHash string) error
func (s *Store) DeleteUser(ctx, tenantID, id, actorID int64) error
func (s *Store) CreateSession(ctx, tokenHash string, userID int64, expiresAt time.Time) error
func (s *Store) PrincipalByTokenHash(ctx, tokenHash string, now time.Time) (Principal, bool, error)
```

- 用户名只在租户内唯一；登录需同时提供租户 slug。
- `EnsureBootstrapAdmin` 仅在 `users` 为空时创建 `default` 租户的超级管理员。
- 最后管理员保护、禁止删除自己、保护超级管理员都在此层强制。
- 会话只保存令牌的 SHA-256 摘要；重置密码/删除用户会删除对应会话。

## 审计

```go
func (s *Store) InsertAudit(ctx, entry AuditEntry) error
func (s *Store) ListAudit(ctx, filter AuditFilter) ([]AuditEntry, int, error)
```

- `AuditFilter.TenantID` 为空表示全局查询（仅超级管理员接口使用）。
- 分页默认 50、上限 200；支持按动作与时间范围过滤。

## 异步任务

```go
func (s *Store) CreateJob(ctx, job Job) (Job, error)
func (s *Store) NextQueuedJob(ctx) (Job, bool, error)
func (s *Store) UpdateJob(ctx, id, status, stage string, chunks int, errMsg string) error
func (s *Store) CancelQueuedJob(ctx, tenantID, id int64) (bool, error)
```

- 状态：`queued` / `running` / `done` / `failed` / `canceled`。
- `NextQueuedJob` 在事务内把最老的排队任务置为 `running`，保证单 worker 串行消费。
- 服务启动时 `MarkInterruptedJobs` 把残留 `running` 标记失败，`queued` 保留继续处理。
- 已完成/失败/取消的任务保留 7 天后由 `DeleteOldJobs` 清理。

## 检索

```go
func (s *Store) Search(ctx, tenantID int64, query []float32, topK int) ([]Hit, error)
```

- 校验查询维度与库内维度；
- 归一化查询向量；
- 只读取本租户 `status='ok'` 文档的片段，计算点积；
- 按分数降序截取 top-k。

向量在写入时归一化，检索用点积等价于余弦相似度。当前为内存暴力检索，适合小规模库；CLI 在片段超过 20 万时提示迁移 Qdrant。

## 统计

```go
func (s *Store) Stats(ctx, tenantID int64) (Stats, error)
```

返回本租户文档总数、失败文档数、片段数、失败路径、本租户逻辑数据量（`tenant_size_bytes`）、全库物理文件大小（含 `-wal` / `-shm`）与嵌入元信息。

## 测试

[store_test.go](store_test.go)、[migrate_test.go](migrate_test.go)、[accounts_test.go](accounts_test.go)、[audit_test.go](audit_test.go) 覆盖：

- 替换后 top-k 排序与旧片段删除；
- 文档生命周期与失败状态；
- 模型/维度/chunk_size 不一致拒绝；
- v2→v4、v3→v4 迁移保留数据、级联、唯一约束与失败回滚；未知版本拒绝；
- 异步任务创建、状态推进、取消、重启中断标记与旧任务清理；
- 租户隔离：同路径跨租户、检索/统计/删除/跨租户 404；
- 用户 CRUD、最后管理员保护、超级管理员保护、会话过期与密码重置失效；
- 审计插入、分页、动作过滤与 NULL 租户；
- 事务中途插入失败时旧版本完整回滚；
- Windows 路径大小写身份。

## 相关文档

- [入库管道](../ingest/README.md)
- [查询](../query/README.md)
- [设计文档](../../docs/superpowers/specs/2026-10-23-KB-RAG-design.md)

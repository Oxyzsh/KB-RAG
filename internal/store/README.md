# internal/store

SQLite 持久化与向量检索层。使用纯 Go 驱动 `modernc.org/sqlite`，无需 CGO。

## 打开与模式

```go
func Open(path string) (*Store, error)
```

- 自动创建库目录；
- 启用 WAL、`busy_timeout=5000`、外键；
- 单连接（`SetMaxOpenConns(1)`），避免进程内写锁竞争；
- 校验 `meta.schema_version`，当前为 `2`。版本不一致直接报错，提示删除库文件重建；本期不实现迁移。

## 表结构

```sql
documents(
  id INTEGER PRIMARY KEY,
  path TEXT NOT NULL,              -- 展示路径
  path_key TEXT NOT NULL UNIQUE,   -- 文件身份：绝对路径，Windows 上折叠大小写
  title TEXT NOT NULL,
  content_hash TEXT NOT NULL,      -- SHA-256
  status TEXT NOT NULL,            -- ok / failed
  error TEXT NOT NULL,
  updated_at TEXT NOT NULL
)

chunks(
  id INTEGER PRIMARY KEY,
  doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  heading TEXT NOT NULL,
  text TEXT NOT NULL,
  vector BLOB NOT NULL,            -- float32 小端，写入时已归一化
  dim INTEGER NOT NULL
)

meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)
-- schema_version / embedding_model / dim / chunk_size
```

## 元信息一致性

| 方法 | 作用 |
|------|------|
| `InitMeta(model, chunkSize)` | 写入或校验嵌入模型与切分参数；不一致报错 |
| `Meta()` | 返回 schema 版本、模型、维度、chunk_size |
| `SetDim(dim)` | 首次写入维度；之后维度不一致直接报错 |

模型、维度或 `chunk_size` 变化都不允许新旧向量混存，需删除库文件重新 ingest。

## 文档与片段

```go
func (s *Store) GetDocument(path string) (DocumentInfo, bool, error)
func (s *Store) ReplaceDocument(ctx, path, title, hash string, chunks []ChunkInput) error
func (s *Store) MarkFailed(ctx, path, title, hash, msg string) error
```

- `GetDocument` 用 `path_key` 查找，Windows 下路径大小写不同也能命中同一文档。
- `ReplaceDocument` 在一个事务内：upsert 文档、删除旧片段、插入新片段；任意一步失败整体回滚。
- `MarkFailed` 只更新文档状态与错误，**保留旧片段**；`status=failed` 的文档不参与检索，但会在下次 ingest 时重试。
- 源文件删除不会自动清理记录（第一期没有 prune）。

## 检索

```go
func (s *Store) Search(ctx, query []float32, topK int) ([]Hit, error)
```

- 校验查询维度与库内维度；
- 归一化查询向量；
- 读取所有 `status='ok'` 文档的片段，计算点积；
- 按分数降序截取 top-k。

向量在写入时归一化，检索用点积等价于余弦相似度。当前为内存暴力检索，适合小规模库；CLI 在片段超过 20 万时提示迁移 Qdrant。

## 统计

```go
func (s *Store) Stats(ctx) (Stats, error)
```

返回文档总数、失败文档数、片段数、失败路径、库大小（含 `-wal` / `-shm`）与嵌入元信息。

## 测试

[store_test.go](store_test.go) 覆盖：

- 替换后 top-k 排序与旧片段删除；
- 文档生命周期与失败状态；
- 模型/维度/chunk_size 不一致拒绝；
- schema 版本不一致拒绝；
- 事务中途插入失败时旧版本完整回滚；
- Windows 路径大小写身份；
- 统计字段与空库检索。

## 相关文档

- [入库管道](../ingest/README.md)
- [查询](../query/README.md)
- [设计文档](../../docs/superpowers/specs/2026-10-23-KB-RAG-design.md)

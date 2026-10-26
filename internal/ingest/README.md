# internal/ingest

入库编排层：扫描目录、判断增量、调用 loader/chunker/embedder，并通过 store 以单文档事务写入。

## 入口

```go
func Run(ctx context.Context, root string, st Store, emb Embedder, opt Options) (Summary, error)
```

```go
type Options struct {
    TenantID  int64
    Model     string
    ChunkSize int
    Overlap   int
    Logger    *slog.Logger
    Progress  func(stage string)
}

type Summary struct {
    Succeeded   int
    Skipped     int
    Failed      int
    FailedPaths []string
}
```

`Store` 与 `Embedder` 都是窄接口，便于测试和替换实现；生产环境分别由 `internal/store` 与 `internal/ingest/embedder` 提供。

单文件入口：

```go
type FileResult struct {
    Outcome string // ok / skipped / failed
    Chunks  int
    Err     error
    Fatal   bool
}

func IngestFile(ctx context.Context, path string, st Store, emb Embedder, opt Options) FileResult
```

调用前必须已执行 `Store.InitMeta`。Web 上传与重试使用该函数；`Run` 对目录扫描也复用同一内部逻辑。`Options.TenantID` 决定写入哪个租户，调用方需先通过 `store.TenantBySlug` 解析。`Options.Progress` 可选，用于异步任务展示“读取文件/切分文档/生成向量/写入索引”等阶段。

## 扫描规则

- 递归遍历目录，跳过隐藏目录（`.` 开头，包含 `.git`）与 `node_modules`。
- 只处理 [loader](loader/README.md) 支持的扩展名：`.md`、`.markdown`、`.txt`、`.docx`、`.pdf`。
- 目录遍历错误计入失败清单，不会静默跳过。
- 单个文件失败只记录该文件，继续处理其他文件；有失败时 CLI 返回退出码 1。

## 单文件流程

```text
读取文件 → SHA-256
  → 查询 documents 表
  → status=ok 且 hash 相同：跳过
  → loader.Load → chunker.Split
  → 批量嵌入 → SetDim
  → 单事务替换旧片段并更新文档
```

- 解析失败、切分结果为空、嵌入失败：调用 `MarkFailed`，旧片段保留在库中，下次 ingest 因状态不是 `ok` 而重试。
- 嵌入全部成功后才进入事务；`ReplaceDocument` 内部先删旧片段再插新片段，失败会整体回滚。
- `SetDim` 发现维度与库内不一致时返回致命错误，终止本次 ingest，避免混存不同模型的向量。
- 嵌入模型与 `chunk_size` 由 `store.InitMeta` 在开始时校验；不一致直接报错，要求删除库文件重建。

## 失败与中断语义

| 场景 | 行为 |
|------|------|
| 文件读取失败 | 计入失败，继续 |
| 解析/切分/嵌入失败 | `MarkFailed`，保留旧片段；下次重试 |
| 目录遍历失败 | 计入失败，继续 |
| 维度不一致 | 致命错误，终止本次运行 |
| `ctx` 取消（Ctrl+C） | 正在执行的事务回滚，已成功文档保留；CLI 返回 130 |

## 日志

- 每个文件：成功/跳过/失败、路径、片段数、耗时。
- 汇总：成功/跳过/失败数量。
- 日志只记录路径、数量和耗时，不记录正文。

## 测试

[ingest_test.go](ingest_test.go) 覆盖首次入库、重复跳过、文件变更重入库、嵌入失败后重试、损坏 DOCX 不影响其他文件。

## 相关文档

- [文档解析](loader/README.md)
- [切分](chunker/README.md)
- [嵌入客户端](embedder/README.md)
- [存储](../store/README.md)

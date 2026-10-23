# internal/ingest/embedder

调用本地 Ollama 的 `/api/embed`，把文本批量转成向量。

## 接口

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}

func New(baseURL, model string, batchSize int) *Ollama
```

## 请求

```http
POST {base_url}/api/embed
Content-Type: application/json

{"model":"bge-m3","input":["文本1","文本2"]}
```

响应读取 `embeddings: [][]float32`。

## 行为

| 项目 | 约定 |
|------|------|
| 批量 | 按 `batch_size` 分批，默认 16 |
| 超时 | 单次 HTTP 请求 120 秒 |
| 重试 | 每批最多 3 次，指数退避 500ms、1s |
| 数量校验 | 返回向量数必须等于输入条数 |
| 维度校验 | 所有向量维度必须一致，且非空 |
| 错误信息 | 只包含状态码，不拼接响应体，避免服务端回显正文 |

失败时返回带批次范围的中文错误，例如“嵌入第 1-16 条失败”。上层 ingest 会把该文件标记为失败并保留旧版本。

## 测试

[ollama_test.go](ollama_test.go) 使用 `httptest` 覆盖分批请求、重试三次、数量不一致、维度不一致、错误响应不泄露正文。

## 相关文档

- [入库管道](../README.md)
- [配置](../../config/README.md)

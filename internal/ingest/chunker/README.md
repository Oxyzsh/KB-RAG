# internal/ingest/chunker

把 `loader.Document` 切成带标题路径的检索片段。每个 `Chunk` 会单独嵌入、写入一条 `chunks` 记录，并单独参与 top-k 检索。

## 入口

```go
type Chunk struct {
    Seq     int
    Heading string // 例如 "人事制度 > 休假 > 年假"
    Text    string
}

func Split(doc loader.Document, size, overlap int) []Chunk
```

参数来自配置：默认 `size=500`、`overlap=80`，必须满足 `size > overlap > 0`，否则返回空。

## 规则要点

- 按标题边界切分，片段不跨标题，`Heading` 由标题栈用 ` > ` 连接；
- 同一标题下普通正文按段落打包，超过 `size`（按 rune 计数）时输出；
- 超长段落按 `。！？；` / 换行切分，相邻片段重叠 `overlap` 字符；
- Markdown 表格独立成片；超大表格按行拆分并重复表头，单行不拆开；
- 少于 10 字符的碎片丢弃；
- `Seq` 在单文档内从 0 递增。

完整规则、示例与限制见 [文档切分规则](../../../docs/文档切分规则.md)。

## 测试

[chunker_test.go](chunker_test.go) 覆盖标题边界、嵌套路径、段落打包、100 字符上限、20 字符重叠、碎片丢弃、非法参数，以及表格完整、超大表格重复表头、表格与段落分离。

## 相关文档

- [文档切分规则](../../../docs/文档切分规则.md)
- [文档解析](../loader/README.md)
- [入库管道](../README.md)

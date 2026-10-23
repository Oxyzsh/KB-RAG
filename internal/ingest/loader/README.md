# internal/ingest/loader

把不同格式的文档统一解析为 `Document`，供切分模块使用。

## 数据模型

```go
type Block struct {
    Level int    // 0=正文段落；1-6=标题层级
    Text  string
}

type Document struct {
    Path   string
    Title  string
    Format string // md / txt / docx / pdf
    Blocks []Block
}
```

- `Blocks` 按文档顺序保留标题与正文，切分器据此维护标题路径。
- `Title`：Markdown/DOCX 取第一个一级标题；TXT/PDF 使用文件名（不含扩展名）。

## 分发

```go
func Supported(path string) bool
func Load(path string) (Document, error)
```

扩展名不区分大小写，支持 `.md`、`.markdown`、`.txt`、`.docx`、`.pdf`。

## 各格式行为

| 格式 | 实现 | 关键行为 | 限制 |
|------|------|----------|------|
| Markdown | `goldmark` AST | 提取 1-6 级标题、段落、列表文本、代码块 | 不做导航文本识别 |
| TXT | 按空行分段 | UTF-8 优先，非法 UTF-8 回退 GB18030；剥离 UTF-8 BOM | 无标题结构 |
| DOCX | `archive/zip` + `encoding/xml` | 读 `word/document.xml`；标题取 `pStyle` / `outlineLvl`；表格按行拼成 `单元格 \| 单元格` | 不读页眉页脚、脚注、批注 |
| PDF | `github.com/ledongthuc/pdf` | 逐页提取，插入 `第 N 页` 一级标题；标题取文件名 | 无真实标题；扫描件无文本；双栏/表格可能顺序错乱 |

PDF 解析对畸形文件做了 panic 恢复，单个坏文件不会终止整个 ingest；panic 只记录类型，不记录可能携带内容的异常值。

## 编码

TXT/Markdown 读取后：

1. `utf8.Valid` 为真：按 UTF-8 使用；
2. 否则用 GB18030 解码；
3. 两种编码都失败则返回错误。

## 测试

- [text_test.go](text_test.go)：扩展名识别、Markdown 标题、TXT 分段、GB18030、BOM、非法格式。
- [docx_test.go](docx_test.go)：标题样式/outlineLvl、制表符、表格行、损坏 zip。
- [pdf_test.go](pdf_test.go)：中文 PDF 提取、损坏 PDF 报错。
- [testdata/sample_zh.pdf](testdata/sample_zh.pdf)：用 `simhei.ttf` 经 `go-pdf/fpdf` 生成的中文测试夹具。

## 相关文档

- [入库管道](../README.md)
- [切分](../chunker/README.md)

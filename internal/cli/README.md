# internal/cli

CLI 命令层：解析三个子命令，装配配置、日志、存储、嵌入与生成客户端，并把结果写到 stdout/stderr。

## 子命令

| 命令 | 参数 | 作用 |
|------|------|------|
| `ingest` | `--config <path>`、目录 | 扫描目录并增量入库 |
| `ask` | `--config <path>`、`--top-k N`（默认 8）、问题 | 检索并生成带引用的答案 |
| `stats` | `--config <path>` | 输出文档/片段数量、失败路径、嵌入元信息与库大小 |

`--config` 默认 `./config.yaml`。Go 标准库 `flag` 在第一个位置参数处停止解析，因此 flag 必须写在目录/问题之前。

## 入口

```go
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int
```

`Run` 不直接读 `os.Args` 或写 `os.Stdout`，便于在测试中注入缓冲区和假服务。`cmd/KB-RAG` 负责传入真实标准流。

## 执行流程

```text
解析 flag
  → config.Load + Validate（按命令校验）
  → logging.New（stderr + 文件）
  → store.Open
  → ingest / query / stats
  → 打印结果，返回退出码
```

- `ingest`：调用 `ingest.Run`，打印“成功/跳过/失败”和失败路径；有失败返回 1。
- `ask`：调用 `query.Service.Ask`；生成失败但检索成功时，仍把检索片段打印到 stdout，并在 stderr 记警告，返回 1。
- `stats`：调用 `store.Stats`；库为空时显示“嵌入模型：未初始化”。

## 退出码

| 退出码 | 场景 |
|--------|------|
| 0 | 成功 |
| 1 | 运行期错误、部分文件失败、服务不可达、生成失败 |
| 2 | 配置/参数错误、未知命令、缺少必填参数 |
| 130 | `ctx` 被取消（Ctrl+C） |

## 测试

[cli_test.go](cli_test.go) 使用 `httptest` 启动假 Ollama 与假 OpenAI 兼容服务，覆盖：

- 临时目录 → `ingest` → `ask` → `stats` 全链路；
- 重复 ingest 的跳过行为；
- prompt 中包含来源、标题与问题；
- 生成失败时输出检索片段；
- 缺少 API Key、无参数/未知命令的退出码；
- 中断时 `stats` 返回 130。

## 相关文档

- [配置模块](../config/README.md)
- [日志模块](../logging/README.md)
- [入库管道](../ingest/README.md)
- [查询模块](../query/README.md)
- [存储模块](../store/README.md)

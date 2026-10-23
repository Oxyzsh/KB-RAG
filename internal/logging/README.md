# internal/logging

基于 Go 标准库 `log/slog` 的日志初始化，同时写 stderr 与文件，并按大小轮转。

## 接口

```go
func New(cfg config.LogConfig, command string, stderr io.Writer) (*slog.Logger, io.Closer, error)
```

- 返回的 logger 自动带 `command=<ingest|ask|stats>` 字段。
- 第二个返回值是轮转写入器，调用方应在命令结束时 `Close()`。
- 使用 `slog.NewTextHandler`，同时写 `stderr` 和 `lumberjack.Logger`。

## 轮转

| 参数 | 值 |
|------|----|
| 单文件上限 | 10 MB |
| 保留份数 | 5 |
| 压缩 | 关闭 |

## 隐私约定

- 文档正文、问题原文不写入日志。
- 问题只记录 `question_len`、`duration_ms`、`top_k`、`chunk_ids`、`scores`。
- API Key 永不写入日志。
- HTTP 客户端错误只保留状态码，不拼接响应体，避免服务端回显内容。

## 测试

[logging_test.go](logging_test.go) 验证日志同时出现在 stderr 与文件、包含 `command` 字段与业务字段。

## 相关文档

- [配置模块](../config/README.md)
- [CLI 模块](../cli/README.md)

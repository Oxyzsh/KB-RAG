# cmd/KB-RAG

KB-RAG 的进程入口。这里只做两件事：

1. 把 `os.Interrupt`（Ctrl+C）转换成可取消的 `context.Context`；
2. 调用 `internal/cli.Run`，用其返回值作为进程退出码。

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()
os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
```

## 约定

- 参数解析、配置加载、日志初始化、依赖装配全部在 [internal/cli](../../internal/cli/README.md)，这里不写业务逻辑。
- 退出码：`0` 成功；`1` 运行期错误或部分文件失败；`2` 配置/参数错误；`130` 用户中断。
- 构建：`go build -o KB-RAG.exe ./cmd/KB-RAG`。

## 相关文档

- [CLI 模块](../../internal/cli/README.md)
- [根 README](../../README.md)

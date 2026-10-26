# internal/query

把“问题嵌入 → 检索 → 拼上下文 → 生成 → 引用格式化”串起来。

## 入口

```go
type Service struct {
    Embedder   Embedder
    Store      Store
    LLM        LLM
    EmbedModel string
    TopK       int
    BaseDir    string
    Logger     *slog.Logger
}

func (s *Service) Ask(ctx context.Context, question string) (Result, error)
```

`Result` 包含答案、命中的 `store.Hit` 列表以及是否真正生成成功（`Generated`）。

- `Ask` 使用 Service 的固定 `TopK`，供 CLI 使用；
- `AskWithTopK(ctx, question, topK)` 允许按请求指定 top-k，供 Web API 使用；
- `Retrieve` 只做校验、问题嵌入与检索；`Generate` / `GenerateStream` 负责生成，供 SSE 接口先返回引用再流式输出；
- `TenantID` 决定检索哪个租户；`WithTenant(id)` 返回设置了租户的浅拷贝，供 Web 按当前登录用户隔离；
- `ErrEmptyStore` / `ErrNoResults` / `ErrModelMismatch` 用于 API 区分 409 响应。

## 流程

1. 校验库内元信息：`meta.model` 与配置的嵌入模型一致、维度已初始化；
2. 用嵌入客户端把问题转成向量，并校验维度；
3. 调用 `Store.Search`，默认 top-k 8；
4. 按编号拼装资料上下文；
5. 调用 LLM 生成；
6. 返回答案与来源。

## Prompt

系统提示词约束模型：

- 只依据“资料”回答，禁止使用资料外信息；
- 资料中没有答案时只回答“知识库中未找到相关依据”；
- 每个结论后用 `[n]` 标注资料编号；
- 中文、简洁。

用户消息格式：

```text
资料：
[1] 来源：docs/人事制度.md
    标题：人事制度 > 休假
    内容：……

问题：年假怎么请
```

来源路径相对当前工作目录，统一使用 `/`。

## 输出格式

```text
答案正文[1]。

引用来源：
[1] docs/人事制度.md > 人事制度 > 休假
[2] docs/食堂.txt
```

- `FormatResult`：答案 + 引用来源；
- `FormatHits`：生成失败时输出的检索片段；
- `DisplayPath`：把绝对路径转成相对 `BaseDir` 的路径。

## 失败语义

- 库为空、模型/维度不一致、嵌入或检索失败：返回错误，没有命中；
- 生成失败但检索成功：返回 `Result{Hits: ...}` 与错误，CLI 打印检索片段并以退出码 1 结束。

## 日志

只记录问题长度、耗时、top-k 数量、片段 ID 与分数，不记录问题原文或片段正文。

## 测试

[query_test.go](query_test.go) 覆盖 prompt 组装、模型/维度不一致、空库、生成失败保留命中、引用格式化与相对路径。

## 相关文档

- [存储](../store/README.md)
- [生成客户端](../llm/README.md)
- [CLI](../cli/README.md)

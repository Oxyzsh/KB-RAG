# internal/llm

OpenAI 兼容的 `/chat/completions` 客户端，用于生成带引用的答案。

## 接口

```go
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

func New(baseURL, model, apiKey string) *Client
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error)
```

## 请求

```http
POST {base_url}/chat/completions
Authorization: Bearer <KB_RAG_LLM_API_KEY>
Content-Type: application/json

{
  "model": "deepseek-chat",
  "messages": [...],
  "temperature": 0.2,
  "max_tokens": 1024
}
```

| 项目 | 约定 |
|------|------|
| 超时 | 90 秒 |
| 温度 | 0.2 |
| 最大输出 | 1024 tokens |
| 返回 | 取 `choices[0].message.content`，去首尾空白 |
| 空结果 | `choices` 为空或内容为空均返回错误 |
| 错误 | 非 2xx 只保留 HTTP 状态码，不拼接响应体；不打印 API Key |

第一期不做流式输出，拿到完整答案后一次性返回。

## 测试

[client_test.go](client_test.go) 验证请求路径、鉴权头、模型/温度/最大输出参数，以及 HTTP 错误、空 choices、错误信息不泄露 Key 与响应体。

## 相关文档

- [查询模块](../query/README.md)
- [配置模块](../config/README.md)

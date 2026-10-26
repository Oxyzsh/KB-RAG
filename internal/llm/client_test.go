package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatSendsExpectedRequest(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "答案[1]。"}}},
		})
	}))
	defer server.Close()

	client := New(server.URL+"/v1", "deepseek-chat", "secret")
	answer, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "规则"},
		{Role: "user", Content: "问题"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "答案[1]。" {
		t.Errorf("answer = %q", answer)
	}
	if got.Model != "deepseek-chat" || got.Temperature != 0.2 || got.MaxTokens != 1024 {
		t.Errorf("请求参数 = %+v", got)
	}
	if len(got.Messages) != 2 || got.Messages[1].Content != "问题" {
		t.Errorf("messages = %+v", got.Messages)
	}
}

func TestChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	if _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}}); err == nil {
		t.Fatal("期望 HTTP 错误")
	}
}

func TestChatEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	if _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}}); err == nil {
		t.Fatal("期望空 choices 报错")
	}
}

func TestChatErrorMessageDoesNotLeakKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()
	client := New(server.URL, "m", "super-secret-key")
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}})
	if err == nil {
		t.Fatal("期望错误")
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Fatal("错误信息泄露 API Key")
	}
}

func TestChatErrorDoesNotEchoResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "document text and question", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "q"}})
	if err == nil {
		t.Fatal("期望错误")
	}
	if strings.Contains(err.Error(), "document text") {
		t.Fatal("错误信息回显了响应体")
	}
}

func TestChatStream(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, text := range []string{"员工", "满1年", "享5天年假[1]。"} {
			_, _ = w.Write([]byte("data: " + `{"choices":[{"delta":{"content":"` + text + `"}}]}` + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := New(server.URL, "m", "k")
	var answer strings.Builder
	err := client.ChatStream(context.Background(), []Message{{Role: "user", Content: "q"}}, func(delta string) error {
		answer.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if answer.String() != "员工满1年享5天年假[1]。" {
		t.Fatalf("answer = %q", answer.String())
	}
	if !got.Stream {
		t.Fatal("请求未设置 stream=true")
	}
}

func TestChatStreamEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	if err := client.ChatStream(context.Background(), []Message{{Role: "user", Content: "q"}}, func(string) error { return nil }); err == nil {
		t.Fatal("空流应报错")
	}
}

func TestChatStreamHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret document text", http.StatusBadGateway)
	}))
	defer server.Close()
	client := New(server.URL, "m", "k")
	err := client.ChatStream(context.Background(), []Message{{Role: "user", Content: "q"}}, func(string) error { return nil })
	if err == nil {
		t.Fatal("期望错误")
	}
	if strings.Contains(err.Error(), "secret document text") {
		t.Fatal("错误信息回显了响应体")
	}
}

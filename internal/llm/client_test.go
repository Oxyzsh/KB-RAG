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

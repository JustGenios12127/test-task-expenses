package groq

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"test-task-expenses/backend/internal/domain/chat"
)

func TestCompleteSendsDialogueAndReadsToolCalls(t *testing.T) {
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Role       string `json:"role"`
			Content    any    `json:"content"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name       string         `json:"name"`
				Parameters map[string]any `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_2","type":"function","function":{"name":"delete_expense","arguments":"{\"id\":7}"}}]}}]}`))
	}))
	defer server.Close()

	client := New("secret", "test-model", server.URL+"/v1", option.WithHTTPClient(server.Client()))
	answer, err := client.Complete(context.Background(), "system text", []chat.Message{
		{Role: chat.RoleUser, Text: "удали такси"},
		{Role: chat.RoleAssistant, ToolCalls: []chat.ToolCall{{ID: "call_1", Name: "list_expenses", Arguments: `{"search":"такси"}`}}},
		{Role: chat.RoleTool, ToolCallID: "call_1", Text: `{"count":1}`},
	}, []chat.Tool{{Name: "delete_expense", Description: "Удалить", Parameters: map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if auth != "Bearer secret" || got.Model != "test-model" {
		t.Fatalf("auth = %q, model = %q", auth, got.Model)
	}
	if len(got.Messages) != 4 || got.Messages[0].Role != "system" || got.Messages[0].Content != "system text" {
		t.Fatalf("messages = %+v", got.Messages)
	}
	assistant := got.Messages[2]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_1" || assistant.ToolCalls[0].Function.Name != "list_expenses" {
		t.Fatalf("assistant = %+v", assistant)
	}
	if tool := got.Messages[3]; tool.Role != "tool" || tool.ToolCallID != "call_1" {
		t.Fatalf("tool = %+v", tool)
	}
	if len(got.Tools) != 1 || got.Tools[0].Type != "function" || got.Tools[0].Function.Name != "delete_expense" {
		t.Fatalf("tools = %+v", got.Tools)
	}

	if answer.Role != chat.RoleAssistant || len(answer.ToolCalls) != 1 {
		t.Fatalf("answer = %+v", answer)
	}
	if call := answer.ToolCalls[0]; call.ID != "call_2" || call.Name != "delete_expense" || call.Arguments != `{"id":7}` {
		t.Fatalf("call = %+v", call)
	}
}

func TestCompleteReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API Key","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	client := New("bad", "m", server.URL, option.WithHTTPClient(server.Client()), option.WithMaxRetries(0))
	if _, err := client.Complete(context.Background(), "s", []chat.Message{{Role: chat.RoleUser, Text: "hi"}}, nil); err == nil {
		t.Fatal("expected error")
	}
}

package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendText(t *testing.T) {
	var gotPath string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()

	client := testClient(t, server)
	if err := client.SendText(context.Background(), 15, "Привет!"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if gotPath != "/bottoken/sendMessage" {
		t.Fatalf("path = %s", gotPath)
	}
	if !strings.Contains(gotBody, `"chat_id":15`) || !strings.Contains(gotBody, `"text":"Привет!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestGetUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/getUpdates" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("offset") != "11" {
			t.Errorf("offset = %s", r.URL.Query().Get("offset"))
		}
		if r.URL.Query().Get("allowed_updates") != `["message"]` {
			t.Errorf("allowed_updates = %s", r.URL.Query().Get("allowed_updates"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"message":{"text":"/start","chat":{"id":5},"from":{"first_name":"Оля"}}}]}`))
	}))
	defer server.Close()

	client := testClient(t, server)
	updates, err := client.GetUpdates(context.Background(), 11)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(updates))
	}
	if updates[0].UpdateID != 10 || updates[0].Message.Chat.ID != 5 || updates[0].Message.From.FirstName != "Оля" {
		t.Fatalf("update = %+v", updates[0])
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	}))
	defer server.Close()

	client := testClient(t, server)
	err := client.SendText(context.Background(), 1, "hi")
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("error = %v", err)
	}
}

func testClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client := NewClient("token")
	client.baseURL = server.URL
	client.httpClient = server.Client()
	return client
}

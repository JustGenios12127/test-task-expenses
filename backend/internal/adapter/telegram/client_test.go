package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tele "gopkg.in/telebot.v3"
)

func TestSendText(t *testing.T) {
	var gotPath string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":15}}}`))
	}))
	defer server.Close()

	client := testClient(t, server, nil)
	if err := client.SendText(context.Background(), 15, "Привет!"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if gotPath != "/bottoken/sendMessage" {
		t.Fatalf("path = %s", gotPath)
	}
	if !strings.Contains(gotBody, `"chat_id":"15"`) || !strings.Contains(gotBody, `"text":"Привет!"`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/getChat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"type":"private","username":"anna_k","first_name":"Анна"}}`))
	}))
	defer server.Close()

	username, firstName, err := testClient(t, server, nil).Profile(context.Background(), 42)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if username != "anna_k" || firstName != "Анна" {
		t.Fatalf("profile = %q %q", username, firstName)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	}))
	defer server.Close()

	err := testClient(t, server, nil).SendText(context.Background(), 1, "hi")
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("error = %v", err)
	}
}

func TestNetworkErrorHidesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := testClient(t, server, nil)
	server.Close()

	err := client.SendText(context.Background(), 1, "hi")
	if err == nil || strings.Contains(err.Error(), "token") {
		t.Fatalf("error = %v", err)
	}
}

func TestSendTextStopsOnContext(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := testClient(t, server, nil).SendText(ctx, 1, "hi"); err == nil {
		t.Fatal("expected context error")
	}
}

func testClient(t *testing.T, server *httptest.Server, p tele.Poller) *Client {
	t.Helper()
	if p == nil {
		p = &poller{timeout: 0, retry: 20 * time.Millisecond}
	}
	client, err := newClient(tele.Settings{
		Token:       "token",
		URL:         server.URL,
		Poller:      p,
		Synchronous: true,
		Offline:     true,
		OnError:     logError,
		Client:      server.Client(),
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}

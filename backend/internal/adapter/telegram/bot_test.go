package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"test-task-expenses/backend/internal/app/bot"
	expenseapp "test-task-expenses/backend/internal/app/expense"
)

type fakeStart struct {
	cmd    bot.StartCommand
	called bool
	err    error
}

func (f *fakeStart) HandleStart(_ context.Context, cmd bot.StartCommand) error {
	f.called = true
	f.cmd = cmd
	return f.err
}

type fakeExpenses struct {
	cmd    expenseapp.Command
	called bool
}

func (f *fakeExpenses) Handle(_ context.Context, cmd expenseapp.Command) error {
	f.called = true
	f.cmd = cmd
	return nil
}

func TestDispatchStart(t *testing.T) {
	start := &fakeStart{}
	expenses := &fakeExpenses{}
	err := dispatch(context.Background(), start, expenses, Update{
		UpdateID: 3,
		Message: &Message{
			Text: "/start@ExpenseBot сейчас",
			From: &User{FirstName: "Марк"},
			Chat: Chat{ID: 99},
		},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !start.called {
		t.Fatal("start handler was not called")
	}
	if expenses.called {
		t.Fatal("expense handler was called for /start")
	}
	if start.cmd.ChatID != 99 || start.cmd.FirstName != "Марк" {
		t.Fatalf("command = %+v", start.cmd)
	}
}

func TestDispatchExpenseSeparatesUserAndChat(t *testing.T) {
	start := &fakeStart{}
	expenses := &fakeExpenses{}
	err := dispatch(context.Background(), start, expenses, Update{
		Message: &Message{
			Text: "кофе 350",
			From: &User{ID: 55, FirstName: "Анна"},
			Chat: Chat{ID: 77},
		},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if start.called {
		t.Fatal("start handler was called for an expense")
	}
	if !expenses.called || expenses.cmd.UserID != 55 || expenses.cmd.ChatID != 77 || expenses.cmd.Text != "кофе 350" {
		t.Fatalf("command = %+v", expenses.cmd)
	}
}

func TestMessageProfileReadsNick(t *testing.T) {
	id, username, firstName, ok := messageProfile(Update{
		Message: &Message{
			Text: "кофе 350",
			From: &User{ID: 55, Username: "anna_k", FirstName: "Анна"},
		},
	})
	if !ok || id != 55 || username != "anna_k" || firstName != "Анна" {
		t.Fatalf("profile = %d %q %q ok=%v", id, username, firstName, ok)
	}
	if _, _, _, ok := messageProfile(Update{}); ok {
		t.Fatal("expected no profile")
	}
}

func TestDispatchExpenseRequiresSender(t *testing.T) {
	expenses := &fakeExpenses{}
	err := dispatch(context.Background(), &fakeStart{}, expenses, Update{
		Message: &Message{Text: "кофе 350", Chat: Chat{ID: 1}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if expenses.called {
		t.Fatal("expense handler was called without a sender")
	}
}

func TestDispatchReturnsHandlerError(t *testing.T) {
	start := &fakeStart{err: errors.New("send failed")}
	err := dispatch(context.Background(), start, &fakeExpenses{}, Update{
		Message: &Message{Text: "/start", Chat: Chat{ID: 1}},
	})
	if !errors.Is(err, start.err) {
		t.Fatalf("error = %v", err)
	}
}

type blockingExpenses struct {
	release chan struct{}
	handled chan expenseapp.Command
}

func (f *blockingExpenses) Handle(ctx context.Context, cmd expenseapp.Command) error {
	if cmd.UserID == 1 {
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.handled <- cmd
	return nil
}

func TestRunDoesNotBlockOtherUsers(t *testing.T) {
	var served atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getUpdates") && served.CompareAndSwap(false, true) {
			_, _ = w.Write([]byte(`{"ok":true,"result":[
				{"update_id":1,"message":{"text":"кофе 350","chat":{"id":1},"from":{"id":1}}},
				{"update_id":2,"message":{"text":"такси 500","chat":{"id":1},"from":{"id":1}}},
				{"update_id":3,"message":{"text":"чай 100","chat":{"id":2},"from":{"id":2}}}
			]}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			select {
			case <-r.Context().Done():
			case <-time.After(200 * time.Millisecond):
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer server.Close()

	expenses := &blockingExpenses{release: make(chan struct{}), handled: make(chan expenseapp.Command, 3)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		NewBot(testClient(t, server), &fakeStart{}, expenses, nil).Run(ctx)
	}()

	select {
	case cmd := <-expenses.handled:
		if cmd.UserID != 2 {
			t.Fatalf("first handled = %+v", cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second user waited for the first one")
	}

	close(expenses.release)
	first := <-expenses.handled
	second := <-expenses.handled
	if first.Text != "кофе 350" || second.Text != "такси 500" {
		t.Fatalf("order = %q, %q", first.Text, second.Text)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bot did not stop")
	}
}

func TestIsStartCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "/start", want: true},
		{text: "/Start", want: true},
		{text: "  /start  ", want: true},
		{text: "/start payload", want: true},
		{text: "/start@my_bot", want: true},
		{text: "/stop", want: false},
		{text: "кофе 350", want: false},
		{text: "", want: false},
		{text: "смотри /start", want: false},
	}

	for _, tt := range tests {
		if got := isStartCommand(tt.text); got != tt.want {
			t.Errorf("isStartCommand(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

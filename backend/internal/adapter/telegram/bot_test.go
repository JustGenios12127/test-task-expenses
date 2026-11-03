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

	tele "gopkg.in/telebot.v3"

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

type fakeProfiles struct {
	id        int64
	username  string
	firstName string
}

func (f *fakeProfiles) SaveUser(_ context.Context, id int64, username, firstName string) error {
	f.id, f.username, f.firstName = id, username, firstName
	return nil
}

func TestProcessStart(t *testing.T) {
	start := &fakeStart{}
	expenses := &fakeExpenses{}
	b := &Bot{start: start, expenses: expenses}
	err := b.process(context.Background(), job{start: true, msg: &tele.Message{
		Text:   "/start",
		Sender: &tele.User{FirstName: "Марк"},
		Chat:   &tele.Chat{ID: 99},
	}})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if !start.called || expenses.called {
		t.Fatalf("start called = %v, expenses called = %v", start.called, expenses.called)
	}
	if start.cmd.ChatID != 99 || start.cmd.FirstName != "Марк" {
		t.Fatalf("command = %+v", start.cmd)
	}
}

func TestProcessExpenseSeparatesUserAndChat(t *testing.T) {
	expenses := &fakeExpenses{}
	profiles := &fakeProfiles{}
	b := &Bot{start: &fakeStart{}, expenses: expenses, profiles: profiles}
	err := b.process(context.Background(), job{msg: &tele.Message{
		Text:   " кофе 350 ",
		Sender: &tele.User{ID: 55, Username: "anna_k", FirstName: "Анна"},
		Chat:   &tele.Chat{ID: 77},
	}})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if !expenses.called || expenses.cmd.UserID != 55 || expenses.cmd.ChatID != 77 || expenses.cmd.Text != "кофе 350" {
		t.Fatalf("command = %+v", expenses.cmd)
	}
	if profiles.id != 55 || profiles.username != "anna_k" || profiles.firstName != "Анна" {
		t.Fatalf("profile = %+v", profiles)
	}
}

func TestProcessExpenseRequiresSender(t *testing.T) {
	expenses := &fakeExpenses{}
	b := &Bot{start: &fakeStart{}, expenses: expenses}
	err := b.process(context.Background(), job{msg: &tele.Message{Text: "кофе 350", Chat: &tele.Chat{ID: 1}}})
	if err == nil || expenses.called {
		t.Fatalf("err = %v, called = %v", err, expenses.called)
	}
}

func TestProcessReturnsHandlerError(t *testing.T) {
	start := &fakeStart{err: errors.New("send failed")}
	b := &Bot{start: start, expenses: &fakeExpenses{}}
	err := b.process(context.Background(), job{start: true, msg: &tele.Message{Text: "/start", Chat: &tele.Chat{ID: 1}}})
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

type recordingStart struct {
	called chan bot.StartCommand
}

func (f *recordingStart) HandleStart(_ context.Context, cmd bot.StartCommand) error {
	f.called <- cmd
	return nil
}

func TestRunRoutesUpdatesWithoutBlockingOtherUsers(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":100,"is_bot":true,"username":"family_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			switch polls.Add(1) {
			case 1:
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
			case 2:
				_, _ = w.Write([]byte(`{"ok":true,"result":[
					{"update_id":1,"message":{"message_id":1,"text":"кофе 350","chat":{"id":1},"from":{"id":1}}},
					{"update_id":2,"message":{"message_id":2,"text":"такси 500","chat":{"id":1},"from":{"id":1}}},
					{"update_id":3,"message":{"message_id":3,"text":"чай 100","chat":{"id":2},"from":{"id":2}}},
					{"update_id":4,"message":{"message_id":4,"text":"/start@family_bot","entities":[{"type":"bot_command","offset":0,"length":17}],"chat":{"id":3},"from":{"id":3,"first_name":"Оля"}}}
				]}`))
			default:
				select {
				case <-r.Context().Done():
				case <-time.After(100 * time.Millisecond):
				}
				_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
			}
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	defer server.Close()

	expenses := &blockingExpenses{release: make(chan struct{}), handled: make(chan expenseapp.Command, 3)}
	start := &recordingStart{called: make(chan bot.StartCommand, 1)}
	b := NewBot(testClient(t, server, nil), start, expenses, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Run(ctx)
	}()

	select {
	case cmd := <-expenses.handled:
		if cmd.UserID != 2 {
			t.Fatalf("first handled = %+v", cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second user waited for the first one")
	}
	select {
	case cmd := <-start.called:
		if cmd.ChatID != 3 || cmd.FirstName != "Оля" {
			t.Fatalf("start = %+v", cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("start command was not handled")
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

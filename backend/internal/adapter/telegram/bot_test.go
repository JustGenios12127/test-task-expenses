package telegram

import (
	"context"
	"errors"
	"testing"

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

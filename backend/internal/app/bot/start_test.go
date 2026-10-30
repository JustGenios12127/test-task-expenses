package bot

import (
	"context"
	"errors"
	"testing"
)

type fakeMessenger struct {
	chatID int64
	text   string
	err    error
}

func (f *fakeMessenger) SendText(_ context.Context, chatID int64, text string) error {
	f.chatID = chatID
	f.text = text
	return f.err
}

func TestHandleStartGreetsByName(t *testing.T) {
	messenger := &fakeMessenger{}
	handler := NewStartHandler(messenger)

	err := handler.HandleStart(context.Background(), StartCommand{
		ChatID:    42,
		FirstName: "  Анна ",
	})
	if err != nil {
		t.Fatalf("HandleStart: %v", err)
	}
	if messenger.chatID != 42 {
		t.Fatalf("chat id = %d, want 42", messenger.chatID)
	}
	const want = "Привет, Анна!\n\nЯ помогу записывать расходы."
	if messenger.text != want {
		t.Fatalf("text = %q, want %q", messenger.text, want)
	}
}

func TestHandleStartWithoutName(t *testing.T) {
	messenger := &fakeMessenger{}
	handler := NewStartHandler(messenger)

	if err := handler.HandleStart(context.Background(), StartCommand{ChatID: 7}); err != nil {
		t.Fatalf("HandleStart: %v", err)
	}
	const want = "Привет!\n\nЯ помогу записывать расходы."
	if messenger.text != want {
		t.Fatalf("text = %q, want %q", messenger.text, want)
	}
}

func TestHandleStartReturnsMessengerError(t *testing.T) {
	messenger := &fakeMessenger{err: errors.New("network down")}
	handler := NewStartHandler(messenger)

	err := handler.HandleStart(context.Background(), StartCommand{ChatID: 1, FirstName: "Илья"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, messenger.err) {
		t.Fatalf("error = %v, want wrapped messenger error", err)
	}
}

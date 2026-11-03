package bot

import (
	"context"
	"fmt"

	"test-task-expenses/backend/internal/app/replies"
)

type Messenger interface {
	SendText(ctx context.Context, chatID int64, text string) error
}

type StartCommand struct {
	ChatID    int64
	FirstName string
}

type StartHandler struct {
	messenger Messenger
}

func NewStartHandler(messenger Messenger) *StartHandler {
	return &StartHandler{messenger: messenger}
}

func (h *StartHandler) HandleStart(ctx context.Context, cmd StartCommand) error {
	if err := h.messenger.SendText(ctx, cmd.ChatID, replies.Greeting(cmd.FirstName)); err != nil {
		return fmt.Errorf("send greeting: %w", err)
	}
	return nil
}

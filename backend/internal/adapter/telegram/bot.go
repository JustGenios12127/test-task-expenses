package telegram

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"test-task-expenses/backend/internal/app/bot"
)

var _ bot.Messenger = (*Client)(nil)

type startHandler interface {
	HandleStart(ctx context.Context, cmd bot.StartCommand) error
}

type Bot struct {
	client *Client
	start  startHandler
}

func NewBot(client *Client, start startHandler) *Bot {
	return &Bot{client: client, start: start}
}

func (b *Bot) Run(ctx context.Context) {
	slog.Info("telegram bot started")
	defer slog.Info("telegram bot stopped")

	if err := b.client.DeleteWebhook(ctx); err != nil && ctx.Err() == nil {
		slog.Error("delete telegram webhook", "err", err)
	}

	var offset int64
	for {
		if ctx.Err() != nil {
			return
		}

		updates, err := b.client.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("telegram get updates", "err", err)
			if !wait(ctx, 3*time.Second) {
				return
			}
			continue
		}

		for _, upd := range updates {
			if next := upd.UpdateID + 1; next > offset {
				offset = next
			}
			if err := dispatch(ctx, b.start, upd); err != nil {
				slog.Error("telegram update", "err", err, "update_id", upd.UpdateID)
			}
		}
	}
}

func dispatch(ctx context.Context, start startHandler, upd Update) error {
	if upd.Message == nil || !isStartCommand(upd.Message.Text) {
		return nil
	}

	name := ""
	if upd.Message.From != nil {
		name = upd.Message.From.FirstName
	}
	return start.HandleStart(ctx, bot.StartCommand{
		ChatID:    upd.Message.Chat.ID,
		FirstName: name,
	})
}

func isStartCommand(text string) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return false
	}
	command := strings.ToLower(parts[0])
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return command == "/start"
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

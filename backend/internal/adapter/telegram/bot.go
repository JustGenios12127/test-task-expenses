package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"test-task-expenses/backend/internal/app/bot"
	expenseapp "test-task-expenses/backend/internal/app/expense"
)

var (
	_ bot.Messenger     = (*Client)(nil)
	_ expenseapp.Sender = (*Client)(nil)
)

type startHandler interface {
	HandleStart(ctx context.Context, cmd bot.StartCommand) error
}

type expenseHandler interface {
	Handle(ctx context.Context, cmd expenseapp.Command) error
}

type profileStore interface {
	SaveUser(ctx context.Context, id int64, username, firstName string) error
}

type Bot struct {
	client   *Client
	start    startHandler
	expenses expenseHandler
	profiles profileStore
}

func NewBot(client *Client, start startHandler, expenses expenseHandler, profiles profileStore) *Bot {
	return &Bot{client: client, start: start, expenses: expenses, profiles: profiles}
}

func (b *Bot) Run(ctx context.Context) {
	slog.Info("telegram bot started")
	defer slog.Info("telegram bot stopped")

	if err := b.client.DeleteWebhook(ctx); err != nil && ctx.Err() == nil {
		slog.Error("delete telegram webhook", "err", err)
	}

	queues := map[int64]chan Update{}
	var workers sync.WaitGroup
	defer workers.Wait()

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
			key, ok := queueKey(upd)
			if !ok {
				continue
			}
			queue, exists := queues[key]
			if !exists {
				queue = make(chan Update, 32)
				queues[key] = queue
				workers.Add(1)
				go func() {
					defer workers.Done()
					b.serve(ctx, queue)
				}()
			}
			select {
			case queue <- upd:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (b *Bot) serve(ctx context.Context, queue <-chan Update) {
	for {
		select {
		case <-ctx.Done():
			return
		case upd := <-queue:
			b.remember(ctx, upd)
			if err := dispatch(ctx, b.start, b.expenses, upd); err != nil {
				slog.Error("telegram update", "err", err, "update_id", upd.UpdateID)
			}
		}
	}
}

func queueKey(upd Update) (int64, bool) {
	if upd.Message == nil {
		return 0, false
	}
	if upd.Message.From != nil && upd.Message.From.ID > 0 {
		return upd.Message.From.ID, true
	}
	return upd.Message.Chat.ID, true
}

func (b *Bot) remember(ctx context.Context, upd Update) {
	if b.profiles == nil {
		return
	}
	id, username, firstName, ok := messageProfile(upd)
	if !ok {
		return
	}
	if err := b.profiles.SaveUser(ctx, id, username, firstName); err != nil {
		slog.Error("save telegram profile", "err", err, "user_id", id)
	}
}

func messageProfile(upd Update) (int64, string, string, bool) {
	if upd.Message == nil || upd.Message.From == nil || upd.Message.From.ID <= 0 {
		return 0, "", "", false
	}
	user := upd.Message.From
	return user.ID, user.Username, user.FirstName, true
}

func dispatch(ctx context.Context, start startHandler, expenses expenseHandler, upd Update) error {
	if upd.Message == nil {
		return nil
	}
	text := strings.TrimSpace(upd.Message.Text)
	if text == "" {
		return nil
	}
	if isStartCommand(text) {
		name := ""
		if upd.Message.From != nil {
			name = upd.Message.From.FirstName
		}
		return start.HandleStart(ctx, bot.StartCommand{
			ChatID:    upd.Message.Chat.ID,
			FirstName: name,
		})
	}
	if upd.Message.From == nil || upd.Message.From.ID <= 0 {
		return fmt.Errorf("telegram message has no sender")
	}
	return expenses.Handle(ctx, expenseapp.Command{
		UserID: upd.Message.From.ID,
		ChatID: upd.Message.Chat.ID,
		Text:   text,
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

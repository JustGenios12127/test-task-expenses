package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"

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

type job struct {
	start bool
	msg   *tele.Message
}

type Bot struct {
	client   *Client
	start    startHandler
	expenses expenseHandler
	profiles profileStore
	ctx      context.Context
	queues   map[int64]chan job
	workers  sync.WaitGroup
}

func NewBot(client *Client, start startHandler, expenses expenseHandler, profiles profileStore) *Bot {
	b := &Bot{
		client:   client,
		start:    start,
		expenses: expenses,
		profiles: profiles,
		ctx:      context.Background(),
		queues:   map[int64]chan job{},
	}
	client.api.Handle("/start", func(c tele.Context) error {
		return b.enqueue(job{start: true, msg: c.Message()})
	})
	client.api.Handle(tele.OnText, func(c tele.Context) error {
		return b.enqueue(job{msg: c.Message()})
	})
	return b
}

func (b *Bot) Run(ctx context.Context) {
	slog.Info("telegram bot started")
	defer slog.Info("telegram bot stopped")

	for {
		err := b.client.connect()
		if err == nil {
			break
		}
		slog.Error("telegram connect", "err", err)
		if !wait(ctx, retryDelay) {
			return
		}
	}

	b.ctx = ctx
	defer b.workers.Wait()
	go func() {
		<-ctx.Done()
		b.client.api.Stop()
	}()
	b.client.api.Start()
}

func (b *Bot) enqueue(j job) error {
	if j.msg == nil || j.msg.Chat == nil {
		return nil
	}
	key := j.msg.Chat.ID
	if j.msg.Sender != nil && j.msg.Sender.ID > 0 {
		key = j.msg.Sender.ID
	}
	queue, ok := b.queues[key]
	if !ok {
		queue = make(chan job, 32)
		b.queues[key] = queue
		b.workers.Go(func() {
			b.serve(queue)
		})
	}
	select {
	case queue <- j:
	case <-b.ctx.Done():
	}
	return nil
}

func (b *Bot) serve(queue <-chan job) {
	for {
		select {
		case <-b.ctx.Done():
			return
		case j := <-queue:
			if err := b.process(b.ctx, j); err != nil {
				slog.Error("telegram message", "err", err, "chat_id", j.msg.Chat.ID)
			}
		}
	}
}

func (b *Bot) process(ctx context.Context, j job) error {
	msg := j.msg
	b.remember(ctx, msg.Sender)
	if j.start {
		name := ""
		if msg.Sender != nil {
			name = msg.Sender.FirstName
		}
		return b.start.HandleStart(ctx, bot.StartCommand{ChatID: msg.Chat.ID, FirstName: name})
	}
	if msg.Sender == nil || msg.Sender.ID <= 0 {
		return fmt.Errorf("telegram message has no sender")
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}
	return b.expenses.Handle(ctx, expenseapp.Command{
		UserID: msg.Sender.ID,
		ChatID: msg.Chat.ID,
		Text:   text,
	})
}

func (b *Bot) remember(ctx context.Context, user *tele.User) {
	if b.profiles == nil || user == nil || user.ID <= 0 {
		return
	}
	if err := b.profiles.SaveUser(ctx, user.ID, user.Username, user.FirstName); err != nil {
		slog.Error("save telegram profile", "err", err, "user_id", user.ID)
	}
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

package expense

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"test-task-expenses/backend/internal/app/replies"
	"test-task-expenses/backend/internal/domain/chat"
	domain "test-task-expenses/backend/internal/domain/expense"
)

const (
	maxSteps      = 6
	maxExchanges  = 4
	maxInputRunes = 1000
	maxReplyRunes = 3500
)

var (
	errEmptyReply   = errors.New("model returned an empty reply")
	errTooManySteps = errors.New("model did not finish in time")
)

type Sender interface {
	SendText(ctx context.Context, chatID int64, text string) error
}

type Model interface {
	Complete(ctx context.Context, system string, messages []chat.Message, tools []chat.Tool) (chat.Message, error)
}

type Store interface {
	Add(ctx context.Context, item domain.Expense) (domain.Expense, error)
	List(ctx context.Context, query domain.Query) ([]domain.Expense, error)
	Total(ctx context.Context, query domain.Query) (int64, int, error)
	Get(ctx context.Context, userID, id int64) (domain.Expense, error)
	Update(ctx context.Context, userID, id int64, patch domain.Patch) (domain.Expense, error)
	Delete(ctx context.Context, userID, id int64) (domain.Expense, error)
}

type Command struct {
	UserID int64
	ChatID int64
	Text   string
}

type Handler struct {
	model     Model
	store     Store
	messenger Sender
	now       func() time.Time
	mu        sync.Mutex
	history   map[int64][][]chat.Message
}

func NewHandler(model Model, store Store, messenger Sender, loc *time.Location) *Handler {
	if loc == nil {
		loc = time.UTC
	}
	return &Handler{
		model:     model,
		store:     store,
		messenger: messenger,
		history:   map[int64][][]chat.Message{},
		now: func() time.Time {
			return time.Now().In(loc)
		},
	}
}

func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	if cmd.UserID <= 0 {
		return domain.ErrUserRequired
	}
	if cmd.ChatID == 0 {
		return fmt.Errorf("chat id is required")
	}

	text := strings.TrimSpace(cmd.Text)
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "/") {
		return h.send(ctx, cmd, replies.NoCommands)
	}

	exchange := []chat.Message{{Role: chat.RoleUser, Text: limitText(text, maxInputRunes)}}
	reply, exchange, err := h.converse(ctx, cmd.UserID, h.now(), exchange)
	if err != nil {
		return errors.Join(err, h.send(ctx, cmd, replies.Failed))
	}
	h.remember(cmd.UserID, exchange)
	return h.send(ctx, cmd, reply)
}

func (h *Handler) converse(ctx context.Context, userID int64, today time.Time, exchange []chat.Message) (string, []chat.Message, error) {
	system, err := systemPrompt(today)
	if err != nil {
		return "", nil, err
	}
	for range maxSteps {
		answer, err := h.model.Complete(ctx, system, append(h.recent(userID), exchange...), tools)
		if err != nil {
			return "", nil, fmt.Errorf("model: %w", err)
		}
		exchange = append(exchange, answer)
		if len(answer.ToolCalls) == 0 {
			reply := strings.TrimSpace(answer.Text)
			if reply == "" {
				return "", nil, errEmptyReply
			}
			return limitText(reply, maxReplyRunes), exchange, nil
		}
		for _, call := range answer.ToolCalls {
			exchange = append(exchange, chat.Message{
				Role:       chat.RoleTool,
				ToolCallID: call.ID,
				Text:       h.execute(ctx, userID, call, today),
			})
		}
	}
	return "", nil, errTooManySteps
}

func (h *Handler) send(ctx context.Context, cmd Command, text string) error {
	if err := h.messenger.SendText(ctx, cmd.ChatID, text); err != nil {
		return fmt.Errorf("send expense reply: %w", err)
	}
	return nil
}

func (h *Handler) recent(userID int64) []chat.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []chat.Message
	for _, exchange := range h.history[userID] {
		out = append(out, exchange...)
	}
	return out
}

func (h *Handler) remember(userID int64, exchange []chat.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	items := append(h.history[userID], exchange)
	if len(items) > maxExchanges {
		items = items[len(items)-maxExchanges:]
	}
	h.history[userID] = items
}

func limitText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

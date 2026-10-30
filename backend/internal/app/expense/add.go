package expense

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

type Sender interface {
	SendText(ctx context.Context, chatID int64, text string) error
}

type Model interface {
	Recognize(ctx context.Context, text string, today time.Time, history []domain.Turn) (domain.Intent, error)
}

type Store interface {
	Add(ctx context.Context, item domain.Expense) (domain.Expense, error)
	List(ctx context.Context, query domain.Query) ([]domain.Expense, error)
	Total(ctx context.Context, query domain.Query) (int64, int, error)
	FindLatest(ctx context.Context, query domain.Query) (domain.Expense, error)
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
	memory    *chatMemory
}

type chatMemory struct {
	mu    sync.Mutex
	turns map[int64][]domain.Turn
	shown map[int64][]int64
}

func NewHandler(model Model, store Store, messenger Sender, loc *time.Location) *Handler {
	if loc == nil {
		loc = time.UTC
	}
	return &Handler{
		model:     model,
		store:     store,
		messenger: messenger,
		memory:    &chatMemory{},
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
		return h.reply(ctx, cmd, "Команды не нужны. Напиши трату, например: «кофе 350».")
	}

	today := h.now()
	intent, err := h.model.Recognize(ctx, text, today, h.recent(cmd.UserID))
	if err != nil {
		if errors.Is(err, domain.ErrNotExpense) {
			return h.reply(ctx, cmd, "Не понял. Можно записать трату, посмотреть, изменить или удалить. Например: «кофе 350».")
		}
		if sendErr := h.reply(ctx, cmd, "Не получилось обработать сообщение. Попробуй ещё раз."); sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return err
	}

	switch intent.Action {
	case domain.ActionAdd:
		return h.add(ctx, cmd, intent, today)
	case domain.ActionList:
		return h.list(ctx, cmd, intent, today)
	case domain.ActionUpdate:
		return h.update(ctx, cmd, intent, today)
	case domain.ActionDelete:
		return h.remove(ctx, cmd, intent, today)
	default:
		return h.reply(ctx, cmd, "Не понял. Можно записать трату, посмотреть, изменить или удалить. Например: «кофе 350».")
	}
}

func (h *Handler) add(ctx context.Context, cmd Command, intent domain.Intent, today time.Time) error {
	draft := domain.Draft{
		Amount:   intent.Amount,
		Category: intent.Category,
		SpentAt:  intent.SpentAt,
		Comment:  intent.Comment,
	}
	if draft.SpentAt.IsZero() {
		draft.SpentAt = today
	}
	if draft.Category == "" {
		draft.Category = domain.CategoryFromText(draft.Comment)
	}

	item, err := domain.New(cmd.UserID, draft, today)
	if err != nil {
		if errors.Is(err, domain.ErrFutureDate) {
			return h.reply(ctx, cmd, "Дата траты не может быть позже сегодняшнего дня.")
		}
		if errors.Is(err, domain.ErrUserRequired) {
			return err
		}
		return h.reply(ctx, cmd, "Не понял трату. Напиши сумму и что купил, например: «кофе 350».")
	}

	saved, err := h.store.Add(ctx, item)
	if err != nil {
		if sendErr := h.reply(ctx, cmd, "Не получилось записать трату. Попробуй ещё раз."); sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return fmt.Errorf("save expense: %w", err)
	}
	return h.reply(ctx, cmd, confirmation(saved))
}

func (h *Handler) reply(ctx context.Context, cmd Command, text string) error {
	if err := h.messenger.SendText(ctx, cmd.ChatID, text); err != nil {
		return fmt.Errorf("send expense reply: %w", err)
	}
	h.remember(cmd.UserID, cmd.Text, text)
	return nil
}

func (h *Handler) recent(userID int64) []domain.Turn {
	h.memory.mu.Lock()
	defer h.memory.mu.Unlock()
	items := h.memory.turns[userID]
	out := make([]domain.Turn, len(items))
	copy(out, items)
	return out
}

func (h *Handler) remember(userID int64, userText, reply string) {
	userText = strings.TrimSpace(userText)
	reply = strings.TrimSpace(reply)
	if userID <= 0 || userText == "" || reply == "" {
		return
	}
	h.memory.mu.Lock()
	defer h.memory.mu.Unlock()
	if h.memory.turns == nil {
		h.memory.turns = map[int64][]domain.Turn{}
	}
	items := append(h.memory.turns[userID],
		domain.Turn{Role: domain.RoleUser, Text: limitText(userText, 300)},
		domain.Turn{Role: domain.RoleAssistant, Text: limitText(reply, 300)},
	)
	if len(items) > 8 {
		items = items[len(items)-8:]
	}
	h.memory.turns[userID] = items
}

func (h *Handler) rememberShown(userID int64, ids []int64) {
	if userID <= 0 {
		return
	}
	h.memory.mu.Lock()
	defer h.memory.mu.Unlock()
	if h.memory.shown == nil {
		h.memory.shown = map[int64][]int64{}
	}
	h.memory.shown[userID] = append([]int64(nil), ids...)
}

func (h *Handler) shownIDs(userID int64) []int64 {
	h.memory.mu.Lock()
	defer h.memory.mu.Unlock()
	return append([]int64(nil), h.memory.shown[userID]...)
}

func limitText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func confirmation(item domain.Expense) string {
	when := item.SpentAt.Format("02.01.2006")
	amount := domain.FormatRubles(item.Amount)
	if item.Comment == "" {
		return fmt.Sprintf("Записал: %s, %s, %s.", amount, item.Category, when)
	}
	return fmt.Sprintf("Записал: %s — %s, %s, %s.", item.Comment, amount, item.Category, when)
}

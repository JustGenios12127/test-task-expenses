package expense

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"test-task-expenses/backend/internal/domain/chat"
	domain "test-task-expenses/backend/internal/domain/expense"
)

const (
	toolAdd    = "add_expense"
	toolList   = "list_expenses"
	toolUpdate = "update_expense"
	toolDelete = "delete_expense"

	defaultListLimit = 10
	maxListLimit     = 20
	dateLayout       = "2006-01-02"
)

var (
	errUnknownTool = errors.New("unknown tool")
	errBadArgs     = errors.New("arguments are not valid JSON")
	errBadRange    = errors.New("from must not be after to")
	errNoChanges   = errors.New("nothing to change")
	errStore       = errors.New("storage is unavailable, try later")
)

var tools = []chat.Tool{
	{
		Name:        toolAdd,
		Description: "Записать новую трату пользователя.",
		Parameters: object(map[string]any{
			"amount":   number("Сумма в рублях, например 350 или 1250.5."),
			"category": categoryField("Категория траты."),
			"spent_at": date("Дата траты. Если пользователь её не назвал, сегодняшняя дата."),
			"comment":  text("Что купили, коротко, словами пользователя, например «кофе» или «такси»."),
		}, "amount", "category"),
	},
	{
		Name:        toolList,
		Description: "Найти траты пользователя и посчитать их сумму. Возвращает total и count по всем подходящим тратам и до limit последних из них с id.",
		Parameters: object(map[string]any{
			"from":     date("Начало периода включительно."),
			"to":       date("Конец периода включительно."),
			"category": categoryField("Только эта категория."),
			"search":   text("Слово из названия траты, например «такси»."),
			"amount":   number("Только траты с этой суммой в рублях."),
			"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": maxListLimit, "description": "Сколько трат вернуть, по умолчанию 10."},
		}),
	},
	{
		Name:        toolUpdate,
		Description: "Изменить трату пользователя по id. Передавай только поля, которые нужно поменять.",
		Parameters: object(map[string]any{
			"id":            map[string]any{"type": "integer", "description": "id траты из результата list_expenses."},
			"amount":        number("Новая сумма в рублях."),
			"category":      categoryField("Новая категория."),
			"spent_at":      date("Новая дата траты."),
			"comment":       text("Новое название траты."),
			"clear_comment": map[string]any{"type": "boolean", "description": "true, если название нужно стереть."},
		}, "id"),
	},
	{
		Name:        toolDelete,
		Description: "Удалить трату пользователя по id.",
		Parameters: object(map[string]any{
			"id": map[string]any{"type": "integer", "description": "id траты из результата list_expenses."},
		}, "id"),
	},
}

type addArgs struct {
	Amount   json.Number `json:"amount"`
	Category string      `json:"category"`
	SpentAt  string      `json:"spent_at"`
	Comment  string      `json:"comment"`
}

type listArgs struct {
	From     string      `json:"from"`
	To       string      `json:"to"`
	Category string      `json:"category"`
	Search   string      `json:"search"`
	Amount   json.Number `json:"amount"`
	Limit    int         `json:"limit"`
}

type updateArgs struct {
	ID           int64       `json:"id"`
	Amount       json.Number `json:"amount"`
	Category     string      `json:"category"`
	SpentAt      string      `json:"spent_at"`
	Comment      string      `json:"comment"`
	ClearComment bool        `json:"clear_comment"`
}

type deleteArgs struct {
	ID int64 `json:"id"`
}

type expenseJSON struct {
	ID       int64  `json:"id"`
	SpentAt  string `json:"spent_at"`
	Amount   string `json:"amount"`
	Category string `json:"category"`
	Comment  string `json:"comment,omitempty"`
}

func (h *Handler) execute(ctx context.Context, userID int64, call chat.ToolCall, today time.Time) string {
	result, err := h.run(ctx, userID, call, today)
	if err != nil {
		if !isUserError(err) {
			slog.Error("expense tool", "tool", call.Name, "err", err)
			err = errStore
		}
		return encode(map[string]string{"error": err.Error()})
	}
	return encode(result)
}

func (h *Handler) run(ctx context.Context, userID int64, call chat.ToolCall, today time.Time) (any, error) {
	switch call.Name {
	case toolAdd:
		var args addArgs
		if err := decode(call.Arguments, &args); err != nil {
			return nil, err
		}
		return h.add(ctx, userID, args, today)
	case toolList:
		var args listArgs
		if err := decode(call.Arguments, &args); err != nil {
			return nil, err
		}
		return h.list(ctx, userID, args)
	case toolUpdate:
		var args updateArgs
		if err := decode(call.Arguments, &args); err != nil {
			return nil, err
		}
		return h.update(ctx, userID, args, today)
	case toolDelete:
		var args deleteArgs
		if err := decode(call.Arguments, &args); err != nil {
			return nil, err
		}
		deleted, err := h.store.Delete(ctx, userID, args.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": toJSON(deleted)}, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnknownTool, call.Name)
	}
}

func (h *Handler) add(ctx context.Context, userID int64, args addArgs, today time.Time) (any, error) {
	amount, err := domain.ParseAmount(args.Amount.String())
	if err != nil {
		return nil, err
	}
	category, err := domain.ParseCategory(args.Category)
	if err != nil {
		return nil, err
	}
	spent := today
	if args.SpentAt != "" {
		if spent, err = parseDate(args.SpentAt); err != nil {
			return nil, err
		}
	}
	item, err := domain.New(userID, domain.Draft{Amount: amount, Category: category, SpentAt: spent, Comment: args.Comment}, today)
	if err != nil {
		return nil, err
	}
	saved, err := h.store.Add(ctx, item)
	if err != nil {
		return nil, err
	}
	return map[string]any{"added": toJSON(saved)}, nil
}

func (h *Handler) list(ctx context.Context, userID int64, args listArgs) (any, error) {
	query := domain.Query{UserID: userID, Match: strings.TrimSpace(args.Search), Limit: defaultListLimit}
	if args.Limit > 0 {
		query.Limit = min(args.Limit, maxListLimit)
	}
	var err error
	if args.Category != "" {
		if query.Category, err = domain.ParseCategory(args.Category); err != nil {
			return nil, err
		}
	}
	if args.Amount != "" {
		if query.Amount, err = domain.ParseAmount(args.Amount.String()); err != nil {
			return nil, err
		}
	}
	if args.From != "" {
		if query.From, err = parseDate(args.From); err != nil {
			return nil, err
		}
	}
	if args.To != "" {
		if query.To, err = parseDate(args.To); err != nil {
			return nil, err
		}
	}
	if !query.From.IsZero() && !query.To.IsZero() && query.To.Before(query.From) {
		return nil, errBadRange
	}

	total, count, err := h.store.Total(ctx, query)
	if err != nil {
		return nil, err
	}
	items, err := h.store.List(ctx, query)
	if err != nil {
		return nil, err
	}
	expenses := make([]expenseJSON, 0, len(items))
	for _, item := range items {
		expenses = append(expenses, toJSON(item))
	}
	return map[string]any{"total": rubles(total), "count": count, "expenses": expenses}, nil
}

func (h *Handler) update(ctx context.Context, userID int64, args updateArgs, today time.Time) (any, error) {
	patch := domain.Patch{Comment: args.Comment, SetComment: args.ClearComment || strings.TrimSpace(args.Comment) != ""}
	var err error
	if args.Amount != "" {
		if patch.Amount, err = domain.ParseAmount(args.Amount.String()); err != nil {
			return nil, err
		}
	}
	if args.Category != "" {
		if patch.Category, err = domain.ParseCategory(args.Category); err != nil {
			return nil, err
		}
	}
	if args.SpentAt != "" {
		if patch.SpentAt, err = parseDate(args.SpentAt); err != nil {
			return nil, err
		}
		if patch.SpentAt.After(domain.DateUTC(today)) {
			return nil, domain.ErrFutureDate
		}
	}
	if args.ClearComment {
		patch.Comment = ""
	}
	if patch.Empty() {
		return nil, errNoChanges
	}

	before, err := h.store.Get(ctx, userID, args.ID)
	if err != nil {
		return nil, err
	}
	after, err := h.store.Update(ctx, userID, args.ID, patch)
	if err != nil {
		return nil, err
	}
	return map[string]any{"before": toJSON(before), "after": toJSON(after)}, nil
}

func isUserError(err error) bool {
	for _, target := range []error{
		domain.ErrInvalidAmount, domain.ErrInvalidCategory, domain.ErrInvalidDate, domain.ErrFutureDate,
		domain.ErrNotFound, errUnknownTool, errBadArgs, errBadRange, errNoChanges,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func decode(raw string, dest any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("%w: %v", errBadArgs, err)
	}
	return nil
}

func encode(value any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return `{"error":"internal error"}`
	}
	return strings.TrimSpace(buf.String())
}

func parseDate(raw string) (time.Time, error) {
	parsed, err := time.Parse(dateLayout, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, domain.ErrInvalidDate
	}
	return parsed, nil
}

func toJSON(item domain.Expense) expenseJSON {
	return expenseJSON{
		ID:       item.ID,
		SpentAt:  item.SpentAt.Format(dateLayout),
		Amount:   rubles(item.Amount),
		Category: string(item.Category),
		Comment:  item.Comment,
	}
}

func rubles(kopecks int64) string {
	return fmt.Sprintf("%d.%02d", kopecks/100, kopecks%100)
}

func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func number(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func date(description string) map[string]any {
	return map[string]any{"type": "string", "description": description + " Формат YYYY-MM-DD."}
}

func categoryField(description string) map[string]any {
	names := make([]string, 0, len(domain.Categories()))
	for _, category := range domain.Categories() {
		names = append(names, string(category))
	}
	return map[string]any{"type": "string", "enum": names, "description": description}
}

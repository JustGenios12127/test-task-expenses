package expense

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func (h *Handler) list(ctx context.Context, cmd Command, intent domain.Intent, today time.Time) error {
	query := scopedQuery(cmd.UserID, intent, today)
	query.Limit = 10
	total, count, err := h.store.Total(ctx, query)
	if err != nil {
		return h.storeError(ctx, cmd, err)
	}
	if count == 0 {
		return h.reply(ctx, cmd, "За этот период трат нет.")
	}
	items, err := h.store.List(ctx, query)
	if err != nil {
		return h.storeError(ctx, cmd, err)
	}
	ids := make([]int64, 0, len(items))
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		ids = append(ids, items[i].ID)
	}
	h.rememberShown(cmd.UserID, ids)
	return h.reply(ctx, cmd, listReply(intent, total, count, items))
}

func (h *Handler) update(ctx context.Context, cmd Command, intent domain.Intent, today time.Time) error {
	patch := domain.Patch{
		Amount:     intent.NewAmount,
		Category:   intent.NewCategory,
		SpentAt:    intent.NewSpentAt,
		Comment:    intent.NewComment,
		SetComment: intent.SetComment,
	}.Normalized()
	if patch.Empty() {
		return h.reply(ctx, cmd, "Что изменить? Можно сумму, категорию, дату или название. Например: «кофе было не 350, а 450».")
	}
	if patch.Amount > 0 {
		if err := domain.CheckAmount(patch.Amount); err != nil {
			return h.reply(ctx, cmd, "Не вижу новую сумму. Например: «последнее было не 350, а 450».")
		}
	}
	if !patch.SpentAt.IsZero() && patch.SpentAt.After(domain.DateUTC(today)) {
		return h.reply(ctx, cmd, "Дата траты не может быть позже сегодняшнего дня.")
	}
	target := clarifyTarget(cmd.Text, intent)
	found, err := h.locate(ctx, cmd, target, today, target.Amount > 0)
	if err != nil {
		if reply, ok := h.lookupReply(err); ok {
			return h.reply(ctx, cmd, reply)
		}
		return h.storeError(ctx, cmd, err)
	}
	saved, err := h.store.Update(ctx, cmd.UserID, found.ID, patch)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return h.reply(ctx, cmd, "Не нашёл такую трату среди своих записей.")
		}
		return h.storeError(ctx, cmd, err)
	}
	return h.reply(ctx, cmd, updateReply(found, saved))
}

func (h *Handler) remove(ctx context.Context, cmd Command, intent domain.Intent, today time.Time) error {
	intent = clarifyTarget(cmd.Text, intent)
	if !hasTarget(intent) {
		return h.reply(ctx, cmd, "Уточни, какую трату удалить. Например: «удали такси».")
	}
	found, err := h.locate(ctx, cmd, intent, today, false)
	if err != nil {
		if reply, ok := h.lookupReply(err); ok {
			return h.reply(ctx, cmd, reply)
		}
		return h.storeError(ctx, cmd, err)
	}
	deleted, err := h.store.Delete(ctx, cmd.UserID, found.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return h.reply(ctx, cmd, "Не нашёл такую трату среди своих записей.")
		}
		return h.storeError(ctx, cmd, err)
	}
	return h.reply(ctx, cmd, "Удалил: "+formatExpense(deleted)+".")
}

func (h *Handler) lookupReply(err error) (string, bool) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "Не нашёл такую трату среди своих записей.", true
	case errors.Is(err, errNoShown):
		return "Покажи список и потом скажи номер строки, например «удали вторую».", true
	case errors.Is(err, errOutsideList):
		return "В последнем списке нет такой строки.", true
	default:
		return "", false
	}
}

func (h *Handler) locate(ctx context.Context, cmd Command, intent domain.Intent, today time.Time, loose bool) (domain.Expense, error) {
	if intent.Index > 0 {
		item, err := h.shownExpense(ctx, cmd.UserID, intent.Index)
		if err == nil || !errors.Is(err, errNoShown) {
			return item, err
		}
		if !hasTextTarget(intent) {
			return domain.Expense{}, err
		}
	}
	found, err := h.store.FindLatest(ctx, matchQuery(cmd.UserID, intent, today))
	if err == nil || !errors.Is(err, domain.ErrNotFound) || intent.Match == "" || !loose {
		return found, err
	}
	unnamed := intent
	unnamed.Match = ""
	return h.store.FindLatest(ctx, matchQuery(cmd.UserID, unnamed, today))
}

func (h *Handler) shownExpense(ctx context.Context, userID int64, index int) (domain.Expense, error) {
	ids := h.shownIDs(userID)
	if len(ids) == 0 {
		return domain.Expense{}, errNoShown
	}
	if index > len(ids) {
		return domain.Expense{}, errOutsideList
	}
	return h.store.Get(ctx, userID, ids[index-1])
}

func hasTextTarget(intent domain.Intent) bool {
	return strings.TrimSpace(intent.Match) != "" || intent.Category != "" || intent.Amount > 0 || !intent.SpentAt.IsZero() || !intent.From.IsZero() || !intent.To.IsZero() || intent.Period != ""
}

func (h *Handler) storeError(ctx context.Context, cmd Command, err error) error {
	if sendErr := h.reply(ctx, cmd, "Не получилось обработать сообщение. Попробуй ещё раз."); sendErr != nil {
		return errors.Join(err, sendErr)
	}
	return err
}

func scopedQuery(userID int64, intent domain.Intent, today time.Time) domain.Query {
	from, to := intentRange(intent, today)
	return domain.Query{
		UserID:   userID,
		Category: intent.Category,
		From:     from,
		To:       to,
		Match:    intent.Match,
	}
}

func matchQuery(userID int64, intent domain.Intent, today time.Time) domain.Query {
	query := domain.Query{
		UserID:   userID,
		Category: intent.Category,
		Match:    intent.Match,
		Amount:   intent.Amount,
	}
	if !intent.From.IsZero() || !intent.To.IsZero() || intent.Period != "" {
		query.From, query.To = intentRange(intent, today)
	} else if !intent.SpentAt.IsZero() {
		query.From, query.To = intent.SpentAt, intent.SpentAt
	}
	return query
}

func intentRange(intent domain.Intent, today time.Time) (time.Time, time.Time) {
	if intent.Period != "" {
		return dateBounds(intent.Period, today)
	}
	if !intent.From.IsZero() || !intent.To.IsZero() {
		from, to := intent.From, intent.To
		if from.IsZero() {
			from = to
		}
		if to.IsZero() {
			to = from
		}
		if to.Before(from) {
			from, to = to, from
		}
		return from, to
	}
	return dateBounds(intent.Period, today)
}

func hasTarget(intent domain.Intent) bool {
	return strings.TrimSpace(intent.Match) != "" || intent.Category != "" || intent.Amount > 0 || intent.Latest || intent.Index > 0 || !intent.SpentAt.IsZero() || !intent.From.IsZero() || !intent.To.IsZero() || intent.Period != ""
}

func dateBounds(period domain.Period, today time.Time) (time.Time, time.Time) {
	day := domain.DateUTC(today)
	switch period {
	case domain.PeriodDay:
		return day, day
	case domain.PeriodWeek:
		return day.AddDate(0, 0, -6), day
	case domain.PeriodMonth:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC), day
	default:
		return time.Time{}, time.Time{}
	}
}

func listReply(intent domain.Intent, total int64, count int, items []domain.Expense) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s: %s", listTitle(intent), domain.FormatRubles(total))
	shown := len(items)
	if shown > 10 {
		shown = 10
	}
	for i := 0; i < shown; i++ {
		fmt.Fprintf(&builder, "\n• %s", formatExpense(items[i]))
	}
	if count > shown {
		fmt.Fprintf(&builder, "\nИ ещё %d.", count-shown)
	}
	return builder.String()
}

func listTitle(intent domain.Intent) string {
	switch intent.Period {
	case domain.PeriodDay:
		if intent.Category != "" {
			return "Сегодня, " + string(intent.Category)
		}
		return "Сегодня"
	case domain.PeriodWeek:
		if intent.Category != "" {
			return "За неделю, " + string(intent.Category)
		}
		return "За неделю"
	case domain.PeriodMonth:
		if intent.Category != "" {
			return "В этом месяце, " + string(intent.Category)
		}
		return "В этом месяце"
	}
	if !intent.From.IsZero() || !intent.To.IsZero() {
		from, to := intent.From, intent.To
		if from.IsZero() {
			from = to
		}
		if to.IsZero() {
			to = from
		}
		title := "За " + from.Format("02.01.2006")
		if !from.Equal(to) {
			title = "С " + from.Format("02.01.2006") + " по " + to.Format("02.01.2006")
		}
		if intent.Category != "" {
			return title + ", " + string(intent.Category)
		}
		return title
	}
	if intent.Category != "" {
		return "Траты, " + string(intent.Category)
	}
	return "Траты"
}

func updateReply(before, after domain.Expense) string {
	if before.Amount != after.Amount && before.Category == after.Category && before.SpentAt.Equal(after.SpentAt) && before.Comment == after.Comment {
		return fmt.Sprintf("Изменил: %s — было %s, стало %s.", expenseLabel(after), domain.FormatRubles(before.Amount), domain.FormatRubles(after.Amount))
	}
	return "Изменил: " + formatExpense(after) + "."
}

func formatExpense(item domain.Expense) string {
	when := item.SpentAt.Format("02.01.2006")
	amount := domain.FormatRubles(item.Amount)
	if item.Comment == "" {
		return fmt.Sprintf("%s — %s, %s", when, amount, item.Category)
	}
	return fmt.Sprintf("%s — %s, %s, %s", when, item.Comment, amount, item.Category)
}

func expenseLabel(item domain.Expense) string {
	if item.Comment == "" {
		return string(item.Category)
	}
	return item.Comment
}

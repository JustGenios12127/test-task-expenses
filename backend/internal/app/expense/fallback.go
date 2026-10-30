package expense

import (
	"context"
	"errors"
	"log/slog"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

type fallbackModel struct {
	model Model
}

func WithFallback(model Model) Model {
	return fallbackModel{model: model}
}

func (f fallbackModel) Recognize(ctx context.Context, text string, today time.Time, history []domain.Turn) (domain.Intent, error) {
	intent, err := f.model.Recognize(ctx, text, today, history)
	if err == nil || errors.Is(err, domain.ErrNotExpense) || ctx.Err() != nil {
		return intent, err
	}

	simple, simpleErr := domain.ParseSimple(text, today)
	if simpleErr != nil {
		return domain.Intent{}, err
	}
	slog.Warn("expense recognition fallback", "err", err)
	return domain.Intent{
		Action:   domain.ActionAdd,
		Amount:   simple.Amount,
		Category: simple.Category,
		SpentAt:  simple.SpentAt,
		Comment:  simple.Comment,
	}, nil
}

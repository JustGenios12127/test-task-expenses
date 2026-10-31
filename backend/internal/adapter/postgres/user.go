package postgres

import (
	"context"
	"fmt"
	"strings"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func (r *ExpenseRepository) SaveUser(ctx context.Context, id int64, username, firstName string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres pool is not configured")
	}
	if id <= 0 {
		return domain.ErrUserRequired
	}

	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	firstName = strings.TrimSpace(firstName)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, username, first_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET username = EXCLUDED.username,
		    first_name = EXCLUDED.first_name,
		    updated_at = now()
	`, id, username, firstName)
	if err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

const familyLimit = 1000

func (r *ExpenseRepository) ListFamily(ctx context.Context, filter domain.FamilyFilter) ([]domain.FamilyExpense, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("postgres pool is not configured")
	}

	where, args := familyFilter(filter)
	args = append(args, familyLimit)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT e.id,
		       e.user_id,
		       COALESCE(u.username, ''),
		       COALESCE(u.first_name, ''),
		       u.id IS NOT NULL,
		       e.amount::text,
		       e.category,
		       e.spent_at,
		       COALESCE(e.comment, ''),
		       e.created_at,
		       e.updated_at
		FROM expenses e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE %s
		ORDER BY e.spent_at DESC, e.id DESC
		LIMIT $%d
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list family expenses: %w", err)
	}
	defer rows.Close()

	items := make([]domain.FamilyExpense, 0)
	for rows.Next() {
		item, err := scanFamilyExpense(rows)
		if err != nil {
			return nil, fmt.Errorf("scan family expense: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list family expenses: %w", err)
	}
	return items, nil
}

func (r *ExpenseRepository) FamilyTotal(ctx context.Context, filter domain.FamilyFilter) (int64, int, error) {
	if r == nil || r.pool == nil {
		return 0, 0, fmt.Errorf("postgres pool is not configured")
	}

	where, args := familyFilter(filter)
	var total int64
	var count int
	err := r.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(SUM(e.amount * 100), 0)::bigint, COUNT(*)
		FROM expenses e
		WHERE %s
	`, where), args...).Scan(&total, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("total family expenses: %w", err)
	}
	return total, count, nil
}

func familyFilter(filter domain.FamilyFilter) (string, []any) {
	parts := []string{"TRUE"}
	args := make([]any, 0, 4)
	if filter.UserID > 0 {
		args = append(args, filter.UserID)
		parts = append(parts, fmt.Sprintf("e.user_id = $%d", len(args)))
	}
	if filter.Category != "" {
		args = append(args, string(filter.Category))
		parts = append(parts, fmt.Sprintf("e.category = $%d", len(args)))
	}
	if !filter.From.IsZero() {
		args = append(args, filter.From)
		parts = append(parts, fmt.Sprintf("e.spent_at >= $%d::date", len(args)))
	}
	if !filter.To.IsZero() {
		args = append(args, filter.To)
		parts = append(parts, fmt.Sprintf("e.spent_at <= $%d::date", len(args)))
	}
	return strings.Join(parts, " AND "), args
}

func scanFamilyExpense(row scanner) (domain.FamilyExpense, error) {
	var (
		item       domain.FamilyExpense
		amountText string
		category   string
	)
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.Username,
		&item.FirstName,
		&item.HasProfile,
		&amountText,
		&category,
		&item.SpentAt,
		&item.Comment,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return domain.FamilyExpense{}, err
	}
	amount, err := domain.ParseAmount(amountText)
	if err != nil {
		return domain.FamilyExpense{}, err
	}
	item.Amount = amount
	item.Category = domain.Category(category)
	item.SpentAt = domain.DateUTC(item.SpentAt)
	return item, nil
}

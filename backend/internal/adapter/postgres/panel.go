package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func (r *ExpenseRepository) ListMembers(ctx context.Context) ([]domain.FamilyMember, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("postgres pool is not configured")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, username, first_name
		FROM users
		ORDER BY username, first_name, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	members := make([]domain.FamilyMember, 0)
	for rows.Next() {
		var member domain.FamilyMember
		if err := rows.Scan(&member.ID, &member.Username, &member.FirstName); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

func (r *ExpenseRepository) FindMember(ctx context.Context, id int64) (domain.FamilyMember, error) {
	if r == nil || r.pool == nil {
		return domain.FamilyMember{}, fmt.Errorf("postgres pool is not configured")
	}
	if id <= 0 {
		return domain.FamilyMember{}, domain.ErrNotFound
	}
	var member domain.FamilyMember
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, first_name
		FROM users
		WHERE id = $1
	`, id).Scan(&member.ID, &member.Username, &member.FirstName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FamilyMember{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FamilyMember{}, fmt.Errorf("find member: %w", err)
	}
	return member, nil
}

func (r *ExpenseRepository) Replace(ctx context.Context, id int64, item domain.Expense, version time.Time) (domain.Expense, error) {
	if r == nil || r.pool == nil {
		return domain.Expense{}, fmt.Errorf("postgres pool is not configured")
	}
	if id <= 0 {
		return domain.Expense{}, domain.ErrNotFound
	}
	if item.UserID <= 0 {
		return domain.Expense{}, domain.ErrUserRequired
	}

	var comment any
	if strings.TrimSpace(item.Comment) == "" {
		comment = nil
	} else {
		comment = item.Comment
	}
	var expected any
	if !version.IsZero() {
		expected = version
	}

	row := r.pool.QueryRow(ctx, `
		UPDATE expenses
		SET user_id = $2,
		    amount = $3::numeric,
		    category = $4,
		    spent_at = $5::date,
		    comment = $6,
		    updated_at = now()
		WHERE id = $1 AND ($7::timestamptz IS NULL OR updated_at = $7::timestamptz)
		RETURNING id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
	`, id, item.UserID, item.NumericAmount(), string(item.Category), item.SpentAt, comment, expected)
	saved, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Expense{}, r.missingOrChanged(ctx, id)
	}
	if err != nil {
		return domain.Expense{}, fmt.Errorf("replace expense: %w", err)
	}
	return saved, nil
}

func (r *ExpenseRepository) missingOrChanged(ctx context.Context, id int64) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM expenses WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("check expense: %w", err)
	}
	if exists {
		return domain.ErrConflict
	}
	return domain.ErrNotFound
}

func (r *ExpenseRepository) DeleteByID(ctx context.Context, id int64) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres pool is not configured")
	}
	if id <= 0 {
		return domain.ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM expenses WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete expense: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

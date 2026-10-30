package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "test-task-expenses/backend/internal/domain/expense"
)

type ExpenseRepository struct {
	pool *pgxpool.Pool
}

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.MaxConns = 4

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func NewExpenseRepository(pool *pgxpool.Pool) *ExpenseRepository {
	return &ExpenseRepository{pool: pool}
}

func (r *ExpenseRepository) Add(ctx context.Context, item domain.Expense) (domain.Expense, error) {
	if item.UserID <= 0 {
		return domain.Expense{}, domain.ErrUserRequired
	}

	var comment any
	if strings.TrimSpace(item.Comment) == "" {
		comment = nil
	} else {
		comment = item.Comment
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO expenses (user_id, amount, category, spent_at, comment)
		VALUES ($1, $2::numeric, $3, $4::date, $5)
		RETURNING id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
	`, item.UserID, item.NumericAmount(), string(item.Category), item.SpentAt, comment)

	saved, err := scanExpense(row)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("insert expense: %w", err)
	}
	return saved, nil
}

func (r *ExpenseRepository) List(ctx context.Context, query domain.Query) ([]domain.Expense, error) {
	where, args, err := expenseFilter(query)
	if err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
		FROM expenses
		WHERE %s
		ORDER BY spent_at DESC, id DESC
		LIMIT $%d
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Expense, 0)
	for rows.Next() {
		item, err := scanExpense(rows)
		if err != nil {
			return nil, fmt.Errorf("scan expense: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	return items, nil
}

func (r *ExpenseRepository) Total(ctx context.Context, query domain.Query) (int64, int, error) {
	where, args, err := expenseFilter(query)
	if err != nil {
		return 0, 0, err
	}
	var amountText string
	var count int
	err = r.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(ROUND(SUM(amount), 2), 0)::text, COUNT(*)
		FROM expenses
		WHERE %s
	`, where), args...).Scan(&amountText, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("total expenses: %w", err)
	}
	if count == 0 {
		return 0, 0, nil
	}
	total, err := domain.ParseAmount(amountText)
	if err != nil {
		return 0, 0, fmt.Errorf("parse total %q: %w", amountText, err)
	}
	return total, count, nil
}

func (r *ExpenseRepository) FindLatest(ctx context.Context, query domain.Query) (domain.Expense, error) {
	where, args, err := expenseFilter(query)
	if err != nil {
		return domain.Expense{}, err
	}
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
		FROM expenses
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, where), args...)
	item, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Expense{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Expense{}, fmt.Errorf("find expense: %w", err)
	}
	return item, nil
}

func (r *ExpenseRepository) Get(ctx context.Context, userID, id int64) (domain.Expense, error) {
	if userID <= 0 {
		return domain.Expense{}, domain.ErrUserRequired
	}
	if id <= 0 {
		return domain.Expense{}, domain.ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `
		SELECT id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
		FROM expenses
		WHERE id = $2 AND user_id = $1
	`, userID, id)
	item, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Expense{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Expense{}, fmt.Errorf("get expense: %w", err)
	}
	return item, nil
}

func (r *ExpenseRepository) Update(ctx context.Context, userID, id int64, patch domain.Patch) (domain.Expense, error) {
	if userID <= 0 {
		return domain.Expense{}, domain.ErrUserRequired
	}
	if id <= 0 {
		return domain.Expense{}, domain.ErrNotFound
	}
	patch = patch.Normalized()
	if patch.Amount > 0 {
		if err := domain.CheckAmount(patch.Amount); err != nil {
			return domain.Expense{}, err
		}
	}
	if patch.Category != "" && !patch.Category.Valid() {
		return domain.Expense{}, domain.ErrInvalidCategory
	}
	row := r.pool.QueryRow(ctx, `
		UPDATE expenses
		SET amount = CASE WHEN $3::text IS NULL THEN amount ELSE $3::numeric END,
			category = COALESCE($4::text, category),
			spent_at = COALESCE($5::date, spent_at),
			comment = CASE WHEN $6::bool THEN $7::text ELSE comment END,
			updated_at = now()
		WHERE id = $2 AND user_id = $1
		RETURNING id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
	`, userID, id, amountValue(patch), categoryValue(patch), spentValue(patch), patch.SetComment, commentValue(patch))
	item, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Expense{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Expense{}, fmt.Errorf("update expense: %w", err)
	}
	return item, nil
}

func (r *ExpenseRepository) Delete(ctx context.Context, userID, id int64) (domain.Expense, error) {
	if userID <= 0 {
		return domain.Expense{}, domain.ErrUserRequired
	}
	if id <= 0 {
		return domain.Expense{}, domain.ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `
		DELETE FROM expenses
		WHERE id = $2 AND user_id = $1
		RETURNING id, user_id, amount::text, category, spent_at, COALESCE(comment, ''), created_at, updated_at
	`, userID, id)
	item, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Expense{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Expense{}, fmt.Errorf("delete expense: %w", err)
	}
	return item, nil
}

func expenseFilter(query domain.Query) (string, []any, error) {
	if query.UserID <= 0 {
		return "", nil, domain.ErrUserRequired
	}
	parts := []string{"user_id = $1"}
	args := []any{query.UserID}
	if query.Category != "" {
		args = append(args, string(query.Category))
		parts = append(parts, fmt.Sprintf("category = $%d", len(args)))
	}
	if !query.From.IsZero() {
		args = append(args, query.From)
		parts = append(parts, fmt.Sprintf("spent_at >= $%d::date", len(args)))
	}
	if !query.To.IsZero() {
		args = append(args, query.To)
		parts = append(parts, fmt.Sprintf("spent_at <= $%d::date", len(args)))
	}
	if query.Amount > 0 {
		args = append(args, kopecksNumeric(query.Amount))
		parts = append(parts, fmt.Sprintf("amount = $%d::numeric", len(args)))
	}
	if match := strings.TrimSpace(query.Match); match != "" {
		args = append(args, "%"+escapeLike(match)+"%", match)
		pattern := len(args) - 1
		plain := len(args)
		parts = append(parts, fmt.Sprintf(`(COALESCE(comment, '') ILIKE $%d ESCAPE '\' OR (comment <> '' AND $%d ILIKE '%%' || comment || '%%'))`, pattern, plain))
	}
	return strings.Join(parts, " AND "), args, nil
}

func kopecksNumeric(kopecks int64) string {
	return fmt.Sprintf("%d.%02d", kopecks/100, kopecks%100)
}

func amountValue(patch domain.Patch) any {
	if patch.Amount <= 0 {
		return nil
	}
	return kopecksNumeric(patch.Amount)
}

func categoryValue(patch domain.Patch) any {
	if patch.Category == "" {
		return nil
	}
	return string(patch.Category)
}

func spentValue(patch domain.Patch) any {
	if patch.SpentAt.IsZero() {
		return nil
	}
	return patch.SpentAt
}

func commentValue(patch domain.Patch) any {
	if !patch.SetComment || strings.TrimSpace(patch.Comment) == "" {
		return nil
	}
	return patch.Comment
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanExpense(row scanner) (domain.Expense, error) {
	var (
		item       domain.Expense
		amountText string
		category   string
		spentAt    time.Time
	)
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&amountText,
		&category,
		&spentAt,
		&item.Comment,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return domain.Expense{}, err
	}
	amount, err := domain.ParseAmount(amountText)
	if err != nil {
		return domain.Expense{}, err
	}
	item.Amount = amount
	item.Category = domain.Category(category)
	item.SpentAt = domain.DateUTC(spentAt)
	return item, nil
}

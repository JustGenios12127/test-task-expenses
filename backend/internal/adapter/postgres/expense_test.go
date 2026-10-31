package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
	"test-task-expenses/backend/internal/migrate"
)

func TestAddRequiresUserID(t *testing.T) {
	repo := NewExpenseRepository(nil)
	ctx := context.Background()
	if _, err := repo.Add(ctx, domain.Expense{}); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("add error = %v", err)
	}
	if _, err := repo.List(ctx, domain.Query{}); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("list error = %v", err)
	}
	if _, _, err := repo.Total(ctx, domain.Query{}); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("total error = %v", err)
	}
	if _, err := repo.FindLatest(ctx, domain.Query{}); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("find error = %v", err)
	}
	if _, err := repo.Update(ctx, 0, 1, domain.Patch{Amount: 100}); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("update error = %v", err)
	}
	if _, err := repo.Get(ctx, 0, 1); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("get error = %v", err)
	}
	if _, err := repo.Delete(ctx, 0, 1); !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("delete error = %v", err)
	}
}

func TestExpenseFilterEscapesMatch(t *testing.T) {
	where, args, err := expenseFilter(domain.Query{UserID: 8, Match: `100%_a\b`})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if !strings.Contains(where, `ILIKE`) || !strings.Contains(where, `ESCAPE`) {
		t.Fatalf("where = %s", where)
	}
	if len(args) != 3 || args[0] != int64(8) || args[1] != `%100\%\_a\\b%` || args[2] != `100%_a\b` {
		t.Fatalf("args = %#v", args)
	}
}

func TestAddKeepsUsersApart(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	t.Setenv("MIGRATIONS_DIR", filepath.Join("..", "..", "..", "migration"))
	ctx := context.Background()
	if err := migrate.Up(ctx, dsn); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM expenses WHERE user_id IN (10, 20)`)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM expenses WHERE user_id IN (10, 20)`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	repo := NewExpenseRepository(pool)
	today := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	first, err := repo.Add(ctx, testExpense(t, 10, "кофе", today))
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	if _, err := repo.Add(ctx, testExpense(t, 20, "такси", today)); err != nil {
		t.Fatalf("add second: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM expenses WHERE id = $1 AND user_id = $2`, first.ID, int64(20)).Scan(&count); err != nil {
		t.Fatalf("count foreign: %v", err)
	}
	if count != 0 {
		t.Fatalf("user 20 sees expense %d", first.ID)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM expenses WHERE user_id = $1`, int64(10)).Scan(&count); err != nil {
		t.Fatalf("count own: %v", err)
	}
	if count != 1 || first.UserID != 10 {
		t.Fatalf("owner rows = %d, saved user = %d", count, first.UserID)
	}

	if err := migrate.Down(ctx, dsn); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if err := migrate.Up(ctx, dsn); err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
}

func TestChangesStayWithOwner(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	t.Setenv("MIGRATIONS_DIR", filepath.Join("..", "..", "..", "migration"))
	ctx := context.Background()
	if err := migrate.Up(ctx, dsn); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM expenses WHERE user_id IN (10, 20)`)
	})
	if _, err := pool.Exec(ctx, `DELETE FROM expenses WHERE user_id IN (10, 20)`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	repo := NewExpenseRepository(pool)
	today := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	coffee, err := repo.Add(ctx, savedExpense(t, 10, 35000, domain.CategoryFood, "кофе", today))
	if err != nil {
		t.Fatalf("add coffee: %v", err)
	}
	taxi, err := repo.Add(ctx, savedExpense(t, 10, 120000, domain.CategoryTransport, "такси", today.AddDate(0, 0, -1)))
	if err != nil {
		t.Fatalf("add taxi: %v", err)
	}
	other, err := repo.Add(ctx, savedExpense(t, 20, 35000, domain.CategoryFood, "кофе", today))
	if err != nil {
		t.Fatalf("add other: %v", err)
	}

	month := domain.Query{
		UserID:   10,
		Category: domain.CategoryFood,
		From:     time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
	}
	total, count, err := repo.Total(ctx, month)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 35000 || count != 1 {
		t.Fatalf("own food total = %d count = %d", total, count)
	}

	if _, err := repo.Update(ctx, 20, coffee.ID, domain.Patch{Amount: 45000}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign update = %v", err)
	}
	changed, err := repo.Update(ctx, 10, coffee.ID, domain.Patch{Amount: 45000})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if changed.Amount != 45000 || changed.UserID != 10 {
		t.Fatalf("changed = %+v", changed)
	}
	still, err := repo.FindLatest(ctx, domain.Query{UserID: 20, Amount: 35000})
	if err != nil {
		t.Fatalf("other coffee: %v", err)
	}
	if still.ID != other.ID || still.Amount != 35000 {
		t.Fatalf("other changed = %+v", still)
	}

	if _, err := repo.Delete(ctx, 20, taxi.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign delete = %v", err)
	}
	deleted, err := repo.Delete(ctx, 10, taxi.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted.Comment != "такси" {
		t.Fatalf("deleted = %+v", deleted)
	}
	if _, err := repo.FindLatest(ctx, domain.Query{UserID: 10, Match: "такси"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("taxi still visible: %v", err)
	}
	left, err := repo.List(ctx, domain.Query{UserID: 10, Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(left) != 1 || left[0].ID != coffee.ID || left[0].Amount != 45000 {
		t.Fatalf("left = %+v", left)
	}
}

func testExpense(t *testing.T, userID int64, comment string, today time.Time) domain.Expense {
	t.Helper()
	item, err := domain.New(userID, domain.Draft{
		Amount:   35000,
		Category: domain.CategoryFood,
		SpentAt:  today,
		Comment:  comment,
	}, today)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return item
}

func savedExpense(t *testing.T, userID, amount int64, category domain.Category, comment string, spent time.Time) domain.Expense {
	t.Helper()
	item, err := domain.New(userID, domain.Draft{
		Amount:   amount,
		Category: category,
		SpentAt:  spent,
		Comment:  comment,
	}, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return item
}

func TestPanelFiltersAndVersion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	t.Setenv("MIGRATIONS_DIR", filepath.Join("..", "..", "..", "migration"))
	ctx := context.Background()
	if err := migrate.Up(ctx, dsn); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	clean := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM expenses WHERE user_id IN (30, 40)`)
	}
	clean()
	t.Cleanup(clean)

	repo := NewExpenseRepository(pool)
	today := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	fuel, err := repo.Add(ctx, savedExpense(t, 30, 300000, domain.CategoryTransport, "заправка", today))
	if err != nil {
		t.Fatalf("add fuel: %v", err)
	}
	if _, err := repo.Add(ctx, savedExpense(t, 30, 99999999999, domain.CategoryOther, "машина", today.AddDate(0, -1, 0))); err != nil {
		t.Fatalf("add car: %v", err)
	}
	if _, err := repo.Add(ctx, savedExpense(t, 30, 99999999999, domain.CategoryOther, "машина", today.AddDate(0, -1, 0))); err != nil {
		t.Fatalf("add second car: %v", err)
	}
	if _, err := repo.Add(ctx, savedExpense(t, 40, 50000, domain.CategoryTransport, "такси", today)); err != nil {
		t.Fatalf("add taxi: %v", err)
	}

	filter := domain.FamilyFilter{
		UserID:   30,
		Category: domain.CategoryTransport,
		From:     time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	}
	rows, err := repo.ListFamily(ctx, filter)
	if err != nil {
		t.Fatalf("list family: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != fuel.ID {
		t.Fatalf("rows = %+v", rows)
	}
	total, count, err := repo.FamilyTotal(ctx, domain.FamilyFilter{UserID: 30})
	if err != nil {
		t.Fatalf("family total: %v", err)
	}
	if total != 300000+2*99999999999 || count != 3 {
		t.Fatalf("total = %d count = %d", total, count)
	}
	if own, _, err := repo.Total(ctx, domain.Query{UserID: 30}); err != nil || own != total {
		t.Fatalf("bot total = %d err = %v", own, err)
	}

	found, err := repo.FindLatest(ctx, domain.Query{UserID: 30, Match: "заправку"})
	if err != nil || found.ID != fuel.ID {
		t.Fatalf("stem match = %+v err = %v", found, err)
	}

	edit := fuel
	edit.Amount = 310000
	saved, err := repo.Replace(ctx, fuel.ID, edit, fuel.UpdatedAt)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := repo.Replace(ctx, fuel.ID, edit, fuel.UpdatedAt); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale replace = %v", err)
	}
	if _, err := repo.Replace(ctx, fuel.ID, edit, saved.UpdatedAt); err != nil {
		t.Fatalf("fresh replace: %v", err)
	}
	if _, err := repo.Replace(ctx, 1<<40, edit, saved.UpdatedAt); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing replace = %v", err)
	}
}

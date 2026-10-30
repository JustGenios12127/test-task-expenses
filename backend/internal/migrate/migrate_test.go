package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDirUsesEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MIGRATIONS_DIR", dir)

	got, err := ResolveDir()
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if got != dir {
		t.Fatalf("dir = %s, want %s", got, dir)
	}
}

func TestResolveDirRejectsMissingEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := ResolveDir(); err == nil {
		t.Fatal("expected error")
	}
}

func TestExpenseMigrationHasGooseSections(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migration", "00001_create_expenses.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	text := string(raw)
	for _, part := range []string{"-- +goose Up", "-- +goose Down", "user_id", "spent_at", "CREATE TABLE expenses"} {
		if !strings.Contains(text, part) {
			t.Fatalf("migration missing %q", part)
		}
	}
}

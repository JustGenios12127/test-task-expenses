package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func Up(ctx context.Context, dsn string) error {
	if err := run(ctx, dsn, goose.UpContext); err != nil {
		return err
	}
	slog.Info("migrations ready")
	return nil
}

func Down(ctx context.Context, dsn string) error {
	if err := run(ctx, dsn, goose.DownContext); err != nil {
		return err
	}
	slog.Info("migration rolled back")
	return nil
}

func Status(ctx context.Context, dsn string) error {
	return run(ctx, dsn, goose.StatusContext)
}

func ResolveDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR")); dir != "" {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("migrations directory %s: %w", dir, err)
		}
		return dir, nil
	}

	candidates := []string{"migration", filepath.Join("backend", "migration")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "migration"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found")
}

func run(ctx context.Context, dsn string, command func(context.Context, *sql.DB, string, ...goose.OptionsFunc) error) error {
	dir, err := ResolveDir()
	if err != nil {
		return err
	}
	db, err := open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := command(ctx, db, dir); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func open(ctx context.Context, dsn string) (*sql.DB, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var last error
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		db, err := sql.Open("pgx", dsn)
		if err == nil {
			pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
			err = db.PingContext(pingCtx)
			pingCancel()
			if err == nil {
				db.SetMaxOpenConns(1)
				return db, nil
			}
			db.Close()
		}
		last = err
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return nil, fmt.Errorf("connect postgres: %w", last)
		case <-ticker.C:
		}
	}
}

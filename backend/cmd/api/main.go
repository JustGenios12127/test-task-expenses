package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"test-task-expenses/backend/internal/adapter/groq"
	"test-task-expenses/backend/internal/adapter/postgres"
	"test-task-expenses/backend/internal/adapter/telegram"
	botapp "test-task-expenses/backend/internal/app/bot"
	expenseapp "test-task-expenses/backend/internal/app/expense"
	"test-task-expenses/backend/internal/config"
	"test-task-expenses/backend/internal/migrate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	loc, err := cfg.Location()
	if err != nil {
		slog.Error("load timezone", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	migrateCtx, cancelMigrate := context.WithTimeout(ctx, 30*time.Second)
	err = migrate.Up(migrateCtx, cfg.PostgresDSN())
	cancelMigrate()
	if err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	pool, err := postgres.NewPool(ctx, cfg.PostgresDSN())
	if err != nil {
		slog.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	tgClient := telegram.NewClient(cfg.TelegramBotToken)
	expenses := expenseapp.NewHandler(
		expenseapp.WithFallback(groq.NewParser(cfg.GroqAPIKey, cfg.GroqModel)),
		postgres.NewExpenseRepository(pool),
		tgClient,
		loc,
	)
	tgBot := telegram.NewBot(tgClient, botapp.NewStartHandler(tgClient), expenses)
	go tgBot.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server started", "addr", cfg.Addr())
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		stop()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		slog.Info("server shutting down")
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "err", err)
			os.Exit(1)
		}
	}
}

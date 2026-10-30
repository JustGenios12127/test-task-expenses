package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"test-task-expenses/backend/internal/config"
	"test-task-expenses/backend/internal/migrate"
)

func main() {
	if len(os.Args) > 2 {
		usage()
	}
	command := "up"
	if len(os.Args) == 2 {
		command = os.Args[1]
	}
	switch command {
	case "up", "down", "status":
	default:
		usage()
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	switch command {
	case "up":
		err = migrate.Up(ctx, cfg.PostgresDSN())
	case "down":
		err = migrate.Down(ctx, cfg.PostgresDSN())
	case "status":
		err = migrate.Status(ctx, cfg.PostgresDSN())
	}
	if err != nil {
		slog.Error("migrate", "command", command, "err", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate [up|down|status]")
	os.Exit(2)
}

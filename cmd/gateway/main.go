package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/llm-router/gateway/internal/config"
	"github.com/llm-router/gateway/internal/handler"
	"github.com/llm-router/gateway/internal/server"
)

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.Log)

	srv := server.New(cfg.Server, logger)
	srv.Router().Get("/ping", handler.Ping())

	if err := srv.ListenAndServe(context.Background()); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/igor-kupczynski/alpha-monday/internal/api"
	"github.com/igor-kupczynski/alpha-monday/internal/config"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool init failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := db.NewStore(pool)
	handler := api.NewRouter(store, logger, cfg.CORSAllowOrigins)

	addr := fmt.Sprintf(":%d", cfg.Port)
	server := api.NewHTTPServer(addr, handler)

	logger.Info("api listening", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

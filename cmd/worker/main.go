package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	hatchetclient "github.com/hatchet-dev/hatchet/pkg/client"
	hatchet "github.com/hatchet-dev/hatchet/sdks/go"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/alphavantage"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/openai"
	appworker "github.com/igor-kupczynski/alpha-monday/internal/worker"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := appworker.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	clientOpts := []hatchetclient.ClientOpt{
		hatchetclient.WithToken(cfg.HatchetClientToken),
	}
	if cfg.HatchetClientHostPort != "" {
		host, portStr, err := net.SplitHostPort(cfg.HatchetClientHostPort)
		if err != nil {
			logger.Error("invalid HATCHET_CLIENT_HOST_PORT", "error", err)
			os.Exit(1)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			logger.Error("invalid HATCHET_CLIENT_HOST_PORT port", "error", err)
			os.Exit(1)
		}
		clientOpts = append(clientOpts, hatchetclient.WithHostPort(host, port))
	}

	client, err := hatchet.NewClient(clientOpts...)
	if err != nil {
		logger.Error("hatchet client init failed", "error", err)
		os.Exit(1)
	}
	if err := appworker.ConfigureRateLimits(client, logger); err != nil {
		logger.Error("hatchet rate limit configuration failed", "error", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool init failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := db.NewStore(pool)
	openAIClient := openai.NewClient(cfg.OpenAIAPIKey, openai.WithModel(cfg.OpenAIModel))
	alphaClient := alphavantage.NewClient(cfg.AlphaVantageAPIKey)
	steps := appworker.NewSteps(store, openAIClient, alphaClient, logger)

	workflows, err := appworker.BuildWorkflows(client, logger, steps)
	if err != nil {
		logger.Error("workflow build failed", "error", err)
		os.Exit(1)
	}

	w, err := client.NewWorker(cfg.WorkerName, hatchet.WithWorkflows(workflows...))
	if err != nil {
		logger.Error("hatchet worker init failed", "error", err)
		os.Exit(1)
	}

	cleanup, err := w.Start()
	if err != nil {
		logger.Error("worker start failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := cleanup(); err != nil {
			logger.Error("worker cleanup failed", "error", err)
		}
	}()

	logger.Info("worker started", "name", cfg.WorkerName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	logger.Info("worker shutdown requested")
}

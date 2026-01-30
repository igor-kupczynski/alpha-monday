package worker

import (
	"fmt"
	"os"
	"strings"

	"log/slog"
)

const defaultWorkerName = "alpha-monday-worker"

// Config holds worker configuration loaded from environment variables.
type Config struct {
	HatchetClientToken    string
	HatchetClientHostPort string
	WorkerName            string
	LogLevel              slog.Level
}

func LoadConfig() (Config, error) {
	token := strings.TrimSpace(os.Getenv("HATCHET_CLIENT_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("HATCHET_CLIENT_TOKEN is required")
	}

	workerName := strings.TrimSpace(os.Getenv("HATCHET_WORKER_NAME"))
	if workerName == "" {
		workerName = defaultWorkerName
	}

	cfg := Config{
		HatchetClientToken:    token,
		HatchetClientHostPort: strings.TrimSpace(os.Getenv("HATCHET_CLIENT_HOST_PORT")),
		WorkerName:            workerName,
		LogLevel:              parseLogLevel(getenvDefault("LOG_LEVEL", "info")),
	}

	return cfg, nil
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

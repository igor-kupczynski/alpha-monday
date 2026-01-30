package worker

import (
	"fmt"
	"os"
	"strings"

	"log/slog"
)

const defaultWorkerName = "alpha-monday-worker"
const defaultOpenAIModel = "gpt-4o-mini"

// Config holds worker configuration loaded from environment variables.
type Config struct {
	DatabaseURL           string
	OpenAIAPIKey          string
	OpenAIModel           string
	AlphaVantageAPIKey    string
	HatchetClientToken    string
	HatchetClientHostPort string
	WorkerName            string
	LogLevel              slog.Level
}

func LoadConfig() (Config, error) {
	databaseURL := getenvDefault("DATABASE_URL", "postgres://alpha:alpha@localhost:5432/alpha_monday?sslmode=disable")

	openAIKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if openAIKey == "" {
		return Config{}, fmt.Errorf("OPENAI_API_KEY is required")
	}

	openAIModel := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if openAIModel == "" {
		openAIModel = defaultOpenAIModel
	}

	alphaKey := strings.TrimSpace(os.Getenv("ALPHA_VANTAGE_API_KEY"))
	if alphaKey == "" {
		return Config{}, fmt.Errorf("ALPHA_VANTAGE_API_KEY is required")
	}

	token := strings.TrimSpace(os.Getenv("HATCHET_CLIENT_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("HATCHET_CLIENT_TOKEN is required")
	}

	workerName := strings.TrimSpace(os.Getenv("HATCHET_WORKER_NAME"))
	if workerName == "" {
		workerName = defaultWorkerName
	}

	cfg := Config{
		DatabaseURL:           databaseURL,
		OpenAIAPIKey:          openAIKey,
		OpenAIModel:           openAIModel,
		AlphaVantageAPIKey:    alphaKey,
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

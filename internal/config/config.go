package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"log/slog"
)

type Config struct {
	DatabaseURL      string
	Port             int
	LogLevel         slog.Level
	CORSAllowOrigins []string
}

func Load() (Config, error) {
	cfg := Config{}

	cfg.DatabaseURL = getenvDefault("DATABASE_URL", "postgres://alpha:alpha@localhost:5432/alpha_monday?sslmode=disable")

	portStr := getenvDefault("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid PORT: %w", err)
	}
	cfg.Port = port

	cfg.LogLevel = parseLogLevel(getenvDefault("LOG_LEVEL", "info"))
	cfg.CORSAllowOrigins = parseCSV(getenvDefault("CORS_ALLOW_ORIGINS", ""))

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

func parseCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

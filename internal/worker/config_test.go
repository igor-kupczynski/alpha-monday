package worker

import (
	"log/slog"
	"testing"
)

func TestLoadConfigRequiresHatchetToken(t *testing.T) {
	t.Setenv("HATCHET_CLIENT_TOKEN", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error when HATCHET_CLIENT_TOKEN missing")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("HATCHET_CLIENT_TOKEN", "token")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("HATCHET_WORKER_NAME", "")
	t.Setenv("HATCHET_CLIENT_HOST_PORT", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkerName != defaultWorkerName {
		t.Fatalf("expected default worker name %q, got %q", defaultWorkerName, cfg.WorkerName)
	}

	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("expected default log level info, got %v", cfg.LogLevel)
	}

	if cfg.HatchetClientHostPort != "" {
		t.Fatalf("expected empty hatchet host port, got %q", cfg.HatchetClientHostPort)
	}
}

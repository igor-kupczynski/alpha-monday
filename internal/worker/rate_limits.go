package worker

import (
	"fmt"
	"log/slog"

	"github.com/hatchet-dev/hatchet/pkg/client/types"
	hatchet "github.com/hatchet-dev/hatchet/sdks/go"
	"github.com/hatchet-dev/hatchet/sdks/go/features"
)

func ConfigureRateLimits(client *hatchet.Client, logger *slog.Logger) error {
	if client == nil {
		return fmt.Errorf("hatchet client is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := client.RateLimits().Upsert(features.CreateRatelimitOpts{
		Key:      alphaVantageRateLimitMinuteKey,
		Limit:    alphaVantageRateLimitMaxMinute,
		Duration: types.Minute,
	}); err != nil {
		return fmt.Errorf("configure minute rate limit: %w", err)
	}
	if err := client.RateLimits().Upsert(features.CreateRatelimitOpts{
		Key:      alphaVantageRateLimitDayKey,
		Limit:    alphaVantageRateLimitMaxDay,
		Duration: types.Day,
	}); err != nil {
		return fmt.Errorf("configure day rate limit: %w", err)
	}

	logger.Info("hatchet rate limits configured",
		"minute_key", alphaVantageRateLimitMinuteKey,
		"minute_max", alphaVantageRateLimitMaxMinute,
		"day_key", alphaVantageRateLimitDayKey,
		"day_max", alphaVantageRateLimitMaxDay,
	)
	return nil
}

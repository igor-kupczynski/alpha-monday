package worker

import (
	"fmt"
	"log/slog"

	hatchetclient "github.com/hatchet-dev/hatchet/pkg/client"
	"github.com/hatchet-dev/hatchet/pkg/client/types"
)

func ConfigureRateLimits(client hatchetclient.Client, logger *slog.Logger) error {
	if client == nil {
		return fmt.Errorf("hatchet client is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := client.Admin().PutRateLimit(alphaVantageRateLimitMinuteKey, &types.RateLimitOpts{
		Max:      alphaVantageRateLimitMaxMinute,
		Duration: types.Minute,
	}); err != nil {
		return fmt.Errorf("configure minute rate limit: %w", err)
	}
	if err := client.Admin().PutRateLimit(alphaVantageRateLimitDayKey, &types.RateLimitOpts{
		Max:      alphaVantageRateLimitMaxDay,
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

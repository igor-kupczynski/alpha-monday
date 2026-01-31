package retry

import (
	"context"
	"math/rand"
	"time"
)

// Config defines retry behavior for transient failures.
type Config struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      float64
}

// DefaultConfig returns the default retry policy (3 attempts, exponential backoff, jitter).
func DefaultConfig() Config {
	return Config{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      0.2,
	}
}

// Do executes fn with retries when shouldRetry returns true.
func Do(ctx context.Context, cfg Config, shouldRetry func(error) bool, fn func() error) error {
	if fn == nil {
		return nil
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if shouldRetry == nil {
		shouldRetry = func(error) bool { return false }
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			lastErr = err
			if attempt == maxAttempts || !shouldRetry(err) {
				return err
			}
			if delay := nextDelay(cfg, attempt); delay > 0 {
				if err := sleep(ctx, delay); err != nil {
					return err
				}
			}
			continue
		}
		return nil
	}

	return lastErr
}

func nextDelay(cfg Config, attempt int) time.Duration {
	if cfg.BaseDelay <= 0 {
		return 0
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := cfg.BaseDelay << (attempt - 1)
	if cfg.MaxDelay > 0 && delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	if cfg.Jitter > 0 && delay > 0 {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		jitter := time.Duration(rng.Float64() * cfg.Jitter * float64(delay))
		delay += jitter
	}
	if cfg.MaxDelay > 0 && delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	return delay
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

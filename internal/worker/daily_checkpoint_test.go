package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/alphavantage"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

type fakeSleeper struct {
	clock *fakeClock
	calls []time.Time
}

func (f *fakeSleeper) SleepUntil(ctx context.Context, target time.Time) error {
	f.calls = append(f.calls, target)
	if target.After(f.clock.now) {
		f.clock.now = target
	}
	return nil
}

type fakeStore struct {
	mu               sync.Mutex
	checkpoints      []db.CreateCheckpointInput
	statusUpdates    []string
	statusBatchIDs   []string
	createCheckpoint error
}

func (f *fakeStore) CreateBatchWithInitialCheckpoint(ctx context.Context, input db.CreateBatchInput) (db.CreateBatchResult, error) {
	return db.CreateBatchResult{}, fmt.Errorf("not implemented")
}

func (f *fakeStore) CreateCheckpointWithMetrics(ctx context.Context, input db.CreateCheckpointInput) (db.CreateCheckpointResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkpoints = append(f.checkpoints, input)
	if f.createCheckpoint != nil {
		return db.CreateCheckpointResult{}, f.createCheckpoint
	}
	return db.CreateCheckpointResult{CheckpointID: fmt.Sprintf("checkpoint-%d", len(f.checkpoints))}, nil
}

func (f *fakeStore) UpdateBatchStatus(ctx context.Context, batchID string, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusUpdates = append(f.statusUpdates, status)
	f.statusBatchIDs = append(f.statusBatchIDs, batchID)
	return nil
}

type sequenceAlpha struct {
	mu              sync.Mutex
	nextTradingDay  time.Time
	lastTradingDay  time.Time
	benchmarkPrice  string
	pickPrice       string
	benchmarkSymbol string
}

func (s *sequenceAlpha) FetchPreviousClose(ctx context.Context, symbol string) (alphavantage.Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if symbol == s.benchmarkSymbol {
		tradingDay := s.nextTradingDay
		s.lastTradingDay = tradingDay
		s.nextTradingDay = nextWeekday(tradingDay.AddDate(0, 0, 1))
		return alphavantage.Quote{
			Symbol:        symbol,
			PreviousClose: s.benchmarkPrice,
			TradingDay:    tradingDay.Format("2006-01-02"),
		}, nil
	}

	return alphavantage.Quote{
		Symbol:        symbol,
		PreviousClose: s.pickPrice,
		TradingDay:    s.lastTradingDay.Format("2006-01-02"),
	}, nil
}

func (s *sequenceAlpha) SnapshotPreviousCloses(ctx context.Context, benchmark string, picks []string) (map[string]alphavantage.Quote, error) {
	return nil, fmt.Errorf("not implemented")
}

type staticAlpha struct {
	quotes map[string]alphavantage.Quote
	err    error
}

func (s *staticAlpha) FetchPreviousClose(ctx context.Context, symbol string) (alphavantage.Quote, error) {
	if s.err != nil {
		return alphavantage.Quote{}, s.err
	}
	if quote, ok := s.quotes[symbol]; ok {
		return quote, nil
	}
	return alphavantage.Quote{Symbol: symbol}, nil
}

func (s *staticAlpha) SnapshotPreviousCloses(ctx context.Context, benchmark string, picks []string) (map[string]alphavantage.Quote, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestDailyCheckpointLoopSchedulesAndCompletes(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	runDate := "2026-01-05"
	startTime := time.Date(2026, 1, 5, 8, 0, 0, 0, location)
	clock := &fakeClock{now: startTime}
	sleeper := &fakeSleeper{clock: clock}
	store := &fakeStore{}

	alpha := &sequenceAlpha{
		nextTradingDay:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		benchmarkPrice:  "100.00",
		pickPrice:       "50.00",
		benchmarkSymbol: "SPY",
	}

	steps := &Steps{
		alphaVantage: alpha,
		store:        store,
		clock:        clock,
		sleeper:      sleeper,
	}

	state := WeeklyPickState{
		BatchID:               "batch-123",
		RunDate:               runDate,
		BenchmarkSymbol:       "SPY",
		BenchmarkInitialPrice: "95.00",
		Picks: []PickState{
			{PickID: "pick-1", Ticker: "AAPL", InitialPrice: "45.00"},
			{PickID: "pick-2", Ticker: "MSFT", InitialPrice: "60.00"},
		},
	}

	if err := steps.runDailyCheckpoints(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTargets := expectedDailyTargets(runDate, location)
	if len(sleeper.calls) != len(expectedTargets) {
		t.Fatalf("expected %d sleep calls, got %d", len(expectedTargets), len(sleeper.calls))
	}
	for i, target := range expectedTargets {
		if !sleeper.calls[i].Equal(target) {
			t.Fatalf("expected sleep target %s, got %s", target, sleeper.calls[i])
		}
	}

	if len(store.checkpoints) != 14 {
		t.Fatalf("expected 14 checkpoints, got %d", len(store.checkpoints))
	}

	expectedDates := expectedTradingDays(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 14)
	for i, input := range store.checkpoints {
		if input.Status != "computed" {
			t.Fatalf("expected computed status, got %s", input.Status)
		}
		if !input.CheckpointDate.Equal(expectedDates[i]) {
			t.Fatalf("expected checkpoint date %s, got %s", expectedDates[i], input.CheckpointDate)
		}
	}

	if len(store.statusUpdates) != 1 || store.statusUpdates[0] != "completed" {
		t.Fatalf("expected completed status update, got %v", store.statusUpdates)
	}
}

func TestDailyCheckpointSkippedWhenBenchmarkMissing(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 1, 6, 9, 0, 0, 0, location)}
	store := &fakeStore{}
	alpha := &staticAlpha{
		quotes: map[string]alphavantage.Quote{
			"SPY": {Symbol: "SPY", PreviousClose: "", TradingDay: ""},
		},
	}

	steps := &Steps{
		alphaVantage: alpha,
		store:        store,
		clock:        clock,
		sleeper:      &fakeSleeper{clock: clock},
	}

	state := WeeklyPickState{
		BatchID:               "batch-456",
		RunDate:               "2026-01-05",
		BenchmarkSymbol:       "SPY",
		BenchmarkInitialPrice: "100.00",
		Picks: []PickState{
			{PickID: "pick-1", Ticker: "AAPL", InitialPrice: "45.00"},
		},
	}

	scheduledAt := time.Date(2026, 1, 6, 9, 0, 0, 0, location)
	if err := steps.runDailyCheckpoint(context.Background(), state, scheduledAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(store.checkpoints))
	}
	input := store.checkpoints[0]
	if input.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", input.Status)
	}
	if input.BenchmarkPrice != nil || input.BenchmarkReturnPct != nil {
		t.Fatalf("expected null benchmark fields for skipped checkpoint")
	}
	if len(input.Metrics) != 0 {
		t.Fatalf("expected no metrics for skipped checkpoint")
	}
	expectedDate := previousWeekday(scheduledAt)
	if !input.CheckpointDate.Equal(expectedDate) {
		t.Fatalf("expected checkpoint date %s, got %s", expectedDate, input.CheckpointDate)
	}
}

func TestDailyCheckpointSkippedWhenPickMissing(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 1, 6, 9, 0, 0, 0, location)}
	store := &fakeStore{}
	alpha := &staticAlpha{
		quotes: map[string]alphavantage.Quote{
			"SPY":  {Symbol: "SPY", PreviousClose: "100.00", TradingDay: "2026-01-05"},
			"AAPL": {Symbol: "AAPL", PreviousClose: "", TradingDay: "2026-01-05"},
		},
	}

	steps := &Steps{
		alphaVantage: alpha,
		store:        store,
		clock:        clock,
		sleeper:      &fakeSleeper{clock: clock},
	}

	state := WeeklyPickState{
		BatchID:               "batch-789",
		RunDate:               "2026-01-05",
		BenchmarkSymbol:       "SPY",
		BenchmarkInitialPrice: "100.00",
		Picks: []PickState{
			{PickID: "pick-1", Ticker: "AAPL", InitialPrice: "45.00"},
		},
	}

	scheduledAt := time.Date(2026, 1, 6, 9, 0, 0, 0, location)
	if err := steps.runDailyCheckpoint(context.Background(), state, scheduledAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(store.checkpoints))
	}
	input := store.checkpoints[0]
	if input.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", input.Status)
	}
	if input.BenchmarkPrice != nil || input.BenchmarkReturnPct != nil {
		t.Fatalf("expected null benchmark fields for skipped checkpoint")
	}
	if len(input.Metrics) != 0 {
		t.Fatalf("expected no metrics for skipped checkpoint")
	}

	expectedDate, err := parseDate("2026-01-05")
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	if !input.CheckpointDate.Equal(expectedDate) {
		t.Fatalf("expected checkpoint date %s, got %s", expectedDate, input.CheckpointDate)
	}
}

func TestComputeMetrics(t *testing.T) {
	benchmarkReturn, err := calculateReturnPct("100", "95")
	if err != nil {
		t.Fatalf("benchmark return: %v", err)
	}
	absoluteReturn, err := calculateReturnPct("50", "55")
	if err != nil {
		t.Fatalf("absolute return: %v", err)
	}
	vsBenchmark, err := subtractDecimalStrings(absoluteReturn, benchmarkReturn)
	if err != nil {
		t.Fatalf("vs benchmark: %v", err)
	}

	if benchmarkReturn != "-5.00000000" {
		t.Fatalf("expected benchmark return -5.00000000, got %s", benchmarkReturn)
	}
	if absoluteReturn != "10.00000000" {
		t.Fatalf("expected absolute return 10.00000000, got %s", absoluteReturn)
	}
	if vsBenchmark != "15.00000000" {
		t.Fatalf("expected vs benchmark 15.00000000, got %s", vsBenchmark)
	}
}

func TestComputeMetricsRejectsInvalidInputs(t *testing.T) {
	if _, err := calculateReturnPct("0", "100"); err == nil {
		t.Fatalf("expected error for zero initial price")
	}
	if _, err := calculateReturnPct("-1", "100"); err == nil {
		t.Fatalf("expected error for negative initial price")
	}
	if _, err := calculateReturnPct("100", "-1"); err == nil {
		t.Fatalf("expected error for negative current price")
	}
}

func expectedDailyTargets(runDate string, location *time.Location) []time.Time {
	parsed, err := time.ParseInLocation("2006-01-02", runDate, location)
	if err != nil {
		panic(err)
	}
	targets := make([]time.Time, 0, 14)
	for i := 0; i < 14; i++ {
		targets = append(targets, time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 9, 0, 0, 0, location).AddDate(0, 0, i))
	}
	return targets
}

func expectedTradingDays(start time.Time, count int) []time.Time {
	result := make([]time.Time, 0, count)
	current := start
	for i := 0; i < count; i++ {
		result = append(result, time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC))
		current = nextWeekday(current.AddDate(0, 0, 1))
	}
	return result
}

func nextWeekday(candidate time.Time) time.Time {
	current := candidate
	for current.Weekday() == time.Saturday || current.Weekday() == time.Sunday {
		current = current.AddDate(0, 0, 1)
	}
	return current
}

func previousWeekday(at time.Time) time.Time {
	previous := at.AddDate(0, 0, -1)
	for previous.Weekday() == time.Saturday || previous.Weekday() == time.Sunday {
		previous = previous.AddDate(0, 0, -1)
	}
	return time.Date(previous.Year(), previous.Month(), previous.Day(), 0, 0, 0, 0, time.UTC)
}

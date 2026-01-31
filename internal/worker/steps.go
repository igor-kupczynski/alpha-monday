package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	hatchetworker "github.com/hatchet-dev/hatchet/pkg/worker"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/alphavantage"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/openai"
)

const (
	defaultBenchmarkSymbol = "SPY"
	dailyCheckpointDays    = 14
	dailyCheckpointHour    = 9
	dailyCheckpointMinute  = 0
	metricPrecisionScale   = 8
	priceFanoutConcurrency = 3
)

const (
	checkpointStatusComputed = "computed"
	checkpointStatusSkipped  = "skipped"
	batchStatusCompleted     = "completed"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

type sleepContext interface {
	context.Context
	SleepFor(duration time.Duration) (*hatchetworker.SingleWaitResult, error)
}

type Sleeper interface {
	SleepUntil(ctx sleepContext, target time.Time) error
}

type realSleeper struct {
	clock Clock
}

func (s realSleeper) SleepUntil(ctx sleepContext, target time.Time) error {
	if s.clock == nil {
		s.clock = realClock{}
	}
	now := s.clock.Now()
	if !target.After(now) {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("durable context is required for sleep")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := ctx.SleepFor(target.Sub(now))
	return err
}

type OpenAIClient interface {
	GeneratePicks(ctx context.Context) ([]openai.Pick, error)
}

type AlphaVantageClient interface {
	SnapshotPreviousCloses(ctx context.Context, benchmark string, picks []string) (map[string]alphavantage.Quote, error)
	FetchPreviousClose(ctx context.Context, symbol string) (alphavantage.Quote, error)
}

type Store interface {
	CreateBatchWithInitialCheckpoint(ctx context.Context, input db.CreateBatchInput) (db.CreateBatchResult, error)
	CreateCheckpointWithMetrics(ctx context.Context, input db.CreateCheckpointInput) (db.CreateCheckpointResult, error)
	UpdateBatchStatus(ctx context.Context, batchID string, status string) error
}

type Steps struct {
	openAI       OpenAIClient
	alphaVantage AlphaVantageClient
	store        Store
	logger       *slog.Logger
	clock        Clock
	sleeper      Sleeper
}

func NewSteps(store Store, openAI OpenAIClient, alpha AlphaVantageClient, logger *slog.Logger) *Steps {
	if logger == nil {
		logger = slog.Default()
	}
	steps := &Steps{
		openAI:       openAI,
		alphaVantage: alpha,
		store:        store,
		logger:       logger,
		clock:        realClock{},
	}
	steps.sleeper = realSleeper{clock: steps.clock}
	return steps
}

type PickDraft struct {
	Ticker    string `json:"ticker"`
	Action    string `json:"action"`
	Reasoning string `json:"reasoning"`
}

type GeneratePicksOutput struct {
	RunDate         string      `json:"run_date"`
	BenchmarkSymbol string      `json:"benchmark_symbol"`
	Picks           []PickDraft `json:"picks"`
}

type PickWithPrice struct {
	Ticker       string `json:"ticker"`
	Action       string `json:"action"`
	Reasoning    string `json:"reasoning"`
	InitialPrice string `json:"initial_price"`
}

type SnapshotOutput struct {
	RunDate               string          `json:"run_date"`
	BenchmarkSymbol       string          `json:"benchmark_symbol"`
	BenchmarkInitialPrice string          `json:"benchmark_initial_price"`
	CheckpointDate        string          `json:"checkpoint_date"`
	Picks                 []PickWithPrice `json:"picks"`
}

func (s *Steps) GeneratePicks(ctx hatchetworker.HatchetContext) (*GeneratePicksOutput, error) {
	if s.openAI == nil {
		return nil, fmt.Errorf("openai client not configured")
	}

	picks, err := s.openAI.GeneratePicks(ctx)
	if err != nil {
		return nil, err
	}

	drafts := make([]PickDraft, 0, len(picks))
	for _, pick := range picks {
		drafts = append(drafts, PickDraft{
			Ticker:    pick.Ticker,
			Action:    pick.Action,
			Reasoning: pick.Reasoning,
		})
	}

	runDate := formatDate(s.clock.Now())
	output := &GeneratePicksOutput{
		RunDate:         runDate,
		BenchmarkSymbol: defaultBenchmarkSymbol,
		Picks:           drafts,
	}

	s.logger.Info("picks generated", "run_date", runDate, "picks", drafts)

	return output, nil
}

func (s *Steps) SnapshotInitialPrices(ctx hatchetworker.HatchetContext) (*SnapshotOutput, error) {
	if s.alphaVantage == nil {
		return nil, fmt.Errorf("alpha vantage client not configured")
	}

	var input GeneratePicksOutput
	if err := ctx.StepOutput(StepGeneratePicksID, &input); err != nil {
		return nil, err
	}
	if len(input.Picks) == 0 {
		return nil, fmt.Errorf("no picks found from generate step")
	}

	tickers := make([]string, 0, len(input.Picks))
	for _, pick := range input.Picks {
		tickers = append(tickers, pick.Ticker)
	}

	prices, err := s.alphaVantage.SnapshotPreviousCloses(ctx, input.BenchmarkSymbol, tickers)
	if err != nil {
		return nil, err
	}

	benchmarkQuote, ok := prices[input.BenchmarkSymbol]
	if !ok {
		return nil, fmt.Errorf("missing benchmark quote for %s", input.BenchmarkSymbol)
	}
	if strings.TrimSpace(benchmarkQuote.PreviousClose) == "" {
		return nil, fmt.Errorf("missing benchmark price for %s", input.BenchmarkSymbol)
	}
	if strings.TrimSpace(benchmarkQuote.TradingDay) == "" {
		return nil, fmt.Errorf("missing benchmark trading day for %s", input.BenchmarkSymbol)
	}

	picks := make([]PickWithPrice, 0, len(input.Picks))
	for _, pick := range input.Picks {
		quote, ok := prices[pick.Ticker]
		if !ok {
			return nil, fmt.Errorf("missing quote for %s", pick.Ticker)
		}
		price := strings.TrimSpace(quote.PreviousClose)
		if price == "" {
			return nil, fmt.Errorf("missing previous close for %s", pick.Ticker)
		}
		picks = append(picks, PickWithPrice{
			Ticker:       pick.Ticker,
			Action:       pick.Action,
			Reasoning:    pick.Reasoning,
			InitialPrice: price,
		})
	}

	output := &SnapshotOutput{
		RunDate:               input.RunDate,
		BenchmarkSymbol:       input.BenchmarkSymbol,
		BenchmarkInitialPrice: benchmarkQuote.PreviousClose,
		CheckpointDate:        benchmarkQuote.TradingDay,
		Picks:                 picks,
	}

	s.logger.Info("initial prices snapped", "run_date", input.RunDate, "benchmark_price", benchmarkQuote.PreviousClose)

	return output, nil
}

func (s *Steps) PersistBatch(ctx hatchetworker.HatchetContext) (*WeeklyPickState, error) {
	if s.store == nil {
		return nil, fmt.Errorf("db store not configured")
	}

	var input SnapshotOutput
	if err := ctx.StepOutput(StepSnapshotPricesID, &input); err != nil {
		return nil, err
	}

	runDate, err := parseDate(input.RunDate)
	if err != nil {
		return nil, fmt.Errorf("invalid run_date %q: %w", input.RunDate, err)
	}
	checkpointDate, err := parseDate(input.CheckpointDate)
	if err != nil {
		return nil, fmt.Errorf("invalid checkpoint_date %q: %w", input.CheckpointDate, err)
	}

	picks := make([]db.NewPick, 0, len(input.Picks))
	for _, pick := range input.Picks {
		picks = append(picks, db.NewPick{
			Ticker:       pick.Ticker,
			Action:       pick.Action,
			Reasoning:    pick.Reasoning,
			InitialPrice: pick.InitialPrice,
		})
	}

	result, err := s.store.CreateBatchWithInitialCheckpoint(ctx, db.CreateBatchInput{
		RunDate:               runDate,
		BenchmarkSymbol:       input.BenchmarkSymbol,
		BenchmarkInitialPrice: input.BenchmarkInitialPrice,
		Status:                "active",
		Picks:                 picks,
		CheckpointDate:        checkpointDate,
		CheckpointStatus:      checkpointStatusComputed,
		BenchmarkPrice:        input.BenchmarkInitialPrice,
		BenchmarkReturnPct:    nil,
	})
	if err != nil {
		if errors.Is(err, db.ErrRunDateConflict) {
			return nil, fmt.Errorf("batch already exists for run_date %s: %w", input.RunDate, err)
		}
		return nil, err
	}

	state := &WeeklyPickState{
		BatchID:               result.BatchID,
		RunDate:               input.RunDate,
		BenchmarkSymbol:       input.BenchmarkSymbol,
		BenchmarkInitialPrice: input.BenchmarkInitialPrice,
		Picks:                 make([]PickState, 0, len(result.Picks)),
	}

	for _, pick := range result.Picks {
		state.Picks = append(state.Picks, PickState{
			PickID:       pick.ID,
			Ticker:       pick.Ticker,
			Action:       pick.Action,
			Reasoning:    pick.Reasoning,
			InitialPrice: pick.InitialPrice,
		})
	}

	s.logger.Info("batch persisted", "batch_id", result.BatchID, "checkpoint_id", result.CheckpointID, "picks", state.Picks)

	return state, nil
}

func (s *Steps) DailyCheckpointLoop(ctx hatchetworker.HatchetContext) error {
	if s.alphaVantage == nil {
		return fmt.Errorf("alpha vantage client not configured")
	}
	if s.store == nil {
		return fmt.Errorf("db store not configured")
	}
	if s.sleeper == nil {
		s.sleeper = realSleeper{clock: s.clock}
	}

	var state WeeklyPickState
	if err := ctx.StepOutput(StepPersistBatchID, &state); err != nil {
		return err
	}

	durableCtx := hatchetworker.NewDurableHatchetContext(ctx)
	return s.runDailyCheckpoints(durableCtx, state)
}

func (s *Steps) runDailyCheckpoints(ctx sleepContext, state WeeklyPickState) error {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("load timezone: %w", err)
	}

	runDate, err := parseDateInLocation(state.RunDate, location)
	if err != nil {
		return fmt.Errorf("invalid run_date %q: %w", state.RunDate, err)
	}

	base := time.Date(runDate.Year(), runDate.Month(), runDate.Day(), dailyCheckpointHour, dailyCheckpointMinute, 0, 0, location)
	for day := 0; day < dailyCheckpointDays; day++ {
		scheduledAt := base.AddDate(0, 0, day)
		if err := s.sleeper.SleepUntil(ctx, scheduledAt); err != nil {
			return err
		}
		if err := s.runDailyCheckpoint(ctx, state, scheduledAt); err != nil {
			return err
		}
	}

	if err := s.store.UpdateBatchStatus(ctx, state.BatchID, batchStatusCompleted); err != nil {
		return fmt.Errorf("update batch status: %w", err)
	}

	return nil
}

func (s *Steps) runDailyCheckpoint(ctx context.Context, state WeeklyPickState, scheduledAt time.Time) error {
	benchmarkQuote, err := s.alphaVantage.FetchPreviousClose(ctx, state.BenchmarkSymbol)
	if err != nil {
		return err
	}

	checkpointDate := previousTradingDayFallback(scheduledAt)
	if strings.TrimSpace(benchmarkQuote.PreviousClose) == "" {
		return s.persistCheckpoint(ctx, state, checkpointDate, nil, nil, nil, checkpointStatusSkipped)
	}
	if strings.TrimSpace(benchmarkQuote.TradingDay) == "" {
		return fmt.Errorf("missing benchmark trading day for %s", state.BenchmarkSymbol)
	}

	parsedDate, err := parseDate(benchmarkQuote.TradingDay)
	if err != nil {
		return fmt.Errorf("invalid trading day %q: %w", benchmarkQuote.TradingDay, err)
	}
	checkpointDate = parsedDate

	pickQuotes, err := s.fetchPickQuotes(ctx, state.Picks)
	if err != nil {
		return err
	}

	for _, pick := range state.Picks {
		quote := pickQuotes[pick.Ticker]
		if strings.TrimSpace(quote.PreviousClose) == "" {
			return s.persistCheckpoint(ctx, state, checkpointDate, nil, nil, nil, checkpointStatusSkipped)
		}
	}

	benchmarkPrice := strings.TrimSpace(benchmarkQuote.PreviousClose)
	benchmarkReturn, err := calculateReturnPct(state.BenchmarkInitialPrice, benchmarkPrice)
	if err != nil {
		return err
	}

	metrics := make([]db.NewCheckpointMetric, 0, len(state.Picks))
	for _, pick := range state.Picks {
		quote := pickQuotes[pick.Ticker]
		currentPrice := strings.TrimSpace(quote.PreviousClose)
		absoluteReturn, err := calculateReturnPct(pick.InitialPrice, currentPrice)
		if err != nil {
			return err
		}
		vsBenchmark, err := subtractDecimalStrings(absoluteReturn, benchmarkReturn)
		if err != nil {
			return err
		}

		metrics = append(metrics, db.NewCheckpointMetric{
			PickID:            pick.PickID,
			CurrentPrice:      currentPrice,
			AbsoluteReturnPct: absoluteReturn,
			VsBenchmarkPct:    vsBenchmark,
		})
	}

	return s.persistCheckpoint(ctx, state, checkpointDate, &benchmarkPrice, &benchmarkReturn, metrics, checkpointStatusComputed)
}

func (s *Steps) persistCheckpoint(ctx context.Context, state WeeklyPickState, checkpointDate time.Time, benchmarkPrice *string, benchmarkReturn *string, metrics []db.NewCheckpointMetric, status string) error {
	if s.logger == nil {
		s.logger = slog.Default()
	}
	_, err := s.store.CreateCheckpointWithMetrics(ctx, db.CreateCheckpointInput{
		BatchID:            state.BatchID,
		CheckpointDate:     checkpointDate,
		Status:             status,
		BenchmarkPrice:     benchmarkPrice,
		BenchmarkReturnPct: benchmarkReturn,
		Metrics:            metrics,
	})
	if err != nil {
		if errors.Is(err, db.ErrCheckpointConflict) {
			s.logger.Info("checkpoint already exists", "batch_id", state.BatchID, "checkpoint_date", checkpointDate)
			return nil
		}
		return err
	}
	return nil
}

func (s *Steps) fetchPickQuotes(ctx context.Context, picks []PickState) (map[string]alphavantage.Quote, error) {
	tickers := make([]string, 0, len(picks))
	seen := map[string]struct{}{}
	for _, pick := range picks {
		ticker := strings.TrimSpace(pick.Ticker)
		if ticker == "" {
			return nil, fmt.Errorf("pick ticker is required")
		}
		if _, ok := seen[ticker]; ok {
			continue
		}
		seen[ticker] = struct{}{}
		tickers = append(tickers, ticker)
	}

	type result struct {
		ticker string
		quote  alphavantage.Quote
		err    error
	}

	results := make(chan result, len(tickers))
	sem := make(chan struct{}, priceFanoutConcurrency)

	for _, ticker := range tickers {
		sem <- struct{}{}
		go func(symbol string) {
			defer func() { <-sem }()
			quote, err := s.alphaVantage.FetchPreviousClose(ctx, symbol)
			results <- result{ticker: symbol, quote: quote, err: err}
		}(ticker)
	}

	quotes := make(map[string]alphavantage.Quote, len(tickers))
	for i := 0; i < len(tickers); i++ {
		res := <-results
		if res.err != nil {
			return nil, res.err
		}
		quotes[res.ticker] = res.quote
	}

	return quotes, nil
}

func calculateReturnPct(initialValue, currentValue string) (string, error) {
	initial, err := parsePositiveDecimal(initialValue, "initial")
	if err != nil {
		return "", err
	}
	current, err := parsePositiveDecimal(currentValue, "current")
	if err != nil {
		return "", err
	}

	diff := new(big.Rat).Sub(current, initial)
	diff.Mul(diff, big.NewRat(100, 1))
	result := new(big.Rat).Quo(diff, initial)

	return formatDecimal(result), nil
}

func subtractDecimalStrings(left, right string) (string, error) {
	leftRat, err := parseDecimal(left)
	if err != nil {
		return "", fmt.Errorf("invalid decimal %q: %w", left, err)
	}
	rightRat, err := parseDecimal(right)
	if err != nil {
		return "", fmt.Errorf("invalid decimal %q: %w", right, err)
	}
	result := new(big.Rat).Sub(leftRat, rightRat)
	return formatDecimal(result), nil
}

func parseDecimal(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("value is required")
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", value)
	}
	return rat, nil
}

func parsePositiveDecimal(value string, label string) (*big.Rat, error) {
	rat, err := parseDecimal(value)
	if err != nil {
		return nil, err
	}
	if rat.Sign() <= 0 {
		return nil, fmt.Errorf("%s value must be positive", label)
	}
	return rat, nil
}

func formatDecimal(value *big.Rat) string {
	return value.FloatString(metricPrecisionScale)
}

func parseDateInLocation(value string, location *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, location), nil
}

func previousTradingDayFallback(scheduledAt time.Time) time.Time {
	previous := scheduledAt.AddDate(0, 0, -1)
	for previous.Weekday() == time.Saturday || previous.Weekday() == time.Sunday {
		previous = previous.AddDate(0, 0, -1)
	}
	return time.Date(previous.Year(), previous.Month(), previous.Day(), 0, 0, 0, 0, time.UTC)
}

func formatDate(now time.Time) string {
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return date.Format("2006-01-02")
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), nil
}

var _ context.Context = hatchetworker.HatchetContext(nil)

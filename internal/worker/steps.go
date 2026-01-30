package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	hatchetworker "github.com/hatchet-dev/hatchet/pkg/worker"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/alphavantage"
	"github.com/igor-kupczynski/alpha-monday/internal/integrations/openai"
)

const defaultBenchmarkSymbol = "SPY"

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

type Steps struct {
	openAI       *openai.Client
	alphaVantage *alphavantage.Client
	store        *db.Store
	logger       *slog.Logger
	clock        Clock
}

func NewSteps(store *db.Store, openAI *openai.Client, alpha *alphavantage.Client, logger *slog.Logger) *Steps {
	if logger == nil {
		logger = slog.Default()
	}
	return &Steps{
		openAI:       openAI,
		alphaVantage: alpha,
		store:        store,
		logger:       logger,
		clock:        realClock{},
	}
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

	benchmarkPrice := prices[input.BenchmarkSymbol]
	if benchmarkPrice == "" {
		return nil, fmt.Errorf("missing benchmark price for %s", input.BenchmarkSymbol)
	}

	picks := make([]PickWithPrice, 0, len(input.Picks))
	for _, pick := range input.Picks {
		price := prices[pick.Ticker]
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
		BenchmarkInitialPrice: benchmarkPrice,
		Picks:                 picks,
	}

	s.logger.Info("initial prices snapped", "run_date", input.RunDate, "benchmark_price", benchmarkPrice)

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
		CheckpointDate:        runDate,
		CheckpointStatus:      "computed",
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

package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestCreateBatchWithInitialCheckpoint(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)
	runDate := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)

	input := CreateBatchInput{
		RunDate:               runDate,
		BenchmarkSymbol:       "SPY",
		BenchmarkInitialPrice: "401.25",
		Status:                "active",
		Picks: []NewPick{
			{Ticker: "AAPL", Action: "BUY", Reasoning: "ok", InitialPrice: "178.10"},
			{Ticker: "MSFT", Action: "SELL", Reasoning: "ok", InitialPrice: "342.55"},
			{Ticker: "NVDA", Action: "BUY", Reasoning: "ok", InitialPrice: "610.00"},
		},
		CheckpointDate:   runDate,
		CheckpointStatus: "computed",
		BenchmarkPrice:   "401.25",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := store.CreateBatchWithInitialCheckpoint(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BatchID == "" {
		t.Fatalf("expected batch id")
	}
	if result.CheckpointID == "" {
		t.Fatalf("expected checkpoint id")
	}
	if len(result.Picks) != 3 {
		t.Fatalf("expected 3 pick ids, got %d", len(result.Picks))
	}

	var batchCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM batches").Scan(&batchCount); err != nil {
		t.Fatalf("count batches: %v", err)
	}
	if batchCount != 1 {
		t.Fatalf("expected 1 batch, got %d", batchCount)
	}

	var pickCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM picks").Scan(&pickCount); err != nil {
		t.Fatalf("count picks: %v", err)
	}
	if pickCount != 3 {
		t.Fatalf("expected 3 picks, got %d", pickCount)
	}

	var checkpointCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM checkpoints").Scan(&checkpointCount); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if checkpointCount != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", checkpointCount)
	}

	var benchmarkPrice string
	var benchmarkReturn sql.NullString
	row := testPool.QueryRow(ctx, `SELECT benchmark_price::text, benchmark_return_pct::text FROM checkpoints WHERE id = $1`, result.CheckpointID)
	if err := row.Scan(&benchmarkPrice, &benchmarkReturn); err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if benchmarkPrice != input.BenchmarkPrice {
		t.Fatalf("expected benchmark price %s, got %s", input.BenchmarkPrice, benchmarkPrice)
	}
	if benchmarkReturn.Valid {
		t.Fatalf("expected null benchmark_return_pct for initial checkpoint")
	}
}

func TestCreateBatchWithInitialCheckpointRunDateConflict(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)
	runDate := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)

	input := CreateBatchInput{
		RunDate:               runDate,
		BenchmarkSymbol:       "SPY",
		BenchmarkInitialPrice: "401.25",
		Status:                "active",
		Picks: []NewPick{
			{Ticker: "AAPL", Action: "BUY", Reasoning: "ok", InitialPrice: "178.10"},
			{Ticker: "MSFT", Action: "SELL", Reasoning: "ok", InitialPrice: "342.55"},
			{Ticker: "NVDA", Action: "BUY", Reasoning: "ok", InitialPrice: "610.00"},
		},
		CheckpointDate:   runDate,
		CheckpointStatus: "computed",
		BenchmarkPrice:   "401.25",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := store.CreateBatchWithInitialCheckpoint(ctx, input); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	_, err := store.CreateBatchWithInitialCheckpoint(ctx, input)
	if err == nil {
		t.Fatalf("expected run_date conflict error")
	}
	if !errors.Is(err, ErrRunDateConflict) {
		t.Fatalf("expected ErrRunDateConflict, got %v", err)
	}

	var batchCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM batches").Scan(&batchCount); err != nil {
		t.Fatalf("count batches: %v", err)
	}
	if batchCount != 1 {
		t.Fatalf("expected 1 batch, got %d", batchCount)
	}

	var pickCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM picks").Scan(&pickCount); err != nil {
		t.Fatalf("count picks: %v", err)
	}
	if pickCount != 3 {
		t.Fatalf("expected 3 picks, got %d", pickCount)
	}

	var checkpointCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM checkpoints").Scan(&checkpointCount); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if checkpointCount != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", checkpointCount)
	}
}

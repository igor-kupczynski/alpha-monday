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

func TestCreateCheckpointWithMetricsComputed(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)
	batchID := "11111111-2222-3333-4444-555555555555"
	pick1ID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	pick2ID := "ffffffff-1111-2222-3333-444444444444"

	if err := seedBatch(batchID, "2026-01-27", "SPY", "401.25", "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}
	if err := seedPick(pick1ID, batchID, "AAPL", "BUY", "ok", "178.10"); err != nil {
		t.Fatalf("seed pick1: %v", err)
	}
	if err := seedPick(pick2ID, batchID, "MSFT", "SELL", "ok", "342.55"); err != nil {
		t.Fatalf("seed pick2: %v", err)
	}

	checkpointDate := time.Date(2026, 1, 28, 0, 0, 0, 0, time.UTC)
	benchmarkPrice := "410.00"
	benchmarkReturn := "2.18200000"

	input := CreateCheckpointInput{
		BatchID:            batchID,
		CheckpointDate:     checkpointDate,
		Status:             "computed",
		BenchmarkPrice:     &benchmarkPrice,
		BenchmarkReturnPct: &benchmarkReturn,
		Metrics: []NewCheckpointMetric{
			{
				PickID:            pick1ID,
				CurrentPrice:      "181.00",
				AbsoluteReturnPct: "1.62900000",
				VsBenchmarkPct:    "-0.55300000",
			},
			{
				PickID:            pick2ID,
				CurrentPrice:      "335.00",
				AbsoluteReturnPct: "-2.20600000",
				VsBenchmarkPct:    "-4.38800000",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := store.CreateCheckpointWithMetrics(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CheckpointID == "" {
		t.Fatalf("expected checkpoint id")
	}

	var checkpointCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM checkpoints").Scan(&checkpointCount); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if checkpointCount != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", checkpointCount)
	}

	var metricCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM pick_checkpoint_metrics").Scan(&metricCount); err != nil {
		t.Fatalf("count metrics: %v", err)
	}
	if metricCount != 2 {
		t.Fatalf("expected 2 metrics, got %d", metricCount)
	}

	var storedPrice string
	var storedReturn string
	row := testPool.QueryRow(ctx, `SELECT benchmark_price::text, benchmark_return_pct::text FROM checkpoints WHERE id = $1`, result.CheckpointID)
	if err := row.Scan(&storedPrice, &storedReturn); err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if storedPrice != benchmarkPrice {
		t.Fatalf("expected benchmark price %s, got %s", benchmarkPrice, storedPrice)
	}
	if storedReturn != benchmarkReturn {
		t.Fatalf("expected benchmark return %s, got %s", benchmarkReturn, storedReturn)
	}
}

func TestCreateCheckpointWithMetricsSkipped(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)
	batchID := "22222222-3333-4444-5555-666666666666"

	if err := seedBatch(batchID, "2026-01-27", "SPY", "401.25", "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	checkpointDate := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)

	input := CreateCheckpointInput{
		BatchID:        batchID,
		CheckpointDate: checkpointDate,
		Status:         "skipped",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := store.CreateCheckpointWithMetrics(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CheckpointID == "" {
		t.Fatalf("expected checkpoint id")
	}

	var benchmarkPrice sql.NullString
	var benchmarkReturn sql.NullString
	row := testPool.QueryRow(ctx, `SELECT benchmark_price::text, benchmark_return_pct::text FROM checkpoints WHERE id = $1`, result.CheckpointID)
	if err := row.Scan(&benchmarkPrice, &benchmarkReturn); err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if benchmarkPrice.Valid || benchmarkReturn.Valid {
		t.Fatalf("expected null benchmark fields for skipped checkpoint")
	}

	var metricCount int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM pick_checkpoint_metrics").Scan(&metricCount); err != nil {
		t.Fatalf("count metrics: %v", err)
	}
	if metricCount != 0 {
		t.Fatalf("expected 0 metrics, got %d", metricCount)
	}
}

func TestCreateCheckpointWithMetricsConflict(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)
	batchID := "33333333-4444-5555-6666-777777777777"

	if err := seedBatch(batchID, "2026-01-27", "SPY", "401.25", "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	checkpointDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	if err := seedCheckpoint("99999999-0000-1111-2222-333333333333", batchID, "2026-01-30", "skipped", "0", "0"); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := store.CreateCheckpointWithMetrics(ctx, CreateCheckpointInput{
		BatchID:        batchID,
		CheckpointDate: checkpointDate,
		Status:         "skipped",
	})
	if err == nil {
		t.Fatalf("expected checkpoint conflict error")
	}
	if !errors.Is(err, ErrCheckpointConflict) {
		t.Fatalf("expected ErrCheckpointConflict, got %v", err)
	}
}

func TestUpdateBatchStatus(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)
	batchID := "44444444-5555-6666-7777-888888888888"

	if err := seedBatch(batchID, "2026-01-27", "SPY", "401.25", "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.UpdateBatchStatus(ctx, batchID, "completed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	if err := testPool.QueryRow(ctx, "SELECT status FROM batches WHERE id = $1", batchID).Scan(&status); err != nil {
		t.Fatalf("read batch: %v", err)
	}
	if status != "completed" {
		t.Fatalf("expected status completed, got %s", status)
	}
}

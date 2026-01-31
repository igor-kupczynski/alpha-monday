package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrRunDateConflict = errors.New("run_date already exists")
var ErrCheckpointConflict = errors.New("checkpoint already exists")

type NewPick struct {
	Ticker       string
	Action       string
	Reasoning    string
	InitialPrice string
}

type CreateBatchInput struct {
	RunDate               time.Time
	BenchmarkSymbol       string
	BenchmarkInitialPrice string
	Status                string
	Picks                 []NewPick
	CheckpointDate        time.Time
	CheckpointStatus      string
	BenchmarkPrice        string
	BenchmarkReturnPct    *string
}

type CreateBatchResult struct {
	BatchID      string
	CheckpointID string
	Picks        []Pick
}

type NewCheckpointMetric struct {
	PickID            string
	CurrentPrice      string
	AbsoluteReturnPct string
	VsBenchmarkPct    string
}

type CreateCheckpointInput struct {
	BatchID            string
	CheckpointDate     time.Time
	Status             string
	BenchmarkPrice     *string
	BenchmarkReturnPct *string
	Metrics            []NewCheckpointMetric
}

type CreateCheckpointResult struct {
	CheckpointID string
}

func (s *Store) CreateBatchWithInitialCheckpoint(ctx context.Context, input CreateBatchInput) (CreateBatchResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreateBatchResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	batchID := uuid.New()
	_, err = tx.Exec(ctx, `
        INSERT INTO batches (id, run_date, benchmark_symbol, benchmark_initial_price, status)
        VALUES ($1, $2, $3, $4, $5)`,
		batchID,
		input.RunDate,
		input.BenchmarkSymbol,
		input.BenchmarkInitialPrice,
		input.Status,
	)
	if err != nil {
		if isRunDateConflict(err) {
			return CreateBatchResult{}, ErrRunDateConflict
		}
		return CreateBatchResult{}, err
	}

	picks := make([]Pick, 0, len(input.Picks))
	for _, pick := range input.Picks {
		pickID := uuid.New()
		_, err := tx.Exec(ctx, `
            INSERT INTO picks (id, batch_id, ticker, action, reasoning, initial_price)
            VALUES ($1, $2, $3, $4, $5, $6)`,
			pickID,
			batchID,
			pick.Ticker,
			pick.Action,
			pick.Reasoning,
			pick.InitialPrice,
		)
		if err != nil {
			return CreateBatchResult{}, err
		}
		picks = append(picks, Pick{
			ID:           pickID.String(),
			Ticker:       pick.Ticker,
			Action:       pick.Action,
			Reasoning:    pick.Reasoning,
			InitialPrice: pick.InitialPrice,
		})
	}

	checkpointID := uuid.New()
	_, err = tx.Exec(ctx, `
        INSERT INTO checkpoints (id, batch_id, checkpoint_date, status, benchmark_price, benchmark_return_pct)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		checkpointID,
		batchID,
		input.CheckpointDate,
		input.CheckpointStatus,
		input.BenchmarkPrice,
		input.BenchmarkReturnPct,
	)
	if err != nil {
		return CreateBatchResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateBatchResult{}, err
	}

	return CreateBatchResult{
		BatchID:      batchID.String(),
		CheckpointID: checkpointID.String(),
		Picks:        picks,
	}, nil
}

func (s *Store) CreateCheckpointWithMetrics(ctx context.Context, input CreateCheckpointInput) (CreateCheckpointResult, error) {
	if input.Status == "computed" {
		if input.BenchmarkPrice == nil || input.BenchmarkReturnPct == nil {
			return CreateCheckpointResult{}, errors.New("benchmark price and return are required for computed checkpoint")
		}
	} else if input.Status == "skipped" {
		if input.BenchmarkPrice != nil || input.BenchmarkReturnPct != nil || len(input.Metrics) > 0 {
			return CreateCheckpointResult{}, errors.New("skipped checkpoint cannot include benchmark metrics or pick metrics")
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreateCheckpointResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	checkpointID := uuid.New()
	_, err = tx.Exec(ctx, `
        INSERT INTO checkpoints (id, batch_id, checkpoint_date, status, benchmark_price, benchmark_return_pct)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		checkpointID,
		input.BatchID,
		input.CheckpointDate,
		input.Status,
		input.BenchmarkPrice,
		input.BenchmarkReturnPct,
	)
	if err != nil {
		if isCheckpointConflict(err) {
			return CreateCheckpointResult{}, ErrCheckpointConflict
		}
		return CreateCheckpointResult{}, err
	}

	for _, metric := range input.Metrics {
		metricID := uuid.New()
		_, err := tx.Exec(ctx, `
            INSERT INTO pick_checkpoint_metrics (id, checkpoint_id, pick_id, current_price, absolute_return_pct, vs_benchmark_pct)
            VALUES ($1, $2, $3, $4, $5, $6)`,
			metricID,
			checkpointID,
			metric.PickID,
			metric.CurrentPrice,
			metric.AbsoluteReturnPct,
			metric.VsBenchmarkPct,
		)
		if err != nil {
			return CreateCheckpointResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateCheckpointResult{}, err
	}

	return CreateCheckpointResult{CheckpointID: checkpointID.String()}, nil
}

func (s *Store) UpdateBatchStatus(ctx context.Context, batchID string, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE batches SET status = $2 WHERE id = $1`, batchID, status)
	return err
}

func isRunDateConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if pgErr.ConstraintName == "batches_run_date_unique" {
		return true
	}
	return false
}

func isCheckpointConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if pgErr.ConstraintName == "checkpoints_batch_date_unique" {
		return true
	}
	return false
}

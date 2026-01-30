package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrRunDateConflict = errors.New("run_date already exists")

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

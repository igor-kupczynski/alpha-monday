package db

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

type Batch struct {
	ID                    string
	RunDate               string
	Status                string
	BenchmarkSymbol       string
	BenchmarkInitialPrice string
}

type Pick struct {
	ID           string
	Ticker       string
	Action       string
	Reasoning    string
	InitialPrice string
}

type PickMetric struct {
	ID                string
	PickID            string
	CurrentPrice      string
	AbsoluteReturnPct string
	VsBenchmarkPct    string
}

type Checkpoint struct {
	ID                 string
	CheckpointDate     string
	Status             string
	BenchmarkPrice     *string
	BenchmarkReturnPct *string
	Metrics            []PickMetric
}

type LatestBatchResult struct {
	Batch            Batch
	Picks            []Pick
	LatestCheckpoint *Checkpoint
}

type BatchesPage struct {
	Batches    []Batch
	NextCursor *string
}

type BatchDetails struct {
	Batch       Batch
	Picks       []Pick
	Checkpoints []Checkpoint
}

func (s *Store) LatestBatch(ctx context.Context) (*LatestBatchResult, error) {
	const latestBatchSQL = `
        SELECT id::text, run_date::text, status, benchmark_symbol, benchmark_initial_price::text
        FROM batches
        ORDER BY run_date DESC
        LIMIT 1`

	var batch Batch
	row := s.pool.QueryRow(ctx, latestBatchSQL)
	if err := row.Scan(&batch.ID, &batch.RunDate, &batch.Status, &batch.BenchmarkSymbol, &batch.BenchmarkInitialPrice); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	picks, err := s.listPicks(ctx, batch.ID)
	if err != nil {
		return nil, err
	}

	checkpoint, err := s.latestCheckpoint(ctx, batch.ID)
	if err != nil {
		return nil, err
	}

	return &LatestBatchResult{
		Batch:            batch,
		Picks:            picks,
		LatestCheckpoint: checkpoint,
	}, nil
}

func (s *Store) ListBatches(ctx context.Context, limit int, cursor *string) (BatchesPage, error) {
	const listSQL = `
        SELECT id::text, run_date::text, status, benchmark_symbol, benchmark_initial_price::text
        FROM batches
        ORDER BY run_date DESC
        LIMIT $1`
	const listCursorSQL = `
        SELECT id::text, run_date::text, status, benchmark_symbol, benchmark_initial_price::text
        FROM batches
        WHERE run_date < $1::date
        ORDER BY run_date DESC
        LIMIT $2`

	queryLimit := limit + 1
	var rows pgx.Rows
	var err error

	if cursor != nil {
		rows, err = s.pool.Query(ctx, listCursorSQL, *cursor, queryLimit)
	} else {
		rows, err = s.pool.Query(ctx, listSQL, queryLimit)
	}
	if err != nil {
		return BatchesPage{}, err
	}
	defer rows.Close()

	batches := make([]Batch, 0, limit)
	for rows.Next() {
		var batch Batch
		if err := rows.Scan(&batch.ID, &batch.RunDate, &batch.Status, &batch.BenchmarkSymbol, &batch.BenchmarkInitialPrice); err != nil {
			return BatchesPage{}, err
		}
		batches = append(batches, batch)
	}
	if err := rows.Err(); err != nil {
		return BatchesPage{}, err
	}

	var nextCursor *string
	if len(batches) > limit {
		last := batches[limit-1].RunDate
		nextCursor = &last
		batches = batches[:limit]
	}

	return BatchesPage{Batches: batches, NextCursor: nextCursor}, nil
}

func (s *Store) BatchDetails(ctx context.Context, batchID string) (*BatchDetails, error) {
	const batchSQL = `
        SELECT id::text, run_date::text, status, benchmark_symbol, benchmark_initial_price::text
        FROM batches
        WHERE id = $1`

	var batch Batch
	row := s.pool.QueryRow(ctx, batchSQL, batchID)
	if err := row.Scan(&batch.ID, &batch.RunDate, &batch.Status, &batch.BenchmarkSymbol, &batch.BenchmarkInitialPrice); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	picks, err := s.listPicks(ctx, batch.ID)
	if err != nil {
		return nil, err
	}

	checkpoints, err := s.listCheckpoints(ctx, batch.ID)
	if err != nil {
		return nil, err
	}

	if len(checkpoints) > 0 {
		metrics, err := s.listMetricsForBatch(ctx, batch.ID)
		if err != nil {
			return nil, err
		}
		metricsByCheckpoint := map[string][]PickMetric{}
		for _, metric := range metrics {
			metricsByCheckpoint[metric.checkpointID] = append(metricsByCheckpoint[metric.checkpointID], metric.metric)
		}
		for i := range checkpoints {
			checkpoints[i].Metrics = metricsByCheckpoint[checkpoints[i].ID]
		}
	}

	return &BatchDetails{
		Batch:       batch,
		Picks:       picks,
		Checkpoints: checkpoints,
	}, nil
}

type metricRow struct {
	checkpointID string
	metric       PickMetric
}

func (s *Store) listMetricsForBatch(ctx context.Context, batchID string) ([]metricRow, error) {
	const metricsSQL = `
        SELECT m.id::text, m.checkpoint_id::text, m.pick_id::text,
               m.current_price::text, m.absolute_return_pct::text, m.vs_benchmark_pct::text
        FROM pick_checkpoint_metrics m
        JOIN checkpoints c ON c.id = m.checkpoint_id
        WHERE c.batch_id = $1
        ORDER BY c.checkpoint_date ASC, m.pick_id`

	rows, err := s.pool.Query(ctx, metricsSQL, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []metricRow
	for rows.Next() {
		var row metricRow
		var metric PickMetric
		if err := rows.Scan(&metric.ID, &row.checkpointID, &metric.PickID, &metric.CurrentPrice, &metric.AbsoluteReturnPct, &metric.VsBenchmarkPct); err != nil {
			return nil, err
		}
		row.metric = metric
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) listPicks(ctx context.Context, batchID string) ([]Pick, error) {
	const picksSQL = `
        SELECT id::text, ticker, action, reasoning, initial_price::text
        FROM picks
        WHERE batch_id = $1
        ORDER BY ticker`

	rows, err := s.pool.Query(ctx, picksSQL, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var picks []Pick
	for rows.Next() {
		var pick Pick
		if err := rows.Scan(&pick.ID, &pick.Ticker, &pick.Action, &pick.Reasoning, &pick.InitialPrice); err != nil {
			return nil, err
		}
		picks = append(picks, pick)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return picks, nil
}

func (s *Store) listCheckpoints(ctx context.Context, batchID string) ([]Checkpoint, error) {
	const checkpointsSQL = `
        SELECT id::text, checkpoint_date::text, status,
               benchmark_price::text, benchmark_return_pct::text
        FROM checkpoints
        WHERE batch_id = $1
        ORDER BY checkpoint_date ASC`

	rows, err := s.pool.Query(ctx, checkpointsSQL, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkpoints []Checkpoint
	for rows.Next() {
		var checkpoint Checkpoint
		var benchmarkPrice sql.NullString
		var benchmarkReturn sql.NullString
		if err := rows.Scan(&checkpoint.ID, &checkpoint.CheckpointDate, &checkpoint.Status, &benchmarkPrice, &benchmarkReturn); err != nil {
			return nil, err
		}
		checkpoint.BenchmarkPrice = nullStringPtr(benchmarkPrice)
		checkpoint.BenchmarkReturnPct = nullStringPtr(benchmarkReturn)
		checkpoints = append(checkpoints, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return checkpoints, nil
}

func (s *Store) latestCheckpoint(ctx context.Context, batchID string) (*Checkpoint, error) {
	const latestCheckpointSQL = `
        SELECT id::text, checkpoint_date::text, status,
               benchmark_price::text, benchmark_return_pct::text
        FROM checkpoints
        WHERE batch_id = $1
        ORDER BY checkpoint_date DESC
        LIMIT 1`

	var checkpoint Checkpoint
	var benchmarkPrice sql.NullString
	var benchmarkReturn sql.NullString

	row := s.pool.QueryRow(ctx, latestCheckpointSQL, batchID)
	if err := row.Scan(&checkpoint.ID, &checkpoint.CheckpointDate, &checkpoint.Status, &benchmarkPrice, &benchmarkReturn); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	checkpoint.BenchmarkPrice = nullStringPtr(benchmarkPrice)
	checkpoint.BenchmarkReturnPct = nullStringPtr(benchmarkReturn)

	metrics, err := s.listMetricsForCheckpoint(ctx, checkpoint.ID)
	if err != nil {
		return nil, err
	}
	checkpoint.Metrics = metrics

	return &checkpoint, nil
}

func (s *Store) listMetricsForCheckpoint(ctx context.Context, checkpointID string) ([]PickMetric, error) {
	const metricsSQL = `
        SELECT id::text, pick_id::text, current_price::text, absolute_return_pct::text, vs_benchmark_pct::text
        FROM pick_checkpoint_metrics
        WHERE checkpoint_id = $1
        ORDER BY pick_id`

	rows, err := s.pool.Query(ctx, metricsSQL, checkpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []PickMetric
	for rows.Next() {
		var metric PickMetric
		if err := rows.Scan(&metric.ID, &metric.PickID, &metric.CurrentPrice, &metric.AbsoluteReturnPct, &metric.VsBenchmarkPct); err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return metrics, nil
}

func nullStringPtr(value sql.NullString) *string {
	if value.Valid {
		return &value.String
	}
	return nil
}

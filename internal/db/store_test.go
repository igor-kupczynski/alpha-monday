package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

var (
	testPool    *pgxpool.Pool
	databaseURL string
	lockConn    *sql.Conn
)

const advisoryLockID int64 = 424242

func TestMain(m *testing.M) {
	databaseURL = getenvDefault("DATABASE_URL", "postgres://alpha:alpha@localhost:5432/alpha_monday?sslmode=disable")

	sqlDB, err := sql.Open("postgres", databaseURL)
	if err != nil {
		failFast("open db", err)
	}
	sqlDB.SetMaxOpenConns(4)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		failFast("ping db", err)
	}

	lockConn, err = sqlDB.Conn(ctx)
	if err != nil {
		failFast("lock connection", err)
	}
	if _, err := lockConn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		failFast("advisory lock", err)
	}

	if err := resetSchema(sqlDB); err != nil {
		failFast("reset schema", err)
	}
	if err := runMigrations(databaseURL); err != nil {
		failFast("run migrations", err)
	}

	testPool, err = pgxpool.New(ctx, databaseURL)
	if err != nil {
		failFast("pgxpool", err)
	}

	code := m.Run()

	testPool.Close()
	if _, err := lockConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to release advisory lock: %v\n", err)
	}
	_ = lockConn.Close()
	_ = sqlDB.Close()

	os.Exit(code)
}

func TestLatestBatchQuery(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)

	batch1ID := "11111111-1111-1111-1111-111111111111"
	batch2ID := "22222222-2222-2222-2222-222222222222"

	if err := seedBatch(batch1ID, "2026-01-13", "SPY", "400.00", "completed"); err != nil {
		t.Fatalf("seed batch1: %v", err)
	}
	if err := seedBatch(batch2ID, "2026-01-20", "SPY", "410.00", "active"); err != nil {
		t.Fatalf("seed batch2: %v", err)
	}

	pick1ID := "33333333-3333-3333-3333-333333333333"
	pick2ID := "44444444-4444-4444-4444-444444444444"

	if err := seedPick(pick1ID, batch2ID, "AAPL", "BUY", "reason", "150.00"); err != nil {
		t.Fatalf("seed pick1: %v", err)
	}
	if err := seedPick(pick2ID, batch2ID, "MSFT", "SELL", "reason", "320.00"); err != nil {
		t.Fatalf("seed pick2: %v", err)
	}

	checkpointID := "55555555-5555-5555-5555-555555555555"
	if err := seedCheckpoint(checkpointID, batch2ID, "2026-01-21", "computed", "412.00", "0.0049"); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	if err := seedMetric("66666666-6666-6666-6666-666666666666", checkpointID, pick1ID, "151.00", "0.0067", "0.0018"); err != nil {
		t.Fatalf("seed metric1: %v", err)
	}
	if err := seedMetric("77777777-7777-7777-7777-777777777777", checkpointID, pick2ID, "318.00", "-0.0062", "-0.0111"); err != nil {
		t.Fatalf("seed metric2: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latest, err := store.LatestBatch(ctx)
	if err != nil {
		t.Fatalf("latest batch: %v", err)
	}
	if latest == nil {
		t.Fatalf("expected latest batch")
	}
	if latest.Batch.ID != batch2ID {
		t.Fatalf("expected latest batch %s, got %s", batch2ID, latest.Batch.ID)
	}
	if latest.Batch.RunDate != "2026-01-20" {
		t.Fatalf("expected run_date 2026-01-20, got %s", latest.Batch.RunDate)
	}
	if len(latest.Picks) != 2 {
		t.Fatalf("expected 2 picks, got %d", len(latest.Picks))
	}
	if latest.LatestCheckpoint == nil {
		t.Fatalf("expected latest checkpoint")
	}
	if latest.LatestCheckpoint.CheckpointDate != "2026-01-21" {
		t.Fatalf("expected checkpoint_date 2026-01-21, got %s", latest.LatestCheckpoint.CheckpointDate)
	}
	if len(latest.LatestCheckpoint.Metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(latest.LatestCheckpoint.Metrics))
	}
}

func TestListBatchesPagination(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)

	if err := seedBatch("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "2026-01-06", "SPY", "390.00", "completed"); err != nil {
		t.Fatalf("seed batch1: %v", err)
	}
	if err := seedBatch("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "2026-01-13", "SPY", "400.00", "completed"); err != nil {
		t.Fatalf("seed batch2: %v", err)
	}
	if err := seedBatch("cccccccc-cccc-cccc-cccc-cccccccccccc", "2026-01-20", "SPY", "410.00", "active"); err != nil {
		t.Fatalf("seed batch3: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	page, err := store.ListBatches(ctx, 2, nil)
	if err != nil {
		t.Fatalf("list batches: %v", err)
	}
	if len(page.Batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(page.Batches))
	}
	if page.Batches[0].RunDate != "2026-01-20" {
		t.Fatalf("expected first batch 2026-01-20, got %s", page.Batches[0].RunDate)
	}
	if page.NextCursor == nil {
		t.Fatalf("expected next_cursor")
	}

	page2, err := store.ListBatches(ctx, 2, page.NextCursor)
	if err != nil {
		t.Fatalf("list batches page2: %v", err)
	}
	if len(page2.Batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(page2.Batches))
	}
	if page2.Batches[0].RunDate != "2026-01-06" {
		t.Fatalf("expected last batch 2026-01-06, got %s", page2.Batches[0].RunDate)
	}
	if page2.NextCursor != nil {
		t.Fatalf("expected no next_cursor")
	}
}

func TestBatchDetailsQuery(t *testing.T) {
	truncateTables(t)

	store := NewStore(testPool)

	batchID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	if err := seedBatch(batchID, "2026-01-27", "SPY", "420.00", "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	pick1ID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	pick2ID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if err := seedPick(pick1ID, batchID, "TSLA", "BUY", "reason", "250.00"); err != nil {
		t.Fatalf("seed pick1: %v", err)
	}
	if err := seedPick(pick2ID, batchID, "NVDA", "BUY", "reason", "900.00"); err != nil {
		t.Fatalf("seed pick2: %v", err)
	}

	checkpoint1ID := "11111111-2222-3333-4444-555555555555"
	checkpoint2ID := "22222222-3333-4444-5555-666666666666"
	if err := seedCheckpoint(checkpoint1ID, batchID, "2026-01-28", "computed", "421.00", "0.0024"); err != nil {
		t.Fatalf("seed checkpoint1: %v", err)
	}
	if err := seedCheckpoint(checkpoint2ID, batchID, "2026-01-29", "computed", "430.00", "0.0238"); err != nil {
		t.Fatalf("seed checkpoint2: %v", err)
	}

	if err := seedMetric("99999999-9999-9999-9999-999999999999", checkpoint1ID, pick1ID, "255.00", "0.0200", "0.0176"); err != nil {
		t.Fatalf("seed metric1: %v", err)
	}
	if err := seedMetric("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", checkpoint1ID, pick2ID, "905.00", "0.0056", "0.0032"); err != nil {
		t.Fatalf("seed metric2: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	detail, err := store.BatchDetails(ctx, batchID)
	if err != nil {
		t.Fatalf("batch details: %v", err)
	}
	if detail == nil {
		t.Fatalf("expected batch details")
	}
	if detail.Batch.ID != batchID {
		t.Fatalf("expected batch %s, got %s", batchID, detail.Batch.ID)
	}
	if len(detail.Picks) != 2 {
		t.Fatalf("expected 2 picks, got %d", len(detail.Picks))
	}
	if len(detail.Checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(detail.Checkpoints))
	}
	if len(detail.Checkpoints[0].Metrics) != 2 {
		t.Fatalf("expected 2 metrics on first checkpoint, got %d", len(detail.Checkpoints[0].Metrics))
	}
}

func truncateTables(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := testPool.Exec(ctx, "TRUNCATE TABLE pick_checkpoint_metrics, checkpoints, picks, batches RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

func seedBatch(id, runDate, benchmarkSymbol, benchmarkPrice, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `
        INSERT INTO batches (id, run_date, benchmark_symbol, benchmark_initial_price, status)
        VALUES ($1, $2, $3, $4, $5)`,
		id,
		runDate,
		benchmarkSymbol,
		benchmarkPrice,
		status,
	)
	return err
}

func seedPick(id, batchID, ticker, action, reasoning, initialPrice string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `
        INSERT INTO picks (id, batch_id, ticker, action, reasoning, initial_price)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		id,
		batchID,
		ticker,
		action,
		reasoning,
		initialPrice,
	)
	return err
}

func seedCheckpoint(id, batchID, checkpointDate, status, benchmarkPrice, benchmarkReturn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `
        INSERT INTO checkpoints (id, batch_id, checkpoint_date, status, benchmark_price, benchmark_return_pct)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		id,
		batchID,
		checkpointDate,
		status,
		benchmarkPrice,
		benchmarkReturn,
	)
	return err
}

func seedMetric(id, checkpointID, pickID, currentPrice, absoluteReturn, vsBenchmark string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `
        INSERT INTO pick_checkpoint_metrics (id, checkpoint_id, pick_id, current_price, absolute_return_pct, vs_benchmark_pct)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		id,
		checkpointID,
		pickID,
		currentPrice,
		absoluteReturn,
		vsBenchmark,
	)
	return err
}

func resetSchema(db *sql.DB) error {
	_, err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	return err
}

func runMigrations(dbURL string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	migrationsPath := filepath.Join(root, "migrations")
	sourceURL := "file://" + filepath.ToSlash(migrationsPath)

	migrator, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return err
	}
	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func failFast(action string, err error) {
	fmt.Fprintf(os.Stderr, "db test setup failed (%s): %v\n", action, err)
	os.Exit(1)
}

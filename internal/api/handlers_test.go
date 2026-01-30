package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/igor-kupczynski/alpha-monday/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"log/slog"
)

var (
	testPool    *pgxpool.Pool
	testStore   *db.Store
	testHandler http.Handler
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
	testStore = db.NewStore(testPool)
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	testHandler = NewRouter(testStore, logger, nil)

	code := m.Run()

	testPool.Close()
	if _, err := lockConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to release advisory lock: %v\n", err)
	}
	_ = lockConn.Close()
	_ = sqlDB.Close()

	os.Exit(code)
}

func TestHealth(t *testing.T) {
	truncateTables(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload map[string]any
	decodeJSON(t, rr.Body, &payload)

	if payload["ok"] != true {
		t.Fatalf("expected ok true, got %v", payload["ok"])
	}
	if payload["db_ok"] != true {
		t.Fatalf("expected db_ok true, got %v", payload["db_ok"])
	}
}

func TestLatestEmpty(t *testing.T) {
	truncateTables(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)

	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Batch            *map[string]any `json:"batch"`
		Picks            []any           `json:"picks"`
		LatestCheckpoint *map[string]any `json:"latest_checkpoint"`
	}
	decodeJSON(t, rr.Body, &payload)

	if payload.Batch != nil {
		t.Fatalf("expected batch null")
	}
	if len(payload.Picks) != 0 {
		t.Fatalf("expected no picks, got %d", len(payload.Picks))
	}
	if payload.LatestCheckpoint != nil {
		t.Fatalf("expected latest_checkpoint null")
	}
}

func TestBatchesEmpty(t *testing.T) {
	truncateTables(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/batches", nil)

	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Batches    []any   `json:"batches"`
		NextCursor *string `json:"next_cursor"`
	}
	decodeJSON(t, rr.Body, &payload)

	if len(payload.Batches) != 0 {
		t.Fatalf("expected empty batches, got %d", len(payload.Batches))
	}
	if payload.NextCursor != nil {
		t.Fatalf("expected next_cursor null")
	}
}

func TestBatchesInvalidParams(t *testing.T) {
	truncateTables(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/batches?limit=0", nil)
	testHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/batches?limit=101", nil)
	testHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/batches?cursor=bad-date", nil)
	testHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestBatchNotFound(t *testing.T) {
	truncateTables(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/batches/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)

	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestBatchInvalidID(t *testing.T) {
	truncateTables(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/batches/not-a-uuid", nil)

	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestLatestAndDetails(t *testing.T) {
	truncateTables(t)

	batchID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if err := seedBatch(batchID, "2026-01-20", "SPY", "410.00", "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	pick1ID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	pick2ID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	if err := seedPick(pick1ID, batchID, "AAPL", "BUY", "reason", "150.00"); err != nil {
		t.Fatalf("seed pick1: %v", err)
	}
	if err := seedPick(pick2ID, batchID, "MSFT", "SELL", "reason", "320.00"); err != nil {
		t.Fatalf("seed pick2: %v", err)
	}

	checkpointID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	if err := seedCheckpoint(checkpointID, batchID, "2026-01-21", "computed", "412.00", "0.0049"); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	if err := seedMetric("ffffffff-ffff-ffff-ffff-ffffffffffff", checkpointID, pick1ID, "151.00", "0.0067", "0.0018"); err != nil {
		t.Fatalf("seed metric1: %v", err)
	}
	if err := seedMetric("11111111-1111-1111-1111-111111111111", checkpointID, pick2ID, "318.00", "-0.0062", "-0.0111"); err != nil {
		t.Fatalf("seed metric2: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload map[string]any
	decodeJSON(t, rr.Body, &payload)
	batch := payload["batch"].(map[string]any)
	if _, ok := batch["benchmark_initial_price"].(string); !ok {
		t.Fatalf("expected benchmark_initial_price string")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/batches", nil)
	testHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/batches/"+batchID, nil)
	testHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var detail map[string]any
	decodeJSON(t, rr.Body, &detail)
	if detail["batch"] == nil {
		t.Fatalf("expected batch in detail")
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

func decodeJSON(t *testing.T, body *bytes.Buffer, target any) {
	t.Helper()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil && err != io.EOF {
		t.Fatalf("decode json: %v", err)
	}
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
	fmt.Fprintf(os.Stderr, "api test setup failed (%s): %v\n", action, err)
	os.Exit(1)
}

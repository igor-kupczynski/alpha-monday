package dbtests

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

var (
	testDB      *sql.DB
	databaseURL string
	lockConn    *sql.Conn
)

const advisoryLockID int64 = 424242

func TestMain(m *testing.M) {
	databaseURL = getenvDefault("DATABASE_URL", "postgres://alpha:alpha@localhost:5432/alpha_monday?sslmode=disable")

	var err error
	testDB, err = sql.Open("postgres", databaseURL)
	if err != nil {
		failFast("open db", err)
	}
	testDB.SetMaxOpenConns(4)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := testDB.PingContext(ctx); err != nil {
		failFast("ping db", err)
	}

	lockConn, err = testDB.Conn(ctx)
	if err != nil {
		failFast("lock connection", err)
	}
	if _, err := lockConn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		failFast("advisory lock", err)
	}

	if err := resetSchema(testDB); err != nil {
		failFast("reset schema", err)
	}
	if err := runMigrations(databaseURL); err != nil {
		failFast("run migrations", err)
	}

	code := m.Run()
	if _, err := lockConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to release advisory lock: %v\n", err)
	}
	_ = lockConn.Close()
	_ = testDB.Close()
	os.Exit(code)
}

func TestMigrationsApplied(t *testing.T) {
	row := testDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1")
	var version int
	var dirty bool
	if err := row.Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if dirty {
		t.Fatalf("schema_migrations is dirty")
	}
	if version != 4 {
		t.Fatalf("expected latest migration version 4, got %d", version)
	}
}

func TestSchemaTables(t *testing.T) {
	expected := []string{"batches", "picks", "checkpoints", "pick_checkpoint_metrics"}
	for _, table := range expected {
		var name sql.NullString
		if err := testDB.QueryRow("SELECT to_regclass($1)", "public."+table).Scan(&name); err != nil {
			t.Fatalf("lookup table %s: %v", table, err)
		}
		if !name.Valid {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	var events sql.NullString
	if err := testDB.QueryRow("SELECT to_regclass('public.events')").Scan(&events); err != nil {
		t.Fatalf("lookup events table: %v", err)
	}
	if events.Valid {
		t.Fatalf("events table should not exist in v1")
	}
}

func TestSchemaColumns(t *testing.T) {
	cases := map[string][]columnSpec{
		"batches": {
			{name: "id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "created_at", udt: "timestamptz", nullable: false, defaultRequired: true},
			{name: "run_date", udt: "date", nullable: false, defaultForbidden: true},
			{name: "benchmark_symbol", udt: "text", nullable: false, defaultRequired: true},
			{name: "benchmark_initial_price", udt: "numeric", nullable: false, defaultForbidden: true},
			{name: "status", udt: "text", nullable: false, defaultForbidden: true},
		},
		"picks": {
			{name: "id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "batch_id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "ticker", udt: "text", nullable: false, defaultForbidden: true},
			{name: "action", udt: "text", nullable: false, defaultForbidden: true},
			{name: "reasoning", udt: "text", nullable: false, defaultForbidden: true},
			{name: "initial_price", udt: "numeric", nullable: false, defaultForbidden: true},
		},
		"checkpoints": {
			{name: "id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "batch_id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "checkpoint_date", udt: "date", nullable: false, defaultForbidden: true},
			{name: "status", udt: "text", nullable: false, defaultForbidden: true},
			{name: "benchmark_price", udt: "numeric", nullable: true, defaultForbidden: true},
			{name: "benchmark_return_pct", udt: "numeric", nullable: true, defaultForbidden: true},
		},
		"pick_checkpoint_metrics": {
			{name: "id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "checkpoint_id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "pick_id", udt: "uuid", nullable: false, defaultForbidden: true},
			{name: "current_price", udt: "numeric", nullable: false, defaultForbidden: true},
			{name: "absolute_return_pct", udt: "numeric", nullable: false, defaultForbidden: true},
			{name: "vs_benchmark_pct", udt: "numeric", nullable: false, defaultForbidden: true},
		},
	}

	for table, expected := range cases {
		cols, err := fetchColumns(table)
		if err != nil {
			t.Fatalf("fetch columns for %s: %v", table, err)
		}
		if len(cols) != len(expected) {
			t.Fatalf("expected %d columns for %s, got %d", len(expected), table, len(cols))
		}
		for _, spec := range expected {
			actual, ok := cols[spec.name]
			if !ok {
				t.Fatalf("missing column %s.%s", table, spec.name)
			}
			if actual.udt != spec.udt {
				t.Fatalf("%s.%s expected type %s, got %s", table, spec.name, spec.udt, actual.udt)
			}
			if actual.nullable != spec.nullable {
				t.Fatalf("%s.%s expected nullable=%v, got %v", table, spec.name, spec.nullable, actual.nullable)
			}
			if spec.defaultRequired && !actual.hasDefault {
				t.Fatalf("%s.%s expected default", table, spec.name)
			}
			if spec.defaultForbidden && actual.hasDefault {
				t.Fatalf("%s.%s should not have default", table, spec.name)
			}
		}
	}

	if err := ensureNoUUIDExtensions(); err != nil {
		t.Fatalf("uuid extension check failed: %v", err)
	}
}

func TestSchemaConstraints(t *testing.T) {
	constraints := []constraintSpec{
		{table: "batches", name: "batches_status_check", contype: "c"},
		{table: "picks", name: "picks_action_check", contype: "c"},
		{table: "checkpoints", name: "checkpoints_status_check", contype: "c"},
		{table: "batches", name: "batches_run_date_unique", contype: "u"},
		{table: "picks", name: "picks_batch_ticker_unique", contype: "u"},
		{table: "checkpoints", name: "checkpoints_batch_date_unique", contype: "u"},
		{table: "pick_checkpoint_metrics", name: "pick_checkpoint_metrics_checkpoint_pick_unique", contype: "u"},
		{table: "picks", name: "picks_batch_fk", contype: "f"},
		{table: "checkpoints", name: "checkpoints_batch_fk", contype: "f"},
		{table: "pick_checkpoint_metrics", name: "pick_checkpoint_metrics_checkpoint_fk", contype: "f"},
		{table: "pick_checkpoint_metrics", name: "pick_checkpoint_metrics_pick_fk", contype: "f"},
	}

	for _, c := range constraints {
		if err := ensureConstraint(c); err != nil {
			t.Fatalf("constraint %s on %s: %v", c.name, c.table, err)
		}
	}
}

func TestIntegrityConstraints(t *testing.T) {
	truncateTables(t)

	t.Run("bad status enum", func(t *testing.T) {
		tx, err := testDB.Begin()
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback()

		_, err = tx.Exec(`INSERT INTO batches (id, run_date, benchmark_symbol, benchmark_initial_price, status)
			VALUES ($1, $2, $3, $4, $5)`,
			"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			mustDate(t, "2026-01-06"),
			"SPY",
			400.00,
			"invalid",
		)
		if err == nil {
			t.Fatalf("expected enum check failure for batches.status")
		}
	})

	t.Run("missing fk", func(t *testing.T) {
		tx, err := testDB.Begin()
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback()

		_, err = tx.Exec(`INSERT INTO picks (id, batch_id, ticker, action, reasoning, initial_price)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			"cccccccc-cccc-cccc-cccc-cccccccccccc",
			"AAPL",
			"BUY",
			"reason",
			150.00,
		)
		if err == nil {
			t.Fatalf("expected fk failure for picks.batch_id")
		}
	})

	t.Run("duplicate run_date", func(t *testing.T) {
		tx, err := testDB.Begin()
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback()

		_, err = tx.Exec(`INSERT INTO batches (id, run_date, benchmark_symbol, benchmark_initial_price, status)
			VALUES ($1, $2, $3, $4, $5)`,
			"dddddddd-dddd-dddd-dddd-dddddddddddd",
			mustDate(t, "2026-01-13"),
			"SPY",
			400.00,
			"active",
		)
		if err != nil {
			t.Fatalf("seed batch: %v", err)
		}

		_, err = tx.Exec(`INSERT INTO batches (id, run_date, benchmark_symbol, benchmark_initial_price, status)
			VALUES ($1, $2, $3, $4, $5)`,
			"eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
			mustDate(t, "2026-01-13"),
			"SPY",
			401.00,
			"active",
		)
		if err == nil {
			t.Fatalf("expected unique violation for batches.run_date")
		}
	})
}

func TestQueriesLatestBatchAndDetails(t *testing.T) {
	truncateTables(t)

	batch1ID := "11111111-1111-1111-1111-111111111111"
	batch2ID := "22222222-2222-2222-2222-222222222222"

	if err := seedBatch(batch1ID, "2026-01-13", "SPY", 400.00, "completed"); err != nil {
		t.Fatalf("seed batch1: %v", err)
	}
	if err := seedBatch(batch2ID, "2026-01-20", "SPY", 410.00, "active"); err != nil {
		t.Fatalf("seed batch2: %v", err)
	}

	pick1ID := "33333333-3333-3333-3333-333333333333"
	pick2ID := "44444444-4444-4444-4444-444444444444"

	if err := seedPick(pick1ID, batch2ID, "AAPL", "BUY", "reason", 150.00); err != nil {
		t.Fatalf("seed pick1: %v", err)
	}
	if err := seedPick(pick2ID, batch2ID, "MSFT", "SELL", "reason", 320.00); err != nil {
		t.Fatalf("seed pick2: %v", err)
	}

	checkpoint1ID := "55555555-5555-5555-5555-555555555555"
	checkpoint2ID := "66666666-6666-6666-6666-666666666666"

	if err := seedCheckpoint(checkpoint1ID, batch2ID, "2026-01-21", "computed", 412.00, 0.0049); err != nil {
		t.Fatalf("seed checkpoint1: %v", err)
	}
	if err := seedCheckpoint(checkpoint2ID, batch2ID, "2026-01-22", "computed", 418.00, 0.0195); err != nil {
		t.Fatalf("seed checkpoint2: %v", err)
	}

	if err := seedMetric("77777777-7777-7777-7777-777777777777", checkpoint1ID, pick1ID, 151.00, 0.0067, 0.0018); err != nil {
		t.Fatalf("seed metric1: %v", err)
	}
	if err := seedMetric("88888888-8888-8888-8888-888888888888", checkpoint1ID, pick2ID, 318.00, -0.0062, -0.0111); err != nil {
		t.Fatalf("seed metric2: %v", err)
	}
	if err := seedMetric("99999999-9999-9999-9999-999999999999", checkpoint2ID, pick1ID, 155.00, 0.0333, 0.0138); err != nil {
		t.Fatalf("seed metric3: %v", err)
	}
	if err := seedMetric("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaab", checkpoint2ID, pick2ID, 310.00, -0.0312, -0.0507); err != nil {
		t.Fatalf("seed metric4: %v", err)
	}

	row := testDB.QueryRow(`SELECT id::text, run_date FROM batches ORDER BY run_date DESC LIMIT 1`)
	var latestID string
	var latestDate time.Time
	if err := row.Scan(&latestID, &latestDate); err != nil {
		t.Fatalf("latest batch query: %v", err)
	}
	if latestID != batch2ID {
		t.Fatalf("expected latest batch %s, got %s", batch2ID, latestID)
	}
	if latestDate.Format("2006-01-02") != "2026-01-20" {
		t.Fatalf("expected latest run_date 2026-01-20, got %s", latestDate.Format("2006-01-02"))
	}

	rows, err := testDB.Query(`
		SELECT p.ticker, c.checkpoint_date
		FROM batches b
		JOIN picks p ON p.batch_id = b.id
		JOIN checkpoints c ON c.batch_id = b.id
		LEFT JOIN pick_checkpoint_metrics m ON m.checkpoint_id = c.id AND m.pick_id = p.id
		WHERE b.id = $1
		ORDER BY p.ticker, c.checkpoint_date`, batch2ID)
	if err != nil {
		t.Fatalf("batch detail query: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var ticker string
		var checkpointDate time.Time
		if err := rows.Scan(&ticker, &checkpointDate); err != nil {
			t.Fatalf("scan detail row: %v", err)
		}
		key := fmt.Sprintf("%s-%s", ticker, checkpointDate.Format("2006-01-02"))
		seen[key] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("detail rows err: %v", err)
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 detail rows, got %d", len(seen))
	}

	expectedKeys := []string{
		"AAPL-2026-01-21",
		"AAPL-2026-01-22",
		"MSFT-2026-01-21",
		"MSFT-2026-01-22",
	}
	for _, key := range expectedKeys {
		if !seen[key] {
			t.Fatalf("missing detail row for %s", key)
		}
	}
}

func TestIndexSanity(t *testing.T) {
	indexes := map[string][]string{
		"batches":                 {"batches_run_date_unique"},
		"picks":                   {"picks_batch_id_idx", "picks_batch_ticker_unique"},
		"checkpoints":             {"checkpoints_batch_id_idx", "checkpoints_batch_date_unique"},
		"pick_checkpoint_metrics": {"pick_checkpoint_metrics_checkpoint_id_idx", "pick_checkpoint_metrics_pick_id_idx", "pick_checkpoint_metrics_checkpoint_pick_unique"},
	}

	for table, expected := range indexes {
		found, err := fetchIndexes(table)
		if err != nil {
			t.Fatalf("fetch indexes for %s: %v", table, err)
		}
		for _, name := range expected {
			if !found[name] {
				t.Fatalf("missing index %s on %s", name, table)
			}
		}
	}

	truncateTables(t)
	if err := seedBatch("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", "2026-01-27", "SPY", 420.00, "active"); err != nil {
		t.Fatalf("seed batch: %v", err)
	}
	if err := seedPick("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb1", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", "TSLA", "BUY", "reason", 250.00); err != nil {
		t.Fatalf("seed pick: %v", err)
	}
	if err := seedCheckpoint("cccccccc-cccc-cccc-cccc-ccccccccccc1", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", "2026-01-28", "computed", 421.00, 0.0024); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	if err := seedMetric("dddddddd-dddd-dddd-dddd-ddddddddddb1", "cccccccc-cccc-cccc-cccc-ccccccccccc1", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb1", 255.00, 0.02, 0.0176); err != nil {
		t.Fatalf("seed metric: %v", err)
	}

	assertExplainUsesIndex(t, `SELECT * FROM batches ORDER BY run_date DESC LIMIT 1`, "batches_run_date_unique")
	assertExplainUsesIndex(t, `SELECT * FROM picks WHERE batch_id = $1`, "picks_batch_id_idx", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1")
	assertExplainUsesIndex(t, `SELECT * FROM checkpoints WHERE batch_id = $1`, "checkpoints_batch_id_idx", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1")
	assertExplainUsesIndex(t, `SELECT * FROM pick_checkpoint_metrics WHERE checkpoint_id = $1`, "pick_checkpoint_metrics_checkpoint_id_idx", "cccccccc-cccc-cccc-cccc-ccccccccccc1")
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

func truncateTables(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(`TRUNCATE TABLE pick_checkpoint_metrics, checkpoints, picks, batches RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

type columnSpec struct {
	name             string
	udt              string
	nullable         bool
	defaultRequired  bool
	defaultForbidden bool
}

type columnInfo struct {
	udt        string
	nullable   bool
	hasDefault bool
}

func fetchColumns(table string) (map[string]columnInfo, error) {
	rows, err := testDB.Query(`
		SELECT column_name, udt_name, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := map[string]columnInfo{}
	for rows.Next() {
		var name, udt, nullableStr string
		var defaultVal sql.NullString
		if err := rows.Scan(&name, &udt, &nullableStr, &defaultVal); err != nil {
			return nil, err
		}
		cols[name] = columnInfo{
			udt:        udt,
			nullable:   nullableStr == "YES",
			hasDefault: defaultVal.Valid,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

func ensureNoUUIDExtensions() error {
	rows, err := testDB.Query("SELECT extname FROM pg_extension WHERE extname IN ('uuid-ossp', 'pgcrypto')")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var ext string
		if err := rows.Scan(&ext); err != nil {
			return err
		}
		return fmt.Errorf("unexpected extension %s installed", ext)
	}
	return rows.Err()
}

type constraintSpec struct {
	table   string
	name    string
	contype string
}

func ensureConstraint(spec constraintSpec) error {
	var exists bool
	row := testDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			WHERE n.nspname = 'public'
			AND t.relname = $1
			AND c.conname = $2
			AND c.contype = $3
		)`, spec.table, spec.name, spec.contype)
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("constraint not found")
	}
	return nil
}

func fetchIndexes(table string) (map[string]bool, error) {
	rows, err := testDB.Query(`
		SELECT indexname
		FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = $1`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

func assertExplainUsesIndex(t *testing.T, query, indexName string, args ...any) {
	t.Helper()
	plan, err := explainQuery(query, args...)
	if err != nil {
		t.Fatalf("explain %s: %v", query, err)
	}
	if !strings.Contains(plan, "Index Scan using "+indexName) &&
		!strings.Contains(plan, "Index Scan Backward using "+indexName) &&
		!strings.Contains(plan, "Index Only Scan using "+indexName) &&
		!strings.Contains(plan, "Index Only Scan Backward using "+indexName) &&
		!strings.Contains(plan, "Bitmap Index Scan on "+indexName) {
		t.Fatalf("expected plan to use index %s, got:\n%s", indexName, plan)
	}
}

func explainQuery(query string, args ...any) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		return "", err
	}

	rows, err := tx.QueryContext(ctx, "EXPLAIN (COSTS OFF) "+query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("parse date %s: %v", value, err)
	}
	return date
}

func seedBatch(id, runDate, benchmarkSymbol string, benchmarkPrice float64, status string) error {
	_, err := testDB.Exec(`
		INSERT INTO batches (id, run_date, benchmark_symbol, benchmark_initial_price, status)
		VALUES ($1, $2, $3, $4, $5)`,
		id,
		mustDateForSeed(runDate),
		benchmarkSymbol,
		benchmarkPrice,
		status,
	)
	return err
}

func seedPick(id, batchID, ticker, action, reasoning string, initialPrice float64) error {
	_, err := testDB.Exec(`
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

func seedCheckpoint(id, batchID, checkpointDate, status string, benchmarkPrice, benchmarkReturn float64) error {
	_, err := testDB.Exec(`
		INSERT INTO checkpoints (id, batch_id, checkpoint_date, status, benchmark_price, benchmark_return_pct)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id,
		batchID,
		mustDateForSeed(checkpointDate),
		status,
		benchmarkPrice,
		benchmarkReturn,
	)
	return err
}

func seedMetric(id, checkpointID, pickID string, currentPrice, absoluteReturn, vsBenchmark float64) error {
	_, err := testDB.Exec(`
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

func mustDateForSeed(value string) time.Time {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(fmt.Sprintf("invalid date %s: %v", value, err))
	}
	return date
}

# Alpha Monday - Implementation Plan

Status: draft
Date: 2026-01-30

Each major section ends in a working feature. References point to the HLD and relevant LLDs.

## 0) Developer Experience
References: HLD `docs/001-high-level-design.md` (Developer Experience), LLD `docs/011-devex-tooling.md`

- [x] Add top-level Makefile with dynamic `help` output.
- [x] Add Makefile targets for `db-up`, `db-down`, `db-reset`, `test`, and `fmt`.
- [x] Add `fmt-check` and `lint` targets (go vet + staticcheck).
- [x] Pin Go toolchain to 1.25.6 in `go.mod`.

## 0.1) CI baseline
References: HLD `docs/001-high-level-design.md` (Developer Experience), LLD `docs/012-ci.md`

- [x] Add GitHub Actions workflow to run `make lint` and `go test ./...` with a Postgres service container.

## 1) Data layer ready
References: HLD `docs/001-high-level-design.md` (Data Model), LLD `docs/002-database-schema.md`

- [x] Add local Postgres harness via docker compose with simple `db-up`, `db-down`, `db-reset` scripts (wrapped by Makefile targets).
- [x] Adopt `golang-migrate` and define migration layout (`migrations/` with up/down).
- [x] Implement migrations for domain tables and indexes (batches, picks, checkpoints, pick_checkpoint_metrics).
- [x] Ensure UUIDs are app-generated (no DB extension, no default UUIDs in schema).
- [x] Skip `events` table for v1.
- [x] Validate minimal read queries needed by the API.

### Local schema + DB tests
- [x] Migration test: run all migrations on a clean DB and fail on any error.
- [x] Schema assertions: verify tables, columns, FKs, uniques, and CHECK constraints via SQL (or pgTAP).
- [x] Integrity tests: insert valid fixtures and assert invalid inserts fail (bad enum, missing FK, duplicate run_date).
- [x] Query tests: seed minimal data and validate latest batch + batch detail queries.
- [x] Index sanity: confirm required indexes exist; use `EXPLAIN` on key reads to ensure index usage.

**Success criteria:** Fresh local DB can be created, migrated, and validated via the test suite; schema includes only domain tables; UUIDs are supplied by the app; latest batch and batch detail queries return expected results.

## 2) Read-only API online
References: HLD `docs/001-high-level-design.md` (API, Components), LLD `docs/003-api-service.md`

### Tests first
- [x] Create API query tests for latest batch, batch list pagination, and batch details.
- [x] Add handler tests for `/health`, `/latest`, `/batches`, `/batches/{id}` (empty state, invalid params, not found).

### API implementation
- [x] Initialize Go module structure (`cmd/api`, `internal/api`, `internal/db`).
- [x] Add config loading for `DATABASE_URL`, `PORT`, `LOG_LEVEL`, `CORS_ALLOW_ORIGINS`.
- [x] Set up HTTP server with chi router, middlewares, and timeouts.
- [x] Implement DB pool (pgxpool) and query layer for latest, list, and detail reads.
- [x] Implement `/health`, `/latest`, `/batches`, `/batches/{id}` handlers.
- [x] Add response/error helpers and JSON serialization for numeric strings.

**Working feature:** API serves batch data (even if empty) from Postgres.

## 3) Worker baseline and workflow wiring
References: HLD `docs/001-high-level-design.md` (Workflows, Components), LLD `docs/004-worker-service.md`, `docs/005-workflows-hatchet.md`

### Tests first
- [x] Add worker config tests for required env vars and defaults.
- [x] Add workflow registration wiring test to assert IDs (`weekly_pick_v1`, `daily_checkpoint_v1`).

### Worker baseline
- [x] Create `cmd/worker` entrypoint with graceful shutdown.
- [x] Add worker bootstrap (config, logger, Hatchet client).
- [x] Register workflows and steps with Hatchet at startup.
- [x] Define workflow state struct and no-op step handlers.

**Goal:** Running the worker registers the weekly workflow in Hatchet and logs registration; steps are stubbed but wired.

**Working feature:** Worker starts and registers the weekly workflow with Hatchet.

## 4) Picks + initial snapshot
References: HLD `docs/001-high-level-design.md` (Weekly Pick Workflow), LLD `docs/006-integrations-openai.md`, `docs/007-integrations-alpha-vantage.md`, `docs/004-worker-service.md`

### Tests first
- [ ] OpenAI parsing/validation tests: invalid JSON, wrong count, dup tickers, bad action -> retries then fail.
- [ ] Price snapshot tests: SPY + picks previous-close map; fail when any previous close missing.
- [ ] DB write tests: transaction inserts batch + picks + initial checkpoint; re-run fails fast by run_date.

### Implementation
- [ ] Add OpenAI client wrapper with strict JSON schema and retry-on-invalid output.
- [ ] Implement pick validation (count=3, unique, BUY|SELL, ticker format; no S&P 500 allowlist).
- [ ] Implement Alpha Vantage snapshot client (SPY first, then picks) using previous close only.
- [ ] Add workflow steps: generate picks -> snapshot prices -> persist batch/picks/initial checkpoint.
- [ ] Persist batch+pick+initial checkpoint in one transaction; fail fast on run_date conflicts.
- [ ] Log pick list and created IDs.

**Goal:** Weekly workflow creates the run_date batch with 3 validated picks and an initial checkpoint containing SPY + pick previous-close prices. If the run_date already exists, the workflow fails fast.

**Working feature:** Weekly run creates a batch with picks and initial prices stored.

## 5) Daily checkpoints and metrics
References: HLD `docs/001-high-level-design.md` (Daily Checkpoint Step, Computation), LLD `docs/005-workflows-hatchet.md`, `docs/008-computation-metrics.md`, `docs/007-integrations-alpha-vantage.md`

- [ ] Implement durable sleep + daily loop for 14 days.
- [ ] Fetch daily prices, handle market-closed cases, compute metrics, persist checkpoints.

**Working feature:** Checkpoints are created daily with metrics or skipped status.

## 6) Reliability and limits
References: HLD `docs/001-high-level-design.md` (Rate Limiting and Backoff, Observability), LLD `docs/005-workflows-hatchet.md`, `docs/004-worker-service.md`

- [ ] Configure Hatchet rate limiting and fan-out concurrency.
- [ ] Add retries with exponential backoff for transient API failures.
- [ ] Log workflow lifecycle and errors (stdout).

**Working feature:** Workflow runs reliably without violating API limits and produces useful logs.

## 7) Deployment slice
References: HLD `docs/001-high-level-design.md` (Deployment), LLD `docs/009-deployment-ops.md`

- [ ] Containerize API and worker.
- [ ] Deploy worker to Scaleway; configure Hatchet cron.
- [ ] Deploy API and connect to Neon.

**Working feature:** System runs end-to-end on hosted infrastructure.

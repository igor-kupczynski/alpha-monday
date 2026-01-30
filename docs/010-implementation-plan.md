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

### Open decisions (resolve before implementation)
- [ ] **Daily checkpoint time**
  - Options:
    - 9am ET (align with weekly run; simple).
      - Pros: matches HLD example; simple scheduling.
      - Cons: often before market open; may reflect previous close.
    - Market close (end-of-day).
      - Pros: matches end-of-day analytics; clearer daily performance.
      - Cons: more timezone/holiday handling; longer waits.
    - Fixed UTC time (e.g., 14:00 UTC).
      - Pros: deterministic across timezones.
      - Cons: less intuitive for US market context.
  - Industry standard: end-of-day close for daily performance metrics.
  - Recommendation: use 9am ET for v1 simplicity and to align with HLD; document that daily snapshots may reflect previous close.
- [ ] **Missing-price handling for checkpoints**
  - Options:
    - Skip entire checkpoint if SPY missing or any pick missing.
      - Pros: simplest; matches LLD.
      - Cons: loses partial data.
    - Allow partial checkpoints (per-pick nulls).
      - Pros: retains partial data.
      - Cons: complicates schema/API and metrics.
    - Carry-forward last close for missing tickers.
      - Pros: continuous series.
      - Cons: hides data gaps; adds rules.
  - Industry standard: skip or carry-forward depending on analytics needs; for minimal systems, skip.
  - Recommendation: skip entire checkpoint when SPY missing or any pick missing (per LLD).

### Tests first
- [ ] Checkpoint loop tests: 14 calendar days, durable sleep scheduling, and batch completion.
- [ ] Market-closed tests: SPY missing -> checkpoint skipped; pick missing -> checkpoint skipped.
- [ ] Metrics tests: absolute_return_pct and vs_benchmark_pct formulas; reject non-finite values.

### Implementation
- [ ] Implement durable sleep + daily loop for 14 calendar days (day 1..14).
- [ ] Fetch daily prices with fan-out + concurrency cap; detect market-closed cases.
- [ ] Compute metrics per LLD and persist checkpoints + pick_checkpoint_metrics.
- [ ] Mark batch status completed after day 14 checkpoint (computed or skipped).

**Working feature:** Checkpoints are created daily with metrics or skipped status.

**Success criteria:**
- For each of 14 calendar days, exactly one checkpoint is written with status computed or skipped.
- Metrics match formulas in `docs/008-computation-metrics.md` and are stored without rounding.
- Missing SPY or any pick results in a skipped checkpoint (no partial metrics).
- Batch status becomes `completed` after day 14 checkpoint.

## 6) Reliability and limits
References: HLD `docs/001-high-level-design.md` (Rate Limiting and Backoff, Observability), LLD `docs/005-workflows-hatchet.md`, `docs/004-worker-service.md`

### Open decisions (resolve before implementation)
- [ ] **Retry policy parameters**
  - Options:
    - 3 attempts with exponential backoff + jitter (short total delay).
      - Pros: quick recovery; minimal runtime.
      - Cons: fewer chances for flaky APIs.
    - 5 attempts with capped exponential backoff + jitter.
      - Pros: higher resilience.
      - Cons: longer workflow duration.
  - Industry standard: exponential backoff with jitter and a cap.
  - Recommendation: 3-5 attempts, cap at ~60s per retry to stay within workflow SLAs.
- [ ] **Rate limit enforcement location**
  - Options:
    - Hatchet rate limiter only.
      - Pros: centralized control.
      - Cons: relies on orchestrator correctness.
    - Hatchet + client-side guard (token bucket).
      - Pros: extra safety and local control.
      - Cons: more code complexity.
  - Industry standard: orchestrator + client guard for external APIs.
  - Recommendation: Hatchet limiter + concurrency caps; add a lightweight client guard only if needed.
- [ ] **Log format**
  - Options:
    - Structured JSON logs.
      - Pros: easier aggregation/search; industry standard.
      - Cons: slightly more setup.
    - Plain text logs.
      - Pros: simplest.
      - Cons: harder to query.
  - Industry standard: structured logs with workflow/step IDs.
  - Recommendation: structured logs if logger already supports it; otherwise key=value text.

### Implementation
- [ ] Configure Hatchet rate limiting (5 req/min) and fan-out concurrency (2-3).
- [ ] Add retries with exponential backoff + jitter for transient API failures.
- [ ] Log workflow lifecycle and errors (stdout) with workflow/step identifiers.

**Working feature:** Workflow runs reliably without violating API limits and produces useful logs.

**Success criteria:**
- No run exceeds Alpha Vantage limits during fan-out (verified in logs).
- Transient API errors are retried and do not fail the entire workflow unless retries exhaust.
- Logs include workflow ID, step name, and error context for failures.

## 7) Deployment slice
References: HLD `docs/001-high-level-design.md` (Deployment), LLD `docs/009-deployment-ops.md`

### Open decisions (resolve before implementation)
- [ ] **API hosting target**
  - Options:
    - Scaleway Serverless Containers (same as worker).
      - Pros: fewer providers; shared tooling.
      - Cons: may require more setup for public HTTP.
    - Managed HTTP platform (Fly.io/Render/Railway).
      - Pros: fast HTTP deploys; simple routing.
      - Cons: adds another provider.
  - Industry standard: deploy API + worker on the same managed container platform when possible.
  - Recommendation: use Scaleway for API if HTTP ingress is straightforward; otherwise choose a managed HTTP platform with minimal ops.
- [ ] **Deployment pipeline**
  - Options:
    - Manual build + push + deploy.
      - Pros: fastest to start.
      - Cons: manual errors; no audit trail.
    - CI build/push with tagged images and manual deploy.
      - Pros: repeatable artifacts; minimal automation.
      - Cons: some CI setup.
    - CI build + auto-deploy on main.
      - Pros: fully automated.
      - Cons: riskier for early-stage changes.
  - Industry standard: CI builds with immutable tags; deploy via manual approval.
  - Recommendation: CI build/push with manual deploy for v1.
- [ ] **Migration execution**
  - Options:
    - One-off migration job (manual or CI) before deploy.
      - Pros: explicit control; standard practice.
      - Cons: extra step.
    - Run migrations on API startup.
      - Pros: simple.
      - Cons: risky in production; harder to roll back.
  - Industry standard: run migrations as a separate job with explicit approval.
  - Recommendation: one-off migration job before deploy.

### Implementation
- [ ] Containerize API and worker.
- [ ] Deploy worker to Scaleway; configure Hatchet cron.
- [ ] Deploy API and connect to Neon.
- [ ] Apply migrations on Neon before first deploy.

**Working feature:** System runs end-to-end on hosted infrastructure.

**Success criteria:**
- API container responds to `/health` in hosted environment.
- Worker container starts, registers workflows, and receives cron triggers.
- Neon DB has schema applied and workflow writes succeed.
- Secrets are stored in provider secret manager or env injection (no local `.env`).

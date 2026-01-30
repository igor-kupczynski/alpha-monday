# Alpha Monday - Implementation Plan

Status: draft
Date: 2026-01-30

Each major section ends in a working feature. References point to the HLD and relevant LLDs.

## 1) Data layer ready
References: HLD `docs/001-high-level-design.md` (Data Model), LLD `docs/002-database-schema.md`

- [ ] Add local Postgres harness via docker compose with simple `db-up`, `db-down`, `db-reset`.
- [ ] Adopt `golang-migrate` and define migration layout (`migrations/` with up/down).
- [ ] Implement migrations for domain tables and indexes (batches, picks, checkpoints, pick_checkpoint_metrics).
- [ ] Ensure UUIDs are app-generated (no DB extension, no default UUIDs in schema).
- [ ] Skip `events` table for v1.
- [ ] Validate minimal read queries needed by the API.

### Local schema + DB tests
- [ ] Migration test: run all migrations on a clean DB and fail on any error.
- [ ] Schema assertions: verify tables, columns, FKs, uniques, and CHECK constraints via SQL (or pgTAP).
- [ ] Integrity tests: insert valid fixtures and assert invalid inserts fail (bad enum, missing FK, duplicate run_date).
- [ ] Query tests: seed minimal data and validate latest batch + batch detail queries.
- [ ] Index sanity: confirm required indexes exist; use `EXPLAIN` on key reads to ensure index usage.

**Success criteria:** Fresh local DB can be created, migrated, and validated via the test suite; schema includes only domain tables; UUIDs are supplied by the app; latest batch and batch detail queries return expected results.

## 2) Read-only API online
References: HLD `docs/001-high-level-design.md` (API, Components), LLD `docs/003-api-service.md`

- [ ] Build API service skeleton and DB read layer.
- [ ] Implement `/health`, `/latest`, `/batches`, `/batches/{id}`.
- [ ] Add basic request validation and error handling.

**Working feature:** API serves batch data (even if empty) from Postgres.

## 3) Worker baseline and workflow wiring
References: HLD `docs/001-high-level-design.md` (Workflows, Components), LLD `docs/004-worker-service.md`, `docs/005-workflows-hatchet.md`

- [ ] Set up worker runtime and Hatchet registration.
- [ ] Implement workflow skeleton and state model.

**Working feature:** Worker starts and registers the weekly workflow with Hatchet.

## 4) Picks + initial snapshot
References: HLD `docs/001-high-level-design.md` (Weekly Pick Workflow), LLD `docs/006-integrations-openai.md`, `docs/007-integrations-alpha-vantage.md`, `docs/004-worker-service.md`

- [ ] Implement OpenAI pick generation and validation.
- [ ] Fetch initial prices (picks + SPY) and persist batch/picks.

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

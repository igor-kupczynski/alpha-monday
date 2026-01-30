# Alpha Monday - Implementation Plan

Status: draft
Date: 2026-01-30

Each major section ends in a working feature. References point to the HLD and relevant LLDs.

## 1) Data layer ready
References: HLD `docs/001-high-level-design.md` (Data Model), LLD `docs/002-database-schema.md`

- [ ] Implement migrations for domain tables and indexes.
- [ ] Validate minimal read queries needed by the API.

**Working feature:** Database schema exists and supports reads for latest batch and batch detail.

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
- [ ] Log workflow lifecycle and errors (stdout) and optional events.

**Working feature:** Workflow runs reliably without violating API limits and produces useful logs.

## 7) Deployment slice
References: HLD `docs/001-high-level-design.md` (Deployment), LLD `docs/009-deployment-ops.md`

- [ ] Containerize API and worker.
- [ ] Deploy worker to Scaleway; configure Hatchet cron.
- [ ] Deploy API and connect to Neon.

**Working feature:** System runs end-to-end on hosted infrastructure.

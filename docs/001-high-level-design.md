# Alpha Monday - High Level Design

Date: 2026-01-30

## Context
Alpha Monday is a minimal, fun project to learn Hatchet workflows. Every Monday at 9am ET, the system asks an LLM for 3 S&P 500 stock picks, snapshots prices, and then checks performance daily for 14 calendar days. The benchmark is SPY.

## Document Structure
This document is the high-level design. Component-specific low-level design docs will follow for deeper detail (progressive disclosure), including dev tooling (LLD `docs/011-devex-tooling.md`) and CI (LLD `docs/012-ci.md`).

## Goals
- Learn Hatchet features: cron, durable sleep, fan-out, rate limiting, retries, workflow state.
- Keep the system minimal and API-only.
- Store results in Postgres (Neon) with a schema optimized for batches, picks, and checkpoints.
- Be easy to reason about and extend later.

## Non-goals (v1)
- User accounts or authentication.
- UI/dashboard.
- Perfect market calendar handling or sophisticated trading logic.
- Complex analytics beyond simple return and benchmark comparison.

## Scope (MVP)
- Weekly pick batch created by Hatchet cron.
- LLM generates 3 tickers (S&P 500) + BUY/SELL + reasoning.
- Persist batches, picks, checkpoints, and metrics in Postgres.
- Daily checkpoint for 14 calendar days using previous trading day close; if previous close missing, record a skip event. Checkpoint dates reflect the trading day of the close (may predate the batch run_date on day 1).
- API exposes latest batch and historical batches.
- Logs only (stdout) for workflow state.

## Architecture
- Hatchet Cloud runs workflows and schedules.
- Scaleway Serverless Containers run the worker.
- Neon Postgres stores domain tables only in v1.
- External APIs:
  - OpenAI: pick generation.
  - Alpha Vantage: price data for tickers and SPY.

## System Context
- Actor: Public API consumer (no auth in v1).
- System: Alpha Monday API + worker.
- External systems:
  - Hatchet Cloud (workflow orchestration)
  - OpenAI (pick generation)
  - Alpha Vantage (market data)
  - Neon Postgres (data storage)

## Components
- API Service
  - Read-only HTTP API.
  - Serves batch and checkpoint data from Postgres.
  - Implementation: Go + chi + pgx (read-only).
- Worker Service
  - Hatchet worker running workflows and steps.
  - Calls OpenAI and Alpha Vantage.
  - Writes batches, picks, checkpoints, and metrics.
  - Implementation: Go + Hatchet SDK.
- Database (Neon Postgres)
  - Source of truth for domain tables.
- Hatchet Cloud
  - Cron trigger and workflow orchestration.

## Workflows (Hatchet)
### 1) Weekly Pick Workflow (cron)
Trigger: Every Monday 9am ET.
Steps:
1. Generate picks (LLM) with constraint: S&P 500.
2. Snapshot initial prices for 3 picks + SPY using previous close only (no intraday).
3. Persist batch creation and initial snapshot data.
4. For day in 1..14:
   - Durable sleep until next day at 9am ET.
   - Run Daily Checkpoint step (fan-out previous-close fetch; checkpoint_date is previous trading day).

Hatchet patterns:
- Cron: weekly kickoff.
- Workflow state: initial prices, benchmark baseline, pick list, batch id.
- Sleep loop: daily schedule within the same workflow using Hatchet durable sleep (Go SDK DurableContext.SleepFor).
- Fan-out: per-ticker price fetch (plus SPY).
- Rate limiting: price fetch step concurrency and per-minute caps.
- Retries: transient API failures with exponential backoff.

### 2) Daily Checkpoint Step (within workflow)
Steps:
1. Fetch previous trading day close for each ticker and SPY (fan-out).
2. If previous close is unavailable, emit checkpoint_skipped event.
3. Compute return metrics and emit checkpoint_computed event.

## Data Model (Domain-first)
Postgres (Neon) with tables optimized for the app’s queries. An events log is deferred in v1.

Core tables:
- batches
  - id (uuid, pk)
  - created_at (timestamptz)
  - run_date (date, the Monday date)
  - benchmark_symbol (text, default "SPY")
  - benchmark_initial_price (numeric)
  - status (text: active, completed, failed)
- picks
  - id (uuid, pk)
  - batch_id (uuid, fk -> batches.id, indexed)
  - ticker (text)
  - action (text: BUY|SELL)
  - reasoning (text)
  - initial_price (numeric)
- checkpoints
  - id (uuid, pk)
  - batch_id (uuid, fk -> batches.id, indexed)
  - checkpoint_date (date)
  - status (text: computed, skipped)
  - benchmark_price (numeric, nullable)
  - benchmark_return_pct (numeric, nullable)
- pick_checkpoint_metrics
  - id (uuid, pk)
  - checkpoint_id (uuid, fk -> checkpoints.id, indexed)
  - pick_id (uuid, fk -> picks.id, indexed)
  - current_price (numeric)
  - absolute_return_pct (numeric)
  - vs_benchmark_pct (numeric)

Rationale:
- Domain tables match the API needs and keep reads simple.
- Derived metrics are stored at checkpoint time to avoid recomputation.

## API (v1)
Minimal, read-only endpoints:
- GET /health
- GET /latest (latest batch summary)
- GET /batches (list batches, newest first)
- GET /batches/{id} (batch details with computed checkpoints)

Suggested response shape:
- Latest/batch endpoints read from domain tables.
- Include pick list, initial prices, checkpoint metrics, and current status.

## Computation
At each checkpoint:
absolute_return_pct = ((current_price - initial_price) / initial_price) * 100
vs_benchmark_pct = absolute_return_pct - benchmark_return_pct

## Rate Limiting and Backoff
- Use Hatchet rate limiting for Alpha Vantage calls (5 req/min, 500/day); no client-side guard in v1.
- Limit concurrency in fan-out steps (e.g., 2-3 at a time).
- Retries with exponential backoff and jitter for transient failures (3 attempts).

## Observability
- All workflow state visible in stdout logs (structured JSON with workflow/step IDs).
- Errors logged in stdout.

## Developer Experience
- Makefile is the canonical entrypoint for local tasks.
- `make help` provides a dynamic list of available targets.
- DB scripts remain the source of truth; the Makefile wraps them.
- Linting uses `go vet` and `staticcheck`, exposed via `make lint`.
- GitHub Actions runs CI with a Postgres service, `make lint`, and `go test ./...`.
- Go toolchain is pinned to 1.25.6 in `go.mod`.

## Deployment
- Worker and API containers on Scaleway Serverless Containers.
- Hatchet Cloud for orchestration.
- Neon Postgres for storage.
- Environment variables for API keys.
- CI builds and tags images; deployments are manual approvals.
- Migrations run as a separate approved job.

## Future Ideas
- Add a simple dashboard UI.
- Add user accounts and personalized picks.
- Add materialized views for faster reads.
- Add alternative benchmarks (QQQ, sector ETFs).

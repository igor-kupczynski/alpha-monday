# Alpha Monday - Low Level Design: Worker Service

Date: 2026-01-30

## Overview
Hatchet worker runs the weekly workflow and daily checkpoints. The worker is the only component that writes to Postgres.

## Service Structure
- Language/runtime: Go (Hatchet SDK), aligned with API service.
- Entry point: `cmd/worker`.
- Modules:
  - worker: Hatchet client, worker bootstrap, workflow registration
  - workflows: Hatchet workflow definitions + state types
  - steps: pick generation, price fetch, compute metrics
  - integrations: OpenAI, Alpha Vantage
  - db: inserts/updates
  - config: env vars, secrets

## Environment Variables
- DATABASE_URL
- OPENAI_API_KEY
- OPENAI_MODEL (default: gpt-4o-mini)
- ALPHA_VANTAGE_API_KEY
- HATCHET_CLIENT_TOKEN
- HATCHET_CLIENT_HOST_PORT (required if not embedded in token)
- HATCHET_WORKER_NAME (default: `alpha-monday-worker`)
- LOG_LEVEL

## DB Write Patterns
- Insert batch first, then picks, then initial checkpoint (all in one transaction).
- Use upsert on checkpoints by (batch_id, checkpoint_date) if retries happen.
- Guard weekly reruns via run_date unique constraint; on conflict, fail fast.
- Initial checkpoint stores benchmark_price and leaves benchmark_return_pct null to represent the baseline snapshot.
- Initial checkpoint_date reflects the trading day of the previous close (can be before run_date).

## Idempotency
- Ensure steps can be retried safely:
  - Batch creation guarded by run_date unique constraint.
  - Checkpoint creation uses unique(batch_id, checkpoint_date).
  - Metrics use unique(checkpoint_id, pick_id).

## Error Handling
- Retry transient API failures (3 attempts, exponential backoff + jitter, base 500ms, max 5s).
- Mark batch failed if unrecoverable errors occur.
- Emit events for failures when events table is enabled.

## Logging
- Structured JSON logs (slog JSON handler).
- Hatchet middleware logs step start/end and failures with workflow_run_id, step_name, step_run_id, retry_count.
- Log key IDs: batch_id, checkpoint_id.

## Rate Limiting
- Configure Hatchet rate limits on worker startup:
  - alpha_vantage_minute: 5 req/min
  - alpha_vantage_day: 500 req/day

## Testing
- Unit tests for computation.
- Wiring tests for workflow registration and step naming.
- Integration tests with mocked OpenAI/Alpha Vantage.

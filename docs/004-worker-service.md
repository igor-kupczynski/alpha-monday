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
- ALPHA_VANTAGE_API_KEY
- HATCHET_CLIENT_ID / HATCHET_CLIENT_SECRET (if needed)
- LOG_LEVEL

## DB Write Patterns
- Insert batch first, then picks, then initial checkpoint.
- Use transactions for batch + pick insertion.
- Use upsert on checkpoints by (batch_id, checkpoint_date) if retries happen.

## Idempotency
- Ensure steps can be retried safely:
  - Batch creation guarded by run_date unique constraint.
  - Checkpoint creation uses unique(batch_id, checkpoint_date).
  - Metrics use unique(checkpoint_id, pick_id).

## Error Handling
- Retry transient API failures.
- Mark batch failed if unrecoverable errors occur.
- Emit events for failures when events table is enabled.

## Logging
- Log workflow start/end, step start/end, and errors.
- Log key IDs: batch_id, checkpoint_id.

## Testing
- Unit tests for computation.
- Wiring tests for workflow registration and step naming.
- Integration tests with mocked OpenAI/Alpha Vantage.

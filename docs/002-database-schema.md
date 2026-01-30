# Alpha Monday - Low Level Design: Database Schema

Date: 2026-01-30

## Overview
Defines the concrete Postgres schema for batches, picks, checkpoints, and metrics. The schema is optimized for simple reads for the API and append-only writes by the worker. The optional events table is deferred in v1.

## Design Principles
- Domain tables are the source of truth.
- Store derived metrics at checkpoint time to avoid recomputation.
- Keep queries simple for API endpoints.
- Use explicit enums via CHECK constraints for portability.
- UUIDs are generated in the application, not the database.

## Tables

### batches
Purpose: Represents a weekly run started on Monday.

Columns:
- id uuid pk
- created_at timestamptz not null default now()
- run_date date not null
- benchmark_symbol text not null default 'SPY'
- benchmark_initial_price numeric not null
- status text not null check (status in ('active','completed','failed'))

Indexes:
- unique(run_date)

Notes:
- run_date should be the Monday date of the batch.

### picks
Purpose: Stores the 3 picks for a batch.

Columns:
- id uuid pk
- batch_id uuid not null references batches(id)
- ticker text not null
- action text not null check (action in ('BUY','SELL'))
- reasoning text not null
- initial_price numeric not null

Indexes:
- index on batch_id
- unique(batch_id, ticker)

### checkpoints
Purpose: Daily snapshot for the batch (computed or skipped).

Columns:
- id uuid pk
- batch_id uuid not null references batches(id)
- checkpoint_date date not null
- status text not null check (status in ('computed','skipped'))
- benchmark_price numeric null
- benchmark_return_pct numeric null

Indexes:
- index on batch_id
- unique(batch_id, checkpoint_date)

Notes:
- checkpoint_date reflects the trading day of the previous close and may predate run_date for the first checkpoint.

### pick_checkpoint_metrics
Purpose: Metrics for each pick per checkpoint.

Columns:
- id uuid pk
- checkpoint_id uuid not null references checkpoints(id)
- pick_id uuid not null references picks(id)
- current_price numeric not null
- absolute_return_pct numeric not null
- vs_benchmark_pct numeric not null

Indexes:
- index on checkpoint_id
- index on pick_id
- unique(checkpoint_id, pick_id)

## Migrations
- Use one migration per table in order: batches, picks, checkpoints, pick_checkpoint_metrics.
- Add indexes in the same migration as table creation.
- Use `golang-migrate` to apply migrations locally and in CI.

## Query Patterns
- Latest batch: select from batches order by run_date desc limit 1.
- Batch details: join batches -> picks -> checkpoints -> pick_checkpoint_metrics by batch_id.
- API list: batches ordered by run_date desc with pagination.

## Data Integrity
- Ensure batch exists before inserting picks and checkpoints.
- Only allow checkpoint inserts for batches with status active (enforced at the app layer).
- Mark batch status completed after day 14 checkpoint computed or skipped.

## Numeric Precision
- Use numeric for prices and returns to avoid floating error.
- Application should round for display; store raw computed numeric values.

## TODOs
- Consider partial index for active batches if needed.

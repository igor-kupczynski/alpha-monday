# Alpha Monday - Low Level Design: API Service

Date: 2026-01-30

## Overview
Defines the read-only HTTP API. The API reads from Postgres domain tables only.

## Service Structure
- Language/runtime: TBD.
- Layers:
  - http: routing, request parsing, response formatting
  - data: query functions
  - config: env vars

## Endpoints

### GET /health
Response:
- 200 { "ok": true }

### GET /latest
Purpose: returns the latest batch summary.
Response includes:
- batch id, run_date, status
- benchmark symbol + initial price
- picks (ticker, action, reasoning, initial_price)
- latest checkpoint (if exists) with metrics

### GET /batches
Purpose: list batches (newest first).
Query params:
- limit (default 20, max 100)
- cursor (optional, opaque or run_date-based)
Response:
- list of batch summaries
- next_cursor (if pagination)

### GET /batches/{id}
Purpose: return full batch details.
Response includes: batch info, picks, all checkpoints, pick metrics per checkpoint.

### GET /events?batch_id=...
Optional debug endpoint. Returns events by batch_id.

## Response Shape (suggested)
- batch:
  - id, run_date, status, benchmark_symbol, benchmark_initial_price
- picks:
  - id, ticker, action, reasoning, initial_price
- checkpoints:
  - id, checkpoint_date, status, benchmark_price, benchmark_return_pct
  - metrics: list of pick metrics

## Error Handling
- 400 for invalid params
- 404 for missing batch id
- 500 for unexpected errors
- No auth in v1

## DB Queries
- Use explicit SELECT lists; avoid SELECT *.
- Read-only connections; no writes.

## Performance
- Simple joins; no heavy aggregation.
- Pagination for /batches.

## Security
- Validate path params as uuid.
- Basic request logging.

## Testing
- Unit tests for query functions.
- Integration tests for endpoints with test DB.

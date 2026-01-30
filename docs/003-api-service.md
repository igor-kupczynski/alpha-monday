# Alpha Monday - Low Level Design: API Service

Date: 2026-01-30

## Overview
Defines the read-only HTTP API. The API reads from Postgres domain tables only.

## Service Structure
- Language/runtime: Go (1.22+).
- Router: go-chi/chi v5 (minimal deps, URL params, middleware).
- DB access: pgx v5 with pgxpool (explicit SQL, no ORM).
- JSON: encoding/json.
- Logging: slog (structured, JSON output).
- Layers:
  - http: routing, request parsing, response formatting
  - data: query functions
  - config: env vars

## HTTP Server
- Port: `PORT` env var (default 8080).
- Timeouts: set read/write/idle timeouts (10s/10s/60s).
- No auth in v1.

## Endpoints

### GET /health
Response:
- 200 { "ok": true }
- Includes `db_ok` boolean; returns 503 if DB ping fails.

### GET /latest
Purpose: returns the latest batch summary.
Response includes:
- batch id, run_date, status
- benchmark symbol + initial price
- picks (ticker, action, reasoning, initial_price)
- latest checkpoint (if exists) with metrics (`latest_checkpoint`)
- Empty state: 200 with `"batch": null` when no batches exist.

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
Optional debug endpoint. Returns events by batch_id. (Deferred in v1.)

## Response Shape (suggested)
- batch:
  - id, run_date, status, benchmark_symbol, benchmark_initial_price
- picks:
  - id, ticker, action, reasoning, initial_price
- checkpoints:
  - id, checkpoint_date, status, benchmark_price, benchmark_return_pct
  - metrics: list of pick metrics
- top-level responses:
  - `/latest`: `{ "batch": <batch|null>, "picks": [...], "latest_checkpoint": <checkpoint|null> }`
  - `/batches`: `{ "batches": [...], "next_cursor": <run_date|null> }`
  - `/batches/{id}`: `{ "batch": <batch>, "picks": [...], "checkpoints": [...] }`

## Serialization
- Numeric values (prices and percentages) are serialized as strings to preserve precision.
- Dates are ISO-8601 (`YYYY-MM-DD`).

## Pagination
- Cursor-based pagination on `run_date` (unique).
- When `cursor` is provided, return batches with `run_date` < cursor.
- `next_cursor` is the last batch's run_date when more results exist.

## Error Handling
- 400 for invalid params
- 404 for missing batch id
- 500 for unexpected errors
- Error format: `{ "error": { "code": "invalid_argument", "message": "..." } }`

## DB Queries
- Use explicit SELECT lists; avoid SELECT *.
- Read-only connections; no writes.
- Prefer multiple focused queries over a single wide join to avoid duplication.

## Performance
- Simple joins; no heavy aggregation.
- Pagination for /batches.

## Security
- Validate path params as uuid.
- Basic request logging.
- CORS disabled by default; allowlist via `CORS_ALLOW_ORIGINS` (comma-separated origins) if needed.

## Testing
- Unit tests for query functions.
- Integration tests for endpoints with test DB.

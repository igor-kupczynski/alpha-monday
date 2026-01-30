# Alpha Monday - Low Level Design: Alpha Vantage Integration

Date: 2026-01-30

## Overview
Fetches stock prices for picks and SPY from Alpha Vantage.

## Endpoints
- Global Quote for current price.

## Request Strategy
- Fetch SPY first to detect market closed.
- Fan-out for pick tickers.

## Rate Limits
- Free tier: 5 requests per minute, 500 per day.
- Enforce with Hatchet rate limiting and step concurrency caps.

## Response Handling
- Parse price from Global Quote.
- Treat missing/empty price as market closed or unavailable.

## Market Closed Logic
- Initial snapshot:
  - Always use previous close for baseline prices (no intraday data).
  - If any previous close price is missing, fail the step to allow retry (no partial baseline).
- Daily checkpoints:
  - If benchmark (SPY) price missing: mark checkpoint as skipped.
  - If SPY present but a pick missing: skip entire checkpoint for simplicity (v1).

## Error Handling
- Retry transient HTTP failures.
- Fail step for invalid responses; rely on Hatchet retries.

## Caching
- No caching in v1.

## TODOs
- Add fallback data source.
- Improve per-ticker missing data handling.

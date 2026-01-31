# Alpha Monday - Low Level Design: Hatchet Workflows

Date: 2026-01-30

## Overview
Defines Hatchet workflows and their state, steps, retries, and rate limiting.

## Workflow: Weekly Pick (cron)
Trigger:
- Cron: Every Monday at 9am ET (`0 9 * * 1` with timezone configured in Hatchet).
Workflow ID:
- `weekly_pick_v1`

Workflow State:
- batch_id
- run_date
- benchmark_symbol
- benchmark_initial_price
- picks: list of { pick_id, ticker, action, reasoning, initial_price }

Steps:
1. generate_picks
   - Call OpenAI with S&P 500 constraint.
   - Validate tickers (format + uniqueness + count = 3).
2. snapshot_initial_prices
   - Fetch price for 3 picks and SPY.
   - Store benchmark_initial_price and pick initial_price.
3. persist_batch
   - Create batch + picks + initial checkpoint in a transaction.
   - Initial checkpoint_date is the trading day of the previous close.
4. daily_loop (for day in 1..14)
   - sleep until next day at 9am ET using Hatchet durable sleep (Go SDK DurableContext.SleepFor).
   - run daily_checkpoint using previous trading day close (checkpoint_date is that trading day and may be before run_date on day 1).
   - sleep uses absolute 9am ET targets; if a run resumes after the target time, it proceeds without sleeping.

## Step: Daily Checkpoint
Inputs:
- batch_id, list of picks, benchmark_symbol, benchmark_initial_price
Step ID:
- `daily_checkpoint_v1`

Steps:
1. fetch_prices_fanout
   - Fetch previous trading day close for each ticker and SPY.
   - Concurrency limit: 2-3.
   - Rate limit: 5 req/min via Hatchet.
2. handle_market_closed
   - If SPY or any pick previous close unavailable, insert checkpoint with status=skipped.
   - If SPY trading day is unavailable (market closed), fallback checkpoint_date to the previous weekday.
3. compute_metrics
   - Compute benchmark_return_pct and pick metrics.
4. persist_checkpoint
   - Insert checkpoint and pick_checkpoint_metrics.

## Retries
- Transient API failures: retry 3 attempts with exponential backoff + jitter (base 500ms, max 5s).
- Non-retry errors: mark batch failed and emit event.

## Rate Limiting
- Configure Hatchet rate limits for Alpha Vantage calls:
  - alpha_vantage_minute: 5 req/min (units=4 per step run).
  - alpha_vantage_day: 500 req/day (units=4 per step run).
- Fan-out concurrency capped at 3.

## Idempotency
- Checkpoint step safe for retries due to unique constraints.
- Batch creation safe due to unique run_date.
 - Duplicate checkpoint_date inserts are treated as already completed and skipped.

## Metrics and Monitoring
- Log step duration.
- Log API response errors with request IDs.

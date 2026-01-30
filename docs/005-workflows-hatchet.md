# Alpha Monday - Low Level Design: Hatchet Workflows

Date: 2026-01-30

## Overview
Defines Hatchet workflows and their state, steps, retries, and rate limiting.

## Workflow: Weekly Pick (cron)
Trigger:
- Cron: Every Monday at 9am ET.

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
   - Create batch + picks in a transaction.
4. daily_loop (for day in 1..14)
   - durable_sleep until next day at 9am ET.
   - run daily_checkpoint.

## Step: Daily Checkpoint
Inputs:
- batch_id, list of picks, benchmark_symbol, benchmark_initial_price

Steps:
1. fetch_prices_fanout
   - Fetch current prices for each ticker and SPY.
   - Concurrency limit: 2-3.
   - Rate limit: 5 req/min via Hatchet.
2. handle_market_closed
   - If SPY or all picks unavailable, insert checkpoint with status=skipped.
3. compute_metrics
   - Compute benchmark_return_pct and pick metrics.
4. persist_checkpoint
   - Insert checkpoint and pick_checkpoint_metrics.

## Retries
- Transient API failures: retry with exponential backoff + jitter.
- Non-retry errors: mark batch failed and emit event.

## Rate Limiting
- Configure Hatchet rate limiter for Alpha Vantage calls: 5 req/min.
- Fan-out concurrency capped at 2-3.

## Idempotency
- Checkpoint step safe for retries due to unique constraints.
- Batch creation safe due to unique run_date.

## Metrics and Monitoring
- Log step duration.
- Log API response errors with request IDs.

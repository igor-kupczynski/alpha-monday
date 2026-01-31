# Alpha Monday - Low Level Design: Computation and Metrics

Date: 2026-01-30

## Overview
Defines computation of daily returns and benchmark comparisons.

## Inputs
- initial_price (per pick)
- current_price (per pick)
- benchmark_initial_price
- benchmark_price

## Formulas
- benchmark_return_pct = ((benchmark_price - benchmark_initial_price) / benchmark_initial_price) * 100
- absolute_return_pct = ((current_price - initial_price) / initial_price) * 100
- vs_benchmark_pct = absolute_return_pct - benchmark_return_pct

## Precision and Rounding
- Store all values as numeric with 8 decimal places (scale=8).
- Round to 2 decimal places in API output (display only).

## Edge Cases
- Missing prices: mark checkpoint as skipped.
- Zero initial price: should never happen; treat as error and fail step.
- Negative prices: invalid; treat as error.

## Validation
- All computed values must be finite.

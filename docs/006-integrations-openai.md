# Alpha Monday - Low Level Design: OpenAI Integration

Date: 2026-01-30

## Overview
Uses OpenAI to generate 3 S&P 500 stock picks with BUY/SELL and reasoning.

## Model Selection
- Model: TBD by implementation constraints.
- Use temperature low-to-moderate for consistency.

## Prompt Design
- System: concise instructions for analyst-style picks.
- User: request exactly 3 unique S&P 500 tickers, each with BUY/SELL and reasoning.
- Output format: strict JSON array for easy parsing.

## Output Schema
Example JSON:
[
  {"ticker":"AAPL","action":"BUY","reasoning":"..."},
  {"ticker":"MSFT","action":"SELL","reasoning":"..."},
  {"ticker":"JNJ","action":"BUY","reasoning":"..."}
]

## Validation
- Ensure exactly 3 entries.
- Unique tickers.
- Ticker format: 1-5 uppercase letters.
- action in BUY|SELL.
- Reasoning non-empty.

## Failure Handling
- If invalid output: retry with a stricter prompt.
- If still invalid: fail workflow and emit event.

## Notes
- Optionally add a validation step against a cached S&P 500 ticker list.

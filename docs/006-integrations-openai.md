# Alpha Monday - Low Level Design: OpenAI Integration

Date: 2026-01-30

## Overview
Uses OpenAI to generate 3 S&P 500 stock picks with BUY/SELL and reasoning.

## Model Selection
- Model: configurable via env var (default `gpt-4o-mini`, a small/fast model suitable for JSON extraction).
- Use low temperature for consistency (e.g., 0.2).

## Environment Variables
- `OPENAI_API_KEY` (required)
- `OPENAI_MODEL` (optional, defaults to `gpt-4o-mini`)

## Prompt Design
- System: concise instructions for analyst-style picks.
- User: request exactly 3 unique S&P 500 tickers, each with BUY/SELL and reasoning.
- Output format: strict JSON array for easy parsing.
  - Enforce via JSON schema / response format when available.

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
- If invalid output: retry with a stricter prompt (max 2 total attempts).
- If still invalid: fail workflow and emit event.

## Notes
- Do not enforce an S&P 500 allowlist in v1; rely on the prompt constraint.

# Golden fixtures

Store sanitized historical inputs and baseline outputs here before switching a production entrypoint.
The initial scaffold intentionally does not contain copied real-user stock data. Add fixtures only after
removing tokens, account data, personal watchlists, and provider identifiers that should not be committed.

Recommended fixture groups:
- `stock/`: code, normalized daily bars, quote, chip/fundamental blocks, prior Python analysis result.
- `market/`: region, market overview, news items, prior Python market-review result.
- `expected/`: Go-derived deterministic indicators, normalized context quality and output contract checks.

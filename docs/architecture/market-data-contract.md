# Market Data Canonical Contract

This is the normalization contract for `services/market-data` and the data consumed by `go-core`. Provider adapters must keep source-native values and units in their raw metadata and normalize once before returning data.

## Canonical units

| Field | Canonical type/meaning | Canonical unit |
|---|---|---|
| `current_price`, `open`, `high`, `low`, `prev_close`, `close` | Price per share | CNY/share |
| `change` | Price change per share | CNY/share |
| `change_percent` / `change_percent` in analysis | Percentage points (`1.25` means 1.25%) | % |
| `volume` | Trade volume | shares |
| `amount` | Turnover | CNY |
| `trade_date` | Exchange trading date, not a UTC-midnight timestamp | `YYYY-MM-DD` |
| `quote_time` / `provider_timestamp` | Provider data time, if present | RFC3339 timestamp with zone |
| `fetched_at` | Received by this service | UTC RFC3339 timestamp |
| `normalized_at` | Canonicalized by this service | UTC RFC3339 timestamp |

## Required provenance

Every normalized quote/bar must carry the actual leaf provider name, source version, fetch and normalization timestamps, raw value/unit metadata where provided, normalized units, adjustment basis, quality status and quality reasons. `daily_stock_analysis_api` is a transport/API name, not an acceptable leaf-provider value. Missing provider timestamps remain null; the system must not substitute `fetched_at`.

## Unit conversion rules

- Provider adapters document their raw `volume` unit. Convert hands to shares exactly once (`1 hand = 100 shares` for ordinary A-share equity trades) only when the provider contract says the raw value is hands.
- Amount is converted to CNY according to each provider's published/observed field contract. Do not convert a CNY value again.
- Keep original number and original unit in `raw_volume`, `raw_volume_unit`, `raw_amount`, and `raw_amount_unit` for audit.
- Never infer an unknown unit from magnitude. Use source contract, explicit configuration or mark the field/record unknown and prevent unsafe downstream use.
- Quote sanity check uses `amount / volume` as an approximate average traded price. Gross mismatch against OHLC is rejected and invokes provider fallback; no arbitrary x100 correction is applied.

## Adjustment rules

Allowed history values: `none`, `forward_adjusted`, `backward_adjusted`, `unknown`. `unknown` is a valid observation state but cannot be a requested API basis. Never combine daily candles from different bases in one indicator series. Real-time quotes are marked `none`; when those coexist with adjusted daily history, the bundle is degraded and both the warning and raw quote are preserved. The analysis context must keep its reference price on the daily-bar basis unless a verified conversion factor exists.

Historical forward/backward adjusted provider responses can be recomputed using corporate actions known at the current request time. This first version does not maintain point-in-time adjustment factors; historical `as_of` bundles therefore carry an explicit warning. Do not use these outputs for claiming leak-free historical backtests. A future point-in-time adjustment-factor/snapshot store is required for that.

## Time and partial bars

- `trade_date` is a date, not a timestamp.
- `quote_time` is supplied by the provider or null. `fetched_at` is local receipt time, not market-event time.
- During live sessions, old provider timestamps become `stale`; timestamps too far in the future become `degraded`. Freshness checks run on cache hits as well as provider responses.
- Daily bars fetched before the local market close for the current trade date are marked partial. Historical `as_of` queries before close exclude that date's eventual completed candle. This implementation estimates session close using the `Asia/Shanghai` clock, not an authoritative exchange calendar; holiday-aware session state remains future work.
- Latest-only chip/fundamental/board snapshots are skipped when `as_of` refers to a historical moment, to prevent current auxiliary data from leaking backward in time. These fields are then explicitly missing.

## Quality states

`valid`, `degraded`, `stale`, `invalid`, `unknown`. Quality is purpose-specific: a quote may still be suitable for display while its unknown volume unit prohibits volume-based analysis. Missing amount stays null through the Go bridge and is represented in legacy analysis maps as null, never as zero.

## Current implementation boundary

Implemented here: Eastmoney and Tencent real-time quotes and daily K-lines; source priority/failover; bounded retries with exponential backoff; per-provider/per-capability circuit breakers; normalized units and validation; JSON HTTP contract; Redis cache; optional MariaDB persistence; Go-core HTTP adapter; Python auxiliary bridge. Not yet implemented: all remaining Python providers; native Go finance, industry/breadth, fund-flow, chip, LHB and news collectors; point-in-time corporate action factors/calendar; real-service integration tests against the deployment's MariaDB/Redis and live endpoints.

# Go Market Data Service

Standalone Go service for A-share real-time quotes and daily K-lines. It is a separate service/module in the same repository as `go-core`; its source, module, environment file, provider configuration, Dockerfile and migrations are isolated under `services/market-data/`.

## Scope of this first slice

- Providers: Eastmoney first, Tencent fallback.
- Endpoints: live/ready health, quote, daily bars, bundle and provider status.
- Standard contract: `volume` is shares; `amount` is CNY; prices are CNY/share; `trade_date` is a trading date; `quote_time` is the provider event time, never the local receipt time.
- History adjustment is explicit: `none`, `forward_adjusted` or `backward_adjusted`. `unknown` is not a supported request basis.
- Provider metadata, raw volume/amount values and units, timestamps, fallback diagnostics and quality reasons are returned so the caller can audit transformations. The service does not guess arbitrary 100x price-scale corrections; a price/amount/volume inconsistency fails provider validation and moves to the next configured provider.
- Freshness checks also apply to Redis cache hits, so a cached intraday quote can transition to `stale` after crossing the configured age threshold.
- Historical `as_of` requests avoid same-day unfinished daily bars and disclose that current adjusted history may include later corporate-action factors. This implementation is not yet a point-in-time adjustment-factor database.
- MariaDB and Redis are optional in local smoke mode and configurable via this service's own `.env`. In the intended deployment, point them to existing infrastructure rather than launching new database services.

This first slice does **not** yet implement financial statements, funds flow, chip distribution, industry breadth, top-list/龙虎榜 or news. During migration, the Python bridge remains responsible for auxiliary fields and existing LLM/persistence/notification capabilities.

## Configuration isolation

Copy `.env.example` to `services/market-data/.env` and replace every `replace-with-*` value with real credentials; generate `MARKET_DATA_API_KEY` with `openssl rand -hex 32`. Template credentials are rejected at startup. Provider ordering and switches belong to `configs/providers.json`; environment variables override runtime settings. The service does not load the repository root `.env` implicitly. Invalid duration, integer and boolean settings fail startup rather than silently falling back to defaults.

`MARKET_DATA_MYSQL_DSN` should normally include `parseTime=true&charset=utf8mb4&loc=UTC`, for example:

```text
appuser:REPLACE_WITH_DB_PASSWORD@tcp(Mariadb:3306)/stock_analysis?parseTime=true&charset=utf8mb4&loc=UTC
```

Replace `Mariadb`, the database/user and password with the actual DNS alias and least-privilege credentials reachable on the attached Docker network. The [Go MySQL driver DSN documentation](https://github.com/go-sql-driver/mysql#dsn-data-source-name) says password escaping is not required; do not URL-encode it as if the DSN were an HTTP URL. Check Compose `.env` parsing when passwords contain `$`. If using the user's existing 1Panel MariaDB/Redis containers, attach this service to the existing `1panel-network`; do not create duplicate production databases.

`MARKET_DATA_REDIS_ADDR` is an address such as `Redis:6379`, not a URL. Provide Redis ACL username when applicable and a password through this service's `.env`.

## HTTP API

All data endpoints require `X-API-Key: <MARKET_DATA_API_KEY>` or `Authorization: Bearer <key>`. `/health/live` is unauthenticated; `/health/ready` checks configured Redis/MariaDB connections.

- `GET /health/live`
- `GET /health/ready`
- `GET /api/v1/quotes/{symbol}`
- `GET /api/v1/bars/{symbol}/daily?start=YYYY-MM-DD&end=YYYY-MM-DD&limit=120&adjustment=forward_adjusted`
- `GET /api/v1/bundles/{symbol}?lookback_days=120&adjustment=forward_adjusted&as_of=RFC3339`
- `GET /api/v1/data-sources`

Symbols accept `600519`, `600519.SH`, `000001.SZ` and `430047.BJ`. Bare symbols are normalized to an exchange suffix. Tencent does not support Beijing Stock Exchange fallback in this slice.

## Build and test

From this directory:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build ./cmd/market-data
```

The module depends on `github.com/go-sql-driver/mysql`. An environment without network access may fail the first dependency download; run `go mod download` where Go module access is available, then repeat the commands. Do not use a fake MySQL driver to validate actual database behavior.

## Smoke tests

After starting, test `GET /health/live`; then use the configured key for `/api/v1/quotes/600519.SH` and `/api/v1/bars/600519.SH/daily?limit=30&adjustment=forward_adjusted`. Check: price, source, provider timestamp, `volume_unit=shares`, `amount_unit=CNY`, adjustment basis and quality warnings. A live provider smoke test is still required in the deployment environment; unit tests use deterministic mocked responses and do not prove current provider availability or current provider field semantics.


## Resilience controls

`MARKET_DATA_PROVIDER_RETRIES` controls additional attempts after a transient provider error (0–5; default 1). Retries use bounded exponential backoff starting at 100 ms and honor request cancellation. `MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD` sets consecutive failed provider calls before opening a circuit (default 3), and `MARKET_DATA_CIRCUIT_COOLDOWN` controls cooldown (Go duration, minimum 1 s; default 45 s). Circuits are isolated per provider and capability (`quote` versus `daily_bars`), permit one half-open probe after cooldown, and do not count database/cache write errors as provider failures. Invalid provider payloads and stale/unusable quotes count as failures.


Provider resilience: default provider timeout is 12s, retries default to 1, and the overall request timeout defaults to 110s so the sequential daily-bars and quote bundles can accommodate the configured primary/fallback attempts and backoff budget. Configuration validation rejects an overall timeout too short for the configured provider count, retry count, backoff budget and provider timeout. When tuning retries or per-provider timeouts, increase both the data-service request timeout and the Go-core client `MARKET_DATA_REQUEST_TIMEOUT` (default 115s) together; the client timeout must exceed the data service request timeout plus its HTTP write-timeout margin.

### Quote snapshot retention

`MARKET_DATA_QUOTE_RETENTION` defaults to `30d` and must be at least `24h`. The market-data process purges `market_data_quotes` older than this period in batches of at most 5,000 rows, once at startup and then every 24 hours. This policy applies only to real-time quote snapshots; historical daily bars are retained. If a database migration/cleanup fails, an explicit error is logged. Do not use this table as a permanent tick archive without changing the retention policy and storage design.

### Provider capability interfaces

Provider registration uses separate `NamedProvider`, `QuoteProvider`, and `DailyBarProvider` interfaces. The aggregate `Provider` interface remains only as a compatibility helper for the currently implemented Eastmoney and Tencent adapters. This does not mean other providers have already been ported; see the repository-level `PROVIDER_MIGRATION_MATRIX.md`.

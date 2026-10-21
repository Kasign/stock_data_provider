# Go analysis core (migration slice 1)

This is a separate Go module. All Go source stays below `go-core/`; no Go files are placed under the existing Python `src/`, `api/`, or `data_provider/` folders.

## Included in this slice

- Authenticated internal HTTP endpoints for a single-stock analysis and market review.
- Go-owned orchestration: fetch data, calculate deterministic technical indicators, build a context/quality summary, call analysis, persist, and optionally notify.
- Python capability adapter isolated in `internal/adapters/pythonbridge`.
- No direct ORM or pandas imports in Go. Python owns the current storage schema and provider SDKs.
- Stage trace with status and duration for each step.

## Run tests and build

```bash
cd go-core
gofmt -w ./cmd ./internal
go test ./...
go build ./cmd/analysisd
```

## Required environment

- `GO_CORE_TOKEN`: required, non-placeholder secret of at least 32 characters for `/internal/v1/analysis/*` endpoints (generate with `openssl rand -hex 32`).
- `PYTHON_BRIDGE_TOKEN`: required, non-placeholder secret of at least 32 characters for calls to Python bridge.
- `PYTHON_BRIDGE_BASE_URL`: defaults to `http://python-bridge:8090`.
- `HTTP_ADDR`: defaults to `127.0.0.1:8088`; Docker Compose explicitly overrides it to `:8088` for private container-network access.
- `GO_CORE_REQUEST_TIMEOUT` and `PYTHON_BRIDGE_TIMEOUT`: Go duration syntax, for example `20m`.

## Endpoints

- `GET /healthz` — liveness only.
- `POST /internal/v1/analysis/stock` — one stock analysis.
- `POST /internal/v1/analysis/market-review` — one regional market review.

The service must remain on the private container network or loopback-bound host port. Do not publish it directly to the public Internet.

## Important rollout boundary

This source slice does **not** automatically replace `main.py`, FastAPI routes, Bot or the existing Scheduler. Existing entrypoints must be routed through `python_bridge/go_core_client.py` only after compatibility and golden tests pass. This prevents both old and new orchestration paths from running the same request during initial validation.

The first version reuses the Python `GeminiAnalyzer` for output generation, the current provider layer for raw data, the old persistence schema and notification implementation. Some analysis-context parity work remains before replacing the production `StockAnalysisPipeline` path. See the migration plan for rollout and rollback acceptance criteria.

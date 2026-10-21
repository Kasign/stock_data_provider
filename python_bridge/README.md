# Python capability bridge

This package is a separate Python integration surface, not a second analysis workflow. It calls existing repository modules on behalf of Go and converts their values into the versioned JSON contract at `docs/architecture/python-bridge.openapi.yaml`.

## Run from repository root

```bash
export PYTHON_BRIDGE_TOKEN="$(openssl rand -hex 32)"
uvicorn python_bridge.capability_app:app --host 127.0.0.1 --port 8090
```

For Docker, use the opt-in `docker/docker-compose.go-migration.yml` overlay with the existing compose file. The bridge does not publish a host port.

## Endpoints

The liveness route `/healthz` has no authentication. All `/internal/v1/*` capability routes require `X-Python-Bridge-Token`. The bridge fails closed if the token is missing, shorter than 32 characters, still a template placeholder, or incorrect. Generate a separate random token for each service boundary; place the same bridge token in root `.env` and `go-core/.env`.

The Go-side Python HTTP client lives only in `go-core/internal/adapters/pythonbridge/`. Existing Python callers of Go must use `go_core_client.py`, never build URLs independently. `GO_CORE_ENABLED` defaults to `false` so merely adding these files cannot unexpectedly change production routing.

## Compatibility note

The bridge wraps existing interfaces such as `DataFetcherManager`, `GeminiAnalyzer`, `MarketAnalyzer`, `AnalysisRepository`, and `NotificationService`. Existing upstream package signatures can evolve; bridge contract tests and live smoke tests must run against the pinned repository commit before production cutover.

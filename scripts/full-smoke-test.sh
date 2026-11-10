#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8092}"
SYMBOLS="${SYMBOLS:-600519}"

command -v curl >/dev/null 2>&1 || { echo "ERROR: curl is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "ERROR: python3 is required" >&2; exit 1; }

printf '\n== 1/3 Go process health ==\n'
curl --fail-with-body --silent --show-error "$API_BASE_URL/healthz"
printf '\n\n== 2/3 Dependencies and schema readiness ==\n'
curl --fail-with-body --silent --show-error "$API_BASE_URL/readyz"
printf '\n\n== 3/3 End-to-end analysis API ==\n'
python3 - "$API_BASE_URL" "$SYMBOLS" <<'PY'
import json
import sys
import urllib.error
import urllib.request

base = sys.argv[1].rstrip("/")
symbols = [s.strip() for s in sys.argv[2].split(",") if s.strip()]
if not symbols:
    raise SystemExit("ERROR: SYMBOLS must contain at least one six-digit code")


def request(path, method="GET", payload=None):
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    headers = {"Accept": "application/json"}
    if data is not None:
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=240) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", errors="replace")
        try:
            body = json.loads(raw)
        except json.JSONDecodeError:
            body = {"raw": raw}
        print(json.dumps(body, ensure_ascii=False, indent=2), file=sys.stderr)
        raise SystemExit(f"ERROR: {method} {path} returned HTTP {e.code}")
    except urllib.error.URLError as e:
        raise SystemExit(f"ERROR: cannot reach {base + path}: {e}")

status, result = request("/api/v1/analysis/runs", "POST", {"symbols": symbols})
run_id = result.get("run_id")
if not run_id:
    print(json.dumps(result, ensure_ascii=False, indent=2))
    raise SystemExit("ERROR: analysis response has no run_id")

_, persisted = request("/api/v1/analysis/runs/" + run_id)
if persisted.get("run_id") != run_id:
    raise SystemExit("ERROR: persisted result run_id does not match submitted run")
_, event_payload = request("/api/v1/analysis/runs/" + run_id + "/events")
events = event_payload.get("events")
if not isinstance(events, list) or not events:
    print(json.dumps(event_payload, ensure_ascii=False, indent=2))
    raise SystemExit("ERROR: expected persisted audit events for this run")

quality = result.get("quality") or {}
print(json.dumps({
    "http_status": status,
    "run_id": run_id,
    "status": result.get("status"),
    "symbols": symbols,
    "quality_core_pass": quality.get("core_pass"),
    "quality_partial": quality.get("partial"),
    "signal_count": len(result.get("signals") or []),
    "event_count": len(events),
    "raw_snapshot_id": result.get("raw_snapshot_id"),
    "market_snapshot_id": result.get("market_snapshot_id"),
    "result_query": base + "/api/v1/analysis/runs/" + run_id,
    "events_query": base + "/api/v1/analysis/runs/" + run_id + "/events",
}, ensure_ascii=False, indent=2))
print("\nPASS: API request, result persistence/readback and audit-event readback succeeded.")
print("NOTE: PASS confirms the engineering/data path, not strategy profitability or trade readiness.")
PY

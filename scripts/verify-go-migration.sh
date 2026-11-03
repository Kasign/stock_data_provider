#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

printf '\n== Go module path references ==\n'
if grep -R -nE 'github\.com/ZhuLinsen/daily_stock_analysis/(go-core|services/market-data)' --include='*.go' --include='go.mod' "$ROOT"; then
  echo "发现旧 Go 模块路径引用，请先完成迁移。"
  exit 1
fi
grep -Fxq 'module github.com/Kasign/stock_data_provider/go-core' "$ROOT/go-core/go.mod"
grep -Fxq 'module github.com/Kasign/stock_data_provider/services/market-data' "$ROOT/services/market-data/go.mod"
export PYTHONPATH="$ROOT${PYTHONPATH:+:$PYTHONPATH}"

printf '\n== go-core: test/vet/build ==\n'
(cd "$ROOT/go-core" && go test ./... && go vet ./... && go build ./cmd/analysisd)

printf '\n== market-data: test/vet/build ==\n'
(cd "$ROOT/services/market-data" && go test ./... && go vet ./... && go build ./cmd/market-data)

printf '\n== Python bridge syntax/tests ==\n'
python3 -m compileall -q "$ROOT/python_bridge"
python3 -m pytest -q "$ROOT/python_bridge/tests"

printf '\nVerification passed. Live provider, MariaDB/Redis and Compose-network tests are separate deployment acceptance steps.\n'

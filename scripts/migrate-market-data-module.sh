#!/usr/bin/env bash
set -euo pipefail

# 仅执行一次：将 Market Data 源码从嵌套 Module 迁入根 Module。
if [[ ! -d services/market-data/internal ]]; then
  echo "Market Data 源码已迁移，跳过。"
  exit 0
fi
if [[ -e internal/marketdata ]]; then
  echo "::error::目标目录 internal/marketdata 已存在，拒绝覆盖。"
  exit 1
fi

mkdir -p internal cmd configs/marketdata
git mv services/market-data/internal internal/marketdata
git mv services/market-data/cmd/market-data cmd/market-data
if [[ -f services/market-data/configs/providers.json ]]; then
  git mv services/market-data/configs/providers.json configs/marketdata/providers.json
fi
git rm services/market-data/go.mod services/market-data/go.sum

python3 - <<'PY'
from pathlib import Path

old = "github.com/Kasign/stock_data_provider/services/market-data/internal/"
new = "example.com/stock-analysis-flow/internal/marketdata/"
for root in (Path("internal"), Path("cmd")):
    for path in root.rglob("*.go"):
        raw = path.read_text(encoding="utf-8")
        updated = raw.replace(old, new)
        updated = updated.replace('"configs/providers.json"', '"configs/marketdata/providers.json"')
        if updated != raw:
            path.write_text(updated, encoding="utf-8")

# Data Dockerfile 改为仓库根目录作为 build context；保留独立 Data 镜像回滚能力。
Path("services/market-data/Dockerfile").write_text("""FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn|https://goproxy.io|direct
ENV GOPROXY=${GOPROXY}
RUN go mod download
COPY . .
RUN test -z "$(gofmt -l ./internal/marketdata ./cmd/market-data)" \\
    && go test ./internal/marketdata/... ./cmd/market-data/... \\
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/market-data ./cmd/market-data

FROM alpine:3.21
RUN addgroup -S marketdata && adduser -S -G marketdata marketdata && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/market-data /usr/local/bin/market-data
COPY --from=build /src/configs/marketdata /app/configs/marketdata
USER marketdata
ENV MARKET_DATA_HTTP_ADDR=:8091 LOG_LEVEL=info TZ=Asia/Shanghai
EXPOSE 8091
HEALTHCHECK --interval=20s --timeout=5s --retries=3 --start-period=15s CMD wget -q -O - http://127.0.0.1:8091/health/live >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/market-data"]
""", encoding="utf-8")

# 更新现有 Go CI：Data 独立测试不再依赖全仓测试；MariaDB 测试与 Docker 构建切换到根 Module。
workflow = Path(".github/workflows/go-test.yml")
if workflow.exists():
    text = workflow.read_text(encoding="utf-8")
    text = text.replace("          - services/market-data\n", "")
    text = text.replace(
        "          grep -Fxq 'module github.com/Kasign/stock_data_provider/services/market-data' services/market-data/go.mod",
        "          test ! -f services/market-data/go.mod"
    )
    text = text.replace("cache-dependency-path: services/market-data/go.sum", "cache-dependency-path: go.sum")
    text = text.replace("working-directory: services/market-data", "working-directory: .")
    text = text.replace("./internal/repository -run", "./internal/marketdata/repository -run")
    text = text.replace("./internal/service -run", "./internal/marketdata/service -run")
    text = text.replace("./internal/httpapi -run", "./internal/marketdata/httpapi -run")
    text = text.replace(
        "docker build --tag stock-analysis-data:ci --file services/market-data/Dockerfile services/market-data",
        "docker build --tag stock-analysis-data:ci --file services/market-data/Dockerfile ."
    )
    if "  market-data-module-test:" not in text:
        anchor = "  market-data-mariadb-integration:\n"
        job = """  market-data-module-test:
    name: Test Market Data in root module
    runs-on: ubuntu-latest
    steps:
      - name: Check out repository
        uses: actions/checkout@v4
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23.x'
          cache: true
      - name: Check Market Data gofmt
        run: |
          files="$(gofmt -l ./internal/marketdata ./cmd/market-data)"
          if [[ -n "$files" ]]; then
            printf '%s\\n' "$files"
            exit 1
          fi
      - name: Run Market Data tests
        run: go test ./internal/marketdata/... ./cmd/market-data/...
      - name: Run Market Data race detector
        run: go test -race ./internal/marketdata/... ./cmd/market-data/...
      - name: Run Market Data vet
        run: go vet ./internal/marketdata/... ./cmd/market-data/...

"""
        text = text.replace(anchor, job + anchor)
    workflow.write_text(text, encoding="utf-8")

# Compose 保留双容器部署作为迁移期回滚方式，但 Data 镜像从根 Module 构建。
compose = Path("docker-compose.yml")
if compose.exists():
    text = compose.read_text(encoding="utf-8")
    text = text.replace("context: ./services/market-data\n      dockerfile: Dockerfile",
                        "context: .\n      dockerfile: services/market-data/Dockerfile")
    compose.write_text(text, encoding="utf-8")

# 迁移进度文档记录实际动作；不覆盖历史盘点。
progress = Path("doc/api/migration/MarketData模块化单体迁移计划与进度.md")
if progress.exists():
    with progress.open("a", encoding="utf-8") as f:
        f.write("""
\n\n### M1-2：Data 源码归并（自动迁移批次）\n
- Data 生产代码、测试与 testdata 从 `services/market-data/internal/**` 迁入 `internal/marketdata/**`，保留现有 package 边界，暂不做领域包重命名。
- 独立命令从 `services/market-data/cmd/market-data` 迁入 `cmd/market-data`。
- Data 内部 import 统一切换为根 Module 路径；嵌套 `services/market-data/go.mod` 与 `go.sum` 删除，依赖由根 Module 管理。
- Provider 配置迁入 `configs/marketdata/providers.json`，默认路径同步调整。
- Data 独立 Dockerfile 改为根目录 build context；Compose 保留双容器回滚部署，Data 镜像由根 Module 构建。
- CI 增加独立 Market Data 测试门禁，MariaDB/PIT 测试与 Data 镜像构建改用根 Module 路径。
- 本批只做 Module/路径归并，不抽取 Bootstrap、不切换 Root InProcess Adapter，也不改变 Provider/领域逻辑。
- 待 CI 验证：Data gofmt、单测、race、vet、MariaDB/PIT、Docker；全仓失败按文件归属单独处理。
""")
PY

gofmt -w ./internal/marketdata ./cmd/market-data
git add -A

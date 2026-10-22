#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_FILE="docker-compose.go.yml"
ANALYSIS_CONTAINER="stock-analysis-go"
MARKET_DATA_CONTAINER="stock-market-data"

fail() {
  printf '[部署失败] %s\n' "$*" >&2
  exit 1
}

if ! command -v docker >/dev/null 2>&1; then
  fail "未找到 docker 命令，请先确认 Docker 已安装并运行。"
fi
if ! docker compose version >/dev/null 2>&1; then
  fail "当前 Docker 未提供 Compose V2；请使用 docker compose 命令。"
fi
if ! command -v curl >/dev/null 2>&1; then
  fail "未找到 curl 命令；请先安装 curl，以便检查 Go API 健康状态。"
fi
if [[ ! -f .env ]]; then
  fail "根目录缺少 .env。仅第一次配置时执行 cp .env.example .env，然后填写真实密钥、数据库和 Redis 配置；已有 .env 不要覆盖。"
fi
if ! docker network inspect 1panel-network >/dev/null 2>&1; then
  fail "Docker 网络 1panel-network 不存在。请先在 1Panel 创建/确认该网络，并确保现有 MariaDB/Redis 已接入。"
fi

printf '[1/4] 检查 Docker Compose 配置……\n'
# --quiet prevents the expanded configuration, including credentials, from being printed.
docker compose -f "$COMPOSE_FILE" config --quiet

printf '[2/4] 构建 Go 行情服务和分析 API 镜像……\n'
docker compose -f "$COMPOSE_FILE" build market-data stock-analysis-go

printf '[3/4] 启动服务……\n'
docker compose -f "$COMPOSE_FILE" up -d

container_state() {
  docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$1" 2>/dev/null || printf 'missing'
}

print_logs() {
  printf '\n===== market-data 最近日志 =====\n'
  docker compose -f "$COMPOSE_FILE" logs --tail=80 market-data || true
  printf '\n===== stock-analysis-go 最近日志 =====\n'
  docker compose -f "$COMPOSE_FILE" logs --tail=80 stock-analysis-go || true
}

printf '[4/4] 等待两个容器通过健康检查（最长 180 秒）……\n'
healthy=0
for ((attempt = 1; attempt <= 90; attempt++)); do
  market_state="$(container_state "$MARKET_DATA_CONTAINER")"
  analysis_state="$(container_state "$ANALYSIS_CONTAINER")"

  if [[ "$market_state" == "healthy" && "$analysis_state" == "healthy" ]]; then
    healthy=1
    break
  fi

  if [[ "$market_state" == "unhealthy" || "$market_state" == "exited" || "$market_state" == "dead" ||
        "$analysis_state" == "unhealthy" || "$analysis_state" == "exited" || "$analysis_state" == "dead" ]]; then
    printf '[状态] market-data=%s，stock-analysis-go=%s\n' "$market_state" "$analysis_state" >&2
    print_logs
    fail "容器未能进入健康状态。请根据上方日志检查密钥、数据库名称/账号权限、Redis 密码及网络服务名。"
  fi

  sleep 2
done

if [[ "$healthy" -ne 1 ]]; then
  printf '[状态] market-data=%s，stock-analysis-go=%s\n' "$(container_state "$MARKET_DATA_CONTAINER")" "$(container_state "$ANALYSIS_CONTAINER")" >&2
  print_logs
  fail "等待健康检查超时。"
fi

if ! curl -fsS http://127.0.0.1:8092/healthz >/dev/null; then
  print_logs
  fail "容器健康检查通过，但宿主机无法访问 127.0.0.1:8092/healthz。"
fi
if ! curl -fsS http://127.0.0.1:8092/readyz >/dev/null; then
  print_logs
  fail "API 已启动，但 /readyz 未通过；请检查 MariaDB 表结构和 Redis 连接。"
fi

printf '\n部署启动成功。容器状态：\n'
docker compose -f "$COMPOSE_FILE" ps
printf '\n健康检查：\n'
curl -fsS http://127.0.0.1:8092/healthz
curl -fsS http://127.0.0.1:8092/readyz
printf '\n后续可查看日志：docker compose -f %s logs -f market-data stock-analysis-go\n' "$COMPOSE_FILE"

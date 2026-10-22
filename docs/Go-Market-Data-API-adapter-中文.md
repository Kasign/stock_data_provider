# Go 原生行情 API 适配器（中文）

## 1. 主流程当前接法

根目录 cmd/analysis-api 已通过 internal/adapters/marketdata 访问原生 Go market-data 服务。个股实时行情和日线 K 线不再调用旧 Python API，也没有静默回退到 Python 的逻辑。

调用关系：

~~~text
cmd/analysis-api
  -> internal/service.NewRunnerWithMarketDataStores
     -> internal/adapters/marketdata.Collector
        -> internal/adapters/marketdata.Client
           -> GET /api/v1/quotes/{symbol}
           -> GET /api/v1/bars/{symbol}/daily
              -> services/market-data (Go)
                 -> Eastmoney / Tencent / Sina quote-only
                 -> MariaDB / Redis cache & persistence
~~~

Sina 目前仅是实时行情兜底源；日 K 由现有 Go 的东方财富和腾讯 Provider 提供。此适配层只消费 Go 服务的统一响应，不直接依赖其内部 Go package，因而不会跨越独立 Go module 的 internal 导入边界。

## 2. 配置

根目录 .env 同时供分析 API 与 market-data 服务使用。需要使用同一个随机 API key 保护两个服务：

~~~dotenv
MARKET_DATA_BASE_URL=http://127.0.0.1:8091
MARKET_DATA_API_KEY=这里替换为随机生成且不少于32字符的密钥
MARKET_DATA_REQUEST_TIMEOUT=135s
MARKET_DATA_HISTORY_DAYS=200
MARKET_DATA_MAX_CONCURRENCY=4
MARKET_DATA_MAX_RESPONSE_BYTES=16777216
MARKET_DATA_MARKET_TIMEZONE=Asia/Shanghai
MARKET_DATA_ADJUSTMENT=forward_adjusted
MARKET_DATA_INDEX_SYMBOLS=000001.SH

# 连接已有 MariaDB / Redis，不需要另建数据库容器
MARKET_DATA_DB_HOST=Mariadb
MARKET_DATA_DB_PORT=3306
MARKET_DATA_DB_USER=stock_analysis_server
MARKET_DATA_DB_PASSWORD=替换为真实数据库密码
MARKET_DATA_DB_NAME=stock_analysis_server
MARKET_DATA_DB_CHARSET=utf8mb4
MARKET_DATA_DB_TIMEZONE=UTC
MARKET_DATA_REDIS_ADDR=Redis:6379
MARKET_DATA_REDIS_PASSWORD=真实 Redis 密码
MARKET_DATA_REDIS_DB=0
MARKET_DATA_DB_AUTO_MIGRATE=true
~~~

实际部署时按服务分别配置 `MARKET_DATA_DB_*`、`MARKET_DATA_REDIS_*` 与分析 API 的 `DB_*`、`REDIS_*`。服务会在代码中生成 DSN，不需要手工拼接；不要提交真实密钥或密码。

| 配置项 | 说明 |
|---|---|
| MARKET_DATA_BASE_URL | 宿主机运行 Go 分析 API 时通常是 http://127.0.0.1:8091；Compose 会覆盖为 http://market-data:8091 |
| MARKET_DATA_API_KEY | 分析 API 的 X-API-Key 与行情服务的 MARKET_DATA_API_KEY 必须一致；启动时拒绝短密钥与占位值 |
| MARKET_DATA_REQUEST_TIMEOUT | 单个 HTTP 请求超时；应覆盖市场数据 API 的最大请求预算 |
| MARKET_DATA_HISTORY_DAYS | 每个证券查询的日 K 数，范围 1–365 |
| MARKET_DATA_MAX_CONCURRENCY | 并发证券查询数，范围 1–32 |
| MARKET_DATA_ADJUSTMENT | none、forward_adjusted 或 backward_adjusted |

## 3. HTTP 契约

客户端请求以下接口，均通过 X-API-Key 鉴权：

| 方法 | 路径 | 关键字段 |
|---|---|---|
| GET | /api/v1/quotes/{symbol} | 行情、来源、时间、质量、成交量/额单位 |
| GET | /api/v1/bars/{symbol}/daily?limit=N&adjustment=... | 日 K、逐条来源、复权口径、抓取时间、单位、质量警告 |

客户端将裸 6 位 A 股代码转成显式交易所代码，如 600519 → 600519.SH、000001 → 000001.SZ；指数应在配置里明确写 000001.SH。支持 sh000001 / sz399001 这种旧式前缀表达法，但不推断美股等海外交易代码。

客户端限制响应体大小、拒绝 HTTP 重定向、校验价格/成交量数值并避免输出响应体中的潜在敏感数据。接口认证失败、超时或数据格式无效时直接记为采集失败，不回退到 Python。

## 4. 数据质量与时点限制

- 原始成交量以“股”归一化；成交额以人民币金额单位读取。没有的数据保留为空，不填零。
- 每根 K 线保留来源、来源版本、抓取时间、复权口径和供应商质量标记。若 Provider 没给出来源或复权口径，则明确标记未知。
- 行情使用上游 `quote_time`；该值是供应商报告的快照/更新时间，不保证等于最近成交时间，并通过 `quote_time_semantics=provider_snapshot_time` 显式标注。`fetched_at` 是服务采集时间。缺失/无法解析的时间、晚于请求分析时点的行情会被丢弃。
- 日 K 的同日数据在中国市场本地时区 15:05 之前不作为已完成日 K 使用；未来日期和 OHLC 不合法的记录会被过滤。
- 市场数据服务尚未提供严格定义的 publication available_at，因此采集器会在来源诊断里明确标记该字段缺失，质量状态为 partial；历史回放继续 fail-closed。
- 一条日 K 的供应商时间不等于它当时已对市场参与者可用的时间。不能将 fetched_at 冒充 available_at，也不能只因字段已经是 forward_adjusted 就宣称获得了严格的无前视偏差历史数据。

## 5. 本地测试

在仓库根目录执行：

~~~bash
go test ./...
go test -race ./...
go vet ./...
~~~

当前 CI 还分别检查根 module、go-core、services/market-data 三个 Go module。单元测试和 CI 成功只验证代码契约与控制流程，不代表生产网络、供应商限流、MariaDB/Redis 权限或目标 Docker 主机的联调已验收。

## 6. Docker Compose 部署

从仓库根目录执行：

~~~bash
cp .env.example .env
# 修改 .env：设置随机 MARKET_DATA_API_KEY，并填写真实 DB/Redis 连接
docker compose -f docker-compose.go.yml up -d --build
docker compose -f docker-compose.go.yml ps
docker compose -f docker-compose.go.yml logs -f market-data stock-analysis-go
~~~

docker-compose.go.yml 会启动 market-data 与 stock-analysis-go 两个 Go 服务，数据服务健康后才启动分析 API。两个服务在已有 1panel-network 上通信；只将分析 API 的 8092 端口暴露到宿主机，market-data 仅向容器网络暴露 8091。分析 API 和 market-data 共用已有 MariaDB / Redis 容器。

若数据库或 Redis 容器名不是 `Mariadb` / `Redis`，修改 `MARKET_DATA_DB_HOST`、`MARKET_DATA_REDIS_ADDR` 和分析 API 的 `DB_HOST`、`REDIS_ADDR`，使各服务连接到现有容器。

## 7. 仍未迁移的数据域

这次切换仅将根目录分析主 API 的实时行情/日 K 采集指向 Go market-data API。原 Python Fetcher 全量迁移仍未完成；全市场指数/广度、板块/概念、涨停池、筹码、财务、资金流、龙虎榜、新闻公告等并未全部具备 Go 原生 Provider。另一个 go-core 分析服务仍通过 python_bridge 承载新闻、LLM 和其他辅助能力。详见 docs/architecture/PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md。

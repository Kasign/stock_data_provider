# Go 行情数据服务

本服务负责 A 股实时行情、日线 K 线和独立的补充数据能力。它与 `go-core` 位于同一 Git 仓库，但拥有单独的 Go module、源代码、配置、Provider 注册、Dockerfile 和数据库迁移。

## 当前实现范围

- 实时行情 Provider：东方财富、腾讯、新浪（新浪仅提供实时行情能力）。
- 日线 K 线 Provider：东方财富、腾讯；新增 BaoStock 原生 Go TCP Provider，默认关闭，完成真实上游验收后才可启用。
- 可选补充 Provider：妙想 MX_API，提供筹码分布和个股主力资金流，默认关闭；不参与行情与 K 线兜底。
- HTTP 接口：存活/就绪检查、实时行情、日线、行情与 K 线组合、数据源能力列表，以及可选补充数据接口。
- 标准单位：`volume` 为股，`amount` 为人民币元，价格为元/股；`trade_date` 是交易日。`quote_time` 是供应商报告的行情快照/更新时间，不一定等于最近一笔成交时间；新增的 `quote_time_semantics=provider_snapshot_time` 明确该语义。`fetched_at` 是本服务采集时间，`normalized_at` 是标准化时间，三者不能互相替代。
- 复权参数只能是 `none`、`forward_adjusted` 或 `backward_adjusted`。`unknown` 只用于说明数据未知，不是可请求的复权类型。
- 标准化行情与 K 线保留底层 Provider 名称、版本、原始单位、抓取/标准化时间、复权口径、兜底诊断和质量原因。不能依据数值大小猜测单位，不能随意执行 100 倍价格修正。
- Redis 缓存命中同样要检查新鲜度；盘中旧行情可能降级为 `stale`。供应商的快照/更新时间与 `fetched_at` 的差值应结合交易时段解释，不能仅凭异步任务开始或结束时间替换供应商时间。
- 历史 `as_of` 查询会排除当日尚未完成的日 K，并明确提示复权历史可能包含之后发生的公司行动因子。本服务尚未建立严格的时点复权因子库，因此不能声称历史回测完全无前视偏差。
- 本地 smoke 模式可不连接数据库；生产部署应复用现有 MariaDB/Redis，不要为了本服务额外启动一套数据库容器。

本阶段尚未完整迁移财务报表、指数/全市场广度、板块排名、龙虎榜、新闻公告等数据域。另一个 `go-core` 服务仍通过 Python bridge 使用部分市场概况、新闻、LLM、持久化和通知能力。

## BaoStock 日 K 数据源（新增，默认关闭）

BaoStock 通过 TCP 长连接访问 public-api.baostock.com:10030。当前代码实现登录、CRC 校验、96 类型 zlib 压缩响应、分页和日 K 字段标准化；Provider 只提供历史日 K，不提供实时行情。Provider 默认关闭，模拟协议测试不等于真实上游登录和查询已通过。

启用前需要从 stock-market-data 容器真实验证 600519.SH 与 688981.SH 的前复权（flag 2）、后复权（flag 1）和不复权（flag 3），检查价格、成交量（股）、成交额（人民币元）、停牌和日期覆盖。BaoStock 的涨跌幅复权算法可能与其他来源不同，因此返回字段 adjustment_method 会标记 baostock_pct_change；同名 forward_adjusted 不代表不同来源的绝对价格一定等价。当前未验收的北交所代码会被明确拒绝。

启用时需要同时设置 MARKET_DATA_BAOSTOCK_ENABLED=true、MARKET_DATA_BAOSTOCK_DAILY_BARS_ENABLED=true，并确保 MARKET_DATA_PROVIDER_PRIORITY 中包含 baostock；保持 MARKET_DATA_BAOSTOCK_QUOTE_ENABLED=false。不得将未复权数据伪装为前复权，也不得跨来源拼接成混合 K 线。

## 腾讯日 K 数据限制

腾讯日 K 通过财经前端接口 `web.ifzq.gtimg.cn/appstock/app/fqkline/get` 获取。当前未找到腾讯官方为该前端接口提供的、可依赖的公开字段契约；代码只把实测稳定的前六列按 `[日期, 开盘, 收盘, 最高, 最低, 成交量]` 解析。额外列没有可核验的公开定义，不能推断为成交额。

因此，使用腾讯作为日 K 兜底时，`amount` 保持 `null`、单根 K 线的 `amount_unit` 为 `unknown`，并标记数据质量降级。服务也会在顶层 `warnings` 报告使用了兜底源或成交额缺失。禁止用 `收盘价 × 成交量` 或 OHLC 平均价估算值伪装成真实成交额；成交额敏感的量价分析必须先检查数据质量，必要时等待具备可信成交额的来源恢复。

腾讯公开前端接口不是稳定的官方开发者 API 合约。成交量的原始单位转换仍需持续通过其他数据源交叉核验，不能将当前观察到的字段布局视为永久保证。

## 配置隔离

本目录的 `.env.example` 是配置参考模板，但该 Go 进程不会自动读取任何 `.env` 文件。使用 `docker-compose.go.yml` 时，Compose 会把仓库根目录 `.env` 注入两个 Go 容器，因此要在根目录 `.env` 配置 `MARKET_DATA_API_KEY`、`MARKET_DATA_DB_*`、`MARKET_DATA_REDIS_*` 和可选的 `MARKET_DATA_MX_*`。本地单独运行服务时，可将本模板复制为 `.env`，再显式执行 `set -a; source .env; set +a` 后启动进程；也可通过 shell/服务管理器直接注入环境变量。可使用 `openssl rand -hex 32` 生成 `MARKET_DATA_API_KEY`。服务启动时拒绝占位密钥、无效时长、整数和布尔值，不会静默吞掉配置错误。

Provider 优先级和基本能力配置在 `configs/providers.json` 中；环境变量可覆盖开关。根目录的 `MARKET_DATA_ADJUSTMENT` 同时作为分析 API 请求和行情服务的默认复权口径；只有确实需要两个服务不同的默认口径时，才额外设置 `MARKET_DATA_DEFAULT_ADJUSTMENT` 覆盖行情服务。独立运行本服务时请在 `services/market-data/` 工作目录加载本目录 `.env`；在 Compose 部署时不要误以为此文件会自动参与配置。

MariaDB 配置使用拆分字段，不需要手工拼 DSN：

```dotenv
MARKET_DATA_DB_HOST=Mariadb
MARKET_DATA_DB_PORT=3306
MARKET_DATA_DB_USER=appuser
MARKET_DATA_DB_PASSWORD=替换为真实数据库密码
MARKET_DATA_DB_NAME=stock_analysis
MARKET_DATA_DB_CHARSET=utf8mb4
MARKET_DATA_DB_TIMEZONE=UTC
```

服务启动时会在 Go 代码中生成 MySQL driver DSN。`MARKET_DATA_MYSQL_DSN` 只作为旧配置的临时兼容入口；新部署应使用上述 `MARKET_DATA_DB_*` 字段，避免把用户名、密码、主机、数据库名和连接参数重复编码在一个字符串中。请按最小权限原则创建数据库账号，不要将真实凭据提交到 Git。

Redis 地址应是 `Redis:6379` 这样的主机和端口，而不是 URL。若使用 Redis ACL，配置用户名并将密码放在服务自己的环境变量中。部署到 1Panel 时应连接现有 `1panel-network`，不应再创建重复的 MariaDB/Redis 服务。

## HTTP API

健康检查外的所有接口都要求 `X-API-Key: <MARKET_DATA_API_KEY>` 或 `Authorization: Bearer <key>`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/health/live` | 存活检查，不需要鉴权 |
| GET | `/health/ready` | 检查已配置的 Redis/MariaDB |
| GET | `/api/v1/quotes/{symbol}` | 实时行情 |
| GET | `/api/v1/bars/{symbol}/daily?start=YYYY-MM-DD&end=YYYY-MM-DD&limit=120&adjustment=forward_adjusted` | 日线 K 线 |
| GET | `/api/v1/bundles/{symbol}?lookback_days=120&adjustment=forward_adjusted&as_of=RFC3339` | 行情与 K 线组合 |
| GET | `/api/v1/data-sources` | 已注册 Provider 与能力 |
| GET | `/api/v1/stocks/{symbol}/chip-distribution` | 可选筹码分布；MX 未启用时返回 Provider 不可用 |
| GET | `/api/v1/stocks/{symbol}/capital-flow` | 可选个股资金流；MX 未启用时返回 Provider 不可用 |

证券代码支持 `600519`、`600519.SH`、`000001.SZ`、`430047.BJ` 等形式；无后缀代码会规范化为带交易所后缀的代码。当前腾讯 Provider 不提供北交所兜底。

## MX_API 配置（可选）

```dotenv
MARKET_DATA_MX_ENABLED=false
MARKET_DATA_MX_API_KEY=
MARKET_DATA_MX_API_URL=https://mkapi2.dfcfs.com/finskillshub/api/claw/query
MARKET_DATA_MX_API_TIMEOUT=10s
```

只有明确设置 `MARKET_DATA_MX_ENABLED=true` 才会注册该 Provider；开启后必须提供有效的 `MARKET_DATA_MX_API_KEY`，也兼容原项目的 `MX_APIKEY`。MX 是自然语言查询型补充接口，不支持实时行情或日线主链路。筹码与资金流结果缺少精确的发布时间/可用时间，质量状态会保持降级；不得把这些当前快照回灌到历史时点分析中。

## 构建与测试

在本目录执行：

```bash
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/market-data
```

此 module 依赖 `github.com/go-sql-driver/mysql`。第一次构建可能需要下载依赖；网络可用时运行 `go mod download`。不要用假 MySQL driver 来证明真实数据库行为可用。

## 冒烟测试

服务启动后，先请求 `GET /health/live`，再用已配置的 API key 测试：

```bash
curl -H "X-API-Key: $MARKET_DATA_API_KEY" http://127.0.0.1:8091/api/v1/quotes/600519.SH
curl -H "X-API-Key: $MARKET_DATA_API_KEY" "http://127.0.0.1:8091/api/v1/bars/600519.SH/daily?limit=30&adjustment=forward_adjusted"
```

检查价格、来源、供应商时间、`volume_unit=shares`、`amount_unit=CNY`、复权口径和质量警告。未指定起始日期时，东方财富适配器会按请求条数计算一个有限的历史回溯区间（每条 K 线 4 个自然日并额外增加 30 天缓冲），避免扫描全历史时把远古区间外的异常复权价格混入本次质量警告；显式传入的起始/结束日期仍优先遵守。真实供应商 smoke test 仍必须在部署环境执行；单元测试使用可控的 mock 响应，并不能证明当前网络可用，也不能替代对供应商真实字段口径的核验。

## 容错策略

- `MARKET_DATA_PROVIDER_RETRIES` 控制临时失败后的额外重试次数，范围 0–5，默认 1；重试使用有界指数退避，并遵守请求取消。
- `MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD` 设置连续失败多少次后打开熔断，默认 3；`MARKET_DATA_CIRCUIT_COOLDOWN` 设置冷却时间，默认 45 秒，最短 1 秒。
- 熔断状态按 Provider 与能力隔离（quote 与 daily bars 分开）；冷却后允许半开探测。数据库/缓存写入错误不应误判为供应商故障。
- 默认 Provider 超时为 12 秒，重试默认 1 次。整体请求超时必须覆盖配置的 Provider 数量、重试和串行组合请求预算；调整任一参数时，要同时检查服务端与调用方的超时配置。
- `MARKET_DATA_QUOTE_RETENTION` 默认 30 天且不得低于 24 小时。服务启动时及每 24 小时分批清理过期实时行情快照，每批最多 5000 行。该策略只清理实时行情快照，不清理历史 K 线；这不是永久逐笔行情归档方案。
- Provider 使用 `NamedProvider`、`QuoteProvider`、`DailyBarProvider`、`SupplementProvider` 等按能力拆分的接口。某数据源只支持一个能力时，不得把它注册成其他能力可用，也不要用单一全局 fallback 顺序混用不同数据域。
- 日志采用结构化字段，保留请求 ID、服务名、接口、状态码和耗时；禁止记录 API Key、Authorization 头、完整自然语言查询或原始响应正文。


### BaoStock 真实上游验收（启用前）

默认保持 MARKET_DATA_BAOSTOCK_ENABLED=false。通过 1Panel 修改环境变量并重新部署后，使用当前日 K API 分别请求 600519.SH 前复权和 688981.SH 不复权。验收时检查 source=baostock、adjustment_method、成交量/成交额单位、quality_status 与 warnings；再对两只股票的三种复权口径做实际对账。通过真实容器登录、查询和数据口径检查后才决定是否保持启用。

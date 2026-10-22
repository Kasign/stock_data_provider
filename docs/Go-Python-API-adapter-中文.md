# Go 接入 daily_stock_analysis Python API（中文）

> **迁移状态（2026-10-21）：本文描述的是保留的旧 Python 适配器，不是主 API 当前的数据链路。** 根目录 cmd/analysis-api 已切换到原生 Go market-data API，参见 docs/Go-Market-Data-API-adapter-中文.md。此文档仅供维护 cmd/pythonapi-smoke、旧适配代码与迁移对照使用；不要按照本文把 PYTHON_API_* 重新配置为主服务依赖。

## 1. 目标与状态

本模块将现有 `ZhuLinsen/daily_stock_analysis` Python 服务作为第一阶段数据提供方。Go 侧不直接调用 AkShare、Tushare 等第三方源，而是复用 Python 项目已经维护的数据源优先级、fallback、缓存和质量诊断能力。

代码位置：

- `internal/adapters/pythonapi/client.go`：HTTP 客户端、JSON 解析、超时、认证头与响应大小限制。
- `internal/adapters/pythonapi/collector.go`：实现 `app.DataCollector`，把行情/日 K 转换为 Go 的 `domain.RawSnapshot`。
- `internal/adapters/pythonapi/config.go`：读取 `.env`/进程环境变量配置。
- `internal/adapters/pythonapi/types.go`：Python API 响应类型。
- `internal/platform/config/dotenv.go`：无第三方依赖的简化 `.env` 加载器；不会覆盖容器或系统已设置的环境变量。
- `cmd/pythonapi-smoke`：真实 API 连通性与字段契约检查命令，不运行策略。

目前该适配器已经实现并通过本地 `httptest` 测试，但还没有把它组装成完整生产应用：实际因子、策略、标准化、质量门禁、组合持久化等仍需分别实现。`cmd/demo` 仍使用假数据，仅用于演示主流程。

## 2. 配置 `.env`

项目根目录提供 `.env` 和 `.env.example`。部署时按环境修改 `.env`，不要把真实 Cookie/Token 提交到 Git。

```dotenv
PYTHON_API_BASE_URL=http://127.0.0.1:8000
PYTHON_API_TIMEOUT=20s
PYTHON_API_HISTORY_DAYS=200
PYTHON_API_MAX_CONCURRENCY=4
PYTHON_API_MAX_RESPONSE_BYTES=8388608
PYTHON_API_MARKET_TIMEZONE=Asia/Shanghai
PYTHON_API_BEARER_TOKEN=
PYTHON_API_COOKIE=
PYTHON_API_INDEX_SYMBOLS=sh000001
```

| 配置项 | 默认值 | 说明 |
|---|---|---|
| `PYTHON_API_BASE_URL` | `http://127.0.0.1:8000` | Python FastAPI 服务根地址，不附加 `/api/v1`；支持部署在反向代理子路径下，例如 `https://example.com/dsa`。 |
| `PYTHON_API_TIMEOUT` | `20s` | 单次 HTTP 请求超时，使用 Go duration 格式。 |
| `PYTHON_API_HISTORY_DAYS` | `200` | 每个证券拉取的日 K 数；受 Python 接口限制，范围为 1–365。 |
| `PYTHON_API_MAX_CONCURRENCY` | `4` | 并发查询证券数量，范围为 1–32；免费源不稳定时建议降低。 |
| `PYTHON_API_MAX_RESPONSE_BYTES` | `8388608` | 单响应最大字节数，范围 1 KiB–128 MiB。 |
| `PYTHON_API_MARKET_TIMEZONE` | `Asia/Shanghai` | Python 的 `update_time` 若没有时区信息，则按该 IANA 时区解析。该设置用于解析无时区 quote 时间和计算交易日截止日期，不修改源数据。 |
| `PYTHON_API_BEARER_TOKEN` | 空 | 仅当 API 服务或反向代理明确支持 Bearer 认证时使用。不要假设它等价于 Python 项目的管理员 Session。 |
| `PYTHON_API_COOKIE` | 空 | 需要管理员 Session Cookie 时，填写请求应发送的完整 Cookie 字符串。会话过期后需更新。 |
| `PYTHON_API_INDEX_SYMBOLS` | `sh000001` | 需要市场指数日线时查询的逗号分隔代码；必须是当前 Python 项目已登记且 API 接受的指数代码。设为空表示不请求额外配置指数。 |

### 地址配置注意事项

- Go 程序与 Python 程序运行在同一台宿主机且 Go 在宿主机运行时，可使用 `http://127.0.0.1:8000`。
- 若 Go 在 Docker 容器内运行，`127.0.0.1` 指 Go 容器自己。应改为同一 Docker 网络中 Python 服务的服务名和容器端口，例如 `http://daily-stock-analysis:8000`；具体服务名以你的 `docker-compose.yml` 为准。
- 若 Python 经反向代理子路径提供服务，配置根路径即可，例如 `https://nas.example.com/dsa`；不要把 `/api/v1/stocks` 放到 Base URL 中。
- 环境变量优先于 `.env`。这便于 Docker、systemd 或面板部署覆盖本地默认值。

## 3. 已接入的 Python API

| API | Go 方法 | 用途 |
|---|---|---|
| `GET /api/v1/stocks/{stock_code}/quote` | `Client.GetQuote` | 实时行情。验证 `current_price` 有效，并保留源返回的更新时间。 |
| `GET /api/v1/stocks/{stock_code}/history?period=daily&days=N` | `Client.GetHistory` / `Collector.Collect` | 日 K 线；验证 OHLC、过滤晚于分析时点日期的 K 线、识别重复日期与缺失值。 |
| `GET /api/v1/stocks/{stock_code}/profile?history_days=N` | `Client.GetProfile` | 聚合个股档案，用于诊断和联调；可选块使用原始 JSON 保持向后兼容。 |
| `GET /api/v1/data/overview` | `Client.GetDataOverview` | 只读数据源能力/质量诊断，不会主动触发行情拉取。 |

调用示例：

```go
cfg, err := pythonapi.ConfigFromEnv()
if err != nil {
    return err
}
client, err := pythonapi.NewClient(cfg)
if err != nil {
    return err
}
collector, err := pythonapi.NewCollector(client, cfg)
if err != nil {
    return err
}

// 将 collector 注入 app.Dependencies.Data；主流程本身不依赖 HTTP 细节。
deps.Data = collector
```

应用启动时先调用 `config.LoadDotEnvIfExists(".env")`，再读取 `pythonapi.ConfigFromEnv()`。环境变量已由 Docker/systemd 注入时，无需 `.env` 文件。

## 4. 原始数据和质量语义

`Collector` 实现 `app.DataCollector`，输出 `domain.RawSnapshot`：

- 股票日 K 写入 `RawSnapshot.Bars`，指数日 K 写入 `RawSnapshot.IndexBars`。
- 实时行情写入 `RawSnapshot.Quotes`。
- 每个证券/数据集都会产生 `SourceNote`，包含接口来源标识、采集时间、状态、缺失字段和诊断信息。某一只股票失败不会直接中止整批请求；门禁根据策略所需字段判断可否继续。
- 并发数量受 `PYTHON_API_MAX_CONCURRENCY` 限制。
- 如果行情更新时间晚于 `RunRequest.AsOf`，或缺少/无法解析更新时间，适配器不会把该实时行情放入快照。
- 历史 K 线只保留按 `PYTHON_API_MARKET_TIMEZONE` 换算后不晚于 `AsOf` 交易日期的记录，并校验 OHLC 逻辑；重复日期和异常行会被跳过并记录在来源诊断中。
- 没有成交量/成交额时，Go 领域类型以 `nil` 保留缺失，而不是写成零；`MissingFields` 也会列出对应字段。

`SourceNote.Source=daily_stock_analysis_api` 只代表数据是从本仓库的 Python API 取回，并不代表其底层实际用了 AkShare、Tushare 或某个其他 Provider。Python 现有的 quote/history 响应并没有对每一行统一暴露底层 Provider、精确 `available_at` 或复权口径；因此适配器显式标记：

- `provider_source` 缺失：无法从旧端点准确知道该证券的这条数据实际由哪个 Provider 返回。
- `available_at` 缺失：`fetched_at` 是 Go 取得 HTTP 响应的时间，不能冒充该数据对市场参与者可用的时间。
- `adjustment_basis=unknown`：旧历史接口没有声明原始、前复权或后复权等价格口径。

这些缺失信息会保留在 `SourceNote` 和数据质量状态中，不能被主流程当成已确认事实。若某策略或回测要求精确复权、历史点时数据、逐行来源审计，应先扩展 Python API 响应契约或接入专门的历史数据接口；不能仅凭当前端点宣称回测无前视偏差。

## 5. 本地测试

在 Go 模块根目录执行：

```bash
go test ./...
go vet ./...
go test -race ./...
```

真实接口冒烟测试：

```bash
# 使用项目根目录 .env，默认检查 600519 的实时行情和最近 200 根日 K
go run ./cmd/pythonapi-smoke

# 指定股票代码和历史根数
go run ./cmd/pythonapi-smoke --stock 600519 --history-days 200

# 额外调用聚合档案和数据源诊断端点
go run ./cmd/pythonapi-smoke --stock 600519 --profile --overview

# 使用其他环境配置文件
go run ./cmd/pythonapi-smoke --env-file /path/to/test.env --stock 600519
```

命令成功时输出 JSON 摘要；任一所选端点失败会在 JSON 的 `errors` 中说明并以非零状态码退出。不要将响应中的当前价格/报告当成策略验收结果。该命令会发起真实 HTTP 请求，需要 Python 服务正在运行、网络可达，并且认证有效。

如先用 curl 验证 Python 服务：

```bash
curl -fsS "$PYTHON_API_BASE_URL/api/v1/stocks/600519/quote"
curl -fsS "$PYTHON_API_BASE_URL/api/v1/stocks/600519/history?period=daily&days=200"
curl -fsS "$PYTHON_API_BASE_URL/api/v1/data/overview"
```

## 6. 当前边界与下一步

本次完成的是 Go 到现有 Python 服务的网络接入边界，不等于完整分析系统已投入使用。后续建议：

1. 在真实服务上运行 `pythonapi-smoke`，把输出的接口错误和字段质量作为联调依据。
2. 实现/复核 `Normalizer`，把 `RawSnapshot.Quotes/Bars/IndexBars` 映射成 `MarketSnapshot`，保留所有来源注记和缺失值。
3. 实现质量门禁，根据核心字段与策略所需数据判断失败、部分可用或放行。
4. 对复权、公司行动、证券状态、交易日历和行情可用时间补齐接口契约。
5. 再实现确定性因子、少量策略、风控/仓位与模拟成交。

不要将 `cmd/demo` 当成真实联调；它仍明确使用模拟数据。真实 Python API 接入的测试入口是 `cmd/pythonapi-smoke`。

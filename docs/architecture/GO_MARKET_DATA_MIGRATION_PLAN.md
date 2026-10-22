# A 股数据服务 Go 重写与架构迁移完整计划

**文档版本：** V1.1  
**制定日期：** 2026-10-21  
**目标仓库：** [Kasign/stock_data_provider](https://github.com/Kasign/stock_data_provider)  
**上游参考：** [ZhuLinsen/daily_stock_analysis](https://github.com/ZhuLinsen/daily_stock_analysis)（用于盘点原 Python Provider 和业务能力；不代表当前仓库已包含其全部实现）  
**参考仓库：** [ZhuLinsen/daily_stock_analysis](https://github.com/ZhuLinsen/daily_stock_analysis)  
**核心决策：** 同一 Git 仓库、两个独立 Go 服务、代码与配置物理隔离、独立构建与部署。

---

## 1. 执行摘要

建议将行情数据能力重写为独立的 Go 行情数据服务，并让 Go 股票分析服务通过内部 HTTP API 调用它。两个服务放在同一个 Git 仓库中，但代码、Go 模块、配置、环境变量、Docker 镜像、日志和运行职责都分开管理。

本项目不是简单地把 Python 文件逐行翻译成 Go。重写必须首先建立统一的数据契约，解决原始数据源追踪、成交量与成交额单位、复权口径、行情时间、缺失值及数据质量准入等问题。不能只以“接口返回 200”或“代码能编译”作为完成标准。

### 1.1 目标架构

```text
Web / 外部调用方
       |
       v
Go 股票分析服务  (独立代码、配置、镜像和进程)
  - 市场环境分析
  - 个股分析与技术指标
  - 策略评分、风险评估和报告
       |
       | 内部 HTTP API
       v
Go 行情数据服务  (独立代码、配置、镜像和进程)
  - 多数据源适配和路由
  - 原始数据保留、字段映射
  - 单位及时间标准化
  - 复权元信息和数据质量校验
  - 缓存、持久化、来源追踪
       |
       +---- 外部行情、K 线、财务、板块、资金流、龙虎榜、新闻公告等数据源
       |
       +---- 现有 Redis / MariaDB 实例（按服务划分访问职责）
```

### 1.2 关键决策

1. **同一个仓库，不等于同一个应用。** 两个服务有独立入口、依赖、配置、镜像、健康检查和发布流程。
2. **代码隔离。** 分析服务不得 import 行情服务的 `internal` 包，也不得直接依赖它的数据库表结构；通过稳定的 HTTP API 契约交互。
3. **配置隔离。** 两个服务分别拥有 `.env.example` 和配置目录；根目录只放编排配置、通用文档及示例，不放实际密钥。
4. **数据口径集中管理。** Provider 原始字段及其单位只在行情服务的适配/标准化边界处理一次，不在分析服务重复转换。
5. **以真实 Provider 为准。** API 入口名称不是底层数据源名称。无法确认实际 Provider 时必须标记 `unknown`，不得伪造来源。
6. **先核心、后扩展。** 优先完成实时行情和日 K 的完整闭环，验证通过后再扩展其他数据域。
7. **先影子运行，再切流。** 新服务在正式替代旧链路前，先对相同证券、时间范围及复权口径做字段级对账。

## 2. 现状问题和重写动机

参考仓库的部分实现存在需要在迁移时明确处理的契约问题：

- 历史行情请求能取得数据源信息，但 API 转换层没有完整保留实际 Provider。
- 部分 Provider 直接把供应商“成交量”字段重命名为 `volume`，不同路径的单位转换逻辑并不一致。
- 部分历史行情路径明确请求前复权，但 API 没有完整暴露最终复权口径及参考信息。
- 实时行情转换逻辑可能以服务端当前时间作为 `update_time`，不能据此证明行情源更新时间。
- 多数据源兜底后，若来源、切换原因和原始单位没有记录，就难以定位错误数据。
- 缓存中的旧格式数据可能缺少来源、单位、复权和质量信息，不能不经检查直接视为有效数据。

参考源码入口：

- [数据源优先级及初始化](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/data_provider/base.py)
- [Efinance 历史行情适配](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/data_provider/efinance_fetcher.py)
- [AkShare 历史行情适配](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/data_provider/akshare_fetcher.py)
- [统一实时行情数据类型](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/data_provider/realtime_types.py)
- [历史行情服务层](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/src/services/stock_service.py)
- [股票 API Endpoint](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/api/v1/endpoints/stocks.py)

这些问题说明需要重建数据契约，而不是简单照搬现有 API 的输出格式。参考仓库的实际 Provider、配置项和可用数据域仍需在实施第一阶段逐项盘点；本文的目标架构不代表所有 Provider 当前都已验证可用。

## 3. 目标仓库结构：同库、两服务、强隔离

建议的目标结构如下。实际迁移前先盘点当前仓库，按阶段移动或新增目录，不要一开始就大规模搬动现有代码。

```text
stock_analysis_server/
├── services/
│   ├── market-data/                    # Go 行情数据服务
│   │   ├── cmd/market-data/main.go
│   │   ├── internal/
│   │   │   ├── provider/               # Provider 接口和路由
│   │   │   ├── adapters/               # 各数据源适配器
│   │   │   ├── normalize/               # 单位、时间、字段标准化
│   │   │   ├── validate/                # 质量规则与准入
│   │   │   ├── service/                 # 数据获取用例
│   │   │   ├── repository/              # MariaDB 持久化
│   │   │   ├── cache/                   # Redis 缓存
│   │   │   ├── api/                     # HTTP Handler / Middleware
│   │   │   └── observability/           # 日志、指标、追踪
│   │   ├── configs/                     # 仅行情服务专属配置模板/规则
│   │   ├── migrations/                  # 行情服务数据库迁移
│   │   ├── tests/                       # 契约、集成及对账测试
│   │   ├── Dockerfile
│   │   ├── go.mod                       # 独立 Go module
│   │   ├── go.sum
│   │   └── .env.example                 # 仅行情服务变量，不含真实密钥
│   │
│   └── stock-analysis/                 # Go 股票分析服务
│       ├── cmd/stock-analysis/main.go
│       ├── internal/
│       │   ├── marketcontext/            # 大盘及市场环境
│       │   ├── stockanalysis/            # 个股分析主流程
│       │   ├── indicators/               # 技术指标
│       │   ├── strategy/                 # 策略与评分
│       │   ├── risk/                     # 风险评估
│       │   ├── marketdataclient/          # 行情服务 HTTP 客户端
│       │   ├── report/                   # 分析报告
│       │   └── observability/
│       ├── configs/                     # 仅分析服务专属配置
│       ├── tests/
│       ├── Dockerfile
│       ├── go.mod                       # 独立 Go module
│       ├── go.sum
│       └── .env.example                 # 仅分析服务变量，不含真实密钥
│
├── api-contracts/
│   └── market-data/v1/openapi.yaml       # 唯一有意共享的 HTTP 契约
├── deploy/
│   ├── compose.yaml                     # 只负责编排两个服务及网络
│   └── README.md
├── docs/
│   ├── architecture.md
│   ├── data-contract.md
│   ├── provider-matrix.md
│   ├── config-migration.md
│   ├── runbook.md
│   └── adr/                              # 重要架构决策记录
├── legacy-python/                        # 需要保留的 Python 部分；按现状决定是否使用此目录
├── .gitignore
└── README.md
```

### 3.1 物理隔离规则

- `services/market-data` 与 `services/stock-analysis` 分别维护 `go.mod`、`go.sum`、Dockerfile 和 `.env.example`。
- 两个服务不互相 import 对方的 `internal` 代码，不共享数据库 ORM Model，不共享业务配置结构。
- 唯一必需的跨服务接口是版本化 HTTP 契约；先通过 `api-contracts/market-data/v1/openapi.yaml` 管理，不让一个服务直接调用另一个服务的内部函数。
- `.env`、私有 API Key 和密码不提交 Git。每个服务保留 `.env.example`，由部署环境提供实际变量。
- 根目录的 `deploy/compose.yaml` 只负责网络、端口、依赖、健康检查、重启策略和 env 文件路径，不承载 Provider 业务配置。
- 如果现有 Go 项目只有一个根 `go.mod`，先设计迁移步骤并确保旧构建可复现，再逐个迁移到独立 module；不要直接删除旧入口。
- `legacy-python` 是否移动目录要依据现有部署约束决定。初始阶段可原地保留 Python 项目，先让 Go 服务与其并存，避免为目录整洁而打断运行链路。

### 3.2 配置隔离规则

示意配置：

`services/market-data/.env.example`

```dotenv
MARKET_DATA_HTTP_ADDR=:8080
MARKET_DATA_LOG_LEVEL=info
MARKET_DATA_DB_DSN=
MARKET_DATA_REDIS_ADDR=
MARKET_DATA_REDIS_PASSWORD=
MARKET_DATA_PROVIDER_PRIORITY=efinance,akshare
MARKET_DATA_REQUEST_TIMEOUT=10s
MARKET_DATA_MAX_RETRIES=2
MARKET_DATA_INTERNAL_API_KEY=
```

`services/stock-analysis/.env.example`

```dotenv
STOCK_ANALYSIS_HTTP_ADDR=:8090
STOCK_ANALYSIS_LOG_LEVEL=info
STOCK_ANALYSIS_MARKET_DATA_BASE_URL=http://market-data:8080
STOCK_ANALYSIS_MARKET_DATA_API_KEY=
STOCK_ANALYSIS_REQUEST_TIMEOUT=15s
```

以上只说明配置隔离方式，Provider 名称和默认优先级必须以实际盘点、可用性验证及用户配置为准，不能因为示例里出现某个 Provider 就认定它已可用。

禁止：

- 两个服务都读取同一份大型 `.env` 后各自解释一部分变量。
- 在分析服务中存放第三方行情源 API Key。
- 将密码、Token、真实 DSN 写入 `configs/` 并提交 Git。
- 用硬编码默认值悄悄覆盖错误配置。

## 4. 职责边界

### 4.1 行情数据服务负责

- Provider 注册、选择、优先级及故障切换。
- 请求超时、有限重试、退避、熔断和限流控制。
- 原始字段映射和必要的原始载荷留存/脱敏审计。
- 单位、日期、时间戳及枚举标准化。
- 复权元信息记录和跨口径校验。
- 数据质量校验、有效期判定及用途准入。
- Redis 缓存、行情数据持久化与缓存失效策略。
- 对外提供与具体 Provider 无关的统一 API。
- 记录来源、回退原因、错误类型和质量问题。

### 4.2 股票分析服务负责

- 大盘环境、市场宽度、指数与板块环境分析。
- 个股趋势、量价关系、技术指标及其他分析逻辑。
- 风险评估、策略评分、情景分析及报告生成。
- 调用行情数据服务获取符合口径的数据。
- 根据数据的质量状态和分析场景决定是否可继续，不自行修复来源不明的数据。

### 4.3 明确禁止的重复职责

- 分析服务不得再次把历史 `volume` 乘以 100。
- 分析服务不得自行猜测成交额单位或复权口径。
- 分析服务不得将采集时间当作供应商行情时间。
- 分析服务不得通过直接查行情服务的业务表绕过统一 API。
- 行情服务不负责输出“买入/卖出”结论，也不内嵌分析策略评分。

## 5. 统一数据契约

所有 Provider 必须先产出携带原始口径的 Raw 数据，再由统一标准化层生成 Canonical 数据。原始数据与标准化结果要能关联，不能在适配器中丢弃单位信息后再让通用层猜测。

### 5.1 证券标识

建议标准标识使用明确的交易所后缀，例如：

- `600519.SH`
- `000001.SZ`
- `430047.BJ`

API 边界可以暂时接受旧式的 `600519` 输入，但应在边界归一化并明确交易所。对不能唯一解析的代码应返回参数错误，而不是随意选择交易所。具体后缀格式在 OpenAPI 契约中固定，并保持全服务一致。

### 5.2 实时行情 Quote

| 字段 | 标准定义 | 规则 |
|---|---|---|
| `symbol` | 标准证券标识 | 必须可识别交易所 |
| `name` | 证券名称 | 可缺失，但需报告字段状态 |
| `current_price` | 当前价，CNY/股 | 保留原始精度，不提前取整 |
| `open` / `high` / `low` | 当日 OHLC，CNY/股 | 与供应商定义及交易阶段一致 |
| `prev_close` | 参考收盘价，CNY/股 | 必须说明实际口径，不能默认和复权 close 等价 |
| `change` | 涨跌额，CNY/股 | 保留供应商值并做一致性校验 |
| `change_percent` | 百分比数值 | `1.25` 代表 `1.25%`，不是 `0.0125` |
| `volume` | 成交量，股 | 标准输出固定为 shares |
| `amount` | 成交额，CNY | 标准输出固定为人民币元 |
| `quote_time` | 供应商行情时间 | 缺失时为 null/unknown，不得伪造 |
| `fetched_at` | 本系统收到响应的时间 | 系统生成，UTC RFC 3339 |
| `normalized_at` | 完成标准化的时间 | 系统生成，UTC RFC 3339 |
| `source` | 实际底层 Provider | 不能只写 API 中转服务名称 |
| `transport_source` | 采集入口/代理 API | 如通过 Python API 采集时单独记录 |
| `raw_volume_unit` | 原始成交量单位 | shares/lots/ten_thousand_shares/unknown 等 |
| `volume_unit` | 标准成交量单位 | 固定为 `shares` |
| `raw_amount_unit` | 原始成交额单位 | CNY/ten_thousand_CNY/100_million_CNY/unknown |
| `amount_unit` | 标准成交额单位 | 固定为 `CNY` |
| `quality_status` | 数据质量状态 | 由校验层计算 |
| `quality_reasons` | 质量问题列表 | 可供调用方解释或审计 |

价格和金额推荐使用十进制定点类型表达和存储，避免单位换算时的无意精度损失。技术指标计算可在明确转换边界后使用 `float64`，但原始/标准价格字段不得因计算方便而提前舍入。

### 5.3 历史 K 线 Bar

| 字段 | 标准定义 | 规则 |
|---|---|---|
| `symbol` | 标准证券标识 | 与 Quote 一致 |
| `trade_date` | 交易所交易日期 | `YYYY-MM-DD`，不是 UTC 午夜时间戳 |
| `period` | K 线周期 | 如 `1m`、`5m`、`1d` |
| `open/high/low/close` | OHLC，CNY/股 | 必须有一致的复权口径 |
| `volume` | 成交量，股 | 统一转换一次 |
| `amount` | 成交额，CNY | 统一转换为元 |
| `adjustment_basis` | 复权口径 | `none`、`forward_adjusted`、`backward_adjusted`、`unknown` |
| `adjustment_reference_date` | 复权参考日期 | 仅在可确认时提供 |
| `source` | 实际底层 Provider | 保留真实来源 |
| `fetched_at` | 本系统采集时间 | 系统生成 |
| `quality_status` | 质量状态 | 经校验生成 |

**禁止混接不同复权口径的 K 线。** 实时成交价与复权历史收盘价比较前，必须检查是否口径兼容。未知复权口径的数据可根据用途保留展示，但不能无条件用于依赖连续价格序列的策略。

### 5.4 通用来源元数据

至少包括：`source`、`transport_source`、`source_version`、`request_id`、`provider_timestamp`、`fetched_at`、`normalized_at`、`fallback_from`、`fallback_reason`、原始单位、复权口径、`normalization_version`、`quality_status` 和 `quality_reasons`。

当上游 API 丢弃了底层 Provider 信息时，应记录类似：

```json
{
  "source": "unknown",
  "transport_source": "daily_stock_analysis_api",
  "quality_status": "degraded",
  "quality_reasons": ["provider_identity_unavailable"]
}
```

不得把 `daily_stock_analysis_api` 伪装成实际行情供应商，也不得靠猜测将 `source` 填成 Efinance 或 AkShare。

## 6. 单位标准化政策

### 6.1 成交量：统一为股

标准字段 `volume` 的单位固定为股（shares）。每个 Provider 的字段映射必须声明原始单位：

- 原始单位是股：不转换。
- 原始单位是手：根据该证券市场规则转换；普通 A 股通常为 100 股/手，但规则必须来源明确。
- 原始单位是万股等其他单位：按已确认的原始契约转换。
- 原始单位未知：禁止猜测，相关数据标记降级或无效，并阻止其参与依赖成交量的分析。

样例：原始 `34136` 手只有在 Provider 明确声明单位为手时，标准值才是 `3,413,600` 股。不得对全部 Provider 无条件乘以 100，以免重复转换。

每条数据至少保留原始量值、原始单位、标准量值、标准单位和标准化版本。可通过 `amount / price` 与成交量进行数量级一致性检查，但该检查只能用于发现异常，不能替代供应商单位契约。

### 6.2 成交额：统一为人民币元

- 元：不转换。
- 万元：乘以 `10,000`。
- 亿元：乘以 `100,000,000`。
- 未知：保持未知，不猜测。

### 6.3 价格、涨跌幅与换手率

- 价格统一为 CNY/股，保留数据源精度。
- `change_percent` 和 `turnover_rate_percent` 统一使用百分比数值，`2.5` 表示 `2.5%`。
- 若 Provider 提供的是比例小数，必须在其独立适配器中明确转换。
- 供应商原值应保留，标准化后的值另存，不静默覆盖来源数据。

### 6.4 缺失值规则

- 未知不得写成 0。
- 不可用的数值用 null 或字段状态表达。
- 不得用默认日期补造行情时间。
- 不得用最新缓存值冒充实时值。
- 缺少必需字段时，按数据域和用途返回 `degraded` 或 `invalid`，并列出原因。

## 7. 时间、交易日和行情新鲜度

统一区分：

- `provider_timestamp` / `quote_time`：供应商声明的行情时间。
- `fetched_at`：本系统收到响应的时间。
- `normalized_at`：标准化结束时间。
- `trade_date`：交易所交易日期。

规则：

1. 时间戳以带时区的 RFC 3339 表达，存储时可统一 UTC，展示时使用 `Asia/Shanghai`。
2. 没有供应商时间时明确记录未知，不以 `now()` 代替。
3. 日 K 使用 `trade_date` 对齐；禁止依赖 UTC 转换结果推导交易日期。
4. 过期判定应结合交易时段、交易状态、Provider 更新时间、采集时间和缓存策略。
5. 非交易时段的最后行情可以作为“最近已知行情”提供，但不得标成正在更新的实时行情。
6. 新鲜度阈值按数据类型和交易时段配置，不设置一个适用于所有接口的固定阈值。

## 8. 数据质量与使用准入

建议状态枚举：

- `valid`：满足对应用途的契约及质量要求。
- `degraded`：有明确问题，只能用于允许降级的用途。
- `stale`：超过相应场景的时效要求。
- `invalid`：存在关键字段、单位或约束错误。
- `unknown`：关键元数据尚无法确认。

最低校验项：

1. 证券代码、交易所和数据周期是否合法。
2. OHLC 满足 `low <= open/close <= high`。
3. 价格、成交量、成交额数值合法且非异常负值。
4. 原始单位和转换规则明确。
5. 交易日期无重复、乱序和不合理缺口；缺口规则应考虑停牌及非交易日。
6. 实时行情新鲜度符合当前用途要求。
7. 复权口径与分析输入要求兼容。
8. 涨跌额、涨跌幅、参考收盘价之间没有无法解释的明显冲突。
9. 来源可追踪，兜底切换原因可见。
10. 缓存数据有写入时间、版本及过期判断。

数据质量不是单一总分能替代的。比如成交量单位未知时，价格可以在提示后用于展示，但不得用于量价背离分析；行情来源未知或复权不兼容时，也不能无条件用于依赖这些条件的交易判断。

## 9. 行情数据服务 API 初步规划

接口由 `api-contracts/market-data/v1/openapi.yaml` 管理，路径以 `/v1` 版本化。首期不必全部开发，可按优先级逐步开放。

### 9.1 首期接口

- `GET /health/live`：进程是否存活。
- `GET /health/ready`：关键依赖是否满足就绪条件。
- `GET /v1/quotes/{symbol}`：实时/最近行情，响应中明确实时性及质量状态。
- `GET /v1/bars/{symbol}?period=1d&start_date=...&end_date=...&adjustment_basis=...`：历史 K 线。
- `GET /v1/data-quality/{symbol}`：最近行情及历史数据的质量概况（可以后置实现）。

### 9.2 后续接口域

- 指数及市场环境数据。
- 板块和行业数据。
- 财务指标及财报。
- 个股资金流与筹码分布。
- 龙虎榜。
- 新闻、公告及其来源/发布时间。

所有响应应返回统一错误结构、request ID、数据来源及质量元信息。API Key、超时、速率限制和内部网络访问策略应在服务边界实现。行情服务默认只在 Docker 内部网络暴露，不直接公开第三方数据接口。

## 10. 配置、Redis 与 MariaDB

### 10.1 配置迁移

第一阶段先产出完整配置清单，包含：旧配置名、用途、默认值、作用服务、迁移目标、是否保留、验证方式。之后再创建 Go 配置结构，避免照搬大量失效的 Python 专属变量。

应尽量保留有效配置能力，例如 Provider 开关、优先级、API Base URL、Key、超时、重试、缓存时长和数据域开关。只保留已确认有用的配置，不保证 Python 每个配置项都能原样复用。

### 10.2 Redis

- 行情服务管理行情缓存、TTL 和数据新鲜度。
- Key 使用服务专属前缀及版本号，例如 `market-data:v1:quote:600519.SH`。
- 缓存值必须保留来源、更新时间、复权/单位及质量元数据。
- 旧格式缓存不能直接当作新标准格式使用；切换时应通过版本化 Key、失效或显式迁移处理。
- 分析服务不应通过 Redis 直接读取行情服务的内部 Key，除非未来有明确的只读共享缓存契约。

### 10.3 MariaDB

- 行情服务拥有行情及 Provider 相关表的写入责任。
- 分析服务如果需要保存分析报告，应使用自己的表或 schema/前缀，不直接修改行情服务表。
- 同一个 MariaDB 实例可以复用，但应清晰划分表、权限和迁移脚本。
- 表结构变化通过版本化 migration 管理，先备份，再迁移并验证回滚方案。
- 如果已有行情表被旧 Python 服务使用，不可直接破坏性改表；采用新增字段、兼容读取、回填/验证、切换、清理的分阶段策略。

## 11. Python 与 Go 的迁移策略

### 11.1 过渡原则

- 既有 Python 应用先保持可运行，不要求随 Go 数据服务上线立即停止。
- 可暂时把 Python API 当作一个兼容型上游 Provider，但要明确其数据来源、单位和时间可能不完整。
- 对没有底层 Provider 信息的旧接口，Go 侧应标记来源未知，不可凭经验硬编码实际源。
- 最终目标是 Go 数据服务直接实现需要保留的 Provider；Python 兼容适配器仅为过渡手段。
- 不迁移所有 Python 业务模块；本次只先迁移数据提供能力及其必要依赖。

### 11.2 数据源迁移策略

逐个 Provider 建立迁移表：

| 条目 | 必填信息 |
|---|---|
| Provider 名称 | 原始实现与 Go 目标名称 |
| 数据域 | 实时行情、日 K、分钟 K、财务、资金流等 |
| 配置项 | URL、Key、超时、开关、优先级 |
| 原始字段 | 字段名、类型、单位、缺失语义 |
| 复权方式 | 请求参数及最终返回口径 |
| 时间语义 | 行情时间、更新日期或仅采集时间 |
| 限制 | 限流、授权、频率、失效处理 |
| 失败行为 | 超时、重试、兜底条件及切换原因 |
| 测试样本 | 已脱敏的真实响应、预期标准化输出 |

只迁移经过盘点且确实需要的 Provider。不要为了追求与参考仓库文件数量一致而机械重写全部适配器。

## 12. 测试方案与验收标准

### 12.1 单元测试

- 每个 Provider 的原始字段映射。
- 手/股/万股转换及防止重复转换。
- 元/万元/亿元转换。
- 百分比数值转换。
- 时间戳与交易日期处理。
- 复权口径及未知口径处理。
- 缺失值、异常 OHLC、过期数据和质量状态判定。
- Provider 失败及回退原因记录。

### 12.2 契约测试

每个 Provider 都覆盖正常、空响应、字段缺失、类型变化、超时、限流、错误码、时间缺失、单位未知、兜底切换等场景。测试必须验证源字段、原始单位、标准值、来源和质量状态，而不仅仅是没有返回 error。

### 12.3 集成与真实数据对账

针对同一证券、同一周期、同一时间范围和相同复权口径，比较 Python 旧链路和 Go 新链路：

- 字段完整性。
- 原始单位与转换结果。
- 成交量和成交额数量级。
- OHLC 及交易日期。
- 行情时间、采集时间和数据新鲜度。
- 底层来源与回退链。
- 缺失数据、停牌、除权和供应商异常场景。

供应商之间价格可能有合理的更新时间或口径差异。对账不能简单要求不同 Provider 的所有值逐字相等；必须先确认交易时点、复权口径和数据定义，再设定允许误差及解释规则。

### 12.4 上线验收门槛

1. 首批 Provider 的字段和单位映射清单完整。
2. 标准成交量统一为股，成交额统一为 CNY。
3. 来源、行情时间和复权信息不再被默默丢弃。
4. 未知单位、过期行情和不兼容复权的数据会被正确标记，并受到对应分析用途限制。
5. Redis 缓存和 MariaDB 读写后保留标准字段及关键元数据。
6. 两个服务能分别构建、测试、启动、重启和查看日志。
7. 分析服务只能通过版本化 API 调用数据服务，没有访问其内部代码或表结构的隐性依赖。
8. 单元测试、集成测试、`go test ./...`、`go vet ./...` 通过；若支持且无环境障碍，应执行 race 检查。
9. 真实数据影子对账达到约定的字段完整率、单位正确率和可解释差异标准。
10. 已验证切流、回滚、密钥隔离和服务健康检查。

## 13. 分阶段实施路线

### 阶段 0：仓库与现状盘点

**任务：**
- 盘点当前 Go 仓库入口、`go.mod`、目录、配置、Dockerfile 和 compose。
- 盘点 Python 参考仓库的 Provider、数据域、优先级、配置、缓存和模型。
- 盘点 Redis/MariaDB 当前表、Key、访问者与旧服务依赖。
- 用当前真实 API 样本记录现有字段、来源丢失、单位不明、复权和时间问题。
- 建立配置映射表和 Provider 能力矩阵。

**产出：** `docs/provider-matrix.md`、`docs/config-migration.md`、现状目录图、风险列表。  
**完成条件：** 明确第一阶段实际要迁移的 Provider 与数据域，不能只凭参考仓库文件名推测。

### 阶段 1：架构骨架与数据契约

**任务：**
- 明确两个服务独立运行、独立 Go module 的构建方式。
- 建立两个独立配置目录及 `.env.example`。
- 定义 Raw 模型、标准 Quote/Bar、来源元数据和质量状态。
- 定义 `/v1` OpenAPI、统一错误格式、request ID、内部鉴权。
- 确定 Redis Key 版本和 MariaDB 表迁移方式。

**产出：** 目录骨架、配置模板、OpenAPI、数据契约文档和契约测试样例。  
**完成条件：** 两个服务可分别构建，且不互相 import 内部包。

### 阶段 2：行情服务核心框架

**任务：**
- 实现 Provider 接口、注册表、优先级和有限的故障切换。
- 实现超时、重试、退避、错误分类和来源记录。
- 实现标准化与质量校验管线。
- 实现健康检查、结构化日志和基本指标。

**产出：** 可以使用 mock Provider 验证的行情服务核心。  
**完成条件：** 单测覆盖关键失败分支，不依赖真实第三方接口也可验证路由和质量规则。

### 阶段 3：首批 Provider 与核心数据闭环

**优先级：** 实时行情与日 K 优先；是否复用 TickFlow、Efinance、AkShare 或其他 Provider，以阶段 0 的可用性、授权、频率和稳定性盘点为准。

**任务：**
- 接入首批实际可用 Provider。
- 明确每个 Provider 的原始字段、成交量单位、成交额单位、时间及复权口径。
- 实现字段标准化、原始来源保存和质量报告。
- 对 `600519.SH` 等测试证券进行真实接口样本验证，同时覆盖不同交易所和异常场景。

**完成条件：** 实时行情与日 K 可从 Go 数据服务返回标准字段，未知口径会明确降级，不会悄悄猜测。

### 阶段 4：Redis 与 MariaDB

**任务：**
- 接入现有 Redis 与 MariaDB，使用服务专属配置和权限。
- 实现缓存键版本、TTL、缓存新鲜度和失效规则。
- 完成必要 schema/migration，并记录 Provider 与质量元数据。
- 兼容并处置旧缓存格式，避免旧记录被当成新契约数据。

**完成条件：** 缓存命中与数据库读写不会丢失单位、来源、时间和复权信息；迁移可验证、可回滚。

### 阶段 5：影子运行、对账与切流

**任务：**
- 新旧链路并行查询相同数据，Go 结果不立即影响正式分析决策。
- 生成自动化对账结果，按差异类型分类。
- 修复单位错误、复权混接、日期错位、时间语义和来源追踪问题。
- 达到验收门槛后，先让分析服务的测试环境调用 Go 数据服务，再逐步切换正式流量。

**完成条件：** 重要差异可以解释，关键口径错误为零；具备切换与回滚操作步骤。

### 阶段 6：分析服务隔离与正式部署

**任务：**
- 整理分析服务为独立入口、module、配置和镜像。
- 实现带超时、鉴权、错误处理和质量策略的 `marketdataclient`。
- 将分析主流程对原数据获取逻辑的依赖改为调用统一 API。
- 创建内部 Docker 网络、服务健康检查、重启策略及部署手册。

**完成条件：** 两个容器可独立发布；分析服务不依赖行情服务内部数据库结构或 Go 内部包。

### 阶段 7：扩展数据域

核心链路稳定后，按需求和数据源可用性逐步接入：

1. 指数、市场宽度与市场环境。
2. 分钟 K 线和其他周期。
3. 财务指标及财报。
4. 板块、行业和概念。
5. 个股资金流与筹码分布。
6. 龙虎榜。
7. 新闻与公告。

每个数据域重复执行字段契约、单位/时间声明、Provider 对账、质量准入和 API 版本评审，不允许因数据类型不同而绕过标准。

### 阶段 8：旧链路退役与持续运维

**任务：**
- 确认所有目标消费者已经切换。
- 删除或禁用不再需要的兼容适配器，但保留短期回滚能力。
- 更新 README、架构图、配置说明和故障排查手册。
- 监控 Provider 成功率、切换次数、过期数据比例、质量拒绝比例、接口延迟和缓存命中率。

**完成条件：** 旧链路退役有验证记录，数据服务成为标准行情入口，后续新增 Provider 有明确接入流程。

## 14. 部署与运维要求

- 两个服务分别有 Dockerfile、镜像标签、健康检查和重启策略。
- 通过 Docker 内部网络互相通信；行情服务默认不向公网暴露端口。
- Compose 只负责编排，实际配置由各服务自己的 env 文件或部署密钥提供。
- 行情服务应有存活与就绪检查；Redis 或数据库暂时不可用时的 readiness 行为应明确，避免错误地报告完全就绪。
- 所有日志包含服务名、request ID、Provider、耗时、回退原因和质量状态，但不输出 API Key、密码或完整敏感请求头。
- 监控 Provider 成功率、错误分类、限流、回退次数、行情新鲜度、标准化失败数和缓存命中率。
- 对关键接口设置连接超时、请求超时、最大重试和并发限制，防止多数据源故障造成请求放大。
- 变更表结构或标准化规则时应备份，并考虑 `normalization_version` 对旧数据的影响。

## 15. 项目风险与控制措施

| 风险 | 控制办法 |
|---|---|
| 参考仓库的部分 Provider 不可用、限流或数据定义不明 | 先做能力和可用性盘点；保留多个候选但只实现验证通过者 |
| Python API 已丢弃实际 Provider 信息 | 标记来源未知；修复 API 契约或直接接入真实 Provider |
| 成交量重复乘以 100 | 每个 Provider 声明原始单位；只允许标准化层转换一次；加入单测 |
| 前/后复权序列混合 | 明确 `adjustment_basis`，分析前强制检查兼容性 |
| API 时间被误当行情时间 | 分离 Provider 时间、采集时间和标准化时间 |
| 两个服务仍然共享隐性配置或数据库结构 | 独立 `.env.example`、go.mod、migrations 和 API 客户端 |
| 共享 MariaDB/Redis 造成相互干扰 | 表及 Key 命名空间、独立权限、清晰的数据所有权 |
| 切换后分析结果变化 | 影子运行与差异报告；对策略结果变化做可追踪解释 |
| 迁移期间影响原 Python 系统 | 并行运行、非破坏性 schema 变更、分阶段切流和回滚 |
| 新服务发生故障导致分析中断 | 客户端超时、有限重试、明确降级，不把旧数据伪装为实时数据 |

## 16. 工作量估算

这是单人开发、现有服务器与数据库可用、可以访问必要数据源的粗略规划，不是完成源码逐项盘点后的工期承诺。

| 范围 | 粗略估算 |
|---|---:|
| 仓库、Provider、配置和数据库盘点 | 1–3 个工作日 |
| 双服务骨架、配置隔离与 API 契约 | 2–4 个工作日 |
| 数据模型、标准化和质量规则 | 2–4 个工作日 |
| 首批 Provider（实时行情与日 K） | 3–7 个工作日 |
| Redis、MariaDB 与迁移脚本 | 2–4 个工作日 |
| 影子对账、异常测试与安全切换 | 3–7 个工作日 |
| 部署、监控、文档和验收收尾 | 2–4 个工作日 |

部分工作可并行，阶段间也会复用成果。核心行情服务的初步计划约为 **2–5 周**；若要求覆盖全部 Provider 及财务、板块、资金流、龙虎榜、新闻公告等所有数据域，整体可能需要 **4–8 周或更长**，取决于可用接口、数据质量和授权限制。不能仅根据仓库代码量确定准确工期。

## 17. 最终交付清单

1. 两个独立 Go 服务源码及各自的 `go.mod`、`go.sum`。
2. 两份服务专属 `.env.example` 及配置说明。
3. 两份 Dockerfile 和一份只负责编排的 Compose 文件。
4. 版本化 OpenAPI 契约、统一 Quote/Bar/来源/质量字段规范。
5. Provider 与配置迁移清单。
6. 各 Provider 的字段映射、原始单位、复权及时间契约。
7. 标准化、质量校验、故障切换及缓存代码。
8. MariaDB migration、Redis Key 及缓存格式说明。
9. 单元测试、契约测试、集成测试及真实数据对账工具/报告。
10. 部署、启动、健康检查、日志排查、切流和回滚手册。
11. 旧 Python 兼容链路的保留、切换和退役记录。

## 18. 本计划的执行顺序

开发时严格按以下顺序推进，不建议跳步：

1. 盘点当前 Go 项目及参考 Python 仓库，确认真正需要保留的 Provider、配置和数据域。
2. 冻结数据契约：单位、来源、复权、时间、缺失值及质量准入。
3. 建立同一仓库内的双服务隔离结构与版本化 API。
4. 完成实时行情和日 K 的 Provider、标准化、质量校验及真实数据对账。
5. 接入现有 Redis/MariaDB，完成缓存和表结构迁移。
6. 影子运行并验收后，让分析服务通过 HTTP API 消费数据。
7. 扩展其余数据域；每个数据域独立验收后再上线。
8. 最后退役过渡兼容层，并完善运维监控。

**最终原则：一个仓库管理项目，两个服务各自独立；一个统一的数据契约管理口径，所有 Provider 都必须遵守；只有通过质量校验且满足具体分析用途的数据，才能进入分析决策。**


---

## 19. 2026-10-21 设计补充：Provider 迁移、日志及任务进度

本节为当前仓库的实施补充；主设计的双服务隔离、统一数据契约、渐进切流和质量门禁仍然有效。当前代码状态应以以下文档为准：

- [架构 Review 记录与工程规范](./ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md)
- [日志与任务步骤进度方案](./LOGGING_AND_TASK_PROGRESS_PLAN.md)
- [Go Provider 迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)
- [Go 数据源切换进度](./GO_DATA_PROVIDER_SWITCH_STATUS.md)

### 19.1 同花顺（THS）候选源

将 [Kasign/Financial-API](https://github.com/Kasign/Financial-API) 纳入待评估的同花顺接入参考。该仓库描述了通过 REST API、MCP、CLI、Python 接入同花顺金融数据的能力；这不是对每个账户权限、配额或字段口径的保证。

建议顺序：先验证 API Key、服务条款、速率限制和端点清单；再优先对比最新行情/日 K，之后才评估指数/板块、财务、交易日历、涨跌停/炸板/连板、龙虎榜等独立数据域。分钟 K、tick、海外行情、宏观数据、新闻公告原文或研报不应在没有官方文档证据时宣称支持。

实现时新增独立 THS Provider 与环境变量，默认关闭；按能力注册，而不是强制每个源实现所有接口。只有经过真实样本对账、单位/时点/复权契约测试和错误场景测试的能力才能进入 fallback 顺序。

### 19.2 结构化日志与可观测性

所有服务逐步统一 JSON structured logging；request_id、run_id/task_id、service、operation/stage、provider/dataset、status、duration、quality/fallback 是必须字段。日志只输出安全摘要，不能默认打印请求体、响应体、密钥、完整 watchlist 或 LLM prompt。数据库 stage event 继续作为可追溯审计记录。

### 19.3 实时任务步骤状态

当前同步分析 API 与已持久化的 stage events 不能完整满足“任务运行中显示每一步”的 UX。下一阶段应在保留同步兼容 API 的基础上增加异步任务表、有限 worker、POST 202 + task_id、task 状态查询和 after_sequence 事件轮询；SSE 可作为后续提升。状态必须来自实际阶段事件，不能估算或编造百分比。

### 19.4 提交与验收纪律

每项完整模块功能在代码、配置、测试、部署变量、中文说明齐备后再作为单个 feature commit；文档独立翻译和进度更新应汇总成一组文档提交，不要每改一页就提交。根模块、go-core、services/market-data 三个 Go module 都必须运行测试、竞态检测与 vet；任何 Provider 扩展都要附真实字段契约或脱敏 fixture、失败/限流/缺字段覆盖和质量说明。

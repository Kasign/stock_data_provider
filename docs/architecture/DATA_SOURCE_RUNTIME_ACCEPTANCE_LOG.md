# 数据源真实环境验收与故障排查记录

> 记录日期：2026-10-22（Asia/Shanghai）
> 仓库：`Kasign/stock_data_provider`
> 分支：`feat/full-go-data-providers-api-switch`
> 部署环境：Ubuntu NAS；Docker Compose 项目目录为 `/server-whl/docker/stock_analysis_server`
> 目的：保存真实容器测试证据、失败现象和后续诊断步骤。本文不包含 API Key、数据库密码或其他秘密。

## 1. 当前验收状态

| 能力 / Provider | 状态 | 证据与边界 |
|---|---|---|
| BaoStock 原生 Go 日 K | **真实查询通过** | 容器通过 TCP 10030 登录并返回多个 A 股的历史日 K；前复权、后复权、不复权均有样本测试。仍应持续观察稳定性与更广范围的数据质量，不等于 BaoStock SLA 保证。 |
| 多 Provider 日 K 回退 | **通过** | 人为将 BaoStock 地址改成 `127.0.0.1` 后连接拒绝；Eastmoney 当时请求 EOF；Tencent 返回 K 线，HTTP 200 且按缺失成交额标记 degraded。 |
| MariaDB 日 K 写入 | **通过** | 已用 SQL 查询到 `stock_market_db.market_data_daily_bars` 中的 BaoStock/Tencent 行及其来源、复权基准、质量状态和 fetched_at。 |
| MariaDB 不复权回退 | **通过** | 在所有日 K Provider 均不可用的测试配置下，`adjustment=none` 请求返回 MariaDB 历史行，状态为 `stale`，含 `provider_unavailable_served_from_mariadb`。 |
| MariaDB 前/后复权回退 | **按当前保护条件拒绝** | 对缺少可验证 `AdjustmentRefDate` 的调整后历史序列，`coherentStoredSeries` 会拒绝回退，避免混用复权参考不明的价格。相关设计限制不是 MariaDB 连接失败。 |
| 缓存行为 | **应用层缓存命中/未命中有观察证据** | 请求曾返回 `X-Cache: HIT` 和 `X-Cache: MISS`。该头表示服务处理结果的缓存状态；单凭该头不能独立证明某次数据来自 Redis，Redis 是否正常应结合服务启动日志/健康检查确认。 |
| Eastmoney 历史日 K | **未通过 / 根因未定位** | 特定历史 K 线接口请求多次在 TCP 建连阶段报 `Network unreachable`。域名根路径却可建立 HTTPS 并返回 404；不能据此断言整个域名被封。 |
| Tencent 独立单源验证 | **尚未完成** | 已验证其作为回退源能够解析日 K，但还需在仅启用 Tencent 的配置下独立验收。 |
| Sina 独立单源验证 | **尚未完成** | 后续按实际支持能力独立验证。 |

## 2. BaoStock 真实数据验收

### 2.1 代表性返回

- `600519.SH`，交易日 `2026-10-21`，前复权：close 1309.69，volume 3,351,416 股，amount CNY 4,403,823,069.64；`source=baostock`、`quality_status=valid`、`warnings=[]`。
- `600519.SH`，交易日 `2026-09-02`：
  - 前复权 close 1297.5；
  - 不复权 close 1297.5；
  - 后复权 close 9950.8609575；
  - 三种请求均返回 volume 2,030,845 股、amount CNY 2,634,084,231.35，且状态为 valid。
- `688981.SH`，交易日 `2026-09-02`：前复权、后复权、不复权样本的 close 均为 122.55，volume 28,545,334 股、amount CNY 3,518,403,461.28，状态均为 valid。单个交易日三种价格相同本身不足以推断整个复权序列没有调整。
- `000001.SZ`，交易日 `2026-10-21`，不复权：OHLC 为 11.44 / 11.51 / 11.39 / 11.41，volume 81,354,137 股，amount CNY 930,546,787.77，状态 valid。
- `600036.SH`，`2026-10-19` 至 `2026-10-21`，前复权：三根日 K 均由 BaoStock 返回，包含成交量和成交额，状态 valid、warnings 为空。

### 2.2 数据库存储与多日范围

- 已在 MariaDB 中确认 `600036.SH` 这三条前复权记录，来源均为 `baostock`，`fetched_at` 对应首次获取时间。
- 曾查询 `600519.SH` 的 `2026-09-02` 不复权记录，数据库同时有 Tencent（degraded）和 BaoStock（valid）行；按 source 保存多份记录符合当前唯一键策略。
- `2026-09-02` 至 `2026-10-21` 的 `600519.SH` 后复权范围请求返回 30 条记录。此区间包含周末和非交易日，返回数小于日历天数或 limit 并不能单独认定缺失。
- 缓存命中测试得到过 `X-Cache: HIT`；恢复 BaoStock 后的冷请求得到 `X-Cache: MISS` 和最新的 `fetched_at`。

## 3. 多 Provider 故障回退验收

故障注入时，BaoStock 被临时设置为 `127.0.0.1:10030`，导致 connection refused。请求：

```text
GET /api/v1/bars/600036.SH/daily
  ?start=2026-10-20
  &end=2026-10-21
  &limit=2
  &adjustment=none
```

观察结果：

- HTTP 200，`X-Cache: MISS`。
- BaoStock 连接拒绝；Eastmoney 错误为 EOF；Tencent 返回两条日 K，故障回退链路成功。
- Tencent 原始 volume 以手返回，标准化 volume 转换为股（例如 760,948 手 → 76,094,800 股）。
- Tencent 返回的成交额为 null，`amount_unit=unknown`，数据质量为 degraded；原因包含 `tencent_daily_kline_has_no_verified_amount_field`、`amount_missing_from_provider`、`provider_fallback_used` 和 `amount_unit_unknown_because_amount_missing`。
- 此测试证明服务能在前序源不可用时回退到 Tencent，并显式标记字段限制；不证明 Eastmoney 当前长期不可用。

## 4. MariaDB 回退验收

### 4.1 不复权序列：通过

当测试配置只启用 BaoStock 日 K、把 BaoStock 主机改为 `127.0.0.1`，并关闭 Eastmoney/Tencent/Sina 后，请求：

```text
GET /api/v1/bars/600519.SH/daily
  ?start=2026-09-02
  &end=2026-09-02
  &limit=7
  &adjustment=none
```

返回 HTTP 200、`X-Cache: MISS`，记录 `source=baostock`、`quality_status=stale`、`quality_reasons=["provider_unavailable_served_from_mariadb"]`。顶层 warnings 为：

- `provider_unavailable_served_from_mariadb`
- `mariadb_bars_use_one_coherent_source_and_adjustment_reference`

该结果验证了上游不可用时从 MariaDB 读取同一来源历史序列的逻辑。记录的 `source=baostock` 表示原始来源，不表示本次重新请求到了 BaoStock。

### 4.2 前复权序列：当前实现有意拒绝

对 `600036.SH` 的 `2026-10-19` 至 `2026-10-21` 前复权数据库记录，在模拟上游全失效后测试返回 HTTP 502。代码中的 `coherentStoredSeries` 要求调整后序列具有可验证的 `AdjustmentRefDate`；缺少时会拒绝用这批调整后数据作为回退。当前数据模型/Provider 没有证明共同复权参考日的证据，故该保护行为符合当前实现约束。后续如需支持调整后历史回退，应先定义并保存复权参考日，再添加一致性校验和专项测试，不应仅删除保护条件。

## 5. Eastmoney 网络故障记录（2026-10-22）

### 5.1 单源配置已在容器生效

故障诊断期间，容器内实际配置输出为：

```text
MARKET_DATA_PROVIDER_PRIORITY=eastmoney
MARKET_DATA_EASTMONEY_ENABLED=true
MARKET_DATA_EASTMONEY_QUOTE_ENABLED=false
MARKET_DATA_EASTMONEY_DAILY_BARS_ENABLED=true
MARKET_DATA_BAOSTOCK_ENABLED=false
MARKET_DATA_TENCENT_ENABLED=false
MARKET_DATA_SINA_ENABLED=false
```

服务启动日志：

```text
2026-10-22T19:19:39.010828023+08:00
level=INFO
msg="market-data service starting"
providers=eastmoney
quote_providers=
daily_bar_providers=eastmoney
mariadb_enabled=true
redis_enabled=true
```

因此该次诊断环境确实只注册了 Eastmoney 日 K Provider，不是因为路由仍在优先调用 Tencent。

### 5.2 API 请求结果与缓存/数据库回退

- 对 `600036.SH` 查询 `2026-10-19` 至 `2026-10-21`、`adjustment=none`、`limit=9`，请求返回 HTTP 200、`X-Cache: MISS`，但数据是 MariaDB 返回的 Tencent 历史记录：`source=tencent`、`quality_status=stale`，记录中的 fetched_at 为 `2026-10-22T10:24:22.933983463Z` 左右。
- 该响应记录里的 `fallback_reason` 包含此前 BaoStock 连接拒绝和 Eastmoney EOF，它属于已存储记录的 metadata；不能将其直接视为本次 HTTP 请求的实时错误。
- 对 `2026-10-22` 单日请求（`limit=11`、`adjustment=none`）返回 HTTP 502，说明 Eastmoney 没有交付本次请求可用的数据，但当时没有保存其响应体，故尚不能精确区分网络失败、HTTP 错误、空 payload 或响应解析/契约校验错误。
- 当时过滤 `docker compose logs --since=15m --tail=300 market-data` 只得到服务启动信息，没有逐次 provider 错误。这直接促成分支后续提交增加结构化日志：`market data provider failed`，包含 `fetch_id`、`capability`、`symbol`、`provider`、`error_kind` 和已清理 URL 查询串的 `error`；同一清理后的错误摘要也用于本次请求的失败原因/回退元数据，避免将完整 URL 查询参数写入新错误记录。该改动已经添加单元测试并通过 Go 测试、race detector、vet 和 Docker 镜像构建；需等包含该提交的镜像实际部署后，运行时才会输出这些新日志。旧 MariaDB 行中已有的 `fallback_reason` 不会被自动回写清理。下一轮仍需同时保存 HTTP 错误响应体。

### 5.3 原始域名与接口探测

以下均从 `stock-market-data`（Compose 服务名 `market-data`）容器测试，另有一条在 Ubuntu 宿主机执行。

| 时间（约，Asia/Shanghai） | 目标 | 结果 | 可支持的结论 |
|---|---|---|---|
| 约 19:20 | `www.baidu.com:443`（容器） | HTTP 200 | 容器当时可通过 HTTPS 访问外部站点；不能排除对特定目的 IP 的路由问题。 |
| 约 19:20 | `push2.eastmoney.com:443` 根路径（容器） | 解析到 `61.129.129.196`，HTTP 404 | 该次到这个 IP 的 HTTPS 连接已建立；根路径 404 并非行情接口成功。 |
| 约 19:20 | `push2his.eastmoney.com:443` 根路径（容器） | 解析到 `14.103.188.89`，HTTP 404 | 该次到这个 IP 的 HTTPS 连接已建立；不能由此推断其他解析 IP 均可达。 |
| 约 19:28 | `push2his.eastmoney.com:443` 根路径（宿主机） | IPv4 `14.103.188.89` 和 IPv6 地址均被解析；IPv4 TCP/TLS 成功，HTTP 404 | 宿主机对当次选择的 IPv4 地址可建立连接。 |
| 约 19:20 | 历史行情接口（容器） | 一次解析到 `101.42.128.173:443`，报 `Network unreachable` | 该次尝试在网络连接阶段失败，没有 HTTP 响应。 |
| 约 19:28 | 历史行情接口（容器） | 一次解析到 `61.129.129.199:443`，报 `Network unreachable` | 又一次在网络连接阶段失败；域名的解析目标和根路径探测时不同。 |

接口探测使用的公开端点、样本和主要参数：

```text
https://push2his.eastmoney.com/api/qt/stock/kline/get
secid=1.600036
fields1=f1,f2,f3,f4,f5,f6
fields2=f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61
klt=101
fqt=0
beg=20261019
end=20261022
lmt=11
ut=<public front-end token configured by provider code>
```

完整请求中的 `ut` 是项目代码中默认配置的公开前端 token，不是个人 API Key；为避免不必要传播，本记录不复制其值。

### 5.4 当前判断

**根因未确定。现有证据不足以判断为运营商封锁，也不足以认定整个 Eastmoney 域名被封。**

目前证据更精确的描述是：容器对若干解析到的 IP 的根路径访问曾能建立 HTTPS，而真正的历史行情接口在尝试连接其他解析 IP 时报告 `Network unreachable`。这可能与特定 IP 的路由、DNS 解析结果差异、Docker/宿主机路由或网络限制有关；这些仍是待验证假设，不能写成已确定原因。

暂时搁置 Eastmoney 故障，不因一次/多次接口失败就删除 Provider 或改变质量门禁。恢复原始生产配置后继续验证其余数据源。

### 5.5 后续继续排查时的建议步骤

1. 先确认 `market-data` 容器 DNS 对 `push2his.eastmoney.com` 的当前 A/AAAA 记录，并把时间、每条 IP 与实际连接结果一起记录；不要只保存一次解析结果。
2. 在容器和宿主机分别测试相同的某一个解析 IP，区分容器路由与宿主机路由。一次根路径 404 只能证明该次 TCP/TLS 可达，不证明行情 path 可用。
3. 对真实 K 线接口保留 HTTP 状态与响应体；避免使用会抑制错误正文的静默输出参数。请求使用的日期/复权模式/limit 也必须记录。
4. 查看容器路由（例如 `cat /proc/net/route`）、DNS 配置（`cat /etc/resolv.conf`）和宿主机路由/防火墙策略；若需要，按 IP 固定测试，但不要将测试 IP 永久写进生产配置。
5. 增加 provider 失败结构化日志：至少记录 provider、能力、symbol、错误类别、耗时和 request ID；不要记录 API Key、完整带查询参数的 URL 或响应正文中的敏感字段。
6. 根因明确前，保持 Eastmoney 状态为“未通过/待调查”，不要把网络不可达解释成股票不支持或服务商永久不可用。

## 5.6 Tencent Provider 自动化契约测试边界

Tencent Go Provider 当前单测覆盖成交额缺失时保持 null/unknown/degraded、拒绝把未文档化的 K 线尾字段解释为成交额、普通 A 股成交量按“手”转“股”、科创板样本成交量以“股”处理等规则。本轮新增的测试继续固定三种请求口径：

- `adjustment=none` 应请求不复权 endpoint，并读取 `day`；
- `adjustment=forward_adjusted` 应向调整 endpoint 请求 `qfq`，并读取 `qfqday`；
- `adjustment=backward_adjusted` 应向调整 endpoint 请求 `hfq`，并读取 `hfqday`；
- 若请求的复权数组缺失，即使响应含 `day`，也必须报错，不能将不复权价格冒充调整后价格。

对应自动化用例为 `TestTencentDailyBarsUsesRequestedAdjustmentEndpointAndPayloadKey` 与 `TestTencentDailyBarsRejectsMissingRequestedAdjustedSeries`，使用本地 `httptest` fixture 断言接口、参数、响应字段映射与复权缺失保护。这些测试不是对 Tencent 线上服务可达性、当日数据覆盖或成交额真实性的证明。Tencent 独立单源验收仍待部署环境恢复正常配置后执行。实际测试应选用近期且尚未在 MariaDB 保存的交易日（例如 `2026-10-22`），并用新的 `limit` 避开既有缓存；请求成功必须看到本次新鲜数据的 `source=tencent` 和接近当前请求时间的 `fetched_at`。Tencent 日 K 当前没有经过验证的成交额字段，故 `amount=null` 与 `quality_status=degraded` 是预期限制；如果响应为 `stale` 且带 `provider_unavailable_served_from_mariadb`，只能说明数据库兜底生效，不能判定 Tencent 独立验收通过。

## 5.7 Sina 实时报价契约测试边界

Sina 当前只实现 QuoteProvider，不提供日 K。既有 fixture 测试覆盖原始成交量按“股”保存、人民币成交额解析、时间按中国时区解释后存 UTC，以及不支持交易所后缀拒绝逻辑。本轮补充 `TestSinaMissingOrInvalidAmountDoesNotClaimRawFieldMetadata`，覆盖成交额缺失/非法时保持 `Amount=null`、`RawAmount=null`、单位为 `unknown`、`RawAmountField` 为空并标记 `degraded` 的契约。

这些单测使用本地 `httptest` fixture，只能验证解析规则，不能证明新浪公网接口在 NAS 容器中的实时可用性。真实验收时应单独启用 Sina 报价能力，检查新鲜的 `source=sina` 结果、报价时间、成交量单位和成交额质量标记；不能把 MariaDB 的 `stale` 回退记录视为新浪实时取数成功。

## 5.8 GitHub Actions 真实 Provider 探测方式

新增工作流 `.github/workflows/live-provider-smoke.yml`，在相关 PR 更新时自动执行，也可从 GitHub Actions 页面手动触发。该工作流使用项目实际 Go Provider 访问公开接口，探测新浪/腾讯报价、腾讯不复权与前复权日 K、东方财富报价与前复权日 K、BaoStock 前复权日 K；将结构化解析结果和截断后的原始 HTTP 响应片段输出到 Actions 日志，并上传 `live-provider-report` artifact。报告隐藏完整 URL 查询串，不需要个人 API Key。单次低频探测失败不会阻断常规 CI。

运行结果必须按供应商分别解读：成功请求中包含 `source`、时间、成交量/成交额单位、质量状态与最新几条 K 线；失败则记录 HTTP 状态或网络错误类型。Tencent K 线缺少经验证的成交额时，`amount=null` / `degraded` 是当前已知的数据契约限制，不应直接判定请求失败。

**GitHub Runner 的网络与 NAS 不同**。该工作流可以回答“GitHub 托管环境实际收到了什么”，但不能替代 NAS 网络和真实部署回归。报告中的 HTTP 响应样本有字节数上限，目的是便于快速判断协议/字段，不作为完整上游原始响应存档。

## 6. 下一步验收顺序

1. 用户恢复测试前的 `.env`，重新创建 `market-data` 容器，并复测 BaoStock 正常取数。
2. 在仅启用 Tencent 日 K 的独立配置下验证 `adjustment=none`（并按接口真实能力测试复权请求），检查来源、成交量单位、成交额、日期覆盖和数据质量标记。
3. 独立测试 Sina 报价能力；不将其误认为日 K Provider，因为当前实现中 Sina 仅配置为 QuoteProvider。
4. Eastmoney 保持待调查；等后续具备时间再按第 5.5 节排查。
5. 最后进行完整回归：优先级与故障切换、缓存、MariaDB 写入/回退、质量降级、服务配置恢复和日志关联。

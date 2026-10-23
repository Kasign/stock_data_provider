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

新增工作流 `.github/workflows/live-provider-smoke.yml`，在相关 PR 更新时自动执行，也可从 GitHub Actions 页面手动触发。探测器本身的错误处理也有单测，确保网络错误分类可读、URL 查询串（包括 token 参数）不会进入报告。该工作流使用项目实际 Go Provider 访问公开接口，探测新浪/腾讯报价、腾讯不复权与前复权日 K、东方财富报价与前复权日 K、BaoStock 前复权日 K；将结构化解析结果和截断后的原始 HTTP 响应片段输出到 Actions 日志，并上传 `live-provider-report` artifact。报告隐藏完整 URL 查询串，不需要个人 API Key。单次低频探测失败不会阻断常规 CI。

探测测试除记录真实返回外，还会校验报价的 symbol/source/价格/成交量与缺失成交额元数据，并校验 K 线的标的、来源、请求复权口径、日期范围、重复日期、OHLC 与成交量/成交额质量语义；契约异常会在报告中标记为 invalid。运行结果必须按供应商分别解读：成功请求中包含 `source`、时间、成交量/成交额单位、质量状态与最新几条 K 线；失败则记录 HTTP 状态或网络错误类型。Tencent K 线缺少经验证的成交额时，`amount=null` / `degraded` 是当前已知的数据契约限制，不应直接判定请求失败。

**GitHub Runner 的网络与 NAS 不同**。该工作流可以回答“GitHub 托管环境实际收到了什么”，但不能替代 NAS 网络和真实部署回归。报告中的 HTTP 响应样本有字节数上限，目的是便于快速判断协议/字段，不作为完整上游原始响应存档。

## 5.9 GitHub Runner 首轮真实返回（2026-10-22）

首轮真实探测证实，GitHub 托管 Runner 能访问部分公开接口，但其网络环境与 NAS 不同：

- **新浪报价**：HTTP 200，`Content-Type: application/javascript; charset=GB18030`。数值字段成功解析，价格为 1318.49，成交量为 4,249,133 股，成交额为 CNY 5,576,216,948；股票名称因 GB18030 未正确解码而变成乱码/空值，质量降级。代码现已按响应的 charset 解码，并为 GB18030 建立本地回归测试；真实探测需重跑确认线上结果已改善。
- **腾讯报价**：HTTP 200。价格为 1318.49；原始成交量为 42,491 手，通过成交额/换手信息推断为 4,249,100 股，因此质量仍为 degraded，原因包含 `volume_unit_inferred_from_turnover_metrics`。成交额字段可从高精度 tuple 读取。
- **腾讯日 K**：不复权和前复权请求均 HTTP 200，返回 `day` 与 `qfqday` 数组，样本从 2026-10-12 至 2026-10-21。当前接口没有经验证的日 K 成交额字段，所以 amount 保持 null、质量标记 degraded；不能把未声明的尾字段当成交额。
- **东方财富日 K**：前复权请求 HTTP 200，JSON 的 `rc=0`，返回 19 根日 K，覆盖 2026-09-17 至 2026-10-21；成交量、成交额与 OHLC 成功解析，样本质量为 valid。这证明 GitHub Runner 当时能访问历史行情接口，不证明 NAS 网络已经恢复。
- **东方财富实时报价**：HTTP 302。首次探测未保存 `Location`，尚不能判定是需要跟随的规范重定向、鉴权/反爬中间页还是其他情况。探测器现增加脱敏的重定向目的主机/路径和 query key 名记录，不记录参数值。GitHub Runner 实测重定向到 `push2delay.eastmoney.com/api/qt/stock/get`，因此新增了对该域名的直接报价探测，用于比较主域与延迟行情域的实际响应；在拿到独立请求的 HTTP/JSON/字段结果前，不改生产端重定向策略。
- **BaoStock**：GitHub Runner 登录读取超时，目标地址解析/连接结果为 `114.94.20.42:10030`。NAS 已有成功真实取数证据，不能把 GitHub Runner 的 timeout 推广成 BaoStock 全局不可用。

同时修复了探测代码自身先前的编译/格式问题。GitHub Runner 探测只能证明该 Runner 的观测；上线验收仍需同时保留 NAS 上的真实请求证据。


## 5.10 GitHub Actions 二次真实探测与报告核验（2026-10-22 20:42，Asia/Shanghai）

本轮以目标分支当时的 HEAD `dae377cb2433cd6f3e048eabc1109295e1f5f65e` 运行真实 Provider 探测。报告生成时间为 `2026-10-22T20:42:14+08:00`，查询标的为报价 `600519.SH`、日 K `600036.SH`，日 K 日期窗口为 `2026-09-17` 至 `2026-10-21`。

GitHub Actions 证据：
- 普通 Go CI：运行 [Go test suite #198](https://github.com/Kasign/stock_data_provider/actions/runs/39780490359)，根模块、`go-core`、`services/market-data` 的 gofmt、测试、race detector、vet，以及 Compose 配置校验和两个镜像构建均通过。
- 真实探测：运行 [Live provider probe #7](https://github.com/Kasign/stock_data_provider/actions/runs/39780490231)。探测 job 本身结论为 **failure**，因为代码以非零退出码报告两项真实探测失败；工作流运行整体显示 success 是因为 job 设置了 `continue-on-error: true`，不应解释为全部 Provider 探测通过。
- Artifact：GitHub Actions 已上传并可下载 [live-provider-report（artifact 12427245200）](https://github.com/Kasign/stock_data_provider/actions/runs/39780490231/artifacts/12427245200)。压缩包大小 4,337 字节，包含 23,426 字节的 `live-provider-report.json`；GitHub artifact 元数据记录的压缩包 SHA-256 为 `d47c93431dec34366321e1b727e762b8afa9a0fc3ca076918a3f2edb1723e598`。日志与 artifact 中的结构化结果相互对应。

### 5.10.1 本轮 Runner 观测

| Provider / 能力 | GitHub Runner 真实结果 | 质量与结论 |
|---|---|---|
| Sina 实时报价 | HTTP 200；声明 `charset=GB18030`；名称正确解析为“贵州茅台”；现价 1318.49；成交量 4,249,133 股；成交额 CNY 5,576,216,948 | `quality_status=valid`。此前的名称解码问题在该次实际响应中已修复。 |
| Tencent 实时报价 | HTTP 200；现价 1318.49；原始成交量 42,491 手 | 返回成功但为 `degraded`；成交量单位依据指标推断，且该接口不支持当前契约中的量比。 |
| Tencent 不复权日 K | HTTP 200；返回 8 根 K 线，日期覆盖 2026-10-12 至 2026-10-21 | 请求和解析成功；成交额仍为 null、单位 unknown、质量 degraded。不能将响应中未验证的字段伪装成日 K 成交额。 |
| Tencent 前复权日 K | HTTP 200；返回 8 根 K 线，读取 `qfqday` | 与不复权序列一样，成交额 null/degraded 是当前保守数据契约，不代表 HTTP 或解析失败。 |
| Eastmoney 主报价端点 | HTTP 302，重定向到 `push2delay.eastmoney.com/api/qt/stock/get` | 按当前 Provider 不跟随重定向的策略，结果仍标记失败。报告只记录重定向主机/路径和 query key 名（`_`、`fields`、`fltt`、`invt`、`secid`），没有记录 query value。该证据本身不足以要求改动生产重定向策略。 |
| Eastmoney 延迟行情域直连 | HTTP 200，JSON `rc=0`；解析出“招商银行”、现价 41.82、成交量 98,924,400 股、成交额 CNY 4,140,322,738 | Provider 返回成功，但质量为 degraded，原因是缺少可验证的报价时间戳（`provider_timestamp_missing`）。这证明该次直接请求可取数，不等同于主端点重定向策略已验证。 |
| Eastmoney 前复权日 K | HTTP 200，JSON `rc=0`；19 根 K 线，日期从 2026-09-17 到 2026-10-21 | OHLC、成交量和成交额均解析；样本状态 valid。只证明该次 GitHub Runner 请求成功。 |
| BaoStock 前复权日 K | 向 `114.94.20.42:10030` 登录读取时发生约 20 秒 socket `i/o timeout`，没有拿到数据响应 | **GitHub Runner 网络/读取超时**，不是 HTTP 状态码或行情解析错误，也不能推翻 NAS 端先前通过 TCP 10030 登录并取数的实测。该次报告的 `error_kind=provider_error` 未准确反映底层超时，后续已在提交 `b81a1812080edecb556ec369162fcdb96fc753e4` 中修复超时分类，并在第 5.11 节记录复验结果。 |

### 5.10.2 报告与安全审查说明

- 结构化行情字段（如新浪已解码的名称）是解析结果；HTTP `body_sample` 是从原始响应截取并转成可写入 JSON 的有限长度样本。对 GB18030 非 UTF-8 原始字节，该样本会出现替换字符，因此不能把它当作字节级原始响应存档。报告同时保留响应字节数和 Content-Type，便于解释这一点。
- 捕获器不记录请求 URL 的 query value；重定向仅输出目标 host/path 和排序后的 query 参数名。错误摘要中的 URL query value 同样脱敏。
- 本地 `httptest` 单测证明的是解码、契约、错误分类和脱敏逻辑；GitHub Runner 日志与 artifact 证明的是该时刻托管 Runner 收到的真实响应；NAS 端实测只以 NAS 自己的日志/API/数据库证据为准。三种证据不能互相替代。


## 5.11 GitHub Actions 三次探测与超时分类修复复验（2026-10-22 20:53，Asia/Shanghai）

复验提交：`b81a1812080edecb556ec369162fcdb96fc753e4`（`fix: 正确识别真实探测的网络读取超时`）。该提交增加 `net.Error.Timeout()`、`i/o timeout` 等超时识别，并添加 `TestLiveProbeErrorSummaryClassifiesNetworkTimeout`。最新普通 CI 和真实探测已重新运行。

- 普通 Go CI：[Go test suite #200](https://github.com/Kasign/stock_data_provider/actions/runs/39782109950)，workflow conclusion 为 **success**。根模块、`go-core`、`services/market-data` 中 gofmt、普通测试、race detector、vet 均通过；Compose、部署脚本语法及两个 Go Docker 镜像构建均通过。
- 真实探测：[Live provider probe #9](https://github.com/Kasign/stock_data_provider/actions/runs/39782109968)。workflow 的整体 conclusion 是 success，但 **探测 job 本身为 failure**，因为探测测试报告 3 项失败并返回非零退出码；job 的 `continue-on-error: true` 使工作流整体仍显示 success。
- Artifact：[live-provider-report（artifact 12427112267）](https://github.com/Kasign/stock_data_provider/actions/runs/39782109968/artifacts/12427112267)，压缩包 3,504 字节，GitHub 元数据 SHA-256 为 `2da84dbb03ed5b2ed62ae4fdad58e68fad0f5e5b76cf1bead317c1df38fb0590`。已通过 GitHub Actions artifact 下载并解析内部 `live-provider-report.json`，报告文件大小 19,090 字节，生成时间 `2026-10-22T20:53:50+08:00`。该 JSON SHA-256 为 `66d698fb801d8b35929d3bef692e4da0a4abeb06771595a84a1190ce91a3633c`。

### 5.11.1 修复后真实结果

| Provider / 能力 | 本轮观测 | 结论 |
|---|---|---|
| Sina 报价 | HTTP 200；GB18030 名称正确解析；贵州茅台现价 1318.49、成交量 4,249,133 股、成交额 CNY 5,576,216,948；质量 valid | 编码修复再次通过真实响应验证。 |
| Tencent 报价 | HTTP 200；现价 1318.49；成交量单位仍需指标推断 | 数据返回成功但质量 degraded，原因与成交量单位推断及量比字段契约有关。 |
| Tencent 不复权/前复权日 K | 两个请求均 HTTP 200，各返回 8 根；前复权使用 `qfqday` | 解析成功；成交额保持 null、单位 unknown、质量 degraded，这是已知且刻意保守的数据契约。 |
| Eastmoney 主报价 | HTTP 302，目标为 `push2delay.eastmoney.com/api/qt/stock/get` | 仍按 `http_status` 记失败。没有据此改变生产端重定向策略。只记录重定向 host/path 和 query key 名，不记录 query values。 |
| Eastmoney 延迟行情域直连报价 | HTTP 200；`rc=0`；招商银行现价 41.82、成交量 98,924,400 股、成交额 CNY 4,140,322,738 | 数据返回成功但质量 degraded，原因是缺少可验证的报价时间戳（`provider_timestamp_missing`）。 |
| Eastmoney 前复权日 K | 本轮连接返回 EOF；没有 HTTP 状态、响应正文或可解析 K 线 | 分类为 `connection_closed`，属于传输连接中断，不是解析/质量门禁失败。上一次实测该接口 HTTP 200、19 根且质量 valid；两轮结果说明接口可达性存在波动，不能只挑选成功样本或宣称稳定。 |
| BaoStock 前复权日 K | 对 `114.94.20.42:10030` 登录读取约 20 秒后 `i/o timeout` | 现在正确记录 `error_kind=timeout`。这是 GitHub Runner 的网络读取超时，不是 HTTP 状态或行情解析失败；不否定 NAS 曾成功登录并取数的独立证据。 |

### 5.11.2 分类与验收边界

本轮真实报告将失败分为三类：Eastmoney 主报价是收到 HTTP 302（HTTP 状态/重定向策略问题）；Eastmoney 日 K 是连接后 EOF（传输连接被关闭，未收到 HTTP 状态）；BaoStock 是 socket 读取超时（网络/读取超时）。三者不能笼统归为“接口解析失败”。

截至本轮，自动化契约测试、GitHub Runner 实测和 NAS 运行实测仍为三种独立证据：
1. 自动化测试通过，证明 Go 代码、错误分类、脱敏和质量契约符合测试；
2. GitHub Runner 可重复取得新浪、腾讯、Eastmoney 延迟报价和部分 Eastmoney 历史 K 线，但并非每个端点每轮都成功；
3. BaoStock 的 NAS 成功取数证据属于 NAS 网络环境，本轮 GitHub Runner timeout 不能替代或推翻它。若要再次证明 NAS 当前可用，需要 NAS 侧带时间戳的 API/服务日志或数据库新鲜记录。

不将 Tencent 的 amount=null/degraded 视为错误，不把 Eastmoney 302 直接改成自动跟随，不把任意一次 Eastmoney EOF 推广为整个 Eastmoney 不可用。后续应继续采集独立请求结果与运行环境证据。

## 5.12 BaoStock 协议回归与生产数据契约防护（2026-10-22）

本轮代码提交：[ae451a3](https://github.com/Kasign/stock_data_provider/commit/ae451a3b55d99c2cddf58249b2c8e054b5deb52d)，提交信息为 `fix: 强化 BaoStock 日K数据校验与会话恢复`。

### 5.12.1 本次开发

- BaoStock 日 K 适配器现在拒绝重复交易日期，并验证 `low <= high`、`low <= open <= high`、`low <= close <= high`。违反规则时整批返回明确错误，避免异常记录被标为 `valid`。
- 新增表驱动单测覆盖开盘价超过最高价、最低价超过最高价、收盘价超出日内范围，以及同一交易日重复记录。
- 新增本地 TCP 模拟集成测试，覆盖连续分页直到空末页、上游查询错误码/消息正确返回、连接在查询中途断开后下一次独立调用重新连接并登录、成功查询及登出。
- 这些测试均连接本机临时 TCP listener，不依赖真实 BaoStock 服务或公网可达性，因此可以稳定在 GitHub Actions 中执行。

### 5.12.2 GitHub Actions 验证

- 普通 Go CI：[Go test suite #202](https://github.com/Kasign/stock_data_provider/actions/runs/39784141286) 结论为 **success**。根模块、`go-core`、`services/market-data` 的 gofmt、普通测试、race detector、vet 均通过；Compose 校验、部署脚本语法与两个 Go Docker 镜像构建均通过。
- 真实 Provider 探测：[Live provider probe #11](https://github.com/Kasign/stock_data_provider/actions/runs/39784141342)。工作流整体显示 success，但探测 job 本身为 **failure**，因为它将两项真实探测错误汇总后返回非零退出码；`continue-on-error: true` 只防止这个诊断 job 阻断普通 CI，不代表所有供应商通过。
- Artifact：[live-provider-report（12428630735）](https://github.com/Kasign/stock_data_provider/actions/runs/39784141342/artifacts/12428630735)，压缩包 4,320 字节，GitHub 元数据 SHA-256 为 `c165fd9a3f9599e403177b0ee9e52e01549168a9e0cd4845eeba890f2e04b7d3`。日志和 artifact 的结构化结果一致。

### 5.12.3 本轮真实 Runner 结果

| Provider / 能力 | GitHub Runner 结果 | 解释 |
|---|---|---|
| Sina 报价 | HTTP 200；名称为“贵州茅台”，价格 1318.49；质量 valid | 编码与解析路径成功 |
| Tencent 报价 | HTTP 200；返回价格；质量 degraded | 成交量单位仍依赖指标推断 |
| Tencent 不复权/前复权日 K | HTTP 200，各返回 8 根 | 成交额仍为 null/degraded，符合当前保守契约 |
| Eastmoney 主报价 | HTTP 302，重定向到 `push2delay.eastmoney.com/api/qt/stock/get` | Provider 当前不自动跟随，按 HTTP 状态失败记录 |
| Eastmoney 延迟行情域报价 | HTTP 200，招商银行现价 41.82 | 返回成功但缺少可信时间戳，质量 degraded |
| Eastmoney 前复权日 K | HTTP 200，19 根 | 本轮解析及样本质量 valid；与上一轮 EOF 说明网络结果有波动 |
| BaoStock 日 K | TCP 连接 `public-api.baostock.com:10030` 超时 | GitHub Runner 连接超时，不是协议解析失败；不推翻此前 NAS 真实取数证据 |

本轮关闭的是“自动化协议边界完全没有测试”的缺口；没有关闭 BaoStock 目标网络稳定性、更多日期/证券覆盖、停牌/非交易日行为与 NAS 端持续可用性验收。测试通过与供应商连通是两个独立结论。

## 5.13 多源报价契约、日 K 统一归一化与最新真实探测（2026-10-23）

### 5.13.1 代码改进

- 报价边界：Sina、Tencent、Eastmoney 的报价在返回前调用 `normalize.ValidateQuote`，对关键价格、OHLC、成交量/成交额单位和数值合法性做统一校验。
- 日 K 质量：Tencent 与 Eastmoney 共用 `normalizeProviderDailyBars`，按交易日期升序返回并遵守调用方 limit；过滤重复、范围外、非法 OHLC 和无法解析的行。若返回中夹有坏行，保留结果会标记为 degraded，并写入 `provider_response_contract_warning` 和可适用的 `provider_rows_rejected_during_parse`。
- 安全重定向：Eastmoney 仅跟随 HTTPS 下官方 `*.eastmoney.com` 域名，不跟随外站和 HTTPS 到 HTTP 的降级跳转，最多允许有限次数的跳转。
- BaoStock：原先在显式日期范围时会返回范围内全部交易日，即使调用方的 `limit=8` 也可能返回 19 根。提交 `3e94fdd3` 改为共享排序与 limit 归一化；已加测试保障保留最新 N 根且结果按日期升序，单纯截断不会降级数据质量。

### 5.13.2 代码 CI 验收

- [Go test suite #206](https://github.com/Kasign/stock_data_provider/actions/runs/39885215590) 结论为 **success**。根模块、`go-core` 和 `services/market-data` 的格式、单测、竞态测试、`go vet` 全部通过；Compose 校验及 Go Docker 镜像构建也通过。
- CI #204 曾因腾讯前复权测试样本本身违反 OHLC（收盘价 43.05、最低价 43.10）而失败。修复提交 `f5725e1a` 只将夹具最低价调为 43.00，没有放松生产校验，随后 #205 通过；BaoStock limit 改动的完整验收见 #206。

### 5.13.3 GitHub Runner 真实 Provider 探测 #15

本次 [Live provider probe #15](https://github.com/Kasign/stock_data_provider/actions/runs/39885215595) 的 Provider 探测 job 结论为 **success**，八项请求都返回数据：

| Provider / 能力 | 实测结果 | 质量与解释 |
|---|---|---|
| Sina 报价 | 返回成功，约 883 ms | `valid` |
| Tencent 报价 | 返回成功，约 1,078 ms | `degraded`；保留字段不确定项 |
| Tencent 不复权日 K | 8 根，约 1,256 ms | `degraded`；成交额字段未确认，保持 null/unknown |
| Tencent 前复权日 K | 8 根，约 169 ms | `degraded`；同一成交额契约限制 |
| Eastmoney 主报价 | 返回成功，约 2,029 ms | `degraded`；不因可解析就提升为 valid |
| Eastmoney 延迟域报价 | 返回成功，约 267 ms | `degraded`；缺少可验证报价时间戳时保留降级 |
| Eastmoney 前复权日 K | 8 根，约 837 ms | `valid` |
| BaoStock 前复权日 K | 8 根，约 1,501 ms | `valid`；limit 上限已生效 |

Artifact：[live-provider-report #12472037706](https://github.com/Kasign/stock_data_provider/actions/runs/39885215595/artifacts/12472037706)，4,516 bytes，GitHub 元数据 SHA-256 为 `0d691ca31b6049623caabcd3dca3ab0be2a84bb77df91387539fca50abbce5ff`。

### 5.13.4 结果边界与下一步

- [探测 #14](https://github.com/Kasign/stock_data_provider/actions/runs/39884929219) 的两个 Eastmoney 报价请求曾分别返回 HTTP 502；#15 同一代码线在稍后的窗口里又都成功。此差异应解释为上游/网络端点波动，不能声称重定向代码“修复了 502”，也不能仅凭成功轮次宣称稳定。
- GitHub Runner 结果只证明在该次 Runner 网络环境、该时间窗口中真实请求可达；与模拟测试以及 NAS 容器实测是不同的证据类型。仍需目标容器的带时间戳请求日志完成部署侧验收。
- 下一阶段继续验证 BaoStock 的停牌/无数据日期、更长日期范围及重复/缺失记录；Eastmoney 应持续记录主报价 302/502 和延迟域表现，腾讯成交额仍保持未知，未验证的单位不得猜测。

## 5.14 东方财富 Unix 时间戳兼容与最新验证（2026-10-23）

### 5.14.1 问题与修复

实时响应显示 `f86` 有时为数字型 Unix 秒时间戳（例如 `1792719731`），而非常见的 `YYYYMMDDHHmm` / `YYYYMMDDHHmmss` 字符串。旧解析器无法识别数字时间，因此即使响应包含时间，也会追加 `provider_timestamp_missing` 并将报价降级。

提交 [`619522a6`](https://github.com/Kasign/stock_data_provider/commit/619522a62a69e73b566336e9ec19584be00bb26e) 已支持常见上海本地时间格式、10 位 Unix 秒和 13 位 Unix 毫秒；并拒绝明显早于 2000 年或晚于当前时间 24 小时以上的数字值。回归测试覆盖数字时间完整报价、秒/毫秒解析、合理性边界，以及报价质量门禁。

### 5.14.2 GitHub 自动化验证

- [Go test suite #208](https://github.com/Kasign/stock_data_provider/actions/runs/39885682985) 结论为 **success**：根模块、`go-core` 和 `services/market-data` 的格式检查、单测、竞态测试、`go vet` 全部通过；Compose 校验和 Docker 镜像构建也通过。
- [Live provider probe #17](https://github.com/Kasign/stock_data_provider/actions/runs/39885682979) 的 Provider 探测 job 为 **success**。Artifact：[live-provider-report #12472601632](https://github.com/Kasign/stock_data_provider/actions/runs/39885682979/artifacts/12472601632)，4,476 bytes，SHA-256 `9cea73b2a24595e6fb14f5ebb4fc0b98366101aae2cbe67373aab4ad1a5d01f1`。

### 5.14.3 实时 Provider 结果

| Provider / 能力 | 实测结果 | 质量与解释 |
|---|---|---|
| Sina 报价 | 成功，约 960 ms | `valid`，包含可解析报价时间 |
| Tencent 报价 | 成功，约 1,311 ms | `degraded`；仍保留供应商字段/成交量口径的不确定项 |
| Tencent 不复权日 K | 8 根，约 1,257 ms | `degraded`；未确认成交额字段不做推断 |
| Tencent 前复权日 K | 8 根，约 191 ms | `degraded`；同一成交额契约限制 |
| Eastmoney 主报价 | 第一个端点 HTTP 302，安全跳转至 `push2delay.eastmoney.com` 后 HTTP 200；约 2,087 ms | `valid`；`f86` 数字型 Unix 时间被解析，quote_time 为 `2026-10-23T01:44:47Z` |
| Eastmoney 延迟域报价 | HTTP 200，约 279 ms | `valid`，quote_time 成功解析 |
| Eastmoney 前复权日 K | 8 根，约 991 ms | `valid` |
| BaoStock 前复权日 K | 8 根，约 1,603 ms | `valid`；与请求 `limit=8` 一致 |

### 5.14.4 边界

更早的探测 #14 中两个 Eastmoney 报价入口曾返回 HTTP 502，但 #17 又成功；这说明公共端点存在时段/网络波动，不能声称时间戳修复解决了 502，也不能以一次成功证明稳定性。Tencent 的 degraded 是质量契约状态，不是请求失败；不能为追求 valid 而臆造成交额。GitHub Runner 真实返回仍不替代 NAS 容器端独立验收。

## 5.15 五类已实现 Provider 统一实测矩阵与 MX 密钥阻塞（2026-10-23）

### 5.15.1 统一自动化覆盖

- 已扩展 `TestLiveProviderProbe`，一次运行依次探测新浪报价、腾讯报价/不复权日 K/前复权日 K、东方财富主报价/延迟报价/前复权日 K、BaoStock 前复权日 K，并在一份结构化 JSON 报告中列出每项能力的返回状态、耗时、质量、K 线数量、脱敏 HTTP 响应。
- 日 K 实测契约现在还会检查：交易日期可解析、来源/证券/复权口径一致、日期不超范围、无重复日期、升序排列、数量不超过请求 limit、OHLC 合法、缺少成交额不能被标记为 valid。
- MX_API 的筹码分布和个股资金流也已加入同一个报告的独立能力项。由于 MX 使用独立 Key、请求配额与 300ms 级串行准入，普通 PR 探测默认记录为 skipped；只有手动运行 Live provider probe 并显式设置 `include_mx=true` 才应发出这两次真实请求。已撤销为了诊断而临时增加的 branch-push 触发器，避免后续提交自动消耗 MX 配额。

### 5.15.2 Go CI 与当前真实上游结果

- [Go test suite #212](https://github.com/Kasign/stock_data_provider/actions/runs/39887486551) 结论为 **success**：根模块、`go-core`、`services/market-data` 的测试/race/vet 与 Compose、部署脚本检查及 Docker 镜像构建均通过。
- [Live provider probe #22](https://github.com/Kasign/stock_data_provider/actions/runs/39887647358) 的工作流总体显示 success，但真实 Provider job 为 **failure**（该 job 配置了 `continue-on-error`，工作流绿色不代表所有 Provider 通过）。本轮报告 Artifact：[live-provider-report #12472528946](https://github.com/Kasign/stock_data_provider/actions/runs/39887647358/artifacts/12472528946)，4,277 bytes，GitHub 元数据 SHA-256 为 `61e5a906ba2b6170005b92573a481a2f85d3b1328a6cadc312546ac06095c69f`。
- 本轮真实结果：新浪报价返回 valid；腾讯报价与两种日 K 返回但降级（成交额/字段口径仍未完全验证）；东方财富前复权日 K 返回 8 根有效 K 线；BaoStock 前复权日 K 返回 8 根有效 K 线。东方财富主报价和延迟域报价本轮均遇到 TLS handshake timeout，但较早的 #20/#21 中曾成功；此属于端点/Runner 网络波动，不能因此判定解析器失效，也不能因历史成功声称其稳定。
- MX 的两条报告记录均为 `status=failed`、`error=MX API key is missing`，没有向 MX 端点发出查询。此次以 branch push 触发后仍未从 Actions 环境拿到 `MX_APIKEY`，所以**不能称为妙想线上验证通过**。要完成这一项，需要确认仓库 GitHub Actions Secrets 中存在名为 `MX_APIKEY` 的可用 secret，然后从工作流页面手动执行并勾选 `include_mx`。密钥值不写入代码或日志。
- 先前 [Live provider probe #20](https://github.com/Kasign/stock_data_provider/actions/runs/39887133839) 已将所有八项公开报价/日 K 请求验证为 returned，且东方财富主/延迟报价的 Unix 时间戳已解析为 valid；#22 的超时体现公网调用存在波动，而不是持续性通过/失败保证。

### 5.15.3 接入范围边界

截至本轮，本 Go 服务现有数据能力都已进入统一测试框架：新浪报价、腾讯报价/日 K、东方财富报价/日 K、BaoStock 日 K、妙想筹码/资金流（真实请求要求独立 Key）。Tushare、同花顺、Efinance、AkShare、Pytdx、YFinance 以及新闻公告、财务、板块、龙虎榜等并未都存在本仓库的 Go Adapter；它们仍属于待接入的数据源或数据域，不能只改配置就标为“已接入”。后续按域新增 Adapter 时，应沿用同一结果结构和数据契约测试。

### 5.15.4 恢复常规自动探测后的复验（2026-10-23）

为避免自动消耗 MX API 配额，已撤销临时 branch-push 触发器；当前只有手动运行工作流并勾选 `include_mx` 才会调用妙想接口。统一 Provider 契约测试和日 K 上限测试仍保留在普通 CI。

- [Go test suite #214](https://github.com/Kasign/stock_data_provider/actions/runs/39887930886) 已全部通过：三个 Go 模块的测试/race/vet、Compose 校验、部署脚本和两套 Go 镜像构建均成功。
- [Live provider probe #24](https://github.com/Kasign/stock_data_provider/actions/runs/39887930885) 的真实 Provider job 成功，八项公开行情/日 K 请求均返回：
  - Sina 报价：`valid`，包含已解析的报价时间。
  - Tencent 报价：returned / `degraded`；不复权与前复权日 K 各 8 根 / `degraded`，成交额仍未确认。
  - Eastmoney 主报价：官方重定向从 HTTP 302 到延迟域 HTTP 200，`quote_time` 解析成功，`valid`。
  - Eastmoney 延迟域报价：HTTP 200，`valid` 且含 `quote_time`。
  - Eastmoney 前复权日 K：8 根 / `valid`；BaoStock 前复权日 K：8 根 / `valid`。
- Artifact：[live-provider-report #12473805201](https://github.com/Kasign/stock_data_provider/actions/runs/39887930885/artifacts/12473805201)，4,526 bytes，GitHub 元数据 SHA-256 为 `79109c2c2794b1693abab5657c9a29579c9bd904761bf94b6928db91e35841d8`。
- 同日的一次性 push 探测 [#22](https://github.com/Kasign/stock_data_provider/actions/runs/39887647358) 曾遇到 Eastmoney 两个报价入口的 TLS 握手超时，并显示 MX API key missing；稍后的 #24 Eastmoney 又恢复成功。这是对公共端点波动的再次实证，不应把 #22 解释为持久性代码回归。
- 妙想筹码与资金流尚未完成线上验收：在一次性 push 事件中，环境里没有可用的 `MARKET_DATA_MX_API_KEY`，两项报告都明确记录 `MX API key is missing`，没有对 MX 端点执行查询。需要核对 GitHub Actions Secret `MX_APIKEY` 的配置与该工作流的访问权限，再手动运行并勾选 `include_mx`；没有这一步前不可标记为已验证。

## 5.16 新增 Yahoo Finance 与新浪历史日 K 能力（2026-10-23）

### 5.16.1 代码与契约

- Yahoo Finance 新增原生 Go Chart JSON 适配器，支持沪深 A 股历史日 K 与明确记录的复权参考日；默认关闭，不进入默认生产回退链，原因是端点属于非官方公开接口且缺少可核验的 A 股成交额。
- 新浪从报价 Provider 扩展为报价 + 历史日 K Provider，历史 K 线 endpoint 独立配置为 `MARKET_DATA_SINA_KLINE_URL`。新浪日 K 只接受 `adjustment=none`；请求前/后复权会明确返回不支持，不把原始价格冒充复权序列。
- 新浪与 Yahoo 日 K 都不提供可核验的成交额字段，`amount=null`、`amount_unit=unknown` 并固定降级；新浪 K 线接口不回显证券代码，因此额外记录 `provider_symbol_not_echoed_by_kline_response`。
- 新增 Yahoo / 新浪的 mock JSON、复权与日期边界、符号校验、配置开关测试；统一真实 Provider probe 同时覆盖新浪原始日 K 和 Yahoo 前复权日 K。
- 本轮实现与配置已分别提交：[`3c5f3e8`](https://github.com/Kasign/stock_data_provider/commit/3c5f3e8f573a732b7a58b2a02dba7b7144f44cb8)（Yahoo adapter）、[`2b88b89`](https://github.com/Kasign/stock_data_provider/commit/2b88b892b5cc5af0a11552f7fbb2d102467e917b)（Sina adapter）；随后修复测试 JSON、gofmt 与扩展 provider 后的配置超时预算。

### 5.16.2 Go CI

- [Go test suite #39891606308](https://github.com/Kasign/stock_data_provider/actions/runs/39891606308) 在提交 `393edb896bc14e6a805e218fb5dd7e53c43a111e` 上成功。根模块、`go-core`、`services/market-data` 测试均通过，包含竞态检测、`go vet`、Compose 校验与 Docker 镜像构建。
- 此次过程中 CI 先后捕获测试 JSON 字面量语法错误、三处 `gofmt` 差异和新增日 K Provider 导致旧测试的请求超时预算不足，均已按测试日志修正；最终成功结果对应修正后的提交，而不是中间失败提交。

### 5.16.3 GitHub Runner 实测

最新 [Live provider probe #39891606288](https://github.com/Kasign/stock_data_provider/actions/runs/39891606288) 的工作流总体为 success，但 Provider 探测内部仍记录一次东方财富主报价 TLS handshake timeout；诊断 job 配置 `continue-on-error`，因此必须查看单项记录，不能把整体绿色解释成全部上游请求都成功。Artifact：[live-provider-report #12475072085](https://github.com/Kasign/stock_data_provider/actions/runs/39891606288/artifacts/12475072085)，6,517 bytes，GitHub 元数据 SHA-256 为 `243f7251e1f52cbfb2bc20ff5e64f362eb48694c2ddc2d83bed9cb487af3b377`。

| Provider / 能力 | 实测结果 | 解释 |
|---|---|---|
| 新浪实时行情 | HTTP 200，报价字段和时间可解析，当前样本质量 valid | 此处仍是实时行情端点，不与历史 K 线端点混用 |
| 新浪原始日 K | HTTP 200，返回 8 根；2026-10-20 至 2026-10-22 的成交量分别为 76,094,847、74,818,026、98,924,396 股 | 日期升序与 limit 正常；样本量与 BaoStock 逐日一致，腾讯在换算到股后相差少量整数精度；缺成交额/证券代码回显，固定 degraded |
| Yahoo Finance 前复权日 K | HTTP 200，返回 8 根，成交量与 BaoStock 样本一致 | `forward_adjusted` 及参考日有明确标记；成交额缺失、固定 degraded，默认关闭 |
| Eastmoney 主报价 | 本轮 TLS handshake timeout | 其他 Eastmoney 接口同轮可以返回，属于端点/网络不稳定，不据单轮判定适配器不可用 |
| Eastmoney 延迟报价/前复权日 K | 报价 HTTP 200；前复权日 K 返回 8 根且质量 valid | 同一时间窗口内延迟域可用，仍需持续观察主入口重定向后的网络稳定性 |
| Tencent 日 K | 不复权与前复权各返回 8 根 | 缺成交额，固定 degraded |
| BaoStock 前复权日 K | 返回 8 根且质量 valid | 金额和股数可供交叉校验，GitHub Runner 网络成功不替代部署容器验收 |
| MX_API | skipped / 未请求 | Actions 环境仍无可用 `MX_APIKEY`，筹码和资金流不能标记为线上验证通过 |

### 5.16.4 使用与未完成项

- 新浪日 K 可以给 `adjustment=none` 的请求提供可选回退；对前复权或后复权请求必须让新浪返回不支持并继续回退其他源。
- Yahoo 保持默认关闭，只有在目标 NAS/Docker 环境验证可达并确认消费方接受 `degraded` / amount unknown 后再考虑启用。两项新数据源都不可单独支撑依赖成交额的资金流、量价金额判断。
- 本轮没有完成财务报表、新闻公告、板块/指数广度、龙虎榜等数据域，也没有接入所有路线图候选源；这些仍需按数据域逐项实现 Go Adapter 与来源质量契约。
- GitHub Runner 实测只证明对应时刻的公网响应。目标 NAS 的供应商网络、实际服务路由和持久化端到端仍需单独验收。

## 5.17 网易实时行情 JSONP 与历史 CSV 的独立验收（2026-10-23）

- 网易历史 CSV endpoint `quotes.money.163.com/service/chddata.html` 在 GitHub Runner 连续返回 HTTP 502 + HTML（不是 CSV），因此该日 K 能力不通过真实验收，默认保持关闭。
- 新增独立的网易实时 JSONP 端点 `api.money.126.net/data/feed/{code},money.api`，按 `price/percent/updown/open/high/low/yestclose/volume/turnover/time` 字段实现 QuoteProvider。契约测试覆盖 percent 比例到百分点换算、JSONP callback 包装、证券代码回显、成交量股数、成交额人民币元和供应商快照时间。
- GitHub Actions 实测：Live provider probe #39894680276（提交 `2314710d8e214737500483d70c613cd37a324859`）对实时端点报 `dial tcp: lookup api.money.126.net on 127.0.0.53:53: no such host`；同轮历史 CSV 返回 HTTP 502 + HTML，东方财富历史 K 线出现 EOF。该轮实际 Provider job 失败，不能将 NetEase 报价或历史日 K 标记为已接通。
- 最新常规探测 [Live provider probe #39895301747](https://github.com/Kasign/stock_data_provider/actions/runs/39895301747) 对应提交 `46a59919716ece18c8547da3108acebd22c0e7b1`。实际 Provider job 仍为 failure（工作流因 `continue-on-error` 显示 success），本轮 Yahoo 日 K 返回 8 根且质量 degraded、BaoStock 返回 8 根且质量 valid、Eastmoney 日 K 返回 8 根且质量 valid、Sina 报价与日 K均返回、Tencent 报价与日 K均返回；Eastmoney 主报价及 delay 报价两端点同轮均返回 HTTP 502。
- 本次报告 artifact [live-provider-report #12475823907](https://github.com/Kasign/stock_data_provider/actions/runs/39895301747/artifacts/12475823907)，6,520 bytes，SHA-256 `dc83abfe0d0ebe1c9ec19641caf465fe1332542719a32098b1723e8c1b12b3aa`。NetEase 报价与历史 CSV 在本轮常规探测中都按新策略记录为 `skipped`；先前 #39894680276 的主动实测已确认报价 DNS 失败、历史 CSV HTTP 502。
- 为避免可选候选端点持续污染其他 Provider 的日常健康报告，默认 probe 将 NetEase 两项记录为 `skipped`；仅 workflow_dispatch 勾选 `include_netease=true` 才重试。应用配置仍默认 `MARKET_DATA_NETEASE_ENABLED=false`，历史日 K 能力默认关闭。
- `MARKET_DATA_NETEASE_ENABLED` 默认为 false；启用服务端报价还需设置 `MARKET_DATA_NETEASE_QUOTE_ENABLED=true`。根配置样例保持 `MARKET_DATA_NETEASE_DAILY_BARS_ENABLED=false`，直至 CSV 返回真实数据。

## 5.18 东方财富 F10 股票基础资料与 Profile API（2026-10-23）

- 本轮起点 HEAD：`9a1d49b3ab262d3604214d5655f894eb482c6de0`（`test: 修正股票资料接口契约断言`）。该提交链已引入 StockBasic 服务与 HTTP 路由，但漏提交 `StockBasicProvider`、`model.StockBasic`、`ProviderSettings.StockBasic` 等支撑类型，且 `internal/httpapi/server_test.go` 存在 gofmt 门禁错误，因此不能视为可编译可用。
- 本轮补齐 `StockBasicProvider` 能力接口和响应模型；东方财富通过 `RPT_PCF10_ORG_ISSUEINFO` F10 数据查询简称、上市日期及可用的退市日期字段，严格核对返回证券代码，缺数据和代码不匹配均视为失败。新端点由 `MARKET_DATA_EASTMONEY_STOCK_BASIC_URL` 配置，不与实时报价或日 K 复用端点。能力可用 `MARKET_DATA_EASTMONEY_STOCK_BASIC_ENABLED` 独立开关。
- 新增 mock HTTP 契约测试和真实 Runner probe。Live provider probe [#39901894675](https://github.com/Kasign/stock_data_provider/actions/runs/39901894675) 在提交 `91fb1f8798b1b925f96926336efc61ac2492e1ec` 上实际请求 F10 接口，两次响应均为 HTTP 200，返回 `600036.SH`、名称“招商银行”、上市日期 `2002-04-09`，证券代码与基础资料契约校验通过；该项可以标记为 **GitHub Runner 实测通过**，但尚未代表目标 NAS / Docker 环境通过。
- 同一 HEAD 的 Go CI [#39901894665](https://github.com/Kasign/stock_data_provider/actions/runs/39901894665) 格式门禁通过，之后 `services/market-data/internal/httpapi` 测试编译失败：Profile 测试调用 `service.NewWithCapabilities` 多传了一个 `time.Duration` 参数。本次统一修正参数后重跑；不能将此前 CI 标记为通过。

## 5.19 东方财富行业 / 概念板块排名与实时探测结果（2026-10-23）

- 新增独立 `SectorRankingProvider`，通过 `https://push2.eastmoney.com/api/qt/clist/get` 分别请求行业过滤 `m:90+t:2+f:!50` 与概念过滤 `m:90+t:3+f:!50`，API 为 `GET /api/v1/market/sectors?type=industry|concept&limit=10`。字段包括板块代码/名称、涨跌额与涨跌幅、成交额、上涨/下跌家数和领涨股。成交额单位为人民币元；缺少的字段保留为 null/unknown，不填零。
- GitHub Runner 实测见 [Live provider probe #39905085321](https://github.com/Kasign/stock_data_provider/actions/runs/39905085321)，对应板块代码提交 `f1f62671f0b5939a796279b76b694e5a74f6f2bf`。行业接口 HTTP 302 重定向至 `push2delay.eastmoney.com/api/qt/clist/get` 后 HTTP 200 JSON，返回 10 条、总量 496；样本包括 `BK1347`（纺织鞋类制造，涨幅 5.46%）与 `BK1499`（医美服务，涨幅 4.32%）。概念接口同样 HTTP 302→200，返回 10 条、总量 516；样本包括 `BK1754`（2026三季报首亏，涨幅 4.00%）与 `BK1630`（昨日首板，涨幅 3.68%）。这些数值是该次 Runner 查询时点的快照，不是恒定值。
- 两种板块快照均标记 `quality_status=degraded`、原因 `provider_snapshot_timestamp_not_exposed`：上游未提供可验证的行情数据时间，本地 `fetched_at` 不可冒充供应商快照时间。虽然这轮已证明 CLIST 响应可解析和关键字段可用，但不代表板块行情的时间新鲜度达到盘中交易决策的完整验收标准；`MARKET_DATA_EASTMONEY_SECTOR_RANKINGS_ENABLED=false` 继续作为默认值。
- 同一探测轮中，东方财富个股主报价和延迟报价都返回 HTTP 502 HTML，实际 Provider job 因这两项失败；工作流总状态因 `continue-on-error` 仍显示 success。不可把整条工作流的 success 解读为所有 Provider 能力都通过。
- 同一 HEAD 的 Go CI [#39905085083](https://github.com/Kasign/stock_data_provider/actions/runs/39905085083) 曾因新增 8 个 Go 文件未通过 `gofmt` 门禁，测试步骤没有执行。之后提交 `42e6360` / `59caf3f` 仅修正格式，不改变 Provider 行为；它们对应的全量 Go CI 需要以各自 HEAD 的后续运行记录为准，不继承早先 HEAD 的测试状态。
- 网易真实行情仍是 DNS 失败 / 历史 CSV HTTP 502，所以保持关闭；MX_API 因 GitHub Actions 的 `MX_APIKEY` 仍不可用，保持 `skipped / 未验证`，不得标记接通。

### 最新复验：Go CI 与真实 Provider（2026-10-23）

- 最新完整 Go CI 为 [#39905678476](https://github.com/Kasign/stock_data_provider/actions/runs/39905678476)，对应 HEAD `bbcb34ff44c104a688b00a87176a55e2e4b6009c`。根模块、`go-core`、`services/market-data` 测试均通过，Compose 配置校验、部署脚本语法检查、market-data 与 analysis API 两个 Go 镜像构建均通过。此前 #39905085083、#39905355956、#39905514554 和 #39905617416 的失败均被定位为 `services/market-data/internal/config/config.go` 的 `gofmt` 对齐门禁；已修正后，当前 HEAD 的所有 CI job 均成功。
- 最新真实 Provider probe 为 [#39905678555](https://github.com/Kasign/stock_data_provider/actions/runs/39905678555)，同样对应 HEAD `bbcb34f`。实际运行的 14 条 Provider/能力结果全部为 `returned`，探测 job 本身通过；这仅证明该次 Runner 请求成功，不等同于部署环境或长期 SLA 验收。
- 东方财富 F10 基础资料 HTTP 200，返回 `600036.SH`、招商银行、上市日期 `2002-04-09`，质量 `valid`。行业与概念板块端点均为 HTTP 302 跳转至 `push2delay.eastmoney.com` 后 HTTP 200 JSON：行业返回 10 条、总量 496；概念返回 10 条、总量 516。两者固定 `degraded`，原因是 `provider_snapshot_timestamp_not_exposed`。本次样本包括行业 `BK1295` 其他数字媒体（涨幅 9.07%）以及概念 `BK1754` 2026三季报首亏（涨幅 4.96%）；排名和百分比会随市场时点变化。
- 本次东方财富个股主报价先 HTTP 302 转向延迟行情域，后 HTTP 200，延迟报价直连也 HTTP 200，样本价格均为 41.86、成交额约 32.32 亿元，质量 `valid`。前一轮 #39905085321 的两个报价端点返回 HTTP 502，说明该入口有间歇性波动，不能把单次 502 当成永久不可用。
- 新浪实时报价 HTTP 200 且样本质量 `valid`；新浪日 K、腾讯报价/日 K、Yahoo 报价/日 K 均返回可解析数据。新浪日 K、腾讯相关能力和 Yahoo 按各自字段契约降级；Yahoo 报价继续保持 `amount=null`、`amount_unit=unknown`、`quality=degraded`。东方财富、BaoStock 前复权日 K 本轮各返回 8 根且质量 `valid`。
- 网易两个能力在普通 probe 中继续 `skipped`（之前真实测试已证实 DNS 失败 / CSV HTTP 502）；MX_API 筹码和资金流也仍为 `skipped / 未验证`，因为 Actions 没有可用的 `MX_APIKEY`。这些不会因为当前 probe job 绿色而转为已接通。板块 ranking 的生产开关仍默认关闭，直至完成目标 NAS/Docker 环境验收。

## 5.20 东方财富核心指数实时行情（2026-10-23）

- 新增独立 `IndexQuoteProvider`，请求白名单内的上证指数、深证成指、创业板指、沪深300、上证50、中证500、科创50；指数代码不经过个股 `NormalizeSymbol`，避免 `000001.SH` 按个股规则被误识别成深交所股票。
- 使用东方财富批量指数端点 `/api/qt/ulist.np/get`，按 `f12/f13` 严格匹配指数代码和市场；字段包括最新价、涨跌幅/额、开高低、昨收、成交额与供应商时间字段。缺失成交额保留 `amount=null`、`amount_unit=unknown`。
- [Live provider probe #39906844130](https://github.com/Kasign/stock_data_provider/actions/runs/39906844130) 对该能力的首次真实请求返回 HTTP 302 重定向到 `push2delay.eastmoney.com`，随后 HTTP 200 JSON，并通过白名单代码及核心字段校验，返回 7 个指数。端点的 `f86` 在本次响应里是数值型行情字段而非可解析时间戳，所以快照质量为 `degraded`，原因包含 `provider_snapshot_timestamp_not_exposed`；没有将本地 `fetched_at` 冒充行情时间。该轮其他能力失败导致工作流失败，不能把单一能力通过等同于整条工作流全通过。
- 格式修复提交 `74ea496` 后，完整 Go CI [#39907221645](https://github.com/Kasign/stock_data_provider/actions/runs/39907221645) 的所有 job 通过；对应真实 Provider probe [#39907221646](https://github.com/Kasign/stock_data_provider/actions/runs/39907221646) 也通过。指数能力仍保持独立开关 `MARKET_DATA_EASTMONEY_INDEX_QUOTES_ENABLED=false`，本次通过只证明 GitHub Runner 当时可达，不等于目标 NAS/Docker 网络已验收。
- 当前接口为 `GET /api/v1/market/indices`，使用 15 秒缓存。指数日 K、完整市场广度、官方交易日历和各指数专用回退路径仍未完成；本能力不补齐这些缺口。

## 6. 下一步验收顺序

1. 用户恢复测试前的 `.env`，重新创建 `market-data` 容器，并复测 BaoStock 正常取数。
2. 在仅启用 Tencent 日 K 的独立配置下验证 `adjustment=none`（并按接口真实能力测试复权请求），检查来源、成交量单位、成交额、日期覆盖和数据质量标记。
3. 独立测试新浪报价与历史日 K：报价用 quote endpoint；日 K 仅接受 `adjustment=none`，验证目标容器网络、来源路由和 amount unknown/degraded 处理。
4. Eastmoney 保持待调查；等后续具备时间再按第 5.5 节排查。
5. 最后进行完整回归：优先级与故障切换、缓存、MariaDB 写入/回退、质量降级、服务配置恢复和日志关联。


## 5.21 国内市场广度与涨跌停池（实现已加，真实验收待 CI）

新增独立 MarketBreadthProvider 与 LimitPoolProvider，均只对市场级 API 提供数据，不加入个股报价或日 K 兜底链。

- 市场广度通过东方财富全市场证券列表分页读取并校验证券身份、重复项和完整覆盖；上涨/下跌/平盘/缺失涨跌幅的分类计数必须合计等于证券总数。成交额只有在每条证券记录都有可用金额时才输出总值，否则返回 null 与覆盖数。供应商没有可验证快照时间时质量降级。
- 涨停、跌停、炸板池使用独立 push2ex 端点。响应 qdate 仅作诊断字段，trade_date 保留请求日期；空列表保持 has_data=false、quality_status=unknown，避免把非交易日或供应商数据不可用误判为零事件。
- MARKET_DATA_EASTMONEY_MARKET_BREADTH_ENABLED 与 MARKET_DATA_EASTMONEY_LIMIT_POOLS_ENABLED 默认关闭。真实 Runner 结果及目标 NAS/Docker 验收必须在对应流程执行后补记。


## 5.22 国内市场广度和涨跌停池首次真实探测复核（2026-10-23）

基线 commit `5466ef1c8b8df980502d3993c04049a73963a822` 的 GitHub Actions Live Provider Probe #39912333395 含有两类国内市场实测结果，需与工作流整体结论区分：该工作流的探测 job 实际失败，不能视为全量数据源通过。

- **市场广度**：东方财富 CLIST 首页从 `push2.eastmoney.com` 重定向至 `push2delay.eastmoney.com` 后 HTTP 200，首屏声称证券总数为 5,925；但当时探测器只给全量分页 30 秒，超时前未完成覆盖校验。因此不能把首屏返回或此超时表述为全市场统计已验收。
- **涨停池 / 炸板池**：当次分别返回有明细的池数据（报告的总数为 72 和 13），记录质量仍为 `degraded`，原因是上游没有可验证的原子快照时间。
- **跌停池**：HTTP 200 响应出现 `tc=9` 但 `pool=[]` 的不一致组合。旧逻辑拒绝该响应，没有伪造为零跌停；新修正将对这种非空总数/空明细做一次有界重试，若仍不一致则继续报错。
- **本批修正**：全市场分页改按证券代码排序，降低实时涨跌幅变化造成跨页重复/遗漏的风险；主域 CLIST 连续失败时有限回退到 `push2delay.eastmoney.com`，成功后后续页继续走该域；跨页结果额外标记 `provider_pages_not_atomic`；探测预算放宽到 180 秒；广度缓存延长至 90 秒。跌停池改用 `sort=fund:asc`（该 endpoint 与涨停池的 `fbt:asc` 行为不同），并保留一次有界重试，若仍为 `tc>0/pool=[]` 则继续报错。对应测试同时覆盖主域失败回退、部分成交额、临时 502 重试和持续不完整池响应。

本节记录的是修正前实际观测和本批修正内容，不是修正后的通过声明。新提交的 Go CI、完整 Runner 探测以及目标 NAS/Docker 环境验收仍需分别确认。市场广度和涨跌停池生产开关继续保持关闭。


## 5.23 国内市场第二轮真实探测结果（2026-10-23）

[Go test suite #39915426194](https://github.com/Kasign/stock_data_provider/actions/runs/39915426194) 的根模块和 `go-core` 测试通过；`services/market-data` 的测试与镜像构建被 `domestic_market_test.go` 的 gofmt 格式门禁挡住，因此这一轮尚不能据此认定 market-data 单元测试全部通过。

[Live provider probe #39915426198](https://github.com/Kasign/stock_data_provider/actions/runs/39915426198) 的探测任务实际失败（工作流整体绿色来自 `continue-on-error`）：

- **市场广度**：`push2.eastmoney.com/api/qt/clist/get` 两次都返回 HTTP 502；未取得可完成全市场覆盖校验的快照。
- **涨停池 / 炸板池**：本次返回有明细的池，分别报告总数 82 和 12；仍因无可验证上游快照时间而降级。
- **跌停池**：`getTopicDTPool` 两次都返回 `tc=6,pool=[]`。保守校验正确拒绝该不完整结果，没有返回“0 只跌停”。

随后提交 `6805ceec753bd7eb4302e2af0f1361ec4dc57485` 为 CLIST 增加到 `push2delay.eastmoney.com` 的有限回退，并将跌停池排序独立设置为 `fund:asc`；该提交还在真实探测之前修正了这一文件中的冗余空行。以上修复是否有效，以后续该提交之后的 CI 和实测报告为准，不能沿用旧探测结果宣称通过。生产开关继续关闭。


## 5.24 延迟域回退和跌停池排序实测（2026-10-23）

[Live provider probe #39916080475](https://github.com/Kasign/stock_data_provider/actions/runs/39916080475) 已编译并执行上一提交中的回退逻辑，得到以下实际结果：

- **涨停池**：HTTP 200，报告总数 82，本次按 API 请求限制返回 30 条明细。
- **跌停池**：HTTP 200，返回总数 6、明细 6 条。将 `getTopicDTPool` 的排序参数切换为 `fund:asc` 后，此次不再出现 `tc>0,pool=[]`。
- **炸板池**：HTTP 200，报告总数与返回明细均为 12。
- **市场广度**：首个 `push2` 请求发生 HTTP 502 后回退到 `push2delay`，前几页成功；随后延迟域在分页过程中仍间歇性 HTTP 502，页面 5 的有限重试耗尽，未形成全市场完整快照。因此此能力仍未通过全量验收。
- **其他同时探测的国内能力**：新浪、腾讯报价和日 K、东方财富股票基础资料、行业/概念板块榜、指数报价、东方财富个股报价及 BaoStock 日 K 均有返回记录；东方财富个股日 K 本次发生 TLS handshake timeout。可逐项查看该运行的脱敏结果，不能把某一项失败推广到所有能力，也不能将整份探测标记为完全通过。

市场广度本批继续将瞬时 HTTP 5xx / 超时重试上限从 2 次提高到 3 次；这是有限重试而非无限重试。新的 Go CI 和 Runner 探测仍待当前提交验证。市场广度和涨跌停池生产开关保持关闭。


## 5.25 涨跌停池证券代码归属补正（2026-10-23）

最新真实涨停池探测在返回样本中出现了北交所 `920xxx` 代码（例如 `920285`）。旧归一化仅将 `4xxxxxx/8xxxxxx` 前缀映射到 `.BJ`，因此会错误地把该批较新的北交所代码标为深交所。当前修正将 `920` 前缀也映射到 `.BJ`，并增加真实响应形态的契约回归测试。该修正只涉及证券代码归属，不改变上游返回计数。仍需当前 HEAD 的 Go CI 确认通过；所有新增国内市场开关保持关闭，直到目标 NAS/Docker 联调。

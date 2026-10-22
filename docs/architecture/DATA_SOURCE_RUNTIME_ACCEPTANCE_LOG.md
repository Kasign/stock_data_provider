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

## 6. 下一步验收顺序

1. 用户恢复测试前的 `.env`，重新创建 `market-data` 容器，并复测 BaoStock 正常取数。
2. 在仅启用 Tencent 日 K 的独立配置下验证 `adjustment=none`（并按接口真实能力测试复权请求），检查来源、成交量单位、成交额、日期覆盖和数据质量标记。
3. 独立测试 Sina 报价能力；不将其误认为日 K Provider，因为当前实现中 Sina 仅配置为 QuoteProvider。
4. Eastmoney 保持待调查；等后续具备时间再按第 5.5 节排查。
5. 最后进行完整回归：优先级与故障切换、缓存、MariaDB 写入/回退、质量降级、服务配置恢复和日志关联。

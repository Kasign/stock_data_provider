# 国内 A 股数据层真实黄金样例对账记录

- 采集时间：2026-10-30（UTC）
- Git 提交基线：`986ed9cabf7711884a288efb20044632b624d685`
- GitHub Actions：Live provider probe，Run ID `41018766329`
- 固化夹具：`services/market-data/internal/provider/testdata/live_market_golden_20261030.json`
- 回归测试：`TestLiveGoldenCrossProviderQuoteAndDailyBarsParity`
- 证据类型：真实公网 Provider 请求后的 Go 归一化结果，不是手工构造的模拟行情。
- 范围限制：本记录是 Go 多 Provider 交叉样例，不是 Python/Go 双侧字段等价证明。

## 1. 实时行情：贵州茅台（600519.SH）

| 字段 | 新浪 | 腾讯 | 判断 |
|---|---:|---:|---|
| 最新价 | 1392.05 | 1392.05 | 一致 |
| 涨跌额 | 38.05 | 38.05 | 一致 |
| 涨跌幅 | 2.810192% | 2.810192% | 一致 |
| 开盘 | 1363.00 | 1363.00 | 一致 |
| 最高 | 1393.37 | 1393.37 | 一致 |
| 最低 | 1352.98 | 1352.98 | 一致 |
| 昨收 | 1354.00 | 1354.00 | 一致 |
| 成交量 | 5,992,729 股 | 5,992,700 股 | 相差 29 股 |
| 成交额 | 8,257,381,778 CNY | 8,257,381,778 CNY | 一致 |
| 行情时间 | 15:14:08 | 15:14:01（北京时间） | 相差 7 秒 |
| 质量状态 | valid | degraded | 保留来源差异 |

腾讯成交量原始字段以手为输入，并结合换手指标推断标准 shares；当前质量原因包括 `volume_unit_inferred_from_turnover_metrics` 和 `volume_ratio_unsupported_by_tencent_quote_contract`。因此即使价格与成交额一致，也不能把腾讯报价提升为 valid。

## 2. 日 K：招商银行（600036.SH）

复权口径：`none`。比较 2026-10-27 至 2026-10-29 三个共同交易日。

| 日期 | OHLC（新浪/腾讯） | 新浪成交量（股） | 腾讯成交量（股） | 差值（股） |
|---|---|---:|---:|---:|
| 2026-10-27 | 42.30 / 43.06 / 42.04 / 42.99 | 86,728,303 | 86,728,300 | 3 |
| 2026-10-28 | 42.51 / 42.68 / 41.85 / 42.13 | 107,789,897 | 107,789,900 | 3 |
| 2026-10-29 | 42.50 / 43.03 / 42.09 / 42.94 | 108,749,446 | 108,749,400 | 46 |

三个交易日 OHLC 完全一致，成交量差异在 50 股以内，符合腾讯日 K 返回手数、换算成股后精度取整的观察结果。

两家来源的日 K 均没有经过验证的成交额字段，因此必须保留：
- `amount = null`
- `amount_unit = unknown`
- `quality_status = degraded`

不得用收盘价乘成交量反推成交额，也不得因为两个来源的 OHLC 一致而删除缺失字段质量原因。

## 3. 回归保护

黄金样例测试验证：
1. 同一证券的双源实时价格、涨跌额、涨跌幅、OHLC、昨收和成交额在指定容差内一致。
2. 成交量标准单位均为 shares，允许本次捕获的源端精度差异（报价不超过 100 股、日 K 不超过 50 股）。
3. 保留新浪 valid 与腾讯 degraded 的不同质量等级。
4. 日 K 复权口径固定为 none；成交额必须保持 null/unknown/degraded。
5. 样例包含采集时间、源提交和 Actions Run ID，确保来源可追溯。

该测试只锁定已捕获的这组真实输出，防止归一化契约被无意破坏；不意味着以后每次实时行情都必须等于本次价格。

## 4. 仍未完成的对账

- 尚未取得同一证券/日期下旧 Python `DataFetcherManager` 的原始结果，故本记录不判定 Python 与 Go 等价。
- 尚未覆盖北交所、创业板、科创板、停牌、新股、除权日、非交易日及缺字段边界样本。
- 尚未验证 Tushare、TickFlow、同花顺的真实账号权限和数据字段。
- 东方财富部分实时请求本轮出现 HTTP 502，需另行复测，不通过其他 Provider 成功抵消。
- 当前仅对 `none` 复权样本做交叉比较；前复权、后复权需在相同参考日期与算法下另建样例。

## 5. 验收判定

本组样例判定为：**Go 双 Provider 样例一致（存在可解释成交量精度差异）；字段缺失与质量状态符合预期。**

Go/Python 字段级等价、同花顺真实账号权限和全市场样本覆盖仍为待验收，不能据此宣布阶段五完成。


## 6. 同花顺真实权限验收的执行入口

当前代码已存在 `TestLiveHithinkCapabilities`，本轮补齐 GitHub Actions 手动执行入口：
1. 在仓库 Secrets 设置 `HITHINK_FINANCE_API_KEY`。
2. 手动运行 `Live provider probe` workflow，勾选 `include_hithink`。
3. 检查报告中的每个 capability 状态、返回行数和质量降级原因。
4. 日 K 分别验证 none、forward、backward 三种复权方式。
5. 对 `2001`、`2003`、限流、空结果和字段不完整分别记录，不得因某一接口成功而启用全部能力。

当前仓库没有可读取的账号 Key，因此这次仅补齐可重复的安全验收流程，不代表已完成同花顺真实请求。


## 7. 后续 CI 验收状态补记（2026-10-30）

- Go/Python 质量契约回归 Run `41019948107`：通过。
- Go 测试 Run `41019702930`：根模块与 go-core 的测试/race/vet 通过；market-data 全量测试通过，但 market-data race 在下一提交触发并发取消后中止，market-data vet 未执行。Docker 镜像构建和 MariaDB PIT 集成通过。
- 真实 Provider Run `41019702953`：5 项 Eastmoney 请求失败（stock_search、sector_rankings、quote、备用 quote 为 HTTP 502；daily_bars 为 EOF）。其余成功只代表本次请求返回。
- 同花顺可选测试 `TestLiveHithinkCapabilities` 被跳过，未提供 API Key，也未执行真实账号权限验收。


## 8. 最新复测补记（2026-10-30）

- Live provider Run `41019954185`：4 项失败，均为 Eastmoney HTTP 502（股票搜索、板块排行、主行情、备用行情）；本次日 K EOF 问题没有再次出现。
- 同花顺可选探测仍跳过，不能据此判断账号权限。
- Go/Python 质量契约回归 Run `41019954339` 通过；但它是契约/静态兼容检查，不是 Python/Go 真实数据值差分。
- 最新 Go suite Run `41019954395` 的 market-data race 与 Docker 镜像构建被后续提交触发并发取消；根模块和 go-core test/race/vet、market-data 全量测试、MariaDB PIT 集成已通过。后续需要完整跑完 market-data race/vet 和镜像构建。


## 9. 跨 Provider 日 K 对账契约（2026-10-30）

新增内部比较器 `CompareCrossProviderDailyBars`，用于避免只在单个测试中手写字段循环，当前不暴露 HTTP API。

| 项目 | 对账规则 |
|---|---|
| 证券 | 两侧标准 symbol 必须完全一致 |
| 交易日 | 两侧交易日期集合必须一致；不一致判为 `not_comparable` |
| 复权 | `adjustment_basis` 必须一致；不同口径不能直接比较 |
| 质量 | 两侧必须均为 `valid`；degraded/unknown 不作为基准 |
| OHLC | 单字段绝对差不超过 0.001 |
| 成交量 | 双方必须为 shares；差异不超过 50 股 |
| 成交额 | 仅双方都有值且单位均为 CNY 时比较，差异不超过 1 CNY；否则该字段不可比较，不反推金额 |

结果分为 `matched`、`partial`、`mismatch`、`not_comparable`。其中 `partial` 不能当作完全通过，`not_comparable` 不能当作失败行情，也不能计入通过率。当前 600036.SH 新浪/腾讯日 K 黄金夹具两侧均为 degraded，成交额缺失，因此只证明质量降级被保留，不构成比较器 matched 样例。

新增固定测试覆盖身份/复权前置条件、降级质量拒绝、核心字段一致、成交额缺失、日期集合不一致及 OHLC 超差。真实 Provider 多源复验仍待单独执行；本轮不需要 Key/Token，也没有发起上游请求。


## 10. 比较器接入状态与样例边界（2026-10-30）

- 固定真实夹具 `600036.SH` 已纳入统一比较器调用路径；两侧质量状态均为 degraded，因此预期结果是 `not_comparable`，原因 `quality_status_must_be_valid`。
- 本夹具仍保留真实源数据、成交量差异和成交额 null/unknown；不得修改为 valid 或补算成交额。
- 合成 valid fixture 的 matched/partial/mismatch 仅用于比较器逻辑测试，不属于真实 Provider 数据证据。
- 后续新增创业板、科创板、北交所及停牌、新股、除权除息、非交易日样例时，必须记录原始来源、采集时间、源提交和 workflow Run ID；无真实有效样本时只记录待办，不生成伪造黄金值。


## 11. 真实夹具单位声明补充（2026-10-30）

600036.SH 日 K 两侧的标准成交量单位明确记录为 `shares`。测试转换从来源级 `volume_unit` 读取，不根据 Provider 名称或数值大小推断单位。成交额仍为 null/unknown，质量仍为 degraded。


## 12. 创业板/科创板真实样例归档（Run 41042950111）

- 真实样本：创业板 `300750.SZ`、科创板 `688981.SH`；Sina/Tencent quote 与日 K 已归档。
- Sina quote quality=valid；Tencent quote quality=degraded。两市场日 K 的 Sina/Tencent 均为 degraded，成交额 null、amount_unit=unknown；Sina 另有 symbol echo contract warning。
- 该批次只证明板块样本能取回数据，不证明日 K 跨 Provider 一致。统一比较器的预期为 not_comparable。
- 真实采集来源 SHA `720f2fc685376e29452ce3742ba398ff79089a03`，workflow Run `41042950111`。原始脱敏报告已保留为 Actions artifact。


## 12. 创业板与科创板真实降级样例（Run 41042950111）

- 创业板：`300750.SZ`；科创板：`688981.SH`。
- 采集源：Sina、Tencent；行情快照和日 K 均记录来源版本、源端时间/交易日期、单位、质量状态和降级原因。
- Sina quote 为 valid，Tencent quote 为 degraded（成交量单位推断、volume_ratio 不支持）；两边日 K 均 degraded，成交额缺失且 amount_unit=unknown。Sina 日 K 还存在响应未回显证券代码的契约告警。
- 日 K 两侧单位均为 shares，复权口径 none；真实样本不能作为 matched 基准。
- 固定夹具路径：`services/market-data/internal/provider/testdata/live_market_cross_market_golden_20261030.json`。
- 来源提交：`720f2fc685376e29452ce3742ba398ff79089a03`；Live provider probe Run：`41042950111`。
- 自动化测试将两市场日 K 输入统一比较器，要求 `not_comparable / quality_status_must_be_valid`。这是质量门禁证据，不是跨 Provider 对账通过证据。


## 13. 2026-11-04 Live Provider 样本补充（Run 41770474643）

- 代码基线：`fc113a6f43bf6004b7591b10f0fc65f6b0b8c86a`。
- GitHub Actions：Live provider probe Run `41770474643`。
- 脱敏报告 Artifact：`live-provider-report`，ID `13294771958`。
- 证据类型：托管 Runner 对真实公网 Provider 发起请求，保存规范化结果、HTTP 状态、有限长度脱敏响应片段；不是 Go/Python 双侧差分，也不是数据库生产部署验收。

### 13.1 DailyBar 实际请求

| Provider | Symbol | Adjustment | Bars | Quality |
|---|---|---|---:|---|
| Sina | 600036.SH | none | 8 | degraded |
| Tencent | 600036.SH | none | 8 | degraded |
| Tencent | 600036.SH | forward_adjusted | 8 | degraded |
| Eastmoney | 600036.SH | forward_adjusted | 8 | degraded |
| BaoStock | 600036.SH | forward_adjusted | 8 | degraded |
| Yahoo Finance | 600036.SH | forward_adjusted | 8 | degraded |

主要降级原因：
- Sina：Provider 未提供成交额，且日 K 响应未回显证券代码。
- Tencent：日 K 没有可验证成交额；前复权响应另有参考日期未验证和响应契约告警。
- Eastmoney / BaoStock：前复权参考日期未验证，并有响应契约告警。
- Yahoo：成交额缺失；前复权报告的参考日期为 2026-11-03。

### 13.2 其他市场样本与结论

- 600519.SH Quote：Sina valid；Tencent degraded（成交量单位推断、volume_ratio 不支持）。
- 300750.SZ 与 688981.SH：Sina/Tencent Quote 和 none 日 K 有返回；两市场日 K 均 degraded。
- NetEase：本次状态 skipped，报告记录托管 Runner DNS 失败/历史接口 HTTP 502，opt-in 未启用。
- 妙想和原生新闻搜索：因未配置/启用所需凭据而 skipped；不得将其计为 Provider 成功。
- 本次没有 HFQ 真实请求，没有同一证券/日期下满足 valid 质量门禁的跨 Provider matched 样本，也没有逐日交易状态上下文对账。

**验收判定：** 本次新增真实降级样本和请求证据；不构成 none/qfq/hfq 全矩阵验收，不构成跨 Provider 完全一致证明，不关闭阶段 B。


## 14. 2026-11-04 HFQ 真实样本与探测失败记录（Run 41771896984）

- 代码基线：`444feef6c5ab7b8c1237d990fc9416c7309baa06`。
- Live provider probe Run：`41771896984`；实际探测 Job：`126383286296`（failure）。
- Artifact：`live-provider-report`，ID `13295471518`。
- 证券：招商银行 `600036.SH`；请求区间：2026-09-30 至 2026-11-03；每个 Provider 返回 8 根 HFQ 日 K。

### 14.1 HFQ 返回结果

| Provider | HTTP | Status | Quality | Method | Reference date |
|---|---:|---|---|---|---|
| Tencent | 200 | returned | degraded | `tencent_hfqday` | 未提供 |
| Eastmoney | 200 | returned | valid | `eastmoney_fqt_2` | 未提供 |
| BaoStock | 报告未单列 | returned | valid | `baostock_pct_change` | 未提供 |
| Yahoo Finance | 200 | returned | degraded | `yahoo_adjclose_ratio_rebased_to_start` | 2026-09-30 |

2026-10-30 样本 close：Tencent 230.714、Eastmoney 232.1、BaoStock 250.0752488、Yahoo 43.549999。四家虽然都返回 `backward_adjusted`，但参考基准并未证明一致，不能以同一个 adjustment enum 直接比较价格。

Tencent、Yahoo 的 amount 缺失保持 null/unknown/degraded；Eastmoney、BaoStock 的 quality valid 只说明字段质量规则通过，不等于复权基准或数值准确性通过。

### 14.2 同批次其他失败

Live Probe Job 中 5 项 Eastmoney 能力失败，均为 HTTP 502：
- stock_search（600519.SH）
- sector_rankings（industry）
- sector_rankings（concept）
- quote（600036.SH）
- eastmoney-delay quote（600036.SH）

Run `41771896984` 的 workflow conclusion 为 success，但实际 Job conclusion 为 failure（容错配置）；不得将该 Job 记为通过。

### 14.3 验收结论

本批新增了四家 Provider 的真实 HFQ 请求和返回样本，但尚未证明参考基准一致，也未形成可用于严格 matched 的有效样本。后续需补齐参考日期/因子语义、同基准数值差分、交易上下文真实日期矩阵和 Probe 历史 MariaDB 部署验收。阶段 B 未关闭。


## 15. 跨 Provider 复权基准误比较风险（2026-11-04）

代码审查基线：`88eac9e70bb77dd02d05ed868daf9d6ca41dffa5`。

### 15.1 审查证据

- Provider 的 `DailyBar` 已包含 `AdjustmentBasis`、`AdjustmentMethod`、`AdjustmentRefDate`。
- Probe 生成跨源快照时只复制 `AdjustmentBasis`，没有复制 method/ref date。
- 比较器只判断双方 `AdjustmentBasis` 是否相同，随后比较 OHLC/Volume/Amount。
- 服务层也按相同 adjustment enum 配对，没有额外参考基准门禁。
- 当前测试 matched 样例为 none 口径，尚无 adjusted 不同基准反例测试。

### 15.2 影响范围与当前结论

这是 adjusted 日 K 对账的潜在误判入口，不是已经证实发生的错误匹配。当前 600036.SH HFQ 真实样本仍未形成 matched；此前文档中记录的价格尺度差异不能直接判定为某个 Provider 的算法错误。

### 15.3 后续验收

1. 快照必须完整传递复权方法和参考日期。
2. adjusted 比较必须有可验证的同基准证据；未知参考基准保持 not_comparable。
3. 为同 enum 不同 method、不同/缺失参考日、快照内部元数据不一致增加单测。
4. none 比较现有行为不回退。
5. CI 通过后重新执行受控 HFQ 矩阵，再判断是否能形成严格 matched。

本次只完成代码审查与风险登记，未修改 Provider 或比较器实现；阶段 B 未关闭。


## 2026-11-04 阶段 B：复权基准比较门禁实施记录

本轮针对跨 Provider 日 K 对账的复权口径误放行风险，实施保守门禁：

- `none`（不复权）继续沿用既有比较路径，不要求复权元数据。
- `forward_adjusted` / `backward_adjusted` 必须逐根 K 线提供非空 `AdjustmentMethod` 与 `AdjustmentRefDate`。
- 同一 Provider 快照内所有 K 线的 method/refdate 必须一致；缺失或行间不一致时，结果为 `not_comparable`。
- 两侧复权 method 与 reference date 必须完全一致，才允许进入 OHLC/成交量/成交额数值比较。仅 `AdjustmentBasis` 相同不再构成可比证据。
- Probe 结果保留左右 Provider 的 method/refdate，便于解释被跳过的比较。
- 服务层仍按同证券、同复权枚举配对；比较器统一执行基准门禁，避免服务层与比较器各自采用不同标准。

当前规则有意偏保守：不同 Provider 的方法标识不同，即使其算法可能在经济含义上等价，也会被标记为不可比较。后续如需放宽，必须先建立有来源证据和回归样例的跨 Provider 基准映射；不得直接按 method 名称猜测语义，也不得用共同日期归一化替代绝对基准证明。

本规则仅防止错误 matched，不代表已经证明各 Provider 的复权值彼此正确。真实 Provider 样本仍需分别记录来源算法、参考日期、因子来源及数据质量。CI 通过仅代表代码测试通过，不替代真实数据验收。


### 最新代码验证与 Probe 历史持久化验收（2026-11-04）

代码提交 `b552ab94fdebbccf22f3241765adb080658414d5` 的 Go test suite Run `41788725704` 已全部通过：
- 根 Go module、`go-core`、`services/market-data` 均通过单元测试、race detector 和 vet。
- MariaDB 11.4 migration / PIT / Probe 历史持久化集成 Job 通过。
- Docker Compose 校验及 market-data、analysis API 两个镜像构建通过。
- Go-Python parity Run `41788725699`、WebUI checks Run `41788725677` 通过。

Probe 历史持久化新增真实 MariaDB 回归覆盖：
- 保存包含左右 Provider 的 `AdjustmentMethod`、`AdjustmentRefDate`、`not_comparable` 原因和字段比较证据的完整 Probe Run。
- 从 `market_data_provider_probe_runs.metadata` 读取并反序列化，核对复权方法、参考日期、不可比较原因和 Provider Item 的 `AdjustmentBasis`。
- 关闭数据库连接并重新连接后再次读取，确认复权证据仍完整。
- 当前采用整批 Probe Run JSON 存入既有 `metadata` JSON 列，不需要为左右 Provider 元数据增加独立 SQL 列。

Repository / MariaDB 的独立持久化验收已先行完成；HTTP + MariaDB 联合端到端验收的后续结果见本文对应的验收补充记录。

Live Provider Probe Run `41788725692` 的 workflow conclusion 为 success，但内部 Job `Probe real public provider responses` 实际 failure，不能视为 Provider 全部通过：
- Eastmoney `stock_search`、`sector_rankings/industry`、`sector_rankings/concept` 三项失败，主要为 HTTP 502。
- Eastmoney `market_breadth` 返回数据，但质量为 degraded：行情快照时间未暴露、证券总体范围未与 Security Master 核验、市场成交额请求失败。
- NetEase quote DNS 解析失败、历史 CSV HTTP 502，按 opt-in 规则 skipped。
- 本轮失败属于真实上游可用性 / 数据质量结果，与 Go 代码 CI 分开统计。

复权比较结论保持不变：本轮四家 Provider 的 HFQ 样本不能证明绝对复权基准兼容；方法标识不同或参考基准证据不足时必须保持 `not_comparable`，不得生成 adjusted `matched`。阶段 B 仍未关闭，后续还需补充 HTTP + MariaDB 联合端到端验收，并等待真实 Provider 提供可证明兼容的复权基准。


### HTTP + MariaDB 联合端到端验收（2026-11-04）

提交 `1f6e2d570fb76abc3267f2a37e2d30fa9071fc4d` 新增 `TestProviderProbeHTTPMariaDBEndToEnd`，并将其纳入 Go test suite 的 MariaDB 11.4 集成 Job（Run `41790053418`）：

- 使用两个独立的模拟 DailyBar Provider，分别返回 `eastmoney_fqt_2` 与 `baostock_pct_change`，参考日期相同，构造同为后复权但算法标识不同的样本。
- 通过真实 HTTP Handler 调用 `POST /api/v1/data-sources/probe`，请求 `daily_bars`、`backward_adjusted` 和 `cross_provider` 检查。
- 校验 POST 响应中的 RunID、Persisted、左右 Provider、复权方法、参考日期，以及 `comparison_not_comparable` / `adjustment_method_mismatch` 结果；不得把该样本判定为 matched。
- 关闭首次 MariaDB 连接并重新打开数据库，再通过新的 HTTP Handler 调用 `GET /api/v1/data-sources/probe/history`，按 RunID 验证同一批次及完整复权证据仍可读取。
- 集成测试步骤已通过：MariaDB migration / PIT persistence 和 Probe HTTP + MariaDB end-to-end 两步均为 success。WebUI checks、Go-Python parity 通过；本次 Go test suite 其余 Job 与 Live Provider Probe 在记录时仍运行中，不能提前记为全部通过。

因此，HTTP 序列化、Probe 服务执行、MariaDB 持久化、连接重建后的历史读取这一联合链路已有 CI 端到端证据。该测试使用确定性模拟 Provider，不代表真实上游复权算法已对账成功；真实 Provider 的 adjusted 基准兼容性仍需独立证据。阶段 B 暂不关闭。


## 16. 2026-11-05：600036.SH / 002982.SZ 历史不复权日 K 样本

### 16.1 样本边界与来源

- GitHub Actions Live Provider Probe Run：`41950643180`。
- 交易日期：2025-07-07 至 2025-07-17（含首尾），共 9 个交易日。
- 股票：600036.SH（招商银行）、002982.SZ（湘佳股份）。
- 复权口径：`none`。Provider：Sina、Tencent、Eastmoney、Yahoo。
- 四家 Provider 对两只股票均返回 9 条，日期序列完整一致。

### 16.2 数据字段与质量

| Provider | 两只股票结果 | OHLC | 成交量 | 成交额 | 质量 |
|---|---|---|---|---|---|
| Sina | 各 9 条 | 与其他三家按分一致 | 精确股数 | 缺失 | degraded |
| Tencent | 各 9 条 | 与其他三家按分一致 | 百股粒度 | 缺失 | degraded |
| Eastmoney | 各 9 条 | 与其他三家按分一致 | 百股粒度 | 有值 | valid |
| Yahoo | 各 9 条 | 与其他三家按分一致 | 精确股数 | 缺失 | degraded |

- 600036.SH 与 002982.SZ 的全部 9 个交易日，四家 OHLC 均按 0.01 元精度一致。
- Sina/Yahoo 的成交量值一致；Tencent/Eastmoney 的成交量值一致，但与 Sina/Yahoo 存在小幅股数差异，符合返回精度/粒度不同的表现，当前不把该差异折算成错误。
- Eastmoney 的成交额字段有值，质量标记 valid；Sina、Tencent、Yahoo 没有可验证成交额字段，必须保留 null 与 degraded，不用价格×成交量推算。
- 上述 valid/degraded 是字段质量契约结果，不代表官方基准或绝对准确性已完成验收。

### 16.3 Sina 历史窗口问题与修复

- 首轮请求时 Sina HTTP 200，但返回的是 2026 年近期 K 线；因请求窗口为 2025 年，既有日期校验正确拒绝了该结果。
- 根因是 Sina 端点只接受 `datalen`，不接受结束日期；原条数估算仅覆盖目标窗口长度，无法回溯到历史日期。
- 修正为按目标起始日到当前最新日期估算 `datalen`，并保留 1023 条安全上限；既有归一化逻辑负责按结束日期过滤。回归测试验证历史窗口会请求足够的最新行数。
- 修复后同一 GitHub Actions Run 中，Sina 对两只股票均返回完整 9 条。

### 16.4 当前结论与下一步

- `none` 口径的 OHLC 跨源一致性通过本批样本核验；成交额字段质量仍为 Eastmoney valid、其他三家 degraded。
- 该记录不是交易所官方数据对账，也未覆盖生产 Probe 的跨源比较器、交易日历、证券交易状态和 MariaDB 历史持久化链路。
- 下一步对同样两个窗口执行 qfq/hfq，记录 method/refdate 和源端公司行为证据；不同或缺失绝对基准时保持 not_comparable，不允许仅凭 adjustment enum 相同判 matched。

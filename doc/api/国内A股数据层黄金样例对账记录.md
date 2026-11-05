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


## 17. 2026-11-05：历史窗口 qfq/hfq 与生产 Probe 诊断结果

### 17.1 复权样本来源

- 直接 Provider 报告：GitHub Actions Run `41951184379`。
- 生产 Probe 历史窗口诊断：GitHub Actions Run `41952778969`。
- 股票：600036.SH、002982.SZ；窗口：2025-07-07 至 2025-07-17；每种口径目标 9 个交易日。
- 复权能力矩阵：

| Provider | none | qfq / forward_adjusted | hfq / backward_adjusted | 参考日期 |
|---|---|---|---|---|
| Sina | 9 条 | not_supported | not_supported | 不适用 |
| Tencent | 9 条 | 9 条 | 9 条 | adjusted 未提供可验证值 |
| Eastmoney | 9 条 | 9 条 | 9 条 | 未提供可验证值 |
| Yahoo | 9 条 | 9 条 | 9 条 | qfq=2025-07-17；hfq=2025-07-07（窗口重基准） |

### 17.2 复权方法与比较限制

- Tencent：`tencent_qfqday` / `tencent_hfqday`，没有可验证的 AdjustmentRefDate。
- Eastmoney：`eastmoney_fqt_1` / `eastmoney_fqt_2`，没有可验证的 AdjustmentRefDate；forward 还产生 reference-date-unverified 警告。
- Yahoo：`yahoo_adjclose_ratio_rebased_to_end` / `yahoo_adjclose_ratio_rebased_to_start`。它将 Yahoo adjclose 比率分别重基准到请求窗口末日/首日。
- Sina：只支持 none。adjusted 项由 Probe 标记 not_supported，而非 failed。
- Tencent 与 Yahoo 的 adjusted OHLC 数值尺度存在明显差异，且方法/参考日期没有共同证据；Eastmoney 虽然返回 9 条，但缺少参考日期证据。所有 adjusted 样本均不得互相判定 matched。

### 17.3 生产 Probe 历史窗口结果

- 每只股票 12 个 Provider/adjustment 项：10 项返回 9 条并完成公共校验，2 项 Sina adjusted 明确 not_supported；Provider 失败数为 0。
- Run 状态为 degraded：10 项 degraded、2 项 not_supported。测试未注入交易日历 Provider，因此有 `daily_bar_context_calendar_unavailable`；也未连接 MariaDB，因此 `persisted=false`。
- cross_provider 项均 skipped，原因 `quality_status_must_be_valid`。该状态不代表 mismatch，更不能计为 matched。
- none 口径的原始 Provider 完整样本在第 16 节；直接样本 OHLC 按分一致，但因为多个源成交额字段缺失，整体质量仍 degraded，生产比较器按现有门禁不执行对账。

### 17.4 当前决策

- 保留现有质量门禁，不允许 degraded 样本进入 matched。
- 不猜测 Tencent/Eastmoney 的复权参考日期，不把 Yahoo 的窗口重基准日期等同于源端原始参考日期。
- 下一步查找可验证的源端复权基准/公司行为证据；如果不能证明同一参考基准，继续 not_comparable。


## 17. 2026-11-05：Pytdx 历史窗口复权样本与生产 Probe 持久化

### 17.1 Pytdx 样本

- GitHub Actions Pytdx 专项 Run：`41955146155`。
- 交易窗口：2025-07-07 至 2025-07-17，600036.SH、002982.SZ，各 9 个交易日。
- none：两只股票各 9 条，质量 9/9 valid；OHLC 与 Sina/Tencent/Eastmoney/Yahoo none 样本按分一致。
- qfq：两只股票各 9 条，方法 `pytdx_qfq`，质量 9/9 degraded；当前缺少可验证参考日期/完整因子证据。
- hfq：两只股票各 9 条，方法 `pytdx_hfq`，质量 9/9 valid；但尚未证实其绝对基准与其他源一致。
- Pytdx GBBQ 输出不是公司行为权威数据的替代品。qfq/hfq 不得仅按枚举或方法名判定跨源 matched。

### 17.2 生产 Probe + MariaDB

- Live Provider Probe Run：`41954775830`。
- 两只股票各自 Probe Run 均 `persisted=true`，storage check passed，并通过 ProviderProbeHistory 读回同一 RunID。
- BaoStock 日历已装配并参与窗口校验，但因源端快照时间未暴露而为 degraded；日 K 上下文随之 degraded。
- Eastmoney 本次三个 adjustment 请求均遇到 EOF；其他 Provider 返回的质量状态仍按原规则保留。
- CrossProviderDailyCompare 因 `quality_status_must_be_valid` skipped，没有 matched。


## 18. 2026-11-05：Tushare 权限证据与生产 Probe 复验

### 18.1 Tushare 权限核验

- GitHub Actions Live Provider Probe Run：`41955543025`。
- 对 600036.SH、002982.SZ 的 2025-07-07..2025-07-17 窗口，Tushare `adj_factor` 均返回 code 40203；`dividend` 也均返回 code 40203。
- 当前账号没有这两个接口的访问权限；报告标记 `permission_required`。没有拿到因子或公司行为数据，不得把空响应解释为无事件。
- 当前不再重试，除非账号权限发生变化。

### 18.2 生产 Probe 结果

- 同一 Run 的服务层历史 Probe 步骤成功执行。
- 两只股票 Probe 均持久化到 MariaDB，并通过历史接口读回 RunID。
- 四家 Provider 各返回 9 条；Eastmoney 本轮成功。每只股票 10 项 degraded、2 项 Sina adjusted not_supported、0 failed。
- BaoStock 日历参与上下文检查但 quality degraded；跨源比较按 valid 门禁 skipped。
- 本次服务链路通过不改变数据质量结论，Probe run 状态仍为 degraded。


## 19. 2026-11-05：BaoStock 历史复权因子与日 K 内部一致性

- GitHub Actions Live Provider Probe Run：`41958045930`，BaoStock 专项步骤 success。
- 样本窗口 2025-07-07..2025-07-17，600036.SH、002982.SZ 各 9 个交易日。
- 600036 返回 30 条历史因子，002982 返回 8 条；因子覆盖从 2002-04-09 / 2020-04-24 延续到 2026 年最近事件。
- 对每个交易日的 open/high/low/close 核验：
  - `raw × foreAdjustFactor` 与 BaoStock `adjustflag=2` qfq OHLC 一致；
  - `raw × backAdjustFactor` 与 BaoStock `adjustflag=1` hfq OHLC 一致。
- 两只股票各 9×4×2 个价格字段均未报告 mismatch。600036 的 2025-07-11 事件处于窗口中，事件因子生效日期选择通过。
- 该结论仅证明 BaoStock 自身因子与自身复权 K 线内部一致，不证明与 Pytdx/Tushare 基准相同；不能作为跨源 matched。
- 当前 Tushare adj_factor/dividend 仍权限不足；Pytdx GBBQ 对账继续进行。


### 19.1 自动化验收

- Go test suite Push Run：`41958045915`，全部 5 个 Job success。
- 包含 market-data gofmt/unit/race/vet、MariaDB migration/PIT persistence、根模块与 go-core 测试、Compose/Docker 镜像构建。
- BaoStock Live Provider Probe Run `41958045930` 的专项因子一致性测试 success。


## 20. 2026-11-05：Pytdx GBBQ 与 BaoStock 公司行为事件对账

- GitHub Actions Live Provider Probe Run：`41959062007` success；Go test suite Run：`41959062051` success。
- 2025 年核对 3 个事件，600036.SH 1 个、002982.SZ 2 个。
- 600036.SH 2025-07-11：现金分红 20 元/10 股，Pytdx 理论除权参考价与 BaoStock preclose 均为 46.24 元。
- 002982.SZ 2025-01-14：现金分红 2 元/10 股，理论价与 BaoStock preclose 均为 16.45 元。
- 002982.SZ 2025-05-30：现金分红 1 元/10 股、送转 4 股，理论价 14.885714 元，BaoStock preclose 14.89 元，差 0.004286 元。
- 三个事件的 BaoStock back/forward 累积因子增量与 Pytdx 事件隐含比率均在 0.0005 容差内，preclose 差在 0.01 元容差内。
- 事件级匹配不代表复权后历史 OHLC 可以直接对账：Pytdx v0.0.86 为仿射变换，BaoStock 为累计因子乘法。跨源比较必须保留 method/ref date 一致门禁。


## 21. 2026-11-05：复权参考日期元数据盘点

- BaoStock/Pytdx/Eastmoney/Tencent 当前记录 adjustment method，但未提供 adjustment reference date。
- Tushare qfq 记录本次因子集合最大交易日，hfq 未填 ref date。
- Yahoo qfq/hfq 分别重基准到请求窗口末日/首日，并记录日期。
- 生产比较器对 adjusted basis 要求 method 与 ref date 同时相同；缺失/不一致时 not_comparable。
- 不将请求窗口日期冒充源端参考日期。Pytdx 锚点可从实际加载历史推导，其他 Provider 需要源端证据后再补。


## 22. 2026-11-05：Pytdx 复权参考日期元数据

- Pytdx qfq 的 `AdjustmentRefDate` 取实际加载历史的最新交易日；hfq 取完整历史扫描的最早交易日；none 为空。
- GitHub Actions Live Provider Probe Run `41960406846` success。
- 600036.SH：qfq 2026-11-05，hfq 2002-04-09；002982.SZ：qfq 2026-11-05，hfq 2020-04-24。
- 两只股票 none/qfq/hfq 各 9 条，均 9/9 valid，method/ref-date 断言通过。
- Go test suite Run `41960406782` 全部通过。
- 该元数据描述 Pytdx 自身复权锚点；与 BaoStock 的方法不同，不能据此直接跨源比较 adjusted OHLC。


## 23. 2026-11-05：BaoStock 八只证券真实复权与上市日期对账

- 代码 SHA：`687b49a190e661bed0855ab3f5bfd367bf9bd4c1`；Push Live Provider Probe Run `42017059794` success，BaoStock 专项 `TestLiveBaoStockAdjustmentFactorConsistency` 实际运行并产出 Artifact `live-baostock-adjustment-report`（ID `13409567542`）。
- 请求窗口：因子历史 `1990-01-01` 至探测日；OHLC 主窗口 `2025-07-07` 至 `2025-07-17`。

| 证券 | 因子行数 | 首因子日期 | raw/qfq/hfq 行数 | OHLC mismatch |
|---|---:|---|---:|---:|
| 000001.SZ | 42 | 1991-04-03 | 9/9/9 | 0 |
| 000002.SZ | 46 | 1991-01-29 | 9/9/9 | 0 |
| 000858.SZ | 31 | 1998-04-27 | 9/9/9 | 0 |
| 600000.SH | 27 | 1999-11-10 | 9/9/9 | 0 |
| 600016.SH | 40 | 2000-12-19 | 9/9/9 | 0 |
| 600601.SH | 29 | 1990-12-19 | 9/9/9 | 0 |
| 600036.SH | 30 | 2002-04-09 | 9/9/9 | 0 |
| 002982.SZ | 8 | 2020-04-24 | 9/9/9 | 0 |

- 八只证券首因子日期均与独立上市日期核验一致：000001（公司资料）、000002（巨潮/发行人文件）、000858（公司资料）、600000/600601（上交所公司概况）、600016（公司资料）、600036（发行人上市文件）、002982（深交所上市公告）。上市日期是独立证据，不由 BaoStock 因子推断。
- 旧报告八只 `pre_first_event_status=unavailable`，因为 1990 至首事件前一天 raw 查询为空。结合上市日期证据，这八只均不存在上市交易日前的交易日，适用性应为 `not_applicable`，不能计为 matched，也不能据此证明 fallback 公式。
- 本批仅证明 2025 主窗口 BaoStock 自身 raw × 因子与自身 qfq/hfq OHLC 一致；不证明跨 Provider parity、AvailableAt 或 PIT 可见性。生产 helper 继续 fail-closed。


## 24. 2026-11-05：上市日期分类代码专项重跑

- 代码 SHA：`cce59e8de493813a4b04f7d0bd24211e1e919bb1`；Push Go suite Run `42018395454` success；Push Live Probe Run `42018395554` 外层 success，但 BaoStock 专项测试步骤 failure。
- 新报告 Artifact `13410008325`：600601.SH、600036.SH、002982.SZ 成功取回 29/30/8 条因子，raw/qfq/hfq 各 9 行、OHLC mismatch=0；三只首因子日均与独立上市日期相同，`pre_first_event_status=not_applicable`。
- 000001.SZ、000002.SZ、000858.SZ、600000.SH、600016.SH 登录 BaoStock 时发生 TCP timeout，报告为 `factor_count=0`，属于 `unavailable`，不计验收通过。不得把外层 workflow success 解释为八证券专项通过。
- 八证券主窗口完整真实对账仍以 Run `42017059794` / Artifact `13409567542` 为证据。本次仅补充验证新分类逻辑，不覆盖或替代原完整报告。


## 26. 2026-11-05：BaoStock/Pytdx raw 双源成交量精度重评

- 真实原始样本来自 Pytdx/BaoStock live report Artifact `13410429933`，证券 `600036.SH`、`002982.SZ`，窗口 2025-07-07 至 2025-07-17，各 9 个交易日。原始报告 OHLC 全匹配，成交量和成交额因原始比较器固定容差分别出现 mismatch。
- 按源粒度重新评估成交量：TDX `VOL` 以手为单位（1 手=100 股），BaoStock provider 原始量单位为 shares。600036 最大差 84 股、002982 最大差 96 股，均在 100 股粒度内。新增内部 `VolumeGranularity` 后，成交量字段可判 matched；未更改原始成交量数据。
- 成交额单位均为 CNY，当前 1 CNY 容差保持不变。600036 9/9、002982 8/9 行成交额差异超过 1 CNY；最大差 120.74/7.90 CNY。相对误差虽小，但没有足够源精度证据放宽阈值，双源整体状态仍 mismatch。
- 本轮不是新的完整实时双源重跑；Run `42021978339` 的 BaoStock 请求超时，报告为 not_comparable。新粒度结论来自前次真实原始值离线重评和单测，不计新的实时验收。


## 27. 2026-11-05：TDX/BaoStock 成交额精度复核

- TDX `protocol.Kline.Amount` 使用 `Price`（内部单位毫元），协议解码将原始整数 CNY 转为内部 Price，`Float64()` 还原为整数元；TDX 官方 `AMOUNT` 单位为元。BaoStock adapter 保留原始 amount，单位标记 CNY，响应值存在小数。
- 既有 18 个真实交易日样本中，600036.SH 成交额差最大 120.74 CNY，002982.SZ 最大 7.90 CNY；17/18 行超过 1 CNY 容差。相对差很小（约 3.19e-8/4.13e-8），但没有证据证明差异仅来自格式舍入。
- 保持 1 CNY 绝对容差和 mismatch 判定；不将这批 raw 双源样本标记 matched。待增加更多成交额量级样本并明确供应商聚合口径后再评估阈值。

## 28. 2026-11-05：raw 双源成交额按来源精度重评

- 原始证据：Artifact `13410429933`，生成时间 2026-11-05T11:39:09Z，窗口 2025-07-07 至 2025-07-17；Pytdx 与 BaoStock 各提供 600036.SH、002982.SZ 的 9 个共同交易日。两侧样本 QualityStatus 均 valid，证券身份、日期集合一致。
- 来源精度依据：
  - Pytdx v0.0.86 `protocol/model_kline.go` 将 `getVolume(Float32frombits(...))*1000` 转为 `Price`；`protocol/unit.go` 的 `getVolume` 明确由 float32 解码。绝对误差上界为半个 float32 ULP 加 0.001 CNY 的 Price 截断误差。
  - BaoStock Python API 字段说明定义 amount 精度为小数点后 4 位、单位人民币元，步长 0.0001 CNY。
  - 双边比较容差为两侧绝对误差上界之和：TDX 为半个 float32 ULP + 0.001 CNY，BaoStock 保守取 0.0001 CNY。
- 按当前比较器规则离线重评：
  - 600036.SH：OHLC 9/9、volume 9/9、amount 9/9 matched；成交量最大差 84 股；成交额最大差 120.74 CNY，所在金额量级的动态容差为 256.0011 CNY。
  - 002982.SZ：OHLC 9/9、volume 9/9、amount 9/9 matched；成交量最大差 96 股；成交额最大差 7.90 CNY，所在金额量级的动态容差为 8.0011 CNY。
  - 两只证券各 54/54 字段 matched。原始报告中按固定 50 股/1 CNY 得到的 mismatch 是容差没有表达来源量化精度，并非 OHLC 或交易日数据不一致。
- 结论：该 artifact 可作为 raw 双源精度感知对账的黄金样例。此次是对已保存的真实响应离线重评，不代表 2026-11-05 本轮重新拉取成功；后续仍需在 BaoStock 网络可用时 fresh rerun。复权样本与 PIT 可见性不在本条结论范围内；AvailableAt 未知仍禁止 PIT 使用。

## 29. 2026-11-05：成交额误差上界复核

- 对 Pytdx v0.0.86 解码链路进一步核查：float32 舍入误差不超过半个 ULP，但随后乘以 1000 转为 `Price` 时发生厘单位截断，最大附加误差小于 0.001 CNY；BaoStock 4 位小数精度未声明舍入方式，按完整 0.0001 CNY 保守估计。
- 因此比较器不再对两侧量化步长求平均，而是直接相加两侧绝对误差上界。600036.SH 最大差 120.74 CNY，对应动态容差 256.0011 CNY；002982.SZ 最大差 7.90 CNY，对应动态容差 8.0011 CNY。18 个日样本的字段结论保持不变，两只证券各 54/54 matched。
- 新增 100,000 CNY 小额测试锁定 Pytdx 0.00490625 CNY 的误差上界，避免低金额区间漏计 Price 截断误差。当前代码 SHA `95383503` CI 待验。

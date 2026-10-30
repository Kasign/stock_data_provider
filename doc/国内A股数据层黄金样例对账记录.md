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

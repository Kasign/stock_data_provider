# DATA-002：DailyBar 质量与复权参考基准完成状态

完成日期：2026-11-18  
维护分支：`feat/data`  
代码静态核对基线：`90871455b282d6124d3c25fb9189fbd274894e67`  
历史真实数据证据：见本文第 3 节所列 Runtime Acceptance Log Run / Artifact  
任务状态：DONE（有明确证据边界；非所有 Provider 全量通过）

## 1. 任务目标与关闭原则

核实 DailyBar 质量判定、none/forward/backward 复权语义、交易日期与证券状态边界，并用已有可比真实样本形成可追溯结论。

本任务按证据关闭，不以“所有 Provider 都必须通过”为目标：
- 有真实证据且满足契约：passed。
- 有真实证据但存在可接受的质量限制：degraded。
- 真实数据违反字段/单位契约：failed。
- 口径或锚点无法证明一致：not_comparable。
- 无凭据、无权限、无对应运行证据：not_executed / permission_required。
- 不为填满矩阵而重复请求上游；未知项保留未知，不推断、不回填。

## 2. 子项结论

### DATA-002-A：DailyBar 现有实现与质量链审查 — DONE

已确认 DailyBars、DailyBarsAsOf、Provider Adapter、Normalize、Validation、复权重建、Probe 交易上下文和跨 Provider 比较器的现有实现。没有发现需要重复建设 Normalize/Validator/Reporter/Probe/Freshness 或新增 DailyBar 字段的依据。

普通 DailyBars 不逐日查询证券状态，避免 N+1 请求；Probe 使用有预算的交易上下文诊断。PIT 读取不得复用当前状态快照。

### DATA-002-B：复权方法与参考基准 — DONE（按 Provider 明确边界）

| Provider | 已有证据 | 验收结论 |
|---|---|---|
| BaoStock | adjustflag 3/2/1 口径；8 只证券、每只 9 条 raw/qfq/hfq 样本，OHLC 无 mismatch；因子与自身原生 qfq/hfq 样本一致 | 同源样本 passed；因子 AvailableAt 未知，PIT 不可用；跨 Provider adjusted not_comparable |
| Pytdx | GBBQ 本地复权；qfq 锚点为实际加载历史最新日，hfq 锚点为完整历史最早日；真实样本及锚点断言通过 | Provider 内部语义 passed；与 BaoStock 算法不同，adjusted 历史 OHLC 不跨源比较 |
| Eastmoney | fqt=0/1/2 请求及真实样本可确认；AdjustmentRefDate 未回显 | raw 按质量前置条件比较；adjusted not_comparable |
| Tencent | day/qfqday/hfqday 请求口径及真实样本可确认；AdjustmentRefDate 未回显 | raw 按质量前置条件比较；adjusted not_comparable |
| Hithink | none/forward/backward 请求口径已确认；9 年分段请求没有统一锚点证据；后复权真实响应出现小数股数 | 分段 adjusted 锚点 unverified/not_comparable；后复权该样本 failed（小数股数违反整数股契约），不放宽校验 |
| Tushare | raw daily + adj_factor 的实现路径明确；真实接口 40203 权限阻塞 | permission_required；不得记为真实数据通过 |
| Yahoo | adjclose/close 比率重基准；qfq/hfq 锚点分别为请求返回末日/首日 | 仅在 method、锚点、日期集合和质量前置条件匹配时可比较；成交额缺失样本保持 degraded/partial |
| Sina / NetEase | raw OHLC | none 可按字段质量比较；不支持 adjusted |

关键关闭结论：
1. `AdjustmentRefDate` 表示价格序列参考锚点，不是公司行动生效日；不得用请求 endDate 或因子 OperateDate 伪造锚点。
2. BaoStock 的 OperateDate 是公司行动/因子生效日期，不是首次公开时间；AvailableAt 无证据时 PIT 继续 fail-closed。
3. Pytdx GBBQ 与 BaoStock 2025 年 3 个公司行为事件的参考价和因子增量对账通过，但两者历史价格变换算法不同，不构成 adjusted OHLC 跨源 matched。
4. Hithink 跨 9 年分段请求是否共享同一锚点没有证据，风险标记 unverified；不再扩大静态追查。只有未来出现真实尺度不连续样本时才重新打开。
5. adjusted 跨 Provider 比较继续要求 basis、method、非空且一致 ref date、日期集合及质量状态满足比较器契约。当前缺证据的格子保持 not_comparable，不放宽门禁。

### DATA-002-C：交易日期与证券状态边界 — DONE（样本范围受限）

| 样本/场景 | 证据 | 结论 |
|---|---|---|
| BaoStock 全天停牌/复牌 | 002968.SZ，2025-09-15..2025-09-28 停牌；10 个交易日均无日 K，停牌前/复牌后有日 K；tradestatus=0/1 加强断言通过 | 对应窗口 passed |
| BaoStock 盘中临停 | 603072.SH，2025-01-03 有日 K，tradestatus=1 | 对应样本 passed；日线状态不表达盘中暂停时段 |
| Pytdx 盘中临停 | 603072.SH、301595.SZ、001390.SZ、603400.SH 各指定日期均保留日 K | 对应样本 passed；前三条质量降级原因是窗口首条缺少涨跌幅，不是停牌缺失 |
| Pytdx 全天停牌/复牌 | 002968.SZ 同一停牌窗口无日 K，2025-09-29 复牌日恢复 | 对应窗口 passed |
| Tushare SecurityTradingStatus | 真实接口 40203 权限阻塞 | permission_required；不代表状态 Provider 全面可用 |
| 上市/退市、全市场状态覆盖 | 本轮没有足够的同一 SHA 全量真实样本 | not_executed / unknown，不外推 |

缺失日分类规则维持现有实现：
- 上市日期前：excluded_before_listing。
- 全天停牌：excluded_suspended。
- 盘中临停但仍有日 K：保留日 K；不能把盘中暂停误判为全天缺失。
- 有效交易日历、状态证据完整且确认应交易但缺 K：missing_on_confirmed_trading_day。
- 日历/状态未知、质量非 valid、查询失败或 PIT 可见时间无法证明：degraded/unknown，不计为已确认漏数。

65% 显式区间覆盖率和 80% lookback 是 Service 进入后续处理的门槛，不是质量验收通过线；本任务不调整阈值。

### DATA-002-D：样本矩阵与验收结论 — DONE

- 复用 Runtime Acceptance Log 中的既有 Run、Job、Artifact 和官方公告，不发起新的 Provider 请求。
- 可比真实 raw 样本：600036.SH、002982.SZ，2025-07-07..2025-07-17，BaoStock/Pytdx 原始日 K 对账；两只证券各 9 个交易日，OHLC、成交量、成交额 54/54 字段 matched（按已记录的源精度/粒度误差界限）。
- BaoStock 自身复权：8 只证券 × 9 条窗口样本，raw/qfq/hfq OHLC 无 mismatch；该结论不等于跨源 adjusted 可比。
- Pytdx/BaoStock 公司行为事件：3/3 事件参考价及因子增量在既定容差内；只验事件/参考价，不扩展为历史 adjusted OHLC matched。
- 停牌/临停矩阵按 C 项记录的证券和日期限定范围；不宣称全市场覆盖。
- 未执行、权限阻塞、失败、降级和不可比较项均保留独立状态，不用 CI success 或其他 Provider 的结果代替真实数据验收。

## 3. 证据索引

权威追加式运行账本：`doc/api/provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md`

- 第 47–48 节：Artifact `13410429933`，600036.SH / 002982.SZ raw 双源日 K 离线精度重评；两证券各 54/54 字段 matched。
- 第 61 节：BaoStock 因子与自身 qfq/hfq OHLC 一致性真实样本。
- 第 62 节：Pytdx GBBQ 与 BaoStock 3 个公司行为事件对账。
- 第 63–64 节：各 Provider 复权元数据矩阵及 Pytdx 参考日期真实验证。
- 第 69 节：Hithink 后复权原始成交量小数股数样本。
- 第 70 节：Eastmoney/Tencent adjusted 参考日期缺失的真实响应。
- 第 71 节：Pytdx 官方盘中临停、全天停牌和复牌窗口。
- 第 72 节：BaoStock tradestatus/isST 加强断言真实验收。

运行账本记录的历史代码 SHA、Run/Job 和 Artifact 只支持对应历史样本，不自动继承为当前 HEAD 的 CI 或全量 Provider 通过结论。本次 DATA-002 是证据整理与静态结论归档，没有修改 `internal/marketdata/**`，没有新增 API/字段/表，也没有重新请求 Provider。

## 4. 最终验收状态

| 验收面 | 结论 |
|---|---|
| DailyBar 现有质量链 | passed（静态实现/测试审查） |
| raw 双源真实样本 | passed（限定 2 证券、9 交易日） |
| BaoStock 自身复权一致性 | passed（限定 8 证券、9 交易日 OHLC） |
| Pytdx/BaoStock 公司行为事件 | passed（3 个事件；不是 adjusted OHLC 跨源通过） |
| adjusted 跨 Provider | not_comparable（参考锚点/算法不统一） |
| Hithink backward 样本 | failed（小数股数） |
| Tushare 真实能力 | permission_required / not_executed |
| 停牌/临停 | passed（限定样本），全市场覆盖 unknown |
| 上市/退市全量边界 | not_executed / unknown |
| PIT 因子可见性 | BaoStock AvailableAt 未知，相关 PIT 因子继续 fail-closed |

**最终结论：DATA-002 DONE。** 完成的是 DailyBar 质量规则、复权语义边界和已有真实样本的阶段验收；不是所有 Provider、所有证券、所有复权模式的全量通过。后续只有出现新证据、代码行为变化或明确消费者需求，才重新打开对应子项。

## 5. 后续任务

下一任务按模块总规划执行 DATA-003：Fundamentals PIT 完整性。本任务不在 DATA-002 文档中提前展开 DATA-003 实施。

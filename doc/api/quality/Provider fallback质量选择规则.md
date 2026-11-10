# Provider Fallback 质量选择规则

更新日期：2026-11-10

## 1. 目标与边界

本规则统一 Data Service 在多个 Provider 之间选择结果的质量原则，不引入通用 fallback 引擎，也不改变各数据能力的领域校验、候选比较方式和 Provider 优先级。

- Provider 顺序表达配置优先级，不代表首个响应一定具有最高质量。
- invalid 或契约校验失败的结果不得成为候选。
- unknown 不得冒充有效数据；可保留候选，仅在所有 Provider 均未提供可用质量结果时返回。
- degraded 表示数据有明确限制但仍可使用；保留候选并继续寻找 valid。
- valid 必须满足该能力的完整契约和质量门禁；找到后优先返回。
- stale 按能力语义处理：实时行情不得作为 fresh 结果；历史数据只有在明确标记的兜底路径中才可返回。
- Provider fallback 是来源选择事实，不应单独把数据质量从 valid 降为 degraded。通过备用来源取得的数据仍须独立通过该能力的质量校验。

## 2. 统一选择矩阵

| 结果 | 是否可作为候选 | 是否继续尝试 | 最终返回条件 |
|---|---|---|---|
| 契约失败 / invalid | 否 | 是 | 不返回 |
| stale | 仅允许能力明确规定的历史兜底 | 是 | 返回时必须保留 stale 状态和原因 |
| unknown | 是，按能力比较候选 | 是 | 所有 Provider 耗尽且无 degraded/valid 候选 |
| degraded | 是，按能力比较候选 | 是 | 所有 Provider 耗尽且无 valid 候选 |
| valid | 是 | 否 | 满足能力质量门禁后优先返回 |

候选优先级为：valid > degraded > unknown。同一质量等级内使用能力自己的比较规则；没有可靠比较依据时保留配置顺序靠前的候选，不伪造综合分数。

## 3. 各能力的候选比较规则

| 能力 | 主要质量证据 | 降级候选比较 |
|---|---|---|
| Quote | 行情时间可信度、实时性、关键字段完整性 | 优先时间戳可信且满足实时返回门禁的候选；stale 只能作为明确标记的末级兜底 |
| DailyBars | 复权方法/参考日期、来源版本一致性、日期覆盖、K 线完整性 | 不跨 Provider 拼接；优先语义一致、覆盖更充分且最新交易日更近的整组序列 |
| Fundamentals | 报告期、披露日期、available_at、分区和指标覆盖 | 优先 PIT 证据与有效分区/指标覆盖更完整的候选 |
| Valuations | 请求证券覆盖、指标完整性、快照时间及报告日期证据 | 优先请求证券覆盖更多、可用指标更多且时间证据更完整的候选 |
| IndexQuotes | 指数覆盖、必需字段、源端行情时间 | 优先完整且时间证据可信的快照；不跨来源拼接 |
| MarketBreadth | 全市场覆盖、金额覆盖、行情时间覆盖 | 优先覆盖更完整且聚合时间证据更可信的整组快照 |

## 4. 来源回退与数据质量分离

provider_fallback_used 只记录实际使用了非首选 Provider，不是质量缺陷的充分证据。FallbackFrom、FallbackReason 和来源原因应保留，便于追溯；最终 QualityStatus 仍由返回数据自身的字段、时间、覆盖和语义证据决定。

如果备用 Provider 的结果本身存在缺字段、时间不可信或覆盖不足，应按对应质量规则降级；不能因为它是备用来源而降级，也不能因为它是首选来源而豁免质量检查。

## 5. 缓存规则

- 各能力保留自身基础 TTL，不强行统一为一个时间值。
- valid 可使用该能力正常 TTL。
- degraded 应使用短于正常 TTL 的质量降级 TTL，避免降级结果长期阻止后续高质量刷新。
- unknown 使用短 TTL，并且多 Provider 场景必须确认 fallback 已耗尽后才可作为缓存命中。
- invalid 不缓存。
- stale 历史兜底必须保留 stale 状态；不得作为实时 Quote 的正常缓存命中。


## 6. Bundle 聚合质量规则（任务 C）

Bundle 聚合状态必须保留子数据的质量语义，不得把所有非 valid 状态统一压缩为 degraded。

聚合优先级（从高到低）：

1. invalid：防御性保留为 invalid；正常 Service 路径应在子数据校验阶段拒绝 invalid 数据。
2. stale：只要参与 Bundle 的子数据明确为 stale，Bundle 不得标记为普通 degraded。
3. unknown：子数据状态缺失或明确 unknown 时，Bundle 不得将其当作 valid。
4. degraded：任一子数据为 degraded，或 Bundle 自身存在已定义的质量限制时，至少为 degraded。
5. valid：只有参与聚合的子数据均 valid，且 Bundle 自身质量检查无降级原因时才为 valid。

DailyBars 是 Bundle 的核心历史数据；返回的每根 K 线质量状态均参与聚合。Quote 仅在当前/近实时请求中参与聚合；历史 as_of 请求按时间边界规则排除实时 Quote。已有子数据 QualityReasons 继续保留在子对象中，Bundle Warnings 增加带组件名和状态的聚合提示（例如 daily_bars_quality_stale、realtime_quote_quality_unknown），不复制子对象的全部原因明细。

本规则不改变当前缺失 DailyBars、Quote 不可用、历史调整前视风险等 Bundle 自身 Warning 的业务含义；也不新增独立 QualityReasons 字段。后续若消费者确实需要结构化聚合证据，再以实际消费接口为依据扩展。

## 7. 实施状态

- [x] Fundamentals：先保留 degraded 候选并继续寻找 valid；耗尽后优先返回 degraded，再考虑 unknown。
- [x] Valuations：先保留首个 degraded 候选并继续寻找 valid；耗尽后优先返回 degraded，再考虑 unknown。
- [x] Quote：可信时间戳的 degraded 作为候选继续寻找 valid；stale/不可信时间戳仅作末级兜底；fallback 来源不再单独降级。
- [x] DailyBars：覆盖达标但 degraded 的整组序列继续寻找 valid；覆盖不足仍独立保留；不跨 Provider 拼接；fallback 来源与质量分离。
- [x] IndexQuotes：valid 优先；备用来源不再强制降级，快照级 FallbackFrom/FallbackReason 保留来源追溯；degraded 使用短 TTL。
- [x] MarketBreadth：valid 优先；备用来源不再强制降级，快照级 FallbackFrom/FallbackReason 保留来源追溯；degraded 使用短 TTL。
- [ ] 完成六类能力缓存准入与降级 TTL 交叉核对。

本阶段不引入通用 fallback runtime、统一质量评分或跨 Provider 数据拼接。


## 熔断器故障分类（任务 B）

熔断器只保护 Provider + Capability 的可用性：

| 结果 | 熔断器处理 | 诊断 |
|---|---|---|
| 网络连接失败、超时、连接中断 | Failure | 计入可用性失败连续次数 |
| 响应违反字段、版本、证券、复权契约 | ContractFailure | 独立累计契约失败，不触发可用性熔断 |
| Provider 返回 HTTP 4xx 请求拒绝 | RequestFailure | 独立累计请求失败，不触发可用性熔断 |
| 响应成功但 freshness、coverage 或字段质量降级 | QualityDegraded | 独立累计质量降级，不触发可用性熔断 |
| 调用方取消 | Abandon | 释放半开探测，不记录 Provider 故障 |
| 满足契约和质量门禁 | Success | 重置可用性失败连续次数 |

ContractFailure 与 QualityDegraded 都表示 Provider 已完成响应，会释放半开探测并恢复可用性状态；独立计数保留历史问题，不与可用性失败混算。

本次实施范围：IndexQuotes、MarketBreadth、SectorRankings、Quote、DailyBars。

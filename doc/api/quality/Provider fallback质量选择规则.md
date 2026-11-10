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

## 6. 实施状态

- [x] Fundamentals：先保留 degraded 候选并继续寻找 valid；耗尽后优先返回 degraded，再考虑 unknown。
- [x] Valuations：先保留首个 degraded 候选并继续寻找 valid；耗尽后优先返回 degraded，再考虑 unknown。
- [x] Quote：可信时间戳的 degraded 作为候选继续寻找 valid；stale/不可信时间戳仅作末级兜底；fallback 来源不再单独降级。
- [x] DailyBars：覆盖达标但 degraded 的整组序列继续寻找 valid；覆盖不足仍独立保留；不跨 Provider 拼接；fallback 来源与质量分离。
- [ ] IndexQuotes：保留 valid 优先策略，分离 fallback 来源标记与质量状态。
- [ ] MarketBreadth：保留 valid 优先策略，分离 fallback 来源标记与质量状态。
- [ ] 完成六类能力缓存准入与降级 TTL 交叉核对。

本阶段不引入通用 fallback runtime、统一质量评分或跨 Provider 数据拼接。

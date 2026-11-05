# Data OpenAPI 与 Go JSON 契约修正清单

- 所属治理任务：GOV-004
- 状态：待 Data 分支实施；本文件是可执行 Review 清单，不代表 Data 代码已修复或 Schema 已冻结。
- 规划归属：仅维护于 `feat/design_doc`；实际 OpenAPI、Go Model 与测试修改应由 Data 分支负责人实施。
- 核查对象：`services/market-data/openapi.yaml` 与 HTTP Handler 实际序列化的 Go 响应模型。
- 本轮读取的 OpenAPI 文件 blob SHA：`9a15f7810a07c60887a574d82e1f84d56a8c9964`。该值是文件 blob SHA，不是分支 HEAD SHA。

## 1. 修正原则

1. **以实际 HTTP JSON 输出为准。** OpenAPI 必须描述 Handler 实际返回的 JSON，不为迎合旧 Schema 向 Go Model 添加没有业务来源的字段。
2. **区分必需、可选、可空。**
   - Go 非指针且无 `omitempty`：正常序列化时始终输出，OpenAPI 应列入 `required`，并按实际值域声明 nullable。
   - Go 指针且无 `omitempty`：字段始终输出，但值可为 `null`；OpenAPI 应列入 `required` 并声明 nullable。
   - Go 带 `omitempty`：字段可缺省；即使类型是指针，也不能仅因指针类型就断言字段必然输出。
   - Go `string + omitempty` 只表达缺省/非空字符串，不表达 JSON null；OpenAPI 不应额外声明 nullable。
   - 需对自定义 MarshalJSON、嵌入结构、接口类型、条件性输出和 Handler 包装进行单独核实，不能只凭字段声明机械推断。
3. **禁止把 unknown/missing 转换为零值。** 0、false、空数组与 null/缺省可能有不同业务语义；客户端必须按契约区分。
4. **内联对象与内部模型按真实路由判断。** 不要求每个 Go struct 都有同名 Schema；但每个对外可达 JSON 字段都必须能沿响应 Schema 路径描述。
5. **Schema 的类型、格式、枚举、精度、范围、数组元素、嵌套引用和时间语义都要检查。** 字段名对齐不是契约完成。
6. **避免破坏性重命名。** 本清单先补缺失字段和修正语义；字段重命名、删除或单位变更必须单独评估 API 版本与调用方影响。

## 2. Go JSON 字段存在、OpenAPI 缺失：15 项

以下字段在既有静态对照中确认 Go JSON Tag 存在，但对应对外 Schema 未声明。实施时须再次以 Data 分支最新代码和 Handler 响应路径核实后补齐。

| Go 响应模型 | 缺失 JSON 字段 | 修正要求 |
|---|---|---|
| ProviderProbeCheckResult | `left_adjustment_method` | 补充字段定义、类型、枚举/空值语义 |
| ProviderProbeCheckResult | `right_adjustment_method` | 同上 |
| ProviderProbeCheckResult | `left_adjustment_reference_date` | 日期格式与可选语义 |
| ProviderProbeCheckResult | `right_adjustment_reference_date` | 日期格式与可选语义 |
| SectorRanking | `provider_report_date` | 日期格式及其与服务观察时间的区别 |
| SectorRankingSnapshot | `excluded_non_a_share_count` | 非 A 股排除计数，非负整数 |
| MarketBreadthSnapshot | `excluded_non_a_share_count` | 同上，不能与 missing_count 混用 |
| LimitPoolItem | `seal_money` | 金额单位 CNY、nullable/缺省语义 |
| LimitPoolItem | `max_seal_money` | 金额单位 CNY、nullable/缺省语义 |
| FundamentalSection | `available_at` | 仅在来源可证明时输出，不得从公告/采集时间推导 |
| IndexConstituent | `effective_from` | 业务有效起始日期 |
| IndexConstituent | `effective_to` | 业务有效结束日期，开放区间语义明确 |
| IndexConstituent | `available_at` | 来源可获得时间，不等同于 system_observed_at |
| IndexConstituent | `system_observed_at` | 系统观察时间，RFC3339 带时区 |
| SourceStatus | `market_capital_flow_capable` | 明确能力声明的 bool/unknown 表达，不能把未声明解释为 false |

### 2.1 字段补齐的附加要求

- 先定位每个字段所属的实际响应 Schema 与 Handler；若对象通过内联结构或数组元素暴露，应补在实际引用路径，而不是随意创建同名孤立 Schema。
- 日期字段采用 `YYYY-MM-DD`；时间戳采用 RFC3339 带时区；不要把日期与时间戳混用。
- 金额字段必须标注 CNY；比例/百分比继续遵循 Data 既有百分点约定。
- 对能力声明、覆盖统计和来源时间，说明“未提供”与 false/0 的区别。
- `available_at` 只能在来源契约或可验证证据支持时提供；不能用 `published_at`、`fetched_at`、`normalized_at` 或 `system_observed_at` 代填。

## 3. Required 语义不一致：15 项

既有静态核对发现以下差异。最终修改必须对照当前 Go JSON Tag、实际 Marshal 行为和 Handler 序列化结果逐项确认。

### 3.1 Go 始终输出，OpenAPI 未列 required：11 项

| Schema | 字段 | 修正要求 |
|---|---|---|
| StockAnnouncementSnapshot | `source_version` | 列入 required |
| DragonTigerSnapshot | `source_version` | 列入 required |
| MarketSentimentSnapshot | `limit_up_count` | 列入 required |
| MarketSentimentSnapshot | `limit_down_count` | 列入 required |
| MarketSentimentSnapshot | `broken_board_count` | 列入 required |
| Quote | `change` | 列入 required；若 Go 为非 omitempty 指针，另声明 nullable |
| Quote | `change_percent` | 同上 |
| Quote | `amount` | 同上 |
| DailyBar | `amount` | 列入 required；若 Go 为非 omitempty 指针，另声明 nullable |
| DailyBar | `change_percent` | 同上 |
| MarketBreadthSnapshot | `total_amount_cny` | 列入 required；若 Go 为非 omitempty 指针，另声明 nullable |
| SourceStatus | `historical_index_constituents_capable` | 列入 required |

> 数量说明：以上清单按字段逐项计数为 12 项；既有 Review 摘要曾记录为 15 处 required 不一致。实施前必须用自动化核对重新生成完整差异，不得为了维持旧数字而漏项或重复计数。若原始 15 项包含未列出的字段，应在本表补充其 Go Tag、OpenAPI required 和实际 JSON 样例证据。

### 3.2 Go omitempty，OpenAPI 却列入 required：3 项

| Schema | 字段 | 修正要求 |
|---|---|---|
| Bundle | `warnings` | 从 required 移除；存在时仍按实际数组类型校验 |
| HotRankingItem | `source_version` | 从 required 移除 |
| HotRankingSnapshot | `source_version` | 从 required 移除 |

### 3.3 Required 验收要求

- 必须以 JSON 序列化输出断言 required，而非只检查 Go 结构体声明。
- 对非指针零值（0、false、空字符串）验证字段仍输出。
- 对非 omitempty 指针 nil 验证字段输出 null，而不是被省略。
- 对 omitempty 字段分别验证有值输出、零值/空值按 Go 编码规则省略。
- 上表数量存在既有记录与逐项清单不一致，必须由自动化差异报告给出最终准确数量后，才能标记本项完成。

## 4. Nullable 语义不准确：11 项候选修正

既有 Review 记录“10 处 nullable 不准确”，并另外发现 `LimitPoolItem.change_percent` 应补 nullable。当前可见的字段清单共 11 项，应按下列规则重新核实并形成最终报告。

### 4.1 Go string + omitempty，Schema 不应 nullable：10 项

| Schema | 字段 | 修正要求 |
|---|---|---|
| DragonTigerItem | `provider_amount_unit` | 移除 nullable |
| DragonTigerSnapshot | `provider_report_date` | 移除 nullable |
| StockBasic | `industry` | 移除 nullable |
| StockBasic | `board_code` | 移除 nullable |
| LimitPoolSnapshot | `provider_report_date` | 移除 nullable |
| HotRankingItem | `name` | 移除 nullable |
| HotRankingItem | `rank_trend` | 移除 nullable |
| ChipDistribution | `date` | 移除 nullable |
| ChipDistribution | `provider_date` | 移除 nullable |

> 上述逐项记录实际为 9 个字段，不是 10 个。第 10 项须由自动化差异报告从最新 Go Tag 与 OpenAPI 中找出后补录，不能猜测字段名称。

### 4.2 Go 非 omitempty 指针，Schema 应 nullable：1 项

| Schema | 字段 | 修正要求 |
|---|---|---|
| LimitPoolItem | `change_percent` | 保留 required（若 Go 无 omitempty），并增加 nullable |

### 4.3 Nullable 验收要求

- OpenAPI 3.0 使用 `nullable: true` 时，必须有 Go JSON 输出 null 的真实路径。
- `omitempty` 指针字段通常表达“缺省或有值”，不能自动映射成 required nullable。
- `string + omitempty` 不允许 JSON null；若业务确实需要三态，应改 Go 类型/序列化契约并增加专门测试，不能只改 Schema。
- 数值指针的 nil 必须保持 null 或缺省的真实行为，不可由 Adapter 转成 0。

## 5. OpenAPI 多余字段：9 项

以下字段存在于 OpenAPI 同名 Schema，但对应 Go 响应模型没有这些字段；应确认是否由 Handler 包装、嵌入结构或其他自定义序列化逻辑产生。若无实际 JSON 来源，应从 Schema 删除，而不是向 Go Model 添加虚构字段。

| Schema | 多余字段 | Review 结论 |
|---|---|---|
| Quote | `validation_total_count` | 当前 Handler 直接序列化 model.Quote，模型无此字段 |
| Quote | `validation_error_count` | 同上 |
| Quote | `validation_warning_count` | 同上 |
| Quote | `validation_info_count` | 同上 |
| Quote | `validation_truncated` | 同上 |
| Quote | `validation_issues` | 同上 |
| StockBoard | `change_percent_5d` | Go model.StockBoard 未定义 |
| StockBoard | `provider_report_date` | Go model.StockBoard 未定义 |
| PriceLimitEventEvidence | `adjustment_basis` | Go 模型未定义 |

如果业务确实需要这些信息，应由明确的 API 响应 DTO/Handler 聚合对象提供，并有字段来源、计算逻辑、时间语义和测试；不能仅在 OpenAPI 中声明。

## 6. Enum、单位与嵌套对象复核

字段名对照之外，还需补以下检查：

1. `AdjustmentBasis` 与 `RequestAdjustmentBasis`：核对请求枚举与响应枚举的语义和值域。请求值不能包含仅用于响应描述的 `unknown`，除非 Handler 明确接受；响应值也不能因请求枚举较窄而丢失真实状态。
2. `[]AdjustmentBasis` 等数组类型：逐项核对 Go 枚举值、OpenAPI item schema 和示例。
3. 所有日期、时间戳、价格、成交量、成交额、百分比：分别核对格式、单位、百分点/比例定义、精度和 nullable。
4. 所有响应内联对象：验证数组 item、嵌套对象、`allOf/oneOf/anyOf` 引用链能到达真实 JSON 字段。
5. 所有枚举：从 Go 常量或验证逻辑生成测试样例，检查 Schema 不遗漏合法值、不接受无效值。
6. 所有字段约束：核对 min/max、长度、pattern、分页上限、时间窗和排序限制与 Handler 校验逻辑一致。
7. 对 `unknown`、`not_supported`、`skipped`、`failed` 等状态保留各自语义，不得合并为 bool 或通用健康状态。

## 7. 自动化双向契约测试

必须建立自动化测试，避免 Go DTO 与 OpenAPI 再次漂移。测试应放在 Data Module 内，由 Data 分支实施。

### 7.1 Go → OpenAPI：输出字段覆盖

- 对每个实际 HTTP 响应 DTO，反射读取 JSON Tag，处理 `-`、`omitempty`、嵌入结构和自定义 MarshalJSON。
- 对实际路由的响应 Schema 做递归展开，比较 Go 可输出字段与 OpenAPI 声明字段。
- 对每个 Go 可输出字段，必须在 OpenAPI 对应响应 Schema 路径中找到声明。
- 例外必须有明确 allowlist、原因、责任人和到期条件；不能用宽泛忽略规则掩盖差异。
- 不能把内部模型、未暴露 DTO 或测试夹具误判为 API 字段。

### 7.2 OpenAPI → Go：禁止虚构字段

- 对每个实际响应 Schema 的字段，确认其来自 Go JSON 输出、Handler 包装 DTO 或明确的自定义序列化逻辑。
- 对无法映射的字段报错，除非有带理由的 allowlist。
- 不能仅按 Schema 名称匹配；应沿 Operation 的 response content/schema/ref 路径解析实际输出对象。

### 7.3 JSON 序列化语义

至少为下列情况建立 golden/fixture 测试：

- 非指针字段为 0、false、空数组时仍输出；
- 非 omitempty 指针为 nil 时输出 null；
- omitempty 字段在零值时省略；
- omitempty 指针在 nil 时省略；
- nullable 字段既验证 null，也验证正常值；
- required 字段在所有合法响应样例中存在；
- 可选字段在缺省时不违反 Schema；
- 时间、日期、单位、枚举值符合 Schema 约束。

### 7.4 工具和 CI 门禁

- 选定并固定 OpenAPI parser/validator 版本。
- Go 测试从仓库内 OpenAPI 文件读取，不访问外网。
- CI 必须运行契约测试；测试失败阻止合并。
- 契约测试需输出清晰差异：Operation、Schema 路径、字段名、Go JSON Tag、Go 类型、OpenAPI type/format/required/nullable。
- 对自定义序列化无法自动反射的部分，使用真实 Handler JSON golden 测试补足。
- 变更 OpenAPI 或响应 DTO 时，必须同一提交更新契约测试和示例。

## 8. 实施顺序与交付物

建议 Data 分支按一个完整功能颗粒度实施，不要拆成每几个字段一个提交：

1. **Schema 字段对齐**：补齐 15 个缺失字段；移除 9 个无 Go 来源的字段；逐项处理枚举和嵌套引用。
2. **序列化语义对齐**：修正 required/nullable；先由脚本/测试生成最终差异清单，解决本文件记录数量不一致。
3. **自动化测试**：增加双向字段映射、required/nullable、枚举和真实 JSON golden 测试。
4. **完整验证**：OpenAPI lint/validate、Go 单测、Handler 响应测试、契约测试、全量 Data Module 测试。
5. **Review 证据**：提交 SHA、变更文件、差异归零报告、CI 结果、未解决例外列表。

## 9. 验收门禁

只有全部满足才能将 Data OpenAPI 契约标记为可冻结：

- [ ] 所有真实 API 响应字段均有 OpenAPI 声明，无无来源的 Schema 字段。
- [ ] Go JSON Tag 与 OpenAPI 的 required/optional/nullable 一致。
- [ ] 15 个已知缺失字段全部补齐，且没有重复或错误归属。
- [ ] 15 处 required 不一致经自动化重算后归零；最终数量与差异报告一致。
- [ ] 10 处 nullable 不准确经自动化重算后归零；额外发现的 nullable 缺口也已修正。
- [ ] 9 个已知多余字段删除或由真实 Handler DTO 提供并有测试证明。
- [ ] 枚举、单位、时间、格式、范围、嵌套引用完成核对。
- [ ] 双向契约测试纳入 CI，新增字段无法绕过测试。
- [ ] OpenAPI lint、Go 测试、Handler JSON 测试和 CI 均通过。
- [ ] 所有例外都有明确理由、影响和责任归属。

## 10. 当前数量矛盾与待核验说明

既有 Review 摘要中的数量不能直接当作最终基线：

- Required：摘要记为 15 处；当前逐项列出的字段为 12 个 Go 始终输出字段 + 3 个 Go omitempty 字段，共 15 项。第一组的 12 项均应在实际 JSON 输出验证后列入 required。
- Nullable：摘要记为 10 处错误；当前逐项列出的 Go string 不应 nullable 字段为 9 项，另有 1 个应增加 nullable 的字段。必须补出缺失的第 10 个错误项，或修正摘要计数。
- 缺失字段：按当前列表为 15 项；执行时逐字段核验实际 API 可达性。
- 多余字段：按当前列表为 9 项。

因此，**当前不能宣称上述字段级差异已完全穷尽**。本文件先把已知项显式化，并要求 Data 分支通过自动化工具重新生成最终差异报告。只有报告与人工 Review 对齐后，才允许冻结。


## 追加：Data 最新分支快照复核（2026-11-05）

- 当前 Data HEAD：`fd39c663c4ad59836acdf963d5fb8b2f9227abe8`，提交 `docs(market-data): 记录 BaoStock 首事件前探测结果`。
- 相较上次 Schema 核查基线 `4732f14d8998bcd06ffb1118846535b626865b2d`，compare ahead 7 / behind 0。
- Compare 变化文件仅为两份 `doc/api/**` 文档和 `services/market-data/internal/baostock/live_adjustment_probe_test.go`；没有修改 `services/market-data/openapi.yaml` 或 `internal/model/**`。
- 因此本清单既有 OpenAPI ↔ Go JSON 字段、required/nullable 和多余字段差异仍未被本次提交修复；此前字段级结论在该 SHA 上仍适用。

本次新增的 BaoStock 实测说明两只证券首个因子事件日前没有 raw 日 K，`pre_first_event_status=not_applicable`，所以没有验证 latest-factor fallback；生产逻辑继续 fail closed，不采用未来因子。因子 `AvailableAt` 仍未知，当前 qfq/hfq OHLC 匹配不能证明历史 PIT 可见性。该实测是复权重建样本证据，不应被误读为 OpenAPI Schema 修复或 Data 全量 PIT 验收。

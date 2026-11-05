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
- [ ] Go `omitempty` 与 OpenAPI nullable 的 122 处宽松声明完成复核；Go 可输出 null 但 Schema 未允许的 23 处字段完成修正或由 Handler 非空约束和 JSON 测试证明。
- [ ] 9 个已知多余字段删除或由真实 Handler DTO 提供并有测试证明。
- [ ] 枚举、单位、时间、格式、范围、嵌套引用完成核对。
- [ ] 双向契约测试纳入 CI，新增字段无法绕过测试。
- [ ] OpenAPI lint、Go 测试、Handler JSON 测试和 CI 均通过。
- [ ] 所有例外都有明确理由、影响和责任归属。

## 10. 当前数量矛盾与待核验说明

既有 Review 摘要中的数量不能直接当作最终基线：

- Required：摘要记为 15 处；当前逐项列出的字段为 12 个 Go 始终输出字段 + 3 个 Go omitempty 字段，共 15 项。第一组的 12 项均应在实际 JSON 输出验证后列入 required。
- Nullable：旧摘要中的 10 处仅覆盖部分标量字段，并非完整统计。完整模型级扫描结果以第 11 节为准；必须区分 `omitempty` 的省略语义与非 `omitempty` nil 指针/切片/Map 的 null 语义。
- 缺失字段：按当前列表为 15 项；执行时逐字段核验实际 API 可达性。
- 多余字段：按当前列表为 9 项。

因此，**当前不能宣称上述字段级差异已完全穷尽**。本文件先把已知项显式化，并要求 Data 分支通过自动化工具重新生成最终差异报告。只有报告与人工 Review 对齐后，才允许冻结。


## 追加：Data 最新分支快照复核（2026-11-05）

- 当前 Data HEAD：`fd39c663c4ad59836acdf963d5fb8b2f9227abe8`，提交 `docs(market-data): 记录 BaoStock 首事件前探测结果`。
- 相较上次 Schema 核查基线 `4732f14d8998bcd06ffb1118846535b626865b2d`，compare ahead 7 / behind 0。
- Compare 变化文件仅为两份 `doc/api/**` 文档和 `services/market-data/internal/baostock/live_adjustment_probe_test.go`；没有修改 `services/market-data/openapi.yaml` 或 `internal/model/**`。
- 因此本清单既有 OpenAPI ↔ Go JSON 字段、required/nullable 和多余字段差异仍未被本次提交修复；此前字段级结论在该 SHA 上仍适用。

本次新增的 BaoStock 实测说明两只证券首个因子事件日前没有 raw 日 K，`pre_first_event_status=not_applicable`，所以没有验证 latest-factor fallback；生产逻辑继续 fail closed，不采用未来因子。因子 `AvailableAt` 仍未知，当前 qfq/hfq OHLC 匹配不能证明历史 PIT 可见性。该实测是复权重建样本证据，不应被误读为 OpenAPI Schema 修复或 Data 全量 PIT 验收。


## 11. Data 最新 HEAD 字段级完整静态复核（2026-11-05）

### 11.1 核查基线与方法

- Data 分支：`feat/full-go-data-providers-api-switch`
- HEAD：`fd39c663c4ad59836acdf963d5fb8b2f9227abe8`
- OpenAPI：`services/market-data/openapi.yaml`，Blob `9a15f7810a07c60887a574d82e1f84d56a8c9964`
- Model：`services/market-data/internal/model/*.go` 下 11 个 Go 文件。
- 逐个解析 Go struct JSON Tag，并与同名 OpenAPI Schema 的 properties、required、nullable 做静态比较；另检查现有 `openapi_contract_test.go` 和关键 Handler 的实际返回对象。

本次统计得到 57 个 Model struct，其中 51 个存在同名 OpenAPI Schema；6 个未同名映射类型分类如下：

| Go 类型 | 分类 |
|---|---|
| `AdjustmentFactor`、`CorporateAction` | 内部历史模型；当前没有对应 HTTP Handler 路由 |
| `StockUniverseSnapshot` | MarketStocks 证券池构建中间结果 |
| `MarketStockOverviewItem` | `MarketStocksSnapshot.items` 内联对象 |
| `ProviderProbeFieldComparison` | `ProviderProbeCheckResult.field_comparisons` 内联对象 |
| `PriceLimitRuleSnapshot` | 价格涨跌幅规则 Handler 的响应对象，OpenAPI 采用内联响应结构 |

这 6 个类型不能简单按“缺少同名 Schema”判为 API 漏字段；应继续按真实 Operation response 引用/内联结构验证。

### 11.2 Go 有字段、OpenAPI 缺字段：15 项

| Schema | 缺失字段 |
|---|---|
| `ProviderProbeCheckResult` | `left_adjustment_method`、`right_adjustment_method`、`left_adjustment_reference_date`、`right_adjustment_reference_date` |
| `SectorRanking` | `provider_report_date` |
| `SectorRankingSnapshot` | `excluded_non_a_share_count` |
| `MarketBreadthSnapshot` | `excluded_non_a_share_count` |
| `LimitPoolItem` | `seal_money`、`max_seal_money` |
| `FundamentalSection` | `available_at` |
| `IndexConstituent` | `effective_from`、`effective_to`、`available_at`、`system_observed_at` |
| `SourceStatus` | `market_capital_flow_capable` |

字段存在于 Go Model 不代表它们一定已被 Handler 返回；仍需沿真实 Handler 的结果对象确认。本次列表是同名 Model/Schema 差异清单，Data 实施时应按 Operation response 路径裁定。

### 11.3 OpenAPI 有字段、Go 无字段：9 项

| Schema | OpenAPI 多余字段 |
|---|---|
| `Quote` | `validation_total_count`、`validation_error_count`、`validation_warning_count`、`validation_info_count`、`validation_truncated`、`validation_issues` |
| `StockBoard` | `change_percent_5d`、`provider_report_date` |
| `PriceLimitEventEvidence` | `adjustment_basis` |

关键 Handler 核对：
- `Server.quote` 将 `svc.Quote` 返回的 `model.Quote` 直接写入 JSON；Quote Schema 中 6 个 validation 字段没有 Go Model 来源。
- `Server.stockBoards` 将 Service 返回的 StockBoards 结果直接写入 JSON；`model.StockBoard` 不包含上述两个多余字段。
- `Server.priceLimitRule` 将 `ResolvePriceLimitRuleFromTradingStatus` 返回的 `PriceLimitRuleSnapshot` 直接写入 JSON；其 `PriceLimitEventEvidence` Go 类型没有 `adjustment_basis`。

因此不应为了迁就当前 OpenAPI 而向 Go Model 凭空增加字段。若业务确实要求这些数据，应先在真实 Handler DTO/领域来源中定义，再同步更新 Schema 和响应测试。

### 11.4 required 不一致：15 项

Go 非 `omitempty` 字段应始终序列化，OpenAPI 应列入 required；Go `omitempty` 字段在零值时可能省略，OpenAPI 不应强制 required。

| Schema | 差异字段 | Go | OpenAPI |
|---|---|---|---|
| `StockAnnouncementSnapshot` | `source_version` | required | optional |
| `DragonTigerSnapshot` | `source_version` | required | optional |
| `MarketSentimentSnapshot` | `limit_up_count`、`limit_down_count`、`broken_board_count` | required（指针可输出 null） | optional |
| `Quote` | `change`、`change_percent`、`amount` | required（指针可输出 null） | optional |
| `DailyBar` | `amount`、`change_percent` | required（指针可输出 null） | optional |
| `Bundle` | `warnings` | optional（omitempty） | required |
| `MarketBreadthSnapshot` | `total_amount_cny` | required（指针可输出 null） | optional |
| `HotRankingItem` | `source_version` | optional（omitempty） | required |
| `HotRankingSnapshot` | `source_version` | optional（omitempty） | required |
| `SourceStatus` | `historical_index_constituents_capable` | required | optional |

合计 15 个字段。对于 required 指针字段，required 与 nullable 必须同时正确：字段必须存在，但值允许为 null；不能把 required 误解为非空。

### 11.5 nullable 与 JSON 序列化语义

按 Go 标准 `encoding/json` 和当前 JSON Tag：

**A. OpenAPI 声明 nullable，但 Go 使用 omitempty：122 项**

- 112 个 `*T + omitempty` 指针字段；
- 9 个 `string + omitempty` 字段；
- 1 个 `slice + omitempty` 字段（`ProviderProbeCheckResult.field_comparisons`）。

这些字段的 nil/零值会被省略，而不是输出 JSON null。OpenAPI nullable 在此是过宽的响应契约。9 个标量字符串字段为：
`DragonTigerItem.provider_amount_unit`、`DragonTigerSnapshot.provider_report_date`、`StockBasic.industry`、`StockBasic.board_code`、`LimitPoolSnapshot.provider_report_date`、`HotRankingItem.name`、`HotRankingItem.rank_trend`、`ChipDistribution.date`、`ChipDistribution.provider_date`。

112 个指针字段不逐项复制在本文；自动化测试应以 Go JSON Tag 生成完整字段级报告，不能只修字符串字段。

**B. Go Model 允许输出 null，但 OpenAPI 未声明 nullable：23 项**

- 22 个没有 `omitempty` 的 slice/map 字段，若其运行时值为 nil，标准 JSON 编码会输出 null；如果 API 契约要求永远输出数组/对象，则 Handler/Service 必须保证初始化为非 nil，并由真实响应测试证明。
- 1 个非 `omitempty` 指针字段：`LimitPoolItem.change_percent`，Go nil 时会输出 null，Schema 应允许 nullable。

22 个复合字段为：
- `StockAnnouncementSnapshot.items`
- `DragonTigerSnapshot.items`
- `MarketSentimentHistory.items`
- `Bundle.daily_bars`、`Bundle.sources`
- `StockBoardSnapshot.items`
- `StockSearchSnapshot.items`
- `SectorRankingSnapshot.items`
- `IndexQuoteSnapshot.items`
- `LimitPoolSnapshot.items`
- `FundamentalSection.metrics`
- `FundamentalsSnapshot.sections`
- `ValuationSnapshot.requested_symbols`、`ValuationSnapshot.items`
- `IndexCatalogSnapshot.items`
- `IndexConstituentSnapshot.items`
- `IndexDailyBarsSnapshot.items`
- `HotRankingSnapshot.items`
- `MarketCapitalFlowSnapshot.items`
- `ProviderProbeRun.items`
- `ProviderProbeHistory.items`
- `TradingCalendarSnapshot.items`

这些复合字段的最终处理不应机械地把所有数组改成 nullable：优先明确 API 约定为空集合时返回 `[]` / `{}`，在所有 Handler 输出边界保证非 nil，并增加真实 JSON 测试；只有确实允许 null 时才在 Schema 标记 nullable。

### 11.6 AdjustmentBasis 枚举核对

Go `model.AdjustmentBasis` 响应值为 `none`、`forward_adjusted`、`backward_adjusted`、`unknown`；OpenAPI `AdjustmentBasis` 与之相同。

请求参数使用 `RequestAdjustmentBasis`，只允许前三项；`normalize.Adjustment` 对 `unknown` 返回错误。OpenAPI 查询参数和 ProviderProbeRequest 的请求数组均引用 `RequestAdjustmentBasis`。本项通过静态核对，无需改动。

### 11.7 当前自动化测试覆盖不足

当前 `internal/httpapi/openapi_contract_test.go`：
- 解析 YAML 并检查 API Key 安全声明；
- 检查部分路径存在、选定 Schema 字段存在；
- 对 MarketSession、IndexConstituentSnapshot、ProviderProbeItem、SectorRanking 等做少量字符串断言。

但没有：
- 反射所有 Go JSON Tag 与 OpenAPI properties 的双向对照；
- 对所有 Schema 自动校验 required/optional/nullable；
- 按真实 Operation response 路径关联 Handler 返回类型；
- 全量枚举、format、边界和嵌套对象自动校验。

现有测试中的少量人工断言不能替代完整契约测试；测试文件名含 contract 也不代表已实现双向字段契约。

### 11.8 更新后的实施与验收门禁

1. 先按真实 Operation response 路径建立响应 DTO ↔ Schema 映射，明确 6 个无同名 Schema 类型的归属。
2. 补齐 15 个缺失字段；删除 9 个没有实际输出来源的 Schema 字段，或由真实响应 DTO/Handler 实现后提供 JSON 测试。
3. 修正 15 个 required 差异。
4. 对 122 个 `omitempty` nullable 声明逐项校正；对 23 个可能输出 null 的字段，明确是统一保证空集合/对象，还是在 Schema 允许 null。
5. 增加 Go 反射 + OpenAPI 解析的双向契约测试，并用真实 Handler JSON golden 覆盖包装 DTO、nil slice/map、nil pointer 和实际响应结构。
6. 把测试加入 Data Module CI；以最终自动生成报告归零作为验收依据。

**本节是当前 HEAD 的静态字段核对结果，不是 Data 分支已修复或 CI 已通过的声明。** 本次未在 Data 分支修改任何文件，也未执行 Go 测试；Data OpenAPI 契约仍为未冻结状态。


### 11.9 Operation → Handler → Response Schema 映射复核（2026-11-05）

本节将静态字段比对提升到真实 HTTP 输出边界。核查基线：

- Data 分支 HEAD：`4ff4359178be7a055fb81756a5099adb9fcf3f31`，提交 `test(market-data): 扩展 BaoStock 首事件前真实探测样本 [historical_baostock_factors]`。
- 相较 `4732f14d8998bcd06ffb1118846535b626865b2d`：ahead 8 / behind 0。
- Compare 仅包含 `doc/api/**` 两份文档和 `internal/baostock/live_adjustment_probe_test.go`；没有修改 `services/market-data/openapi.yaml`、HTTP Handler 或 Model 文件。因此本轮的 Operation/Handler/Schema 观察与前一静态字段核查使用同一份 API/Model 实现。
- 当前 OpenAPI 文件 blob：`9a15f7810a07c60887a574d82e1f84d56a8c9964`；`internal/httpapi/server.go` blob：`310036ea140bd1d38ec2c99e0595f09712a235e1`。

#### 11.9.1 路由与 200 Response 映射

路由注册集中在 `internal/httpapi/server.go:190-227`。以下列出 OpenAPI 的成功响应 Schema 与真实路由 Handler。标注“内联”表示 OpenAPI 未引用同名顶层 Schema；“包装”表示 Handler 构造了 map 或包装 DTO，而非直接返回单个领域 Model。

| Operation | Handler | 200 Response Schema / 实际输出形态 |
|---|---|---|
| `GET /api/v1/market/quotes` | `batchQuotes`（`batch_quotes.go`） | 内联批量对象；`items[]` 使用私有 `batchQuoteItem`，其中 `quote *model.Quote` |
| `GET /api/v1/quotes/{symbol}` | `quote`（`server.go`） | `Quote`；直接序列化 `DataService.Quote` 返回的 `*model.Quote` |
| `GET /api/v1/bars/{symbol}/daily` | `bars`（`server.go`） | `DailyBarsResponse`；Handler 构造包装对象，`bars[]` 为 `[]model.DailyBar` |
| `GET /api/v1/bundles/{symbol}` | `bundle`（`server.go`） | `Bundle`；直接序列化 `*model.Bundle` |
| `POST /api/v1/data-sources/probe` | `dataSourceProbe`（`provider_probe.go`） | `ProviderProbeRun`；直接序列化 Probe Application 返回值 |
| `GET /api/v1/data-sources/probe/history` | `dataSourceProbeHistory`（`provider_probe.go`） | `ProviderProbeHistory`；直接序列化历史查询结果 |
| `GET /api/v1/data-sources/diagnostics` | `dataSourceDiagnostics`（`server.go`） | 内联对象；`items` 来自 `ProviderDiagnostics()`，无 Service 时返回空数组 |
| `GET /api/v1/data-sources` | `dataSources`（`server.go`） | `DataSourcesResponse`；Handler 包装 `[]model.SourceStatus` 和 normalization version |
| `GET /api/v1/stocks/search` | `searchStocks`（`server.go`） | `StockSearchSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/stocks/{symbol}/announcements` | `stockAnnouncements`（`server.go`） | `StockAnnouncementSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/stocks/{symbol}/profile` | `stockBasic`（`server.go`） | `StockBasic`；直接序列化 Service 返回值 |
| `GET /api/v1/stocks/{symbol}/price-limit-rule` | `priceLimitRule`（`price_limit_rule.go`） | 内联响应对象；Handler 调用 `ResolvePriceLimitRuleFromTradingStatus` 后直接序列化结果，嵌入 `*model.PriceLimitEventEvidence` |
| `GET /api/v1/stocks/{symbol}/trading-status` | `securityTradingStatus`（`security_trading_status.go`） | `SecurityTradingStatusSnapshot`；直接序列化 `*model.SecurityTradingStatusSnapshot` |
| `GET /api/v1/stocks/{symbol}/fundamentals` | `fundamentals`（`fundamentals.go`） | `FundamentalsSnapshot`；普通和 PIT 分支均直接序列化 Service 返回对象 |
| `GET /api/v1/stocks/{symbol}/chip-distribution` | `chipDistribution`（`server.go`） | `ChipDistribution`；直接序列化 Service 返回值 |
| `GET /api/v1/stocks/{symbol}/capital-flow` | `capitalFlow`（`server.go`） | `CapitalFlow`；直接序列化 Service 返回值 |
| `GET /api/v1/stocks/{symbol}/boards` | `stockBoards`（`server.go`） | `StockBoardSnapshot`；直接序列化 Service 返回值，子项为 `StockBoard` |
| `GET /api/v1/market/valuations` | `valuations`（`valuations.go`） | `ValuationSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/sectors` | `sectorRankings`（`server.go`） | `SectorRankingSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/index-catalog` | `indexCatalog`（`index_catalog.go`） | `IndexCatalogSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/indices/{symbol}/constituents` | `indexConstituents`（`index_catalog.go`） | `IndexConstituentSnapshot`；当前/历史分支均直接序列化 Service 返回值 |
| `GET /api/v1/market/indices/{symbol}/daily` | `indexDailyBars`（`index_catalog.go`） | `IndexDailyBarsSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/indices` | `indexQuotes`（`server.go`） | `IndexQuoteSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/trading-calendar` | `tradingCalendar`（`trading_calendar.go`） | `TradingCalendarSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/session` | `marketSession`（`market_session.go`） | `MarketSessionSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/stocks` | `marketStocks`（`market_stocks.go`） | `MarketStocksSnapshot`；直接序列化 Service 返回值，内部包含行情/证券概览子项 |
| `GET /api/v1/market/capital-flow` | `marketCapitalFlow`（`market_capital_flow.go`） | `MarketCapitalFlowSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/sentiment` | `marketSentiment`（`market_sentiment.go`） | `MarketSentimentSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/sentiment/history` | `marketSentimentHistory`（`market_sentiment.go`） | `MarketSentimentHistory`；直接序列化 Service 返回值 |
| `GET /api/v1/market/breadth` | `marketBreadth`（`server.go`） | `MarketBreadthSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/hot-stocks` | `hotStocks`（路由注册于 `server.go`） | `HotRankingSnapshot`；需纳入真实 Handler JSON golden |
| `GET /api/v1/market/dragon-tiger` | `dragonTiger`（`server.go`） | `DragonTigerSnapshot`；直接序列化 Service 返回值 |
| `GET /api/v1/market/limit-pools` | `limitPool`（`server.go`） | `LimitPoolSnapshot`；直接序列化 Service 返回值 |

说明：以上映射以当前路由注册、OpenAPI 200 Response 和本轮已读取的 Handler 实现为依据。对于本轮未逐行展开的 Handler，映射先用于确定 Golden 测试清单，不代替对其完整序列化分支、异常分支和零值行为的逐项测试。健康检查 `/health/live`、`/health/ready` 也有 200 响应，但其响应是简单运行状态对象，不属于领域 Model Schema 对照范围。

#### 11.9.2 三组字段异常已定位到真实响应边界

1. **Quote 的 6 个多余 Schema 字段**：`GET /api/v1/quotes/{symbol}` 的 Handler 直接写出 `*model.Quote`；批量行情也把同一 `*model.Quote` 放入 `batchQuoteItem.quote`。因此 `Quote.validation_total_count`、`validation_error_count`、`validation_warning_count`、`validation_info_count`、`validation_truncated`、`validation_issues` 在当前 Go Model 响应中没有来源。应删除 Schema 字段，或先实现有明确业务来源的响应 DTO，再同步测试；不能仅为匹配 OpenAPI 在 Handler 中伪造值。
2. **StockBoard 的 2 个多余 Schema 字段**：`GET /api/v1/stocks/{symbol}/boards` 返回 `StockBoardSnapshot`，其 `items` 是 `StockBoard`。`change_percent_5d`、`provider_report_date` 在 Go 子项模型没有来源，不能作为当前响应承诺。
3. **PriceLimitEventEvidence 的多余字段**：`GET /api/v1/stocks/{symbol}/price-limit-rule` 通过 `priceLimitRule` Handler 获取 `*model.PriceLimitEventEvidence`，再由规则 Resolver 生成响应。当前 Go Evidence 模型没有 `adjustment_basis`，但 OpenAPI 内联响应的 Evidence 子对象声明了该字段。必须删除该 Schema 字段或实现可验证来源；不能从请求 adjustment 或默认复权口径推导。

此外，`LimitPoolSnapshot` 由 `limitPool` Handler 直接输出，`LimitPoolItem.change_percent` 的 Go 指针在 nil 时会输出 JSON null；Schema 应明确 nullable，或者 Handler/Service 必须保证该字段非 nil。当前契约不能继续保持“Go 可输出 null、Schema 禁止 null”的状态。

#### 11.9.3 包装 DTO、内联对象和非 Model 输出边界

本轮确认以下对象不能按“同名 Go Model ↔ 顶层 Schema”方式机械核对：

- 批量行情：Handler 私有 `batchQuoteItem` 与 map 包装对象，嵌套复用 Quote。
- 日 K：Handler map 包装 `DailyBarsResponse`，并在 PIT 分支额外写出 `point_in_time`、`as_of`。
- Data Sources、Diagnostics：Handler 组装 map；Diagnostics 的 `items` 由 `ProviderDiagnostics()` 动态返回。
- Price Limit Rule：Handler 通过 Resolver 构造规则响应，OpenAPI 使用内联响应结构。
- `MarketStockOverviewItem`、`ProviderProbeFieldComparison`：作为父 Schema 的内联/嵌套对象核对，不要求独立顶层 Schema。
- `StockUniverseSnapshot`、`AdjustmentFactor`、`CorporateAction`：按此前静态追踪属于内部中间/历史模型，不因其存在于 Model 包就自动要求暴露为 HTTP Schema。

#### 11.9.4 nil / empty 集合审查的结论边界

本轮已确认 `writeJSON` 使用标准 `json.Marshal` 序列化，不存在统一的 nil slice/map 归一化层。因此：

- Go 非 `omitempty` 的 nil slice/map 会序列化为 JSON null；OpenAPI 若只允许 array/object，则存在真实契约风险。
- `omitempty` 的 nil slice/map 会被省略；若 OpenAPI 声明字段必需，则同样不匹配。
- 不能仅因某个 Service 逻辑“通常会返回空数组”就认定字段始终非 nil；需要从每个 Provider 返回、缓存反序列化、降级分支和 Handler 输出分支验证。
- Bundle 已看到明确初始化：`DailyBars: bars`、`Sources: map[string]string{}`、`Warnings: append([]string{}, warnings...)`。但这只覆盖 Bundle 的这些字段，不可推广到其他 Snapshot。
- 对 22 个非 `omitempty` slice/map 字段，下一步必须逐项检查生产构造、缓存命中、Provider 降级和空数据路径，并通过真实 Handler JSON Golden 证明输出为 `[]` / `{}`；若设计允许 null，则显式标注 nullable。
- 对 122 个 Schema nullable=true 但 Go JSON Tag 使用 `omitempty` 的字段，也必须结合真实 Handler 输出确定“省略”还是“null”，不能单纯按 Go 指针类型推断。
- 本轮没有运行 Data Go 测试或真实 HTTP Handler 测试，以上 nil/empty 结论属于静态序列化规则与代码路径核查，不是运行时验收结果。

#### 11.9.5 下一批实施任务拆分

按依赖顺序执行，仍由 Data 分支 Owner 实施，本规划分支不代替修改：

1. **DATA-CONTRACT-001：Operation/Handler/DTO 清单固化**。为全部 33 个业务 Operation 固化成功响应类型、包装 DTO、嵌套类型、错误响应和 Handler 路径；为健康检查单独定义运行状态响应。
2. **DATA-CONTRACT-002：修正无来源 Schema 字段**。处理 Quote 6 项、StockBoard 2 项、PriceLimitEventEvidence 1 项；若选择新增 DTO，必须有真实来源、Handler 使用和 JSON Golden。
3. **DATA-CONTRACT-003：修正字段集合和 required/nullable**。按真实 Operation 可达类型过滤此前 15 个 Go-only 字段、15 个 required 差异、122 个过宽 nullable 和 23 个潜在 null 字段；内部模型不得误纳入 HTTP Schema 计数。
4. **DATA-CONTRACT-004：集合字段归一化**。逐一确定 22 个 slice/map 字段的空集合策略，保证空集合输出 `[]` / `{}`，或在允许 null 时明确声明；覆盖 Provider 返回 nil、空结果、缓存反序列化、降级和存储读取。
5. **DATA-CONTRACT-005：自动化契约测试**。从 Handler 路由表建立 Operation → Handler → Response DTO 显式映射；反射 Go JSON Tag 并与 OpenAPI properties/required/nullable/enum/format 双向对照；为包装 DTO、嵌套 DTO、nil 指针、nil slice/map 和 PIT 分支增加真实 Handler JSON Golden。
6. **DATA-CONTRACT-006：CI 门禁**。把双向契约测试和 Golden 加入 Data Module CI，差异报告归零；任何例外需有带理由的显式 allowlist，不能静默忽略。

**本节核查结论**：Operation 与主要 Handler 输出边界已建立；三组无来源字段已落到真实响应链路；集合 nil/empty 仍需要逐项构造路径核验和运行时 Golden。当前只是静态审查，不代表 Data Schema 已修复、测试已通过或 GOV-004 已冻结。


### 11.10 Data 最新 HEAD 与 CI 复核（2026-11-05）

在 11.9 映射核查后，Data 分支又前进一个提交，最新 HEAD 为 `c22470bec87beb89961b76ad9856d5c8709aac73`（`fix(market-data): 修复 BaoStock 历史探测循环语法`）。相较映射核查基线 `4ff4359178be7a055fb81756a5099adb9fcf3f31` ahead 1 / behind 0；Compare 仅涉及两份 `doc/api/**` 文档和 `services/market-data/internal/baostock/live_adjustment_probe_test.go`，没有改动 OpenAPI、HTTP Handler 或 Model。因此本清单的 Operation/Schema 字段差异仍适用于最新 HEAD。

最新 SHA 关联的 GitHub Actions：
- Go test suite：run `42012445022`，completed / success。包括 `services/market-data` 的 gofmt、tests、race、vet，根模块和 go-core 测试，以及 MariaDB Migration/PIT、DataService PIT fallback、Probe HTTP + MariaDB 集成、Docker market-data image build。
- Go-Python data quality parity：run `42012445049`，completed / success。
- WebUI checks：run `42012444875`，completed / success。
- Live provider probe：run `42012444966`，workflow completed / success；但 BaoStock historical adjustment-factor consistency 专项步骤为 **skipped**，因此本次没有八只候选证券的 BaoStock matched/mismatch/not_applicable/unavailable 结果。报告打印/上传步骤 success 不能替代实际专项探测。

这次 CI 证明最新 HEAD 已恢复 Go 语法并通过当前执行的测试/build 门禁；**不证明 OpenAPI ↔ Go JSON 双向契约已修复**，因为本轮提交没有修改相关 Schema/Model，也没有新增契约测试。Data Schema 阻断继续打开。


### 11.11 22 个非 omitempty 集合字段：构造路径与 nil/empty 风险复核（2026-11-05）

核查代码基线仍为 Data HEAD c22470bec87beb89961b76ad9856d5c8709aac73。本节追踪 Service 校验、Provider 返回、缓存反序列化和 Service 构造，不把 len(x)==0 误认为 x 一定是非 nil 空集合。

#### 11.11.1 逐字段结论

| # | JSON 字段 | 构造/数据来源 | 当前静态结论 | 关键证据与后续要求 |
|---:|---|---|---|---|
| 1 | StockAnnouncementSnapshot.items | Announcement Provider / Redis Cache | **确认存在 nil 风险** | 空报告 Total=0 时仅检查 len(Items)==0，unknown 空报告可通过；需归一化或 nullable。 |
| 2 | DragonTigerSnapshot.items | DragonTiger Provider / Redis Cache | **确认存在 nil 风险** | 空报告、全部为非 A 股的 unknown 空页只检查 len(Items)==0，可接受 nil slice。 |
| 3 | MarketSentimentHistory.items | store.ListMarketSentiment | **边界待核实** | Service 直接将 Store 返回的 items 写入响应，没有 make/copy 归一化；需检查 Store 空结果是否返回 []，并做 Handler Golden。 |
| 4 | Bundle.daily_bars | DataService.DailyBars | **条件性 nil 风险** | Bundle 直接赋值 DailyBars: bars，未归一化；若 DailyBars 的合法空结果为 nil，JSON 即为 null。需追踪 DailyBars 的 Provider/缓存/降级空结果。 |
| 5 | Bundle.sources | Bundle Service 构造 | **已确认非 nil** | 显式 map[string]string{} 初始化。 |
| 6 | StockBoardSnapshot.items | StockBoard Provider / Redis Cache | **确认存在 nil 风险** | 无数据分支要求 Total=0、len(Items)=0、QualityUnknown，nil 可通过。 |
| 7 | StockSearchSnapshot.items | StockSearch Provider | **确认存在 nil 风险** | Validator 只检查总数和 len 上限；unknown 空匹配可被保留并最终返回，nil 可通过。 |
| 8 | SectorRankingSnapshot.items | Sector Ranking Provider / Redis Cache | **成功响应非 nil** | Validator 要求 1 <= len(Items) <= limit，空集合不会作为成功响应返回。 |
| 9 | IndexQuoteSnapshot.items | Index Quote Provider / Redis Cache | **成功响应非 nil** | Validator 在入口拒绝 len(Items)==0。 |
| 10 | LimitPoolSnapshot.items | Limit Pool Provider / Redis Cache | **确认存在 nil 风险** | 无数据分支允许 Total=0、len(Items)=0、QualityUnknown，nil 可通过。 |
| 11 | FundamentalSection.metrics | Fundamentals Provider / Redis Cache | **确认存在 nil 风险** | HasData == (len(Metrics)>0)；无数据分区允许 nil。字段没有 omitempty，nil 会输出 null。 |
| 12 | FundamentalsSnapshot.sections | Fundamentals Provider / Redis Cache | **验证后非 nil** | Validator 要求 Sections 数量为 6..9，nil 不可能通过。PIT 过滤分支也使用 make([]model.FundamentalSection, 0, ...)。 |
| 13 | ValuationSnapshot.requested_symbols | Valuation Provider / Redis Cache | **验证后非 nil** | 请求先构造非空 symbols；Validator 要求长度相等并逐项按请求顺序比对，nil 不可能通过。 |
| 14 | ValuationSnapshot.items | Valuation Provider / Redis Cache | **确认存在 nil 风险** | Validator 允许 Total == len(Items) == 0，并允许通过 MissingSymbols 覆盖全部请求证券；未要求空 Items 非 nil。 |
| 15 | IndexCatalogSnapshot.items | Index Catalog Provider / Redis Cache | **确认存在 nil 风险** | 空目录在 QualityUnknown 状态下可通过 Validator，并可能作为 fallback exhausted 的结果返回。 |
| 16 | IndexConstituentSnapshot.items | Index Constituent Provider / Redis Cache | **确认存在 nil 风险** | Validator 允许空 Items，但要求 QualityUnknown；unknown 候选会在后备源耗尽后返回。 |
| 17 | IndexDailyBarsSnapshot.items | Index Daily Bars Provider / Redis Cache | **确认存在 nil 风险** | Validator 以 Total 与 len 相等校验；Service 将空 Items/unknown 作为候选，后备源耗尽后可返回。 |
| 18 | HotRankingSnapshot.items | Hot Ranking Provider / Redis Cache | **确认存在 nil 风险** | Validator 明确允许空榜，但必须 QualityUnknown；后备源耗尽后返回 unknown 候选。 |
| 19 | MarketCapitalFlowSnapshot.items | Market Capital Flow Provider / Redis Cache | **成功响应非 nil** | Validator 要求 Total>0 且 Total=len(Items)，空集合不会作为成功响应返回。 |
| 20 | ProviderProbeRun.items | Probe Service 构造 | **已确认非 nil** | 初始化使用 make([]model.ProviderProbeItem, 0)。 |
| 21 | ProviderProbeHistory.items | store.ListProviderProbeRuns | **边界待核实** | Service 直接使用 Store 返回值；空历史时是否返回 nil 取决于 Store 实现，需核对并 Golden。 |
| 22 | TradingCalendarSnapshot.items | Trading Calendar Provider / Redis Cache | **验证后非 nil** | 日期区间有效且至少覆盖一个自然日；Validator 要求 Items 长度等于预期自然日数，nil 不可能通过。 |

#### 11.11.2 风险分组

- **已确认存在 nil 可通过并可能输出 null 的 Provider 响应字段**：Announcement、DragonTiger、StockBoard、StockSearch、LimitPool、FundamentalSection.metrics、Valuation.items、IndexCatalog、IndexConstituent、IndexDailyBars、HotRanking。
- **需继续核对外部边界实现**：MarketSentimentHistory.items、ProviderProbeHistory.items（Store）；Bundle.daily_bars（DailyBars 返回路径）。
- **Service 校验已保证非空或显式初始化**：Bundle.sources、MarketStocksSnapshot.items（按 selected 长度 make）、SectorRanking.items、IndexQuote.items、FundamentalsSnapshot.sections、Valuation.requested_symbols、MarketCapitalFlow.items、ProviderProbeRun.items、TradingCalendar.items。
- Bundle.warnings 使用 append([]string{}, warnings...) 显式构造非 nil slice，但 JSON Tag 带 omitempty，空时会省略；它不是此前 22 个非 omitempty 字段之一。

这里的“确认 nil 风险”表示当前 Service 校验允许 nil slice 通过，并且没有统一归一化；不表示已在生产 HTTP 响应中抓到实际 null。最终证据仍需真实 Handler JSON Golden 或运行时响应样本。

#### 11.11.3 修正原则

1. 对必需数组/对象，优先在明确的 HTTP Response DTO 边界统一输出空集合；不能随意修改 Domain/Provider Model 语义。
2. 对允许 null 的业务字段，OpenAPI 必须明确 nullable；不能把“无数据/未知”偷换成空数组或 0。
3. Provider 原始快照若在领域层需要保留 nil/unknown 语义，可在 API DTO 映射时转换，避免污染 Provider 适配和缓存模型。
4. Store 查询的空结果应由 Store 契约明确返回空集合，或由 Application Service 在响应边界归一化；不能依赖 SQL Driver 恰好返回非 nil。
5. Golden 至少覆盖：有数据、确定空结果、unknown 空结果、Provider 全部失败后保留 unknown 候选、Redis 命中历史 nil 缓存、Store 空历史、PIT 过滤后空集合。
6. omitempty nullable 字段需另行区分“省略”与“显式 null”；本表不替代 122 个字段的逐字段处理。

#### 11.11.4 下一步实施任务细化

在 DATA-CONTRACT-004 下增加以下子项：
- **004-A**：新增响应 DTO 集合归一化策略，不在全局 JSON Encoder 做无差别递归转换。
- **004-B**：修正上述 11 个已确认风险字段，优先覆盖 unknown/empty fallback 分支。
- **004-C**：核查 Store 的两个 List 方法和 DailyBars 的空结果，补充 Store/Service 契约。
- **004-D**：对 22 个字段逐项增加非 nil / nil 的 JSON 测试，并对 OpenAPI required/nullable 作对应校验。
- **004-E**：检查 Redis 旧缓存反序列化路径，确保缓存命中与 Provider 新鲜响应输出一致。

**结论**：22 个字段已完成第一轮构造/校验路径分类；11 个字段确认存在 nil 可通过风险，3 个字段边界待核实，其余字段有明确的非 nil 约束或构造保证。未运行 Data 测试，未修改 Data 分支；GOV-004 仍 NO-GO。


### 11.12 nullable + omitempty 交叉复核：修正“122 项均为契约错误”的判断（2026-11-05）

本轮重新解析 Data 当前 OpenAPI 的 nullable 声明，并与 57 个 Go Model struct 的 JSON Tag 交叉核对。结果：

- 121 项位于同名顶层 OpenAPI Schema 属性，Go 对应字段带 omitempty。
- 另有 1 项是 Price Limit Rule 的内联响应属性 special_exception_evidence，对应 PriceLimitRuleSnapshot.SpecialExceptionEvidence，Go Tag 为 omitempty。
- 合计 122 项；其中 112 个指针字段、9 个字符串字段、1 个内联指针属性。
- **122 项均未出现在各自 Schema 的 required 列表中**。对已匹配的 121 个顶层字段，程序化扫描 required 交集为 0；内联 special_exception_evidence 也不在该响应的 required 列表中。

因此必须修正此前“122 个 nullable + omitempty 全部是 Schema 不一致/阻断项”的表述：

1. OpenAPI 属性非 required 时，Go omitempty 省略 nil/零值，符合“字段可以不存在”的契约；OpenAPI nullable=true 允许 null，并不强制实现必须输出 null。**这 122 项本身不是 JSON Schema required/property conformance 错误。**
2. 它们仍然存在“实现输出集合是 OpenAPI 允许集合的子集”的语义差异：OpenAPI 允许显式 null，当前 Go JSON Tag 选择省略。若调用方需要区分“明确未知/null”和“字段不适用/未提供”，必须冻结字段级语义，不能仅凭 nullable=true 推断服务会输出 null。
3. 当前已定位到至少一个文档语义需要裁决的字段：Quote.volume_ratio 的 OpenAPI 描述写明“不支持时为 null”，但 Go Tag 为 volume_ratio,omitempty，nil 时实际省略。应二选一：把文档改为“不支持时省略”，或由明确的 Response DTO 输出显式 null；不能保留文档承诺与实际编码行为不一致。
4. 其他 nullable + omitempty 字段逐项检查描述、Provider 语义和调用方需求；不要求为了形式一致而把所有 omitempty 删除，也不要求把所有 nullable 改成非 nullable。
5. required 差异仍按原 15 项单独处理；本次交叉复核没有发现 nullable + omitempty 与 required 交集，不代表此前 15 项 required 差异已自动消失。

建议把原任务“修正 122 个过宽 nullable”改为 **DATA-CONTRACT-003-B：nullable/省略语义裁决**，逐字段输出三种结果：保留 nullable + 允许省略、改为明确 null 输出、移除 nullable 并仅允许省略。Quote.volume_ratio 优先处理。

**修正后的判断**：122 项不应作为 122 个独立阻断缺陷计数。真正的阻断是其中明确承诺 null 却被 omitempty 省略的语义冲突、required 差异和真实 Handler 输出未被 Golden 验证。此前“122 个过宽 nullable”属于过度归因，本节予以纠正。


### 11.13 Data 分支最新提交与当前 CI 状态（2026-11-05）

本轮继续审查期间，Data 分支从 c22470bec87beb89961b76ad9856d5c8709aac73 前进到最新 HEAD：

- SHA：`bc55db99fc6908c77c3c0ec6a213651373a857b2`
- 提交：`docs(api): 同步 BaoStock 历史探测窗口计划与契约`
- Compare：相较 c22470b ahead 2 / behind 0。
- 文件变化：两份 doc/api 规划文档、一份 WebUI/数据接口协作文档，以及 BaoStock Live Probe 测试；没有修改 OpenAPI、HTTP Handler 或 Model。因此 11.9、11.11、11.12 的静态代码结论仍适用于最新实现。

该 SHA 的 Workflow 状态：
- WebUI checks run 42017149949：completed / success。
- Go-Python data quality parity run 42017149954：completed / success。
- Live provider probe run 42017149934：completed / success。
- Go test suite run 42017149908：本次复核时仍为 in_progress，不能提前写成通过。

Live provider probe 整体 success 仅表示 Workflow 成功；如需确认专项 BaoStock 历史复权步骤，仍需检查该 Run 的具体 Job Steps，不能用整体结论代替专项结果。

**当前核查基线**：静态代码结论跟随 bc55db9；最新 Go test suite 尚未完成。规划分支仍只记录审查和待办，不修改 Data 业务代码。


### 11.14 LimitPoolItem.change_percent 的可达性复核（2026-11-05）

此前 23 个“Go Model 可能输出 null、OpenAPI 未声明 nullable”的字段中，除 22 个 slice/map 外，另有 LimitPoolItem.change_percent（Go 类型 *float64，JSON Tag 不带 omitempty）。

本轮追踪其真实 Service Validator 后，修正该项风险判断：

- LimitPoolSnapshot.HasData=true 时，validateLimitPoolSnapshot 对每个 Item 强制要求 ChangePercent != nil；缺失时 Provider 快照被拒绝，不会进入成功响应或正常缓存。
- HasData=false 时，Validator 要求 Total=0 且 len(Items)=0；不存在携带 ChangePercent=nil 的 Item。
- 因而在当前 DataService → Handler 的可达成功路径上，LimitPoolItem.change_percent 不会以 JSON null 输出；OpenAPI 不声明 nullable 与当前运行时约束一致。
- 这项结论以所有该 Operation Handler 都经 DataService.Validator 为前提；未来新增旁路、直接 Model 序列化入口时需重新审查。

因此此前 23 项“可能 null”应拆分为：22 个 slice/map 字段继续按 11.11 逐项验证；LimitPoolItem.change_percent 从当前可达运行时阻断清单中移除。它仍可保留单元测试，明确 nil 必须被 Validator 拒绝。

**修正结论**：不能只看 Go 指针类型就判定 API 一定输出 null；必须结合 omitempty、Validator、Handler 和实际可达分支。


### 11.15 15 处 required 差异：按真实可达输出重新分级（2026-11-05）

本轮重新用 Go JSON Tag 与 OpenAPI Schema required 列表交叉扫描 51 个同名 Go/OpenAPI 类型。对“OpenAPI required、Go omitempty”的方向，得到 3 项：

| Schema.field | Go JSON Tag | Validator / 构造路径 | 运行时结论 |
|---|---|---|---|
| Bundle.warnings | warnings,omitempty | Bundle Service 显式构造 append([]string{}, warnings...)；无警告时 len=0，encoding/json 的 omitempty 仍会省略 | **确认 required 违约风险**：OpenAPI required，但空警告集合会缺席。建议移除 Go Tag 的 omitempty，保留非 nil 空 slice 输出 []。 |
| HotRankingItem.source_version | source_version,omitempty | validateHotRankingSnapshot 要求 Item.SourceVersion 与 Snapshot.SourceVersion 一致；Snapshot.SourceVersion 又必须 identifiable/non-empty | **当前正常路径不可达缺失**：Validator 阻止空值 Item 进入成功响应；保留 omitempty 可接受，但应加回归测试证明。 |
| HotRankingSnapshot.source_version | source_version,omitempty | validateHotRankingSnapshot 要求 Snapshot.SourceVersion 可识别且非空 | **当前正常路径不可达缺失**：Validator 阻止空值 Snapshot 进入成功响应；保留 omitempty 可接受，但应加回归测试证明。 |

其余 12 处差异方向为“Go 无 omitempty、OpenAPI optional”。Go 总是输出这些字段（指针字段为 nil 时输出 null），而 Schema 允许它们缺席；这不是输出违约。对其中的指针字段，OpenAPI 已声明 nullable，需继续保留正确的 null 语义。

因此，原始 15 处 required 差异不能全部算作 15 个运行时错误。当前已确认需要实施修复的重点是 Bundle.warnings；HotRanking 两项由 Validator 保证非空，需用测试固化此约束。

**DATA-CONTRACT-003-C**：修正 Bundle.warnings 的 JSON Tag，或调整 Schema required（优先保持其作为稳定数组字段，建议去掉 omitempty）；补充 Bundle 无警告/有警告两种 Handler Golden。另对 HotRanking 两项增加空 source_version 必须被 Validator 拒绝的测试。


### 11.16 Data 最新 HEAD 再次更新（2026-11-05）

Data 分支最新 HEAD 已推进至 `266ac21f5c840207f79aae8984021dac74185652`：

- 提交：`docs(api): 记录 BaoStock 八证券复权证据 [historical_baostock_factors]`
- 相较上一基线 `bc55db99fc6908c77c3c0ec6a213651373a857b2` ahead 2 / behind 0。
- Compare 变化：三份 `doc/api/**` 文档、一份 WebUI/数据接口协作文档和 BaoStock Live Probe 测试；没有修改 `services/market-data/openapi.yaml`、HTTP Handler 或 Model。
- 因此 11.9 Operation 映射、11.11 集合字段、11.12 nullable + omitempty、11.14 LimitPool 可达性、11.15 required 交叉结论继续适用于最新业务实现。

最新 SHA 关联 Workflow 在本次复核时全部未完成：
- Live provider probe run `42017899688`：in_progress。
- WebUI checks run `42017899766`：in_progress。
- Go-Python parity run `42017899832`：pending。
- Go test suite run `42017899888`：in_progress。

不得沿用前一 SHA 的 CI 结果作为本次最新 SHA 的验收结论。待 Workflow 完成后再更新状态；当前只确认静态代码文件未变化。


### 11.17 Data 最新 HEAD 再次前进（2026-11-05）

最新 Data HEAD 已从 `266ac21f5c840207f79aae8984021dac74185652` 前进到 `587f27683c8422810d2f7fc917a2c9324eb147b0`：

- 提交：`test(baostock): 锁定上市日首因子样本边界 [historical_baostock_factors]`
- Compare：ahead 1 / behind 0。
- 唯一变更文件：`services/market-data/internal/baostock/live_adjustment_probe_test.go`，增加 22 行测试。
- 未修改 OpenAPI、HTTP Handler、Model 或 Service 业务实现，因此 11.9~11.15 的静态契约结论仍适用于最新代码。

最新 SHA 关联 Workflow：
- Go test suite run `42017963089`：pending。
- Live provider probe run `42017963182`：in_progress。
- WebUI checks run `42017963171`：completed / success。
- Go-Python parity run `42017963237`：in_progress。

此处的 Workflow 状态属于 SHA 587f276；不能混用前一个 SHA 266ac21 的状态。静态契约核查基线已更新为 587f276，待其 CI 完成后再记录最终结果。


### 11.18 15 个 Go 有字段、OpenAPI 缺字段：Operation → Handler → Model 逐项追踪（2026-11-05）

本轮以 Data 当前代码为准，追踪 `services/market-data/internal/httpapi/server.go` 路由、Handler 实际写出的对象、Service/Provider 赋值点和 OpenAPI response Schema。结果不是机械地把 15 个字段全部补进 Schema：其中 **13 个字段有真实或明确设计中的响应语义，应补 OpenAPI；另 2 个字段没有生产赋值路径且属于重复/未实现字段，建议从 Go Model 删除，不应制造 API 契约。**

| Go Model 字段 | Operation → Handler → 响应位置 | 赋值/可达性证据 | 结论 |
|---|---|---|---|
| `ProviderProbeCheckResult.left_adjustment_method` | `POST /api/v1/data-sources/probe` → `dataSourceProbe`；`GET /api/v1/data-sources/probe/history` → `dataSourceProbeHistory`；`ProviderProbeRun.check_results[]` | `service/provider_probe.go` 的跨 Provider 对比结果构造处填充 `firstCrossProviderAdjustmentMethod(left)` | **补 Schema**：optional string |
| `ProviderProbeCheckResult.right_adjustment_method` | 同上 | 填充 `firstCrossProviderAdjustmentMethod(right)` | **补 Schema**：optional string |
| `ProviderProbeCheckResult.left_adjustment_reference_date` | 同上 | 填充 `firstCrossProviderAdjustmentRefDate(left)` | **补 Schema**：optional date/string；保留源端原值语义 |
| `ProviderProbeCheckResult.right_adjustment_reference_date` | 同上 | 填充 `firstCrossProviderAdjustmentRefDate(right)` | **补 Schema**：optional date/string；保留源端原值语义 |
| `SectorRanking.provider_report_date` | `GET /api/v1/market/sectors` → `sectorRankings` → `SectorRankingSnapshot.items[]` | 当前 Provider/Service 构造 `SectorRanking` 时没有赋值点；报告日已由 Snapshot 层的 `provider_report_date` 表达 | **从 Go Model 删除**：避免每个排名项重复一个始终缺席的报告日字段 |
| `SectorRankingSnapshot.excluded_non_a_share_count` | 同上 | 当前行业/概念排名 Provider 和 Service 没有赋值点；全仓生产赋值只见 MarketBreadth 与 LimitPool 路径 | **从 Go Model 删除**：当前没有来源证据，不新增 Schema |
| `MarketBreadthSnapshot.excluded_non_a_share_count` | `GET /api/v1/market/breadth` → `marketBreadth` → `MarketBreadthSnapshot` | `provider/tickflow.go` 用 `len(excluded)` 赋值；Handler 直接序列化 snapshot | **补 Schema**：optional integer，minimum 0 |
| `LimitPoolItem.seal_money` | `GET /api/v1/market/limit-pools` → `limitPool` → `LimitPoolSnapshot.items[]` | `provider/hithink.go` 仅对涨停池解析非负 `seal_money`，填入 `SealMoney` | **补 Schema**：optional nullable number；单位遵循同一 Item 的 `provider_amount_unit` |
| `LimitPoolItem.max_seal_money` | 同上 | 同一 Hithink Provider 路径解析非负 `max_seal_money` | **补 Schema**：optional nullable number；单位遵循 `provider_amount_unit` |
| `FundamentalSection.available_at` | `GET /api/v1/stocks/{symbol}/fundamentals` → `fundamentals` → `FundamentalsSnapshot.sections[]` | Provider 当前没有赋值；Service Validator 和 PIT 过滤逻辑会读取该字段；Model 注释明确其为市场参与者可用的精确时间，不等于报告期或采集时间 | **补 Schema**：optional nullable date-time；仅在来源提供可验证时间时返回。当前 Provider 不填时允许省略 |
| `IndexConstituent.effective_from` | `GET /api/v1/market/indices/{symbol}/constituents` → `indexConstituents` → `IndexConstituentSnapshot.items[]` | Hithink/Tushare 当前构造只填 Symbol/Code/Name/Weight；Service 的历史区间质量门禁检查该字段 | **补 Schema**：optional date；不得用报告日推断 |
| `IndexConstituent.effective_to` | 同上 | 同上 | **补 Schema**：optional date；不得用报告日推断 |
| `IndexConstituent.available_at` | 同上 | 当前 Provider 不赋值；历史区间质量门禁要求可验证时间 | **补 Schema**：optional nullable date-time；有来源证据才返回 |
| `IndexConstituent.system_observed_at` | 同上 | 当前 Provider 不赋值；历史区间质量门禁要求系统观察时间 | **补 Schema**：optional nullable date-time；有持久化观察证据才返回 |
| `SourceStatus.market_capital_flow_capable` | `GET /api/v1/data-sources` → `dataSources` → `DataSourcesResponse.sources[]` | Server 从 `svc.MarketCapitalFlowProviders()` 构建 capability map，并填入 `SourceStatus`；false 因 `omitempty` 省略 | **补 Schema**：optional boolean |

#### Schema 修正建议

1. `ProviderProbeCheckResult` 增加 `left_adjustment_method`、`right_adjustment_method`、`left_adjustment_reference_date`、`right_adjustment_reference_date` 四个 optional 属性。Reference date 若最终统一保证 YYYY-MM-DD，应加 `format: date`；否则先按普通 string 并保留源端值，不能在文档中提前承诺未验证格式。
2. `MarketBreadthSnapshot` 增加 `excluded_non_a_share_count: integer, minimum: 0`，optional；0 因 omitempty 会缺席，Schema 不得标 required。
3. `LimitPoolItem` 增加 `seal_money`、`max_seal_money`，均为 optional nullable number。描述必须明确单位取自 `provider_amount_unit`；当前 Hithink 将单位标为 `provider_defined`，不能写成 CNY。
4. `FundamentalSection` 增加 `available_at`，optional nullable date-time。描述为来源可验证的市场可用时间，不得用 report_date、publication_date 或 fetched_at 自动补值。
5. `IndexConstituent` 增加 `effective_from`、`effective_to`（optional date）、`available_at`、`system_observed_at`（optional nullable date-time）。当前 Provider 未提供这些字段，历史快照应继续降级；不能为了填充 Schema 而推断时间。
6. `SourceStatus` 增加 `market_capital_flow_capable: boolean`，optional，描述为 Provider 是否实现市场级资金流能力。
7. 从 `SectorRanking` 删除未赋值且与 Snapshot 报告日重复的 `ProviderReportDate`；从 `SectorRankingSnapshot` 删除无生产来源的 `ExcludedNonAShareCount`。删除前在 Data 分支确认没有外部构造方依赖，并补充 Model JSON/编译测试。

#### 本轮结论与剩余门禁

- 13 个应补 Schema 的字段分布在 6 个 Schema：ProviderProbeCheckResult（4）、MarketBreadthSnapshot（1）、LimitPoolItem（2）、FundamentalSection（1）、IndexConstituent（4）、SourceStatus（1）——实际为 **6 个 Schema、13 个字段**。其中 4 + 1 + 2 + 1 + 4 + 1 = 13。
- 2 个无赋值/重复字段建议从 Go Model 删除，不应新增 OpenAPI 属性。
- 当前 HTTP Handler 对上述父响应对象均直接调用 `writeJSON`，没有独立 Response DTO 转换层；因此 Model 的 JSON Tag 就是实际 HTTP 输出结构的一部分。
- 这仍是静态追踪，没有修改 Data 代码，也没有为这 13 个字段运行真实 Handler Golden。应在 Data 分支实施时补 Schema、删除死字段，并覆盖有值/缺值/零值的 JSON 输出。
- Operation → Handler → Model 的追踪解决了“字段是否有实际路由”的问题，但还没有替代完整自动化双向契约测试。

### 11.19 Data 最新 HEAD 与 CI 结果更新（2026-11-05）

Data 分支最新 HEAD 已从 `587f27683c8422810d2f7fc917a2c9324eb147b0` 前进到：

- SHA：`cce59e8de493813a4b04f7d0bd24211e1e919bb1`
- 提交：`style(baostock): 修正专项报告结构体格式 [historical_baostock_factors]`
- Compare：ahead 1 / behind 0；唯一文件是 `services/market-data/internal/baostock/live_adjustment_probe_test.go`，修复 gofmt 排版，没有业务逻辑变化。
- 最新 SHA 的 Go test suite、Live provider probe、WebUI checks、Go-Python parity 均 completed / success。

前一 SHA `587f276` 的 Go test suite 曾失败：`internal/baostock/live_adjustment_probe_test.go` 未通过 gofmt，导致 market-data 单测、race、vet 步骤跳过；Docker market-data image build 因同一 gofmt 检查失败。MariaDB Migration/PIT integration、go-core 和根模块测试当时通过。新 SHA 的格式修正后，四个 Workflow 全部成功。

本轮 11.18 的静态字段追踪以 cce59e8 为基线；Compare 证明该提交只改了上述测试文件格式，未改 OpenAPI、Handler、Model 或 Service。CI 全绿不代表字段级契约已修复，GOV-004 仍 NO-GO。

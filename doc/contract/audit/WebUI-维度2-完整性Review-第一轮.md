# WebUI 维度 2：第一轮完整性 Review

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**文档维护分支：** `feat/design_doc`  
**Review 范围：** WebUI 维度 2 数据集覆盖、API Client 消费映射、差异编号和跨数据集契约问题  
**Review 性质：** 局部完整性检查；不是 PLAT-002 全项目阶段 F 完成声明，也不是正确性/冻结 Review。

## 1. 主线范围确认

上一阶段明确的剩余数据集已经全部完成独立切片：
- Stock Search：`WebUI-维度2-Stock-Search.md`
- Analysis Run：`WebUI-维度2-Analysis-Run.md`
- Provider Probe History：`WebUI-维度2-Provider-Probe-History.md`

本轮不增加新的数据集，不回头重复已完成切片。当前进入对已有证据和差异登记的一致性复核。

## 2. API Client 与消费映射完整性

依据 `WebUI-维度2-API-DTO全量盘点.md`：
- `web/src/api.js` 共 44 个导出函数：3 个本地设置函数，41 个远端请求/健康检查入口。
- 40 个远端入口在主要页面模块找到引用；`getMarketSentiment` 是唯一已确认没有页面模块消费者的入口。
- API Client 有调用点，不等于页面完成产品消费；`existing-api-tools.js` 中的工具模块调用不能直接算作默认页面闭环。
- 既有数据集切片覆盖 API 盘点中主要的 Market Data 数据集、Analysis Task/Run 和 Provider Probe History。对没有页面消费者的接口，保持“产品范围待确认”，不因无页面而自动登记缺陷。
- `getMarketSentiment` 是否为预留 API 仍需产品范围确认；不扩展成新的本轮审计任务。

结论：API 入口盘点已完成第一轮，尚不能宣称所有 API 都具有完整产品消费闭环；这是页面产品范围与维度 1/3/4 等其他维度的交叉问题，不通过本轮重复扫描解决。

## 3. 差异编号与重叠检查

### 3.1 CTR-003 与 CTR-046

- CTR-003 最初登记：`GET /api/v1/analysis/runs/{run_id}` 未进入正式 Analysis OpenAPI。
- CTR-046 复核扩展：Run 读取和 Run Events 两条路径、RunResult 等响应 Schema 缺失，并补充 HTTP Handler 测试缺口。
- 两者核心问题相同，CTR-046 的范围更完整。
- 处理：CTR-003 标记为历史初审编号，由 CTR-046 承接；不作为独立活动差异重复计数。保留旧编号以维持审计记录可追溯。

### 3.2 CTR-005 与 CTR-047

- CTR-005 初审指出 AnalysisRequest 与 domain.RunRequest 的描述不一致，缺少 TaskType 枚举、required 约束，以及 as_of_mode/parameters。
- CTR-047 进一步核实 as_of_mode、parameters 和服务端生成 run_id 的实际行为。
- 本轮复查固定基线：`domain.TaskType` 确有四个常量，Runner 对未知 task_type 拒绝；AnalysisRequest Schema 目前只声明 task_type 为普通 string，且没有 required 列表。
- 处理：CTR-047 承接字段映射和异步行为差异；CTR-005 保留为初审编号，但其剩余的 TaskType enum 与必填约束必须纳入 CTR-047 的关闭条件，不能把 CTR-005 整体标为已解决。

### 3.3 CTR-010 的多处引用

- CTR-010 是跨 DTO 的统一空值语义差异登记，后续切片将各自的 omitempty 字段追加为影响范围。
- 多次引用是影响字段清单扩充，不是多个同编号独立缺陷。
- 关闭时必须按 DTO/字段逐项核对 Go JSON 实际序列化、OpenAPI nullable/required 和 Golden；不能只修复最早登记字段就关闭整个 CTR-010。

### 3.4 CTR-038 与 CTR-048

- 两者都涉及历史接口 total 不能表达全量记录数/截断状态，但分别属于 Market Sentiment History 和 Provider Probe History。
- DTO、Repository、Handler 和页面消费不同，不合并为同一缺陷；可共享“total/returned/has_more 语义规范”作为修复设计参考。

## 4. P1 与测试门禁回看

- CTR-041 已撤销：固定基线 DTO tag 实际带 omitempty，OpenAPI 非 required/非 nullable，JSON Golden 要求 omitted，三方一致。原 P1 是审计误读，不应保留为门禁。
- CTR-043（Dragon Tiger）仍为 P1：复核 Hithink `FetchDragonTiger` 确认 `SourceReturned=len(data.StockItems)`，但 items 在 Provider 内按页切片；Service 要求当前页原始数。重复 symbol 被跳过且未计入排除数，进一步违反计数恒等式。代码逻辑冲突已确认，但本轮未执行固定基线测试，不将源码审计描述成运行时测试失败。
- CTR-045 至 CTR-049 均登记 P2；P2 不代表可以在未决情况下冻结。
- 多个数据集的 API Client 或 DOM 测试缺失是独立补证项；只有当缺失测试掩盖字段语义/错误处理/用户误读时，才与对应行为差异一起登记，不把“没有测试”本身自动扩大为缺陷。

## 5. 跨数据集语义一致性观察

| 主题 | 涉及差异 | Review 结论 |
|---|---|---|
| OpenAPI nullable 与 Go omitempty | CTR-010、CTR-011 等 | 跨多个 DTO 的同类问题，关闭必须逐字段验证 |
| 历史 total/截断语义 | CTR-038、CTR-048 | 不同接口的独立问题，统一规范可复用 |
| Provider 缺少时间/覆盖证据 | CTR-015、CTR-025、CTR-027、CTR-031、CTR-034 等 | 各数据集的证据来源和业务含义不同，不合并为一个通用字段修复 |
| Provider 金额单位不明 | CTR-016、CTR-042、CTR-044 等相关记录 | 保持 provider_defined/不可比较，不可为了 UI 统一而猜测单位 |
| 页面消费者/DOM 测试缺口 | CTR-030、033、036、041/042 等对应记录 | 区分产品未接入与已有功能缺少回归测试，不能自动扩大产品范围 |
| Run 与 Task 状态 | CTR-046/047 | 必须保持异步执行状态与分析业务结果状态分离 |

## 6. 本轮 Review 结论

<badge color="warning">partial · 继续复核</badge>

已完成：
- 明确三个最后数据集均有独立审计记录。
- 对照 API Client 总盘点，确认没有因为最后三个数据集而遗漏新的数据集入口。
- 识别并处理 CTR-003/046 的重复登记关系。
- 明确 CTR-005/047 的部分重叠及 CTR-005 剩余 TaskType enum/required 约束。
- 确认 CTR-010 的重复出现是影响字段追加；CTR-038/048 是不同 API 的同类语义问题。
- 回看 P1 门禁，未把未执行 CI 的问题误标为通过。

尚未完成：
- 逐个复核所有 CTR-001 至 CTR-049 的原始证据、关闭条件和责任模块；本轮只处理编号重叠与跨数据集主题。
- 对每个数据集逐字段重跑 DTO/OpenAPI/Golden 比对。
- 固定基线 Go/Node CI 和依赖 MariaDB 的集成测试。
- WebUI 页面交互覆盖与无消费者 API 的产品范围决议。
- 与 WebUI 维度 1、3、4、5、6 的交叉检查。

因此 WebUI 维度 2 仍为 partial；本文件只完成第一轮完整性 Review 的一个子批次，不宣布维度 2 或 PLAT-002 完成。下一批继续在本维度内核对差异关闭条件和证据链，不增加数据集范围。


### 3.5 CTR-005 的 required 语义进一步收敛

再次对照 `internal/platform/httpapi/tasks.go:handleCreateTask`、`internal/platform/httpapi/server.go:fillDefaults`、`internal/analysis/app/runner.go` 和 AnalysisRequest Schema 后，不能简单把 RunRequest 所有非 omitempty 字段都列为 HTTP required：

- HTTP Handler 会为缺省 task_type、as_of、universe_id、portfolio_id、market_id、strategy_ids 及配置版本填充服务端默认值。
- historical_replay 是条件例外：必须由调用方显式提供 as_of；Handler 会在缺省时返回 400。
- Runner 会校验 task_type 仅允许 post_market_scan、watchlist_scan、position_review、historical_replay 四个值。
- 因此 CTR-005/047 的正确关闭要求是：将 HTTP CreateTaskRequest 与 domain.RunRequest 的字段映射和默认行为明确分开；OpenAPI 为 task_type 补四值枚举及默认值，为其他服务端默认字段描述默认行为，并表达 historical_replay 对显式 as_of 的条件约束。不能机械地将所有领域字段标成 required。


## 7. P1 门禁复核：CTR-041 撤销、CTR-043 保留

### CTR-041 撤销依据

固定 SHA `b6dfc85ba214fe16268c8cba780641a8bfe15086`：
- `internal/marketdata/model/model.go`：`SealMoney` / `MaxSealMoney` 的 JSON tag 均带 `omitempty`。
- `internal/marketdata/httpapi/openapi.yaml`：两个字段不在 LimitPoolItem required 中，属性没有 nullable=true。
- `internal/marketdata/httpapi/openapi_contract_test.go`：JSON serialization test 将两字段列入 omitted。
- 结论：三方一致，原 CTR-041 的事实前提错误，已从活动差异/P1 门禁中撤销。不是“测试未跑所以待确认”的状态。

### CTR-043 保留依据

固定 SHA：
- `internal/marketdata/provider/hithink.go:FetchDragonTiger`：`total` 来自 stock_items 总数；`SourceReturned` 设置为 `len(data.StockItems)`；之后仅对 `stock_items[startIndex:endIndex]` 本地分页。
- `internal/marketdata/service/data.go:validateDragonTigerSnapshot`：对非空总数要求 `SourceReturned == min(pageSize, total-(page-1)*pageSize)`，并要求 `SourceReturned == Returned + ExcludedNonAShareCount`。
- Provider 在重复 symbol 时跳过该行并记录诊断，但没有增加 ExcludedNonAShareCount；该字段也不应被滥用于表示重复记录。
- 固定基线 tree 未发现 Hithink 龙虎榜 Provider 专项测试文件。
- 结论：CTR-043 是由 Provider 生产逻辑与 Service 公共计数契约直接证明的 P1 冲突；仍需责任模块修复并补 page 1/中间页/末页及重复记录测试。本轮未执行 Go CI。


## 8. CTR-010 影响字段抽查（Limit Pool / Dragon Tiger）

本批只抽查最近两份独立数据集明细，不宣称 CTR-010 全字段复核完成：

- LimitPoolItem：`last_price`、`open_times`、`turnover_ratio_percent`、`turnover`、`consecutive_boards`、`broken_count` 均为 omitempty；LimitPoolSnapshot.provider_report_date 为 omitempty。OpenAPI 对应属性声明 nullable=true，Go nil 实际省略，属于 CTR-010。
- LimitPoolItem.seal_money/max_seal_money 同样带 omitempty，但 OpenAPI 不声明 nullable、required，JSON Golden 断言省略；它们不属于 CTR-010，也不构成 CTR-041。
- DragonTigerItem 的 16 个可选数值指针字段均带 omitempty；provider_amount_unit 和 DragonTigerSnapshot.provider_report_date 是 omitempty 字符串。OpenAPI 将这些属性声明 nullable=true，与 Go nil 时省略不一致，属于 CTR-010 影响范围。
- `openapi_contract_test.go` 的通用反射逻辑能检查“非 omitempty 指针必须 nullable”，但没有对称检查“omitempty 指针/字符串不应被声明 nullable”；DragonTiger JSON serialization Golden 也未覆盖，因此当前测试结构不能自动发现这类反向不一致。

结论：CTR-010 在这两个数据集中的登记范围成立；CTR-041 撤销不影响 CTR-010。后续仍需对 CTR-010 全部受影响 DTO 做逐字段核对，并补充双向 JSON/OpenAPI 契约验证。

## 9. Provider Probe History 差异证据复核（子批次 2）

本批只复核 CTR-048、CTR-049，不扩展新的数据集或审计维度。代码基线仍为 origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。

### 9.1 CTR-048：total 语义

固定基线证据：
- internal/marketdata/repository/provider_probe.go：ListProviderProbeRuns 仅按 started_at DESC、id DESC 和 LIMIT 查询，没有 COUNT 查询。
- internal/marketdata/service/provider_probe.go：ProviderProbeHistory 将 Total 赋值为 len(items)。
- internal/marketdata/httpapi/openapi.yaml：ProviderProbeHistory.total 仅声明 integer，没有说明是本次返回数还是数据库全量数。
- web/src/data-center.js：页面显示“服务端记录 X 批；当前展示最近 N 批”，存在将 X 理解为全量记录数的歧义。

结论：CTR-048 事实成立，保留 P2。关闭仍采用原记录的二选一方案：将 total 明确改为全量 COUNT，或将字段语义明确改为 returned_count，并消除全量总数暗示；不要求本阶段引入复杂分页。

### 9.2 CTR-049：持久化错误 HTTP 映射

固定基线证据：
- internal/marketdata/service/provider_probe.go：SaveProviderProbeRun 失败时设置 run.Persisted=false 并返回包装后的持久化错误。
- internal/marketdata/httpapi/provider_probe.go：dataSourceProbe 对所有 Service error 统一返回 400 VALIDATION_ERROR，未区分请求校验错误和存储故障；错误分支也不返回已执行探测的 run 摘要。
- internal/marketdata/httpapi/openapi.yaml：POST probe 已声明 500 为诊断结果持久化失败。

结论：CTR-049 事实成立，保留 P2。实际 HTTP 状态与 OpenAPI 不一致；关闭条件仍为区分输入错误与持久化错误，按正式契约返回 500（或先经契约决议改为 503），并补充 Store 写入失败的 Handler 测试。是否返回安全的 persisted=false 结果摘要需明确决议。

### 9.3 测试资产复核

- web/tests/api.test.mjs 有 getDataSourceProbeHistory 请求测试，覆盖 URL、API Key、AbortSignal 和 limit 边界。
- web/tests/data-center-probe-browser.mjs 覆盖历史初次读取、探测后刷新、503 失败、无 API Key 不请求，以及历史失败不覆盖本次探测结果。
- internal/marketdata/httpapi/provider_probe_mariadb_integration_test.go 覆盖 POST 写入、关闭并重开 MariaDB 后 GET 历史读取及关键证据保留，但依赖 MARKET_DATA_TEST_MYSQL_DSN；本轮没有执行该集成测试。
- 未发现独立 Repository 历史排序/limit/重复 run_id 更新测试，也未发现 GET History Handler 对非法 limit、未配置存储和 SQL 读取失败的独立单元测试。

本批没有新增差异编号。CTR-048/049 的证据与关闭条件已复核；维度 2 第一轮完整性 Review 仍需继续核对其余差异证据及跨维度交叉项，不宣布完成或冻结。
## 10. Stock Search 差异证据复核（子批次 3）

本批只复核 CTR-045，不扩展新的数据集或审计维度。代码基线仍为 origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。

### 10.1 CTR-045：搜索质量状态消费

固定基线证据：
- internal/marketdata/provider/tencent_search.go：Tencent Smartbox 无可核验匹配总数，非空结果固定标记 quality_status=degraded，原因 provider_search_total_not_reported；无匹配候选返回错误以触发后备 Provider。
- internal/marketdata/service/data.go：SearchStocks 将 unknown 空结果和 unknown 匹配结果保留为候选状态并继续尝试后备源；全部 Provider 耗尽后，unknown 空结果仍以 quality_status=unknown 和 stock_search_fallback_exhausted_without_confirmed_match 返回，不把它转换为确定无匹配。
- internal/marketdata/httpapi/openapi.yaml：Stock Search 描述明确要求调用方按 quality_status 和 quality_reasons 处理未知/降级质量。
- web/src/app.js：loadStockSearch 对 items 为空直接显示“没有匹配的股票”；非空结果只把 stale 标志传给渲染函数，未消费 degraded、unknown 和 quality_reasons。
- web/src/stock-search-view.js 与 web/tests/stock-search-view.test.mjs：覆盖候选规范化、文本安全渲染和 ARIA 键盘状态，但不覆盖 app.js 搜索流程的质量状态 DOM 行为。

结论：CTR-045 证据成立，保留 P2。unknown 空结果不能显示为确定无匹配；degraded、stale、unknown 应有不同提示。需要增加 app.js 搜索流程的质量状态集成 DOM 回归测试，至少覆盖四种质量状态及 unknown 空结果。

### 10.2 范围与重复项检查

- StockSearchItem/StockSearchSnapshot 的 Go JSON tag 与 OpenAPI required/nullable 结构一致；OpenAPI 对 total 已说明它是 Provider 报告/保留的匹配候选数，不是全市场证券总数。
- Tencent Smartbox 优先、Eastmoney 目录后备属于当前明确装配策略；不能为了让 total 看似完整而强制每次自动补全都扫描 Eastmoney 全市场目录。
- 本批没有发现需要新增的 DTO、Provider Port 或排序差异编号；CTR-045 已覆盖当前确认的质量状态消费缺口。
- 本批没有执行固定基线 CI，不将已有测试资产描述为本轮运行通过。

本批无新增 CTR 编号、无业务代码修改。
## 11. Analysis Run 差异证据复核（子批次 4）

本批只复核 CTR-046、CTR-047，并更正测试覆盖描述，不扩展新的数据集。代码基线仍为 origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。

### 11.1 CTR-046：Run 读取和事件契约

固定基线证据：
- internal/platform/httpapi/server.go：Handler 注册 GET /api/v1/analysis/runs/，handleGetRun 按路径分派 RunResult 读取和 /events 读取；Run 不存在映射 404 run_not_found，结果未完成映射 409 run_not_finished，存储错误映射 500。
- doc/api/contracts/分析任务.openapi.yaml：paths 只描述 Task 创建、Task 状态、Task Events，没有 Run GET 和 Run Events；components.schemas 也没有 RunResult、RunStatus、Failure、StageEvent 等完整响应 Schema。
- web/src/api.js：getAnalysisRun 请求 GET /api/v1/analysis/runs/{run_id}；Task Events 与 Run Events 是不同的资源和游标语义。
- internal/platform/httpapi/server_test.go：已有 TestReadResultMapsNotFound 覆盖 GET Run 404，TestReadResultDoesNotExposeStorageError 覆盖 500 脱敏；未发现 GET Run 成功、409 run_not_finished 和 Run Events Handler 专项测试。

结论：CTR-046 的 OpenAPI 缺失事实成立，保留 P2。对原测试缺口描述作精确修正：404 和 500 并非完全没有测试；仍缺成功响应、409 和 Run Events 的 Handler 测试。关闭条件须补齐两条 Run 路径和响应 Schema，并覆盖剩余关键状态。

### 11.2 CTR-047：异步 CreateTask 请求与领域 RunRequest

固定基线证据：
- internal/analysis/domain/types.go：RunRequest 包含 run_id、task_type、as_of、as_of_mode、universe_id、symbols、portfolio_id、strategy_ids、market_id、四类配置版本及 parameters；TaskType 有 post_market_scan、watchlist_scan、position_review、historical_replay 四个常量。
- doc/api/contracts/分析任务.openapi.yaml：AnalysisRequest 声称字段定义与 domain.RunRequest 一致，但缺少 as_of_mode、parameters，未定义 task_type 四值枚举，也没有 required/default 行为声明。
- internal/platform/httpapi/tasks.go：CreateTask Handler 会填充 task_type、as_of、universe_id、portfolio_id、market_id、strategy_ids 和配置版本默认值；historical_replay 必须显式提供 as_of；as_of_mode 根据 as_of 是否显式提供而被 Handler 覆盖。
- internal/platform/taskqueue/queue.go：Queue.Create 生成 run_id 并覆盖请求中的 req.RunID，再持久化请求。因此 run_id 应定义为服务端生成身份，而不是客户端控制字段。
- Runner 实际读取 parameters 中的 symbols 回退值，parameters 不能从异步请求 Schema 中遗漏。

结论：CTR-047 事实成立，保留 P2。关闭时应定义独立的 CreateTaskRequest 契约，不再宣称与领域 RunRequest 字段完全相同；明确 task_type 枚举、服务端默认、historical_replay 的条件约束、as_of_mode 推导、run_id 生成和 parameters 类型。required 不能机械复制领域结构体字段，应按 HTTP Handler 的默认/条件校验行为定义。

### 11.3 本批范围修正

- 修正 WebUI-维度2-Analysis-Run.md 对 HTTP 测试缺口的表述：现有测试覆盖 GET Run 404 与 500 脱敏；缺口收敛为 GET Run 成功、409 和 Run Events。
- 本批未执行固定基线 Go/Node CI；未检查 RunResult 全部嵌套字段 Golden，也未完成 DB migration/Worker 多实例并发专项审计，这些仍按原计划保留，不以本批替代。
- 本批没有新增 CTR 编号、没有修改业务代码。
## 12. CTR-010 字段清单复核（子批次 5：ChipDistribution）

固定代码基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。本批只核对 ChipDistribution，不扩展其他 DTO。

### 12.1 字段级对照

internal/marketdata/model/model.go 的 model.ChipDistribution 中，以下九个字段均带 omitempty，nil/空值时 encoding/json 省略字段：

| JSON 字段 | Go 类型 | JSON tag | OpenAPI nullable | OpenAPI required |
|---|---|---|---|---|
| date | string | date,omitempty | true | 否 |
| cost_90_low | *float64 | cost_90_low,omitempty | true | 否 |
| cost_90_high | *float64 | cost_90_high,omitempty | true | 否 |
| cost_70_low | *float64 | cost_70_low,omitempty | true | 否 |
| cost_70_high | *float64 | cost_70_high,omitempty | true | 否 |
| profit_ratio | *float64 | profit_ratio,omitempty | true | 否 |
| concentration_90 | *float64 | concentration_90,omitempty | true | 否 |
| concentration_70 | *float64 | concentration_70,omitempty | true | 否 |
| provider_date | string | provider_date,omitempty | true | 否 |

另外 source_version、quality_reasons 也带 omitempty，但 OpenAPI 未声明 nullable，故不纳入 CTR-010。average_cost、symbol、source、fetched_at、quality_status 为 required，与 Go 非 omitempty 字段一致。

### 12.2 Golden / 自动化验证

- openapi_contract_test.go 的 TestOpenAPIDataModelFieldsMatchKnownJSONContract 将 ChipDistribution 纳入 51 个模型登记，能核对属性集合和 required 集合；其 nullable 断言只覆盖非 omitempty 指针字段，不能发现 omitempty 字段被错误声明 nullable 的反向差异。
- TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics 没有 ChipDistribution 样例；当前没有该 DTO 的 nil/空值字段省略 Golden。
- 因而 DTO tag 与 OpenAPI nullable 的九处冲突已由源码和 Schema 直接证实，但没有序列化 Golden 对实际 JSON 行为作独立回归保护。本轮未执行固定基线 CI。

结论：CTR-010 在 ChipDistribution 上的九个字段全部确认，保留原编号，不新增差异。关闭条件需逐字段取消不符合 Go JSON 行为的 nullable 声明（或经明确契约决议改 Go 序列化行为），并增加 DTO nil/空值 Golden 与双向契约测试；不能仅修改 OpenAPI 而不验证消费者对省略字段的处理。

本批仅完成 ChipDistribution 子集，不代表 CTR-010 全量字段清单完成。
## 13. CTR-010 字段清单复核（子批次 6：Profile / Trading Status / Price Limit Rule）

固定代码基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。本批仅核对证券资料、历史交易状态、涨跌幅规则三个响应 DTO。

### 13.1 StockBasic（证券资料/Profile）

Go model.StockBasic 有 14 个 omitempty 字段，但其中只有 12 个字段在 OpenAPI 中声明 nullable=true；ipo_date、out_date 虽为 omitempty 字符串，Schema 未声明 nullable，不纳入 CTR-010：

| JSON 字段 | Go 类型 | JSON tag | OpenAPI nullable |
|---|---|---|---|
| pe_dynamic / pb | *float64 | omitempty | true |
| industry | string | omitempty | true |
| market_cap_cny / float_market_cap_cny | *float64 | omitempty | true |
| total_shares / float_shares | *float64 | omitempty | true |
| board_code | string | omitempty | true |
| roe_percent / net_profit_margin_percent | *float64 | omitempty | true |
| net_profit_cny / gross_margin_percent | *float64 | omitempty | true |

上述 12 个字段在 Go nil/空值时均被省略，而不是输出 JSON null。ipo_date/out_date、source_version、quality_reasons 虽带 omitempty，但 OpenAPI 未声明 nullable，不纳入 CTR-010。StockBasic 的 required 字段与 Go 非 omitempty 字段相符。

### 13.2 SecurityTradingStatusSnapshot

- system_observed_at：*time.Time + omitempty，OpenAPI nullable=true，存在省略/null 语义冲突，计入 CTR-010。
- security_name、name_effective_from/to、listing_date、listing_trading_day_number、suspension_timing/type 均带 omitempty，但 OpenAPI 未声明 nullable，不计入 CTR-010。
- required 集合与 Go 非 omitempty 字段相符；quality_reasons 虽 omitempty，但 Schema 未声明 nullable。

### 13.3 PriceLimitRuleSnapshot 与 PriceLimitEventEvidence

- PriceLimitRuleSnapshot 的 inline 200 response Schema 中，limit_percent 为 *float64 + omitempty，且属性直接声明 type: number、nullable: true，计入 CTR-010。special_exception_evidence 虽在 $ref 同层写有 nullable: true，但 OpenAPI 3.0.3 的 nullable 仅在同一 Schema 显式声明 type 时生效；该字段是可选 $ref，不能作为已证实的 nullable 冲突，不计入。
- listing_trading_day_number、no_limit_reason、rule_effective_from/to 也带 omitempty，但 Schema 未声明 nullable，不计入 CTR-010。
- PriceLimitEventEvidence 的 effective_to、quality_reasons、announcement_id 均 omitempty，Schema 未声明 nullable，不计入 CTR-010。
- required 字段总体与 Go 非 omitempty 字段一致；此结论只针对字段 presence，不代表状态枚举和规则语义的其他审计项已关闭。

### 13.4 测试与回归证据

- openapi_contract_test.go 的 TestOpenAPIDataModelFieldsMatchKnownJSONContract 覆盖 StockBasic、SecurityTradingStatusSnapshot 的字段集合/required 对照；但没有对 omitempty 字段不应 nullable 的反向断言。
- PriceLimitRuleSnapshot 是 Handler inline response Schema，没有对应的具名 Go DTO 注册项；当前测试未建立该 inline Schema 与 Go DTO 的逐字段映射。
- TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics 没有 StockBasic、SecurityTradingStatusSnapshot、PriceLimitRuleSnapshot 或 PriceLimitEventEvidence 的 nil/空值样例。
- 本批未执行固定基线 CI；以上是源码与 Schema 静态对照，不表述为测试运行通过。

### 13.5 CTR-010 本批确认数量

- StockBasic：12 个字段。
- SecurityTradingStatusSnapshot：1 个字段。
- PriceLimitRuleSnapshot：1 个字段（仅 limit_percent）。
- 合计确认 14 个字段，均保留原 CTR-010，不新增差异编号。

本批仅完成上述三个 DTO；CTR-010 全量字段清单仍未完成。
## 14. CTR-010 字段清单复核（子批次 7：Fundamentals / Announcements / Valuations）

固定代码基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。本批核对财务指标、公告记录及估值响应 DTO。

### 14.1 Fundamentals

- FundamentalSection.available_at 为 *time.Time + omitempty，OpenAPI 未声明 nullable，不计入 CTR-010；其缺失语义为未知时省略。
- FundamentalsSnapshot.system_observed_at 为 *time.Time + omitempty，但 OpenAPI 声明 nullable=true，确认 1 个 CTR-010 字段。
- source_version、quality_reasons 也为 omitempty，但 Schema 未声明 nullable，不计入。
- FundamentalMetric 没有 omitempty 字段；字段 required 与 Go JSON 输出一致。

### 14.2 Announcements

- StockAnnouncement.published_at 为 *time.Time + omitempty，OpenAPI 声明 nullable=true，确认 1 个 CTR-010 字段。
- stock_name、title_chinese、title_english、category_code、category、display_time、sort_date、ei_time、language、source_type、source_url、quality_reasons 等 omitempty 字段未声明 nullable，不计入。
- StockAnnouncementSnapshot.quality_reasons 为 omitempty，Schema 未声明 nullable；其余字段均非 omitempty，required 集合与 Go JSON 输出一致。

### 14.3 Valuations

- ValuationItem 五个扩展指针字段均为 omitempty，但 OpenAPI 声明 nullable=true：turnover_rate_percent、float_turnover_rate_percent、volume_ratio、dividend_yield_percent、dividend_yield_ttm_percent。确认 5 个 CTR-010 字段。
- 核心估值字段 pe_ttm、pe_mrq、pe_static、pb_mrq、ps_ttm、pcf_ttm 是非 omitempty 指针，OpenAPI required + nullable，与 Go 输出 null 的行为一致。
- ValuationSnapshot.provider_timestamp 是 *time.Time + omitempty，OpenAPI 未声明 nullable，不计入；missing_symbols、provider_report_date、source_version、quality_reasons 也未声明 nullable。

### 14.4 测试与回归证据

- TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas 已将 StockAnnouncement、StockAnnouncementSnapshot、FundamentalMetric、FundamentalSection、FundamentalsSnapshot、ValuationItem、ValuationSnapshot 纳入 51 个模型的字段集合与 required 反射核对。
- 该反射测试只验证非 omitempty 指针必须 nullable，没有验证 omitempty 字段不得 nullable，因而无法发现本批 7 个差异。
- TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics 未包含上述 DTO 的 nil/空值样例；公告和财务/估值 DTO 缺少相应序列化 Golden。
- 本批未执行固定基线 CI；结论来自 Go DTO、OpenAPI Schema 和测试源码静态对照。

### 14.5 CTR-010 本批确认数量

- Fundamentals：1 个字段。
- Announcements：1 个字段。
- Valuations：5 个字段。
- 合计确认 7 个字段，均归入既有 CTR-010，不新增编号。

本批仅完成上述三个数据集；CTR-010 全量字段清单仍未完成。
## 15. CTR-010 字段清单复核（子批次 8：Market Breadth / Indices / Index Catalog / Constituents / Daily Bars）

固定代码基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。本批核对市场广度、指数行情、指数目录、指数成分及指数日线响应 DTO。

### 15.1 Market Breadth

- MarketBreadthSnapshot.oldest_quote_time、newest_quote_time 均为 *time.Time + omitempty，OpenAPI 声明 nullable=true，确认 2 个 CTR-010 字段。
- limit_up_count、limit_down_count、total_amount_cny 是非 omitempty 指针，OpenAPI required + nullable，Go nil 时输出 null，语义一致。
- excluded_non_a_share_count、source_version、provider_report_date、quality_reasons 虽为 omitempty，但 OpenAPI 未声明 nullable，不计入 CTR-010。

### 15.2 Market Indices

- MarketIndexQuote 的 latest_price、change、change_percent、open、high、low、prev_close、amount、quote_time 共 9 个字段均为 omitempty，OpenAPI 声明 nullable=true，确认 9 个 CTR-010 字段。
- IndexQuoteSnapshot 的 source_version、quality_reasons 为 omitempty，Schema 未声明 nullable；无 CTR-010 差异。

### 15.3 Index Catalog

- IndexCatalogSnapshot.provider_time 为 *time.Time + omitempty，OpenAPI nullable=true，确认 1 个 CTR-010 字段。
- source_version、quality_reasons 为 omitempty，但 Schema 未声明 nullable；IndexCatalogItem 无 omitempty 字段。

### 15.4 Index Constituents

- IndexConstituent.weight_percent 为 *float64 + omitempty，OpenAPI nullable=true，确认 1 个 CTR-010 字段。
- IndexConstituent 的 effective_from、effective_to、available_at、system_observed_at 均为 omitempty，但 Schema 未声明 nullable，不计入。
- IndexConstituentSnapshot.provider_time 为 *time.Time + omitempty，OpenAPI nullable=true，确认 1 个 CTR-010 字段。
- as_of_date、provider_report_date、excluded_count、total_is_lower_bound、source_version、quality_reasons 均未声明 nullable，不计入。

### 15.5 Index Daily Bars

- IndexDailyBar.change_percent、volume、amount 三个 omitempty 字段及 provider_time 一个 omitempty 字段，OpenAPI 均声明 nullable=true，确认 4 个 CTR-010 字段。
- IndexDailyBarsSnapshot.provider_time 为 *time.Time + omitempty，OpenAPI nullable=true，确认 1 个 CTR-010 字段。
- source_version、quality_reasons 虽为 omitempty，但 Schema 未声明 nullable，不计入。

### 15.6 测试与回归证据

- TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas 已将 MarketIndexQuote、IndexQuoteSnapshot、MarketBreadthSnapshot、IndexCatalogItem、IndexCatalogSnapshot、IndexConstituent、IndexConstituentSnapshot、IndexDailyBar、IndexDailyBarsSnapshot 纳入 51 个模型的字段集合/required 反射核对。
- 反射测试仅验证非 omitempty 指针必须 nullable，没有验证 omitempty 字段不得 nullable，不能发现本批 19 个差异。
- TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics 未包含本批任何 DTO 的 nil/空值样例；缺少对应的序列化 Golden。
- 本批未执行固定基线 CI；结论来自 Go DTO、OpenAPI Schema 与测试源码静态对照。

### 15.7 CTR-010 本批确认数量

- Market Breadth：2 个字段。
- Market Indices：9 个字段。
- Index Catalog：1 个字段。
- Index Constituents：2 个字段。
- Index Daily Bars：5 个字段。
- 合计确认 19 个字段，全部归入既有 CTR-010，不新增差异编号。

截至子批次 8，CTR-010 专门分批清单累计核对 52 个字段（此前 33 个 + 本批 19 个）。该数字不含此前在其他数据集审计章节中零散登记的 CTR-010 字段；最终总数需在字段总表中去重汇总。本批仅覆盖上述五组 DTO，不代表 CTR-010 全量字段清单完成。

## 16. CTR-010 字段清单复核（子批次 9：Quote / DailyBar / Bundle / StockBoard / SectorRanking）

固定代码基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086。本批核对核心行情、日线、数据包、个股板块和行业/概念排行 DTO，并对照 OpenAPI Schema 与现有模型测试。

### 16.1 Quote

- Quote.volume_ratio、turnover_rate、raw_volume、raw_amount 共 4 个字段使用 omitempty，OpenAPI 声明 nullable=true，确认 4 个 CTR-010 字段。
- current_price、change、change_percent、open、high、low、prev_close、volume、amount、quote_time 均为非 omitempty 指针，Schema required + nullable，Go nil 时输出 null，语义一致。
- name、quote_time_semantics、source_version、raw_volume_unit、raw_amount_unit、raw_amount_field、quality_reasons、fallback_from、fallback_reason、request_id 等 omitempty 字段未声明 nullable，不计入 CTR-010。

### 16.2 DailyBar

- adjustment_factor_system_observed_at、provider_timestamp、system_observed_at、raw_volume、raw_amount 共 5 个字段使用 omitempty，OpenAPI 声明 nullable=true，确认 5 个 CTR-010 字段。
- ma5、ma20、ma120、ma250、volume、amount、change_percent 均为非 omitempty 指针，Schema required + nullable，语义一致。
- is_partial、adjustment_method、adjustment_reference_date、adjustment_factor_source、adjustment_factor_source_version、source_version、raw_volume_unit、raw_amount_unit、raw_amount_field、quality_reasons、fallback_from、fallback_reason、request_id 等 omitempty 字段未声明 nullable，不计入。

### 16.3 Bundle

- Bundle.quote 为 *Quote + omitempty，OpenAPI 使用非 nullable 的可选 $ref；nil 时字段省略，与 Go 行为一致。
- name、warnings 为 omitempty 且 Schema 非 nullable；无 CTR-010 差异。

### 16.4 StockBoard

- StockBoard.change_percent 为 *float64 + omitempty，OpenAPI nullable=true，确认 1 个 CTR-010 字段。
- source_version、quality_reasons 为 omitempty，但 Schema 未声明 nullable；StockBoardSnapshot 的 source_version、quality_reasons 同样未声明 nullable。

### 16.5 SectorRanking

- SectorRanking.latest_price、change、change_percent、change_percent_5d、amount、advancing_count、declining_count、leader_change_percent 共 8 个 omitempty 指针字段，OpenAPI 均声明 nullable=true，确认 8 个 CTR-010 字段。
- leading_stock、provider_report_date、source_version、quality_reasons 为 omitempty，但 Schema 未声明 nullable，不计入。
- SectorRankingSnapshot 的 sort_by、provider_returned、excluded_rows、excluded_non_a_share_count、total_is_lower_bound、provider_report_date、source_version、quality_reasons 为 omitempty，Schema 均未声明 nullable；无 CTR-010 差异。

### 16.6 测试与回归证据

- TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas 已将 Quote、DailyBar、Bundle、StockBoard、StockBoardSnapshot、SectorRanking、SectorRankingSnapshot 纳入 51 个模型的字段集合/required 反射核对。
- 反射测试仍只检查非 omitempty 指针必须 nullable，没有检查 omitempty 字段不得 nullable，不能发现本批 18 个差异。
- TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics 已覆盖 Quote 的核心非 omitempty 指针 null 行为、DailyBar 的核心指针 null 行为及 Bundle.quote nil 时省略；但未覆盖本批 omitempty + nullable 的 18 个字段在 nil/空值时应省略的情况。
- StockBoard、SectorRanking 及其 Snapshot 未加入该 JSON 序列化测试。
- 本批未执行固定基线 CI；结论来自 Go DTO、OpenAPI Schema 与测试源码静态对照。

### 16.7 CTR-010 本批确认数量

- Quote：4 个字段。
- DailyBar：5 个字段。
- Bundle：0 个字段。
- StockBoard：1 个字段。
- SectorRanking：8 个字段。
- 合计确认 18 个字段，全部归入既有 CTR-010，不新增差异编号。

截至子批次 9，CTR-010 专门分批清单经子批次 6 计数纠偏后累计核对 67 个字段（此前 49 个 + 本批 18 个）。该数字不含此前其他数据集章节零散登记的 CTR-010 字段；最终唯一字段数以第 20 节为准。


### WebUI — 维度 2：CTR-010 字段清单复核（子批次 10）

- 固定代码基线：`origin/integration` `b6dfc85ba214fe16268c8cba780641a8bfe15086`。本批只复核 Limit Pool、Dragon Tiger、Market Sentiment、Market Capital Flow 与个股 Capital Flow 的 Go JSON tag、OpenAPI nullable/required 和现有测试，不修改业务代码。
- LimitPoolItem：`last_price`、`consecutive_boards`、`broken_count`、`open_times`、`turnover_ratio_percent`、`turnover` 共 6 个 omitempty 字段仍在 OpenAPI 声明 nullable；LimitPoolSnapshot.provider_report_date 另 1 个字段同样属于既有 CTR-010。seal_money/max_seal_money 虽为 omitempty，但 OpenAPI 未声明 nullable，且现有序列化测试要求省略，明确不计入 CTR-010。
- DragonTigerItem：16 个 omitempty 指针字段（change_percent、close_price_cny、net_buy_amount_cny、buy_amount_cny、sell_amount_cny、deal_amount_cny、deal_amount_ratio、provider_buy_value、provider_sell_value、provider_deal_value、provider_net_value、provider_net_rate、provider_org_net_value、provider_hot_money_net_value、hot_rank、range_days）均被 OpenAPI 声明 nullable；DragonTigerItem.provider_amount_unit 与 DragonTigerSnapshot.provider_report_date 两个 omitempty 字符串也声明 nullable。共 18 个，均是此前龙虎榜章节已登记的 CTR-010 影响字段。
- MarketSentimentSnapshot / MarketSentimentHistory：本批未发现新的 omitempty + OpenAPI nullable 冲突。provider_report_date、三个 lower-bound bool、pool_sources、pool_source_versions、source_version、quality_reasons 均为 omitempty，但 OpenAPI 未声明 nullable；三个计数指针字段不带 omitempty，且在 OpenAPI required + nullable，行为一致。MarketSentimentHistory 所有字段均非 omitempty，未发现 CTR-010 差异。
- MarketCapitalFlowItem：14 个 omitempty 数值指针字段（close_sh、change_percent_sh、close_sz、change_percent_sz、net_amount_cny、net_amount_rate_percent，以及超大单/大单/中单/小单净流入金额和比例字段）均在 OpenAPI 声明 nullable，属于此前市场级资金流章节登记的 CTR-010。MarketCapitalFlowSnapshot.quality_reasons 虽 omitempty，但 OpenAPI 未声明 nullable，不计入。
- CapitalFlow：main_net_inflow_yuan、inflow_5d_yuan、inflow_10d_yuan 三个 omitempty 指针字段均被 OpenAPI 声明 nullable，属于此前个股资金流章节登记的 CTR-010；provider_date、source_version、quality_reasons 虽 omitempty，但 OpenAPI 未声明 nullable。
- 本批复核到 42 个既有 CTR-010 字段：Limit Pool 7、Dragon Tiger 18、Market Capital Flow 14、CapitalFlow 3；Market Sentiment 0。与此前数据集章节的登记重合，不新增差异编号，也不应将 42 再次累加到专门分批统计。seal_money/max_seal_money 等 omitempty 但 Schema 非 nullable 的字段排除。
- 测试证据：`TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas` 已将 DragonTigerItem/Snapshot、MarketSentimentSnapshot/History、LimitPoolItem/Snapshot、MarketCapitalFlowItem/Snapshot、CapitalFlow 纳入 51 个模型字段集合与 required 反射核对；该测试只对非 omitempty 指针检查 nullable，未检查 omitempty 字段的反向 nullable 冲突。
- `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics` 仅对 MarketSentimentSnapshot 的三个非 omitempty 计数指针做 null 断言，对 LimitPoolItem 的 change_percent 做 null、seal_money/max_seal_money 做省略断言；没有覆盖本批 42 个差异字段的 nil 省略。Dragon Tiger、Market Capital Flow、CapitalFlow 没有对应 DTO 序列化样例。
- 本批未执行固定基线 CI；结论为静态代码与测试资产核验，不代表 CI 已通过。无业务代码修改。CTR-010 仍需完成全 DTO 去重汇总及反向序列化断言缺口归档。


> **计数更正（子批次 10）**：本节原记 41 个字段，分项实际为 Limit Pool 7 + Dragon Tiger 18 + Market Capital Flow 14 + CapitalFlow 3 = **42 个字段**。已按分项更正；这 42 个字段均为历史重复核验，不计入子批次 5–9 的 70 个专门分批字段累计。


## 18. CTR-010 字段清单复核（子批次 11：Provider Probe DTO）

固定代码基线：`origin/integration` `b6dfc85ba214fe16268c8cba780641a8bfe15086`。本批针对此前 Provider Probe History 审计未覆盖的嵌套响应 DTO，检查 Go JSON tag、OpenAPI nullable/required 与模型测试覆盖，不重复审查 CTR-048/049 的分页及错误映射。

### 18.1 ProviderProbeFieldComparison

- `left`、`right`、`difference`、`tolerance` 均为 `*float64 + omitempty`；OpenAPI 的 `ProviderProbeCheckResult.field_comparisons.items` 将四个字段声明为 `nullable: true`。
- nil 时 Go JSON 会省略字段，而非输出 null，确认 4 个 CTR-010 字段。
- `reason` 为 omitempty 字符串，但 Schema 未声明 nullable，不计入。

### 18.2 ProviderProbeCheckResult

- `provider_time`、`fetched_at` 均为 `*time.Time + omitempty`；OpenAPI 声明 nullable=true，确认 2 个 CTR-010 字段。
- 其他 omitempty 字段（provider、capability、错误信息、日历/交易状态、年龄、市场阶段、Provider 标识、质量状态、日期集合、计数及 field_comparisons）未声明 nullable，不计入。

### 18.3 ProviderProbeItem

- `coverage_ratio` 为 `*float64 + omitempty`，OpenAPI nullable=true；`provider_time`、`fetched_at` 为 `*time.Time + omitempty`，OpenAPI 均 nullable=true。确认 3 个 CTR-010 字段。
- `coverage_total`、`coverage_returned`、`coverage_validated`、`coverage_missing`、`coverage_excluded` 虽为 omitempty 指针，但 OpenAPI 未声明 nullable，不计入；其他 omitempty 字段亦未声明 nullable。

### 18.4 测试覆盖与边界

- `TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas` 将 ProviderProbeCheckResult、ProviderProbeItem 纳入 51 个模型字段集合/required 检查，但仅检查非 omitempty 指针必须 nullable，不会发现本批 omitempty→nullable 冲突。
- `ProviderProbeFieldComparison` 是 `ProviderProbeCheckResult.field_comparisons` 的嵌套元素，未单独加入 51 个模型注册表；反射测试只检查父字段名，不递归检查嵌套元素字段。
- `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics` 没有 ProviderProbeItem、ProviderProbeCheckResult 或 ProviderProbeFieldComparison 的序列化样例；本批 9 个字段缺 nil 省略断言。
- 本批确认 9 个此前未登记字段：ProviderProbeFieldComparison 4、ProviderProbeCheckResult 2、ProviderProbeItem 3。此前 Provider Probe History 的 CTR-048/049 章节没有登记这些字段，故本批是新增影响字段，不是重复核验；仍归既有 CTR-010，不新增编号。
- 补充边界盘点：MarketStockOverviewItem 以 MarketStocksSnapshot.items 内联对象形式暴露，ma5/ma20 为非 omitempty 指针且 Schema nullable=true，quote 为 omitempty 可选引用且 Schema 非 nullable，未发现 CTR-010 冲突；该内联 DTO 未独立加入模型反射注册表。AdjustmentFactor、CorporateAction、StockUniverseSnapshot 未在当前 Market Data OpenAPI 中找到对应响应 Schema；ProviderProbeOptions 是无 JSON tag 的内部选项结构，不纳入当前 HTTP 响应 CTR-010 清单。PriceLimitEventEvidence 的 omitempty 字段此前第13节已核验，未发现 nullable 冲突。
- 本批未执行固定基线 CI；结论来自固定 SHA 下 Go DTO、OpenAPI 和测试源码静态对照。无业务代码修改。


## 19. CTR-010 字段清单复核（子批次 12：Hot Ranking）

固定代码基线：`origin/integration` `b6dfc85ba214fe16268c8cba780641a8bfe15086`。本批仅核验 Hot Ranking DTO 中尚需纳入 CTR-010 汇总的空值字段，不重复复审 Provider、分页、默认启用项或页面消费逻辑。

### 19.1 字段对照

- `HotRankingItem.latest_price`、`change`、`change_percent`、`rank_change`、`heat`：均为指针类型且带 `omitempty`；OpenAPI `HotRankingItem` 对应属性均声明 `nullable: true`。nil 时 Go JSON 省略字段，确认 5 个 CTR-010 字段。
- `HotRankingSnapshot.provider_time`：`*time.Time + omitempty`；OpenAPI 声明 `nullable: true`，确认 1 个 CTR-010 字段。
- 本批合计 6 个字段。均归入既有 CTR-010，不新增差异编号。HotRankingItem.name/rank_trend 等其他可选字段的 OpenAPI 未声明 nullable，不计入。

### 19.2 测试覆盖

- `TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas` 已登记 HotRankingItem/HotRankingSnapshot，能核对字段集合和 required，但没有 omitempty→nullable 的反向约束。
- `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics` 已覆盖 HotRankingItem 的基础必填字段及 name/rank_trend/source_version 省略，但没有断言五个 nullable 冲突字段在 nil 时省略；也没有 HotRankingSnapshot 序列化样例，未覆盖 provider_time 的省略行为。
- 测试中已有对 HotRankingItem.name、rank_trend 非 nullable 的局部断言，但不覆盖本批六个字段。
- 本批未执行固定基线 CI；结论来自固定 SHA 的 DTO、OpenAPI 和测试源码静态对照。无业务代码修改。

### 19.3 本批结论

Hot Ranking 新确认 6 个 CTR-010 影响字段（5 + 1），属于此前 CTR-010 全量字段去重汇总中的待纳入项。后续统一总表时应按 `DTO.field` 去重；不得把历史数据集章节已列出的相同字段重复累计。


## 20. CTR-010 全量唯一字段汇总（固定基线静态对照）

### 20.1 统计口径

固定代码基线：`origin/integration` `b6dfc85ba214fe16268c8cba780641a8bfe15086`。本节以固定基线下 `internal/marketdata/model/**/*.go`、`internal/marketdata/httpapi/openapi.yaml` 和 `openapi_contract_test.go` 为依据，按 `DTO.JSON字段` 作为唯一键重新计算；不将子批次累计数、数据集章节提及次数直接相加。

- 反射注册表：51 个具名 Go DTO；逐一比对 Go JSON tag 与对应 OpenAPI properties，共确认 119 个 `omitempty + nullable: true` 字段。
- 注册表外的 HTTP 响应 DTO：`PriceLimitRuleSnapshot`（Handler inline 200 Schema，确认 `limit_percent` 1 个字段）、`ProviderProbeFieldComparison`（嵌套 inline Schema，确认 4 个字段）、`MarketStockOverviewItem`（嵌套 inline Schema，0 个冲突字段）。
- 因此本轮覆盖 54 个 HTTP 响应 DTO（51 个具名注册 DTO + 3 个 inline DTO），确认 **124 个唯一 CTR-010 字段**，分布在 29 个 DTO；其余 25 个 DTO 未发现该类差异。
- `AdjustmentFactor`、`CorporateAction`、`StockUniverseSnapshot` 当前没有对应 Market Data HTTP 响应 Schema；`ProviderProbeOptions` 是无 JSON tag 的内部请求选项结构，不纳入响应 CTR-010 统计。
- `PriceLimitRuleSnapshot.special_exception_evidence` 是可选 $ref；OpenAPI 3.0.3 中同层 nullable 没有同层 type，不能据此认定允许 null，故排除。StockBasic.ipo_date/out_date 虽 omitempty，但 OpenAPI 未声明 nullable，也排除。

### 20.2 唯一差异字段清单

以下清单按 DTO 分组；每个字段只出现一次。合计 124 项。

| DTO | CTR-010 唯一字段 | 数量 |
|---|---|---:|
| StockAnnouncement | published_at | 1 |
| DragonTigerItem | change_percent、close_price_cny、net_buy_amount_cny、buy_amount_cny、sell_amount_cny、deal_amount_cny、deal_amount_ratio、provider_buy_value、provider_sell_value、provider_deal_value、provider_net_value、provider_net_rate、provider_org_net_value、provider_hot_money_net_value、provider_amount_unit、hot_rank、range_days | 17 |
| DragonTigerSnapshot | provider_report_date | 1 |
| Quote | volume_ratio、turnover_rate、raw_volume、raw_amount | 4 |
| DailyBar | adjustment_factor_system_observed_at、provider_timestamp、system_observed_at、raw_volume、raw_amount | 5 |
| StockBoard | change_percent | 1 |
| StockBasic | pe_dynamic、pb、industry、market_cap_cny、float_market_cap_cny、total_shares、float_shares、board_code、roe_percent、net_profit_margin_percent、net_profit_cny、gross_margin_percent | 12 |
| SectorRanking | latest_price、change、change_percent、change_percent_5d、amount、advancing_count、declining_count、leader_change_percent | 8 |
| MarketIndexQuote | latest_price、change、change_percent、open、high、low、prev_close、amount、quote_time | 9 |
| MarketBreadthSnapshot | oldest_quote_time、newest_quote_time | 2 |
| LimitPoolItem | last_price、consecutive_boards、broken_count、open_times、turnover_ratio_percent、turnover | 6 |
| LimitPoolSnapshot | provider_report_date | 1 |
| FundamentalsSnapshot | system_observed_at | 1 |
| ValuationItem | turnover_rate_percent、float_turnover_rate_percent、volume_ratio、dividend_yield_percent、dividend_yield_ttm_percent | 5 |
| IndexCatalogSnapshot | provider_time | 1 |
| IndexConstituent | weight_percent | 1 |
| IndexConstituentSnapshot | provider_time | 1 |
| IndexDailyBar | change_percent、volume、amount、provider_time | 4 |
| IndexDailyBarsSnapshot | provider_time | 1 |
| HotRankingItem | latest_price、change、change_percent、rank_change、heat | 5 |
| HotRankingSnapshot | provider_time | 1 |
| ChipDistribution | date、cost_90_low、cost_90_high、cost_70_low、cost_70_high、profit_ratio、concentration_90、concentration_70、provider_date | 9 |
| MarketCapitalFlowItem | close_sh、change_percent_sh、close_sz、change_percent_sz、net_amount_cny、net_amount_rate_percent、extra_large_net_inflow_cny、extra_large_net_inflow_rate_percent、large_net_inflow_cny、large_net_inflow_rate_percent、medium_net_inflow_cny、medium_net_inflow_rate_percent、small_net_inflow_cny、small_net_inflow_rate_percent | 14 |
| CapitalFlow | main_net_inflow_yuan、inflow_5d_yuan、inflow_10d_yuan | 3 |
| ProviderProbeItem | coverage_ratio、provider_time、fetched_at | 3 |
| ProviderProbeCheckResult | provider_time、fetched_at | 2 |
| SecurityTradingStatusSnapshot | system_observed_at | 1 |
| PriceLimitRuleSnapshot（inline） | limit_percent | 1 |
| ProviderProbeFieldComparison（inline） | left、right、difference、tolerance | 4 |
| **合计** | **29 个 DTO** | **124** |

### 20.3 既有批次计数纠偏与重复核验

- 子批次 6 原记 StockBasic 14 个，实际只有 12 个字段满足 omitempty + OpenAPI nullable=true；ipo_date/out_date 的 Schema 没有 nullable。PriceLimitRuleSnapshot 原记 2 个，special_exception_evidence 的 nullable 位于 $ref 旁且没有同层 type，不计入已证实差异；仅 limit_percent 计入。因此子批次 6 从 17 更正为 14。
- 子批次 5—9 原累计 70 个，按上述纠偏后为 **67 个**（9 + 14 + 7 + 19 + 18）。这是批次记录量，不是最终唯一字段数。
- 子批次 10 的 42 个是既有字段重复复核；子批次 12 的 Hot Ranking 6 个也已在早期 Hot Ranking 数据集章节登记，属于重复复核，不可再次累加。
- 子批次 11 的 9 个由 ProviderProbeItem 3、ProviderProbeCheckResult 2 和嵌套 ProviderProbeFieldComparison 4 组成；其中 5 个具名 DTO 字段已包含在 51 模型静态扫描的 119 项内，4 个嵌套 DTO 字段是注册表外新增补齐项。
- 旧审计记录中同一字段以 DTO 分组、无前缀字段名和 CTR-010 影响范围补充等不同方式多次提及；本节给出最终唯一集合。历史章节的提及次数不等于独立差异数，后续以本节清单作为唯一计数源，不再累计旧批次字段数。

### 20.4 测试覆盖汇总

- `TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas` 登记 51 个 DTO，检查 Go JSON 字段集合、required 集合，并检查非 omitempty 指针必须 nullable；没有反向检查 omitempty 字段不得 nullable。因此 119 个具名 DTO 差异不能由现有反射测试自动发现。
- PriceLimitRuleSnapshot、ProviderProbeFieldComparison、MarketStockOverviewItem 三个 inline DTO 未加入该 51 项注册表；嵌套 DTO 也没有递归字段/nullable 对照。
- `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics` 当前只对 Quote、DailyBar、Bundle、HotRankingItem、MarketSentimentSnapshot、LimitPoolItem、SourceStatus、StockAnnouncementSnapshot 建立样例；其中没有对本节 124 个差异字段中的任一字段做 nil/空值省略断言。
- 因此当前公共 JSON Golden 对 CTR-010 的 124 个字段覆盖为 **0/124**；这不表示项目内不存在任何其他针对性测试，而是中央 DTO/OpenAPI 序列化契约测试没有覆盖这些字段。
- 本轮是固定 SHA 的静态字段清单核验，未执行 Go/Node CI，也未修改业务代码。

### 20.5 当前结论

CTR-010 的静态字段完整性清单已收敛为 **124 个唯一字段 / 29 个受影响 DTO / 54 个纳入范围的响应 DTO**。此前子批次累计数字不再用于推导总数。此结论只完成字段清单和测试资产盘点，不代表 CTR-010 已修复，也不代表 WebUI 维度 2 或 PLAT-002 已完成；下一步进入 CTR-010 关闭方案 Review，先确定 OpenAPI 还是 Go JSON 行为作为规范目标，再定义双向反射校验和最小化 Golden 补齐策略。


## 21. CTR-010 契约差异处置边界 Review

### 21.1 已确认事实（不等同于契约裁决）

- 固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 下，CTR-010 静态清单为 124 个唯一字段、29 个受影响 DTO、54 个纳入范围的响应 DTO。
- 这些字段的 Go JSON tag 带 `omitempty`，对应 OpenAPI 属性声明 `nullable: true`；现有 Go 序列化路径在 nil/空值时省略字段，而非输出 JSON null。
- 当前中央反射测试没有覆盖 omitempty -> nullable 的反向约束；中央 JSON Golden 对这 124 个字段的 nil/空值省略行为覆盖为 0/124。
- 上述内容证明实现描述与 OpenAPI 声明之间存在差异，但不能单凭现有实现行为推导产品期望的公共契约语义。

### 21.2 候选方案（尚未形成最终裁决）

此前提出的候选方案是 optional + non-nullable：保留 Go DTO 的 `omitempty` 行为，将 124 个 OpenAPI 属性的 `nullable: true` 移除，并补齐反向反射约束及 JSON 序列化测试。

该方案目前只能作为候选，不能记录为已裁决，原因是：
- 需要确认公共 API 是否明确承诺允许消费者发送/接收 null，还是仅允许字段缺失；
- 需要确认契约权威文档中对这些字段的定义是否已有更高优先级的语义约束；
- 需要从消费者兼容性角度确认移除 nullable 是否属于对既有契约的收窄，而不是无影响的文档清理。

因此，本轮不修改 OpenAPI、不修改 Go DTO、不修改测试，也不要求 Data/API 责任分支实施该方案。

### 21.3 冻结前处置要求

CTR-010 在契约冻结前需要完成的是语义裁决，而不是立即修复代码：
1. 对照项目权威契约，确认这 124 个字段的目标语义：字段缺失是否允许、JSON null 是否允许、非 nil 零值如何表达。
2. 若权威契约已有明确规定，以权威规定为准；若没有，形成明确决议并记录兼容性影响。
3. 将裁决写入唯一权威契约文档，并在审计台账记录对应 DTO/字段范围。
4. 把实现与冻结契约之间的差异登记为实现对齐任务，明确责任模块和后续验收条件；除非该差异导致契约含义无法确定或存在冻结门禁风险，否则不在本轮直接改实现。
5. 冻结评审时分别报告“契约语义已确定”和“当前实现是否符合”，不得用其中一个状态替代另一个状态。

### 21.4 本轮结论与状态

- CTR-010：**差异事实已确认；124 字段清单已完成静态去重；目标契约语义待最终裁决；实现修复未启动。**
- optional + non-nullable 仅为候选方案，不是已批准决议。
- 本轮不修改 Data 代码、OpenAPI、契约测试或其他业务实现；不切换分支、不合并其他分支、不运行实现修复 CI。
- WebUI 维度 2 第一轮完整性 Review 仍未完成。后续继续核对现有 CTR 差异的证据、责任、契约裁决状态和冻结影响，不增加新的数据集，也不把审计任务转成模块实现任务。

## 22. Dragon Tiger：CTR-043 / CTR-044 冻结影响复核

本批只复核既有 CTR-043、CTR-044 的契约含义、实现偏差和冻结影响；不修改 Data 实现，不新增差异编号。

### 22.1 CTR-043：Provider 分页计数实现不符合既有契约

固定基线证据（详见 `WebUI-维度2-Dragon-Tiger.md` 第 2、3、4、6 节）：
- 公共 DTO 语义已经定义 `source_returned` 为“本页 Provider 原始记录数”，并要求 `source_returned = returned + excluded_non_a_share_count`。
- Service 校验进一步将本页预期原始数定义为 `min(page_size, total-(page-1)*page_size)`。
- Hithink Provider 却以整个 `stock_items` 数组长度填充 `SourceReturned`，再在 Provider 内存分页；重复 symbol 被静默跳过，但没有合法的重复记录计数语义。
- 这不是公共契约缺失，而是 Hithink Adapter 对现有契约的实现不符合；当前已知影响是 Hithink 大于单页容量时无法通过 Service 校验，回退能力受损。

冻结影响：
- **不要求为了冻结而改变 `source_returned` 的既有公共语义。**
- CTR-043 保留 P1，作为实现符合性缺陷；冻结评审必须明确记录其延期/修复决议、责任方和验收条件。若没有明确延期决议，不得将该 P1 从冻结门禁中静默移除。
- 后续实现验收应覆盖第一页、中间页、末页、大于 page_size 和重复 symbol；重复项不能伪装成非 A 股排除数。

### 22.2 CTR-044：Provider 历史日期能力的公开边界未定义

固定基线证据（详见 `WebUI-维度2-Dragon-Tiger.md` 第 3、4、6 节）：
- DataService、Handler 和公共 OpenAPI 接受任何格式有效且不晚于当前交易日的日期。
- Hithink Provider 仅支持最近一年；超出范围时 Provider 报错。
- 该限制是 Provider 能力差异，不应直接收窄整个龙虎榜公共 API 的日期范围；默认 Hithink disabled，启用为回退源时才可能影响较早日期。

冻结影响：
- CTR-044 保留 P2，属于 Provider 能力边界的契约披露/可观察性问题。
- 冻结前需明确选择：公共契约是否通过 Provider capability/诊断元数据表达各 Provider 支持范围，还是由权威 Provider 能力文档统一定义并要求运行诊断可查询。
- 不把 Hithink 的一年限制改成公共 API 全局限制；本轮不新增字段、不修改 Provider 或 API。
- 在能力边界的权威定义确定前，CTR-044 仍是待裁决项。

### 22.3 本批结论

- CTR-043：公共语义明确、实现不符合；P1 保留，要求冻结评审显式处置，不在本轮修复。
- CTR-044：Provider 能力范围未形成明确的权威披露方式；P2 待契约裁决，不在本轮实现。
- 本批未执行 CI、未修改业务代码、未新增 CTR 编号。WebUI 维度 2 第一轮完整性 Review 继续进行。

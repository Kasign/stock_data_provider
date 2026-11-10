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

## 23. Market Sentiment History：CTR-038～CTR-040 冻结影响 Review

本批只复核既有 CTR-038、CTR-039、CTR-040。审计代码基线仍为 `b6dfc85ba214fe16268c8cba780641a8bfe15086`；不新增 CTR 编号、不重新扫描 DTO，也不修改 Data/WebUI 实现。

### 23.1 CTR-038：total 的统计范围与截断语义未定义

固定基线证据：
- `internal/marketdata/service/market_sentiment.go` 的 `DataService.MarketSentimentHistory` 在 Repository 已按 limit 取数后设置 `Total: len(items)`。
- `internal/marketdata/repository/market_sentiment.go` 对每个 trade_date 只选一条记录，再按 trade_date DESC 排序并应用 SQL LIMIT。
- `MarketSentimentHistory.total` 在 OpenAPI 中仅声明 integer，没有描述它是“范围内总记录数”还是“本次返回条数”。
- WebUI `market-overview.js` 固定传 `limit: 120`，状态文案使用 `items.length/total`。当范围内有超过 120 个交易日快照时，服务端只返回 120 条且 total 也等于 120，页面无法识别截断。
- 需要限定影响边界：HTTP 日期范围最多 366 个自然日、每个交易日最多一条；API 默认 limit=366、最大 500，因此采用默认 limit 时最多 366 条，理论上足以覆盖完整范围。当前实际截断风险主要来自 WebUI 主动传入的 120，而不是所有 API 请求必然截断。

冻结影响与裁决建议：
- 这是公共响应字段 `total` 的语义缺失，不只是页面文案问题；归入既有 `DATA-CTR-001`（HTTP DTO/响应语义）及 `GOV-CTR-001`（JSON 字段语义）核验范围，不新增项目级契约编号。
- 建议将 `total` 定义为指定闭区间内符合条件的不同 trade_date 快照总数（应用 limit 前统计）。这样 `items.length < total` 可直接表示当前响应未返回完整集合；在最大 366 天范围内，客户端可将 limit 提高至 366/500 获取完整结果，不必为本接口额外引入 cursor/page。
- 另一可选方案是将 total 明确定义为 returned_count，但此时不能再把它用于表示全量或判断截断，WebUI 必须移除 N/N 完整性暗示，并另行定义是否需要 has_more。
- 上述为审计建议，不是已批准决议。项目负责人/契约 Owner 需要确认 total 的目标含义后，才可写入权威登记簿和 Data HTTP 契约。
- 状态：**契约语义待裁决**；冻结前必须明确。当前 total 实现与“范围总数”候选语义不符，但若最终裁决为 returned_count，则属于现有实现可保留、仅需补充明确定义的情况。

### 23.2 CTR-039：每日“最新快照”的选择顺序不一致

固定基线证据：
- `SaveMarketSentiment` 每次采集均 INSERT，允许同一 market/trade_date 保存多次；同时保存 `fetched_at`。
- `ListMarketSentiment` 使用 `MAX(id)` 为每个 trade_date 选择一条记录，按 trade_date DESC 输出。
- Repository 注释将其描述为“每一天最近一次采集的快照”，但自增 id 表达插入顺序，不表达 fetched_at 时间先后。并发采集或写入延迟可能导致较早 fetched_at 的记录后插入并被选中。
- `fetched_at` 是快照 DTO 的必填采集时间，表中也有对应列；现有查询没有使用该列进行选择。
- OpenAPI 对每个日期只返回一条历史快照，但没有定义多条同日记录时哪一条胜出。

冻结影响与裁决建议：
- 该问题同时触及既有 `DATA-CTR-001` 的历史响应选择语义和 `GOV-CTR-002` 的时间字段语义，不新增正式契约编号。
- 建议将“最近一次采集”定义为同一 market/trade_date 下 `fetched_at` 最大的记录；若 fetched_at 相同，以 id DESC 作为确定性 tie-breaker。这样与 DTO 中 fetched_at 的语义及 Repository 注释一致。
- 若项目实际希望表达“最后成功写入”，则必须将该语义明确写入契约，并修正“最近一次采集”的注释/页面说明，避免将写入顺序误称为采集时间顺序。
- 当前没有针对该 Repository 查询的乱序写入/并发选择测试；该测试是未来实现符合性验收证据，不是本轮必须执行的代码修复。
- 状态：**最新记录选择语义待裁决**；在选择顺序未明确前，不应冻结该历史接口的同日记录选择规则。

### 23.3 CTR-040：366 自然日限制已在公共 HTTP 契约明确，WebUI 未对齐

固定基线证据：
- OpenAPI 路径说明明确 start/end 闭区间最多 366 个自然日；Handler 使用 `inclusiveCalendarDays` 校验，并有 366 日通过、367 日返回 400 的边界测试。
- WebUI 的 `validateMarketDateRange(start,end,maxCalendarDays)` 已支持最大自然日参数，但 `loadSentimentHistory` 调用时未传 maxCalendarDays，因此 367 日范围在浏览器端可通过校验，发出请求后才被服务端拒绝。
- WebUI 实际请求固定 limit=120；该事实与 CTR-038 的截断风险相关，但不合并 CTR-038/CTR-040：前者是 total/截断语义，后者是输入边界的消费者校验不一致。
- DataService 内部使用 `end.Sub(start) > 366*24h`，与 Handler 的闭区间自然日计数存在一日边界差异：例如 367 个包含首尾的自然日，其日期差正好为 366 天，Service 该判断不会拒绝。当前 HTTP Handler 会先拒绝该请求，因此已证实的外部 HTTP 路径仍执行 366 日限制；若未来有绕过该 Handler 的 InProcess/Application 调用，则需要统一校验。当前尚无证据证明存在该绕行消费者，故不另立缺陷编号。
- WebUI 测试覆盖了通用日期校验器的显式 max 参数，也特意验证不传 max 时不施加 2000 日限制；没有针对情绪历史调用必须传 366 的测试。

冻结影响与裁决建议：
- 366 个自然日、闭区间的公共限制已有 OpenAPI、Handler 和边界测试三方证据，**公共规则本身不待裁决**；属于 `DATA-CTR-001` 输入约束的一部分。
- CTR-040 应分类为 WebUI 消费者实现未对齐，不应反向修改公共 API 以放宽到任意日期范围。
- 冻结前可将该差异登记为明确的实现对齐/延期事项；若按 DEC-PLAT-002-007 的完整门禁要求 WebUI 消费者必须确认符合约束，则未有明确延期决议前不能把该消费者契约标为 frozen。
- 冻结后实现验收应包括 WebUI 366/367 日边界、Handler 同样边界，以及确认所有 InProcess/Application 调用路径不会绕过 366 日约束。
- 状态：**公共规则明确、WebUI 实现不一致**；契约定义不需要扩展，消费者符合性仍未完成。

### 23.4 本批总体结论与项目级映射

| 审计项 | 问题性质 | 冻结前要求 | 实现处理 |
|---|---|---|---|
| CTR-038 | 响应 total 语义缺失；120 条 UI 请求存在截断不可见风险 | 必须裁决 total 是范围总数还是返回数，并写入权威契约 | 不在本轮修改 |
| CTR-039 | 同日最新记录选择语义缺失，代码按插入 ID 选择但注释称最近采集 | 必须明确按 fetched_at 还是最后写入选择 | 不在本轮修改 |
| CTR-040 | 公共 366 日规则明确，WebUI 未传入同一上限；Service 内部校验另有一日边界风险 | 不需改变公共规则；冻结状态需如实记录消费者未对齐/延期 | 不在本轮修改 |

项目级映射只引用登记簿已有的 `DATA-CTR-001`、`GOV-CTR-001`、`GOV-CTR-002`，不新建正式契约编号。CTR-038/039 的语义裁决由项目负责人/相应契约 Owner 确认；CTR-040 是实现符合性问题，不应包装成新的契约设计。

本批没有修改项目级权威登记簿，因为 CTR-038/039 的候选语义尚未获得裁决；也没有修改模块代码、OpenAPI、测试或分支实现。未执行 CI。WebUI 维度 2 第一轮完整性 Review 继续进行。

## 24. Limit Pool：CTR-042 金额单位证据与冻结影响 Review

本批只复核既有 CTR-042，不重新扫描 LimitPool DTO、不扩展新的数据集、不新增正式契约编号。代码证据固定绑定 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。

### 24.1 固定基线事实

- Hithink `FetchLimitPool` 将 `seal_money`、`max_seal_money`（仅 up 池）和 `turnover`（仅 broken 池）解析为数值后直接写入 `LimitPoolItem`，未进行单位换算。
- 对每个 Hithink 条目，`ProviderAmountUnit` 固定写为 `provider_defined`，条目和快照均附加 `provider_amount_unit_not_explicitly_documented` / `provider_amount_units_not_explicitly_documented` 原因，并保持 degraded。
- Eastmoney 当前不填充上述金额扩展字段；默认配置中 Eastmoney enabled、Hithink disabled，因此默认业务路径不会依赖 Hithink 金额字段。
- `LimitPoolItem` 的 `seal_money`、`max_seal_money`、`turnover` 均为可选字段；`provider_amount_unit` 也是可选字符串。
- OpenAPI 已说明 `turnover` 应结合 `provider_amount_unit` 解读；`seal_money` / `max_seal_money` 只有当 `provider_amount_unit=CNY` 时才可解释为 CNY；并说明 `provider_defined` 不代表 CNY。
- 固定基线仓库内没有找到 Hithink 对上述字段单位的权威说明，也没有找到可支持换算比例的协议定义或可信样本证据。现有 `hithink_limit_pool_diagnostic_test.go` 只验证重复证券诊断，不验证金额字段的单位、保真或禁止换算行为。
- `turnover_ratio_percent` 是百分数值，不是金额字段，不属于 CTR-042 的金额单位范围；不得把它与 `turnover` 混为同一单位问题。

### 24.2 差异性质

CTR-042 不是已证实的数值换算错误：当前代码没有把 Hithink 原值乘除一个猜测系数，也没有将其标记为 CNY。

它也不构成当前默认路径的跨 Provider 金额混算，因为：
- Eastmoney 不提供这些扩展金额字段；
- Hithink 默认关闭；
- Hithink 提供的金额字段明确标记为 `provider_defined` 并降级；
- WebUI 当前不展示这些金额扩展字段。

准确分类为：**Provider 原始金额字段的单位证据缺失；当前实现采用不归一化、显式未知单位的保守策略；公共契约对未知单位值的消费者行为约束仍可进一步明确。**

因此，CTR-042 应映射到既有 `DATA-CTR-002`（单位口径）和 `DATA-CTR-003`（Provider 原始值标准化与精度保真），不新增 CTR 编号。

### 24.3 冻结前应明确的契约边界

即使 Hithink 单位无法证实，也可以明确冻结“未知单位值如何处理”，不必为了冻结而猜测单位或强行补齐换算。

建议将以下语义作为项目级契约裁决候选：
1. `provider_amount_unit=provider_defined` 表示值保留 Provider 原始数值，但其物理/货币单位尚未得到验证；不等于 CNY，也不等于任何默认单位。
2. 对 `provider_defined` 的金额值，下游不得换算、跨 Provider 比较、求和聚合、按金额排序或展示带有 CNY/元的标签；如需展示，必须明确标注单位未知且来源为 Provider 原始值。若产品不接受单位未知展示，则应隐藏这些字段。
3. 只有取得对应 Provider、对应接口字段的权威单位说明，并完成原始样本/换算测试后，才能将单位标记为明确单位（例如 CNY）并参与归一化比较。
4. 缺少金额字段时保持省略，不补零；单位未知与字段缺失是两个不同状态，不得互相替代。
5. 单位证明按字段分别进行：`seal_money`、`max_seal_money`、`turnover` 不因同属一个 Provider 或同一 DTO 而自动共享单位证据。

以上是审计建议，尚未获得项目负责人最终裁决，不写入权威登记簿作为已批准决议。

### 24.4 冻结影响与后续验收

- Limit Pool 公共 DTO 已有可选金额字段及 `provider_amount_unit`，并有 OpenAPI 的基础解释；可以在明确 `provider_defined` 的不可比较规则后冻结该公共边界。
- Hithink 三个金额字段的实际单位仍不可比较，相关 Provider 能力状态必须保持 degraded / not_comparable；这不阻止冻结“未知单位的安全处理规则”，但阻止将这些字段宣称为已归一化金额。
- `DATA-CTR-002` / `DATA-CTR-003` 的全局单位/标准化证据仍待其他 Provider 和字段范围核验，CTR-042 单独关闭不能代表两个项目级契约整体冻结。
- 若未来取得权威单位材料，后续实现验收需按字段覆盖原始响应 Fixture、解析保真、单位声明、换算（如确有必要）和跨 Provider not_comparable/matched 判定；在此之前不得写入猜测性转换。
- 本轮不要求 Data 分支修改代码、不改 OpenAPI、不增 DTO 字段、不执行 CI；仅把“已安全隔离”与“单位已证实”两个状态分开记录。

### 24.5 本批结论

- CTR-042 保留 P2，性质调整为**Provider 单位证据缺失 / 归一化能力不可用**，而非已证实实现换算错误。
- 当前 `provider_defined` + degraded 是保守实现，避免了把未知单位误当 CNY；但消费者禁止比较、换算和金额排序的语义应在项目级单位契约中明确。
- 当前 WebUI 不消费这些扩展字段，Hithink 默认 disabled；无证据表明当前默认页面发生错误金额展示。
- 需要裁决的是未知单位值的公共消费边界，而不是要求在缺少来源证据时补出单位。
- 本批未修改业务代码、OpenAPI、DTO 或测试，未执行 CI；WebUI 维度 2 第一轮完整性 Review 继续进行。

## 25. Stock Search：CTR-045 冻结影响 Review

本批只复核既有 CTR-045 的公共契约边界、实现责任和冻结影响，不重新扫描 DTO，也不新增差异编号。固定代码审计基线仍为 `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。

### 25.1 Provider 与 Service 的质量语义

- Tencent Smartbox 的搜索接口没有提供可核验的匹配总数。Provider 对非空候选返回 `quality_status=degraded`，原因 `provider_search_total_not_reported`；返回的 `total` 是本次 Provider 可报告/保留的匹配候选数，不代表全量目录命中总数。
- Tencent 无匹配 A 股候选时返回错误，Service 因此继续尝试后备 Provider。
- Service 对 `unknown` 空结果和 `unknown` 非空结果均继续尝试后备源；后备源耗尽后，分别以 `stock_search_fallback_exhausted_without_confirmed_match` 和 `stock_search_fallback_exhausted_with_unconfirmed_quality` 保留明确状态。
- Service 对 `stale` 非空候选继续尝试后备源；全部后备源耗尽后才返回 stale 候选，并附加 `stock_search_fallback_exhausted_using_stale_directory`。
- Service 对 `valid` 或 `degraded` 的非 stale 结果直接返回；因此 degraded 非空候选可以作为候选呈现，但不能被描述为完整目录覆盖。degraded 空结果也不能仅凭 `items=[]` 推导为确定不存在，必须结合状态和质量原因判断。
- Stock Search Service 校验接受 `valid`、`degraded`、`stale`、`unknown` 四种状态；`invalid` 不在该用例的 Provider 快照允许集合内。

### 25.2 HTTP/OpenAPI 契约边界

- HTTP Handler 对成功结果直接序列化完整 `StockSearchSnapshot`，不会把 unknown 空结果改成 404 或转换成确定无匹配；搜索失败与搜索结果是不同响应路径。
- OpenAPI 的 StockSearchSnapshot 将 `quality_status` 列为 required，`quality_reasons` 为可选数组；`total` 明确表示所选 Provider 报告/保留的匹配候选数，而不是全市场证券总数。
- `GET /api/v1/stocks/search` 的描述要求调用方按 `quality_status` 和 `quality_reasons` 处理未知或降级质量。这证明公共契约已要求消费者处理质量状态，但尚未定义空结果在不同质量状态下的具体语义，也没有给出各状态对“可否宣告无匹配”的明确约束。
- 因此，公共契约存在可进一步明确的消费语义，但目前没有证据表明 Provider/Service 将 unknown 空结果错误转换成确定无匹配；这个错误发生在 WebUI 展示层。

### 25.3 WebUI 实际消费行为与测试覆盖

- `web/src/app.js:loadStockSearch` 在读取响应后，先判断 `items` 是否为空；只要为空就显示“没有匹配的股票，请检查名称或代码”，没有检查 `quality_status` 或 `quality_reasons`。
- 对非空候选，当前只把 `quality_status === stale` 传给结果渲染；`degraded`、`unknown` 和质量原因均未进入状态提示。unknown 非空候选因此也会显示普通的“找到 N 条匹配结果”。
- `web/src/stock-search-view.js` 只负责候选安全渲染和 ARIA 选择状态，不负责质量判断；其单元测试覆盖规范化、文本安全渲染和键盘可访问性，不覆盖 `app.js` 的响应质量分支。
- `web/tests/api.test.mjs` 已覆盖搜索请求的中文编码、API Key、AbortSignal 和响应读取，但未验证不同质量状态/原因在调用层被完整保留并正确消费。
- Provider 测试验证 Tencent 非空结果为 degraded；Service 测试覆盖 unknown 空/非空结果的后备与耗尽语义；HTTP Handler 测试覆盖鉴权、成功响应和参数校验，但未专门验证 unknown/degraded 空结果的响应语义。当前没有覆盖搜索质量状态端到端展示的 DOM 回归测试。

### 25.4 冻结前应明确的消费者规则

建议将以下规则作为 CTR-045 的契约裁决候选；目前尚未批准，不视为已冻结规则：

1. 只有 `quality_status=valid` 且 `items=[]` 时，WebUI 才能显示确定的“没有匹配的股票”。
2. `unknown` 且 `items=[]` 必须显示“暂时无法确认是否存在匹配证券”，并展示或解释 `quality_reasons`；不得显示确定无匹配。
3. `degraded` 且 `items=[]` 不得默认解释为确定无匹配；应根据质量原因说明覆盖/证据不足，无法证明完整性时提示“结果不完整，暂不能确认无匹配”。
4. 非空候选仍可供用户选择，但必须区分 valid、degraded、stale、unknown：degraded 标示结果可能不完整；stale 标示缓存目录时效；unknown 标示目录质量未确认。unknown 非空候选不得使用“已确认匹配”一类措辞。
5. 对有结果和无结果的所有质量状态，均应消费 `quality_reasons`，以便提示可解释且可诊断；未知原因码应安全显示通用质量提示，不得导致页面渲染失败。
6. Provider 的 `total` 不得被 WebUI解释为全市场或完整匹配总量；当前 UI 仅展示候选条目数时，不应由条目数推导目录覆盖完整。

### 25.5 冻结影响与结论

- **CTR-045 保留 P2，证据成立。** 主要责任是 WebUI 对 Stock Search 质量状态消费不完整；公共契约的空结果/质量状态语义也需要进一步明确，但不需要新增独立 CTR。
- 固定基线中 Service 已保留 unknown 状态与耗尽原因，HTTP Handler 也直接返回质量元数据；没有证据支持通过改变 Service 将 unknown 空结果转换为错误响应来解决 UI 误导。
- 冻结前必须明确“确定无匹配”的判定条件，且 WebUI 必须遵守该条件。建议 valid + empty 才能作为确定无匹配；其他质量状态按质量原因呈现不确定性。
- 关闭 CTR-045 的实现证据至少包括：`loadStockSearch` 对 valid/degraded/stale/unknown × 空/非空结果的分支处理；质量原因展示/安全降级；浏览器 DOM 回归覆盖 unknown 空结果不会显示为确定无匹配、degraded/stale/unknown 非空候选有不同提示、valid 空结果仍显示无匹配。
- API Client 可增加响应元数据保留测试，但不能以 API Client 单测替代 app.js 集成 DOM 测试。
- 本批不修改业务代码、OpenAPI 或测试，不执行 CI；不改变固定审计基线。WebUI 维度 2 第一轮完整性 Review 仍未完成。

## 26. Analysis Run：CTR-046 / CTR-047 冻结影响 Review

本批只复核既有 CTR-046、CTR-047 的冻结影响、责任边界与验收证据，不重新执行 Analysis Run 全链路初审，不新增 CTR 编号。固定代码审计基线仍为 `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。

### 26.1 CTR-046：Run 读取与 Run Events 公共接口

**真实路由与消费者**
- `internal/platform/httpapi/server.go` 注册 `GET /api/v1/analysis/runs/`，同一 Handler 按路径段区分 `GET /api/v1/analysis/runs/{run_id}` 与 `GET /api/v1/analysis/runs/{run_id}/events`。
- WebUI `getAnalysisRun` 明确请求 Run GET；Run Events 路径也由 Server 对外提供，不能因当前 WebUI 主要通过 Task Events 展示阶段进度而从公共契约中省略。
- Run GET 成功时返回完整 `domain.RunResult`；未完成返回 409 `run_not_finished`，不存在返回 404 `run_not_found`，存储读取错误返回 500 `read_result_failed`。
- Run Events 成功返回对象 `{run_id, events}`；不存在返回 404 `run_not_found`，其他读取错误返回 500 `read_events_failed`。当前该端点没有分页参数或游标响应，不能将 Task Events 的 `after_sequence/limit/next_after_sequence` 语义套用到 Run Events。

**权威契约缺失**
- `doc/api/contracts/分析任务.openapi.yaml` 当前只定义异步 Task 创建、Task 状态、Task Events 三条路径，缺少 Run GET 和 Run Events 两条路径。
- Schema 缺少 Run GET 的 `RunResult` 及其嵌套 `RunRequest`、`Failure`、`StageEvent`、质量/市场/组合/信号/计划等响应结构的正式定义；Run Events 的响应包装也没有定义。
- Run GET 的状态响应语义至少应区分 200、404、409、500；Run Events 至少应定义 200、404、500。错误响应应沿用项目统一错误结构，而不是另造格式。
- Run 状态域与 Task 状态域不同，不能把 Task 的 queued/running/succeeded/partial/failed 枚举直接复用为 RunStatus。RunStatus 当前包含 running、completed、blocked_data_quality、rejected_request、partial、failed；例如 blocked_data_quality 是 Run 结果状态，不等于 Task 执行未完成。

**测试证据**
- 已有 `TestReadResultMapsNotFound` 覆盖 Run GET 404；`TestReadResultDoesNotExposeStorageError` 覆盖 Run GET 500 脱敏。
- 固定基线 `internal/platform/httpapi/server_test.go` 未发现 Run GET 成功响应、409 `run_not_finished` 的 Handler 测试，也未发现 Run Events 成功、404、500 的 Handler 专项测试。
- Task Events 的测试不能替代 Run Events 测试，因为两者资源标识、响应包装和事件读取 Port 不同。

**冻结判断**
- CTR-046 保留 P2，属于正式公共接口描述缺失，不是路由不存在或 WebUI 请求路径错误。
- 在 Run GET / Run Events 的路径、响应 Schema、状态码和错误语义进入权威 OpenAPI 前，消费者无法依赖正式契约生成客户端或验证响应；因此该项阻断 Analysis Run 对外契约冻结。
- 关闭至少需要补齐两条 OpenAPI 路径、RunResult 与 Run Events 响应 Schema，并补充 Run GET 成功/409、Run Events 成功/404/500 的 Handler 测试。嵌套 DTO 应有字段级 JSON Golden 或等价序列化验证；本轮未执行该项。

### 26.2 CTR-047：异步 CreateTask 请求与领域 RunRequest 的边界

**真实 HTTP 行为**
- `POST /api/v1/analysis/tasks` 的 Handler 直接将 JSON 解码到 `domain.RunRequest`，启用 `DisallowUnknownFields`，因此当前 HTTP 输入实际上受领域结构体字段集合影响；OpenAPI 却只列出部分字段，并错误描述“字段定义与 domain.RunRequest 一致”。
- 领域 `RunRequest` 包含 `run_id`、`task_type`、`as_of`、`as_of_mode`、`universe_id`、`symbols`、`portfolio_id`、`strategy_ids`、`market_id`、四个配置版本字段及 `parameters map[string]string`。现有 OpenAPI 漏掉 `as_of_mode`、`parameters`，同时没有定义 TaskType 四值枚举和 required/default 语义。
- Handler 默认值：空 `task_type` → `post_market_scan`；空 `as_of` → 当前上海时区时间；空 `universe_id` → `request-watchlist`；空 `portfolio_id` → `paper-unconfigured`；空 `market_id` → `CN-A`；空 `strategy_ids` → `trend-following`；四个配置版本分别补入固定默认版本。
- `historical_replay` 必须由调用方显式提供 `as_of`，否则 Handler 直接返回 400 `as_of_required`。对于其他任务，Handler 根据调用方是否显式提供 `as_of` 覆盖 `as_of_mode`：显式时为 `explicit`，未显式时为 `realtime_snapshot`。因此客户端传入的 `as_of_mode` 不具有控制权。
- Queue 在创建任务时生成新的 `run_id` 并覆盖请求中的 `req.RunID`，随后持久化；客户端传入的 `run_id` 不是创建任务时可控制的身份字段。
- `parameters` 是 `map[string]string`，Runner 在 `symbols` 为空时会尝试从 `parameters["symbols"]` 解析标的。因此不能简单把 `symbols` 声明为所有场景必填，也不能将 parameters 声明成任意 JSON 对象。
- Task Handler 的同步校验有限：它明确拒绝历史任务缺少 as_of；未知 TaskType、缺失 symbols 等领域执行校验可能在任务入队后由 Runner 执行并导致异步任务失败。契约不得误写成这些情况都在 POST 阶段同步返回 400。

**契约裁决候选**
1. 将 HTTP 输入独立定义为 `CreateTaskRequest`，不要继续声称它与领域 `RunRequest` 完全一致；领域对象是内部执行模型，不是 HTTP DTO 契约。
2. TaskType 枚举列出四个正式值，并明确省略时的默认值。需由项目负责人裁决非法值应在创建时同步拒绝，还是允许入队后由 Runner 以失败 Task 结束；OpenAPI 与 Handler 必须采用同一行为。
3. 明确 `as_of` 的默认行为和 `historical_replay` 必须显式提供 `as_of` 的条件约束；`as_of_mode` 是服务端推导值，不应被描述为客户端可控制字段。
4. `run_id` 由服务端/Queue 生成并覆盖，不属于客户端可指定字段；建议从 CreateTaskRequest 中排除。若暂时保留兼容输入，也必须明确其被忽略并由服务端覆盖。
5. 明确各服务端默认字段及其版本值；`symbols` 与 `parameters.symbols` 的互补关系、参数类型和执行期校验时机必须明确，不能机械复制领域结构体的非 omitempty 字段作为 required。
6. 对未知字段、Content-Type、1 MiB 请求体上限、Idempotency-Key 长度与冲突、202 新建/200 幂等重放、429 队列满、503 Worker 未配置等现有行为，保留在 HTTP 请求/响应契约中。

以上为待裁决候选，不代表项目已经批准新的请求 DTO 或改变当前行为。

**测试证据**
- 已有 `TestCreateTaskReturnsAcceptedAndIdempotencyKey` 覆盖省略 task_type 时的默认值与 202；`TestCreateTaskRejectsHistoricalRequestWithoutAsOf` 覆盖 historical_replay 条件约束；`TestIdempotencyConflictReturns409`、队列满 429、超大尾随请求体及错误 Content-Type 测试也已存在。
- 尚未发现对 `as_of_mode` 被覆盖、客户端 `run_id` 被 Queue 覆盖、`parameters.symbols` 回退、TaskType 非法值处理时机，以及所有默认字段的完整 Handler 契约测试。
- 现有测试不足以证明 HTTP CreateTaskRequest 与领域 RunRequest 的字段语义相同；它们反而证明存在 Handler 默认与覆盖规则。

**冻结判断**
- CTR-047 保留 P2，属于 HTTP 请求 DTO 边界及字段语义描述不准确。
- 在 CreateTaskRequest 的字段集合、默认值、条件约束、服务端生成身份、parameters 语义和非法 TaskType 处理时机未裁决并同步到 OpenAPI/Handler 前，异步创建请求契约不能冻结。
- CTR-005 的 TaskType enum/required 相关遗留要求继续由 CTR-047 承接；CTR-003 的 Run 路径缺失由 CTR-046 承接。历史编号保留可追溯，不重复计数。

### 26.3 本批结论与范围控制

- CTR-046、CTR-047 均保留 P2；两者都影响 Analysis Run/Task API 的权威契约冻结，但性质不同：CTR-046 是响应端点/Schema 缺失，CTR-047 是请求 DTO/字段语义边界不清。
- CTR-046 的 404/500 既有测试描述已保持准确；缺口仅列出尚未覆盖的成功、409 和 Run Events 路径状态。
- CTR-047 不把所有领域字段机械标为 required；先裁决 HTTP 输入模型和 Handler 实际校验时机。
- 本批未修改业务代码、OpenAPI、DTO 或测试，未执行 CI；没有新增 CTR 编号，也没有把嵌套 RunResult Golden、DB migration 或 Worker 并发审计混入本轮。后几项仍按既定其他维度/专项任务处理。

## 27. 全市场分页 API Client：CTR-004 证据链复核

本批只复核既有 AUD-WEBUI-CTR-004 的测试缺口是否仍成立、原始描述是否过宽，以及它对 WebUI 维度 2 完整性/冻结的影响。不新增差异编号。固定代码审计基线仍为 `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。

### 27.1 API Client 实际归一化行为

固定基线 `web/src/api.js:getMarketStocks`：
- `page` 默认 1，先转 Number、截断，再限制在 1..10000；无效数值/0 回退到 1，负数钳制到 1，超过上限钳制到 10000。
- `pageSize` 默认 50，截断后限制在 1..100；无效数值/0 回退到 50，负数钳制到 1，超过上限钳制到 100。
- `includeMA` 默认 true，并通过 `Boolean(includeMA)` 序列化为字符串 true/false。该参数的正常调用约定应是布尔值；传入字符串 `"false"` 会因 JS truthiness 被序列化为 true，当前没有显式类型校验。
- `adjustment` 只接受 `none`、`forward_adjusted`、`backward_adjusted`；其他值回退为 `forward_adjusted`。
- 最终请求固定为 `GET /api/v1/market/stocks?page=...&page_size=...&include_ma=...&adjustment=...`，并传递 API Key、AbortSignal 和 timeout。

### 27.2 与页面、Server 和 OpenAPI 对照

- `web/src/market-stocks.js` 页面状态默认 page=1、pageSize=50、adjustment=forward_adjusted；页面每页数量只允许既定选项，异常选项回退到 50；页面固定传 `includeMA: true`。
- `web/tests/market-stocks-lifecycle-browser.mjs` 已覆盖真实页面交互下的首屏 page=1/page_size=50、翻页 page=2、切换 page_size=20、切换 backward_adjusted，以及离开/返回和连接失效行为。该测试使用拦截路由读取请求参数，属于页面到 API 的集成级证据。
- 该浏览器测试只记录 page、page_size、adjustment，没有断言 include_ma；也没有直接验证 API Client 对缺省参数、非法/越界 page、pageSize、未知 adjustment 的归一化结果。
- `web/tests/api.test.mjs` 导入 `getMarketStocks`，目前相关直接测试覆盖请求超时与底层 AbortSignal 取消；未发现独立的 URL 查询参数序列化、默认值和边界归一化测试。
- `web/tests/market-stocks-navigation.test.mjs` 是导航/源码契约类测试，不替代 API Client 请求参数行为断言。
- Server `internal/marketdata/httpapi/market_stocks.go` 对 page 允许 1..10000、page_size 允许 1..100，缺省分别为 1/50；include_ma 缺省 true，非法布尔值返回 400；adjustment 缺省使用 Server 配置，非法口径返回 400。
- OpenAPI `/api/v1/market/stocks` 已声明 page 1..10000/default 1、page_size 1..100/default 50、include_ma/default true 和 adjustment 的 RequestAdjustmentBasis 枚举；服务端 Schema/参数约束本身有表达。
- Server 测试已覆盖 page 下界/上界、page_size 上界、非法 include_ma 和非法 adjustment 的 400 行为。当前没有发现对应的成功响应请求参数组合 Handler 专项测试；这不改变本 CTR 原始指向是 WebUI API Client 测试补证。

### 27.3 CTR-004 复核结论

- 原始问题“前端缺少对全市场分页 API 参数序列化/默认值/边界归一化的直接断言”仍成立。
- 但不能再表述为“全市场分页完全没有请求参数测试”：页面浏览器生命周期测试已证明主要交互路径的 page、page_size、adjustment 会正确传递；缺口集中在 API Client 直接行为、默认值/边界值以及 include_ma 序列化。
- 没有证据证明当前页面正常交互产生错误请求；CTR-004 仍是测试补证项，而不是已证实运行时错误。
- **AUD-WEBUI-CTR-004 保留 P2**，责任在 WebUI API Client 测试层。建议补充 `getMarketStocks` 直接请求测试，至少覆盖全默认值、正常参数序列化、page/pageSize 越界归一化、未知 adjustment 回退、includeMA true/false，并断言 URL、API Key、signal/timeout 配置。
- 需明确 `includeMA` 的输入类型约定。若 API Client 只接受布尔值，应在测试/注释中固定该约定；若需容忍字符串，应先决定转换规则，避免把 `"false"` 的 truthy 行为误当成 false。
- **冻结影响：** 该项不单独阻断 Market Stocks 服务端 OpenAPI/DTO 语义冻结，因为服务端范围、默认值和参数 Schema 已有明确表达；但它阻止将 WebUI 对该 API 的消费者参数适配标记为“测试证据完整”。维度 2 整体冻结仍需按项目门禁补齐适用的消费者 Contract Test，不得把页面浏览器测试误记为 API Client 边界测试的完全替代。

### 27.4 跨维度编号范围校正

- `AUD-WEBUI-CTR-001`（浏览器持有行情 API Key 的迁移安全门禁）和 `AUD-WEBUI-CTR-002`（Analysis Owner-only 鉴权联调阻断）均在既有进度记录中明确登记于 **WebUI 维度 1：对外契约消费/认证安全核验**，不是本轮维度 2 的 DTO/API 参数审计范围。
- 本轮不复核、不改变这两项的状态，也不把它们并入维度 2 CTR-001/002。若后续进行维度 1 交叉检查，应回到其原始安全/认证证据链单独处理。
- 先前下一步计划把 CTR-001/002 与 CTR-004 并列为本维度审查项，属于范围规划错误；本节已纠正。CTR-004 是本批唯一执行对象。

本批未修改业务代码、OpenAPI、DTO 或测试，未执行 CI；没有新增 CTR 编号。WebUI 维度 2 第一轮完整性 Review 继续保持 partial。


## 28. 任务收敛 Review：停止重复审查并固定剩余批次

本节只做任务状态与重复项收敛，不重新读取已审数据集的生产代码，不新增 CTR 编号。

### 28.1 已识别的流程问题

- 总进度原阶段表将阶段 F 标为 not_started、阶段 D 标为 not_started，与本文件已存在的第 7～27 节及总进度中的多个 Review 记录不一致。现已修正为：阶段 D in_progress；阶段 F 仅 WebUI 维度 2 局部 in_progress。
- 将 CTR-001/002 放入维度 2 下一轮属于范围错误；二者归属维度 1，不再进入维度 2 队列。
- Analysis Run 初审后又做 CTR-046/047 冻结影响 Review、分页初审后又做 CTR-004 证据链复核，属于同一数据集分阶段拆得过细。既有证据已足够的范围不再重复全链路读取。
- CTR-003/046、CTR-005/047 已建立承接关系；CTR-041 已撤销；CTR-010 是跨 DTO 聚合项。上述编号不再按独立活动问题重复计数。

### 28.2 唯一执行矩阵

新增唯一索引：[`PLAT-002-差异唯一登记与剩余任务矩阵.md`](./PLAT-002-差异唯一登记与剩余任务矩阵.md)。

该矩阵明确：
- 已完成的 WebUI 维度 2 数据集初审清单；
- 已完成冻结影响/证据链 Review 的批次；
- CTR 别名、撤销项及聚合项的计数规则；
- WebUI 维度 2 剩余工作批次；
- 十模块项目阶段真实状态与冻结门槛。

### 28.3 本轮结论

- WebUI 维度 2 不再增加数据集，不重做已有初审。
- 既有 CTR-004、CTR-038～040、CTR-042～049 的冻结影响/证据链 Review 作为已完成批次，不重复执行。
- 下一批将 CTR-006/007/008 合并为一个 Task/TaskEvent 工作批次；一次核对 StartedAt/FinishedAt 空值语义、StageEvent Schema、事件分页和 ID 持久化语义，输出唯一关闭条件矩阵。
- WebUI 维度 2 第一轮完整性 Review 仍为 partial；项目级阶段 F 也仍为 in_progress。没有宣称十模块核验或契约冻结完成。
- 本轮仅更新审计文档，没有修改业务代码、OpenAPI、DTO、测试或执行 CI。


## 29. Task / TaskEvent：CTR-006 / CTR-007 / CTR-008 合并冻结影响 Review

本批一次性核对 Task DTO、StageEvent、HTTP Events Handler、Queue 查询、OpenAPI 与现有测试；不重复 Analysis Run/Task 初审，不扩展到其他维度。

### 29.1 CTR-006：StartedAt / FinishedAt 空值语义

- 固定基线 `internal/platform/taskqueue/queue.go` 的 `Task.StartedAt`、`FinishedAt` 均为 `*time.Time`，JSON tag 均带 `omitempty`。
- Queue.Get 仅在数据库时间有效时设置指针；queued 任务的 StartedAt/FinishedAt 为 nil，running 任务通常只有 StartedAt，终态任务有 FinishedAt。
- Go JSON 对 nil + omitempty 的实际行为是省略字段；OpenAPI Task Schema 将两字段声明为非 required 且 `nullable: true`，允许显式 null。两者表达集合不一致。
- **CTR-006 保留 P2。** 需要由权威契约裁决缺失与 null 的公共语义，再统一 DTO、OpenAPI 和 Golden；不能直接假定 null 是目标语义。

### 29.2 CTR-007：StageEvent 事件载荷 Schema

- 固定基线 `internal/analysis/domain/types.go` 的 StageEvent 包含：`run_id`、`stage`、`status`、`at`、`duration_ms`、`message`、`attributes`。
- 必需字段为 run_id、stage、status、at；status 值为 started、succeeded、failed、skipped；at 是 time.Time；duration_ms 为可选整数；message 为可选字符串；attributes 为可选 string-to-string map。
- OpenAPI 的 TaskEvent.event 仅为 `type: object`，描述写“原始 domain.StageEvent 结构”，没有字段级 Schema、required、枚举或 date-time 约束。
- **CTR-007 保留 P2。** 应在权威 OpenAPI 中显式定义 StageEvent Schema，并通过 JSON Golden 或等价契约测试验证 Go 序列化与 Schema 一致。

### 29.3 CTR-008：事件分页与 cursor 持久化语义

- Handler 默认 `after_sequence=0)、`limit=100`；after_sequence 必须是非负 int64，limit 必须是 1..500，否则返回 400。
- Queue.Events 使用 `analysis_stage_events` 的 `id` 作为 sequence，按 `id > after` 过滤、`id ASC` 排序并 LIMIT；Handler 将最后一条返回事件的 sequence 作为 next_after_sequence，无事件时保留输入 cursor。
- OpenAPI 已声明 after_sequence minimum=0/default=0、limit 1..500/default=100；因此不能再把问题描述为“分页参数完全没有约束”。
- 本轮仓库树中未找到 `analysis_stage_events` 的建表 DDL，无法证明 id 的唯一性、自增/单调分配、索引和重启后不重用。当前 Handler 测试用 fake service 验证增量游标，不覆盖真实数据库分页。
- **CTR-008 保留 P2。** 关闭需补充权威 migration/DDL 证据，明确 cursor 是 exclusive after 的事件序号，并补数据库集成测试验证多页、空页、边界 limit、重启后续读及并发追加。若 DDL 位于仓库外部迁移系统，责任方需提供可审计版本与部署约束证据。

### 29.4 去重与冻结结论

- CTR-006、007、008 是同一 Task/TaskEvent 数据集的三个不同差异：字段空值语义、事件载荷 Schema、分页游标持久化；不合并为一个 CTR，也不再拆成三个独立审查轮次。
- 当前批次的冻结影响 Review 已完成，但三个差异均未关闭。Task/TaskEvent 公共契约不可标记为完全冻结；是否允许带延期决议冻结，由正式契约治理决策确定。
- 关闭前不重复读取本批相同代码。后续仅在权威决议、实现提交、DDL/迁移证据或测试结果变化时做增量验收。
- 本批未修改业务代码、OpenAPI、DTO 或测试，未执行 CI；固定代码基线不变。

### 29.5 下一批

下一批处理 WebUI 维度 2 的 DailyBar 查询窗口 CTR-009：只核对 lookback_days 缺省值的所有权、WebUI 显式传参、Server/OpenAPI 默认语义及现有测试；完成后把结论写入唯一矩阵。不得重新初审已完成的数据集。


## 30. DailyBar Bundle lookback_days：CTR-009 合并冻结影响 Review

本节只核对 `GET /api/v1/bundles/{symbol}` 的 lookback_days 默认值所有权、WebUI实际传参、服务端配置默认和现有测试，不重复日 K Provider/数据质量初审。

### 30.1 默认值所有权与调用路径

- WebUI 个股行情页的日 K 条数选择默认120；设置页允许60/120/250，并明确该设置影响下一次个股新查询的默认条数。
- `app.js` 无自定义日期范围时调用 `getMarketBundle`，将当前limit夹在20..2000后作为 `lookbackDays` 显式传入。因此WebUI常规路径使用用户偏好，不依赖服务端缺省。
- `api.js` 的 `getMarketBundle` 默认 `lookbackDays=120`，且始终把 lookback_days 序列化到URL。
- HTTP Handler 在参数省略时将0传给Application；DataService.Bundle 对非正lookback使用 `s.defaultBars`。
- 配置默认 `MARKET_DATA_DEFAULT_DAILY_BARS=120`，最大值配置 `MARKET_DATA_MAX_DAILY_BARS=1000`；二者均可部署配置。Handler校验显式值至少20且不超过Server maxBars。
- OpenAPI只声明 minimum=20、maximum=2000，未描述省略参数时使用服务端配置。不可将固定120直接写成公共 default，因为实际默认值可配置，且部署默认max与Schema上限不同。

### 30.2 测试证据

- OpenAPI contract test断言lookback_days的type/minimum/maximum，但不校验省略行为说明或default。
- Handler边界测试覆盖lookback_days=19返回400；本轮未发现省略参数使用配置默认值的专项HTTP测试。
- API Client测试只通过超时用例调用getMarketBundle，没有断言默认120和显式lookback_days的URL参数。
- 配置默认值由配置代码设置；本轮未发现针对Bundle“省略参数→配置默认值→实际读取窗口”的端到端测试证据。

### 30.3 CTR-009 结论与关闭条件

- **CTR-009 保留 P2。** 目前没有证据证明WebUI常规路径窗口错误；问题是公共契约未说明配置化默认行为，且Client/Handler缺少相应直接回归证据。
- OpenAPI应描述：省略lookback_days时使用服务端配置 `MARKET_DATA_DEFAULT_DAILY_BARS`，默认部署值120；最大值由 `MARKET_DATA_MAX_DAILY_BARS` 控制。不要写死固定default=120。
- 关闭条件：OpenAPI补充配置化默认与上下界语义；API Client测试覆盖默认120及显式值序列化；Handler/Service测试覆盖省略参数采用配置默认值、显式参数优先；配置范围测试确认默认值不超过最大值。
- CTR-009不单独阻断整个WebUI维度2公共Schema冻结，但在上述契约说明和测试证据补齐前，不将Bundle查询窗口标记为完整。
- 本轮仅审计文档更新；未修改业务代码、OpenAPI、DTO、测试或执行CI。固定代码基线未改变。

### 30.4 下一批

W2-C：对CTR-011～037已有审计证据进行一次集中归类和冻结影响判断。排除CTR-010聚合项以及已由其他批次承接的编号；不重新初审代码、不新增CTR。


## 31. CTR-011～037 集中归类完成记录

W2-C已完成，详细唯一归类表与关闭条件以 `PLAT-002-差异唯一登记与剩余任务矩阵.md` 第11节为准。本轮只汇总既有切片，不重新初审代码、不新增CTR。全部保持P2；Schema准确性、数据语义证据和产品范围决议分别判断冻结影响，不将所有P2一概视为整体冻结阻断项。CTR-010相关nullable字段不在本批重复统计。下一批为W2-D：准备CTR-010缺失/null/零值语义裁决问题；之后W2-E收尾第一轮完整性Review。

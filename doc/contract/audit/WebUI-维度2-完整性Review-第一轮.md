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
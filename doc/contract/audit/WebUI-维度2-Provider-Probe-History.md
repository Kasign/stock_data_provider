# WebUI 维度 2：Provider Probe History 主动探测历史

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**写入接口：** `POST /api/v1/data-sources/probe`  
**历史接口：** `GET /api/v1/data-sources/probe/history`  
**DTO：** `ProviderProbeRun`、`ProviderProbeHistory`

## 1. 职责边界与数据流

- 主动探测由 `DataService.ProbeProviderCapabilitiesWithOptions` 执行。探测结果按 Provider/capability/复权口径汇总为一个 ProviderProbeRun。
- ProviderProbeRun 是一次完整探测批次；items 是各 Provider 能力探测结果；check_results 是 connectivity/contract/quality/freshness/cross_provider/storage 等独立诊断维度。
- 成功探测后，如 MariaDB Store 已配置，服务端将完整 ProviderProbeRun JSON 写入 `market_data_provider_probe_runs.metadata`；不依赖 Redis。
- 历史查询只读取 MariaDB 中已持久化批次，不触发任何 Provider 请求，也不读取进程内 circuit-breaker 计数。
- WebUI 数据中心主动探测页面展示本次结果；另一个折叠列表展示服务端最近持久化批次。历史读取失败时页面明确提示历史暂不可读，不推断 Provider 健康状态。
- 当前 API 不提供按时间/游标翻页；客户端可请求 limit 1..100，默认 20，UI 固定请求 20。

## 2. DTO、字段语义与持久化

### ProviderProbeRun

- `run_id`：一次探测批次唯一 ID。
- `scope`、`requested_capabilities`、`requested_checks`、`requested_markets`、`requested_symbols`、日线复权和日期窗口、`timeout_seconds`：本次探测请求范围回显。
- `status`：批次汇总状态；与各 item.status 不是同一状态维度。
- `persisted`：本次结果是否已成功写入 MariaDB。
- `started_at` / `finished_at`：批次执行起止时间。
- `total`、`returned`、`degraded`、`unknown`、`failed`、`skipped`、`not_configured`、`not_supported`：按 item.status 汇总的计数。
- `items`：每个 Provider/capability 的探测结果，包括质量、字段校验、错误分类、覆盖率、源端时间和本地获取时间证据。
- `check_results`：检查维度结果；交叉 Provider 检查保留对比字段、状态和不可比较原因。
- 错误文本经 sanitizeProbeError 脱敏 URL query 和 token/api_key/cookie/authorization 类值，并限制 400 字符；结构化 ErrorCode/ErrorStage/HTTPStatus 独立保留。

### ProviderProbeHistory

- `items`：按 started_at DESC、数据库 id DESC 排序的最近 N 个批次。
- `total`：当前实现为 `len(items)`，即本次响应实际返回数量，不是数据库总记录数。
- Go JSON required 与 OpenAPI ProviderProbeHistory 的 required [items,total] 一致；ProviderProbeRun 的 required 字段也已登记在 OpenAPI 反射契约测试中。
- OpenAPI 未说明 total 是返回数还是全量记录数；UI 当前文案“服务端记录 X 批”会让人将其理解为全量记录数。

### MariaDB 表

`market_data_provider_probe_runs`：
- `id` 自增主键；`run_id` 唯一键；`started_at`、`finished_at`、`status` 为查询列；完整批次放在 JSON `metadata`。
- 索引：唯一 `run_id`，以及 `started_at`；历史查询按 `started_at DESC, id DESC` 排序。
- Save 使用 `INSERT ... ON DUPLICATE KEY UPDATE`，同 run_id 重写 finished_at/status/metadata；List 使用 limit 参数化查询。
- List 读取后 JSON 反序列化；损坏 JSON 或 SQL 错误整体返回错误，不静默丢弃单条历史。
- 表定义没有 TTL/保留期或清理任务；当前列表读取最多 100 条，但底层记录会持续累积。由于个人研究工具的探测由用户主动触发，本轮先记录运维边界，不单独升级为契约缺陷；若未来转为周期性自动探测，需要定义保留策略和清理指标。

## 3. HTTP、认证与错误处理

- Server 注册 POST 主动探测和 GET 历史两个路由；路由使用 Market Data HTTP Adapter 的共享 API Key 认证。OpenAPI 根级 security 同样覆盖两个接口。
- GET history 默认 limit=20；显式 limit 必须为整数 1..100；无 Store 时返回 503 STORAGE_UNAVAILABLE。
- POST body 限制 8 KiB，必须是单个 JSON object，拒绝未知字段；scope、checks、capabilities、timeout、日线范围等由 Handler 和 Service 校验。
- POST Service 成功后写入 MariaDB；写入失败时 Service 返回 `run, fmt.Errorf("persist provider-probe run: %w", err)`，并将 run.Persisted 设为 false；若选择 storage check，也会把对应 CheckResult 改为 failed 并写入脱敏错误。
- 但 Handler 对 Service 返回的任何 error 都统一写成 HTTP 400 `VALIDATION_ERROR`，没有区分请求校验错误与 MariaDB 持久化失败。
- OpenAPI 对 POST 明确声明 500 “诊断结果持久化失败”，与 Handler 实际 400 不一致。由于 Handler 在 error 分支不返回已生成的 run，客户端也拿不到带 `persisted=false` 的本次结果对象。

## 4. WebUI 消费与测试

- API Client `getDataSourceProbeHistory` 校验 limit 1..100，使用 URLSearchParams，携带 Market API Key 和 AbortSignal。
- 数据中心页面读取最近 20 批，使用 generation + AbortController 避免旧响应覆盖新状态；切换设置时取消请求并清空过期历史。
- 页面用 DOM textContent 安全渲染 run_id、Provider 名称、错误文本、时间、样本证券和市场；批次详情折叠显示 check_results 和 items。
- 无 API Key 时不发起历史请求；历史接口 503 时显示“历史暂不可读”，不把它解释成 Provider 故障。
- POST 返回 persisted=true 后刷新历史；persisted=false 时页面明确显示“本次未持久化”。
- API Client 测试覆盖默认 limit、URL、API Key、signal 和非法 limit。
- 浏览器专项测试覆盖首次历史读取、最新/旧批次共同展示、刷新失败 503、无 API Key 不发起请求、主动探测后刷新历史、持久化失败提示。
- MariaDB HTTP 集成测试覆盖真实 POST 写入、关闭并重开 MariaDB 后 GET 历史读取，以及 persisted/storage/cross-provider 证据保持；测试依赖 `MARKET_DATA_TEST_MYSQL_DSN`，未配置时会 Skip。
- 当前没有独立的 GET history Handler 单元测试覆盖 limit 非法、无存储 503、SQL 读取失败和认证失败；API Client/浏览器测试主要使用 mock HTTP。MariaDB 集成测试能证明基本持久化往返，但不覆盖历史排序、limit 截断和重复 run_id 更新行为。

## 5. 已登记差异

### AUD-WEBUI-CTR-048（P2）：ProviderProbeHistory.total 被实现为返回条数，无法表达历史总量

- Repository 只执行 `ORDER BY started_at DESC, id DESC LIMIT ?`，不查询数据库全量 COUNT。
- Service 直接将 `Total` 设置为 `len(items)`；当数据库记录数超过 limit 时，total 仍等于 limit。
- OpenAPI 对 total 没有定义其是返回条数还是全量记录数；WebUI 文案“服务端记录 X 批；当前展示最近 N 批”容易把 total 误解为服务端总记录数。
- 影响：调用方无法判断历史是否被 limit 截断，也无法得知历史记录总量；若 UI 将 total 作为全量批次数展示，会产生不准确陈述。
- 关闭条件二选一：
  1. 若 total 语义为全量记录数：Repository 在同一一致性策略下查询 COUNT，并补充 total >= len(items) 的测试；
  2. 若 total 仅表示本次返回条数：重命名为 returned 或在 OpenAPI 明确 returned_count，移除 total 的全量总数暗示；如需判断是否还有更多，增加 has_more 或稳定游标。
- 不要求现在增加复杂分页；至少先让字段语义不误导。

### AUD-WEBUI-CTR-049（P2）：主动探测历史写入失败被错误映射为请求参数错误

- DataService 在 MariaDB SaveProviderProbeRun 失败时返回持久化错误，并将 run.Persisted=false。
- `dataSourceProbe` Handler 将所有 Service error 统一映射为 HTTP 400 `VALIDATION_ERROR`。
- OpenAPI 已将持久化失败定义为 HTTP 500；因此实际 HTTP 状态/错误码与正式契约冲突。
- 影响：数据库故障被客户端误认为请求体或参数错误；服务端已经执行完探测但历史未保存，Handler 又丢弃 run 结果，调用方无法从响应获知本次探测结果及 persisted=false。
- 关闭条件：区分输入校验错误与存储故障；存储失败映射为 OpenAPI 定义的 500（或经契约决议改为 503），并决定是否返回包含 persisted=false 的安全结果摘要。增加 Handler 测试，模拟 Store 写入失败，断言 HTTP 状态、稳定错误 code、request_id 和结果语义。

## 6. 本切片结论

Provider Probe History 已形成完整的主动探测—MariaDB 持久化—历史读取—WebUI 展示闭环。认证、请求限制、错误脱敏、持久化往返和浏览器历史状态均有实现；OpenAPI/DTO 基本结构也有契约检查。当前确认的缺口是历史 total 的计数语义不清（CTR-048），以及写入故障 HTTP 映射与 OpenAPI 冲突（CTR-049）。MariaDB 集成测试本轮未执行；其余结论为固定 SHA 源码与测试资产审计。WebUI 维度 2 三个剩余数据集已审完，下一步按主线进入维度 2 完整性 Review，不继续扩展数据集。

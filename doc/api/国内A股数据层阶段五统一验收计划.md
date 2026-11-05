# 国内 A 股数据层阶段五统一验收计划

> 本文为阶段五的专项实施与验收资料，不再作为整体任务入口。整体开发顺序、优先级和当前执行队列统一以 [数据侧整体规划](数据侧整体规划.md) 为准。本文保留阶段五诊断细节、专项验收工作包和历史证据；与整体排期冲突时以整体规划为准。

- 目标分支：`feat/full-go-data-providers-api-switch`
- 阶段定位：阶段五统一验收，不以“代码已实现”替代真实数据验收。
- 当前状态（2026-11-03，代码基线 `b423c1ef6585924163d3d30a775e7b7713df3c59`）：统一诊断框架、Quote 字段诊断、DailyBar 字段 Reporter、Probe 历史存储、DailyBar 跨 Provider 比较及多复权口径 Probe 请求参数/结果字段已有代码实现。较早章节的待办表属于历史快照，不得作为当前任务重复执行。
- 原则：每项结论必须绑定代码 SHA、样本范围、原始响应证据和质量判定；凭据不得进入文档或日志。


## 2026-11-04 当前 HEAD 回归修复记录

- 当前分支 HEAD：`c4438a9d1822c292eae4d4313011718ce2fd0c0a`。
- 父提交 `e1e61f68` 的 Go test suite 与 Go-Python parity 失败；WebUI checks、Live provider probe 通过。失败日志确认 Reporter 未使用 `FieldIssue.Count` 聚合，以及 Pytdx Probe 测试调用了不存在的 `SetDailyBarProviders`。
- 修复提交 `c4438a9d`：`Report.Add` 和严重程度兼容回退按 Count 加权；`Report.Merge` 不再把聚合行数误判为明细 Truncated；Probe 测试改用现有构造函数注入 DailyBarProvider。
- 新增 Merge 聚合计数与 severityCounts 兼容回退测试。
- 修复后 HEAD 尚无 GitHub Actions 运行记录，所有 CI 项标记为待验证；真实 Provider × 复权口径请求未执行。
- 本记录只修复统一诊断统计和测试构造方式，不改变 Provider 复权算法、业务 API 或数据库结构。


## 2026-11-05 P0 批次 15：DailyBar 复权元数据来源链路审计

- 审计基线：`b4cf1a5dbd898f000cf679fa0eb73be6f1039a2e`。核对 Eastmoney/Tencent/BaoStock/Pytdx Provider 实现、复权契约测试及本地因子重建逻辑。
- Eastmoney 当前请求 `fqt=0/1/2`，K 线解析字段为 `f51..f61`；没有解析独立参考日回显。Tencent 当前解析 `day/qfqday/hfqday` K 线行，也没有独立参考日字段。两者保持 unknown / not_comparable，不从请求 endDate、返回末行日期或采样当前日期推断。
- BaoStock `adjustflag=3/2/1` 只证明请求的 none/qfq/hfq 口径；底层 TCP Client 已有 QueryAdjustmentFactors，但尚未适配到 Baostock Provider/service；历史复权因子与 K 线内部复算 mismatch=0 是既有专项样本证据，不证明跨源绝对基准。
- Pytdx 使用 GBBQ 事件执行仿射 QFQ/HFQ；当前 AdjustmentRefDate 由完整扫描序列的日期边界确定（QFQ 最新日、HFQ 最早日），属于算法定义的基准，不是上游字段回显。必须保留完整扫描边界约束。
- `rebuildAdjustedDailyBars` 已要求同一 source/version 因子覆盖全部原始 Bar 和参考日期，并对前复权记录实际参考因子日期；不需要另建因子重建逻辑。
- 未发现可证明的生产代码缺陷。本轮不改代码、不新增字段/API、不调整 Provider 算法；现有“参考日未知则不可比较”门禁正确。
- 交易上下文证据不变：Pytdx 对 4 条官方盘中临停日期均返回唯一 raw 日 K；Tushare `suspend_d` 仍因 40203 权限不足无法核验时段字段。二者不是同一验收项。
- 本批次代码审计结论通过；Eastmoney/Tencent 参考日仍不可比较；Tushare 时段映射仍阻塞；阶段 B 继续 in_progress。
- 后续优先评估 BaoStock 现有同源因子与 raw K 线能否提供带明确日期语义的可复验锚点；只有取得数据证据后才考虑增加元数据。不得重复请求已完成的样本。
- HEAD `b4cf1a5dbd898f000cf679fa0eb73be6f1039a2e` 的四个关联工作流均 success，但 Live Provider Probe 的 `MARKET_DATA_LIVE_PROVIDER_TESTS=0`，本 SHA 没有重新执行真实 Provider 请求。历史样本不升级为本 SHA 新验收。


## 2026-11-05 P0 批次 16：BaoStock 因子接入前置审计

- 基线：`d9a2872f8259b44f2e59615e2f966bfec2c0ad3a`。TCP Client 已有 QueryAdjustmentFactors，但 Provider 未实现 AdjustmentFactorProvider；DataService.RefreshAdjustmentFactors 因此不会调用 BaoStock。
- BaoStock 单行同时含 foreAdjustFactor、backAdjustFactor、adjustFactor；统一 AdjustmentFactor 只有单一 Factor 字段，不能无损表达三种来源值。
- MariaDB adjustment-factor revision 的查重与 as_of 选择均按 symbol/trade_date/source，不包含口径维度；同日同源的 forward/backward 两行不能安全共存。仅把额外字段放入 JSON metadata 不会修复版本选择键。
- dividiOperateDate（协议字段 dividiOperateDate）只作为操作日期；没有证据证明它等于首次公开时间。适配时不得赋给 AvailableAt；FetchedAt/SystemObservedAt 也不能替代供应商 AvailableAt。
- 现有 BaoStock live probe 的历史样本对账 mismatch=0 仍是有效的窗口内证据，但不能据此推导统一重建器公式。当前重建器对 forward 因子做参考日归一化，必须用真实数据分别确认 BaoStock forward/backward factor 的基准定义。
- 本轮不写 Provider 适配器，避免先接入后返工或丢失一个复权口径。下一步先设计 factor_basis（shared/forward/backward）模型、持久化版本键、as_of 查询与重建规则；明确 Tushare 单列 adj_factor 的 shared 语义，再按真实样本完成公式对账。
- 状态：协议层已有实现；Provider/service 未接入；统一模型与存储需扩展；AvailableAt 仍未知；批次16继续 in_progress，阶段 B 不关闭。未执行真实请求。
- 当前 HEAD `d9a2872f8259b44f2e59615e2f966bfec2c0ad3a` 尚无关联 workflow run；PR #1 仍 Open / Draft，未合并。


## 2026-11-05 P0 批次 16：复权因子口径维度第一阶段实现

- 新增 `AdjustmentFactorBasis`：`shared`、`forward`、`backward`。Tushare `adj_factor` 显式标记为 shared；未显式赋值的单列因子归一为 shared，避免旧单因子调用方产生空口径。
- MariaDB `market_data_adjustment_factor_versions` 增加 `factor_basis` 列，版本查重、最新版本选择和索引均纳入口径维度。同证券/日期/来源的 forward 与 backward 因子可独立追加、独立 PIT 查询。
- DataService 重复行校验改为交易日 + 口径；相同日期的 forward/backward 两行不再互相视为重复。
- 复权重建按请求方向筛选匹配口径，优先使用方向专属因子；Tushare shared 因子仍可作为通用因子来源。不同口径不会进入同一因子序列。
- 新增因子哈希口径隔离测试，以及复权重建方向选择和 shared 回退测试。
- 本阶段只完成统一模型/存储/选择规则，不代表 BaoStock 已接入。BaoStock 仍需基于真实样本核验 foreAdjustFactor/backAdjustFactor 的数值公式和参考基准后再适配；AvailableAt 仍不得从 dividiOperateDate 推断。
- 当前状态：代码已提交待 CI；真实 Provider 验收未执行。阶段 B 继续 in_progress。


## 2026-11-05 P0 批次 16：口径维度首轮 CI 问题修复

- 验证基线：`e7d7d0726decd73f920b7fa967baaf45a3fabb66`。该 SHA 的 Go test suite、Go-Python parity 均失败；WebUI checks 与 Live provider probe 外层工作流成功。PR #1 保持 Open / Draft，未合并。
- Go suite 失败包含三项可复现问题：模型与新增测试文件未通过 gofmt；service 包错误引用 repository 包内不可见的 `validAdjustmentFactorBasis`；MariaDB 集成测试中 `Store.Migrate` 内嵌建表 SQL 漏加 `factor_basis`，导致查询时报 Unknown column。
- 本轮修复将 basis 校验函数放入 service 包、补齐 Store.Migrate 内嵌 schema 的 `factor_basis` 列和对应索引，并修正新增 Go 文件格式。
- Go-Python parity 的样本比较契约测试本身通过；其后 market-data Go 测试因上述编译错误失败。报告中发现的 fixture 差异（close 精度、quality_status、记录覆盖）属于现有 parity 样本报告，不作为本轮复权因子改造的根因，也不应通过放宽容差掩盖。
- 下一 SHA 必须重新执行 gofmt、market-data unit/race/vet、MariaDB migration/PIT、Docker build 和 parity。当前尚未获得修复后 SHA 的 CI 结果，不得标记通过。
- BaoStock Provider adapter 继续暂缓，直到本轮基础模型、存储及重建测试全绿，并完成因子公式/日期语义证据核验。


## 2026-11-05 P0 批次 16：基础 CI 全绿，启动 BaoStock 因子真实对账

- 最新基线 `a778e1b8fd3f8612bf3ab2508ba3198305f6f126` 的 Go test suite（含 gofmt、unit、race、vet）、market-data MariaDB migration/PIT、Docker 构建、Go-Python parity、WebUI checks、Live provider probe 均已结束且 workflow conclusion 为 success。
- 本轮 Live provider probe 的普通成功状态不代表 BaoStock 复权因子专项已经执行。专项测试通过 push commit message 标记 `[historical_baostock_factors]` 单独触发；本批启动该真实查询。
- BaoStock TCP Client 已具备 `QueryAdjustmentFactors`，返回 `dividOperateDate`、`foreAdjustFactor`、`backAdjustFactor`、`adjustFactor`。字段映射与协议解析可用，但 `OperateDate` 的业务日期语义尚不能仅凭字段名认定为可用于 PIT 的生效日期。
- 专项对账将同证券 raw / qfq / hfq 日 K 的 OHLC 与因子重建结果逐日比较。价格重建一致只构成重建映射的实证，不自动证明字段的法律/业务定义或历史首次公开时间。
- BaoStock 响应未提供已验证的首次公开时间，`AvailableAt` 必须保持 unknown；`SystemObservedAt` 仅由系统入库时生成。
- 在专项真实对账结果出来前，不接入 BaoStock AdjustmentFactorProvider，不把 `OperateDate` 直接写入可用于 PIT 的 `TradeDate`。


## 2026-11-05 P0 批次 16：BaoStock 真实因子样本对账结果与 Provider 适配

- 真实探测 workflow run `41990250314` 的 BaoStock 专项步骤成功执行（该 workflow 设置了 `continue-on-error`，因此以专项测试日志中的报告/错误为准，而不是只看 workflow 总结论）。
- `sh.600036` 获取 30 条历史因子，`sz.002982` 获取 8 条；两只股票在 2025-07-07 至 2025-07-17 均取得 raw/qfq/hfq 各 9 根日 K，按 `OperateDate <= trade_date` 选择累计因子，raw OHLC 乘 forward/backward 因子后与 BaoStock qfq/hfq OHLC 对账无 mismatch、无 item error。
- 该真实样本支持将 `OperateDate` 作为当前累计因子重建的生效日期映射；它仍不能证明字段完整的法律/业务定义或首次公开时间，也不能用于推断 `AvailableAt`。
- BaoStock Provider 适配将每个操作日期拆成 forward / backward 两条口径独立记录，忽略第三列 `adjustFactor`（样本中与 backward 数值相同，尚无独立业务语义证据），保留 `AvailableAt=nil`、`QualityStatus=degraded`，并写明可用时间未由接口提供。
- 此次专项探测同时发现新增报告结构的 gofmt 对齐问题，已在当前批次修正；修复 SHA 的 Go suite / Docker build 结果以最新 SHA 对应 CI 为准。
- 由于 BaoStock 因子缺少可证明的 `AvailableAt`，当前 PIT 可见性接口会有意排除它们；不得通过把 `OperateDate` 填入 `AvailableAt` 绕过限制。当前适配先完成真实因子采集与版本存储，重建消费策略仍需区分“当前已观察数据重建”和“历史 PIT 重建”。


## 2026-11-05 BaoStock 因子适配 CI 问题修正

- 真实对账结果已支持按操作日期拆分 forward/backward 两个因子口径；适配器首轮单测暴露了两个测试/实现问题：口径字符串字典序把 backward 排在 forward 前面，以及越界测试夹具日期实际仍落在请求范围内。
- 适配器保持按上游日期顺序输出，每个日期先 forward 后 backward；修正越界夹具范围，未放宽校验。
- MariaDB 集成测试新增同证券/日期/来源下两种 basis 并存验证，并确认无 `AvailableAt` 的 BaoStock 观测可以持久化且保留 `SystemObservedAt`，不伪造可用时间。
- 当前 SHA 后续 CI 必须重新验证 market-data 单测、MariaDB PIT 集成、race/vet 和 Docker 构建；此前失败 SHA 不得算作通过。


## 2026-11-05 P0 批次 16：当前重建与历史 PIT 消费语义隔离

- 已确认 rebuildAdjustedDailyBars 的实际生产调用点只有 DataService.DailyBarsAsOf；普通 DailyBars 目前直接请求 Provider 对应复权口径，不会调用因子重建器。因此不能把“当前重建”误说成已经接入普通行情 API。
- 重建器拆分为显式入口：rebuildAdjustedDailyBarsCurrent 与 rebuildAdjustedDailyBarsPIT。两者共用同一套来源/版本/口径一致性和价格计算逻辑。
- 当前重建允许 AvailableAt=nil，但仍要求 SystemObservedAt 非空、非零且不晚于当前系统时间，表示因子已经由本系统持久化观察；BaoStock 的 degraded 状态和 provider_available_at_not_exposed_by_query_adjust_factor 原因会继续传播到重建后的日 K。
- 历史 PIT 重建继续强制要求 Provider 明确提供非空 AvailableAt，并通过 AdjustmentFactorsAsOf 限制 FetchedAt、SystemObservedAt、AvailableAt 均不晚于查询时点。BaoStock 因子不会进入 PIT。
- DailyBarsAsOf 已改为显式调用 PIT 专用入口；保留旧私有函数名作为 PIT 语义包装，避免未来调用者无意切换到宽松模式。
- 普通 DailyBars 仍使用 Provider 原生 qfq/hfq 数据；本次不新增 API、不自动跨 Provider 拼接 raw bars 与因子。只有后续出现明确的“当前本地因子重建”产品需求时，才增加受控 service 入口，并要求 raw bars 与因子来源/版本可验证匹配。
- 验收覆盖：BaoStock 无 AvailableAt、有 SystemObservedAt 的因子可用于 current helper，重建结果保留 degraded；同一因子在 PIT helper 中被拒绝；缺少 SystemObservedAt 的临时 Provider 结果在 current helper 中也被拒绝。
- 当前状态：语义隔离代码与单测已实现，待最新 SHA 的 CI 验证；不代表普通 DailyBars 已启用本地因子重建。阶段 B 继续 in_progress。


## 2026-11-05 P0 批次 17：PIT 版本选择必须按可见版本回退

- 复核 AdjustmentFactor 与 CorporateAction 的 MariaDB as_of SQL，发现原查询先按系统记录时间选最新 revision，再由 Service 过滤 Provider AvailableAt。若最新 revision 已被系统观察、但 Provider AvailableAt 晚于 as_of，旧的已可用 revision 会被新 revision 遮蔽，造成历史 PIT 误报缺失。
- 两类查询现统一将 recorded_at、fetched_at、available_at 三个时间条件放入候选版本与 NOT EXISTS 子查询；只在“as_of 时点三项时间均可见”的候选集合内选最新版本。
- Service 层现有三时间校验继续保留，作为 SQL 结果的第二道防线；AvailableAt=nil 的 BaoStock 因子仍不会进入 PIT。
- MariaDB 集成测试新增因子与公司行动两组回退场景：v1 已可用，v2 已观察但 AvailableAt 在 as_of 之后，必须返回 v1。
- 该修复只影响 PIT 历史版本选择，不改变当前值表、Provider 采集或对外 API。待最新 SHA 的 MariaDB migration/PIT 集成与完整 CI 验证。

## 2026-11-05 P0 批次 18：Service 层 PIT 版本回退集成覆盖

- 新增真实 MariaDB 驱动的 Service 层回归测试，分别覆盖复权因子和公司行动。
- 两类数据均构造 v1 已在 as_of 前可见、v2 已被系统观察但供应商 available_at 晚于 as_of 的场景；Service 必须返回 v1。
- 该测试补足 Repository SQL 回退与 Service 三时间二次校验之间的调用链验证。MariaDB 未配置时测试 Skip，不能将 Skip 当作成功验收。
- 不改变 PIT 时间语义、版本键、API 或 BaoStock nil AvailableAt 处理规则。待新 SHA CI 验证后再更新批次状态。


## 2026-11-05 P0 批次 19：Service PIT 回退测试纳入真实 MariaDB CI

- 发现批次 18 两项 Service PIT 集成测试此前未纳入专用 MariaDB Job；普通 Go suite 因未设置 `MARKET_DATA_TEST_MYSQL_DSN` 会 Skip，不能作为真实数据库验收。
- 已新增独立 CI 步骤，注入 MariaDB DSN，精确执行因子和公司行动两项 Service PIT 版本回退测试。
- 新 SHA 验收要求两条测试日志明确 PASS，不接受 Skip 或仅凭 workflow success 推断通过。
- 本批只调整自动化验收，不改变业务代码和 PIT 语义；阶段 B 仍为 in_progress，等待新 SHA CI。


## 2026-11-05 P0 批次 20：修正 Service PIT 集成测试证券代码夹具

- 真实 MariaDB CI 已执行批次 19 新增的 Service PIT 测试，暴露测试夹具使用随机字符串而非合法 A 股证券代码的问题；生产代码校验正确，不应放宽。
- 两项测试改用合法证券代码 `600519.SH`，继续验证因子与公司行动在 Service 层回退到 as_of 可见的 v1 revision。
- 批次 19 CI 结果为失败，不能将 Job success 或 Repository 测试通过误报为 Service PIT 通过。批次 20 等待新 SHA 的真实 MariaDB 结果。


## 2026-11-05 P0 批次 21：Service PIT 回退真实 MariaDB 验收通过

- 验收 SHA：`c938d7b5f2c0e690eb70d3246f2b0ad91eb67578`。
- 专用 MariaDB Job `127144202157` 的 Service PIT 步骤中，复权因子与公司行动两项版本回退测试均明确 PASS，未发生 SKIP。
- 测试证券代码夹具已修正为合法的 `600519.SH`；生产代码的证券校验保持不变。
- 同一 SHA 的 5 个 workflow 全部完成且 success：Go test suite push `41997386635`、Go test suite PR `41997394765`、Go-Python parity `41997394676`、Live provider probe `41997394766`、WebUI checks `41997394719`。
- PR #1 保持 Open / Draft，未合并。Service PIT 回退验收通过；阶段 B 仍 in_progress，后续继续复权口径、交易边界和诊断计数审查。


## 2026-11-05 P0 批次 22：修正前复权参考日与请求结束日混淆

- Pytdx 的前复权参考日来自完整加载历史的最新交易日，可能晚于请求区间结束日；请求结束日只限定返回 K 线，不应限制复权计算基准日。
- Service 校验改为：前复权参考日不得早于任一 K 线日期；参考日不得晚于当前 Asia/Shanghai 日期；同一序列 method/reference date 必须一致。移除“参考日不得晚于请求结束日”的错误限制。
- 新增晚于请求结束日的参考日可接受、未来参考日拒绝两项回归测试。
- 本批待新 SHA 完整 CI 验证，阶段 B 继续 in_progress。


## 2026-11-05 P0 批次 23：前复权参考日校验修复通过 CI

- 验收 SHA：`6730e6e612c6b72bf57e18b2c5d8c45bf62e4d50`。
- Go test suite Push Run `41998874246`、PR Run `41998882338` 全部 Job success；Go-Python parity Run `41998881955`、WebUI checks Run `41998881909`、Live provider probe Run `41998881866` 均为 success。
- Live provider probe 的外层 workflow success 不代表真实请求全部执行；日志显示通用实时探测及 Tushare、同花顺、TickFlow 可选实时探测均因环境开关未配置而 SKIP。
- 前复权参考日晚于请求结束日的合法场景和未来参考日拒绝场景已纳入单测，并随当前 SHA 的 market-data Go tests 通过。
- PR #1 仍为 Open / Draft，未合并。批次 22 验收关闭，阶段 B 继续 in_progress。


## 2026-11-05 P0 批次 24：补充前复权参考日 Service 调用链回归

- 新增 DataService.DailyBars 级测试，验证 Pytdx 风格前复权参考日 `2026-10-22` 晚于请求结束日 `2026-10-21` 时，Service 仍返回结果并保留复权 method/reference date，不错误降级。
- 该测试补足 helper 单测与真实 Service 调用路径之间的回归覆盖；本批待新 SHA CI 验证。


## 2026-11-05 P0 批次 25：扩展 DailyBars 变更触发公开 Provider 探测

- live-provider-smoke 的 push / pull_request 路径过滤新增 DataService.DailyBars、复权重建和日 K 交易上下文相关 Service 文件。
- 公开 Provider 实时请求仍由提交标记 `[include_public]` 显式开启；普通 PR 不自动消耗线上接口额度。
- 本批触发两只样本证券 `600036.SH`、`002982.SZ` 在 2025-07-07 至 2025-07-17 的公开 Provider 日 K/复权探测，真实结果待核验。


## 2026-11-05 P0 批次 26：修复完整 Live Probe 多证券样本覆盖

- 真实运行发现完整探测路径忽略 `LIVE_PROBE_DAILY_SYMBOLS` 中第二只证券，只在 public-daily-only 分支遍历样本列表。
- 完整路径现对全部配置样本执行 Sina raw、Tencent none/qfq/hfq、Eastmoney qfq/hfq、BaoStock qfq/hfq、Yahoo qfq/hfq；可选 NetEase raw 同样遍历。Quote、公告等非日 K 能力仍只使用首只样本。
- 增加样本证券列表解析测试，验证多证券、默认值、空项与重复项。
- 新 SHA 待两只样本真实探测和完整 CI 验证。


## 2026-11-05 P0 批次 27：执行 Pytdx 与 Service 真实历史诊断

- Pytdx 前复权参考日测试改名并补充契约注释，明确参考日可晚于请求结束日，但输出 K 线仍受请求日期范围约束。
- 通过 live-provider-smoke 提交标记触发 Pytdx 两只证券 raw/qfq/hfq + GBBQ 真实探测，以及生产 DataService Probe 两证券、三复权口径的 MariaDB 持久化诊断。
- 结果待真实运行完成后核验。


## 2026-11-05 P0 批次 27：Pytdx 与生产 DataService 历史探测结果

- SHA：`2477e92d5fd439f4ed97bae971ad0ffe9ae2085b`。Push Go tests Run `42003400135`、Push Live probe Run `42003400131`、PR Go tests `42003411612`、Parity `42003411993`、WebUI `42003411698`、PR Live probe `42003411518` 均 success；均对应当前 SHA。PR #1 仍 Open/Draft、未合并。
- Pytdx 两只样本 `600036.SH`、`002982.SZ` 的 raw/qfq/hfq 在 2025-07-07 至 2025-07-17 均返回 9 根、quality valid；qfq reference date 均为 2026-11-05（晚于请求结束日），hfq reference date 分别为 2002-04-09、2020-04-24。
- GBBQ/BaoStock 公司行动三条事件全部 matched；`002982.SZ/2025-05-30` 的理论除权价差 0.0042857 元、factor ratio 差 0.0004050，低于现行阈值但接近 factor 容差上限，列为边界样本待评估，不据此扩大容差。
- 生产 DataService Probe 两只证券各 15 项：13 degraded、2 not_supported、0 failed，均持久化且读回通过。Sina qfq/hfq 为 not_supported；两只证券 Eastmoney+BaoStock raw 交叉比较各 9 日期、54 字段全部匹配；每只 storage check passed。
- 全部返回项附加 `daily_bar_context_calendar_degraded`，源于当前只配置 BaoStock 日历，日历快照缺少 Provider snapshot timestamp；这表示交易日上下文质量降级，不代表 K 线 OHLC 全部错误。Sina/Tencent/Yahoo 另缺成交额；Tencent/Eastmoney/BaoStock qfq 参考日未验证。
- 本批专项完成，但生产 Probe 总体仍 degraded，不关闭数据质量验收。下一步审查日历降级语义：区分日历日期覆盖/来源冲突与快照时间戳缺失，确定何时应影响 K 线质量、何时只作为上下文 warning；禁止伪造时间戳或强制 valid。


## 2026-11-05 P0 批次 28：历史交易日历时间戳缺口分级

- 对完整历史日历，若唯一质量缺口是 Provider 未暴露/缺少快照发布时间，且日历结束日期早于当前 Asia/Shanghai 日期，则保留 `daily_bar_context_calendar_snapshot_time_unverified` warning，但不单独降级历史 K 线质量。
- 当前/未来日历、日历来源冲突、仅返回交易日而推断非交易日、其他质量缺口继续 degraded；缺少日历仍 degraded。
- 该修复不伪造上游时间、不放宽日期覆盖校验。新增三项回归测试，等待新 SHA CI 与真实 Probe 复验。


- 边界修正：历史判定按上海时区的自然日比较，当前日期本身仍属于 current calendar，不会因当天零点早于当前时刻而误判为历史日期。


- CI 修正记录：SHA `11e742e5e89df4776798a8622f12d68ed246fc84` 的 Go test suite 在 `Check gofmt` 发现新增测试文件三处字段对齐不符合 gofmt，属于格式检查失败；已在本提交修正格式，并通过提交标记重新触发生产 DataService 历史 Probe。该 SHA 不作为通过验收的依据。


## 2026-11-05 P0 批次 30：历史交易日历质量边界真实复验

- 验收 SHA：`a27067dbdc1ae288afeb75b2fbc60f471e9087cc`；Go push/PR、Parity、WebUI、Live push/PR 六个 workflow 全部 success。旧 SHA `11e742...` 的 gofmt 失败已修复，不作为通过结果。
- 生产 Service Probe 两只证券各 15 项：4 returned、9 degraded、2 not_supported、0 failed，均持久化成功；connectivity/contract 通过，storage 通过。
- 历史日历时间戳缺失只保留 warning；剩余降级来自成交额缺失、前复权参考日未知和 Provider contract warning。
- 每只证券 Eastmoney raw 与 BaoStock raw 9 日 × 6 字段 54/54 matched；其余 21 项 cross-provider 比较 skipped，无 mismatch；adjusted 跨源一致性仍未验收。
- PR #1 Open / Draft，未合并。批次 29 关闭；下一步评估 BaoStock 因子与同源 raw K 的 current 重建证据链，Eastmoney/Tencent 不推断锚点，Tushare 40203 不重复请求。阶段 B 继续 in_progress。


## 2026-11-05 P0 批次 31：复权事件因子向后对齐与同源约束

- BaoStock `query_adjust_factor` 返回除权除息事件日期 `dividOperateDate`，不是逐交易日因子；重建时按每根 raw K 日期选择最近的 `OperateDate <= trade_date` 事件因子。首根 K 之前无可用事件时拒绝重建，不回退到未来因子。
- 强制 raw K 与因子同证券、同 Provider、同 source_version，且 raw K 必须为 none basis；禁止跨源拼接。
- 前复权参考因子按 `OperateDate <= requestedReferenceDate` 选最新事件，参考日期不得早于最新 raw K；后复权按每根 K 的最近事件因子计算。
- current 仍允许 AvailableAt nil，但必须有持久化 SystemObservedAt，输出保留 degraded；PIT 仍要求 AvailableAt 有证据。
- 新增事件因子日期向后对齐、跨源/跨证券拒绝、首因子之前日期拒绝及 current/PIT 可见性测试。当前 helper 未接入普通 DailyBars，等待 CI 与真实复权对账。


- 防御性补充：current helper 即使收到错误标记为 `valid` 但 `AvailableAt=nil` 的因子，也会强制降级并写入 `provider_available_at_unknown`；PIT 仍直接排除该因子。回归测试验证该防御，而不是依赖 Provider 一定正确标记 QualityStatus。


- CI 格式修正：SHA `ac743f19cdbed45d2b0d44c5c163fb3be2cec7a2` 的首次 Go suite 在新增测试函数前多出一行空行，`Check gofmt` 失败；仅修正格式，不改变逻辑。下一 SHA 重新执行完整 CI。

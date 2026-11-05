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

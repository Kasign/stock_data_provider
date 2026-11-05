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
- BaoStock `adjustflag=3/2/1` 只证明请求的 none/qfq/hfq 口径；历史复权因子与 K 线内部复算 mismatch=0 是样本窗口内部一致性证据，不证明跨源绝对基准。
- Pytdx 使用 GBBQ 事件执行仿射 QFQ/HFQ；当前 AdjustmentRefDate 由完整扫描序列的日期边界确定（QFQ 最新日、HFQ 最早日），属于算法定义的基准，不是上游字段回显。必须保留完整扫描边界约束。
- `rebuildAdjustedDailyBars` 已要求同一 source/version 因子覆盖全部原始 Bar 和参考日期，并对前复权记录实际参考因子日期；不需要另建因子重建逻辑。
- 未发现可证明的生产代码缺陷。本轮不改代码、不新增字段/API、不调整 Provider 算法；现有“参考日未知则不可比较”门禁正确。
- 交易上下文证据不变：Pytdx 对 4 条官方盘中临停日期均返回唯一 raw 日 K；Tushare `suspend_d` 仍因 40203 权限不足无法核验时段字段。二者不是同一验收项。
- 本批次代码审计结论通过；Eastmoney/Tencent 参考日仍不可比较；Tushare 时段映射仍阻塞；阶段 B 继续 in_progress。
- 后续优先评估 BaoStock 现有同源因子与 raw K 线能否提供带明确日期语义的可复验锚点；只有取得数据证据后才考虑增加元数据。不得重复请求已完成的样本。
- HEAD `b4cf1a5dbd898f000cf679fa0eb73be6f1039a2e` 的四个关联工作流均 success，但 Live Provider Probe 的 `MARKET_DATA_LIVE_PROVIDER_TESTS=0`，本 SHA 没有重新执行真实 Provider 请求。历史样本不升级为本 SHA 新验收。

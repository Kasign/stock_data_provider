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

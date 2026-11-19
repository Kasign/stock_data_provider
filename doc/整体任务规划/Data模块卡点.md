# Data 模块卡点

更新时间：2026-11-18  
维护分支：`feat/data`  
模块范围：`internal/marketdata/**`；`doc/api/**`

## 当前卡点：DATA-003 Fundamentals PIT 真实披露时点证据

**状态：BLOCKED（外部真实数据证据不足；任务总体仍为 IN_PROGRESS）**

### 已完成且不应重复执行

- PIT Service、修订版本选择、Repository 历史可见性和 MariaDB 持久化链已完成静态审查。
- RFC3339 输入、UTC Instant 归一化、UTC `Z` 输出及相应 Go 测试已完成。
- Provider 时间字段来源、精度、权限和异常样本矩阵已归档。
- 当前代码基线 `3a4ee76b63ab319d9521393b3ebd1d808698d6e5` 的 Go Test Suite Run `44297261928` 通过，包含 Market Data tests、Race、Vet、gofmt 和 MariaDB PIT 集成测试。
- 不再重复开发 PIT 代码，不重复核验 UTC 时间格式，不重复请求权限未变化的 Tushare `fina_indicator`。

### 证据缺口

| Provider / 候选来源 | 已知事实 | 当前判定 | 解除条件 |
|---|---|---|---|
| BaoStock Fundamentals | `pubDate` 仅日期粒度 | degraded；同日精确可见性不可证明 | 供应商提供可审计的精确首次公开时间，或取得等价的可信证据 |
| Hithink 财务报表 | 存在真实披露日期与官方披露日不一致样本 | degraded；异常样本不得提前放行 | 供应商修正或提供可独立验证的原始披露时间证据 |
| Tushare Fundamentals | 历史请求返回 40203 权限错误 | permission_required / not_executed | 权限实际变更后再按授权执行真实探测 |
| Eastmoney 公告 Adapter | 已有 `display_time → PublishedAt` 解析；目前只有合成测试样例 | 对 Fundamentals AvailableAt 为 not_comparable | 取得真实公告响应和公告原文，并证明字段语义及其与目标财务记录的对应关系 |

### 禁止的绕过方式

1. 不得将 `FetchedAt` 或 `SystemObservedAt` 回填为 `AvailableAt`。
2. 不得将报告期末、公告日期或公告展示日期拼接为精确时刻。
3. 不得将 Eastmoney 公告 `PublishedAt` 直接赋给 BaoStock、Hithink 或 Tushare Fundamentals 的 `AvailableAt`。
4. 不得因 Go CI、MariaDB 集成测试或 Live Probe 工作流整体成功，就宣称真实 Provider 披露时点通过。
5. 不得为了关闭任务放宽 PIT fail-closed 门禁。

### 下一步

- 有新的真实 Provider 原始响应、供应商字段契约或权限变化时，补充对应证据并重新判定。
- 在证据到来前，DATA-003 保持 IN_PROGRESS；不为消除阻塞而虚构数据或反复运行相同静态审查。
- DATA-004 Valuations 已有历史部分验收：P0-84 完成输入链路审查；P0-85 已对 600519.SH / 2026-11-12 的 PE_TTM、PE_MRQ、PB_MRQ、PS_TTM、PCF_TTM 完成真实样本独立复算，五项均在六位小数上吻合。该单样本不得重复复算，也不代表全市场/多日期验收完成；PE_MRQ 公式解释仍待供应商确认，ProviderReportDate/逐字段 Freshness 仍缺证据。
- DATA-005 Freshness 已有 Evaluator、策略注册表和 Probe 接入；DailyBar、Fundamentals、Valuations 的独立 SLA 仍缺可靠源端时间/业务周期证据。不得重建框架或用 fetched_at 代替 provider_time。
- DATA-006 Go/Python parity 已有 Go 侧契约与工作流；缺少固定旧 Python 可执行基线及同输入双侧真实输出，不能把 Go-only 工作流成功视为双侧数值对账。
- DATA-007 已有多 Provider/能力级真实样本、权限阻塞和失败记录；只补明确缺口，不重复已有证券/日期/能力/Run。
- DATA-003 当前阶段记录已归档至 `doc/api/任务历史/03-DATA-003-Fundamentals-PIT完整性阶段记录.md`，真实 Provider 披露时点仍外部阻塞，不重复执行已有 PIT 机制验收。
- DATA-009 已完成并归档：Fundamentals/Valuations 重复保存不新增修订、内容变化新增修订的 MariaDB 集成断言通过；Go Test Suite Run `44307383999` success。详见 `doc/api/任务历史/05-DATA-009-Fundamentals-Valuations修订写入幂等性验收完成状态.md`。
- DATA-009 完成后已核对 DATA-003～DATA-007 的剩余缺口：DATA-003 缺披露时点证据；DATA-004 缺 PE_MRQ 公式/ProviderReportDate/逐字段源端更新时间证据；DATA-005 缺 DailyBar 发布宽限规则及财务/估值源端时间；DATA-006 缺固定 Python 运行环境下的双侧配对样本；DATA-007 缺专项 Provider 真实请求证据。以上均不能通过重复静态审查、合成数据或 Go-only CI 关闭。
- 当前无可独立执行的已登记任务。恢复条件和禁止重复事项见 `doc/api/当前任务状态和进度.md`。DATA-008 继续 DEFERRED。
- DATA-004～DATA-007 的历史证据已在总规划标记为部分进行；后续切换时必须先复用这些证据、只处理未关闭子项，并按总规划正式更新当前任务状态。

## 非 Data Owner 卡点（仅供责任边界追踪）

Root Server 尚未实际构造并注册 Data Module，属于 Server SRV-009 的装配责任，不是 Data 分支继续修改 `internal/server/**` 的授权。Data M1/M2 已完成；Data 不复制 Platform 公共能力，也不接管 Server 组合根。


## RepairAll 聚合分支修复（2026-11-19）

`repair_all` 已实施 Data Migration 统一，不代表 `feat/data` 已同步：

- 静态定义现由 `internal/marketdata/repository.MigrationDefinitions()` 导出；V1-V3 保持不变，V4 负责旧索引和缺失列兼容。
- 兼容 DDL 已从 Platform Apply Runner 锁外移入 MigrationOperation，在同一 namespace advisory lock 和 checksum 下执行。
- MariaDB 旧库兼容和双实例并发测试已新增，等待 Run [44447674606](https://github.com/Kasign/stock_data_provider/actions/runs/44447674606)。
- DATA-003 的 Provider AvailableAt 证据、DATA-004～007 的外部数据/字段/Parity 证据仍按原阻塞条件处理；本次迁移收口不改变其状态。


## RepairAll 物理 Schema 漂移检查（2026-11-19）

`repair_all` 新增 `Store.ValidateCompatibilitySchema`，并接入 Data `Ready`：V4 两列必须是 nullable DECIMAL(16,8)，revision 索引必须列顺序正确且为非唯一，旧 unique index 必须不存在。该检查用于补足 Migration checksum 不等于物理 Schema 完整性的边界。MariaDB 集成测试模拟删列并验证 Ready 拒绝；最新代码 SHA `b866188cf2b784a2e9c2d9a11ff002962a3453bd` 的 CI 尚待确认。模块分支 `feat/data` 未同步。


## RepairAll 物理 Schema 漂移门禁 CI 结果（2026-11-19）

- 当前 repair_all 验收 SHA：`5d97dc89acfd0493b8b7f4c316db6a5d5dc609e9`。
- Integration CI [Run 44452547201](https://github.com/Kasign/stock_data_provider/actions/runs/44452547201) 对该精确 SHA 全项 success。
- MariaDB 测试先通过 Root Registry 执行 Data Migration V1-V4，再对 Ready 执行缺列/缺索引 fail-closed 检查和恢复检查；原测试缺少 Data Migration 装配，已修正，不曾通过放宽物理 Schema 校验绕过。
- 当前结论：repair_all 代码层物理 Schema 门禁 PASS；生产环境 Migration 开启/关闭演练仍待执行。模块分支 `feat/data` 未同步。
- DATA-003 的真实 AvailableAt 证据及 DATA-004～DATA-007 的外部字段、Freshness、Parity、Provider 真实请求证据仍未解除。


## RepairAll Migration Checksum 与物理 Schema 漂移边界（2026-11-19）

- 最新 repair_all 代码 SHA：`ff9a98457220ae307b9dc4a62ce274849cd60f12`；Integration CI [Run 44453457919](https://github.com/Kasign/stock_data_provider/actions/runs/44453457919) 全项 success。
- 新增 MariaDB 断言：删除 V4 兼容列或 revision 索引后，Migration checksum 校验仍通过；Data Ready 返回 `market_data.schema_drift`。恢复结构后 Ready 恢复通过。
- 这证明迁移 history checksum 与物理 Schema 漂移检查职责分离，防止未来把 checksum 成功当成数据库结构健康证明。
- 生产环境 Migration enabled/disabled 演练仍待执行；真实 Provider 数据与时间证据卡点不变。模块分支 `feat/data` 未同步。


## RepairAll 物理 Schema 校验最终 MariaDB 兼容结果（2026-11-19）

- 最新代码 SHA：`d386e097db64e6913290441ccc7bce0602358348`；Integration CI [Run 44454928267](https://github.com/Kasign/stock_data_provider/actions/runs/44454928267) 精确 SHA 全项 success。
- 修正了 `COLUMN_DEFAULT` 的 MariaDB 元数据误判：不再把可空列的 NULL 默认表示视为显式默认值；列校验改为完整 signed DECIMAL 类型、precision/scale 和 nullable。
- 索引校验补充拒绝前缀索引、非 BTREE、非升序索引；MariaDB 集成测试覆盖 UNSIGNED 列和前缀索引漂移。
- Root Worker/API Bootstrap 与 Data Repository Schema 漂移集成测试全部通过。生产环境 Migration enabled/disabled 演练仍待执行；`feat/data` 未同步。


## RepairAll Migration disabled 物理漂移门禁（2026-11-19）

- 最新代码 SHA：`02415a319fbe45921be2be469592d84043676626`；Integration CI [Run 44455503870](https://github.com/Kasign/stock_data_provider/actions/runs/44455503870) 全项 success。
- API Bootstrap 集成测试先执行 Migration enabled，再模拟删除 `market_data_quotes.turnover_rate`，之后 Migration disabled 启动必须 fail-closed，且 information_schema 确认 Root 未自动补列。
- 代码级 enabled/disabled 启动路径门禁均已覆盖；生产部署演练和真实 Provider 证据仍未完成。模块分支 `feat/data` 未同步。

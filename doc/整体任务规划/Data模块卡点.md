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

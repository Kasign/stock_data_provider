# Data 模块卡点

更新时间：2026-11-28

维护分支：`repair_all`（聚合验收）

模块范围：`internal/marketdata/**`；`doc/api/**`

## 当前卡点：DATA D2 精度与 Snapshot 链路（先完成 Quant 首个真实指标所需的最小输入切片）

**状态：IN_PROGRESS；本轮仅关闭锚点质量门禁子项，Provider/Decimal 主体尚未关闭。**

- 已完成：无 reference date 时按 Provider method 做后复权锚点门禁；未知方法降级；实时与缓存路径测试覆盖。
- 真实样本：Hithink 23 bars/30 corporate actions 可用仿射公式复算 OHLC，但 adjusted volume 为小数，旧 DailyBar.Volume int64 归一化丢弃，禁止四舍五入。
- 未完成：Hithink 不同 end date 的 anchor 对照；Hithink CorporateActionProvider 生产接入；AdjustmentFactor/CorporateAction exact source token；Decimal 仿射重建及 current rebuild 调用路径。
- 外部阻塞：Tushare adj_factor/dividend 返回 40203；Eastmoney 本轮 EOF/502。不得将 live probe 标记为全 Provider 通过。
- 代码 SHA `e8d1c5464bd78dc83d6a9f51df41ea2d86a7fbba` 的 Go Test Suite 与 Integration CI 通过。完整证据见 `doc/api/依赖/02-DATA-CTR-005与007解阻方案.md`。

## 其他历史卡点：DATA-003 Fundamentals PIT 真实披露时点证据

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
- DATA-003～DATA-007 的真实 Provider/外部证据缺口没有可通过重复静态审查关闭的任务；但 `repair_all` 阶段二已新增 Data 内部可执行工作：不可变交易日历内容版本 Repository（Migration V5），具体实现与 CI 状态见 `doc/api/当前任务状态和进度.md`。这不代表 `feat/data` 已同步。DATA-008 继续 DEFERRED。
- DATA-004～DATA-007 的历史证据已在总规划标记为部分进行；后续切换时必须先复用这些证据、只处理未关闭子项，并按总规划正式更新当前任务状态。

## 非 Data Owner 卡点（仅供责任边界追踪）

Root Server 的 Data Module 构造、共享资源注入、Migration Registry、InProcessHandler、Analysis Application 注入、Readiness 和 Lifecycle 已在 repair_all 代码级完成。WebUI Data API 已切换 Owner Session / 单 /api 入口，Data 独立服务与 Data API Key 认证已退役。剩余工作归 Root 集成验收和真实生产部署，不要求 Data 模块重复装配或重建 HTTP API。


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


## RepairAll DATA-CTR-007 / Research 消费边界复核（2026-11-19）

- internal/marketdata/app/port.go 已提供 MarketDataReader 与 DailyBarsSnapshot，但该快照当前不包含内容寻址 SnapshotRef、Manifest/Payload 绑定和 Data AuthorizationRef。
- internal/research/ 当前没有 Data Adapter；不得将普通 DailyBarsSnapshot 或 Bundle.snapshot_id 推定为不可变 Evidence 引用。
- 项目级 doc/contract/PLAT-002-项目级契约冻结登记簿.md 明确 DATA-CTR-007 为 deferred，解锁责任是 Data 定义不可变行情 Payload、内容寻址 SnapshotRef、Manifest/Payload 绑定及 Research Evidence source_ref/version/locator 映射。
- 判定：Data→Research 生产消费 NOT_COMPARABLE / DEFERRED；本轮未发现可在当前契约边界内安全实现的局部代码修复。Data 继续负责 DATA-CTR-007 契约和数据事实，Research 不复制 Data 模型/存储/授权。


## RepairAll RA009 Data 入口退役状态（2026-11-19）

- Data 的独立 HTTP API Key 字段、配置读取、Handler 认证和独立服务命令已删除；Provider 自身 Hithink/TickFlow/Tushare/MiaoXiang 等凭据仍保留为服务端配置。
- Data HTTP Adapter 仅由 Root Server InProcessHandler 使用；Request ID 从 Platform observability Context 读取，不再复制独立 Data HTTP middleware。
- Data Migration Definitions 仍由 Root Migration Registry 统一注册；已删除只服务独立 Data 命令的 Module.MigrationRunner 兼容入口。
- DATA-CTR-006（Data X-API-Key HTTP 认证）已在项目契约目录标记 retired；外部路由认证以 SRV-CTR-002 为准，Data 领域授权仍以 DATA-CTR-008 为准。
- RA009 代码验收已由精确 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 的 Integration CI Run [44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success；不再等待该项 CI。目标环境 M6 真实验收仍开放。DATA-003～DATA-007 的 Provider/时点/Freshness/Parity 证据卡点保持原状态，不因部署迁移自动关闭。


## RepairAll 阶段二卡点状态校准（2026-11-19）

- RA009 Data Root 单入口代码与精确 SHA CI 已通过（Run 44482650366）；此前“等待最新 CI”的文字仅作为当时历史记录，不再是当前卡点。
- 当前 Data 仍开放的实质卡点为 DATA-CTR-007、DATA-003～DATA-007 外部数据证据；M6 生产部署证据由 Server/部署运维负责，不重复归入 Data 代码待办。
- 统一责任与解锁顺序见本目录 [RepairAll跨模块修复计划与进度](./RepairAll跨模块修复计划与进度.md) 第 2 节“修复顺序”及后续日期记录（旧引用“第 28 节”为历史编号）。


## RepairAll 阶段二：交易日历不可变版本实现（2026-11-19）

- Data 已在 `repair_all` 增加 Migration V5 与内容寻址交易日历 Snapshot Repository；保存和精确读取按 SHA-256 校验，重复内容幂等，内容或质量变化生成新 Ref。
- 该能力为 DATA-CTR-007 的日历版本前置子项；未完成 DailyBars v2、Manifest/Payload 绑定和 AuthorizationRef 前，DATA-CTR-007 仍 deferred。最终代码 SHA `4aa468e4de6fc51a16068d43ab2229c5b0181ab9` 的完整 Integration CI Run `44493758123` 已全项 success。


## RepairAll D1 精确值链路复核（2026-11-19）

- 已确认默认 7 个 DailyBars Provider 均生成 ExactValues；Normalize 按值保留；Repository sidecar 持久化、Revision Hash 绑定、current/PIT 读取恢复均有实现和测试。Pytdx 兼容 Float64() 不是精确值唯一来源，ExactValues 使用 Price.Int64()；原“Pytdx 精度入口丢失”卡点撤销为误判。
- 实际跨模块缺口是 `app.DailyBar` 未暴露 ExactValues，`application.mapDailyBar` 丢弃该字段。本轮增加 Application DTO、深拷贝映射及 Reader 回归测试，待精确 SHA CI 后关闭。
- 仍开放：可配置 Hithink/Tushare/Yahoo 的 ExactValues 覆盖核验；Amount 单位证据；所有 Provider 的 ProviderTime/逐行数据版本；DailyBars v2 DTO/Schema/Golden；DATA-CTR-007 Payload/Manifest/AuthorizationRef。


## RepairAll D1 精确值 Application Port 修复验收与剩余边界（2026-11-19）

- `app.DailyBar` ExactValues 同构 DTO、深拷贝映射和 current/PIT Reader 测试已通过精确 SHA `a2bbfee6fd02f6c54242349419de7273b8fa4a76` 的完整 Integration CI Run `44501617234`；Application Port 丢失缺陷 CLOSED。
- 默认 7 个 Provider 已生成 ExactValues；Hithink/Tushare/Yahoo 也在部分路径生成，但 Tushare/Yahoo QFQ/HFQ 缺精确调整值 sidecar。Canonicalizer 去除尾随零，不保留原始 token scale；ChangePercent 没有精确值字段。
- 仍开放的 Data 卡点：字段 lexical scale/precision evidence、ChangePercent 精确来源、QFQ/HFQ 精确调整、Hithink/Pytdx Amount 单位证据、ProviderTime/逐行数据版本、DailyBars v2 DTO/Schema/Golden、DATA-CTR-007 Snapshot/Manifest/AuthorizationRef。


## RepairAll D2 DailyBars v2 设计审查卡点（2026-11-19）

- 新增 Data 内部 D2 设计审查草案，尚未修改项目级契约状态。
- 必须解决：CalendarRef 与实际 DailyBars 质量评估结果同一性传播；ChangePercent 精确来源；原始 token scale evidence；Tushare/Yahoo QFQ/HFQ 精确调整；Amount 单位证据；ProviderTime/provider_data_version 缺失语义。
- DailyBars v2 必须同时覆盖 Application Port 与 HTTP Schema 的版本化 DTO，并通过 Research Evidence / Quant Consumer Contract Test；不得修改现有 v1 number/float64。
- DATA-CTR-005/007 和 QNT-CTR-003 继续 deferred，直至项目级重新进入条件满足。


## RepairAll PIT 日历首轮 CI 格式问题（2026-11-19）

- SHA dc866b3595fb49594446b15a9b8084c754c70a19 的所有功能测试和构建步骤均通过，唯一失败为新增 MariaDB 集成测试前多一行空行触发 gofmt。
- 已删除多余空行；新 SHA 需重新完整验收。


## RepairAll PIT 日历修复验收结果（2026-11-19）

- 修复 SHA 08684e8a9b50641b4d32c38ab6ac9a3c7dd808e0 的 Integration CI Run 44504593484 全项 success。
- DailyBarsAsOf 使用历史 CalendarRef 的代码、MariaDB 多版本选择测试和禁止实时 Provider 调用测试均通过。
- 当前 Data 卡点从“PIT 使用当前日历”转为“将实际使用的 CalendarRef 绑定到 DailyBars v2 Snapshot Header”；另有 ChangePercent 精度、lexical scale、部分 Provider 复权精度、Amount 单位和 ProviderTime/provider_data_version 等未决证据。


- 新增 Service + MariaDB 集成测试，验证 asOf 选择正确历史 CalendarRef，且配置的实时 Provider 调用次数保持 0；新 SHA 待 CI。


## RepairAll PIT Calendar AsOf Service 集成验收（2026-11-19）

- Service + MariaDB 集成测试验证：当 asOf 前旧 CalendarRef 与 asOf 后新 CalendarRef 同时存在，且实时 Provider 返回新版本时，`TradingCalendarAsOf` 仍只返回旧版本，live Provider 调用数为 0。
- 测试 fixture 使用独立 1990 日期区间，避免与同套件其他日历内容地址/观察时间共享造成顺序依赖。
- 格式修复提交 `443e851b7bd48d98b537c467a9c1493bea751735`，Integration CI Run [44506607149](https://github.com/Kasign/stock_data_provider/actions/runs/44506607149) 全项 success；Run [44505628048](https://github.com/Kasign/stock_data_provider/actions/runs/44505628048) 唯一失败为该测试文件多余空行触发 gofmt，已修正。

## RepairAll D2 字段级设计审查补充（2026-11-19）

- DailyBars coverage 当前会读取真实 Calendar Snapshot，但 helper 丢弃 CalendarRef；这是 v2 Header 尚未能证明日历同一性的实际代码缺口。后续通过同次 coverage evidence 返回 ref，不再二次查询。
- ChangePercent 的来源与计算方式并不统一：Tushare/BaoStock/Netease 有原始来源 token/string；TickFlow 有来源/派生混合；Pytdx/Yahoo 当前是 float64 派生。derived Decimal 需要明确精度与舍入规则，未冻结前必须 null。
- v2 必须区分 canonical Decimal、原始 lexical scale、单位证据、adapter/normalization/provider data version 和 system observation time；不能把 float64 或 DB DECIMAL scale 反向包装为来源精度。
- 版本化 Application/HTTP DTO 独立于 v1；不改变现有消费者契约。DATA-CTR-005/007 与 QNT-CTR-003 保持 deferred。


## RepairAll Pytdx Amount 单位证据待核（2026-11-19）

- 当前代码 `pytdxDailyBarExactValues` 将协议 `Kline.Amount` 按 CNY 传入 ExactValues helper，但所用依赖 v0.0.86 的 protocol 文档只注释为“成交额”，未给出该字段单位。
- TDX 官方量化文档对 `get_market_data` 的 Amount 标注“万元”，公式 AMOUNT/AMO 指标则标注“元”；属于不同 API/字段语义，不能互相替代。现有 Adapter 的 CNY 映射需视为待核假设。
- D2 v2 的 `amount_cny` 暂时不得沿用该假设；单位证据确认前为 null/degraded。待核对的是与 v0.0.86 标准 Kline.Amount 完全对应的协议单位，不是重新猜测 Price 类型的缩放。


## RepairAll DailyBars v2 CalendarRef Application Port 卡点关闭（2026-11-19）

- Service coverage helper 已返回同一次日历评估使用的 CalendarRef 和 coverage evidence；PIT 只使用 asOf 可见日历版本，不触发 live Provider。
- 新增独立 Application `DailyBarsV2Snapshot` / `DailyBarsV2Reader`，提供 `interval=1d`、可空 `trading_calendar_ref`、日历质量、交易日分母、阈值和 warnings；v1 Service/Application/HTTP 契约不变。
- 无明确 start/end 或 CalendarRef 未持久化时 v2 显式降级；v1 既有质量语义保持兼容。
- current/PIT 映射、缺失引用/无界范围降级、MariaDB 日历版本同源测试通过。精确 SHA `1c23678b5a80c07e5f31b3e5008d1e031cdd8048` 的 Integration CI Run [44509890582](https://github.com/Kasign/stock_data_provider/actions/runs/44509890582) 全项 success。
- 本卡点仅关闭 Application Port 传播。HTTP v2 Schema/Handler/Golden、Research/Quant 消费者契约测试仍未完成；DATA-CTR-005/007 和 QNT-CTR-003 保持 deferred。


## RepairAll v2 无界查询门禁与最终 Payload 卡点（2026-11-19）

- Application v2 对缺少 start/end 的查询增加门禁：即使内部结果携带 CalendarRef，也不对 Header 暴露，明确标记 `unbounded_range_estimate` 并降级。精确 SHA `6d00369b6b9c66261eb47625e6bec7a2d80217e1` 的 Integration CI Run [44510702240](https://github.com/Kasign/stock_data_provider/actions/runs/44510702240) 全项 success。
- CalendarRef 同次评估传播到 Application v2 Header 已关闭；但当前 `DailyBarsV2Snapshot.Bars` 仍复用 float64 Application DailyBar DTO，只是 Header 阶段，不是最终 Decimal Payload，不可对外发布或冻结 DATA-CTR-005。
- 下一步：完成 DailyBarV2 Decimal/lexical-scale evidence 字段设计和 Provider 映射；对没有可靠来源的 ChangePercent、AmountCNY、QFQ/HFQ 调整值保持 null/degraded；之后再接 HTTP v2 Schema/Handler/Golden 与 Research/Quant 消费契约。


## RepairAll v2 日历质量 fail-closed 卡点关闭（2026-11-19）

- 有 ref 但日历质量不可靠时，v2 Header 不暴露 CalendarRef，coverage basis 为 `degraded_exchange_calendar`，Snapshot/Bars 降级；仅 timestamp-only 历史降级保留可靠 CalendarRef。
- 精确 SHA `7c5f6e3c5c5b4bb146d46e60d1c723bd8fbce9c2` 的 Integration CI Run [44511526743](https://github.com/Kasign/stock_data_provider/actions/runs/44511526743) 全项 success。
- CalendarRef 同次评估、无界查询和质量 fail-closed 已覆盖；最终 Decimal Body 与 HTTP v2 仍是下一阶段工作。


## RepairAll 日线精确词法与涨跌幅来源证据实现（2026-11-20）

- ExactValues 新增每个 Decimal 字段的 precision evidence：canonical value 独立于本次 Provider 原始 token/小数位；Pytdx 记录协议固定 scale。observed_token 仅代表单次响应，不推断 Provider 合同精度。
- Tushare `pct_chg`、BaoStock `pctChg`、Netease CSV `change_percent` 已保留精确来源值、token、scale 和 source field；TickFlow 等派生值不写入 provider-source evidence。
- Tushare amount 换算记录 1000 因子；Pytdx/Hithink 未证实的成交额单位不生成精确 `amount_cny`。v1 兼容字段不变。
- Application Port 映射及深拷贝测试覆盖新增字段。
- 精确 SHA `baaa2a57051e064f4f91a54851443379636bb685` 的 Integration CI Run [44658725187](https://github.com/Kasign/stock_data_provider/actions/runs/44658725187) 全项 success。
- 后续仍需完成 DailyBarV2 Decimal Body、Tushare/Yahoo QFQ/HFQ 精确调整、ProviderTime/provider_data_version、HTTP v2 与 Research/Quant 消费者契约；DATA-CTR-005/007 和 QNT-CTR-003 不变更为 frozen。


## 2026-11-20 模块分支基线同步状态

- feat/data 已选择性同步 repair_all 的 internal/marketdata/** 与 doc/api/**，代码同步 SHA 5b98ae1abbf0e04fa4c275b15f78087b0481c0f1；上述模块路径已逐文件 Blob SHA 对齐。
- 根模块 Test . 和 Market-data MariaDB migration/PIT job 在 Run 44667706336 通过；整体 Run failure 来自共享 Workflow 仍引用已删除 cmd/market-data 的两个旧 Job，归 CI-01，不归 Data 业务代码。
- Live Provider Probe Run 44667706203 的 11 个真实专项全部 skipped，DATA-003～DATA-007 的外部证据阻塞不变；DATA-008 继续 DEFERRED。


## RepairAll D2 DailyBars v2 现状校准（2026-11-23）

- 当前 `repair_all` SHA `6139bcbe7b40bc82564783ffb4af41710be32b09` 已有 DailyBarsV2Reader/Application Header、CalendarRef 同次覆盖率证据、不可变交易日历 Repository，以及 DailyBar ExactValues sidecar 的 Repository/Application 传递。
- 当前 `DailyBarsV2Snapshot.Bars` 仍复用 float64 `DailyBar`；最终 Decimal Body、HTTP v2 Schema/Handler/Golden、Research/Quant 消费契约均未完成。此状态不得描述为 DailyBars v2 已完成。
- Tushare/Yahoo QFQ/HFQ 精确调整值、部分 Provider Amount 单位、逐条 ProviderTime/provider_data_version 仍缺证据；未知值保持 null/unknown/degraded，不用 float64 格式化、FetchedAt 或 Adapter SourceVersion 伪造。
- 不可变交易日历和 CalendarRef 传播已完成，不重复开发；DATA-CTR-005/007 仍 deferred。


## 2026-11-23 D2：Pytdx QFQ/HFQ 精度与算法来源复核

- 当前 `go.mod` 固定 `github.com/injoyai/tdx v0.0.86`；`internal/marketdata/provider/pytdx.go` 的 Provider SourceVersion 为 `pytdx-injoyai-v0.0.86`。
- 调用链确认：`FetchDailyBars` 在 QFQ/HFQ 时先调用 `protocol.ApplyQFQ/ApplyHFQ`，再对调整后的 `protocol.Kline` 调用 `pytdxDailyBarExactValues`。因此 ExactValues.Open/High/Low/Close 对应的是库输出的调整后价格，不是原始 Kline 词法值。
- 精确版本 `v0.0.86` 的 `protocol/model_gbbq.go` 中，`Factor.QFQPrice/HFQPrice` 使用 float64 仿射因子；`roundHalfUpYuan` 先执行 `math.Round(yuan*100)`，再乘 10 写入 `Price`，即算法结果按分（2 位小数）舍入，随后存入厘单位（ProtocolScale=3）的 Price 整数。
- 当前 `pytdxDailyBarExactValues` 对调整后 Price 的 Int64 记录 `protocol_fixed_scale=3`。该字段只能说明 Price 的协议整数表示单位为厘，不能被解释为 QFQ/HFQ 算法输出保留了 3 位有效小数，也不能当作供应商原始 token 精度。v2 必须把协议表示 scale 与调整算法输出 scale/舍入规则分开表达。
- 当前 Pytdx DailyBar 已写入 `AdjustmentMethod=pytdx_qfq/pytdx_hfq` 和 reference date；但没有显式填充 `AdjustmentAlgorithmVersion`。SourceVersion 包含依赖版本，但不能在 v2 DTO 中默认为它等价于完整算法版本/舍入契约。
- 现有 `pytdx_daily_decimal_test.go` 只直接测试 Price 定点值转换；`daily_bar_adjustment_contract_test.go` 验证 QFQ/HFQ 结果数值，但未覆盖调整后 ExactValues 的证据语义、算法输出 scale 和舍入方式。
- 本轮没有修改代码或测试标准。下一步在 DailyBarV2 DTO 字段语义明确后，增加调整后 Price 的回归用例，验证 raw 与 adjusted 两条路径的来源/scale/algorithm provenance 不混淆；在此之前不把 protocol_scale=3 作为复权结果精度承诺。


## 2026-11-23 D2：HFQ reference date 语义差异审查

- 当前服务门禁 `applyDailyBarAdjustmentMetadataQualityAt` 仅要求 forward series 有 reference date；backward 缺失时不自动降级。
- 该例外目前有明确代码/测试依据的只有 Tushare `tushare_adj_factor_hfq_v1`：HFQ 使用绝对累计 adj_factor，QFQ 才需要最新日期归一化参考；对应单测明确允许 HFQ 无日期。
- BaoStock、Hithink、TickFlow 的 backward Provider Method 也未填 reference date，但没有取得这些具体端点的固定锚点语义证据。不能把 Tushare 的例外推广到所有 Provider；当前 v1 行为暂不改动。
- D2 DTO/Schema 需要区分日期锚定、Provider 累积锚点和未知锚点；未知必须 degraded/not_comparable。下一步逐个核验缺 reference date 的端点和真实响应，再确定是否需要方法级质量门禁。


## 2026-11-23 D2：Tushare/BaoStock 复权锚点官方资料核验

- 已核验 Tushare 官方 `pro_bar` 说明：QFQ = close × 当日 adj_factor / end_date 最新 adj_factor；HFQ = close × 当日 adj_factor。当前 `applyTushareAdjustment` 的 forward/referenceDate 与 backward/无 referenceDate 逻辑和该公式一致。Tushare `adj_factor` 文档仅给出盘前入库时间窗口 9:15–9:20，不足以生成精确 AvailableAt。
- 已核验 BaoStock 官方知识库：复权因子只在除权日记录；backAdjustFactor 的固定锚点是上市首日（首日因子 1），foreAdjustFactor 的锚点是最近一次除权除息日后的交易日。BaoStock HFQ 无显式 reference_date 有官方依据；QFQ 需按其 latest-ex-date 语义描述，不得用 end_date 代替。
- Hithink 默认 endpoint 文档只列 none/forward/backward，未说明 anchor；返回顶层 timestamp 是数据就绪时间，不是逐 Bar 时间。TickFlow 官方 SDK 暴露 ex_factors endpoint，但公开说明未定义 factor baseline。Eastmoney 未找到可审计官方复权公式。
- 因此将“仅 Tushare 有证据”的旧结论修正为 Tushare + BaoStock；Hithink/TickFlow/Eastmoney 保持 anchor unknown。当前仍不改 v1 质量门禁，待真实样本复算后再决定方法级规则。


## RepairAll D2 当前阻塞更新（2026-11-23）

1. **复权锚点门禁已修复**：只允许有来源依据的 Tushare/BaoStock 后复权无日期锚点；其他未知方法降级。代码和缓存测试通过。
2. **Hithink 后复权不是可直接消费的 v1 日线**：真实 API 返回小数成交量，Provider 适配器不将其伪装为整数，Normalizer 按旧契约丢弃。应使用同源未复权日线 + Data 自有复权因子重建 OHLC，volume_shares 保留真实整数成交量；当前 Hithink 公司行为接口尚未接入生产 Provider。
3. **Decimal 复权仍未完成**：AdjustmentFactor/CorporateAction 输入字段为 float64，复权重建使用 float64；需要在现有模型/Repository 上补 exact source token/evidence，再实现 Decimal 仿射计算。
4. **真实 Provider 阻塞**：Tushare adj_factor/dividend 权限码 40203；Eastmoney 本轮 EOF/502；Hithink 两个不同 end date 的锚点对照未执行。不得把这些标为代码 PASS。
5. **验收**：代码 SHA `e8d1c5464bd78dc83d6a9f51df41ea2d86a7fbba` 的 Go Test Suite 与 Integration CI 均通过；Tagged Live Provider Probe 仅部分通过。


## 2026-11-23 DATA D2 与 QNT-014 依赖顺序校准

- 最新执行顺序已在 RepairAll 主计划第 43 节校准：QNT-013 的策略管理/撤销锁等独立子项已完成，但 Q-01 完整闭环需要真实 Quant 结果生产者；QNT-014 又依赖 Data 精确日线与 Snapshot/Authorization 输入。不能继续坚持“Q-01 全部关闭后才允许任何 Data D2 工作”，否则形成生产依赖循环。
- Data 当前先负责首个 Quant 指标必需的最小输入切片：优先审查可验证的日线 OHLC Decimal 原始来源/scale、trade_date、interval、adjustment_basis（首个用例可限定 none，前提是调用契约明确）、同次质量评估的 CalendarRef、Payload 内容指纹及 SnapshotRef/Manifest/AuthorizationRef 绑定。
- 这不是启动完整 Provider D2，也不要求本轮完成 Amount/Volume 单位、所有 Provider 的 QFQ/HFQ、Factor 或所有 DailyBars 字段。没有权威来源的字段继续 null/degraded/not_comparable，不格式化 float64 冒充 Decimal。
- DATA-CTR-008 Snapshot Manifest 1.0.0 Schema/Golden 已存在，必须复用；尚缺 Data 生产侧把 DailyBars Payload 内容、Manifest 身份、可信 caller/purpose 和 GOV-CTR-006 AuthorizationRef 绑定的实现/消费者验收。DATA-CTR-005/007 仍按项目登记簿 deferred，未获正式重新进入/冻结前不发布正式跨模块契约。
- 本轮仅校准卡点与范围，不改生产代码、不调整验收标准。后续先审查真实 Provider 的 raw Close 来源和精度证据，确定首个指标可用输入路径，再实施有证据支持的最小 DTO/Manifest 生产链。


## 2026-11-23 D2：DailyBars Snapshot 存储审计与首个 OHLC 输入证据

- 本轮基线：repair_all `2dc294d48dc57ba186997ea8452c0202a08eaad8`。只读检查现有 Provider、Service、Application Reader、Repository、Migration 和 DATA-CTR-008/GOV-CTR-006 文件；未修改生产代码或冻结契约。
- **单 Bar 精确版本已存在**：`market_data_daily_bar_versions.metadata` 通过 `marshalDailyBarMetadata` 保存 ExactValues；`dailyBarRevisionHash` 将 ExactValues、来源、复权、质量和其他日线元数据纳入修订哈希，仅排除 fetched_at、normalized_at、system_observed_at。current/PIT 读取可恢复 sidecar。
- **单 Bar 版本不等于查询 Snapshot**：current 表可更新；PIT 表按每个来源/日期读取最新修订，再由 Service 选择来源一致序列。它们没有一次完整查询结果的内容寻址入口，也没有绑定该次 CalendarRef、CoverageEvidence 与 Manifest。因此不能直接把某个 Bar revision_hash 当作 SnapshotRef。
- **已有可复用实现**：Migration V5 的交易日历 Snapshot Repository 已实现内容寻址、幂等保存、精确引用读取、重新计算摘要和内容篡改拒绝。日线 Snapshot 应沿用该模式，只新增整次查询结果级的不可变存储，不复制逐 Bar 历史数据。
- **首个 SMA 输入可收窄**：默认 Provider Priority 首位 Eastmoney；其 OHLC ExactValues 从原始 CSV 字段字符串构造，CloseEvidence 保留 observed token 与 source fraction digits。首个指标限定 `adjustment_basis=none`，仅使用精确 Close 字符串；float64 字段不参与 Snapshot 内容数值来源。BaoStock、Tushare、Netease、TickFlow 也存在原始字符串/JSON token 的精确值路径；Pytdx 使用协议整数与 ProtocolScale，必须保留其协议定点语义。
- **来源版本边界**：当前 Provider SourceVersion 是 Adapter/API 版本常量，不是逐 Bar 上游数据版本；默认 Eastmoney 日线没有逐 Bar ProviderTime/provider_data_version。既有解阻方案明确 Research Evidence 的 `source_version` 只填可证明的 Provider 数据版本，缺失时显式缺失，不能用 Adapter 常量或 FetchedAt 替代。DATA-CTR-008 Manifest 1.0.0 的 `sources[].source_version` 却要求非空字符串，旧 Golden 又使用时间戳；这是当前来源数据无法满足的契约输入缺口。不能伪造填充，也不能修改冻结的 1.0.0，需经契约治理明确兼容路径后才能签发正式 Manifest。
- **SnapshotRef 计算边界**：PayloadFingerprint = SHA-256(JCS(canonical Payload))；ManifestFingerprint 独立计算；SnapshotRef 必须绑定两者，对 `manifest_fingerprint + payload_fingerprint` 的规范化组合计算 SHA-256。旧 Golden 中两个 fingerprint 相等不能作为 Payload 已绑定的证据；具体新语义应由 DATA-CTR-007 新版契约与 Golden 定义，不修改冻结的 DATA-CTR-008 1.0.0。
- 当前下一步：明确来源版本缺口的契约治理路径；Snapshot Payload 只包含首个指标必需的精确 Close/OHLC、交易日/周期/复权、Adapter/Normalization 版本、可选 Provider 数据版本、采集/系统观察时点、质量及同次 CalendarRef/CoverageEvidence。之后复用 V5 Repository 模式实现整次 Snapshot 存储、Manifest 绑定和 GOV-CTR-006 Owner 授权。DATA-CTR-005/007 继续 deferred，不提前发布正式跨模块契约。


## 2026-11-23：DailyBars 精确存储、来源版本与 Snapshot 的权威审计结论

本节与本文件此前 D1/D2 记录合并阅读；若旧段落将当前 ExactValues 描述为缺失，以本节最新代码审计为准。

- **精确日线版本**：Repository 将 `DailyBarExactValues` 保存在 `market_data_daily_bar_versions.metadata`；`dailyBarRevisionHash` 纳入 ExactValues、来源、复权和质量等元数据，current/PIT 读回可恢复。无需新增逐 Bar 历史表。
- **查询级 Snapshot 仍缺失**：当前表与 PIT 读取只能重建单 Bar 历史序列，不会固定一次查询的完整 Bars + CoverageEvidence + 同次 CalendarRef + Manifest。不能把单 Bar revision_hash 当 SnapshotRef。
- **日历能力已存在**：Migration V5 的不可变 Calendar Snapshot Repository 已完成内容寻址、幂等保存、精确读取和摘要校验；DailyBars Snapshot 复用，不再重建日历。
- **首个 SMA 精确输入**：Eastmoney 默认优先级第一，Close ExactValues 保留原始 CSV token 和 source fraction digits。首个输入限定 `adjustment_basis=none`，消费精确 Close，不允许 float64 回转 Decimal。Pytdx 的整数/ProtocolScale 是协议定点表示，不能与原始 token precision 混为一谈。
- **来源版本冲突**：DEC-PLAT-002-020 要求 Evidence version 是可获得的 Provider 数据版本；缺失则显式缺失。当前 Provider SourceVersion 实际为 Adapter/API/协议版本，Eastmoney 无逐 Bar ProviderTime/provider_data_version。DATA-CTR-008 1.0.0 却要求 `sources[].source_version` 非空字符串，Golden 还用时间戳填充。此处是冻结契约与真实来源能力的冲突，不是 Data 内部缺一个 fallback。不得伪造值，不得原地改 1.0.0。
- **Snapshot 指纹**：PayloadFingerprint 和 ManifestFingerprint 独立计算；SnapshotRef 必须绑定二者规范化组合。旧 Golden 中相等的两项指纹不能证明 Payload 被哈希。
- **不再复查的代码范围**：后续执行可直接复用上述结论；只有相关代码、Schema、Migration 或 Provider 版本语义发生变化时才重新核对。
- **下一步**：正式确定 DATA-CTR-008 新版本如何表达缺失 Provider 数据版本，并与 DATA-CTR-007 SnapshotRef/Manifest/Payload 新版定义一起做消费者影响评估；契约版本未明确前不进入正式 Snapshot 写入和 Quant 生产读取。


## 2026-11-23 来源版本契约冲突裁决（DEC-PLAT-002-055）

- 正式语义：Manifest `sources[].source_version` 只表示可验证的 Provider 上游数据版本；不存在时显式 null，不得以 Adapter/API/协议版本、FetchedAt、SystemObservedAt、revision hash 或 sentinel 填充。
- DATA-CTR-008 1.0.0 冻结不变；新生产链路采用 2.0.0，显式 `schema_version="2.0.0"`，`source_version` required+nullable，`provider_interface_version` 为独立可选字段。现有 `NormalizationVersion`（当前值 `4`）必须保留在每条 Bar Payload 并参与指纹。
- ManifestFingerprint 覆盖 Schema 版本和来源字段；SnapshotRef 必须由 DATA-CTR-007 定义为 ManifestFingerprint 与 PayloadFingerprint 的绑定引用。
- GOV-CTR-006 resource_ref 继续绑定最终 SnapshotRef；授权主体和读取时当前策略复核不变。
- 本决议只确定语义，不代表新 Schema/Golden、Data Owner 实现或消费者支持已完成。下一步发布 DATA-CTR-008 2.0.0 与 DATA-CTR-007 机器契约及 Golden，然后实现 Snapshot Repository。


## 2026-11-23 DATA-CTR-007/008 Schema 草案与 Golden 缺口

- 新增 DATA-CTR-008 Manifest 2.0.0 Schema 草案：schema_version 必需；sources[].source_version required+nullable；provider_interface_version 可选并与 Provider 数据版本分离。
- 新增 DATA-CTR-007 daily_bars_close_series Payload 1.0.0 Schema 草案：仅覆盖首批 SMA 需要的未复权日线精确 Close；包含同次 CalendarRef、CoverageEvidence、逐 Bar Decimal Evidence 和 normalization_version。
- Data normalize.Version 当前为 4，由 DailyBar/Application DTO 传递；Payload 必须保留该版本并纳入内容指纹。
- 两份 Schema 目前只完成 JSON 语法解析，尚无 Golden、Schema Validator、消费者 Contract Test；DATA-CTR-005/007 仍 deferred，DATA-CTR-008 现行 1.0.0 仍 frozen，2.0.0 不得标记 frozen。
- 历史 Live Provider Artifact 14611734848 仅有汇总结果，不含精确日线 token 和同次日历证据，不能作为 Snapshot Golden。当前不重新触发 Provider 探测；先检查 MariaDB 历史修订/已有 Artifact 能否恢复同一查询批次的全部字段。


## 2026-11-23：Golden 生成不需要重新探测 Provider

- 已确认 DailyBarsAsOfWithCoverageEvidence 先从 Repository ListBarsAsOf 读取历史修订；未复权 none 路径不会调用 AdjustmentFactorsAsOf/外部 Provider。
- 同一函数随后调用 dailyBarCoverageEvidenceAsOf，在同一 as_of 下读取已持久化 Calendar Snapshot，返回 CalendarRef 与 CoverageEvidence。Application Reader.GetDailyBarsV2 会原样映射该次评估结果和 ExactValues。
- 若现有 MariaDB 有完整历史记录，可直接通过 PIT V2 Reader 生成真实 Payload Golden，不消耗 Provider 配额，也不重复 Live Probe。当前可访问的 Artifact 14611734848 只有聚合摘要，没有完整 Bar/Calendar 数据。
- 当前助手执行环境无法直接访问用户部署的 MariaDB；下一步应在现有 Server/应用调用路径暴露一次受控的历史 Snapshot 物化任务，或使用已连接的数据库运行环境，不能用 synthetic CI fixture 伪装真实 Golden。


### 2026-11-20 历史 Workflow 日志原始样本核对（不触发新请求）

- Live Provider Run 44699296891 / Job 136163491198 的日志确实包含 Eastmoney DailyBars 样本，但只有 forward_adjusted 与 backward_adjusted 两组；没有 none 未复权样本。
- 日志中的 sample_bars 是 float64 投影，虽然 HTTP body_sample 含部分原始 CSV 行，但响应采样被截断，且这些请求为复权口径；不具备本次首批 none SMA Payload 所需的完整 ExactValues + 原始 Close token + 同次 CalendarRef/CoverageEvidence。
- 因此该日志可证明 Eastmoney 对应复权请求曾真实返回，但不能作为本次 Snapshot Golden。不要从这些浮点样本反推 Decimal，也不要把复权样本改写为 none。


## 2026-11-23 Data Close Series Snapshot Builder 实现进度

- 新增 internal/marketdata/app/daily_bars_close_series.go：BuildDailyBarsCloseSeriesInput 将同一次 DailyBarsV2Reader PIT 结果映射为 DATA-CTR-007 首批 Payload，并同时产出 Manifest sources 输入和 SystemAsOf。
- 首批实现明确只支持 Eastmoney 未复权 1d：必须有显式日期范围、正数 Limit、显式 SystemObservedAtCutoff、同次 CalendarRef、valid Exchange Calendar、coverage_sufficient=true；不允许用 float64 回填 ExactValues。
- 每条 Bar 必须有精确 Close、observed-token Evidence、SourceFractionDigits、NormalizationVersion、FetchedAt/NormalizedAt/SystemObservedAt；Close 与原始 SourceToken 用精确有理数比较，SourceFractionDigits 必须与 token 小数位一致。
- 当前 Eastmoney SourceVersion 已核实为 eastmoney-stock-api-v1，映射到 provider_interface_version；Manifest source_version 明确为 nil，不伪造 Provider 数据版本。normalization_version 从 Data DTO 读取，当前 normalize.Version 为 4。
- 新增 daily_bars_close_series_test.go，覆盖正确映射、JSON 字段名、缺 CalendarRef、日历降级、覆盖不足、float-only、来源版本不匹配、token 不一致、非 PIT、重复交易日和空诊断字段拒绝。测试代码已写入，但最新 Go Test Suite 在 gofmt 门禁处提前失败，未执行该测试；本轮最后的格式修正提交使用 skip-ci，故最终 HEAD 尚无通过 CI 的证据，不能宣称测试通过。
- 本轮 push 触发的 Live provider probe workflow 中，实际 Provider 探测步骤全部 skipped（没有包含 [include_public] 标记，也未手动 dispatch），未重新请求 Provider。
- 当前尚未实现 Manifest/Payload JCS 指纹、SnapshotRef、Migration V6、不可变 Snapshot Repository、Data Owner 授权签发/撤销复核；Builder 只是输入构建层，不对外开放 Snapshot 读取。

### 下一步

1. 先通过真实 Data PIT V2 Reader 获取一份既有 MariaDB 历史日线的完整输出（不访问 Provider），确认 Builder 能从真实 Eastmoney ExactValues/CalendarRef/CoverageEvidence 生成 Payload。
2. 使用项目 JCS 库分别计算 PayloadFingerprint、ManifestFingerprint，再按 DEC-PLAT-002-055 对两者规范化组合计算 SnapshotRef；补 Golden 和契约校验。
3. 再新增 Migration V6 和不可变 Snapshot Repository，随后接 Data Owner 授权和 Quant Consumer。


## 2026-11-23 Snapshot 双指纹构建与校验实现

- 新增 internal/marketdata/app/daily_bars_close_series_manifest.go。
- PayloadFingerprint：对 Payload JSON 执行 RFC 8785 JCS 后 SHA-256。
- ManifestFingerprint：对 DATA-CTR-008 2.0.0 Manifest 指纹输入（不含 snapshot_ref / manifest_fingerprint）执行 JCS + SHA-256。
- SnapshotRef：对规范化对象 {manifest_fingerprint, payload_fingerprint} 执行 JCS + SHA-256，并加 data_snapshot:sha256: 前缀；不将两份指纹简单拼接，也不将 ManifestFingerprint 单独作为 SnapshotRef。
- Build/Verify 同时检查 schema 版本、purpose、caller_ref、来源绑定、PIT 时间边界、Calendar/coverage、Exact Close Evidence 和三份指纹；返回值对 Payload/来源指针做深拷贝，避免调用方修改原输入后改变已构建 Snapshot。
- 新增 Manifest 指纹测试：Manifest 改变时 PayloadFingerprint 保持不变但 SnapshotRef 改变；Payload 改变时 ManifestFingerprint 保持不变但 SnapshotRef 改变；同时覆盖篡改、PIT as_of、调用主体和用途拒绝。
- 当前仍未实现 Migration V6、不可变 Snapshot Repository、Data Owner 授权签发/撤销复核、Server 注入和 Quant 消费者；真实 MariaDB PIT Payload Golden 也未生成。因此此阶段只是 Snapshot 计算/校验层，不宣称生产闭环完成。


## 2026-11-23 Snapshot 生产服务启动：首批 SMA 质量准入门禁

- 在 `repair_all` HEAD `958f595254baafb04602336464cdc6e1a73e0a06` 上确认：Close Series Builder 原先只校验 QualityStatus 是否属于合法枚举，未限制正式 SMA 输入必须达到 `valid`；Payload Verify 存在同样宽松边界。
- 已将首批 `daily_bars_close_series` Payload 的聚合质量和逐 Bar 质量统一收紧为 `valid`。`degraded`、`stale`、`invalid`、`unknown` 均拒绝进入首批正式 SMA Snapshot；不改变 Data 通用日线读取对降级数据的既有处理。
- DATA-CTR-007 Payload Schema 同步收紧为 `quality_status: const valid`，使 Builder、Verify 和机器契约一致。
- 增加 Builder 对聚合及逐 Bar 四类非 valid 状态的拒绝测试，并增加已构建 Snapshot 被改成 degraded 后 Verify 拒绝的测试。
- 本项是 Snapshot 生产服务的第一项代码修复；尚未实现 Snapshot Repository、Migration V6、Data Owner 授权或 Server Snapshot Port。Schema/Golden 的真实数据一致性与消费者契约验收仍待完成，不将现有草案视为冻结契约。


## 2026-11-23 Snapshot Schema 自动一致性测试

- 新增 Data Application 层契约测试：使用现有 Builder 构造的结构样例，分别编译 DATA-CTR-007 Payload 1.0.0 与 DATA-CTR-008 Manifest 2.0.0 Schema，验证 Go 输出符合 Schema。
- 增加 Schema 负向断言：聚合质量与逐 Bar 质量为 degraded/stale/invalid/unknown 时均不能通过 DATA-CTR-007 Schema。
- 该测试只证明 Go DTO 与 Schema 的结构一致性，不是生产数据 Golden；真实 Golden 仍必须由 MariaDB PIT V2 Reader 的实际历史数据生成，不能用单元测试样例代替。


## 2026-11-23 Snapshot V6 不可变 Repository

- 新增 Data Application Repository Port：`DailyBarsCloseSeriesSnapshotRepository`，仅定义按 SnapshotRef 保存和精确读取，不向其他模块暴露底层 SQL Store。
- 新增 Migration V6：`market_data_daily_bars_close_series_snapshots`，以 `snapshot_ref` 为唯一主键，保存 Manifest/Payload 的规范化 JSON、两份独立指纹、各自 Schema 版本和持久化时间；不复制逐 Bar 历史数据。
- 新增 MariaDB Repository：写入前执行 Snapshot Verify；按 SnapshotRef 幂等写入，不覆盖既有记录；重复写入后读取回验；读取时重新解析 JSON、验证双指纹和组合 SnapshotRef，并要求存储 JSON 为 JCS 规范形式。
- 未配置 MariaDB 时 Save/Get 明确返回错误，不允许空操作成功；新增 V6 物理 Schema 检查。
- 增加单元/集成测试覆盖存储未配置、引用格式、幂等、并发重复写、损坏数据拒绝和 Migration V6 结构。真实 MariaDB 集成测试由 CI 的 Data MariaDB job 执行。
- 当前只完成不可变 Snapshot 存储，不代表授权生产链路已完成；Data Owner 授权、撤销/过期读取复核、Server Principal 注入及 Snapshot Application 生产入口仍未接入。


## 2026-11-23 Data Owner 策略版本存储

- 新增 Data Owner 专属授权策略模型，固定首批策略身份 `data_access / dailybars-v2-access`，策略状态支持 approved/restricted/pending/revoked；策略必须显式列出允许主体、用途和所需 Scope，空集合拒绝，不存在默认 allow。
- 新增 Migration V7：不可变 `market_data_authorization_policy_versions` 与 `market_data_current_authorization_policy_versions`。策略版本写入与当前指针切换在同一 MariaDB 事务内完成；相同版本不同载荷冲突拒绝；当前版本读取会复算 payload SHA-256 并重新校验策略内容。
- 当前没有自动写入任何默认策略。生产授权在显式发布符合策略的版本前保持 fail closed。授权快照签发、撤销引用和原子读取门禁仍待后续阶段。
## 2026-11-23 Data AuthorizationRef 不可变记录与撤销存储

- 新增 Migration V8：`market_data_authorization_decisions` 保存完整 GOV-CTR-006 AuthorizationRef 与对应 Policy Snapshot；`market_data_authorization_revocations` 独立追加撤销状态，通过外键绑定原授权记录，不修改历史授权原件。
- 新增 Repository 操作：`AppendDataAuthorizationDecision`、`GetDataAuthorizationDecision`、`RevokeDataAuthorizationDecision`。授权写入幂等且内容冲突拒绝；读取时重新验证 GOV-CTR-006 决策指纹、规范 JSON、记录哈希及撤销状态；撤销写入幂等，重复撤销内容不一致时拒绝。
- 决策持久化仅接受 Data Owner、allowed、full、首批三种用途、明确主体和 `data_snapshot:sha256:` 资源引用；不接收 denied/conditional 作为可消费授权。
- V8 表结构已加入 Data 物理 Schema 检查。新增负向单测覆盖空授权、非 allowed 决策和不允许用途。
- 这一步只完成授权记录与撤销的持久化边界，不代表授权签发器或生产读取入口已完成。调用方仍必须先执行当前策略评估；读取时还需检查当前策略版本、有效期、主体/资源/用途匹配，并在同一授权边界内读取 Snapshot Payload。
- 当前 Root Server 尚未启用 Service Credentials；不能将 `researchflow` / `quant` 字符串直接作为可信 Principal。服务身份注入需待真实配置完成，Owner Session 的策略映射也必须以实际 Principal Scope 语义为依据。


## 2026-11-23 Data Owner 授权 Application 首版实现

- 新增 `internal/marketdata/application/data_authorization.go`，形成 Data Owner 单一授权签发/读取判定边界；策略存储与授权决策存储通过 Data Application Port 注入。
- Principal 仅从 `security.PrincipalFromContextValue` 获取，不接收 HTTP 参数、caller_ref 或调用方自报 scopes。主体映射固定由可信 Principal 生成：Owner → `principal:owner`；Service → `principal:service:<credential subject>`。匿名主体、未知身份和缺失 Principal 一律拒绝。
- 签发时读取当前 Data Owner 策略，执行状态、有效期、允许主体、允许用途和 RequiredScopes 检查；策略缺失、存储异常、权限不满足均 fail closed。
- AuthorizationRef 按 GOV-CTR-006 1.0.0 结构生成；Policy Snapshot 绑定当前策略版本；使用 Platform `authorization.Fingerprint` 计算决策指纹；再写入 V8 不可变决策存储。
- 读取判定先由 V8 Repository 校验决策指纹、记录哈希和撤销状态，再校验当前 Principal、资源 SnapshotRef、purpose、full content level、有效期及当前策略版本，并重新执行策略评估。
- 新增 Application 单测覆盖正常签发/读取、purpose 不匹配、缺失 Principal、策略存储不可用、策略版本变化和撤销拒绝。
- 为统一 Application Port，授权决策 DTO 与 Store 接口放在 `internal/marketdata/app`，Repository 类型使用该 DTO；没有把 SQL Repository 暴露给跨模块消费者。
- 当前实现仍不是完整生产 Snapshot 闭环：Snapshot Creator 尚未把 PIT V2 Reader → Builder → V6 Repository → Issue 串起来；Authorized Snapshot Reader 尚未把授权检查与 Payload 读取放在同一数据库事务/一致性边界；Server 尚未注入该 Owner Application。Service Credentials 仍须使用真实配置和真实 Principal，不得伪造服务身份。
- V8 当前策略变更采用严格拒绝旧授权（授权策略版本必须等于当前版本）；这是 fail-closed 行为。若未来需要策略版本兼容或授权续期，必须作为明确策略语义单独设计，不能静默放宽。

### 下一步执行

1. 完成授权 Application 单测和 Go/Integration CI。
2. 实现 Snapshot Creator 与 Authorized Snapshot Reader；授权检查和 Snapshot Payload 读取必须在同一事务内完成，确保撤销与读取之间没有竞态。
3. 在 Root Server composition root 装配策略 Repository、决策 Repository、可信 Principal resolver 和 Snapshot Application；当前无服务凭据时保持拒绝，不注入默认服务主体。
4. 再接 Quant 首批 SMA Reader，不提前引入多指标或策略扩展。


## 2026-11-23 Snapshot 生产与授权读取链路接入

- 新增 `DailyBarsCloseSeriesSnapshotCreator`：可信 Principal → PIT `DailyBarsV2Reader` → Close Series Builder → Manifest/Payload 双指纹 Snapshot → V6 Repository → Data Owner Issue。
- Snapshot 创建接口只返回 `SnapshotRef + AuthorizationRef`，不返回 Payload。授权签发失败时 Snapshot 可以作为不可变历史记录保留，但不会返回可消费引用。
- 新增 `AuthorizedDailyBarsCloseSeriesSnapshotReader`：只有 Repository 返回授权决策、撤销状态、当前策略和 Snapshot 后，Application 才会检查主体、资源、用途、content_level、授权期限、当前策略版本及 RequiredScopes；通过后才返回 Payload。
- 新增 Repository `ReadDataAuthorizedDailyBarsCloseSeriesSnapshot`，将授权记录、撤销状态、当前策略版本/内容、Snapshot Manifest/Payload 的读取放在同一 MariaDB Repeatable Read 只读事务内。避免先单独检查撤销、再单独读取 Payload 所产生的竞态；撤销/策略更新与读取按数据库事务快照确定先后。
- Data Module 新增 `CreateDailyBarsCloseSeriesSnapshot` 和 `ReadAuthorizedDailyBarsCloseSeriesSnapshot` 模块入口，复用 Root 注入的同一个 Store，不创建平行 Repository 或独立连接池。
- 目前该模块入口已具备调用路径，但 Root Server 尚未将它注入 Quant/Researchflow 的生产消费者；服务凭据配置也尚未完成。没有真实 service Principal 时仍 fail closed。
- CI 当前针对前序提交有过取消/格式门禁失败；格式修正后最新提交的 Go Test Suite、Integration CI 尚待完成。不得将尚未通过的提交描述为验收通过。


## 2026-11-23 Data Snapshot 与 Owner 策略发布阶段收口

- 最新代码基线：`repair_all` SHA `cdcd5f905de1103e49e1b79686f00882e2ea405d`。
- Snapshot 生产路径已具备：PIT DailyBarsV2 Reader → Close Series Builder → Manifest/Payload 双指纹 → V6 不可变 MariaDB Repository → Data Owner 授权签发；读取路径在同一 MariaDB Repeatable Read 事务内校验授权记录、撤销、当前策略和 Snapshot Payload。
- Data Owner 策略发布 Application 已完成：仅接受可信 Owner Session Principal；策略版本不可变；current pointer 采用 expected-current-version CAS；首次发布必须明确 expected 为空；不创建默认 allow；历史策略版本 ID 禁止再次激活。
- 本轮修复 Publish 成功后重新读取 current pointer 的并发误报问题：现在直接返回本次已持久化的不可变版本，并通过 current pointer 并发前移回归测试。
- 精确 SHA 的 Go Test Suite [45174381526](https://github.com/Kasign/stock_data_provider/actions/runs/45174381526) 与 Integration CI [45174381536](https://github.com/Kasign/stock_data_provider/actions/runs/45174381536) 均 success；Data MariaDB Migration/PIT/Snapshot/授权存储集成和完整跨模块集成均通过。
- `PLATFORM_SERVICE_CREDENTIALS` 已进入 Root Server 配置并传给 Platform Authenticator；这只完成 Service Bearer 身份认证配置入口，不代表 Server 已为 Quant 生产调用注入内部可信 Principal，也不代表 Quant 已消费 Data Snapshot。
- 尚未完成：Data Owner Policy 管理 HTTP API 契约/Handler/Root 注册；Root → Quant 的可信 Service Principal 与 Data Snapshot Reader 注入；Quant SMA 正式计算 Facade/结果生产；Researchflow Quant Stage 与 Run→QuantResult 精确关联。
- DATA-CTR-007 仍按项目登记簿保持 deferred：当前实现与 Schema 已有代码基础，但真实 MariaDB 历史数据 Golden、消费者版本/引用映射及契约重新进入冻结条件尚未完成。不得把 CI 的结构集成测试冒充真实数据 Golden。
- 本轮 Live Provider Probe Run [45174381495](https://github.com/Kasign/stock_data_provider/actions/runs/45174381495) 中全部真实 Provider 请求步骤均 skipped；真实行情数据准确性留待部署后按用户安排验证。
- 下一步先定义最小 Data Owner 策略管理 HTTP 契约（Owner Session only、读取当前策略、基于 expected_current_version 发布不可变版本；撤销通过发布 revoked 状态的新版本，不提供删除/回滚旧版本），再由 Server 组合 Data Owner Application。契约未冻结前不增加公开路由。


## 2026-11-23 Data Owner 策略发布与未发布状态语义补齐

- Data Owner Policy Publisher 已完成 Owner Session Principal 校验、策略内容校验、不可变版本发布、expected-current-version CAS、历史版本 ID 永不复用，以及发布后直接返回本次版本的并发一致性修复。
- `Current()` 现在能区分“尚无已发布策略”与“数据库/存储不可用”：Repository 的 `sql.ErrNoRows` 映射为 `app.ErrDataAuthorizationPolicyNotPublished`，Application 映射为 `ErrDataPolicyNotPublished`。未发布仍不生成默认策略，存储故障仍 fail closed。
- 新增 Application 未发布状态测试与 MariaDB 首次发布前的查询断言；精确 SHA `50b6eba3e3c2f85cad593a9680f6424b536a4054` 的 Go Test Suite [45176291646](https://github.com/Kasign/stock_data_provider/actions/runs/45176291646)、Integration CI [45176291723](https://github.com/Kasign/stock_data_provider/actions/runs/45176291723) 均 success。
- 下一步仍是确认并冻结 `doc/server/contracts/Data-Owner策略管理HTTP接口提案.md`，之后再接 Server Owner Session composition/Handler/Root Router；未冻结前不创建公开路由。


## 2026-11-23 Data Owner 策略发布审计与状态模型收敛

- Data Policy 首版只保留 approved / revoked。此前 restricted 与 approved 在授权 Evaluator 中没有不同的判定行为，pending 没有审批状态机；继续保留会误导策略操作人员。未来若需要审批或细粒度限制，必须有独立行为定义并升级 Policy Schema。
- Data Migration V9 新增 market_data_authorization_policy_publications，记录策略资源、前后版本、Payload Hash、可信 Owner Principal、Platform Request ID、发生时间。
- 发布审计与 immutable version insert、current pointer CAS 同一 MariaDB 事务提交；审计写入失败时策略发布整体回滚。未发布仍不生成默认策略。
- Data Owner Application 从已验证的 Owner Session Principal 取得 actor_ref，从 Platform observability Context 取得 Request ID；缺失 Request ID 时 fail closed。
- 接口评审文档为 doc/server/contracts/Data-Owner策略管理HTTP接口提案.md，推荐 POST /versions 创建不可变版本。文档仍 DRAFT，待项目级契约登记与冻结；Server Handler 尚未开始实现。

## 2026-11-26 交易状态事实快照接入（DEC-060 契约层 + DEC-061 物理链路）

- 契约层（DEC-060）：新增 payload `security_trading_status_facts` 1.0.0 与同族 Manifest 1.0.0，只承载五项真实事实（ST/停牌/退市/上市日/交易日），逐项 `available`/`unavailable`；五项全可用→`valid`，任一不可用→`degraded`。**不含涨跌停与可成交量**——Provider 从未提供，写入即等于把推导伪装为事实。
- 存储（迁移 V2）：共享不可变快照表新增 `payload_kind`（`VARCHAR(48)` ascii_bin 非空）。**刻意不改表名**：MySQL DDL 非事务，`RENAME` 失败会在 migrations 历史里留下 `status=failed` 记录并导致启动被拒绝，代价远大于表名不贴切。V1 未改一字，checksum 不变。
- 读写分流：写入显式给 kind（列默认值只服务历史行回填）；读取 `snapshot_ref` 必须带 kind 条件——`SnapshotRef` 前缀对所有 payload 类型相同，仅凭 ref 无法区分数据类，缺 kind 条件会导致跨类型串读。
- 授权分流：策略引用收敛为闭集（`dailybars-v2-access`、`security-status-facts-access`）。签发、读取授权、Owner 发布、HTTP 路由四处都校验"解析到的当前策略 `policy_ref` == 本次请求的数据类"，不匹配即 fail-closed；旧签名接口不回落日线策略。
- 新增 `SecurityTradingStatusFactsSnapshotCreator`：可信 Principal → PIT 交易状态 Reader → payload 组装 → V2 存储 → Data Owner 签发。组装只搬运不改判：源记录 `invalid`/`unknown` 拒绝，`degraded` 即使五项齐全也拒绝，枚举 `unknown` 一律落 `unavailable` 并给原因；`source_version` 缺失时留空指针而非伪造版本。
- 新增 `AuthorizedSecurityTradingStatusFactsSnapshotReader`：独立方法 + 独立 Port 接口（不扩既有 close series 签名），先校验 `policy_snapshot`/`current` 的 `policy_ref` 与 `Manifest.PurposeCode`，再走同一 `authorization.Evaluate`；授权记录、撤销、当前策略、Payload 仍在同一 Repeatable Read 只读事务内读取。
- 新增 Owner 策略路由 `GET/POST /api/v1/owner/data/security-status-facts-access-policy[/versions]`：策略按数据类独立发布、独立撤销；装配的 Application 若不支持按数据类读写，返回 503 `server.data_policy_unavailable`，不回落日线。
- 验证：`gofmt`/`go vet`/`go test ./internal/...` 全绿，`-race` 覆盖 `internal/marketdata/...`；MariaDB 集成测试（`MARKET_DATA_TEST_MYSQL_DSN` 门控）覆盖迁移/`ValidateSchema`/幂等/跨 kind 隔离/SQL 篡改检测，但**未在真实 MariaDB 环境执行**，不得宣称已验证。契约状态未变：DATA-CTR-007 仍 `deferred`、DATA-CTR-008 仍 `frozen`。

## 2026-11-27 R2 真实五档盘口事实通道（DATA-CTR-007 同族扩展）

- 新增 `OrderBookProvider` 与 Eastmoney push2 五档 Provider：买一～买五 `f19/f20 … f11/f12`、卖一～卖五 `f39/f40 … f31/f32`；手数经 `normalize.HandsToShares` 转股；缺 `f86` 来源时间、证券错配、不完整五档一律拒绝。成交量单位与字段映射均有变异验证。
- 配置能力 `order_book` 与受控出站 origin 接入；`DataService` 维护独立 provider 列表与 fallback；调用方不能传入盘口数值。
- 新增 `order_book` payload/Manifest/Snapshot：复用共享不可变快照表按 `payload_kind=order_book` 分流；`observed_at`（来源时刻）与 `system_as_of`（评估截止）分别绑定，JCS 双指纹与 SnapshotRef 防篡改；新增独立数据类策略 `order-book-facts-access` 与 Owner 策略路由，与 close-series / 交易状态策略互不替代。
- `tradable_volume` 的唯一口径是买盘五档挂单量之和；Provider、Creator、Repository、Authorized Reader 均不接受参与率、成交量推导或调用方声称值。
- 新增 MariaDB 盘口快照集成测试：共享存储、跨 `payload_kind` 隔离、幂等读回、直接改库篡改检测；本地 `MARKET_DATA_TEST_MYSQL_DSN` 未配置时 SKIP，CI 结论按精确 SHA 回填。契约状态未变：DATA-CTR-007 仍 `deferred`、DATA-CTR-008 仍 `frozen`。
- **精确复用补强（2026-11-27）**：`OrderBookFactsSnapshotRepository` 新增按精确 `symbol + purpose + system_as_of` 复用已授权盘口快照的窄方法，MariaDB 实现使用精确相等过滤；Creator 在实时 Provider 前复用，绑定不一致 fail-closed。该修复使 L3 不依赖实时盘口开放时段，且没有引入参与率或近似匹配。真库 `TestMariaDBOrderBookFactsSnapshotExactSystemAsOfReuse` PASS；修复提交 `1423d29c9` 的 Integration CI 45937832229 全项 success，含 L3 PASS 7.02s。

## 2026-11-28 证券目录单点：补 BaoStock 第二来源，解除 /market/stocks 全链路失败

**状态：代码完成，CI 待回填（`not_run`）；push2 阻断的其余独源能力未解决。**

- **触发**：Data 模块「数据获取不全」review 确认，`StockUniverse` 是全仓影响面最大的单点：`/api/v1/market/stocks`、选股筛选、自选的全市场增强都依赖它。部署态该能力恒返回 `no configured provider exposes a complete stock universe` / 503。
- **根因（两个叠加，都是代码结构问题）**：① `Eastmoney.ListStockUniverse` 取数走 `stockListURL = sectorRankingURL = push2 clist`，而当前网络对 `push2/push2his` 整段无 HTTP 响应；进程内 6 小时缓存无法跨重启，且 MariaDB 没有 securities 表。② 更关键的是注册路径：`DataService.StockUniverse` 只遍历 `StockSearchProvider` 列表并动态断言 `StockUniverseProvider`，因此「只有全量目录、没有关键词检索」的 Provider 永远无法接入——即使实现了也没用。
- **方案选择（为什么不先做持久化）**：持久化只能缓存「上一次成功抓到的目录」，而原路径在部署态从未成功抓到过，落表无内容可存；冷启动兜底同样需要先有一条可到达的取数路径。因此先补第二来源（`StockUniverseProvider` 的独立实现），不改物理 Schema、不新增表，符合「能复用就复用、不新增平行实现」的约束。BaoStock 走 `public-api.baostock.com:10030` 独立 TCP 端点，与 Eastmoney 行情域名不共享任何网络路径。
- **实现**：
  - 协议层 `baostock/protocol.go` 增 `query_all_stock` 请求/响应报文类型 35/36；`Client.QueryAllStocks` 负责分页（≤20 页）、错误码、响应日期与请求日期一致、跨页证券代码去重、`maxQueryRows` 安全上限、末页判定；`parseAllStockPage` 校验字段表 `code,tradeStatus,code_name` 与 `sh./sz./bj.` 代码前缀，品种裁剪交给上层。
  - `provider/baostock_universe.go` 新增 `Baostock.ListStockUniverse`：用 `QueryTradeDates` 取最近一个不晚于今日的交易日定档，按 A 股代码段过滤（沪 600/601/603/605/688/689，深 000/001/002/003/300/301/302），排除指数/B 股/场内基金与无名行；条目数 < 1000 失败关闭；快照固定 `degraded`、`total_is_lower_bound=true`，原因码 `provider_universe_excludes_beijing_exchange`；进程内缓存 6 小时，网络查询在锁内避免重复抓取。
  - `DataService` 新增独立 `SetStockUniverseProviders` 注册通道与 `stockUniverseCandidates()`：先取显式目录来源，再追加搜索来源中同时实现目录能力的 Provider，按名称去重、保持 `provider_priority` 顺序（eastmoney → baostock），逐源失败继续、全败才报错；原有完整性校验抽为 `validateStockUniverseSnapshot`。
  - `bootstrap/module.go` 与 `httpapi/server.go` 两条装配路径都按同一优先级收集 `StockUniverseProvider` 并注册；`doc/api/架构/01-Provider与数据域能力边界.md` 同步替换「目录是搜索能力附属」的旧口径。
- **测试（18 个新用例 + 2 处变异）**：`client_allstock_test.go` 6 个（字段解析、大写代码归一、空末页、契约违约拒绝、查询被拒错误码、真实端点 live 用例由环境变量门控默认跳过）；`baostock_universe_test.go` 7 个（非 A 股过滤、只保留有名 A 股、目录截断失败关闭、重复代码拒绝、按最近交易日取数且命中缓存、底层失败上抛、客户端不支持目录查询时拒绝）；`stock_universe_fallback_test.go` 5 个（首源失败回退第二源、只目录不搜索的 Provider 可用、同一 Provider 不重复查询、逐源失败原因全量上报、第二源快照不完整仍拒绝）。
  - 先红证据口径：本组未保留「实现未落地时」的测试运行快照，改以两处变异复现修复前行为并取得失败证据——① 去掉 A 股代码过滤，`TestBuildBaostockStockUniverse*` 失败；② 把兜底链退化为「首源失败即返回」（等价于旧的单源行为），`TestStockUniverse*` 失败。两处恢复后均通过，未删断言、未放宽校验、未伪造数据。
  - 真实数据验证：临时 live 探针（用完已删除）连真实 BaoStock 端点完成 `QueryTradeDates → QueryAllStocks → ListStockUniverse → validateStockUniverseSnapshot` 全链路，返回完整 A 股目录并通过完整性校验。
- **本地验证**：`gofmt -l internal/marketdata/` 空；`go build ./...`、`go vet ./internal/marketdata/...`、`go test ./... -count=1`（全仓 `ok`，无 FAIL/panic）、`git diff --check` 全部通过。
- **状态边界**：目录**仍不含北交所**，故固定 degraded 且声明下界，不能当作全市场完备目录使用；目录仍只做进程内缓存、未落 MariaDB（BaoStock 端点可用时重启后重抓即可，是否需要持久化取决于后续是否出现「两端点同时不可达」的场景）。Eastmoney `push2` 阻断下的 `sector_rankings`、`stock_boards`、`order_book`、`chip_distribution`、`capital_flow`，以及零源的 `index_catalog` / `market_capital_flow` / `security_trading_status` / `price_limit_event` 均不在本组范围，仍是已登记卡点。`writeDomainErr` 把免费档 40203/403/429 统一压成 502 的诊断盲点也仍未处理。
- **提交与 CI**：本组提交后按精确 SHA 回填；在取得 GitHub Actions `required-checks` 结论前，验收状态为 `not_run`，本地结果不冒充 CI。

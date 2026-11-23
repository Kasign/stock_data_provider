# Data 模块卡点

更新时间：2026-11-23  
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
- 统一责任与解锁顺序见 `doc/整体任务规划/05-治理与验收/RepairAll跨模块修复计划与进度.md` 第 28 节。


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
- DATA-CTR-008 1.0.0 冻结不变；新生产链路采用 2.0.0，显式 `schema_version="2.0.0"`，`source_version` required+nullable，`adapter_version` 为独立可选字段。未建立稳定机制的 normalization_version 不新增。
- ManifestFingerprint 覆盖 Schema 版本和来源字段；SnapshotRef 必须由 DATA-CTR-007 定义为 ManifestFingerprint 与 PayloadFingerprint 的绑定引用。
- GOV-CTR-006 resource_ref 继续绑定最终 SnapshotRef；授权主体和读取时当前策略复核不变。
- 本决议只确定语义，不代表新 Schema/Golden、Data Owner 实现或消费者支持已完成。下一步发布 DATA-CTR-008 2.0.0 与 DATA-CTR-007 机器契约及 Golden，然后实现 Snapshot Repository。

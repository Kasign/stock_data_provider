# PLAT-002 项目级契约冻结登记簿

- 仓库：`Kasign/stock_data_provider`
- 审查分支：`integration`
- 审查基线 HEAD：`bb8cf3495b9591ede2a29b554d9121b06b983f0b`
- 审查日期：2026-11-09
- 登记簿版本：`0.1.0-review`
- 当前状态：`review_in_progress`（尚未整体冻结）
- 决策责任：项目负责人
- 执行方式：各模块执行窗口提供本模块生产方/消费者核验及证据；集成窗口汇总

> 本文是项目级冻结结论与状态入口，不宣称所有契约已冻结。当前代码基线来自 integration；契约目录中已有的状态和证据快照主要形成于 2026-11-06，不能自动视为 2026-11-09 最新代码的验收结果。

## 1. 本轮结论

本轮可以确认项目已具备跨模块契约目录、模块职责划分、依赖方向和大量 DTO/Port/Schema 设计资料；但不能据此宣布 PLAT-002 整体冻结。

主要原因：

1. integration 中已有的项目级目录 `doc/整体任务规划/02-契约治理/跨模块契约目录与冻结状态.md` 仍声明其权威维护分支为 `feat/design_doc`，尚未完成权威维护权迁移决议。
2. 该目录记录的多项关键契约仍为 `pending_evidence`，例如 GOV-CTR-003、GOV-CTR-006、PLAT-CTR-001～007，以及若干 Data/News/Quant/Research/Researchflow/LLM/Notification/Integration 契约。
3. 多个模块 README 明确说明部分代码、Schema、Golden、消费者映射或 CI/真实环境证据尚未完成；模块自身实现或测试不能代替跨模块消费者验收。
4. Server README 明确定位 Server 为唯一 Composition Root，但实际依赖图、共享资源唯一性、启动失败回滚、Worker drain、Migration 汇总和生产生命周期仍需集成证据。
5. 本次只读取 integration 上的代表性项目/模块文档和契约目录；没有对所有 Go 源码、全部 Schema/Golden、全部消费者调用点进行逐项静态审计，也没有在本次 SHA 上执行全仓 CI。

因此本次状态定为 `review_in_progress`，不是 `frozen`。不得把本文件的创建提交视为契约冻结完成。

## 2. 冻结状态定义

| 状态 | 含义 | 使用条件 |
|---|---|---|
| `pending_decision` | 关键语义、职责或安全边界未决 | 必须先由项目负责人裁决 |
| `accepted` | 项目层面已确认语义和职责 | 可据此实现；仍不代表证据齐备 |
| `pending_evidence` | 语义已有基线，但缺适用的机器契约、Golden、消费者映射或测试 | 可继续不依赖该证据的开发 |
| `frozen` | 版本、定义、生产方与所有直接消费者证据、兼容性和安全测试齐备 | 可作为稳定项目契约基线 |
| `deferred` | 明确延期、目标批次、触发条件及不阻断范围 | 仅阻断明确列出的调用点 |
| `rejected` | 已否决并记录替代方案 | 不得再作为有效契约引用 |

`implemented`、`closed`、单模块 CI success、Fake 测试通过、文档已写，不是契约状态，也不能单独支持 `frozen`。

## 3. 项目级架构与职责基线

以下内容在 integration 现有契约目录和专项文档中具有一致的设计依据，可作为本轮评审的候选基线；其生产接入仍按各项证据门禁单独验收。

| 领域 | 项目级约定 | 责任边界 |
|---|---|---|
| Platform | 提供 Fault、Request Context、Security 原语、Config/Secret、Diagnostics Runner、受控 HTTP Client、Storage/Migration 执行原语 | 不拥有领域 DTO、Repository、领域授权、业务状态机或领域 Migration 内容 |
| Server | 唯一 Composition Root 和进程运行入口；负责共享资源创建、DI、路由、中间件、Migration 汇总注册、Worker 生命周期和优雅关闭 | 不实现领域规则、不拥有模块 Repository/业务状态 |
| Data | 拥有行情/基础数据 Provider、标准化、单位、质量、PIT、Repository 和领域 Migration | 目标为 Root Server 同 Go Module/同进程模块化单体；HTTP Adapter 可保留兼容/远程用途 |
| News | 拥有来源授权、采集、内容标准化、版本、证券关联、证据查询和领域诊断 | 首期为根 Go Module 内部能力，不默认建设独立公网 News 服务 |
| Research | 拥有 Context、Evidence、Event、Claim、Hypothesis、Scenario 等研究领域对象 | 只通过 Data/News 等公开 Port/DTO 消费事实和证据 |
| Quant | 拥有确定性 Indicator、Factor、Metric、风险计算和结果模型 | 消费 Data 公开 DTO；不依赖 Research 或 Researchflow 私有实现，不调用 LLM |
| Researchflow | 唯一跨模块流程编排者，拥有 Run/Stage/Attempt、状态机、输入快照和执行约束 | 只调用各模块公开 Application Port，不访问其 Repository/Domain 私有对象 |
| LLM | 拥有模型 Provider Adapter、Responses JSON/SSE 协议解析、Schema/输出校验 | 不拥有 Tool Registry/授权/执行，不复制 Platform HTTP Client |
| Notification | 拥有事件 Schema Registry、Inbox、Subscription、Delivery/Attempt、Channel Adapter 和投递诊断 | 不拥有生产方 Outbox/Dispatcher，不反向依赖业务模块 |
| WebUI | 只消费服务端 API，负责浏览器交互和展示 | 不在前端复制领域质量判断、权限决策或服务端业务规则 |

### 3.1 依赖方向

现有项目级契约目录给出的消费者到生产方逻辑依赖基线：

- Data、News、LLM、Notification 依赖 Platform 公共技术原语。
- Quant 消费 Data；风险事件按明确的事件契约发布。
- Research 消费 Data、News 的公开事实/证据契约。
- Researchflow 编排 Data、News、Quant、Research、LLM、Notification 的公开 Application Port。
- Server 作为唯一组合根装配各模块。
- WebUI 通过 Server 对外暴露的 HTTP 契约访问服务。

模块同进程不构成绕过公开 Port、访问对方 Repository/SQL 或依赖私有领域对象的理由。实际依赖若与该图不一致，先登记差异和决策，不以“能编译”作为接受理由。

### 3.2 Outbox / Inbox 所有权

- 业务生产方拥有自己的 Outbox、Dispatcher/Relay、发布状态和重试。
- Server 负责依赖注入、运行角色和进程生命周期装配。
- Notification 负责 Inbox、事件校验、订阅匹配和 Delivery/Attempt。
- Platform 只提供 DB/事务、HTTP、日志、Fault、Diagnostics 等技术原语。

禁止把生产方 Outbox/Relay 迁入 Notification、Server 或 Platform。

## 4. 关键公共语义候选基线

| 主题 | 当前候选约定 | 证据/未决边界 |
|---|---|---|
| JSON 数值 | 精确价格/金额使用 Decimal string；integer 用于离散计数；number 仅用于允许浮点误差的统计量；单位、scale、舍入按字段声明 | 公共规则已在既有目录登记为 accepted；具体字段仍需领域 Schema/Golden |
| 时间与 PIT | 绝对时间 RFC3339Nano UTC；交易日 YYYY-MM-DD 并按 Asia/Shanghai 解释；system_as_of、available_at、published_at、valid_time、system_observed_at 等语义独立 | 项目语义已接受；具体 DTO 映射和历史可见性测试未全部完成 |
| ID | request_id、originating_request_id、correlation_id、event_id、idempotency_key 各自有独立作用域，不互相替代 | GOV-CTR-003 仍 pending_evidence，需 Header/Context/Worker/Event Golden |
| Schema 版本 | DTO/Event 独立 schema_version；破坏性语义变化升 MAJOR；已注册事件 Schema 不可原地改写 | 具体消费者兼容矩阵和测试仍需补齐 |
| Fault | 内部 Fault 不直接序列化；Transport 映射独立公开 DTO；retryable 是候选提示而非重试命令 | PLAT-CTR-001 仍 pending_evidence，HTTP Mapper/Golden/消费者测试待证 |
| Diagnostics | Platform 负责 Check 执行/超时/并发/基础聚合；领域负责 reason_code、质量语义和 evidence_ref 解释授权 | 领域 Adapter 映射和消费者证据逐项核验 |
| Security | Platform 提供身份/Scope 技术原语；Server 建立可信认证上下文；资源授权和实时复核归资源领域 | 授权引用、撤销竞态、可信上下文注入仍需证据 |
| HTTP Client | Platform 提供 DestinationPolicy、DNS/IP 校验、受控 Dial、重定向逐跳验证、超时/响应上限 | A1 包级能力不等于 Server/领域真实 A2 或生产网络 A3 |
| Storage/Migration | Platform 提供连接生命周期/执行原语；领域拥有表、Repository、Migration 内容；Server 汇总注册并管理资源生命周期 | 需要真实 Bootstrap、迁移失败恢复和共享资源生命周期证据 |
| 长任务 | Researchflow 拥有 Run/Stage/Attempt 和领域状态机；Platform 不建设万能工作流 | Repository/Worker/恢复/并发证据需逐项完成 |

## 5. 契约分域状态总览

下表是本次基于 integration 已有项目级契约目录及可见专项 README 形成的审查索引。具体单项状态以详细登记目录和生产方机器契约为准；没有重新核验的内容不升级状态。

| 契约域 | 当前判断 | 主要缺口 |
|---|---|---|
| GOV 公共语义 | 部分 accepted，部分 pending_evidence | ID 传播、AuthorizationRef Schema/Golden/撤销测试 |
| PLAT 公共原语 | 设计与包级实现已推进；项目契约未整体 frozen | Fault HTTP 映射、Request ID 消费者、Security 接入、Diagnostics Adapter、HTTP Client A2/A3、Storage 装配证据 |
| DATA | 有既有 API/领域契约；本次未逐接口审计 | Decimal 精度、复权/PIT/质量、同进程 Principal/Scope、AuthorizationRef、Consumer DTO |
| NEWS | 领域设计和部分实现推进 | AuthorizationRef、Cursor/Snapshot、Query Facade、PIT 和当前授权复核 |
| RSC Research | 领域 DTO/PIT/证据语义有设计基线 | Schema/Golden、引用完整性、授权与真实 Adapter |
| QNT | Indicator/Factor/Registry 实现推进 | Registry Schema/Golden、Data 输入适配、结果授权和读取复核 |
| FLOW Researchflow | 状态机和流程设计有基线 | Stage Output Schema/Golden、Provider Adapter、Repository/Worker/恢复证据 |
| LLM | JSON/SSE Provider 适配已实现一部分 | 当前 integration 与 Platform 流式 API 的版本差异需重新核实；包级 CI 和真实 Gateway E2E |
| NTF Notification | Event/Delivery/Repository 部分实现 | Inbox、Fanout/Dispatcher/Worker、事件生产方映射和真实 MariaDB/渠道证据 |
| SRV Server | 唯一组合根设计已明确 | 实际依赖图、DI、路由、Migration 注册、启动回滚、Worker drain、关闭顺序 |
| WEB WebUI | 浏览器端契约映射资料存在 | 登录 Session/CSRF/权限与 Server 真实接口联调、当前 HEAD Browser Smoke |

### 5.1 当前明确不能宣称 frozen 的事项

- PLAT-CTR-001～007：Platform 候选契约及包级证据不能代替项目消费者/Server 接入证据。
- GOV-CTR-003、GOV-CTR-006：跨请求/异步/事件 ID 传播以及 AuthorizationRef 仍需机器契约和消费者/撤销证据。
- DATA/NEWS/Research/Quant/Researchflow/LLM/Notification：各自待补 Schema、Golden、Contract Test 和直接消费者映射，按详细登记目录逐项判定。
- INT-CTR-001：生产方事件映射、Outbox/Inbox 幂等和消费者测试未齐。
- SRV-CTR-001：真实组合根装配、共享资源、启动失败回滚和关闭/Worker drain 证据未齐。

## 6. 冻结门禁

每条契约按适用性完成以下项目；不适用必须写明理由，不得留空后直接通过：

1. 明确生产方、直接消费者、调用方向和公开边界。
2. 明确字段类型、必填/可空/缺失/零值、单位、精度、时间语义和 ID 作用域。
3. 提供版本化 Schema 或等价机器可校验定义。
4. 提供正常、边界、异常、兼容 Golden。
5. 生产方序列化和消费者反序列化 Contract Test 均通过。
6. 适用的状态机、幂等、并发、重试、取消、恢复测试通过。
7. 适用的授权、Secret 脱敏、SSRF、PIT 和内容级别边界测试通过。
8. 记录消费者 Adapter 字段映射、未知字段策略和兼容矩阵。
9. 记录测试命令、结果、精确代码 SHA、工作流/测试证据链接。
10. 记录迁移/废弃策略及回滚限制。

任何关键项缺失，保持 `pending_evidence`；语义未决定，保持 `pending_decision`。不允许通过降低测试范围、使用 Fake 替代真实生产方或复制同一实现两端自测来绕过门禁。

## 7. 本轮审查范围与限制

本轮核验确认了：
- integration 分支存在，基线 SHA 为 `bb8cf3495b9591ede2a29b554d9121b06b983f0b`。
- integration 包含既有跨模块契约目录、Platform 候选契约、Server 规划和多个模块专项 README。
- 既有项目级契约目录仍声明 `feat/design_doc` 为权威维护分支；这与本次以 integration 作为聚合审查/冻结基线的约定存在治理冲突。
- Platform 文档明确 PLAT-CTR-001～007 仍为 `pending_evidence`。
- Server 规划明确唯一 Composition Root、共享资源所有权和模块 Migration 责任边界，但真实接入验收证据仍需补充。
- 模块 README 对若干未完成能力、测试和真实环境证据有明确限制。

本轮未完成：
- 全量读取所有 `internal/**` 源码、全部 `doc/**` 专项文件、全部 Schema/Golden。
- 对每个直接消费者执行字段级 DTO/Port/Adapter 对照。
- 在该 integration SHA 上运行 `go test ./...`、race、vet、Schema Validator 或真实 MariaDB/Redis/Gateway/Provider E2E。

因此，本文当前只能作为项目级契约审查登记簿的初始版本，不能声明整个 PLAT-002 已冻结。

## 8. 后续执行与审批

1. 项目负责人确认项目级契约权威维护位置由原 `feat/design_doc` 转为 `integration`，并确认本目录为新的项目级冻结入口。
2. 各模块窗口按本登记簿的契约域清单，针对当前 integration SHA 核对生产方定义和直接消费者调用点，提交模块 SHA、Schema/Golden、Contract Test 和 CI 证据。
3. 集成窗口按依赖关系汇总差异，不替模块虚构验收结论；对无法由代码证据判断的事项列入决策记录。
4. 项目负责人裁决关键语义、兼容性、安全边界和延期范围。
5. 所有冻结门禁满足后，更新本登记簿版本与状态，明确冻结的契约编号和代码 SHA。PLAT-002 整体冻结前，不得将 `review_in_progress` 改为 `frozen`。


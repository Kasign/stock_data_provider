# Research 模块卡点

- 更新日期：2026-11-19
- 对应模块分支：`feat/research`
- Research 代码与模块状态核对基线：`0d9dc0651a6e0bda3888d695f2ffec5545c248bd`（Diagnostics 脱敏测试修复后的当前验收 HEAD）
- 代码范围：`internal/research/**`
- 模块文档：`doc/research/**`
- 本文用途：记录 Research 当前无法独立闭环的外部依赖、责任方、影响和重新进入条件；不替代 Research 模块任务规划，也不冻结或修改跨模块契约。

## 1. 当前结论

Research 的领域基础能力、Repository/Outbox、Context 消费契约测试及 Event 版本血缘规则级实现已经具备。当前剩余工作主要依赖上游真实证据、公开消费契约和 Server 生产装配。

**当前 Research 没有可以安全继续扩展的独立业务代码任务。** 本轮 Diagnostics 脱敏测试和 Outbox 终态空租约修复已由 Run `44439983842` 在当前精确 SHA 上通过 Research 专项与 MariaDB 验收。剩余事项主要等待上游真实证据、消费者调用点和 Server 生产装配；不应猜测时间来源、证券/行业引用格式、诊断消费者或 Quant 结果结构。

## 2. 卡点清单

| Research 任务 | 当前状态 | 卡点及原因 | 责任方 | 对 Research 的影响 |
|---|---|---|---|---|
| RSC-002 Evidence 真实成功导入 | SHELVED | News 当前不提供可审计的首次公开可获取时间 `available_at`；News→Research B 组按项目决议搁置。不得用 `published_at`、`observed_at` 或抓取时间推导 | News | Importer 保持 fail-closed；不得伪造成功写入或将 Fixture 成功当作真实 PIT 证据 |
| RSC-003 Event 来源时间与派生关系 | WAITING_EXTERNAL_EVIDENCE（规则级已完成） | 缺少真实 `occurred_at/published_at/available_at` 原始值、来源版本、时区/精度证据；缺少真实多来源合并/拆分及直接父 Event 版本样本；缺少 `asserted_by` 到来源/系统主体的映射依据 | News、Data | 不得扩展时间推断、主体命名空间/枚举/注册表；现有测试只证明规则行为，不证明真实来源符合性 |
| RSC-004 Security/Industry 实体关联 | BLOCKED | 稳定 `SecurityRef/IndustryRef`、历史版本/知识时间语义和公开 Data Application Port 尚无可用冻结依据；项目登记中的 DATA-CTR-007 为 deferred | Data | 不复制 Data 主数据模型、SQL 或解析器；无法可靠实现历史时点实体关联 |
| RSC-005 Claim/Hypothesis Scope 与真实样本 | WAITING_DEPENDENCY | 依赖稳定 Security/Industry 引用、可授权 Evidence 样本和明确消费者调用场景；当前 Domain、版本/状态、正反 Evidence 校验和 Repository 已实现 | Data、News、实际消费者（Researchflow 等） | 不猜测 Scope 引用格式，不新增无调用方接口；当前没有独立可执行代码任务 |
| RSC-006 Scenario/Quant Evaluation 消费 | WAITING_DEPENDENCY | Quant 尚无完整可注入的计算 Application Facade/Metric Registry 生产闭环；Researchflow 尚未提供对应真实消费者调用链；Factor 相关契约受 DATA-CTR-005/007 与 QNT-CTR-003 deferred 约束 | Quant、Researchflow、Data | 不预建 Scenario API/Repository，不在 Research 重算 Quant 指标，不接受未注册或不可验证的结果引用 |
| RSC-008 Research Diagnostics 消费验收 | WAITING_DEPENDENCY | Research 的领域 Check/DiagnosticsPort 已存在，但 Researchflow 尚未提供真实 Facade/Check 消费点；Platform Runner 的通用执行能力由 Platform 提供 | Researchflow、Platform | 不虚构消费者、不复制 Diagnostics Runner；暂不能完成消费者状态映射和运行时装配验收 |
| RSC-001 生产身份与授权装配 | Research 模块范围 DONE；系统验收待完成 | Research 消费契约测试已覆盖；Server 仍需从可信 Principal 注入 CallerRef 并装配真实 Context Owner Authorizer | Server | 不在 Research 内伪造生产 Principal 或重复实现授权装配 |
| RSC-007 Root Migration/Relay/Worker 组合验收 | Research 模块范围 DONE；本轮 Research MariaDB 回归验收通过；系统验收待完成 | Research 自有 Repository、Migration v1、事务 Outbox 及自然过期租约/旧 token fencing/拒绝终态回归已通过 Run `44439983842`；根 Migration 注册、Relay/Worker 生命周期属于组合根职责 | Server、Integration | 不重复建设 Root Migration Runner、Relay 或 Worker 生命周期 |

## 3. 上游当前状态核对依据

以下是本次只读核对到的模块状态，不代表对其代码做了修改或代替其 Owner 验收：

- **News（`feat/news`）**：当前任务为 NEWS-POST-02。采集前当前策略 fail-closed 预检已实现并有 News 测试/MariaDB Repository 集成证据；但合法 Source/Endpoint 注册数据、Server 生产组合、真实 Feed 网络采集和 Collect→Ingest 端到端路径仍未闭环。News 明确指出无合格 `available_at` 来源时不产生生产可用时间，News→Research B 组仍为 SHELVED。
- **Quant（`feat/quant`）**：当前 QNT-001/QNT-002 仍在完善 Registry Entry 和 Indicator 输出契约；没有完整 Indicator/Factor 计算 Application Facade。DATA-CTR-005/007、QNT-CTR-003 仍为 deferred；Factor 不得作为生产结果发布。
- **Researchflow（`feat/researchflow`）**：当前执行 FLOW-001/FLOW-002 契约符合性 Review；正式生产 Stage 仍以 `news.search_candidates@1.0.0` 为已确认项。没有 Research Diagnostics 的真实消费调用点，也没有可用于 RSC-006 的 Research/Quant 完整生产消费者链。
- **Data**：本轮未能从预期的 `doc/data/当前任务状态和进度.md` 路径读取模块状态；因此本文不推断 Data 当前实现进度。DATA-CTR-007 deferred 及 DATA-CTR-005/007 的阻断事实，以已读取的项目/Quant 记录为依据；具体重新进入条件仍须 Data Owner 给出最新证据。

## 4. 重新进入条件

| 卡点 | 重新进入条件 | Research 解锁后的工作 |
|---|---|---|
| RSC-002 | News 提供可审计 `available_at` 原始来源、首次公开可获取语义、授权复核及精确版本读取一致性证据 | 恢复真实 Evidence 成功写入验收；保留缺证据 fail-closed 用例 |
| RSC-003 | News/Data 提供可追溯真实时间样本、来源版本/精度/时区、多来源事件父子版本样本和 asserted_by 主体映射依据 | 只按证据补充规则/测试；将规则级测试与真实来源验收分开记录 |
| RSC-004/RSC-005 | Data 冻结稳定 SecurityRef/IndustryRef、历史版本/知识时间语义及公开 Application Port；News/Data 提供可授权 Evidence 样本；出现真实消费者调用场景 | 通过公开 Port 实现实体关联与 Scope 消费契约，不复制上游模型 |
| RSC-006 | Quant 提供版本化 Metric Registry、QuantEvaluationRef 和可注入 Application Port；Researchflow 给出真实消费者与授权传播方式 | 补 Scenario 消费端 Contract Test 和引用校验，不重算 Quant |
| RSC-008 | Researchflow 提供真实 Facade/Check 调用点，Platform Runner 的生产装配与诊断映射可核验 | 补真实消费者契约测试，确认 pass/degraded/fail/not_comparable 映射 |
| RSC-001/RSC-007 系统验收 | Server/Integration 提供当前精确 SHA 的可信身份注入、真实 Owner Authorizer、Root Migration、Relay/Worker 验收证据 | Research 只复核本模块边界，不代替组合根验收 |

## 5. 当前执行策略

1. Research 继续维护现有领域规则和测试；只有出现可复现的本模块缺陷，才进行代码修复。
2. 对 RSC-003，保持规则级完成、真实来源证据待验收，不继续增加推测性时间或主体规则。
3. 对 RSC-002，保持 SHELVED；对 RSC-004，保持 BLOCKED；对 RSC-005/RSC-006/RSC-008，保持 WAITING_DEPENDENCY。
4. 不为等待中的任务创建空壳接口、默认注册项、假数据、临时表或 Mock 生产装配。
5. 每次上游状态变化后只做定点复核：核验契约状态、公开 Port、真实样本和精确 SHA；满足重新进入条件再恢复对应 Research 任务。
6. 本文记录的是 Research 当前卡点快照；模块内部任务状态仍以 `doc/research/当前任务状态和进度.md` 为准，项目级契约状态仍以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 为准。


## 6. 本轮模块内验收更新（2026-11-19）

- 当前 Research HEAD：`0d9dc0651a6e0bda3888d695f2ffec5545c248bd`。
- Workflow Run [44439983842](https://github.com/Kasign/stock_data_provider/actions/runs/44439983842) 整体结论为 `success`；Research module format/tests/race/vet、Research MariaDB migration/repository integration、根模块 `Test .`、`Test go-core` 均通过。
- Research Diagnostics 脱敏测试使用符合 Event 版本血缘规则的 EventVersion=1 样本；不放宽生产版本校验。诊断结果语义、PIT 时间边界、Owner 授权复核、快照隔离和敏感信息不泄露的测试结论记录于 `doc/research/任务历史/14-RSC-008-Diagnostics脱敏边界与结果语义审查.md`。
- 本次 Compose、Market Data、News 等 Job 为 skipped；它们不构成通过证据，也不改变相应模块卡点。
- 跨模块卡点仍按本文表格执行：News/Data/Quant/Researchflow 提供重新进入证据，Server 负责生产身份、Migration/Relay/Worker 组合装配。


## 7. RepairAll Data / News → Research 交叉审查更新（2026-11-19）

- 审查基线：repair_all 代码树 5334043ea8e1abc2c4006c371a3f95b123098946；随后仅有总体计划文档校准提交。
- Data→Research：当前 internal/research/ 仅有 News Adapter，没有 Data Adapter；internal/marketdata/app/port.go 虽提供 MarketDataReader/DailyBarsSnapshot，但 DATA-CTR-007 在项目级登记中为 deferred。缺少不可变内容寻址 SnapshotRef、Manifest/Payload 绑定、AuthorizationRef 及 Research Evidence source_ref/version/locator 映射。保持 RSC-004 BLOCKED，不新增猜测性 Adapter/DTO。
- News→Research：internal/research/adapters/news/evidence.go 已实现 Importer/Authorizer；News Query Facade、PIT 单条/分页、当前授权复核及事务内锁定当前策略指针后复读 Observation/Version、校验并读取内容均存在。当前实现机制不重复开发。
- Server：internal/server/bootstrap/research_news.go 装配组合存在；bootstrap.go 仅在 Cursor Key Ring 配置时创建，并明确没有真实 Stage/HTTP 消费者。该组合目前不是生产消费闭环。
- News available_at 仍无合格来源证据，RSC-002/NEWS-BLK-07 保持 SHELVED；不以 published_at、fetched_at、observed_at 替代。
- 本轮未发现能在不越过冻结/待决契约的前提下直接修复的代码缺陷；本卡点不代表代码端到端 PASS。生产真实数据验收为 NOT_COMPARABLE。

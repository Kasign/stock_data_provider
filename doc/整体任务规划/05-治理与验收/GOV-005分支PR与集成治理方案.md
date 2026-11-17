# GOV-005 分支、PR、目录 Owner 与代码聚合治理方案

- 所属治理任务：GOV-005
- 状态：现行治理规则
- 规划分支：`feat/design_doc`
- 适用范围：项目所有开发分支、Server 实施分支及 `integration` 聚合分支。
- 本文件只定义治理规则；不自动创建分支、PR，不自动合并代码。

## 1. 目标与基本原则

本方案明确开发责任、共享目录 Owner、代码聚合边界和冲突处理方式。

1. 各开发分支只负责自己领域或专项范围内的实现、测试和文档。
2. `integration` 只做代码聚合：接收已完成的分支代码、处理必要合并冲突、维持聚合结果可构建。
3. `integration` 不承担业务功能开发、缺陷专项修复、根服务装配、架构设计或跨模块系统验收。
4. Server 实施分支负责应用组合根和服务端装配，不把业务领域实现迁入 Server。
5. Platform 提供公共技术原语；业务模块和 Server 消费这些能力，不重复建设。
6. 模块独立验收与聚合后的 CI/组合验收分开记录，失败归属回到对应 Owner。
7. 项目级规划、架构和跨模块契约由 `feat/design_doc` 维护；模块专项文档由各模块分支维护。

## 2. 当前分支角色

| 分支 | 职责 | 主要范围 |
|---|---|---|
| `feat/design_doc` | 项目级规划、架构、跨模块契约和治理 | `doc/整体任务规划/**` |
| `integration` | 代码聚合 | 接收完成的开发分支代码；必要时解决聚合冲突 |
| `feat/research-analysis-flow-native-go` | Server 实施和旧 Analysis 迁移 | 根启动入口、`internal/server/**`、经任务确认的旧 Analysis 迁移 |
| `feat/platform-foundation` | 公共平台能力 | `internal/platform/**`、`doc/platform/**` |
| `feat/full-go-data-providers-api-switch` | Data/Provider/API 专项 | Data 领域实现、Provider、Repository、Data API 和迁移 |
| `feat/research-news-ingestion` | News 专项 | `internal/news/**`、`doc/news/**` |
| `feat/quant` | Quant 专项 | `internal/quant/**`、`doc/quant/**` |
| `feat/research` | Research 专项 | `internal/research/**`、`doc/research/**` |
| `feat/researchflow` | Researchflow 专项 | `internal/researchflow/**`、`doc/researchflow/**` |
| `feat/llm` | LLM 专项 | `internal/llm/**`、`doc/llm/**` |
| `feat/notification` | Notification 专项 | `internal/notification/**`、`doc/notification/**` |
| `feat/webui-market-data-console` | WebUI 专项 | `web/**`、`doc/web/**` |

以上是职责归属，不代表各分支当前任务全部完成。各分支的实施状态、测试结果和专项验收由对应分支记录；本方案不维护各模块 HEAD 快照。

## 3. PR 与代码聚合规则

1. 各开发分支按仓库当前授权流程提交 PR；项目代码聚合目标为 `integration`。
2. PR 的合并对象必须是已完成且经过对应 Owner 自检的变更；未完成的设计草案、临时补丁和未验证的跨模块接口不得仅为解决聚合冲突而合入。
3. `integration` 不作为日常开发分支，不从 integration 分叉新的专项任务，也不在其上直接修复业务缺陷。
4. 聚合时出现冲突，由冲突涉及的文件 Owner 提供正确实现；必要时由 Server/相关模块 Owner 协调后，在对应开发分支形成修复提交，再重新聚合。
5. integration 上的 CI 仅验证聚合结果。失败必须记录失败 SHA、工作流和责任模块；修复回到对应 Owner 分支。
6. 不以 integration CI 绿灯代替模块自身验收，也不以单模块 CI 绿灯代替跨模块组合验收。
7. 本方案不规定 integration 向 main 或其他分支的后续发布流程；该流程按用户另行确认的仓库治理规则执行。

## 4. 目录 Owner 与共享文件

| 路径/能力 | Owner | 规则 |
|---|---|---|
| `internal/platform/**`、`doc/platform/**` | Platform | 业务模块不得直接修改公共平台实现 |
| Data 领域目录、`doc/api/**` | Data | Data 拥有领域模型、Provider、Repository、Migration 和数据语义 |
| `internal/news/**`、`doc/news/**` | News | News 拥有采集、存储、检索和只读 Evidence Facade |
| `internal/quant/**`、`doc/quant/**` | Quant | Quant 独占 |
| `internal/research/**`、`doc/research/**` | Research | Research 独占 |
| `internal/researchflow/**`、`doc/researchflow/**` | Researchflow | Researchflow 独占 |
| `internal/llm/**`、`doc/llm/**` | LLM | LLM 独占 |
| `internal/notification/**`、`doc/notification/**` | Notification | Notification 独占 |
| `cmd/**`、`internal/server/**`、根 DI/Bootstrap | Server | 由 Server 实施分支负责 |
| 根 HTTP Router/传输注册 | Server | 模块提供 Handler/Registrar；Server 负责根路由注册 |
| 根 Migration 汇总/Runner 装配 | Server | 模块拥有自己的 Migration；Server 汇总并装配执行 |
| Worker 进程角色和生命周期 | Server | 模块拥有 Worker/Handler 及其业务语义；Server 负责启停和资源注入 |
| 根 `go.mod`/`go.sum` | Server 协调变更，受影响模块参与 Review | 只修改实际需要的依赖，不做无关升级/重排 |
| 全局部署入口、Compose、Docker、配置装载 | Server | 模块提供模块级配置定义，不改写全局入口 |
| `doc/整体任务规划/**` | design_doc | 仅项目级规划、架构、契约和治理 |
| `doc/<module>/**` | 对应模块 Owner | design_doc 不代替模块维护实现进度 |

共享文件变更必须说明调用方、影响范围、兼容性、回滚和验证方式。业务模块可提交接口/注册清单及测试，不得并行改写根启动入口。

## 5. Server、Platform 与模块的装配边界

- 各业务模块拥有 Domain、Application、Repository、Migration、模块 Worker/Handler 和 Outbox 业务语义。
- Platform 提供 DB/Redis 连接、事务基础组件、HTTP Client、Fault、Request Context、Diagnostics Runner 等公共技术原语。
- Server 是唯一 Application Composition Root：创建并注入公共资源，装配模块 Facade/Adapter，注册根 HTTP 路由，汇总根 Migration，并管理 API/Worker 运行角色和生命周期。
- Server 不拥有业务状态机、领域重试策略、业务幂等或 Outbox 事件语义。
- 模块间同进程调用通过公开 Application Port；不通过 localhost HTTP 绕行，不访问对方 Repository/SQL/私有 Domain。
- Notification 拥有 Inbox、订阅、Delivery 和外部渠道投递；不因此拥有其他模块的 Outbox。

## 6. Outbox / Dispatcher 所有权

1. Quant、Research、Researchflow 等事件生产方拥有自己的 Outbox 表/Repository、领域事件事实、事务内写入和事件发布语义。
2. 每个生产方拥有与其 Outbox 对应的 Dispatcher/Relay 实现，包括批量领取、租约/claim、重试、退避、幂等、死信/人工恢复边界及发布状态诊断。
3. Server 负责把各生产方 Dispatcher 装配到运行角色，注入共享资源，并统一启停、关闭和进程级健康观察；Server 不实现生产方业务发布规则。
4. Notification 拥有 Inbox 接收、Schema Registry、Subscription、Delivery、Attempt 和渠道投递；不扫描、不代发、不恢复其他模块的 Outbox。
5. Platform 可提供通用 DB/HTTP/日志/诊断等技术原语，不提供替代业务 Outbox 的 Task Queue，也不拥有事件投递语义。
6. Outbox 到 Notification Inbox 的本地可靠接收不等于外部渠道 exactly-once；外部副作用不确定性按 Notification 契约处理。
7. 现有模块实现若与以上所有权不一致，应由对应模块 Owner 在自身分支评估并提出兼容迁移，不因本项目级决议直接要求重写既有实现。

## 7. Migration、DI 与 HTTP

### 7.1 Migration

- Migration 文件和领域 Schema 由各模块拥有，namespace 唯一。
- 模块提供 Migration 清单或注册函数，不修改根总注册入口。
- Server 汇总模块 Migration，装配 Platform Migration Runner，并负责执行顺序、启动失败处理和运行生命周期。
- integration 只聚合各分支已完成的变更，不维护 Migration 注册逻辑。

### 7.2 DI / Bootstrap

- 模块提供可注入的构造函数和公开依赖接口，不创建跨模块全局单例。
- Server 实施分支负责 Config 聚合、资源构造、依赖装配、启动失败回滚和优雅关闭。
- 各模块构造函数必须可注入 Fake/Stub，便于模块独立测试。

### 7.3 HTTP 路由

- 模块拥有自己的 Application Port 和必要的 Handler/Route Registrar。
- Server 负责根 Router 注册、统一入口中间件、API 版本和 OpenAPI 汇总。
- 模块 HTTP Adapter 不直接访问其他模块 Repository。
- 认证 Secret 只由服务端注入，不进入 WebUI/浏览器。

## 8. 模块独立开发与验收

模块独立开发、L1/L2/L3 状态、契约变更、测试覆盖和状态表达，遵循《GOV-005模块独立开发与集成治理规范》。本文件不重复定义模块内部任务和验收细则。

## 9. 冲突处理

- 不使用 force push 覆盖他人提交；更新 ref 使用 expected SHA/lease。
- 不通过复制其他模块代码解决编译问题。
- 不在模块 PR 中顺手格式化或重构共享目录。
- 冲突按文件 Owner 解决；解决提交落在对应 Owner 分支，再重新执行聚合。
- integration 的 CI 失败只作为定位信息，不改变代码 Owner，不授权在 integration 直接修复。

## 10. GOV-005 执行边界

- 本文件冻结治理职责，不自动授权任何分支创建、PR 创建、合并或发布操作。
- 各分支按用户指令和仓库既定流程操作；不得为执行本方案擅自创建 `feat/research-integration` 或 `feat/analysis`。
- integration 只聚合代码；Server 实施由 `feat/research-analysis-flow-native-go` 承担。
- 其他模块专项任务和文档维护由对应分支负责。

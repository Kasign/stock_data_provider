# GOV-005 分支、PR、目录 Owner 与集成治理方案

- 所属治理任务：GOV-005
- 状态：治理方案草案；等待 GOV-004 通过及用户确认后执行。
- 规划分支：`feat/design_doc`
- 重要限制：本文件只定义规则，不创建分支、不创建 PR、不合并任何分支、不修改 main。

## 1. 目标

五模块并行开发必须建立在同一个可构建、可验证的代码基线上，并明确：
- 模块分支从哪里创建；
- 模块 PR 进入哪里；
- 哪些目录由模块独占；
- 哪些共享目录由集成分支/Server Owner 独占；
- 跨模块契约变更如何协调；
- Migration、DI、HTTP 路由和启动入口如何集成；
- 如何防止模块并行开发产生互相覆盖和循环依赖。

## 2. 分支拓扑建议

### 2.1 分支角色

| 分支角色 | 建议名称 | 职责 |
|---|---|---|
| 规划分支 | `feat/design_doc` | 架构、契约、任务、治理文档；不承载业务代码 |
| 公共集成分支 | `feat/research-integration` | 汇总通过准入的 Platform/News/Server 基线，作为五模块共同开发起点和 PR 集成目标 |
| Quant 模块 | `feat/quant` | Quant Domain/Application/Adapter/Repository/Tests |
| Research 模块 | `feat/research` | Research Domain/Application/Adapter/Repository/Tests |
| Researchflow 模块 | `feat/researchflow` | Run/Stage/Attempt/DAG/Worker/Workflow/Tests |
| LLM 模块 | `feat/llm` | Model Protocol/Provider Adapter/Structured Output/Tests |
| Notification 模块 | `feat/notification` | Subscription/Template/Channel/Delivery/Retry/Tests |

以上均为建议名称，尚未创建。模块分支名保持短、可读、与模块目录一一对应；不再使用过长的 `feat/research-analysis-flow-native-go` 风格命名。

### 2.2 为什么需要公共集成分支

不建议五个模块直接从各自的 Platform/News/Server 工作分支分叉：
- Platform、News、Server 目前分支独立演进；
- 模块需要同一版本的 Platform 包、News Facade 和根服务构建入口；
- 如果各模块从不同分支分叉，会出现相同依赖在不同模块分支上的代码差异；
- 后续 PR 会在多个分支间重复解决冲突。

因此在 GOV-004 通过后，先创建一个公共集成分支，将经 Review 和验收的依赖基线按明确顺序汇总。五模块统一从该集成分支的同一个 SHA 分叉。

公共集成分支不是新微服务，也不拥有领域代码；它是 Git 集成基线和根服务装配归属。

## 3. PR 目标与合并顺序

### 3.1 PR 目标

- 五个模块 PR 的 Base 统一为 `feat/research-integration`。
- 模块 PR 不直接面向 main。
- 公共集成分支完成五模块集成、端到端测试和启动验收后，再由集成分支发起面向 main 的总集成 PR。
- `feat/design_doc` 只用于规划文档；是否将治理文档合入集成分支，由用户决定，不自动混入业务 PR。

### 3.2 推荐集成顺序

1. **公共基线准备**：确认 Platform 基础包、News Application Facade、Server 根模块/DI/路由入口及 Data Adapter 认证契约的准确提交 SHA。
2. **基础设施准入**：Platform Storage/HTTP Client 等阻断项通过后，将 Platform 变更纳入集成基线。
3. **News 准入**：News Repository/Migration、ResearchEvidenceQuery、PIT、授权快照和分页通过后纳入集成基线。
4. **模块 PR 并行 Review**：
   - Quant；
   - Research；
   - LLM；
   - Notification。
5. **Researchflow 集成**：在 Quant/Research/LLM/Notification 的公开 Application 契约稳定后集成编排；Researchflow 可提前开发自己的 Run/Stage/DAG，但不得复制其他模块领域实现。
6. **根服务装配**：由集成分支 Owner 统一修改 DI、HTTP 路由、Worker 启动、Migration 注册和配置装载。
7. **系统验收**：全量 Go 测试、race/vet、Migration 集成、端到端 Run、故障/取消/重试/权限/诊断和 WebUI 契约测试。
8. **总集成 PR**：从 `feat/research-integration` 提交到 main，Review 通过后再由用户决定合并。

模块可以并行开发，但依赖模块的公开接口必须先冻结或通过接口 PR 确认。不能通过临时复制 DTO 或依赖私有包绕开依赖顺序。

## 4. 目录 Owner 矩阵

| 路径 | Owner | 模块分支能否直接修改 |
|---|---|---|
| `internal/quant/**` | Quant | 仅 `feat/quant` |
| `internal/research/**` | Research | 仅 `feat/research` |
| `internal/researchflow/**` | Researchflow | 仅 `feat/researchflow` |
| `internal/llm/**` | LLM | 仅 `feat/llm` |
| `internal/notification/**` | Notification | 仅 `feat/notification` |
| `internal/platform/**` | Platform | 模块分支禁止直接修改 |
| `internal/news/**` | News | 模块分支禁止直接修改 |
| `services/market-data/**` | Data | 根模块五分支禁止直接修改 |
| `cmd/**` | 集成/Server Owner | 模块分支禁止直接修改 |
| 根服务 DI/Bootstrap | 集成/Server Owner | 模块分支禁止直接修改 |
| 根 HTTP Router/Handler 注册 | 集成/Server Owner | 模块分支禁止直接修改 |
| 根 Migration 总注册/启动执行入口 | 集成/Server Owner | 模块分支禁止直接修改 |
| 根 `go.mod` / `go.sum` | 集成 Owner 协调 | 模块分支不得擅自重排/升级全局依赖 |
| 全局 Config 装载入口 | 集成/Server Owner | 模块分支只提供模块 Config 类型/校验 |
| `doc/<module>/**` | 对应模块 Owner | 仅对应模块分支维护 |
| `doc/整体任务规划/**` | 规划/GOV Owner | 模块分支不擅自改总体规划 |
| `doc/api/**`、`doc/news/**`、`doc/platform/**`、`doc/web/**` | 当前各自工作流 Owner | 未授权不得修改 |

### 4.1 共享接口变更

- 每个模块拥有自己 Application Facade 的定义和实现。
- 消费模块只依赖生产模块明确公开的 Application Port/DTO。
- 跨模块 DTO 变化由生产模块提出接口变更，消费模块 Review；不把共享 DTO 放进万能 common 包。
- Researchflow 是调用方和组合者，不拥有 Quant/Research/LLM/Notification 的领域类型。
- 如果确需公共技术类型，优先由 Platform 提供与领域无关的基础原语；不得将业务 DTO 放入 Platform。

## 5. Migration、DI、路由和启动入口

### 5.1 Migration

- 各模块 Migration 由各模块目录持有，namespace 独立且不可重复。
- 模块分支提供 Migration 清单/注册函数，但不修改根总注册入口。
- 集成分支统一收集模块清单并装配 Runner。
- Migration 版本不可变；修复使用新版本。
- Migration 执行顺序由 namespace 和显式依赖决定，不依赖目录遍历顺序。
- 未通过真实 MariaDB/Redis 集成和失败恢复演练，不允许宣称持久化生产就绪。

### 5.2 DI / Bootstrap

- 模块提供构造函数和依赖接口，不创建跨模块全局单例。
- 模块不得自行改根启动流程。
- 集成分支负责 Config 读取、依赖装配、资源生命周期、启动失败回滚和优雅关闭。
- 每个模块构造函数必须可注入 Fake/Stub，便于单测；生产 Adapter 由组合根创建。

### 5.3 HTTP 路由

- 模块可提供 Handler/Route Registrar，但只有集成 Owner 修改根 Router 注册。
- Owner/service 权限由统一入口执行；模块业务授权由对应领域 Application/Policy 决定。
- 认证 Secret 只在服务端注入，不进入 WebUI/浏览器。
- 公开 liveness、readiness 和详细 diagnostics 的权限/返回字段必须明确。

## 6. 模块开发准入

每个模块分支创建前需具备：
- GOV-004 通过；
- 模块目录和 README/任务验收文档齐备；
- 模块公开 Application DTO/Port 已冻结；
- 依赖模块的契约版本和基线 SHA 已记录；
- 目录 Owner 明确；
- 禁止修改目录和共享文件流程明确；
- 最小构建/测试命令和 CI 门禁明确。

每个模块 PR 至少包含：
- 任务编号和验收项；
- 变更摘要与不变更范围；
- 新增/修改的公开 DTO/Port；
- Repository/Migration 变化；
- 单元/集成测试结果；
- 依赖提交 SHA；
- 已知限制和未完成项。

## 7. 冲突处理与合并策略

- 模块分支只修改自己 Owner 路径；发现共享目录需要变更，先提交变更申请，不直接编辑。
- 同一共享文件只允许一个 Owner 分支修改。
- 不使用 force push 覆盖他人提交；更新分支使用 expected_sha/lease。
- 不通过把别的模块代码复制到当前分支解决编译问题。
- 不在模块 PR 中顺手格式化或重构共享目录。
- 模块 PR 合入集成分支后，其他分支通过正常 merge/rebase 同步集成基线；每次同步后重跑模块测试。
- 合并顺序以依赖 DAG 为准，不以 PR 创建时间为准。
- CI 必须关联 PR head 的精确 SHA；没有 CI 记录时明确标记“尚无 CI 运行记录”。

## 8. GOV-005 执行门禁

GOV-005 的规划设计可先完成；实际执行仍需用户明确授权。

- [ ] GOV-004 全部通过并冻结公开契约。
- [ ] Platform/News/Data/Server 依赖基线 SHA 精确记录。
- [ ] 公共集成分支源 SHA 与用途确认。
- [ ] 五个短命名模块分支名称确认。
- [ ] 所有模块分支从同一个公共集成 SHA 创建。
- [ ] 五个模块 PR Base 统一为公共集成分支。
- [ ] 目录 Owner 和共享文件 Owner 确认。
- [ ] Migration/DI/HTTP 路由/Bootstrap 集成 Owner 确认。
- [ ] CI、测试和合并策略明确。
- [ ] 用户确认后才创建分支和 PR。

## 9. 当前结论

建议短分支名：
- `feat/quant`
- `feat/research`
- `feat/researchflow`
- `feat/llm`
- `feat/notification`

建议公共集成分支：`feat/research-integration`。

这些均为规划建议，**尚未创建**。当前 GOV-004 最终 Review 为 NO-GO，因此 GOV-005 只能完成治理方案设计，不能执行分支创建。等待 GOV-004 所有阻断项通过和用户确认。

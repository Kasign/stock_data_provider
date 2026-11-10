# PLAT-002 十模块核验总进度

**当前状态：** 第一阶段基线已建立；六维核验尚未完成。  
**代码基线：** `b6dfc85ba214fe16268c8cba780641a8bfe15086`（origin/integration）  
**文档维护分支：** feat/design_doc

## 1. 模块进度

状态说明：`not_started` 未开始；`in_progress` 正在核验；`partial` 已有局部证据但未完成；`verified` 六维核验完成且证据齐备；`blocked` 存在外部阻塞。当前不使用 verified 作为仅凭文档/目录存在即可达到的状态。

| 模块 | Port/调用点 | DTO/Schema/Golden | Fault/Diag/Request ID | 授权/Secret | Repo/Migration/Worker | 依赖边界 | 总状态 |
|---|---|---|---|---|---|---|---|
| Platform | partial（技术接口形态已核；消费者调用点待追踪） | partial | partial（Fault/Diagnostics/Request ID 基础实现与单测已核；消费者接入待核） | partial | partial | partial（发现 Task Queue 反向依赖旧 Analysis Domain，待裁决） | partial |
| Server | partial | not_started | partial | partial | partial | partial | partial |
| Data | partial | partial | partial | partial | partial | partial | partial |
| News | partial | partial | partial | partial | partial | partial | partial |
| Research | partial | partial | partial | partial | partial | partial | partial |
| Researchflow | partial | partial | partial | partial | partial | partial | partial |
| Quant | partial | partial | partial | partial | partial | partial | partial |
| LLM | partial | partial | partial | partial | partial | partial | partial |
| Notification | partial | partial | partial | partial | partial | partial | partial |
| WebUI | partial | partial | partial | partial | partial | partial | partial |

说明：partial 仅表示既有登记簿/起始快照中存在局部证据，不表示本轮已完成该维度复核。后续逐项更新并引用证据。

## 2. 当前执行状态

| 阶段 | 内容 | 状态 |
|---|---|---|
| A | 固定 SHA、模块清单、文件规模与并行修改观察 | in_progress（本次已建立初始基线，待完成差异复核） |
| B | 十模块六维接口级核验 | in_progress（Platform 维度 1 接口形态初审、维度 3 基础原语已核；消费者调用点及其余维度待核） |
| C | 跨模块调用图、授权链和依赖图 | not_started |
| D | 差异、待裁决、补证清单 | not_started |
| E | 责任模块修复/补证（本分支不改业务代码） | pending_audit |
| F | 第一轮完整性 Review | not_started |
| G | 第二轮正确性 Review | not_started |
| H | 第三轮冻结条件 Review | not_started |

## 3. 核验完成标准

每个模块六个维度必须分别记录：
- 生产方 Port/DTO/资源所有权；
- 每个直接消费者及真实调用点；
- 对应 Schema/Golden/Contract Test；
- 错误、诊断、Request ID 和授权传递；
- Repository/Migration/Worker 生命周期；
- 依赖方向与禁止依赖检查；
- 证据路径、符号或行范围、结论状态。

跨模块关系必须从生产方和消费者两端交叉验证。未找到消费者时，标记为“未发现消费者/待确认”，不得自动认定接口正确或无用。

## 4. 关键阻塞规则

- 未完成生产代码全量 import 扫描，不宣布依赖边界核验完成。
- 未完成核心 Port 的消费者追踪，不宣布接口核验完成。
- 未确认授权资源 Owner 和读取时复核，不宣布安全核验完成。
- 未核实 Worker 注册、启动、停止和故障退出，不宣布生命周期核验完成。
- P0 未解决或未裁决，不进入冻结建议。
- 三轮 Review 必须分别形成独立记录。


## 5. 本轮增量记录

### Platform — 维度 3：Fault、Diagnostics、Request ID

- 详细记录：[`Platform-维度3-Fault-Diagnostics-RequestID.md`](./Platform-维度3-Fault-Diagnostics-RequestID.md)
- 核验代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 结论：Platform 这三类基础原语的实现与单元测试整体符合已审查的公共契约；本结论不代表各领域模块和 Server 已正确接入。
- 发现一项 P2 文档一致性问题：公共平台契约第 7.2 节对 Check 最小字段的描述，与第 21 节明确收敛后的实际 Go API 不一致，需由 Platform 文档责任方统一表述。
- Request ID 的旧兼容 Middleware 仍会信任单值入站 Header；新接入必须使用零值安全的 MiddlewareWithOptions。真实 Server 调用点尚未核验，列入后续 Server 维度 3。


### Platform — 维度 1：Application Port 与真实调用点（初审）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- 核验代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 结论：Platform 是技术原语层，不应定义统一领域 Application Port；HTTP Client、Diagnostics 等能力通过标准 Go interface、配置、构造函数和回调供调用方组合。
- 发现 AUD-PLAT-PORT-001：`internal/platform/taskqueue` 直接依赖旧 `internal/analysis/domain`，违反 Platform 不依赖领域包的边界原则，登记为 P1 候选，等待旧 Analysis 迁移/Server 责任方裁决。
- 消费者调用点、Server 装配入口、Data 独立 Module 接入方式和全量 Platform import 扫描尚未完成；维度 1 保持 partial。

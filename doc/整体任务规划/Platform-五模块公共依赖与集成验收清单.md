# Platform → 五模块公共依赖与集成验收清单

- 所属治理任务：GOV-004
- 状态：依赖边界与首期可复用能力清单；不是 Platform API 永久冻结声明。
- 规划分支：`feat/design_doc`
- 实现归属：`feat/platform-foundation`
- 核查说明：本轮将 Platform 分支 ref 与既有核查点 `a30ee12130d2a50bc3db672d9d12d1e14040e34f` 比较，当前分支 ahead 2 个提交；变化涉及 Platform 配置文档、dotenv 实现/测试和 security 测试。未发现改变既有 Fault、Diagnostics、HTTP Client、Storage、Task Queue 职责边界的证据。当前不能仅凭比较结果宣称真实部署集成已验收。

## 1. 目标与原则

五模块（Quant、Research、Researchflow、LLM、Notification）只依赖 Platform 的技术原语，不把领域模型、领域策略或业务历史交给 Platform。

- Platform 是根 Go Module 内部公共包，不是独立服务或微服务。
- 领域模块依赖 Platform 时，通过稳定、窄接口使用，不反向要求 Platform import 领域包。
- 不建设共享万能 Domain、Repository、Task、Config 或 Diagnostics History。
- Platform 包级测试通过只证明包级行为，不代表服务入口、真实 DB/Redis、生产网络出口和领域 Worker 已完成集成。
- Data 位于独立 Go Module，禁止 import 根模块 `internal/platform/**`；Data 通过 HTTP/OpenAPI 和自己的 Adapter 使用数据服务契约。

## 2. 包级依赖清单

| Platform 包 | 五模块可依赖的技术能力 | 领域模块必须自行负责 | 状态/限制 |
|---|---|---|---|
| `internal/platform/config` | `Env` 类型化读取、默认值解析、Secret 脱敏、dotenv 本地辅助装载 | Quant/Research/Researchflow/LLM/Notification 各自的 Config struct、Validate、Provider/业务配置 Schema | dotenv 不覆盖进程已有环境变量；生产 Secret 由部署注入；不提供动态配置中心 |
| `internal/platform/security` | owner/service 身份基础、可信调用上下文、路由访问模式基础 | 用户/租户体系、模块内业务授权、资源级权限 | Request ID 不是身份；不得以 request_id 授权 |
| `internal/platform/observability` | Request ID Middleware、Context 读写、出站传播、标准 slog 约定 | Task ID、业务审计主体、领域事件/证据关联 | Gateway 信任边界需在部署层确定；后台 Worker 需自行持久化并恢复创建时 Request ID |
| `internal/platform/fault` | 稳定 Class/Code、安全摘要、Dependency、Retryable、内部 Cause | 领域错误语义、质量状态、业务重试/补偿策略 | Cause 不序列化；SafeMessage/Code/Dependency 不自动脱敏，调用方必须提供安全值 |
| `internal/platform/diagnostics` | Check、Observation、有限并发 Runner、Timeout/Deadline、基础聚合 | 各领域诊断规则、状态细分、历史记录、证据存储、诊断授权策略 | Runner 不是领域工作流；只适合只读、有界检查 |
| `internal/platform/httpclient` | 受控目标地址策略、DNS/IP 校验、受控 Dial、重定向策略、超时/响应大小/并发限制、有限幂等重试 | Provider 配额、来源授权、API 业务错误、Provider 熔断与质量判断 | 必须绑定固定 DestinationPolicy；生产出口安全仍需部署环境验收 |
| `internal/platform/storage` | SQL/Redis 连接生命周期、Ping、Migration Runner | Repository、SQL、表结构、事务边界、Migration 清单、保留策略 | 真实 MariaDB/Redis 集成与故障演练未完成前，不可标记生产验收完成 |
| `internal/platform/httpapi` | 当前历史 HTTP transport 的存在需谨慎处理 | 新模块 API 的 Handler/DTO/路由与认证策略 | 当前实现有历史 Analysis 耦合，不作为五模块新 API 的稳定通用依赖 |
| `internal/platform/taskqueue` | 现有历史任务执行代码，仅作迁移参考 | Researchflow Run/Stage/Attempt、DAG、Worker、恢复、幂等和调度状态 | 直接耦合 `internal/analysis/domain`，禁止五模块直接依赖为通用队列 |

## 3. 推荐的五模块使用矩阵

| 能力 | Quant | Research | Researchflow | LLM | Notification |
|---|---|---|---|---|---|
| Config / Secret | 使用 | 使用 | 使用 | 使用 | 使用 |
| Security trusted context | 按需 | 按需 | 使用 | 不承载工具授权 | 使用 |
| Request ID / slog | 使用 | 使用 | 使用 | 使用 | 使用 |
| Fault | 使用 | 使用 | 使用 | 使用 | 使用 |
| Diagnostics Check | 领域 Adapter 暴露只读检查 | 领域 Adapter 暴露只读检查 | 可组合模块检查 | 暴露模型服务能力检查 | 暴露渠道能力检查 |
| HTTP Client | Data Adapter 调用 | 不直接调用 Data；若有外部源须经 News | 通常调用内部 Application Facade | 调用外部模型 Provider | 调用外部投递渠道 |
| Storage | 自有 Repository/Migration | 自有 Repository/Migration | 自有 Run/Stage/Attempt 存储 | 仅必要调用元数据 | 自有 Subscription/Delivery 存储 |
| Platform Task Queue | 禁止 | 禁止 | 禁止作为 DAG/Run 引擎 | 禁止 | 禁止 |

### 3.1 依赖方向约束

允许：
- Quant → Platform；
- Research → Platform；
- Researchflow → Platform + 各模块公开 Application Facade；
- LLM → Platform；
- Notification → Platform。

禁止：
- Platform → 任一业务模块；
- LLM/Notification → Quant 或 Research Domain；
- Quant ↔ Research；
- Research → Quant Go 包；
- 任何模块通过 Platform 包间接获取其他模块私有 Domain/Repository；
- 根模块任一包 import Data 独立 Module 的 internal 包。

## 4. 可视为首期稳定技术契约的内容

### 4.1 Fault

公开边界只允许依赖：
- 稳定错误类别：validation、unauthenticated、forbidden、not_found、conflict、unavailable、timeout、internal；
- 稳定 Code；
- 安全摘要 SafeMessage；
- 可选 Dependency；
- 可选 Retryable；
- 内部 Cause（不可对外序列化）。

调用约束：
- 领域错误码由领域模块命名空间管理。
- Dependency 使用逻辑依赖名，不写 URL、Host、DSN 或凭据。
- Retryable 只是技术提示，不代表业务可以无条件重试。
- 禁止将原始 `err.Error()` 写入 SafeMessage 或 API response。
- HTTP 状态映射由 transport 层负责。

### 4.2 Diagnostics

公共执行接口只承载：
- Check：ID、Name、Required、Timeout、Run；
- Observation：status、reason_code、safe_message、checked_at、duration、evidence_ref、attributes；
- Runner：总 Deadline、有限并发和独立检查执行。

约束：
- Check 只读、可取消、有明确成本上限；真实写入、通知、高成本全量扫描不能伪装成普通 Check。
- required 依赖缺失不能 skipped；optional 未配置可 skipped 且必须有 reason。
- Platform Observation 不替代领域状态。Data Probe 的 passed/not_supported/not_configured 等状态必须由 Data Adapter 显式映射并保留原状态引用。
- attributes 是有界低敏感结构化标量，不承载正文、snippet、原始响应或任意 JSON。
- Platform 不建设通用诊断历史数据库。

### 4.3 HTTP Client

调用方必须：
- 为每个外部服务/Provider 定义固定 DestinationPolicy；
- 将 Host、Port、协议、重定向规则和允许的网络目标限制在策略中；
- 使用 Context Deadline 和响应体大小上限；
- 仅对幂等且请求体可重放的请求有限重试；
- 将授权、配额、Provider 质量和业务状态留给领域 Adapter。

安全验收：
- 单测通过不代表生产出口 SSRF 门禁通过。
- 部署验收需覆盖 DNS rebinding、解析到私有/保留地址、重定向跨 Host、代理环境变量绕过、IPv4/IPv6、超大响应、超时和取消。
- News 在线采集启用前必须完成部署出口验证。

### 4.4 Storage / Migration

Platform 只负责：
- SQL/Redis 初始化和生命周期；
- Ping/关闭；
- Migration Runner 执行原语；
- Migration namespace/version 的执行隔离。

领域模块负责：
- 自己的 Repository 接口与实现；
- SQL、表结构、索引、约束和事务；
- Migration 文件/清单；
- 数据保留、清理和审计；
- 真实集成测试与恢复演练。

硬约束：
- MariaDB 是事实存储；Redis 只作缓存/短期辅助，不保存唯一事实。
- Migration 版本不可变；修复新增更高版本。
- 各模块使用独立 namespace：quant、research、researchflow、notification；News/Data 也各自拥有命名空间。
- 总注册和启动执行入口由服务端集成分支装配；模块分支不得提前修改根 DI、Migration 总注册、HTTP 路由或启动入口。
- 真实 MariaDB/Redis 集成、Migration 失败恢复和 Redis 故障演练通过之前，Storage 不能作为生产就绪依赖。

### 4.5 Config / Secret / Request ID

- 环境变量只在 bootstrap/config 装载边界读取。
- dotenv 仅本地辅助，不覆盖进程环境；生产 Secret 从部署环境注入。
- Secret 明文只能在构造认证客户端等明确边界显式读取，不得写日志/错误/诊断。
- Request ID 只用于关联，不是身份或权限。
- 公网服务若不信任外部 X-Request-ID，应由可信 Gateway 覆盖/清理后再传入。
- 后台任务持久化创建时 Request ID，Worker 在执行入口显式恢复。

## 5. 当前未通过的集成验收

| 验收项 | 当前结论 | 通过条件 | 责任归属 |
|---|---|---|---|
| Platform 包级测试/race/vet | 有专项通过记录 | 后续改动持续通过 | Platform |
| 真实 MariaDB 连接与事务 | 未完成生产集成验收 | 真实服务环境连接、事务、超时、断连恢复 | Platform + Server |
| 真实 Redis 连接与故障 | 未完成生产集成验收 | 断连、重连、超时、缓存降级 | Platform + Server |
| Migration Runner 故障演练 | 未完成 | 重复执行、部分失败、版本冲突、启动失败恢复 | Platform + Server |
| HTTP Client 生产出口安全 | 未完成 | 部署网络 SSRF 攻击矩阵与 DNS/重定向验收 | Platform + 部署 |
| Diagnostics 路由接入 | 未完成 | owner/service 鉴权、公开最小信息、敏感字段过滤 | Server |
| Diagnostics 领域 Adapter | 未完成全量接入 | Data/News/Quant/Researchflow/LLM/Notification 各自映射 | 各领域 |
| Request ID Worker 恢复 | 原语已有，领域接入待验收 | Run/Task 创建时持久化，执行日志恢复 | 各领域 |
| Fault HTTP 映射 | 需 Server 集成验收 | 不泄露 Cause/SQL/DSN/Token，状态映射稳定 | Server |
| 统一 DI/路由/Migration 注册 | 集成阶段任务 | 根服务只在集成分支装配，模块边界无循环依赖 | Server |

## 6. 对五模块开工的限制

可以在模块内部先设计和实现自己的 Port/Adapter，但必须遵守：
- 不依赖 Platform 尚未稳定的内部实现细节；
- 不直接修改 Platform 包；
- 不在模块分支修改根 DI、HTTP 路由、cmd、全局 Migration 注册；
- 对真实 Storage/HTTP 安全尚未通过的依赖使用明确启动门禁，不允许降级成“成功但无数据”；
- 不把 Platform Task Queue 作为 Researchflow 基础；
- 不把 Platform Diagnostics Observation 当作领域质量结论；
- 不把 Request ID 当作安全主体。

如果模块实现中发现 Platform API 缺少真实调用方所需的公共能力，先提出最小接口变更并在 Platform 分支评审，不在模块分支复制一套平台实现。

## 7. GOV-004 Platform 通过条件

- [ ] 五模块的 Platform 依赖只覆盖本清单允许的技术原语。
- [ ] Fault 对外字段安全、稳定，Cause 不外泄。
- [ ] Diagnostics Check/Observation 状态映射保留领域语义。
- [ ] HTTP Client 固定 DestinationPolicy，生产出口 SSRF 门禁通过。
- [ ] Storage 真实 MariaDB/Redis 集成通过。
- [ ] Migration 重复执行、失败恢复和版本冲突测试通过。
- [ ] Request ID 在入口、出站调用和 Worker 之间按信任边界传播。
- [ ] 历史 Analysis HTTP transport 的领域耦合不被新模块继承。
- [ ] Platform Task Queue 不作为 Researchflow DAG/Run 引擎。
- [ ] 所有未通过的集成验收有明确 Owner、前置依赖和阻断条件。

## 8. 当前结论

Platform 已提供可复用的基础技术包，但“包存在/包级测试通过”不等于五模块可直接认为所有平台能力已生产就绪。五模块可以按本文件使用稳定技术原语；真实 Storage、Migration、生产出口安全、Diagnostics 路由/领域 Adapter 和 Worker Request ID 接入仍需各自验收。

本文件不修改 Platform 代码，不将未验收能力描述为完成；GOV-004 仍未冻结。

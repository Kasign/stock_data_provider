# GOV-004 最终 Review 与契约冻结门禁

- 所属治理任务：GOV-004
- Review 日期：2026-11-05
- 规划分支：`feat/design_doc`
- 结论：**未通过冻结门禁（NO-GO）**。
- 本文是当前审查状态与剩余门禁的唯一结论，不代表 Data、News、Platform 已完成实现验收。

## 1. Review 范围

GOV-004 负责冻结五模块之间的公开契约、Schema、时间/质量语义、认证边界、Repository/Migration 所有权和依赖方向。

本轮已完成规划侧的补充审查：
- Data：OpenAPI 与 Go JSON 字段/序列化语义的差异清单、双向契约测试要求；
- News：Research 只读证据查询 Facade、授权决策引用、PIT 和稳定分页目标契约；
- Platform：五模块可依赖包、禁止依赖和真实集成验收清单；
- Data 服务间认证：Data `X-API-Key` 与 Platform Service Secret 的边界；
- 五模块的 Repository/Migration 归属和 Go import 方向。

## 2. 已可作为稳定设计基线的内容

以下是架构和语义层面已形成一致结论的内容；可继续作为模块设计约束，但不等于所有字段级 DTO 已冻结。

### 2.1 模块职责与依赖方向

- Quant：确定性数值计算、指标/因子/趋势/市场/行业/个股、策略/回测/风险/评估。
- Research：Context、Evidence、Event、Claim/Hypothesis、Scenario。
- Researchflow：Run/Stage/Attempt、DAG、Workflow Template、编排、恢复和结果综合；唯一负责组合模块。
- LLM：模型协议和 Adapter，不执行工具、不负责研究编排和领域持久化。
- Notification：订阅、模板、渠道、Dispatcher、幂等和投递历史，不负责告警条件计算。
- Quant 与 Research 不互相依赖；Research 不 import Quant。
- LLM、Notification 不依赖 Quant/Research/Researchflow。
- Researchflow 组合各模块 Application Facade。
- Platform 不反向依赖任何业务模块。
- Data 独立 Go Module 通过 HTTP/OpenAPI，禁止跨 Module import internal。

### 2.2 时间、质量与数据语义

- 统一区分来源/事件时间、available_at、observed_at、fetched_at、normalized_at、system_as_of。
- 历史 Run 固定 system_as_of；先按系统知识时间过滤，再按业务有效时间解析。
- 价格 CNY/股、成交量 shares、成交额 CNY；百分比按 Data 约定使用百分点。
- 复权、来源、版本、质量状态、fallback、单位和时间元信息必须保留。
- unknown/null 不得被转换为 0/false/空字符串/当前时间。
- HTTP 200 不代表数据 valid；Data Probe/Validation/Freshness/Coverage 的权威所有权仍归 Data。
- Risk unknown 不得当作 pass；硬风险门禁不能被 LLM 或评分覆盖。

### 2.3 持久化与基础设施所有权

- 每个领域模块自有 Repository、SQL、表结构、Migration、保留策略和领域历史。
- Platform 只提供 SQL/Redis 生命周期、Ping、Migration Runner 等技术原语。
- MariaDB 是事实存储；Redis 只作缓存/短期辅助。
- 模块分支不提前修改根 DI、HTTP 路由、cmd、Migration 总注册入口。
- Platform Task Queue 不作为 Researchflow DAG/Run/Attempt 引擎。

## 3. 阻断冻结的事项

### 3.1 Data OpenAPI ↔ Go JSON

<badge color="danger">阻断项</badge>

已知差异已整理至 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md)。

未通过原因：
- 15 个 Go JSON 字段缺口尚未由 Data 分支修正并验证；
- required 差异字段清单与摘要数量需要自动化重算；
- nullable 差异的旧摘要数量与逐字段列表不一致；
- 9 个 OpenAPI 多余字段需要证明实际 Handler 输出来源或删除；
- Go JSON Tag ↔ OpenAPI 双向自动化契约测试尚未提供通过证据；
- 枚举、类型、精度、格式、范围、嵌套引用仍需全量复核。

通过条件：
1. Data 分支从最新 Go Model/Handler/OpenAPI 自动生成双向差异报告；
2. 所有差异修复或登记为有明确理由的例外；
3. Handler JSON golden、required/nullable、枚举和嵌套 Schema 测试通过；
4. OpenAPI lint、Go 测试、契约测试和 CI 全部通过；
5. 提交 SHA 和 CI 结果可精确关联。

### 3.2 News ResearchEvidenceQuery 与持久化/PIT

<badge color="danger">阻断项</badge>

目标契约已整理至 [News → Research 只读证据查询契约](./News-Research只读证据查询契约.md)。

未通过原因：
- 当前没有 GetEvidence/ListEvidence 只读 Facade；
- News Repository/Migration 尚未形成完整持久化闭环；
- Observation、Version、SecurityIdentity、IndustryClassification、Relation 缺完整 system_as_of 查询过滤；
- 当前 PolicyDecision 不包含不可变策略版本/授权快照引用；
- available_at/fetched_at/normalized_at 尚无完整、可验证的模型来源；
- 稳定分页快照与游标一致性未实现；
- 内容引用与按用途/内容级别的授权读取未实现。

通过条件：
1. News 实现公开 Application DTO/Facade，Research 不访问 News 私有包；
2. Repository/Migration 和真实 MariaDB 集成通过；
3. 双时间轴过滤、历史关系解析和版本选择测试通过；
4. 授权决策快照、当前撤销检查和内容分级访问通过；
5. 稳定分页并发测试通过；
6. Research Adapter 只依赖 Facade，不跨模块读取 SQL/Domain。

### 3.3 Platform 真实集成验收

<badge color="danger">阻断项</badge>

目标清单已整理至 [Platform → 五模块公共依赖与集成验收清单](./Platform-五模块公共依赖与集成验收清单.md)。

未通过原因：
- 真实 MariaDB/Redis 集成未完成；
- Migration 重复执行、失败恢复、版本冲突和回滚/恢复演练未完成；
- HTTP Client 生产出口 SSRF 攻击矩阵未完成；
- Diagnostics 路由权限和各领域 Adapter 接入未完成；
- Worker Request ID 恢复、Fault HTTP 安全映射尚需 Server 集成验收。

通过条件：
1. 真实 MariaDB/Redis 环境集成通过；
2. Migration 故障演练和恢复策略通过；
3. 部署出口 DNS/IP/重定向/代理绕过/IPv4/IPv6 安全门禁通过；
4. Diagnostics 详细接口 owner/service 授权和脱敏通过；
5. Server/Worker 的 Request ID 与 Fault 接入通过。

### 3.4 Data 服务间认证

目标设计已整理至 [Data 服务间认证映射与 Secret 注入设计](./Data服务间认证映射与Secret注入设计.md)。

设计结论：
- Data 业务 API 当前 Operation 使用 `X-API-Key`；
- 首期 Root Server 服务端持有 Data 专用 Key，由 Data Adapter 注入 Header；
- 浏览器不接触 Data Key；
- Owner Token、Platform Service Secret、Data API Key、Provider Token 保持独立。

未通过原因：
- 这是规划设计，尚无 Server/Data 实际接入与部署 Secret 验收证据；
- Key 轮换/撤销、网络 ACL、Header 覆盖、日志脱敏和 readiness 诊断尚需验证。

### 3.5 Quant / Research 逐字段公开 DTO

<badge color="warning">待补齐</badge>

当前已冻结语义与所有权，但没有完成所有公开 DTO 的字段级 Schema 表。

需要逐项确认：
- Quant Data Adapter DTO 与 Data OpenAPI 之间的字段映射；
- QuantResultRef、QuantMetric、RiskDecision 的精度、单位、周期、as_of、质量和版本字段；
- Research Evidence/Context/Event/Claim/Hypothesis/Scenario 的引用、证据类型、授权和时间字段；
- Research 与 Quant 的结果传递必须经 Researchflow，不增加 Research → Quant import；
- 所有 DTO 的 required/optional/nullable、枚举、版本策略和兼容规则。

通过条件：公开 DTO 字段表、示例 JSON、Schema 校验、版本策略和模块间契约测试齐备。

## 4. GOV-004 当前状态表

| 审查项 | 设计审查 | 实现/集成验收 | 是否阻断冻结 |
|---|---|---|---|
| 模块职责与依赖 DAG | 完成 | 后续代码静态检查 | 否（持续门禁） |
| Data 输入语义/PIT/单位/质量 | 完成重点审查 | Data Schema 与自动化测试未通过 | 是 |
| Data OpenAPI 字段/序列化 | 差异清单完成 | 未修复/未通过双向测试 | 是 |
| News 领域边界与 Evidence | 目标契约完成 | Facade/Repository/PIT 未实现 | 是 |
| Platform 包职责边界 | 完成 | 真实集成/安全验收未通过 | 是 |
| Data 服务间认证 | 目标方案完成 | Server/Data 接入未验收 | 是 |
| Quant/Research DTO | 语义草案完成 | 逐字段契约未完成 | 是 |
| LLM/Notification 边界 | 设计基线完成 | 模块实现阶段继续验证 | 否（本阶段） |
| Repository/Migration 所有权 | 完成 | 各模块实现与集成阶段验收 | 否（持续门禁） |

## 5. GOV-004 通过标准

GOV-004 只有在以下全部条件满足时才能标记完成：
- [ ] Data OpenAPI/Go JSON 双向差异归零或所有例外经正式评审。
- [ ] Data required/nullable/枚举/类型/单位/时间 Schema 与真实 JSON 一致。
- [ ] Data 契约测试进入 CI 并通过。
- [ ] News 只读 Facade 与公开 DTO 完成。
- [ ] News Repository/Migration、双时间过滤、授权快照、稳定分页通过测试。
- [ ] Platform 真实 MariaDB/Redis 和 Migration 故障演练通过。
- [ ] Platform HTTP Client 生产出口安全门禁通过。
- [ ] Diagnostics、Fault、Request ID 的 Server/Worker 接入验收通过。
- [ ] Data Key 服务端注入、轮换、撤销和脱敏验收通过。
- [ ] Quant/Research 公开 DTO 字段表、示例和 Schema 版本策略完成。
- [ ] 跨模块 import DAG 与 Repository/Migration Owner 无冲突。
- [ ] 所有验收证据均关联精确提交 SHA 和 CI 结果。

## 6. GOV-005 前置说明

GOV-005 可以先做**规划设计**，包括：
- 共同开发基线候选 SHA；
- 五模块分支创建源；
- PR 目标和合并策略；
- 共享目录 Owner；
- 根 DI/路由/Migration 总注册入口归属；
- 跨模块集成顺序、冲突解决和验收流程。

但在 GOV-004 未通过前：
- 不得将候选 SHA 宣称为冻结共同基线；
- 不创建五个模块分支；
- 不启动依赖未冻结契约的并行实现；
- 不把 Data/News/Platform 当前分支 HEAD 自动视为共同集成点。

**最终结论：GOV-004 设计审查已有明显进展，但实现与集成门禁未通过，当前 NO-GO，不冻结，不创建模块分支。**

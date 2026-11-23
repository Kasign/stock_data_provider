# Data Owner 策略管理 HTTP API 契约提案

- 契约编号建议：SRV-CTR-003
- 版本建议：1.0.0
- 状态：DRAFT（已完成方案评审；纳入项目级登记簿并冻结前，不视为正式公开契约）
- 生产方：Root Server（Owner Session HTTP Handler、路由、依赖装配）
- 领域 Owner：Data（策略模型、校验、版本 CAS、发布审计）
- 消费方：WebUI Owner 管理页面；不供 Quant/Researchflow 作为数据读取 API 使用
- 依赖：SRV-CTR-002 1.1.0、PLAT-CTR-001、PLAT-CTR-002、PLAT-CTR-003、GOV-CTR-003、GOV-CTR-004、GOV-CTR-006

## 1. 设计目标与边界

为 Data 已有的 DailyBars v2 Snapshot 授权策略提供唯一 Root Server 管理入口。策略管理属于 Owner 管理控制面，沿用项目现有 /api/v1/owner/** 命名空间。

URL 中的 owner 表示管理控制面资源，不表示策略资源归 Owner 所有，也不意味着 Owner Session 自动获得 Data 读取权限。Owner Session 只证明浏览器 Owner 身份；Data Application 继续独立验证可信 Principal、策略内容和 CAS。

本契约只管理：
- policy_type=data_access
- policy_ref=dailybars-v2-access
- Policy Schema 1.0.0

不管理 Provider 凭据、Snapshot 内容、AuthorizationRef、授权决策记录或其他 Data 资源。Quant/Researchflow 读取数据仍必须走 Data Application 的授权读取边界，不得调用本管理 API。

## 2. HTTP 路由

| 方法 | 路径 | 成功状态 | 用途 | 身份 |
|---|---|---:|---|---|
| GET | /api/v1/owner/data/daily-bars-access-policy | 200 | 读取当前已发布策略 | owner_session |
| POST | /api/v1/owner/data/daily-bars-access-policy/versions | 201 | 创建并发布新的不可变策略版本 | owner_session |

采用 POST /versions，而不是 PUT 当前策略资源：每次发布都会新增不可变版本并 CAS 推进 current pointer；历史版本 ID 永不复用。该操作不是幂等替换，POST 语义更准确。

不提供 DELETE、历史版本覆盖、历史版本重新激活、独立撤销路由或直接修改 current pointer 的接口。撤销是一次普通版本发布，其 Policy.status 设置为 revoked。

## 3. 身份与 HTTP 安全

- 只允许 Owner Session；Owner Bearer、Service Bearer、匿名请求一律拒绝。
- 同时携带 Owner Session Cookie 与 Authorization Header 必须拒绝，不得选择其一继续处理。
- GET 执行 Owner Session、Host、Origin 校验；不执行业务状态变更。
- POST 执行 Owner Session、Host、Origin 和 Platform 原子 CSRF 校验。
- Cookie 及 CSRF 语义完全复用 SRV-CTR-002；Handler 不得自行解析或验证 Session。
- 所有响应设置 Cache-Control: no-store、X-Content-Type-Options: nosniff。
- 所有错误按 PLAT-CTR-001 投影，不回显 SQL 错误、内部栈、Cookie、CSRF Token、Authorization Header 或请求凭据。

## 4. Policy Schema 1.0.0

请求策略对象字段：
- schema_version：固定 1.0.0
- status：approved 或 revoked
- allowed_subjects：非空、合法、排序且去重的 PrincipalRef 列表
- allowed_purposes：非空、合法、排序且去重的 Purpose 列表
- required_scopes：可为空；非空时合法、排序且去重
- effective_at：RFC3339Nano UTC
- expires_at：null 或严格晚于 effective_at 的 RFC3339Nano UTC

首版只保留 approved / revoked：
- approved：按主体、用途、Scope 白名单和生效时间判定访问。
- revoked：拒绝所有访问。

首版不定义 restricted 和 pending：当前授权 Evaluator 对 restricted 与 approved 没有独立行为，pending 也没有配套审批状态机。保留这些枚举会造成 UI/操作者误判。将来若有真正的审批流或限制维度，再通过明确的 Schema 版本升级引入。

其他规则：
1. 空主体或用途集合不产生隐式 allow-all。
2. approved 也不保证一定允许读取；还必须同时满足有效时间、主体、用途、Scope、AuthorizationRef、Snapshot 指纹和当前策略版本检查。
3. revoked 作为一个新版本发布，立即成为当前策略后拒绝所有读取；重新开放只能发布另一个全新版本并通过 CAS。
4. 未知字段、未知状态、非法 Principal/Purpose/Scope、重复或未排序集合全部拒绝。

## 5. GET 当前策略

GET /api/v1/owner/data/daily-bars-access-policy

成功 HTTP 200 的 JSON：
{
  "schema_version": "1.0.0",
  "policy_type": "data_access",
  "policy_ref": "dailybars-v2-access",
  "policy_version": "policy-v1",
  "payload_hash": "<64 lowercase hex>",
  "created_at": "2026-11-23T12:00:00Z",
  "policy": {
    "schema_version": "1.0.0",
    "status": "approved",
    "allowed_subjects": ["principal:service:quant"],
    "allowed_purposes": ["quant_analysis"],
    "required_scopes": ["data:read"],
    "effective_at": "2026-11-23T12:00:00Z",
    "expires_at": null
  }
}

- 无 current pointer：404，错误码 server.data_policy_not_published。
- MariaDB 不可用、策略记录损坏或指纹不匹配：503，错误码 server.data_policy_unavailable。
- 不生成默认策略，不把存储错误伪装成未发布。

## 6. POST 发布新版本

POST /api/v1/owner/data/daily-bars-access-policy/versions

请求 JSON：
{
  "schema_version": "1.0.0",
  "expected_current_version": null,
  "policy_version": "policy-v1",
  "policy": {
    "schema_version": "1.0.0",
    "status": "approved",
    "allowed_subjects": ["principal:service:quant"],
    "allowed_purposes": ["quant_analysis"],
    "required_scopes": ["data:read"],
    "effective_at": "2026-11-23T12:00:00Z",
    "expires_at": null
  }
}

- 首次发布时 expected_current_version=null；后续必须提供当前版本的精确字符串。
- policy_version 是不透明版本 ID，不按字典序或数字大小排序；曾经使用过的 ID 永远不可再次使用。
- Repository 在一个 MariaDB 事务内插入不可变版本、CAS 推进 current pointer、写入发布审计；任何一步失败全部回滚。
- CAS 不匹配或版本 ID 已存在：409，不留下孤立策略版本或审计行。
- 成功 HTTP 201，返回本次实际发布的版本资源；不得重新读取可能已被并发发布推进的 current pointer。

成功响应结构与 GET 相同。

## 7. 发布审计

现有 Platform SecurityAuditEvent / Server SecurityAuditWriter 只记录受限安全事件字段，无法完整表达策略版本前后关系，因此不复用其表来承载领域审计。

Data Migration V9 增加 Data 自有 append-only 发布审计记录，至少包含：
- policy_type、policy_ref
- previous_policy_version（首次发布为 null）
- new_policy_version
- payload_hash
- actor_ref（由可信 Owner Session Principal 得出）
- request_id（由 Platform Request Context 得出）
- occurred_at

审计记录与不可变版本写入、current pointer CAS 在同一事务提交。审计写入失败时整个发布失败，不能出现已生效但无法追溯的策略版本。审计表不提供 HTTP 修改/删除接口。

## 8. 错误映射

| HTTP | 错误码 | 场景 |
|---|---|---|
| 400 | server.invalid_data_policy_request | JSON、字段、策略校验失败 |
| 401 | server.session_unauthenticated | 缺失、无效或过期 Owner Session |
| 403 | Platform 标准错误 | Origin/Host/CSRF 拒绝 |
| 409 | server.data_policy_version_conflict | expected version 过期或版本 ID 已使用 |
| 404 | server.data_policy_not_published | 尚无当前策略 |
| 503 | server.data_policy_unavailable | MariaDB/策略存储不可用或记录校验失败 |

## 9. Server 组合与启动要求

- Owner Session 未启用：不注册 Owner 管理路由。
- Owner Session 已启用：Data Policy Owner Application 必须成功装配；缺失时 Server 启动失败，不允许静默跳过路由。
- Server 只负责认证、Request Context、HTTP DTO/错误映射和装配；不得复制策略规则或直接操作 Data 表。
- Handler 必须通过已认证 Request Context 调用 Data Application，保留可信 Principal 与 Request ID。
- Data Store/Migration 由 Root Server 统一资源和 Migration Registry 管理，不创建第二套连接池或独立 Data 服务。

## 10. 验收

- [ ] OpenAPI、Request/Response Schema、Golden 与实现同步。
- [ ] Owner Session only；Owner Bearer、Service Bearer、匿名、混合凭据均拒绝。
- [ ] GET Origin/Host 校验；POST CSRF 原子校验。
- [ ] no-store / nosniff。
- [ ] 未发布 404 与存储故障 503 明确区分。
- [ ] 首次发布、连续 CAS、并发 CAS、过期 CAS、版本 ID 重用、撤销、撤销后重新开放。
- [ ] 版本记录、current pointer、发布审计在同一事务；审计失败不允许部分成功。
- [ ] Server Owner Session 启用时 Data Policy Application 装配失败会阻止启动。
- [ ] 完整 Go/Race/Vet、MariaDB Migration/Repository、HTTP、Integration CI。

## 11. 未纳入首版

- 策略历史列表/分页 API；当前只需读取 current，历史追溯由 Data 审计和数据库支持，UI 暂不需要额外 API。
- 多人审批、双人复核、定时生效队列、自动回滚。
- Provider 凭据与 Provider 权限管理。
- 真实 Provider 数据准确性验证；由部署后的真实环境验收阶段执行。

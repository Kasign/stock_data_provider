# Data Owner 策略管理 HTTP 接口提案

- 状态：DRAFT，尚未冻结，不授权生产路由实现
- 归属：Server 提供 Owner Session HTTP 入口；Data 拥有策略领域语义和 Application
- 目的：为已实现的 Data Owner Policy Publisher 提供唯一、受保护的管理入口，使部署后能够显式发布/读取 DailyBars v2 Snapshot 访问策略
- 依据：SRV-CTR-002 1.1.0、DataAuthorizationPolicyOwner Application、Migration V7 不可变策略版本存储

## 1. 范围与边界

本接口只管理首批策略：

- policy_type：`data_access`
- policy_ref：`dailybars-v2-access`
- policy schema：`1.0.0`

不管理 Provider 凭据、Snapshot 内容、AuthorizationRef、策略决策记录或其他 Data 资源。

Owner Session 仅证明浏览器 Owner 身份；Data Application 仍负责验证 Principal、策略内容、版本 CAS 和授权规则。此接口不把 Owner Session 转换为 Data 资源授权，也不直接授予 Quant 读取权限。

## 2. 路由提案

| 方法 | 路径 | 用途 | 身份 |
|---|---|---|---|
| GET | `/api/v1/owner/data/daily-bars-access-policy` | 读取当前已发布策略 | owner_session |
| PUT | `/api/v1/owner/data/daily-bars-access-policy` | 发布一个新的不可变策略版本 | owner_session |

- 只允许 Owner Session；拒绝 Owner Bearer、Service Bearer、匿名和同时携带 Cookie/Authorization 的混合凭据。
- GET 必须执行 Origin/Host 与 Session 校验，但不执行策略变更。
- PUT 必须执行 Origin/Host、Session 和原子 CSRF 校验。
- 两个接口均设置 `Cache-Control: no-store`、`X-Content-Type-Options: nosniff`。
- 路由只有在 Owner Session 启用且 Data Policy Owner Application 已装配时才注册；依赖缺失不得注册一个返回成功或默认策略的占位 Handler。

## 3. GET 当前策略

成功返回 HTTP 200：

```json
{
  "schema_version": "1.0.0",
  "policy_type": "data_access",
  "policy_ref": "dailybars-v2-access",
  "policy_version": "policy-v1",
  "payload_hash": "<sha256>",
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
```

- 尚无已发布策略时返回 404，稳定错误码建议为 `server.data_policy_not_published`；不得生成默认策略。
- 存储不可用、策略记录损坏或指纹不匹配返回 503/内部安全错误，不得误报为“尚未发布”。
- 已实现明确的 not-published sentinel：Repository 将 SQL `sql.ErrNoRows` 映射为 `app.ErrDataAuthorizationPolicyNotPublished`，Application 再映射为 `ErrDataPolicyNotPublished`；存储故障仍映射为 unavailable。MariaDB 集成测试已覆盖首次发布前的 not-published 结果。

## 4. PUT 发布策略

请求：

```json
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
```

规则：

1. `expected_current_version=null` 只允许首次发布；后续必须填写当前版本的精确字符串。
2. `policy_version` 是不透明版本标识，不按字符串大小排序；已存在过的版本 ID 永远不可再次使用。
3. Policy 结构和枚举以 Data Application 当前 Validator 为准；未知字段、非法主体/用途/Scope、无效时间、重复或未排序集合均拒绝。
4. `allowed_subjects` 和 `allowed_purposes` 必须非空；`required_scopes` 可为空，但不会放宽主体/用途 allow-list。
5. `status` 仅允许 `approved`、`restricted`、`pending`、`revoked`。只有 approved/restricted 可能允许读取；pending/revoked 明确拒绝。
6. 不提供 DELETE、覆盖历史版本或回滚接口。撤销通过发布一个新的、从未使用过的版本并设置 `status=revoked`；重新开放必须发布另一个新版本并显式 CAS。
7. Data Repository 在一个事务中插入不可变版本并推进 current pointer；expected version 不匹配时返回冲突，事务回滚，不留下孤立版本。
8. 成功响应返回本次实际发布的不可变版本，不重新读取可能已被并发发布推进的 current pointer。

成功返回 HTTP 200，响应结构与 GET 相同，内容为本次刚发布的版本。

## 5. 错误映射提案

| HTTP | 错误码建议 | 场景 |
|---|---|---|
| 400 | `server.invalid_data_policy_request` | JSON、字段或策略校验失败 |
| 401 | `server.session_unauthenticated` | 缺失、无效或过期 Owner Session |
| 403 | Platform 标准错误 | Origin/Host/CSRF 不通过 |
| 409 | `server.data_policy_version_conflict` | expected version 过期或版本 ID 已使用 |
| 503 | `server.data_policy_unavailable` | MariaDB/策略存储不可用或记录校验失败 |

错误必须使用 Platform Fault 的安全投影；不得返回 SQL 错误、策略内部栈、Cookie/CSRF Token 或请求中的凭据。

## 6. 实施前置与验收

- [ ] 用户确认接口范围和 Owner Session only 认证方式后，才冻结正式契约版本。
- [ ] 增加 Data Repository 的“尚未发布”明确错误，不混淆数据库故障。
- [ ] Server Owner Session composition 注入 Data Owner Policy Application；不复制 Data Repository，也不在 Server 重写策略规则。
- [ ] Handler 通过 SessionAuthenticator 返回的 Request Context 调用 Data Application，确保可信 Principal 进入 Context。
- [ ] HTTP 测试覆盖 Owner Bearer 拒绝、混合凭据拒绝、CSRF、Origin/Host、未发布、存储故障、非法策略、CAS 冲突、版本重用拒绝。
- [ ] MariaDB 集成测试覆盖首次发布、连续发布、并发 CAS、撤销版本、历史版本不可重激活。
- [ ] OpenAPI/Schema/Golden 与 Server 路由注册同一批交付；不提前增加 WebUI 假调用。

## 7. 当前未决项

- GET 在未发布时的 404 稳定错误码需要与 Server 公共 ErrorEnvelope 词汇表核对。
- 是否需要单独的 Data 策略变更审计记录：当前 Data V7 策略版本表记录 created_at，Owner Session 安全审计记录身份生命周期；是否需要记录策略前后版本/操作事件，应先核对现有审计能力，不在本提案中新增重复审计表。
- 本提案不改变 DATA-CTR-007 deferred 状态，也不代表 Data Snapshot 已可被 Quant 生产消费。

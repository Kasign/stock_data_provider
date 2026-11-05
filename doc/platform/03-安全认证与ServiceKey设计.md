# 安全认证与 Service Key 详细设计

## 1. 安全目标与威胁模型

保护服务间 API、平台管理 API 和敏感操作，防止未授权调用、权限越界、凭证泄露、身份伪造、重放和审计缺失。首期以当前单体/少量内部服务部署为前提，不建设完整 IAM。

主要威胁：
- 公网或不可信来源伪造内部服务请求。
- 客户端伪造 X-Service-ID、X-User-ID 等身份 Header。
- Service Key 泄露后被长期复用。
- 认证通过但 Scope 过大造成横向越权。
- Key 出现在日志、Trace、错误、URL、CI 输出或配置回显。
- Key 轮换期间旧 Key 长期有效。
- 管理 API 被误开放公网或错误地豁免认证。

## 2. 身份类型

| Principal Type | 用途 | 建立方式 |
|---|---|---|
| user | 外部用户访问业务 API | 由实际用户认证系统或可信上游建立 |
| admin | 平台敏感管理操作 | 独立管理会话/管理凭证 |
| service | 服务到服务调用 | Service Key 认证成功后建立 |
| provider | 外部 Provider 访问凭证 | 由 Provider Adapter 的 Secret 配置持有，不作为请求主体 |
| job | 后台任务执行主体 | 从任务创建时可信保存的 Actor 快照恢复 |

Service Key 不可用于用户登录；Provider Token 不可用于内部服务认证；任务身份不得因异步执行丢失原始 Actor 和授权来源。

## 3. Service Key 格式和存储

建议明文格式包含可公开的 Key ID 和高熵 Secret，例如 sk_<环境标识>_<随机 Key ID>_<随机 Secret>。实际前缀和长度在实现前通过安全评审冻结。

- Secret 使用 crypto/rand 生成，至少 256 bit 随机熵。
- Key ID 用于索引凭证记录，不是认证秘密。
- 数据库只保存 Secret 的 HMAC-SHA-256 摘要或经评审的不可逆摘要；若采用 HMAC，服务端 pepper 必须来自部署 Secret，不能与 DB 一起保存。
- 使用常量时间比较。
- 明文只在创建或轮换时返回一次，服务端不提供再次查看接口。
- DB 唯一约束保证 Key ID 唯一；存储状态包括 active、disabled、revoked、expired。
- 保存服务身份、环境、audience、scopes、创建者、创建时间、过期时间、最后使用时间和轮换关系。
- 不记录完整 Key、Secret、Authorization Header 或摘要值到普通日志。

## 4. 认证与授权流程

    Request
      |
    Extract credential
      |
    Validate format / Key ID
      |
    Load credential metadata
      |
    Verify digest in constant time
      |
    Check active / expiry / environment / audience
      |
    Build trusted Principal in request context
      |
    Route policy lookup
      |
    Scope authorization
      |
    Handler

认证失败统一返回不泄露凭证是否存在的错误。授权失败返回权限不足，不返回 Key 元数据。任何客户端传入的身份 Header 在认证前均不可信。

## 5. Scope 设计原则

Scope 以资源和动作表达，不按 URL 字符串随意拼接。建议首批：

- platform:diagnose:read
- platform:service-key:read
- platform:service-key:manage
- platform:audit:read
- data:read、data:diagnose
- news:read、news:search、news:ingest
- research:run、research:read
- notification:send、notification:manage

最终 Scope 必须以实际路由和操作矩阵冻结。一个 Key 只获得调用方完成工作所需的最小权限；不能默认赋予 platform:* 或所有业务 Scope。

## 6. 路由策略和 Middleware

每条路由必须声明访问策略：

| 路由类型 | 认证 | 授权 | 说明 |
|---|---|---|---|
| liveness | 可匿名 | 无 | 仅表示进程存活，不检查敏感依赖 |
| readiness | 依部署边界 | 无或运维身份 | 仅返回最小依赖状态，不泄露连接信息 |
| public business | user | 对应用户权限 | 明确标记公开，不默认开放 |
| internal service | service | Scope | 服务间调用 |
| admin management | admin | 管理 Scope | Service Key 管理、配置、审计 |
| diagnostic detail | admin/service | diagnose Scope | 不向匿名调用方暴露内部错误细节 |

建议 Middleware 链：
1. Request ID / Trace Context。
2. Recovery。
3. 请求体、Header、超时限制。
4. 安全访问日志和敏感字段脱敏。
5. Authentication。
6. Authorization。
7. Rate Limit / Concurrency Limit。
8. Idempotency（适用写接口）。
9. Handler。

CORS 不是认证手段。内部路由不能仅因路径包含 internal/admin 就跳过认证。

## 7. Key 生命周期

### 创建
- 管理员通过受保护的管理接口创建 Key。
- 指定 service_id、environment、audience、scopes、expires_at。
- 服务端生成 Key，保存摘要和元数据，返回一次性明文。
- 创建动作写入审计记录。

### 轮换
- 创建新 Key，设置短期 overlap。
- 调用方部署新 Key 并完成验证。
- 观察新 Key 使用情况，确认旧 Key 不再被调用。
- 撤销旧 Key，清除缓存并审计。
- overlap 必须有明确截止时间，禁止无限期双 Key。

### 禁用/撤销
- disabled 可临时停用并允许管理员恢复；revoked 为不可恢复终态。
- 过期 Key 不可认证。
- 撤销应尽快传播；若有缓存，缓存 TTL 必须受安全要求限制。
- Key 存储不可用且无有效可信缓存时 fail closed。

## 8. 管理 API 契约草案

实际路径由服务端集成阶段确定，以下是逻辑操作，不是已存在接口：

| 操作 | 输入 | 输出 |
|---|---|---|
| 创建 | service_id、environment、audience、scopes、expires_at | credential_id、明文 key（一次） |
| 列表 | 过滤条件、分页 | 元数据，不含 Secret/摘要 |
| 详情 | credential_id | 元数据、状态和 last_used_at |
| 禁用 | credential_id、reason | 新状态 |
| 轮换 | credential_id、overlap_until | 新 Key 一次性明文和旧 Key 过渡状态 |
| 撤销 | credential_id、reason | revoked 状态 |
| 验证 | 内部诊断使用，不回显凭证 | 最小结果 |

所有管理接口要求 admin 身份、对应 Scope 和审计；禁止通过普通业务 Service Key 管理其他 Key。

## 9. 数据模型草案

service_credentials：
- id、key_id、secret_digest、service_id、environment、audience
- status、scopes_json、expires_at、created_at、created_by
- last_used_at、rotated_from_id、overlap_until、revoked_at、revoked_by

约束：
- key_id 唯一。
- secret_digest 不可查询回明文。
- scopes 结构化校验，不接受任意权限字符串。
- last_used_at 更新应避免每次请求同步写 DB，可采用有界异步/批量策略，但不能因此影响撤销生效。
- 审计事件与凭证生命周期写入同一可靠事务边界，或采用可恢复 Outbox。

## 10. 安全失败策略

- 认证配置缺失：受保护路由启动失败或拒绝所有请求。
- Key Store 不可用：无有效缓存时拒绝；不能降级匿名。
- Scope 缺失或无法解析：拒绝。
- 审计写入失败：Key 创建、轮换、撤销等敏感操作失败关闭。
- 日志脱敏失败：不得回退到原始敏感内容输出。
- 外部代理身份头只有在可信代理边界内由代理覆盖后才可使用。

## 11. 安全测试矩阵

必须覆盖：有效 Key、无 Key、格式错误、未知 Key ID、Secret 错误、过期、禁用、撤销、环境不匹配、audience 不匹配、Scope 不足、身份 Header 伪造、大小写/空白绕过、路径编码、HTTP Method 绕过、并发撤销、缓存过期、Key Store 故障、审计故障、日志脱敏和轮换 overlap。

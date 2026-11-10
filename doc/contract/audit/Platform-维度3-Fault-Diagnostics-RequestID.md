# Platform — 维度 3：Fault、Diagnostics、Request ID 核验

- 固定代码基线：`origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 审计文档分支：`feat/design_doc`
- 本记录范围：仅 Platform 公共原语的实现与对应单元测试；不代表消费者接入完成。

## 1. Fault

### 实现证据

- `internal/platform/fault/fault.go`（blob `ef831c8371d1b9a0aba44e6e5d05096d49e928f3`）
  - Class 固定为 validation / unauthenticated / forbidden / not_found / conflict / unavailable / timeout / internal。
  - Fault 的所有字段均标记 `json:"-"`，要求 transport 显式映射公开 DTO。
  - Validate 校验 Class、稳定小写 Code、SafeMessage 长度/控制字符、Dependency 逻辑标识，以及 Retryable 仅允许 unavailable/timeout。
  - Cause 仅保留内部错误链。
- `internal/platform/fault/fault_test.go`（blob `5c7460978be52859fcb9f06268bd651b206be6e5`）
  - 验证 Fault JSON 不暴露内部字段。
  - 覆盖无效 Retryable、URL/IP Dependency、不稳定 Code 等校验场景。

### 判断

实现与公共契约第 4 节的已审查约束一致。注意 Validate 无法自动判断 SafeMessage 是否包含业务敏感语义；领域 Adapter/调用方仍须保证安全摘要不包含 SQL、DSN、Token、原始 Provider 响应等内容。

## 2. Diagnostics

### 实现证据

- `internal/platform/diagnostics/observation.go`（blob `3c0b351cf340a87d18434609a7afd6b0fb37fa28`）
  - 状态集合为 healthy / degraded / unavailable / skipped / failed。
  - 非 healthy 必须提供稳定 reason_code；SafeMessage、EvidenceRef、Attributes 有长度、控制字符和数量限制。
  - EvidenceRef 为可选不透明引用；Attributes 的领域白名单和敏感信息过滤明确留给领域 Adapter。
- `internal/platform/diagnostics/check.go`（blob `33f263b258a931e3d43a5b9d25ea3bdc83dddda2`）
  - Check 当前字段为 ID、Name、Required、Timeout、Run；检查函数必须遵守 Context 取消。
- `internal/platform/diagnostics/runner.go`（blob `b93f6ef40f525f86f29e29eb9250e36d2d890cdf`）
  - 检查启动前整体校验、重复 ID 拒绝、并发与总 Deadline 限制。
  - 单项错误归一化为 Fault；无效 Fault 归一化为固定 internal Fault。
  - 必需 Check 返回 skipped 会转为 failed。
  - Panic payload 不保留到 Cause。
- `internal/platform/diagnostics/aggregate.go`（blob `2edecd844d5e40574d19f9c9f693cbea15164000`）
  - 聚合优先级为 required failed > required unavailable > degraded/optional failure > healthy；全 skipped 返回 skipped。
- 测试：
  - `internal/platform/diagnostics/observation_test.go`（blob `905ba3de29c8a54e81267effbe67fccdf775685c`）
  - `internal/platform/diagnostics/runner_test.go`（blob `a9e67e3daecc76a1899d54928fbc9f06be87642c`）
  - 覆盖非健康 reason_code、控制字符、不安全 Fault、必需 skipped、可选失败降级、超时、重复 ID、总 Deadline、Panic 脱敏等。
- 契约依据：`doc/platform/08-公共平台契约规范.md` 第 4、7、14、21、24 节（integration 基线）。

### 判断

已审查实现与 PLAT-007 的状态、聚合、Fault 归一化及字段收敛规则基本一致。EvidenceRef 可选且不要求 Platform 可解析，符合证据所有权边界。Attributes 仅做通用边界校验，领域白名单/脱敏仍由 Adapter 负责，符合契约分工。

### 差异项

**AUD-PLAT-DIAG-001 — Check 字段描述存在文档内不一致**

- 严重度：P2（文档一致性，不是已证实的运行时缺陷）。
- 证据：`doc/platform/08-公共平台契约规范.md` 第 7.2 节称 Check 至少包含 ID、名称/类别、必需性、Timeout、副作用等级；第 21 节则明确 Go API 仅含 ID、Name、Required、Timeout、Run，Category、SideEffect、Dependency 尚未进入 API。
- 实现证据：`internal/platform/diagnostics/check.go` 的 Check 结构确实只有 ID、Name、Required、Timeout、Run。
- 影响：消费者可能按第 7.2 节误以为 Category/SideEffect 是当前必需字段，造成接口预期不一致。
- 建议：由 Platform 文档责任方明确第 21 节是当前实际 API 的收敛决议，并同步修订第 7.2 节的“至少包含”表述；本次审计分支不擅自修改 Platform 模块文档。
- 验收：第 7.2 节与 Go API、第 21 节一致，且不重新引入没有真实消费者的字段。

## 3. Request ID

### 实现证据

- `internal/platform/observability/request_id.go`（blob `b9e2d9ddc298ba23bead6ed1fcc9d6ec58ef707b`）
  - MiddlewareWithOptions 零值策略重新生成 Request ID，不信任客户端自报 Header。
  - 只有显式 TrustIncomingRequestID 才接受单值合法 Header；重复值或非法值重新生成。
  - 响应回写最终 ID；Context 保存已验证 ID。
  - PropagateRequestID 只从可信 Context 覆盖出站 Header；Context 无 ID 时删除调用方预设 Header。
  - 历史 Middleware 保留兼容行为并标记 Deprecated，默认信任合法单值入站 ID。
- `internal/platform/observability/request_id_test.go`（blob `a011eebf340dcf22dcfef76522246bfa0ac81b1f`）
  - 覆盖默认重生成、显式可信 Gateway、旧兼容行为、非法/重复 Header、出站覆盖与删除。
- 契约依据：`doc/platform/08-公共平台契约规范.md` 第 5 节。

### 判断

Platform 原语和单测符合 Request ID 契约。旧 Middleware 的信任行为是明确标记 Deprecated 的兼容路径，不应作为新模块接入方式。

### 待核事项

- Server/各模块是否仍在生产代码中调用旧 `Middleware`，尚未在本记录范围内检查。
- 内部 HTTP Client 是否在真实消费者调用链中统一调用 `PropagateRequestID`，尚未完成消费者侧核验。
- 异步任务的 originating_request_id / correlation_id 持久化和 Worker 传递属于领域/Server 生命周期核验，不在本次 Platform 单模块子项中宣告完成。

## 4. 本子项结论

- Platform Fault：实现与对应单测基本符合已审查契约。
- Platform Diagnostics：实现、Runner 归一化和聚合与已审查契约基本符合；发现 1 项 P2 文档一致性差异。
- Platform Request ID：原语与单测符合；旧兼容 Middleware 的消费者使用情况待 Server/模块调用点核验。
- 本子项不是 Platform 六维完成结论，也不是十模块契约冻结结论。

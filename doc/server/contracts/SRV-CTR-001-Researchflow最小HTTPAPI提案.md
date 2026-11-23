# SRV-CTR-001 Researchflow 最小 HTTP API 提案

- 状态：DRAFT，未冻结、未实现
- 适用分支：repair_all
- 提案基线：ca79ef0de11a861bdf9c412eedc42f935a82336d（代码复核基线）
- 目标：仅为首个可运行的 Researchflow Workflow 提供 Root Server HTTP 入口，不扩展成完整业务 API 平台。

## 1. 设计结论

首期只开放三项操作：

| 方法 | 路径 | 对应 Application | 用途 |
|---|---|---|---|
| POST | /api/v1/researchflow/runs | CreateRunService.CreateRun | 创建或幂等重放 Run |
| GET | /api/v1/researchflow/runs/{run_ref} | GetRunService.GetRun | 查询 Run 与 Stage 当前状态 |
| POST | /api/v1/researchflow/runs/{run_ref}/cancel | CancelRunService.CancelRun | 提交取消意图 |

不新增 Run 列表/搜索、模板 CRUD、事件流、Attempt 明细、Stage Output 内容读取、通知管理或 Quant/Research 独立 API。首期用 GET 轮询 Run/Stage 聚合状态；如后续确有页面需求，再按真实 Application 能力单独扩展。

## 2. 身份与安全

- 三个接口均要求已启用的 Root Owner Session。
- GET 使用 HttpOnly Owner Session Cookie。
- POST 创建和取消必须通过现有 SessionAuthenticator 完成 Origin/Host 与 CSRF 校验。
- 不接受浏览器 Owner Bearer，不把 Owner Session 映射为 Owner Bearer，也不把 Owner Session 直接作为 Quant 资源授权。
- Request ID 从 Platform Request Context 读取；没有可信值时由 Root HTTP 中间件生成。
- CallerRef 从已验证 Principal 解析，禁止从请求体接收 caller_ref。

## 3. CreateRun 请求

请求体严格对应现有 Researchflow CreateRunCommand，不增加服务端生成字段：

```json
{
  "schema_version": "1.0.0",
  "idempotency_key": "client-generated-stable-key",
  "template_ref": "news.search",
  "template_version": "1.0.0",
  "input_snapshot_refs": [],
  "parameters": {
    "query": "用户提交的研究问题",
    "limit": 20
  }
}
```

字段来源与规则：

- schema_version：请求 DTO 版本；首期固定 1.0.0。
- idempotency_key：客户端为一次逻辑提交生成并在重试时复用；不得每次网络重试重新生成。
- template_ref/template_version：必须精确指定已发布模板版本，不接受 latest/默认版本。
- input_snapshot_refs：必填数组；News Search 模板允许空数组。不得在 Server 伪造 SnapshotRef。
- parameters：JSON object；由模板参数 Schema 校验。News Search 首期必填 query、limit（1–100），可选 query_dimension，拒绝额外字段。

Server 不接收 system_as_of、deadline_at、caller_ref、request_id、correlation_id、run_ref 或 stage_ref。现有 CreateRun Application 负责从可信身份、MariaDB UTC Clock、IDGenerator 和配置 Deadline 生成这些值。

幂等语义沿用现有 Application：caller + CreateRun + idempotency_key 作为作用域；语义指纹绑定请求 Schema 版本、精确模板身份、规范化输入引用和 JCS 参数；同一意图返回原 Run，不同意图冲突。重放必须先于时钟读取和模板解析。

## 4. Run 查询响应

建议响应只投影现有 Run/Stage DTO，不直接 JSON 序列化 Domain 对象：

```json
{
  "schema_version": "1.0.0",
  "run": {
    "schema_version": "1.0.0",
    "run_ref": "run_xxx",
    "status": "running",
    "version": 2,
    "system_as_of": "2026-11-23T08:00:00Z",
    "deadline_at": "2026-11-23T08:30:00Z",
    "template_ref": "news.search",
    "template_version": "1.0.0",
    "template_fingerprint": "<sha256>",
    "input_snapshot_refs": [],
    "result_summary_ref": null,
    "originating_request_id": "req_xxx",
    "correlation_id": "cor_xxx",
    "termination_reason": null,
    "created_at": "2026-11-23T08:00:00Z",
    "started_at": null,
    "finished_at": null
  },
  "stages": []
}
```

- Run 字段以 doc/researchflow/schema/run-1.0.0.schema.json 为准。
- Stage 字段以 doc/researchflow/schema/stage-1.0.0.schema.json 为准。
- 终态 Run 必须带 result_summary_ref 和 result_summary；非终态不得带终态摘要。
- ResultSummary 以 doc/researchflow/schema/result-summary-1.0.0.schema.json 为准。
- DTO 必须显式投影，不能直接 marshal Domain：当前 Domain 的 ResultSummaryRef JSON tag 为 "-"，Stage 还有不属于当前 Stage Schema 的字段。
- GetRun Application 已验证 Run/Stage 归属和终态摘要一致性；Server 不重复实现这些领域规则。
- Stage 当前只返回 attempt_no、状态、输出引用、质量摘要引用、诊断引用和 reason_codes。首期不返回 Attempt 明细，因为现有 GetRun Application 聚合没有 Attempt 列表读取能力。

## 5. CancelRun

- 使用 POST /api/v1/researchflow/runs/{run_ref}/cancel。
- 不要求请求体；取消意图、版本并发和终态检查由现有 CancelRunService 处理。
- 响应复用 Run + Stages 聚合 DTO。
- 取消请求成功仅表示取消意图已持久化，不表示 Worker 已立即停止；最终状态由 Researchflow Worker/Finalizer 决定。

## 6. 错误与状态语义

HTTP Handler 只负责将 Application/Domain 错误映射为现有 Platform Fault/Server Public Error，不重新定义领域状态：

- 未登录：401。
- 已登录但无 Run 访问/取消权限：403。
- 请求 Schema/参数无效：400。
- 幂等键对应不同请求：409。
- Run 已终态或不能取消、版本并发冲突：409。
- Run 不存在：404（以 Repository 可区分的 not-found 错误为准）。
- 存储、授权依赖或内部故障：按 Platform Fault 分类映射为 5xx；不得把依赖故障伪装成 404/成功。

稳定错误码需在正式 OpenAPI/Schema 中按现有 Fault 词汇核对后冻结；本提案不先造一套与 Platform Fault 平行的错误码体系。

## 7. 首期明确不覆盖

- Attempt 逐次明细、Stage Output 内容、实时事件流和历史 Run 列表。
- Research Context/Evidence、Quant 结果、LLM 对话及 Notification Delivery API。
- 真实 A 股研究报告生成。当前首个已装配模板是 News Search；它只验收 Researchflow Create/Execute/Read 基础链，不代表完整 A 股研究链路或最终报告。
- 数据 Provider 质量或算法正确性。此类问题由相应模块诊断和修复，不改变 Run API 的状态投影。

## 8. 实施准入

正式实施前需确认本提案的路由、Owner Session 认证、请求字段和首期不覆盖范围。确认后再补正式 OpenAPI、请求/响应 Schema、Golden、HTTP Handler、Root Router 注册、CSRF/授权测试、WebUI API 封装和真实 MariaDB/Redis Worker 集成验收。

本文件在确认前仅为讨论提案，不构成 SRV-CTR-001 冻结契约。

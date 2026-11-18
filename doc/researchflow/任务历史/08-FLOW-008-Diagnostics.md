# FLOW-008：Researchflow Diagnostics

## 阶段结论

Researchflow Diagnostics 的职责是把 StageAssessment 投影为 Platform Diagnostics.Check，不是自动扫描/诊断其他模块，也不是独立 DiagnosticRecord 存储。Assessment 由真实 Stage Provider/领域 Adapter 产生；Researchflow 负责四态语义映射和安全投影。

## 已有实现

- `internal/researchflow/diagnostics/adapter.go`
- `internal/researchflow/diagnostics/adapter_test.go`
- pass -> Platform healthy
- degraded -> Platform degraded
- fail -> Platform failed
- not_comparable -> Platform degraded，并通过 `assessment_status=not_comparable` 保留原始语义
- `CheckedAt` 在 Adapter 边界规范化 UTC。
- Attributes 采用 allowlist：当前只允许稳定代码 `source`；禁止自由文本、未知 key 和 Adapter 保留字段（assessment_status、diagnostic_kind、stage_ref、stage_code）。
- SafeMessage 必须由生产者提供安全摘要，不得直接传入 Provider/LLM 原始错误；EvidenceRef 保持不透明引用。

## 2026-11-18 Review 结论

Platform `Observation.Validate` 负责结构、长度和控制字符校验，并明确要求领域 Adapter 负责属性白名单及敏感信息过滤。Researchflow Adapter 原先仅做数量限制，未限制 Attribute key/value；同时未将 CheckedAt 规范化 UTC。本轮已补齐：

1. CheckedAt.UTC()，保证 Platform Diagnostics 输出符合项目 UTC Z 规则。
2. Attributes 仅接受显式 allowlist key `source`，且 value 必须是稳定小写代码。
3. 拒绝 Adapter 保留字段，防止调用方覆盖 assessment_status、diagnostic_kind、stage_ref、stage_code。
4. 新增 +08:00 CheckedAt 输出 Z、未知/敏感属性、保留字段和自由文本属性拒绝测试。

代码验收 SHA：`9ec3ebdbdf79cf548f98c438019b5aff2e2dc929`；Actions Run 44296700307 中 Researchflow module gofmt/tests/race/vet Job 已 success。

## 未完成边界

- 当前未发现生产代码调用 `NewStageCheck`；Server bootstrap/composition 尚未注册 Researchflow Diagnostics.Check。
- 当前未发现真实 StageAssessment 生产者；各领域 Stage Provider 需根据真实数据和诊断事实生成 Assessment。
- 不伪造独立 DiagnosticRecord，不把 Provider Payload 或 QualitySummary 填成 DiagnosticRefs。若未来需要诊断历史，先冻结 Record Owner、生命周期、EvidenceRef 和幂等契约。
- Stage Evidence 与 Platform Diagnostic Record 保持分离；本 Adapter 不复制、不覆盖领域证据。

## 职责边界

- Researchflow：Assessment 四态语义、Stage identity、ReasonCode/SafeMessage/EvidenceRef 的安全投影。
- Data/News/Quant/Research/LLM Stage Provider：产生有来源证据的真实 Assessment，不传原始错误/敏感数据。
- Platform：Diagnostics.Check/Observation/Runner/Report 公共结构和聚合能力。
- Server：Composition Root 注册真实 Checks；不替模块伪造诊断结果。

## 验收状态

- Adapter 结构/映射/UTC/属性安全：代码与模块测试通过。
- 真实 StageAssessment 生产者：待各实际 Stage Provider 接入。
- Server Diagnostics 注册与聚合：待真实 Checks 装配。
- 独立 DiagnosticRecord：当前无明确业务用例，不建设。

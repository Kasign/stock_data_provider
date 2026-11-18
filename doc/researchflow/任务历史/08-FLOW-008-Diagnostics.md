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
5. 追加 StageCode 稳定标识校验，拒绝含空格或大写字符的 StageCode；新增对应负向测试。

代码验收 SHA：`43188259c4b96579ab587742e063d0f254c4009d`；Actions Run 44299178336 中 Researchflow module gofmt/tests/race/vet、根模块 Test、WorkerStore 和相关 MariaDB 集成 Job 均 success。

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

## 当前结论

- Diagnostics Adapter 的模块内映射、UTC、安全属性和 StageCode 校验已完成并通过 SHA `43188259c4b96579ab587742e063d0f254c4009d` 的 CI。
- 真实 StageAssessment 生产者和 Server Checks 注册仍未接入；该部分由真实领域 Stage Provider 与 Server 组合根负责，不在 Researchflow 内制造模拟生产诊断。

## 2026-11-18 UTF-8 输入边界补强

复核发现 Platform Observation 对长度和控制字符有通用校验，但不能代替领域 Adapter 的 UTF-8 完整性校验。无效 UTF-8 字符串可能在 JSON 序列化时被替换，造成诊断字段失真。

本轮在 Researchflow Adapter 增加以下拒绝条件：

- StageRef、StageCode、ReasonCode、SafeMessage、EvidenceRef 必须是有效 UTF-8。
- Attributes 的 key/value 必须是有效 UTF-8；原有 allowlist、稳定代码和保留字段约束保持不变。
- 新增 7 组无效 UTF-8 负向测试，覆盖上述字段及属性键值。

代码 SHA：`384582d37f679d272de708b87b32a6b4c93cf649`。GitHub Actions Run [44309745636](https://github.com/Kasign/stock_data_provider/actions/runs/44309745636) 中 Researchflow module format/tests/race/vet、WorkerStore unit/race/vet、MariaDB lifecycle integration 均 success；根模块 Test 当时仍在执行 Race Detector，不能据此宣称根模块全量 CI 完成。

该补强只收紧 Researchflow Diagnostics Adapter 的输入边界，不修改 Platform 公共实现，也不代表真实 StageAssessment Provider 或 Server Diagnostics 注册已经完成。

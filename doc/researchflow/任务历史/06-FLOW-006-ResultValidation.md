# FLOW-006：Result Composer/Validator

## 阶段结论

Result Composer/Validator 的纯函数层已实现；本轮发现并修复了 Researchflow 与 Quant Envelope 的字段/版本模型不一致。当前代码能按 Quant 的真实 Envelope 1.0.0 结构映射并验证 trend/risk Payload，但生产 Facade/Server 接线及完整结果持久化仍未完成。

## 已有证据

- Researchflow：`internal/researchflow/resultvalidation/**`
- Quant Envelope：`internal/quant/result/envelope.go`
- Quant Application：`internal/quant/application/get_result.go`
- Quant Schema：`doc/quant/schema/QNT-CTR-004-quant-result-envelope-1.0.0.schema.json`、`QNT-CTR-004-quant-result-ref-1.0.0.schema.json`
- 当前代码验收 SHA：`43188259c4b96579ab587742e063d0f254c4009d`；Actions Run 44299178336 对该精确代码 SHA 的整体结论为 success；Researchflow module gofmt/tests/race/vet、根模块 Test 和相关 MariaDB/WorkerStore 集成 Job 均通过。

## 2026-11-18 Review 发现与修复

原实现存在四项具体不一致：

1. 将 Quant Envelope 的 `schema_version` 当成 Payload Schema 版本；实际 Envelope 另有必填 `payload_schema_version`。
2. Researchflow Envelope DTO 带有 `created_at`，但 Quant Envelope 不含该字段；`created_at` 属于 QuantResultRef。
3. Researchflow 预先允许 indicator/factor/market 等类型，但 Quant Envelope 1.0.0 的 result_type 联合目前仅支持 trend/risk。
4. 测试使用 `qres-1` 和 indicator 假数据，不符合 Quant 的内容寻址 ResultID 与真实 Envelope 联合。

修复结果：

- QuantResultEnvelope 现在区分 Envelope SchemaVersion 与 PayloadSchemaVersion，并移除 CreatedAt。
- ResultID 校验为 `quant_result:sha256:<64位小写hex>`。
- Registry 精确键为 `result_type + payload_schema_version`，当前只注册 trend/risk 1.0.0。
- 默认 Validator 复用 Quant `DecodePayload`，不复制 Quant Payload Schema。
- 新增 `FromQuantEnvelope`，先执行 Quant Envelope.Validate，再映射内部 DTO；它不执行授权，必须在 Quant Application GetResult 当前授权和 Manifest 校验之后调用。
- Composer 测试使用真实 Trend Payload 结构和内容寻址 ResultID；Run、Quant、Research、LLM 测试 PIT 使用同一 SystemAsOf。
- 规范 UTC 检查要求 `time.UTC`，拒绝零偏移但非 UTC Location 的时间对象。

## 未完成边界

- 当前没有 Server/Worker 生产调用 Result Composer；ResultValidation 仍是纯函数/DTO 校验层。
- 需要通过 Quant GetResult、Research Context/Evidence/Claim Facade 和 LLM Provider 读取数据，并在各 Owner 读取时复核当前授权。
- Run SystemAsOf/Deadline 必须端到端继承；不得重新生成 PIT 或 Deadline。
- `ComposedResearchResult` 尚无持久化 Store/ResultRef。Run `ResultSummaryRef` 是执行摘要，不是完整研究报告；完整结果唯一持久化 Owner 尚未确定。
- 真实数据、真实授权、真实模型调用和跨模块 Contract Test 未完成。

## 2026-11-18 RiskDecision 契约差异 Review

- Quant `risk.Decision` 与 Researchflow `resultvalidation.RiskDecision` 不是同一结构：Researchflow 增加了 Quant Schema 不包含的 rule_ref、observed_value、expected_range、remediation_code。
- Quant `Decision.Validate` 要求唯一 Reason Code、EvaluatedAt 不早于 SystemAsOf、Status 与 `AggregateStatus(Reasons)` 一致；Researchflow 当前仅检查 pass 不得与 block triggered/unknown 冲突，未实现相同聚合语义。
- Composer 将 Researchflow RiskDecision 作为独立输入，当前没有从 Quant `risk.Decision` 到该类型的正式映射。
- Researchflow 本轮额外加固：拒绝空 Reasons，并拒绝 `EvaluatedAt < SystemAsOf`；这两项不预设完整 Quant 聚合语义。
- Template `HardRiskGateRegistry` 目前只是 `Contains(stageCode)` 存在性接口，不负责执行 Gate 或产生 RiskDecision；没有生产 Gate Provider 证明 Composer RiskDecision 的真实来源。
- 在确认该对象属于 Quant 投影还是独立编排层 HardRiskGate 结果之前，不做生产映射、不擅自修改硬风险聚合规则。
- 代码验收 SHA `132415777bdfcd81f6d4cf2f812dfbefbd225dde`；Actions Run 44300750509 整体 success，Researchflow module 与根模块 gofmt/tests/race/vet 均通过。卡点见 `doc/整体任务规划/Researchflow模块卡点.md` 的 RF-BLOCK-008。
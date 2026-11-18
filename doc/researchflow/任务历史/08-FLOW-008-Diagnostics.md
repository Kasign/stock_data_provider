# FLOW-008：Researchflow Diagnostics

## 阶段结论

Researchflow StageAssessment 已映射至 Platform Diagnostics，支持 pass/degraded/fail/not_comparable 四态；not_comparable 投影为 Platform degraded，并通过 assessment_status 保留原始语义。

## 已有证据

- `internal/researchflow/diagnostics/adapter.go`
- `internal/researchflow/diagnostics/adapter_test.go`
- Stage Evidence 与 Platform Diagnostic Record 分离的项目级 Review 决议见 Integration 交接记录。

## 未完成边界

当前不伪造独立 Diagnostic Record，不把 Provider Payload 或 QualitySummary 填成 DiagnosticRefs。真实领域诊断事实生产者随 News/Data/Quant/Research/LLM 接入补齐；如需独立诊断历史，先冻结 Record Owner、生命周期、EvidenceRef 和幂等契约。

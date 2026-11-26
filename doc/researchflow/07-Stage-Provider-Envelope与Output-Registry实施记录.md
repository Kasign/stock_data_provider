# Stage Provider Envelope 与 Output Registry 实施记录

## 范围

本阶段只实现 FLOW-CTR-004 已冻结字段的通用机器契约与注册框架，不改变冻结契约、不注册虚构 Stage，也不定义尚未决议的领域质量枚举或 ResultSummary coverage 口径。

## 已落地能力

- Stage Provider Result Envelope 使用独立公开结构，不复用 Domain JSON Tags。
- Provider 只能返回 `output_type`、`schema_version`、`payload`、`quality_summary`、`diagnostic_refs`；Run/Stage/Attempt/StageCode/TemplateVersion/CompletedAt 由 Orchestrator 注入。
- Envelope Schema 固定 Draft 2020-12、拒绝额外字段；payload 与 quality_summary 必须为 JSON object，且分别交给注册项对应的强类型 Validator。
- Registry 版本必须与执行模板快照中的版本精确匹配；注册项精确绑定 stage_code、output_type、schema_version、payload_schema_ref、quality_summary_schema_ref。未知组合不回退。
- Registry 注册内容排序后生成稳定 SHA-256 fingerprint；重复项、缺失 Schema 引用或 Validator 均拒绝。
- JSON 对象递归拒绝重复键，避免不同解析器对同一 Envelope 得出不同语义。
- 当前不注册任何业务 Entry。没有真实 Provider 和经确认的领域 Schema 时，空注册表只能用于框架构建，生产 Stage 仍 fail-closed。
- quality_summary 的字段语义继续由领域 Provider 定义；本框架不定义 quality_status 枚举、coverage 分母或 ResultSummary 资源身份绑定。

## 验收

- `go test ./internal/researchflow/stageoutput`
- `go test -race ./internal/researchflow/stageoutput`
- 全仓 gofmt / test / vet 与 Integration CI 应在提交后按精确 SHA 记录。
- Golden 是通用 Envelope 形状示例，不代表任何业务 Stage 已获准注册或可生产执行。

## 已装配的 Output Registry 实例（2026-11-26）

| Registry 版本 | Stage 组合 | 主要 Output | 装配位置 |
|---|---|---|---|
| News 组合版本（1.0.0 / 1.2.0） | News Search / News Feed | News Stage 输出 | `news_search_stage.go`、`news_feed_stage.go` |
| 1.1.0 | `quant.sma20` | `quant.sma20_result` 1.0.0（`QNT-CTR-005-sma20-stage-output` / `-quality-summary`） | `quant_sma20_stage.go` |
| 1.3.0 | `quant.factor` | `quant.factor_result` 1.0.0（`QNT-CTR-003-factor-stage-output` / `-quality-summary`） | `quant_factor_stage.go` |

`NewStageOutputRegistrySet` 以 Registry version 为唯一键，新增 Registry 必须使用尚未占用的版本。`quant.factor` 使用 `1.3.0` 正是该约束的结果：`1.0.0`/`1.2.0` 已属 News、`1.1.0` 已属 SMA20。`quant.factor` 的参数 Schema 为 `doc/researchflow/schema/quant-factor-parameters-1.0.0.schema.json`，`factor_code` 只做 Stage 层封闭分派，不透传给 Quant Facade。

## 后续

1. 各领域模块提供真实 Provider Port、payload Schema/Golden 和质量摘要 Schema/Validator。
2. 契约 Owner 对 ResultSummary 质量枚举、coverage 分子/分母和 Summary 资源身份/版本绑定形成独立决议。
3. Server 在存在正式 CreateRun 消费入口且 Registry/Provider 完备后装配 StageExecutor 和生命周期。

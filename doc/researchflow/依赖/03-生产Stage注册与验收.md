# Researchflow 生产 Stage 注册与验收准入

更新时间：2026-11-18  
维护分支：`feat/researchflow`

## 1. 注册前置条件

一个 Stage 只有同时满足以下条件才允许进入生产 Template/Registry：
1. 有真实调用方和明确研究场景。
2. Provider/Adapter 已实现并由正确模块 Owner 维护。
3. 输入参数 Schema 和输出 Payload/QualitySummary Schema 已冻结并有 Golden。
4. Output Registry 精确绑定 StageCode、OutputType、SchemaVersion；禁止 fallback。
5. 定义质量状态、失败原因、重试条件和不可比较条件。
6. 资源授权在 Owner Facade 读取时复核。
7. 有可追溯的来源、快照、版本和时间语义。
8. 有 MariaDB Runtime 和适用的真实 Provider/模型验收证据。

## 2. 诊断判定

| 状态 | 判定 |
|---|---|
| pass | 真实输入、授权、Schema、来源与质量规则全部满足 |
| degraded | 结果可用但存在已声明且允许的质量限制；必须保留限制和原因 |
| fail | 必需数据缺失、授权失败、Schema/身份不符、硬风险门禁失败或执行失败 |
| not_comparable | 缺少基准、版本、来源或真实数据，无法进行有效比较；不得按 pass 处理 |

skipped 仅表示适用性规则判定不执行，不代表 Provider 可用，也不属于上述诊断通过态。

## 3. 当前注册边界

- 已知正式生产注册：`news.search_candidates@1.0.0`。
- Data/Research/Quant/LLM 未具备完整生产 Port/Schema/Provider/授权材料前，不注册占位 Stage。
- HardRiskGateRegistry 只登记真正的硬风险 Gate；必须 required 且所有异常传播策略 fail closed。
- Output Registry 的 Validator 必须对应精确冻结 Schema，不允许 unknown discriminator fallback。
- Provider Payload/QualitySummary 不等于独立 Platform Diagnostic Record；DiagnosticRefs 无独立记录时允许为空。

### 当前 News Search Payload Validator 差异

- Schema `doc/news/schema/NEWS-STAGE-search-candidates-payload-1.0.0.schema.json` 要求 `candidate.missing_fields` `uniqueItems: true`。
- News 代码 `internal/news/stage/search_candidates_validation.go::ValidatePayloadJSON` 当前只校验每个值属于允许枚举，不检查重复值。
- Researchflow Stage Output Registry 直接使用该 News Validator，因此不能宣称对该 Schema 的全部约束均已执行。
- 责任方：News Owner；Researchflow 只记录跨模块准入缺口，不在 `internal/news/**` 复制领域校验。修复前该项诊断为 fail/未完全符合，不得标记 pass。
## 4. 生产验收记录最小字段

任务编号、分支 HEAD SHA、Provider/模型版本、真实数据来源、输入快照/PIT、Schema/Golden 版本、执行 Run/Stage/Attempt 引用、诊断四态、失败/降级原因、MariaDB 证据、Actions Run 和明确未覆盖范围。

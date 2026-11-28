// 浏览器 smoke 测试共用的 Researchflow 夹具（SRV-CTR-001 1.1.0）。
// 只构造契约允许的字段：Run/Stage 聚合状态与 authorized_domain_results 降级结果，
// 不伪造事件、Attempt 或完整研究报告正文。

export const QUANT_STAGE_CODES = Object.freeze([
  "quant.sma20",
  "quant.factor",
  "quant.trend",
  "quant.risk"
]);

export const RESULT_TYPES = Object.freeze(["indicator", "factor", "trend", "risk"]);

const CREATED_AT = "2026-10-30T06:20:00Z";
const SYSTEM_AS_OF = "2026-10-30";

/**
 * 构造 GET /api/v1/researchflow/runs/{run_ref} 的聚合响应。
 * stageStatuses 可覆盖各 Stage 状态；未覆盖时按 Run 终态给出 succeeded，非终态给出 pending。
 */
export function makeRunAggregate({
  runRef,
  runStatus = "succeeded",
  stageStatuses = {},
  reasonCodes = {},
  resultSummary = { total: 4, quality: "valid" }
} = {}) {
  return {
    schema_version: "1.1.0",
    run: {
      run_ref: runRef,
      status: runStatus,
      template_ref: "quant.research",
      template_version: "1.0.0",
      created_at: CREATED_AT,
      started_at: CREATED_AT,
      finished_at: ["succeeded", "partial", "failed", "cancelled"].includes(runStatus) ? "2026-10-30T06:35:00Z" : null,
      system_as_of: SYSTEM_AS_OF
    },
    stages: QUANT_STAGE_CODES.map((stageCode) => ({
      stage_ref: "stage-" + runRef + "-" + stageCode,
      run_ref: runRef,
      stage_code: stageCode,
      status: stageStatuses[stageCode] || (runStatus === "succeeded" ? "succeeded" : "pending"),
      required: true,
      reason_codes: Array.isArray(reasonCodes[stageCode]) ? reasonCodes[stageCode] : [],
      output_ref: "output-" + runRef + "-" + stageCode
    })),
    result_summary: resultSummary
  };
}

/**
 * 构造 GET /api/v1/researchflow/runs/{run_ref}/results 的降级视图响应。
 * risk 项不带 payload；indicator/factor/trend 带对象 payload。
 */
export function makeResultsResponse({
  runRef,
  resultTypes = [...RESULT_TYPES],
  resultStatus = "available",
  qualityStatus = "valid",
  includeRiskPayload = false
} = {}) {
  const payloads = {
    indicator: { indicator_code: "sma20", value: 1688.42 },
    factor: { factor_code: "momentum_5d", value: 0.021 },
    trend: { direction: "up", strength: 0.37 },
    risk: { decision: "reject" }
  };
  return {
    schema_version: "1.1.0",
    run_ref: runRef,
    resource_kind: "authorized_domain_results",
    results: resultTypes.map((resultType) => {
      const item = {
        result_id: "result-" + runRef + "-" + resultType,
        result_type: resultType,
        scope: "symbol",
        system_as_of: SYSTEM_AS_OF,
        quality_status: qualityStatus,
        result_status: resultStatus,
        authorization_ref: "authz-" + runRef,
        payload_schema_version: "1.0.0"
      };
      if (resultType !== "risk" || includeRiskPayload) item.payload = payloads[resultType];
      return item;
    })
  };
}

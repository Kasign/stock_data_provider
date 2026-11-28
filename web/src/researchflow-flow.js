// WebUI → Researchflow 主流程的纯函数契约层（SRV-CTR-001）。
//
// 边界（与后端冻结契约一致，不在浏览器侧重构任何领域语义）：
// - 创建命令严格为 create-run-request 1.1.0：schema_version / idempotency_key /
//   template_ref / template_version / input_snapshot_refs / parameters。幂等键只在 body，
//   浏览器不持有也不发送 Idempotency-Key header。
// - 首个业务链固定为 quant.research@1.0.0 单标的四 Stage；日期/窗口与 factor code
//   以保守默认填充，调用方可显式覆盖，但必须经过本层的范围校验。
// - GetRun 聚合只投影 Run/Stage 状态；SRV-CTR-001 首期没有事件/Attempt 接口，
//   本层绝不伪造事件或 Attempt 明细。
// - results 是 authorized_domain_results 受限投影，不是完整研究报告；risk 项禁止携带
//   payload。任何违背该边界的响应一律 fail-closed 拒绝。

export const QUANT_RESEARCH_TEMPLATE = Object.freeze({
  ref: "quant.research",
  version: "1.0.0",
  schemaVersion: "1.1.0"
});

export const DEFAULT_FACTOR_CODE = "momentum_5d";

const FACTOR_CODES = new Set(["ma20_distance", "momentum_5d", "momentum_20d"]);
const SYMBOL_PATTERN = /^[0-9]{6}\.(SH|SZ|BJ)$/;
const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;
const RUN_STATUSES = new Set([
  "created", "queued", "running", "cancelling", "succeeded", "partial", "failed", "cancelled"
]);
const TERMINAL_RUN_STATUSES = new Set(["succeeded", "partial", "failed", "cancelled"]);
const STAGE_STATUSES = new Set([
  "pending", "ready", "running", "retry_wait", "succeeded", "degraded",
  "skipped", "failed", "timed_out", "cancelled", "blocked"
]);
const RESULT_TYPES = new Set(["indicator", "factor", "trend", "risk"]);
const PAYLOAD_RESULT_TYPES = new Set(["indicator", "factor", "trend"]);
const MIN_IDEMPOTENCY_KEY_LENGTH = 1;
const MAX_IDEMPOTENCY_KEY_LENGTH = 128;

function fail(message) {
  const error = new Error(message);
  error.responseContractError = true;
  return error;
}

function shiftDate(dateStr, days) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateStr);
  if (!match) throw fail("日期格式无效");
  const utc = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  const shifted = new Date(utc + days * 86400000);
  return shifted.toISOString().slice(0, 10);
}

// 从 asOfDate 推导四 Stage 的保守默认窗口。所有日期均为 YYYY-MM-DD，
// 窗口上界不晚于 asOfDate；risk 的 trade_date 取 asOfDate 当日（由 Stage 侧 PIT 门禁裁决可用性）。
function defaultWindow(asOfDate) {
  const end = asOfDate;
  return {
    smaStartDate: shiftDate(end, -59),
    smaEndDate: end,
    smaLimit: 20,
    factorStartDate: shiftDate(end, -59),
    factorEndDate: end,
    factorLimit: 6,
    trendStartDate: shiftDate(end, -119),
    trendEndDate: end,
    trendLimit: 61,
    riskTradeDate: end
  };
}

function validateWindow(window) {
  const dateFields = [
    "smaStartDate", "smaEndDate", "factorStartDate", "factorEndDate",
    "trendStartDate", "trendEndDate", "riskTradeDate"
  ];
  for (const field of dateFields) {
    if (window[field] !== undefined && !DATE_PATTERN.test(window[field])) {
      throw fail("日期格式无效：" + field);
    }
  }
  const numericFields = [
    ["smaLimit", 20, "sma20"],
    ["factorLimit", 6, "factor"],
    ["trendLimit", 61, "trend"]
  ];
  for (const [field, min, label] of numericFields) {
    if (window[field] !== undefined) {
      if (!Number.isInteger(window[field]) || window[field] < min || window[field] > 1000) {
        throw fail(label + " 的 limit 超出允许范围");
      }
    }
  }
}

/**
 * 构造 quant.research 单标的 CreateRun 命令（create-run-request 1.1.0）。
 * @param {object} input
 * @param {string} input.symbol 六位代码.交易所后缀，如 600519.SH
 * @param {string} input.idempotencyKey 幂等键（只进 body）
 * @param {string} [input.factorCode] 三值 enum，默认 momentum_5d
 * @param {string} [input.asOfDate] 推导默认窗口的基准交易日 YYYY-MM-DD
 * @param {object} [input.window] 显式窗口（各字段经过范围校验）
 * @returns {object} CreateRun 命令对象
 */
export function buildQuantResearchCommand({
  symbol,
  idempotencyKey,
  factorCode = DEFAULT_FACTOR_CODE,
  asOfDate,
  window = {}
} = {}) {
  if (typeof symbol !== "string" || !SYMBOL_PATTERN.test(symbol)) {
    throw fail("标的格式无效，需为六位代码加 .SH/.SZ/.BJ 后缀");
  }
  if (typeof idempotencyKey !== "string") throw fail("幂等键必须是字符串");
  const key = idempotencyKey.trim();
  if (key.length < MIN_IDEMPOTENCY_KEY_LENGTH || key.length > MAX_IDEMPOTENCY_KEY_LENGTH) {
    throw fail("幂等键长度必须为 1–128");
  }
  if (!FACTOR_CODES.has(factorCode)) throw fail("factor_code 不受支持");

  let baseDate = asOfDate;
  if (baseDate === undefined) {
    // 无基准日期时取 UTC 当天；调用方（主流程）应传入 Run 观测交易日。
    baseDate = new Date().toISOString().slice(0, 10);
  }
  if (!DATE_PATTERN.test(baseDate)) throw fail("日期格式无效：asOfDate");

  const w = { ...defaultWindow(baseDate), ...window };
  validateWindow(w);

  const parameters = {
    stages: {
      "quant.sma20": {
        symbol,
        start_date: w.smaStartDate,
        end_date: w.smaEndDate,
        limit: w.smaLimit
      },
      "quant.factor": {
        factor_code: factorCode,
        symbol,
        start_date: w.factorStartDate,
        end_date: w.factorEndDate,
        limit: w.factorLimit
      },
      "quant.trend": {
        symbol,
        start_date: w.trendStartDate,
        end_date: w.trendEndDate,
        limit: w.trendLimit
      },
      "quant.risk": {
        symbol,
        trade_date: w.riskTradeDate
      }
    }
  };

  return {
    schema_version: QUANT_RESEARCH_TEMPLATE.schemaVersion,
    idempotency_key: key,
    template_ref: QUANT_RESEARCH_TEMPLATE.ref,
    template_version: QUANT_RESEARCH_TEMPLATE.version,
    input_snapshot_refs: [],
    parameters
  };
}

/**
 * 把 GetRun 聚合响应投影为页面状态视图。只含 Run/Stage 状态，
 * 不含事件/Attempt 字段，也不伪造任何执行明细。
 */
export function projectRunView(aggregate) {
  if (!aggregate || typeof aggregate !== "object" || Array.isArray(aggregate)) {
    throw fail("Run 聚合响应无效");
  }
  const run = aggregate.run;
  if (!run || typeof run !== "object" || typeof run.run_ref !== "string" || !run.run_ref.trim()) {
    throw fail("Run 聚合响应缺少有效 run_ref");
  }
  if (!RUN_STATUSES.has(run.status)) throw fail("Run 状态不受支持");
  if (!Array.isArray(aggregate.stages)) throw fail("Run 聚合响应缺少 stages");

  const stages = aggregate.stages.map((stage) => {
    if (!stage || typeof stage !== "object") throw fail("Stage 结构无效");
    if (stage.run_ref !== run.run_ref) throw fail("Stage 的 run_ref 与 Run 不一致");
    if (!STAGE_STATUSES.has(stage.status)) throw fail("Stage 状态不受支持");
    const reasonCodes = Array.isArray(stage.reason_codes) ? stage.reason_codes : [];
    return {
      stageRef: stage.stage_ref,
      stageCode: stage.stage_code,
      status: stage.status,
      outputRef: typeof stage.output_ref === "string" ? stage.output_ref : null,
      reasonCodes,
      required: stage.required === true
    };
  });

  return {
    runRef: run.run_ref,
    status: run.status,
    terminal: TERMINAL_RUN_STATUSES.has(run.status),
    templateRef: run.template_ref,
    templateVersion: run.template_version,
    stages
  };
}

/**
 * 把 results 响应投影为降级结果视图。严格校验 resource_kind 与 schema_version，
 * risk 项禁止携带 payload，indicator/factor/trend 必须有 payload。
 */
export function projectResultsView(response) {
  if (!response || typeof response !== "object" || Array.isArray(response)) {
    throw fail("结果读取响应无效");
  }
  if (response.schema_version !== "1.1.0") throw fail("结果读取响应 schema_version 不受支持");
  if (response.resource_kind !== "authorized_domain_results") {
    throw fail("结果读取响应 resource_kind 必须是 authorized_domain_results");
  }
  if (typeof response.run_ref !== "string" || !response.run_ref.trim()) throw fail("结果读取响应缺少 run_ref");
  if (!Array.isArray(response.results)) throw fail("结果读取响应缺少 results");

  const items = response.results.map((item) => {
    if (!item || typeof item !== "object") throw fail("结果项结构无效");
    if (!RESULT_TYPES.has(item.result_type)) throw fail("result_type 不受支持");
    if (typeof item.result_id !== "string" || !item.result_id) throw fail("结果项缺少 result_id");
    if (typeof item.authorization_ref !== "string" || !item.authorization_ref) {
      throw fail("结果项缺少 authorization_ref");
    }
    const hasPayload = Object.prototype.hasOwnProperty.call(item, "payload") && item.payload != null;
    if (item.result_type === "risk") {
      if (hasPayload) throw fail("risk 结果项禁止携带 payload");
      return {
        resultType: item.result_type,
        resultId: item.result_id,
        resultStatus: item.result_status,
        qualityStatus: item.quality_status,
        scope: item.scope,
        systemAsOf: item.system_as_of,
        authorizationRef: item.authorization_ref,
        hasPayload: false,
        payload: undefined
      };
    }
    if (!hasPayload) throw fail("indicator/factor/trend 结果项缺少 payload");
    return {
      resultType: item.result_type,
      resultId: item.result_id,
      resultStatus: item.result_status,
      qualityStatus: item.quality_status,
      scope: item.scope,
      systemAsOf: item.system_as_of,
      authorizationRef: item.authorization_ref,
      hasPayload: true,
      payload: item.payload
    };
  });

  return {
    isFullReport: false,
    resourceKind: response.resource_kind,
    runRef: response.run_ref,
    items
  };
}

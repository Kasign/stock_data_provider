// WebUI 主流程 → Researchflow 契约层的本地适配器。
//
// 职责边界：只做格式/枚举适配，不重构领域语义。
// - researchSymbol：把用户输入的六位 A 股代码补成契约层要求的 .SH/.SZ/.BJ 后缀。
// - mapRunStatusToTaskStatus：把 Researchflow Run 状态映射到页面既有任务状态集，
//   让 state/render/watchlist 骨架无需感知新枚举。
// - runStageToTaskEvent：把 projectRunView 的 Stage 投影成页面事件项；序号由调用方
//   按 Stage 顺序提供，只表达"Stage 状态"，不伪造 attempt/时间戳/执行明细。

const SYMBOL_PATTERN = /^[0-9]{6}\.(SH|SZ|BJ)$/;
const DIGITS_PATTERN = /^[0-9]{6}$/;

const RUN_TO_TASK_STATUS = Object.freeze({
  created: "queued",
  queued: "queued",
  running: "running",
  cancelling: "running",
  succeeded: "succeeded",
  partial: "partial",
  failed: "failed",
  cancelled: "failed"
});

const STAGE_EVENT_LEVEL = Object.freeze({
  degraded: "warning",
  failed: "error",
  timed_out: "error",
  blocked: "error",
  cancelled: "warning",
  skipped: "info"
});

function exchangeSuffix(digits) {
  if (/^6/.test(digits)) return "SH";
  if (/^(0|3)/.test(digits)) return "SZ";
  if (/^(4|8)/.test(digits)) return "BJ";
  return "";
}

export function researchSymbol(input) {
  const value = String(input ?? "").trim().toUpperCase();
  if (SYMBOL_PATTERN.test(value)) return value;
  if (!DIGITS_PATTERN.test(value)) throw new Error("标的代码无效，需为六位 A 股代码");
  const suffix = exchangeSuffix(value);
  if (!suffix) throw new Error("无法识别标的代码的交易所后缀：" + value);
  return value + "." + suffix;
}

export function mapRunStatusToTaskStatus(status) {
  const key = String(status ?? "").toLowerCase();
  return RUN_TO_TASK_STATUS[key] ?? "queued";
}

export function runStageToTaskEvent(stage, sequence) {
  if (!stage || typeof stage !== "object" || typeof stage.stageCode !== "string" || !stage.stageCode) {
    throw new Error("Stage 缺少有效 stage_code");
  }
  const status = String(stage.status ?? "");
  return {
    sequence,
    event: "stage_status",
    stage_ref: stage.stageRef,
    stage_code: stage.stageCode,
    status,
    level: STAGE_EVENT_LEVEL[status] ?? "info",
    reason_codes: Array.isArray(stage.reasonCodes) ? stage.reasonCodes : []
  };
}

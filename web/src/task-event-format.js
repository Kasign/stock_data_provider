const TASK_EVENT_STATUS_LABELS = Object.freeze({
  succeeded: "已完成", success: "已完成", completed: "已完成",
  failed: "失败", error: "异常", blocked: "已阻断", rejected: "已拒绝",
  degraded: "已降级", partial: "部分完成", running: "运行中", started: "已启动",
  "in-progress": "运行中", in_progress: "运行中", processing: "处理中",
  queued: "排队中", pending: "等待中", skipped: "已跳过",
  unknown: "状态未报告", other: "其他状态"
});

export function taskEventStatusKey(status) {
  const normalized = String(status || "").trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-");
  return Object.prototype.hasOwnProperty.call(TASK_EVENT_STATUS_LABELS, normalized)
    ? normalized
    : normalized ? "other" : "unknown";
}

export function taskEventStatusLabel(status, statusKey = taskEventStatusKey(status)) {
  return statusKey === "other" ? String(status) : TASK_EVENT_STATUS_LABELS[statusKey];
}

export function taskEventStatusTone(statusKey) {
  if (["succeeded", "success", "completed"].includes(statusKey)) return "is-local";
  if (["failed", "error", "blocked", "rejected"].includes(statusKey)) return "is-error";
  if (["degraded", "partial"].includes(statusKey)) return "is-warning";
  return "is-readonly";
}


export function taskEventDurationLabel(value) {
  if (value === undefined || value === null || value === "") return "";
  const duration = Number(value);
  if (!Number.isFinite(duration) || duration < 0) return "";
  return duration >= 1000 ? (duration / 1000).toFixed(2) + " 秒" : duration + " 毫秒";
}

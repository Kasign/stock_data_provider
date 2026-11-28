export function buildTaskFlowSummaryViewModel({
  taskLoaded = false,
  taskId = "",
  taskEvents = [],
  taskEventsError = "",
  runResult = null,
  formatDateTime = value => value
} = {}) {
  const events = [...taskEvents].sort((a, b) => Number(a.sequence) - Number(b.sequence));
  let eventsCount;
  let latestEvent = "未报告";
  if (!taskLoaded) {
    eventsCount = taskId ? "等待事件读取" : "未选择任务";
  } else {
    eventsCount = taskEventsError
      ? events.length ? events.length + " 条（读取异常）" : "读取失败"
      : events.length ? String(events.length) : "暂无持久化事件";
    const last = events.length ? events[events.length - 1] : null;
    const lastEvent = last?.event && typeof last.event === "object" ? last.event : last;
    latestEvent = lastEvent?.at ? formatDateTime(lastEvent.at) : "未报告";
  }
  const corePass = runResult?.quality?.core_pass;
  const quality = corePass === true ? "通过"
    : corePass === false ? "未通过"
      : runResult ? "未报告" : "等待 Run 结果";
  const warnings = runResult?.warnings;
  return {
    eventsCount,
    latestEvent,
    quality,
    qualityClass: corePass === true ? "is-valid" : corePass === false ? "is-invalid" : "is-unknown",
    warningCount: Array.isArray(warnings) ? String(warnings.length) : runResult ? "未报告" : "等待 Run 结果"
  };
}

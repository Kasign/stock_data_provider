export const TASK_EVENT_PAGE_SIZE = 100;

function isRecord(value) {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

/**
 * 校验单页阶段事件并筛出可展示项。
 * 游标按服务端分页契约推进，事件序号只用于去重与展示，二者不能混用。
 */
export function validateTaskEventPage(payload, {
  taskId,
  runId,
  afterSequence = 0,
  highestSequence = 0,
  knownSequences = new Map()
} = {}) {
  if (!isRecord(payload) || !Array.isArray(payload.events)) {
    return { error: "阶段事件接口返回的数据结构无效。", identityMismatch: false };
  }
  if (payload.task_id !== taskId) {
    return {
      error: payload.task_id
        ? "阶段事件接口返回了其他任务的事件，已忽略。"
        : "阶段事件接口未返回 task_id，已忽略该响应。",
      identityMismatch: true
    };
  }

  const nextAfterSequence = payload.next_after_sequence;
  const highestReportedSequence = payload.events.reduce((maximum, item) => {
    const sequence = typeof item?.sequence === "number" ? item.sequence : NaN;
    return Number.isSafeInteger(sequence) && sequence >= 1 ? Math.max(maximum, sequence) : maximum;
  }, afterSequence);
  if (!Number.isSafeInteger(nextAfterSequence) ||
      nextAfterSequence < afterSequence ||
      (payload.events.length > 0 && nextAfterSequence <= afterSequence) ||
      nextAfterSequence < highestReportedSequence) {
    return { error: "阶段事件接口返回的 next_after_sequence 无效或未前进。", identityMismatch: false };
  }

  const events = [];
  const rejected = { invalidStructure: 0, mismatchedRun: 0, ignoredSequence: 0 };
  const seenSequences = new Set();
  for (const item of payload.events) {
    const sequence = typeof item?.sequence === "number" ? item.sequence : NaN;
    if (!isRecord(item) || !Number.isSafeInteger(sequence) || sequence < 1 || !isRecord(item.event)) {
      rejected.invalidStructure += 1;
      continue;
    }
    const eventRunId = typeof item.event.run_id === "string" ? item.event.run_id.trim() : "";
    if (!runId || !eventRunId || eventRunId !== runId) {
      rejected.mismatchedRun += 1;
      continue;
    }
    if (sequence <= highestSequence || seenSequences.has(sequence) || knownSequences.has(sequence)) {
      rejected.ignoredSequence += 1;
      continue;
    }
    seenSequences.add(sequence);
    events.push({ ...item, sequence });
  }

  return {
    error: "",
    identityMismatch: false,
    nextAfterSequence,
    hasMore: payload.events.length >= TASK_EVENT_PAGE_SIZE && nextAfterSequence > afterSequence,
    events,
    rejected
  };
}

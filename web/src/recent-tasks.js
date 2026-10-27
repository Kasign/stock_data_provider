export const RECENT_TASKS_KEY = "stock-provider-webui.recent-analysis.v1";
export const MAX_RECENT_TASKS = 10;

function normalizeRecord(record) {
  if (!record || typeof record.task_id !== "string" || !record.task_id.trim()) return null;
  return {
    task_id: record.task_id.trim(),
    run_id: typeof record.run_id === "string" ? record.run_id : "",
    created_at: typeof record.created_at === "string" ? record.created_at : "",
    symbols: Array.isArray(record.symbols) ? record.symbols.filter((item) => typeof item === "string") : [],
    task_type: typeof record.task_type === "string" ? record.task_type : "post_market_scan",
    status: typeof record.status === "string" ? record.status : "",
    analysis_base: typeof record.analysis_base === "string" ? record.analysis_base.trim() : "",
    updated_at: typeof record.updated_at === "string" ? record.updated_at : ""
  };
}

function normalizeLimit(limit) {
  return Math.max(0, Number.isFinite(Number(limit)) ? Math.floor(Number(limit)) : MAX_RECENT_TASKS);
}

export function readRecentTasksState(storage, limit = MAX_RECENT_TASKS) {
  let raw;
  try {
    raw = storage.getItem(RECENT_TASKS_KEY);
  } catch {
    return { items: [], storageReadable: false, storageValid: false, reason: "unavailable" };
  }

  let parsed;
  try {
    parsed = raw === null || raw === undefined ? [] : JSON.parse(raw);
  } catch {
    return { items: [], storageReadable: true, storageValid: false, reason: "invalid" };
  }
  if (!Array.isArray(parsed)) {
    return { items: [], storageReadable: true, storageValid: false, reason: "invalid" };
  }

  let malformedRecord = false;
  const records = [];
  for (const item of parsed) {
    const normalized = normalizeRecord(item);
    if (!normalized) {
      malformedRecord = true;
      continue;
    }
    records.push(normalized);
  }

  return {
    items: records.slice(0, normalizeLimit(limit)),
    storageReadable: true,
    storageValid: !malformedRecord,
    reason: malformedRecord ? "invalid" : ""
  };
}

export function readRecentTasks(storage, limit = MAX_RECENT_TASKS) {
  return readRecentTasksState(storage, limit).items;
}

export function upsertRecentTask(record, storage, limit = MAX_RECENT_TASKS) {
  const currentState = readRecentTasksState(storage, limit);
  if (!currentState.storageReadable || !currentState.storageValid) return currentState.items;

  const normalized = normalizeRecord(record);
  if (!normalized) return currentState.items;
  const records = [normalized, ...currentState.items.filter((item) =>
    item.task_id !== normalized.task_id || (item.analysis_base && item.analysis_base !== normalized.analysis_base)
  )].slice(0, normalizeLimit(limit));

  try {
    storage.setItem(RECENT_TASKS_KEY, JSON.stringify(records));
  } catch {
    // 最近任务历史是可选能力；写入失败时当前任务仍可继续，但不声称已持久化。
  }
  return records;
}

export function removeRecentTask(taskId, storage, limit = MAX_RECENT_TASKS, analysisBase = "") {
  if (typeof taskId !== "string" || !taskId.trim()) return readRecentTasks(storage, limit);
  const currentState = readRecentTasksState(storage, limit);
  if (!currentState.storageReadable || !currentState.storageValid) return currentState.items;

  const target = taskId.trim();
  const records = currentState.items.filter((item) =>
    item.task_id !== target || (analysisBase && item.analysis_base && item.analysis_base !== analysisBase)
  );
  try {
    storage.setItem(RECENT_TASKS_KEY, JSON.stringify(records));
  } catch {
    // 保持当前页面可用；存储异常状态会由界面明确提示。
  }
  return records;
}

export function clearRecentTasks(storage) {
  // 只有用户显式确认清空时才允许删除损坏的本地历史。
  try {
    storage.removeItem(RECENT_TASKS_KEY);
  } catch {
    try {
      storage.setItem(RECENT_TASKS_KEY, "[]");
    } catch {
      // Browser storage may be disabled; clearing is best-effort.
    }
  }
  return [];
}

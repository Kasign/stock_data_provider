export const RECENT_TASKS_KEY = "stock-provider-webui.recent-analysis.v1";
export const MAX_RECENT_TASKS = 10;

function normalizeRecord(record) {
  if (!record || typeof record.task_id !== "string" || !record.task_id.trim()) return null;
  return {
    task_id: record.task_id.trim(),
    run_id: typeof record.run_id === "string" ? record.run_id : "",
    symbols: Array.isArray(record.symbols) ? record.symbols.filter((item) => typeof item === "string") : [],
    task_type: typeof record.task_type === "string" ? record.task_type : "post_market_scan",
    status: typeof record.status === "string" ? record.status : "",
    updated_at: typeof record.updated_at === "string" ? record.updated_at : ""
  };
}

export function readRecentTasks(storage, limit = MAX_RECENT_TASKS) {
  try {
    const parsed = JSON.parse(storage.getItem(RECENT_TASKS_KEY) || "[]");
    if (!Array.isArray(parsed)) return [];
    return parsed.map(normalizeRecord).filter(Boolean).slice(0, Math.max(0, limit));
  } catch {
    return [];
  }
}

export function upsertRecentTask(record, storage, limit = MAX_RECENT_TASKS) {
  const normalized = normalizeRecord(record);
  if (!normalized) return readRecentTasks(storage, limit);
  const records = [normalized, ...readRecentTasks(storage, limit).filter((item) => item.task_id !== normalized.task_id)]
    .slice(0, Math.max(0, limit));
  try {
    storage.setItem(RECENT_TASKS_KEY, JSON.stringify(records));
  } catch {
    // Recent task history is optional. Keep the current task functional without storage.
  }
  return records;
}

export function removeRecentTask(taskId, storage, limit = MAX_RECENT_TASKS) {
  if (typeof taskId !== "string" || !taskId.trim()) return readRecentTasks(storage, limit);
  const target = taskId.trim();
  const records = readRecentTasks(storage, limit).filter((item) => item.task_id !== target);
  try {
    storage.setItem(RECENT_TASKS_KEY, JSON.stringify(records));
  } catch {
    // Keep the current page usable when browser storage is unavailable.
  }
  return records;
}

export function clearRecentTasks(storage) {
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

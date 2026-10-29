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
    const persistedState = readRecentTasksState(storage, limit);
    if (!persistedState.storageReadable || !persistedState.storageValid) return currentState.items;
    const persisted = persistedState.items.some((item) =>
      item.task_id === normalized.task_id &&
      item.analysis_base === normalized.analysis_base &&
      item.run_id === normalized.run_id
    );
    // 只有回读确认成功后，才把新增记录交给界面展示为最近历史。
    return persisted ? persistedState.items : currentState.items;
  } catch {
    // 最近任务历史是可选能力；写入失败时不把未持久化记录作为历史返回。
    return currentState.items;
  }
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
    const persistedState = readRecentTasksState(storage, limit);
    if (!persistedState.storageReadable || !persistedState.storageValid) return currentState.items;
    if (JSON.stringify(persistedState.items) !== JSON.stringify(records)) return currentState.items;
    return persistedState.items;
  } catch {
    // 删除未成功持久化时保留原列表，避免界面误报已删除。
    return currentState.items;
  }
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

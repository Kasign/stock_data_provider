export const WATCHLIST_KEY = "stock-provider-webui.watchlist.v1";
export const MAX_WATCHLIST_ITEMS = 50;

const SYMBOL_PATTERN = /^\d{6}(?:\.(?:SH|SZ|BJ))?$/;

function normalizeName(value, symbol) {
  const name = String(value ?? "")
    .replace(/[\u0000-\u001f\u007f]/g, "")
    .trim()
    .slice(0, 32)
    .trim();
  return name || symbol;
}

function normalizeRecord(record) {
  if (!record || typeof record !== "object") return null;
  const symbol = String(record.symbol ?? "").trim().toUpperCase();
  if (!SYMBOL_PATTERN.test(symbol)) return null;
  return {
    symbol,
    name: normalizeName(record.name, symbol),
    updated_at: typeof record.updated_at === "string" ? record.updated_at : ""
  };
}

function writeRecords(storage, records) {
  try {
    storage.setItem(WATCHLIST_KEY, JSON.stringify(records));
    return true;
  } catch {
    // 保留本次页面的操作结果，但明确告知调用方没有持久化成功。
    return false;
  }
}

function readRecords(storage, limit = MAX_WATCHLIST_ITEMS) {
  let raw;
  try {
    raw = storage.getItem(WATCHLIST_KEY);
  } catch {
    return { items: [], storageReadable: false, storageValid: false, reason: "unavailable" };
  }

  let parsed;
  try {
    parsed = JSON.parse(raw || "[]");
  } catch {
    // 不覆盖无法解析的原始数据，避免一次操作把用户数据清空。
    return { items: [], storageReadable: true, storageValid: false, reason: "invalid" };
  }
  if (!Array.isArray(parsed)) {
    return { items: [], storageReadable: true, storageValid: false, reason: "invalid" };
  }

  const maxItems = Math.max(0, Number.isFinite(Number(limit)) ? Math.floor(Number(limit)) : MAX_WATCHLIST_ITEMS);
  if (maxItems === 0) return { items: [], storageReadable: true, storageValid: true, reason: "" };
  const records = [];
  const seen = new Set();
  for (const item of parsed) {
    const normalized = normalizeRecord(item);
    if (!normalized || seen.has(normalized.symbol)) continue;
    seen.add(normalized.symbol);
    records.push(normalized);
    if (records.length >= maxItems) break;
  }
  return { items: records, storageReadable: true, storageValid: true, reason: "" };
}

export function readWatchlistState(storage, limit = MAX_WATCHLIST_ITEMS) {
  return readRecords(storage, limit);
}

export function readWatchlist(storage, limit = MAX_WATCHLIST_ITEMS) {
  return readWatchlistState(storage, limit).items;
}

export function addWatchlistSymbol(symbol, name, storage, updatedAt = new Date().toISOString()) {
  const normalized = normalizeRecord({ symbol, name, updated_at: updatedAt });
  const currentState = readWatchlistState(storage);
  const current = currentState.items;
  if (!normalized) {
    return {
      items: current, added: false, valid: false, persisted: false,
      storageReadable: currentState.storageReadable, storageValid: currentState.storageValid
    };
  }
  if (!currentState.storageReadable || !currentState.storageValid) {
    return {
      items: current, added: false, valid: true, persisted: false,
      storageReadable: currentState.storageReadable, storageValid: currentState.storageValid
    };
  }

  const existing = current.find((item) => item.symbol === normalized.symbol);
  const next = {
    symbol: normalized.symbol,
    name: existing?.name && existing.name !== existing.symbol ? existing.name : normalized.name,
    updated_at: String(updatedAt || new Date().toISOString())
  };
  const items = [next, ...current.filter((item) => item.symbol !== next.symbol)]
    .slice(0, MAX_WATCHLIST_ITEMS);
  const persisted = writeRecords(storage, items);
  return { items, added: !existing, valid: true, persisted, storageReadable: true, storageValid: true };
}

export function renameWatchlistSymbol(symbol, name, storage, updatedAt = new Date().toISOString()) {
  const normalizedSymbol = String(symbol ?? "").trim().toUpperCase();
  const currentState = readWatchlistState(storage);
  const current = currentState.items;
  if (!SYMBOL_PATTERN.test(normalizedSymbol)) {
    return {
      items: current, changed: false, valid: false, persisted: false,
      storageReadable: currentState.storageReadable, storageValid: currentState.storageValid
    };
  }
  if (!currentState.storageReadable || !currentState.storageValid) {
    return {
      items: current, changed: false, valid: true, persisted: false,
      storageReadable: currentState.storageReadable, storageValid: currentState.storageValid
    };
  }
  const changed = current.some((item) => item.symbol === normalizedSymbol);
  if (!changed) {
    return { items: current, changed: false, valid: true, persisted: true, storageReadable: true, storageValid: true };
  }
  const items = current.map((item) => item.symbol === normalizedSymbol
    ? { ...item, name: normalizeName(name, normalizedSymbol), updated_at: String(updatedAt || new Date().toISOString()) }
    : item);
  const persisted = writeRecords(storage, items);
  return { items, changed: true, valid: true, persisted, storageReadable: true, storageValid: true };
}

export function removeWatchlistSymbol(symbol, storage) {
  const normalizedSymbol = String(symbol ?? "").trim().toUpperCase();
  const currentState = readWatchlistState(storage);
  const current = currentState.items;
  if (!SYMBOL_PATTERN.test(normalizedSymbol)) {
    return {
      items: current, removed: false, valid: false, persisted: false,
      storageReadable: currentState.storageReadable, storageValid: currentState.storageValid
    };
  }
  if (!currentState.storageReadable || !currentState.storageValid) {
    return {
      items: current, removed: false, valid: true, persisted: false,
      storageReadable: currentState.storageReadable, storageValid: currentState.storageValid
    };
  }
  const items = current.filter((item) => item.symbol !== normalizedSymbol);
  const removed = items.length !== current.length;
  if (!removed) {
    return { items: current, removed: false, valid: true, persisted: true, storageReadable: true, storageValid: true };
  }
  const persisted = writeRecords(storage, items);
  return { items, removed: true, valid: true, persisted, storageReadable: true, storageValid: true };
}

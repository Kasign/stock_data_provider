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

export function readWatchlist(storage, limit = MAX_WATCHLIST_ITEMS) {
  try {
    const parsed = JSON.parse(storage.getItem(WATCHLIST_KEY) || "[]");
    if (!Array.isArray(parsed)) return [];
    const maxItems = Math.max(0, Number.isFinite(Number(limit)) ? Math.floor(Number(limit)) : MAX_WATCHLIST_ITEMS);
    if (maxItems === 0) return [];
    const records = [];
    const seen = new Set();
    for (const item of parsed) {
      const normalized = normalizeRecord(item);
      if (!normalized || seen.has(normalized.symbol)) continue;
      seen.add(normalized.symbol);
      records.push(normalized);
      if (records.length >= maxItems) break;
    }
    return records;
  } catch {
    return [];
  }
}

export function addWatchlistSymbol(symbol, name, storage, updatedAt = new Date().toISOString()) {
  const normalized = normalizeRecord({ symbol, name, updated_at: updatedAt });
  const current = readWatchlist(storage);
  if (!normalized) return { items: current, added: false, valid: false };

  const existing = current.find((item) => item.symbol === normalized.symbol);
  const next = {
    symbol: normalized.symbol,
    name: existing?.name && existing.name !== existing.symbol ? existing.name : normalized.name,
    updated_at: String(updatedAt || new Date().toISOString())
  };
  const items = [next, ...current.filter((item) => item.symbol !== next.symbol)]
    .slice(0, MAX_WATCHLIST_ITEMS);
  const persisted = writeRecords(storage, items);
  return { items, added: !existing, valid: true, persisted };
}

export function renameWatchlistSymbol(symbol, name, storage, updatedAt = new Date().toISOString()) {
  const normalizedSymbol = String(symbol ?? "").trim().toUpperCase();
  const current = readWatchlist(storage);
  if (!SYMBOL_PATTERN.test(normalizedSymbol)) {
    return { items: current, changed: false, valid: false, persisted: false };
  }
  const changed = current.some((item) => item.symbol === normalizedSymbol);
  const items = current.map((item) => item.symbol === normalizedSymbol
    ? { ...item, name: normalizeName(name, normalizedSymbol), updated_at: String(updatedAt || new Date().toISOString()) }
    : item);
  const persisted = writeRecords(storage, items);
  return { items, changed, valid: true, persisted };
}

export function removeWatchlistSymbol(symbol, storage) {
  const normalizedSymbol = String(symbol ?? "").trim().toUpperCase();
  const current = readWatchlist(storage);
  if (!SYMBOL_PATTERN.test(normalizedSymbol)) {
    return { items: current, removed: false, valid: false, persisted: false };
  }
  const items = current.filter((item) => item.symbol !== normalizedSymbol);
  const removed = items.length !== current.length;
  const persisted = writeRecords(storage, items);
  return { items, removed, valid: true, persisted };
}

export const WATCHLIST_KEY = "stock-provider-webui.watchlist.v1";
export const MAX_WATCHLIST_ITEMS = 50;

const SYMBOL_PATTERN = /^\d{6}(?:\.(?:SH|SZ|BJ))?$/;

// 将可识别的 A 股纯六位代码映射到默认交易所，确保和行情/研究模块的标的比较一致。
function comparableSymbol(input) {
  const symbol = String(input ?? "").trim().toUpperCase();
  if (symbol.includes(".")) return symbol;
  if (symbol.startsWith("6")) return symbol + ".SH";
  if (/^[03]\d{5}$/.test(symbol)) return symbol + ".SZ";
  if (/^(?:[48]\d{5}|92\d{4})$/.test(symbol)) return symbol + ".BJ";
  return symbol;
}

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
    parsed = JSON.parse(raw === null || raw === undefined ? "[]" : raw);
  } catch {
    // 不覆盖无法解析的原始数据，避免一次操作把用户数据清空。
    return { items: [], storageReadable: true, storageValid: false, reason: "invalid" };
  }
  if (!Array.isArray(parsed)) {
    return { items: [], storageReadable: true, storageValid: false, reason: "invalid" };
  }

  const maxItems = Math.max(0, Number.isFinite(Number(limit)) ? Math.floor(Number(limit)) : MAX_WATCHLIST_ITEMS);
  const records = [];
  const seen = new Set();
  let malformedRecord = parsed.length > MAX_WATCHLIST_ITEMS;

  // 先检查完整原始数组，再按读取上限返回；不能因跳过坏记录或截断超限记录而允许后续写回。
  for (const item of parsed) {
    const normalized = normalizeRecord(item);
    if (!normalized) {
      malformedRecord = true;
      continue;
    }
    const identity = comparableSymbol(normalized.symbol);
    if (seen.has(identity)) {
      malformedRecord = true;
      continue;
    }
    seen.add(identity);
    records.push(normalized);
  }

  return {
    items: records.slice(0, Math.min(maxItems, MAX_WATCHLIST_ITEMS)),
    storageReadable: true,
    storageValid: !malformedRecord,
    reason: malformedRecord ? "invalid" : ""
  };
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

  const identity = comparableSymbol(normalized.symbol);
  const existing = current.find((item) => comparableSymbol(item.symbol) === identity);
  const storedSymbol = existing?.symbol || normalized.symbol;
  const next = {
    symbol: storedSymbol,
    name: existing?.name && existing.name !== existing.symbol ? existing.name : normalized.name,
    updated_at: String(updatedAt || new Date().toISOString())
  };
  const items = [next, ...current.filter((item) => comparableSymbol(item.symbol) !== identity)]
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
  const identity = comparableSymbol(normalizedSymbol);
  const changed = current.some((item) => comparableSymbol(item.symbol) === identity);
  if (!changed) {
    return { items: current, changed: false, valid: true, persisted: true, storageReadable: true, storageValid: true };
  }
  const items = current.map((item) => comparableSymbol(item.symbol) === identity
    ? { ...item, name: normalizeName(name, item.symbol), updated_at: String(updatedAt || new Date().toISOString()) }
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
  const identity = comparableSymbol(normalizedSymbol);
  const items = current.filter((item) => comparableSymbol(item.symbol) !== identity);
  const removed = items.length !== current.length;
  if (!removed) {
    return { items: current, removed: false, valid: true, persisted: true, storageReadable: true, storageValid: true };
  }
  const persisted = writeRecords(storage, items);
  return { items, removed: true, valid: true, persisted, storageReadable: true, storageValid: true };
}

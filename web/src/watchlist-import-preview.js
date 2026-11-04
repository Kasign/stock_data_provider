import { comparableMarketSymbol } from "./format.js";
import { MAX_WATCHLIST_ITEMS } from "./watchlist.js";

function parseWatchlistImportLine(line) {
  const cells = [];
  let value = "", quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (char === '"') {
      if (quoted && line[index + 1] === '"') { value += '"'; index += 1; }
      else quoted = !quoted;
    } else if ((char === "," || char === "\t" || char === ";") && !quoted) {
      cells.push(value.trim()); value = "";
    } else value += char;
  }
  cells.push(value.trim());
  return cells;
}

export function buildWatchlistImportPreview(rawText, existingState) {
  if (!existingState?.storageReadable || !existingState?.storageValid) {
    return { rows: [], accepted: [], error: "当前本地自选数据不可读取或格式异常，已暂停导入。" };
  }
  const existingItems = Array.isArray(existingState.items) ? existingState.items : [];
  const lines = String(rawText || "").replace(/^\uFEFF/, "").split(/\r?\n/).filter((line) => line.trim());
  const rows = [];
  const accepted = [];
  const seen = new Set(existingItems.map((item) => comparableMarketSymbol(item.symbol)));
  const remaining = Math.max(0, MAX_WATCHLIST_ITEMS - existingItems.length);
  let capacity = remaining;
  lines.forEach((line, index) => {
    const cells = parseWatchlistImportLine(line);
    const rawSymbol = String(cells[0] || "").trim().toUpperCase();
    const rawName = String(cells[1] || "").trim();
    if (index === 0 && /^(代码|股票代码|symbol|code)$/i.test(rawSymbol)) return;
    let symbol = rawSymbol;
    if (/^\d{6}$/.test(symbol)) {
      if (symbol.startsWith("6")) symbol += ".SH";
      else if (/^[03]/.test(symbol)) symbol += ".SZ";
      else if (/^(?:[48]|92)/.test(symbol)) symbol += ".BJ";
    }
    if (!/^\d{6}(?:\.(SH|SZ|BJ))?$/.test(symbol)) {
      rows.push({ line: index + 1, symbol: rawSymbol, name: rawName, status: "invalid", message: "代码格式无效" });
      return;
    }
    const identity = comparableMarketSymbol(symbol);
    if (seen.has(identity)) {
      rows.push({ line: index + 1, symbol, name: rawName, status: "duplicate", message: "已存在或文件内重复" });
      return;
    }
    seen.add(identity);
    if (capacity <= 0) {
      rows.push({ line: index + 1, symbol, name: rawName, status: "limit", message: "超过自选股上限" });
      return;
    }
    capacity -= 1;
    const record = { symbol, name: rawName || symbol };
    accepted.push(record);
    rows.push({ line: index + 1, ...record, status: "ready", message: "可导入" });
  });
  return {
    rows, accepted, remaining, counts: {
      total: rows.length,
      ready: accepted.length,
      duplicate: rows.filter((item) => item.status === "duplicate").length,
      invalid: rows.filter((item) => item.status === "invalid").length,
      limit: rows.filter((item) => item.status === "limit").length
    }
  };
}

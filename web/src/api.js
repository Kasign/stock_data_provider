import { normalizeBasePath } from "./format.js";

export const SETTINGS_KEY = "stock-provider-webui.settings.v1";
const DEFAULTS = Object.freeze({ marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "" });

export function loadSettings() {
  try {
    const saved = JSON.parse(localStorage.getItem(SETTINGS_KEY) || "{}");
    return {
      marketBase: normalizeBasePath(saved.marketBase, DEFAULTS.marketBase),
      analysisBase: normalizeBasePath(saved.analysisBase, DEFAULTS.analysisBase),
      apiKey: typeof saved.apiKey === "string" ? saved.apiKey : ""
    };
  } catch {
    return { ...DEFAULTS };
  }
}

export function saveSettings(settings) {
  const value = {
    marketBase: normalizeBasePath(settings.marketBase, DEFAULTS.marketBase),
    analysisBase: normalizeBasePath(settings.analysisBase, DEFAULTS.analysisBase),
    apiKey: String(settings.apiKey ?? "").trim()
  };
  const serialized = JSON.stringify(value);
  localStorage.setItem(SETTINGS_KEY, serialized);
  if (localStorage.getItem(SETTINGS_KEY) !== serialized) {
    throw new Error("设置未能保存到当前浏览器，请检查存储权限或可用空间。");
  }
  return value;
}

export function clearSettings() {
  localStorage.removeItem(SETTINGS_KEY);
  if (localStorage.getItem(SETTINGS_KEY) !== null) {
    throw new Error("未能确认本地连接设置已清除，请检查浏览器存储后重试。");
  }
  return { ...DEFAULTS };
}

function isValidISODate(value) {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day;
}

async function request(base, path, { apiKey = "", method = "GET", body, signal, headers: extraHeaders = {} } = {}) {
  const prefix = normalizeBasePath(base, "/");
  const headers = { Accept: "application/json", ...extraHeaders };
  if (apiKey) headers["X-API-Key"] = apiKey;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const requestPath = prefix === "/" ? path : `${prefix}${path}`;
  const response = await fetch(requestPath, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
    credentials: "same-origin"
  });
  const text = await response.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = { message: text.slice(0, 500) }; }
  }
  if (!response.ok) {
    const error = data && typeof data === "object" ? data.error : undefined;
    const rawMessage = error?.message
      || (typeof error === "string" ? error : "")
      || (typeof data === "string" ? data : "")
      || data?.message
      || `HTTP ${response.status} ${response.statusText}`;
    // Bound diagnostics even when a valid JSON error envelope contains an oversized message.
    const failure = new Error(String(rawMessage).slice(0, 500));
    failure.status = response.status;
    failure.code = String(error?.code || data?.code || "http_error").slice(0, 120);
    failure.requestId = String(error?.request_id || response.headers.get("X-Request-ID") || "").slice(0, 200);
    throw failure;
  }
  return data ?? {};
}

export function marketLiveness(settings, signal) {
  return request(settings.marketBase, "/health/live", { signal });
}
export function marketHealth(settings, signal) {
  return request(settings.marketBase, "/health/ready", { signal });
}
export function analysisLiveness(settings, signal) {
  return request(settings.analysisBase, "/healthz", { signal });
}
export function analysisHealth(settings, signal) {
  return request(settings.analysisBase, "/readyz", { signal });
}
export function getDataSources(settings, signal) {
  return request(settings.marketBase, "/api/v1/data-sources", { apiKey: settings.apiKey, signal });
}
export function getMarketValuations(settings, symbols, signal) {
  const normalizedSymbols = Array.isArray(symbols) ? symbols.map((item) => String(item ?? "").trim()).filter(Boolean) : [];
  if (!normalizedSymbols.length || normalizedSymbols.length > 100) {
    return Promise.reject(new Error("估值查询需要 1–100 个规范 A 股代码。"));
  }
  const query = new URLSearchParams({ symbols: normalizedSymbols.join(",") });
  return request(settings.marketBase, `/api/v1/market/valuations?${query}`, {
    apiKey: settings.apiKey,
    signal
  });
}
export function getMarketHotStocks(settings, { type = "popular", limit = 10, signal } = {}) {
  const rankingType = ["popular", "surge"].includes(String(type)) ? String(type) : "popular";
  const boundedLimit = Math.max(1, Math.min(100, Math.trunc(Number(limit) || 10)));
  const query = new URLSearchParams({ type: rankingType, limit: String(boundedLimit) });
  return request(settings.marketBase, "/api/v1/market/hot-stocks?" + query.toString(), { apiKey: settings.apiKey, signal });
}
export function getMarketIndices(settings, signal) { return request(settings.marketBase, "/api/v1/market/indices", { apiKey: settings.apiKey, signal }); }
export function getMarketSession(settings, at = "", signal) {
  const query = new URLSearchParams();
  if (at) query.set("at", at);
  return request(settings.marketBase, "/api/v1/market/session" + (query.size ? "?" + query.toString() : ""), {
    apiKey: settings.apiKey,
    signal
  });
}

export function getIndexCatalog(settings, { tag = "cn_concept", signal } = {}) {
  const allowedTags = new Set(["cn_concept", "region", "tszs", "industry", "tushare"]);
  const normalizedTag = allowedTags.has(String(tag)) ? String(tag) : "cn_concept";
  const query = new URLSearchParams({ tag: normalizedTag });
  return request(settings.marketBase, "/api/v1/market/index-catalog?" + query.toString(), {
    apiKey: settings.apiKey,
    signal
  });
}

export function getIndexConstituents(settings, symbol, { source = "", asOf = "", signal } = {}) {
  const normalizedSymbol = String(symbol ?? "").trim();
  if (!normalizedSymbol) return Promise.reject(new Error("指数代码不能为空。"));
  const query = new URLSearchParams();
  if (source) query.set("source", source);
  if (asOf) query.set("as_of", asOf);
  const suffix = query.size ? "?" + query.toString() : "";
  return request(settings.marketBase, "/api/v1/market/indices/" + encodeURIComponent(normalizedSymbol) + "/constituents" + suffix, {
    apiKey: settings.apiKey,
    signal
  });
}

export function getIndexDailyBars(settings, symbol, { start, end, source = "", signal } = {}) {
  const normalizedSymbol = String(symbol ?? "").trim();
  if (!normalizedSymbol) return Promise.reject(new Error("指数代码不能为空。"));
  if (!/^\d{4}-\d{2}-\d{2}$/.test(String(start ?? "")) || !/^\d{4}-\d{2}-\d{2}$/.test(String(end ?? ""))) {
    return Promise.reject(new Error("指数日线需要有效的开始和结束日期。"));
  }
  if (String(start) > String(end)) return Promise.reject(new Error("开始日期不能晚于结束日期。"));
  const query = new URLSearchParams({ start: String(start), end: String(end) });
  if (source) query.set("source", source);
  return request(settings.marketBase, "/api/v1/market/indices/" + encodeURIComponent(normalizedSymbol) + "/daily?" + query.toString(), {
    apiKey: settings.apiKey,
    signal
  });
}

export function getStockPriceLimitRule(settings, symbol, date, signal) {
  const normalizedSymbol = String(symbol ?? "").trim();
  if (!normalizedSymbol) return Promise.reject(new Error("股票代码不能为空。"));
  if (!/^\d{4}-\d{2}-\d{2}$/.test(String(date ?? ""))) return Promise.reject(new Error("涨跌幅规则查询需要有效的交易日期。"));
  const query = new URLSearchParams({ date: String(date) });
  return request(settings.marketBase, "/api/v1/stocks/" + encodeURIComponent(normalizedSymbol) + "/price-limit-rule?" + query.toString(), {
    apiKey: settings.apiKey,
    signal
  });
}

export function getStockTradingStatus(settings, symbol, date, { asOf = "", signal } = {}) {
  const normalizedSymbol = String(symbol ?? "").trim();
  if (!normalizedSymbol) return Promise.reject(new Error("股票代码不能为空。"));
  if (!/^\d{4}-\d{2}-\d{2}$/.test(String(date ?? ""))) return Promise.reject(new Error("交易状态查询需要有效的交易日期。"));
  const query = new URLSearchParams({ date: String(date) });
  if (asOf) query.set("as_of", asOf);
  return request(settings.marketBase, "/api/v1/stocks/" + encodeURIComponent(normalizedSymbol) + "/trading-status?" + query.toString(), {
    apiKey: settings.apiKey,
    signal
  });
}

export function getMarketBreadth(settings, signal) { return request(settings.marketBase, "/api/v1/market/breadth", { apiKey: settings.apiKey, signal }); }
export function getMarketSectors(settings, { type = "industry", limit = 10, signal } = {}) { const query=new URLSearchParams({type:String(type),limit:String(limit)});return request(settings.marketBase,"/api/v1/market/sectors?"+query.toString(),{apiKey:settings.apiKey,signal}); }
export function getMarketLimitPool(settings, { type = "up", limit = 30, date = "", signal } = {}) { const query=new URLSearchParams({type:String(type),limit:String(limit)});if(date)query.set("date",date);return request(settings.marketBase,"/api/v1/market/limit-pools?"+query.toString(),{apiKey:settings.apiKey,signal}); }

export function getMarketDragonTiger(settings,{date="",page=1,pageSize=50,signal}={}){const query=new URLSearchParams({page:String(page),page_size:String(pageSize)});if(date)query.set("date",date);return request(settings.marketBase,"/api/v1/market/dragon-tiger?"+query.toString(),{apiKey:settings.apiKey,signal});}
export function getTradingCalendar(settings,{start,end,signal}={}){if(!isValidISODate(start)||!isValidISODate(end))return Promise.reject(new Error("交易日历需要有效的开始和结束日期。"));if(start>end)return Promise.reject(new Error("交易日历开始日期不能晚于结束日期。"));const query=new URLSearchParams({start:String(start),end:String(end)});return request(settings.marketBase,"/api/v1/market/trading-calendar?"+query.toString(),{apiKey:settings.apiKey,signal});}

export function getStockProfile(settings, symbol, signal) { return request(settings.marketBase, "/api/v1/stocks/" + encodeURIComponent(symbol) + "/profile", { apiKey: settings.apiKey, signal }); }
export function getStockBoards(settings, symbol, signal) { return request(settings.marketBase, "/api/v1/stocks/" + encodeURIComponent(symbol) + "/boards", { apiKey: settings.apiKey, signal }); }
export function getStockFundamentals(settings, symbol, { year, quarter, signal } = {}) { const query=new URLSearchParams({year:String(year),quarter:String(quarter)});return request(settings.marketBase, "/api/v1/stocks/" + encodeURIComponent(symbol) + "/fundamentals?" + query.toString(), { apiKey: settings.apiKey, signal }); }
export function getStockAnnouncements(settings, symbol, { startDate="", endDate="", page=1, pageSize=20, signal } = {}) { const query=new URLSearchParams({page:String(page),page_size:String(pageSize)});if(startDate)query.set("start_date",startDate);if(endDate)query.set("end_date",endDate);return request(settings.marketBase, "/api/v1/stocks/" + encodeURIComponent(symbol) + "/announcements?" + query.toString(), { apiKey: settings.apiKey, signal }); }

export function searchStocks(settings, query, { limit = 10, signal } = {}) {
  const params = new URLSearchParams({ q: String(query ?? "").trim(), limit: String(limit) });
  return request(settings.marketBase, `/api/v1/stocks/search?${params}`, { apiKey: settings.apiKey, signal });
}
export function getMarketBundle(settings, symbol, { lookbackDays = 120, adjustment = "forward_adjusted", asOf = "" } = {}, signal) {
  const query = new URLSearchParams({
    lookback_days: String(lookbackDays),
    adjustment
  });
  if (asOf) query.set("as_of", asOf);
  return request(settings.marketBase, `/api/v1/bundles/${encodeURIComponent(symbol)}?${query}`, {
    apiKey: settings.apiKey,
    signal
  });
}
export function getQuote(settings, symbol, signal) {
  return request(settings.marketBase, `/api/v1/quotes/${encodeURIComponent(symbol)}`, { apiKey: settings.apiKey, signal });
}
export function getDailyBars(settings, symbol, { limit = 120, adjustment = "forward_adjusted", start = "", end = "" } = {}, signal) {
  const query = new URLSearchParams({ limit: String(limit), adjustment });
  if (start) query.set("start", start);
  if (end) query.set("end", end);
  return request(settings.marketBase, `/api/v1/bars/${encodeURIComponent(symbol)}/daily?${query}`, { apiKey: settings.apiKey, signal });
}
export function createAnalysisTask(settings, payload, idempotencyKey = "") {
  const headersKey = idempotencyKey.trim();
  return request(settings.analysisBase, "/api/v1/analysis/tasks", {
    method: "POST",
    body: payload,
    signal: undefined,
    headers: headersKey ? { "Idempotency-Key": headersKey } : {}
  }).catch((error) => { throw error; });
}
export function getAnalysisRun(settings, runId) {
  return request(settings.analysisBase, `/api/v1/analysis/runs/${encodeURIComponent(runId)}`);
}
export function getAnalysisTask(settings, taskId) {
  return request(settings.analysisBase, `/api/v1/analysis/tasks/${encodeURIComponent(taskId)}`);
}
export function getAnalysisTaskEvents(settings, taskId, afterSequence = 0) {
  const query = new URLSearchParams({ after_sequence: String(afterSequence), limit: "100" });
  return request(settings.analysisBase, `/api/v1/analysis/tasks/${encodeURIComponent(taskId)}/events?${query}`);
}

export function getChipDistribution(settings, symbol, signal) {
  return request(settings.marketBase, `/api/v1/stocks/${encodeURIComponent(symbol)}/chip-distribution`, {
    apiKey: settings.apiKey,
    signal
  });
}

export function getCapitalFlow(settings, symbol, signal) {
  return request(settings.marketBase, `/api/v1/stocks/${encodeURIComponent(symbol)}/capital-flow`, {
    apiKey: settings.apiKey,
    signal
  });
}

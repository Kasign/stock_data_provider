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
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(value));
  return value;
}

export function clearSettings() {
  localStorage.removeItem(SETTINGS_KEY);
  return { ...DEFAULTS };
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

export function marketHealth(settings) {
  return request(settings.marketBase, "/health/ready");
}
export function analysisHealth(settings) {
  return request(settings.analysisBase, "/readyz");
}
export function getDataSources(settings) {
  return request(settings.marketBase, "/api/v1/data-sources", { apiKey: settings.apiKey });
}
export function getMarketIndices(settings, signal) { return request(settings.marketBase, "/api/v1/market/indices", { apiKey: settings.apiKey, signal }); }
export function getMarketBreadth(settings, signal) { return request(settings.marketBase, "/api/v1/market/breadth", { apiKey: settings.apiKey, signal }); }
export function getMarketSectors(settings, { type = "industry", limit = 10, signal } = {}) { const query=new URLSearchParams({type:String(type),limit:String(limit)});return request(settings.marketBase,"/api/v1/market/sectors?"+query.toString(),{apiKey:settings.apiKey,signal}); }
export function getMarketLimitPool(settings, { type = "up", limit = 30, date = "", signal } = {}) { const query=new URLSearchParams({type:String(type),limit:String(limit)});if(date)query.set("date",date);return request(settings.marketBase,"/api/v1/market/limit-pools?"+query.toString(),{apiKey:settings.apiKey,signal}); }

export function searchStocks(settings, query, { limit = 10, signal } = {}) {
  const params = new URLSearchParams({ q: String(query ?? "").trim(), limit: String(limit) });
  return request(settings.marketBase, `/api/v1/stocks/search?${params}`, { apiKey: settings.apiKey, signal });
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

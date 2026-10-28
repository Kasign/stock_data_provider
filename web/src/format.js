export function normalizeBasePath(input, fallback) {
  const value = String(input ?? "").trim() || fallback;
  if (!value.startsWith("/") || value.startsWith("//") || value.includes("://")) {
    throw new Error("API 路径必须是同源的绝对路径，例如 /market-api。");
  }
  if (/[?#\s]/.test(value)) {
    throw new Error("API 路径不能包含查询参数、片段或空白字符。");
  }
  const normalized = value.replace(/\/+$/, "");
  return normalized || "/";
}

export function normalizeSymbol(input) {
  const value = String(input ?? "").trim().toUpperCase().replace(/\s+/g, "");
  if (/^\d{6}$/.test(value) || /^\d{6}\.(SH|SZ|BJ)$/.test(value)) return value;
  throw new Error("股票代码格式不正确，请输入 6 位代码或带交易所后缀的代码，例如 600519.SH。");
}

export function parseSymbols(input) {
  const parts = String(input ?? "").split(/[，,\s;；]+/).map((item) => item.trim()).filter(Boolean);
  const normalized = [...new Set(parts.map(normalizeSymbol))];
  if (!normalized.length) throw new Error("请至少输入一只股票代码。");
  if (normalized.length > 100) throw new Error("一次最多分析 100 只股票。");
  return normalized;
}

// Go analysis currently accepts six-digit A-share codes only. Do not use this
// normalizer for market-data queries, which may require an exchange suffix.
export function normalizeAnalysisSymbol(input) {
  return normalizeSymbol(input).slice(0, 6);
}

export function parseAnalysisSymbols(input) {
  const symbols = [...new Set(parseSymbols(input).map(normalizeAnalysisSymbol))];
  if (!symbols.length) throw new Error("请至少输入一只股票代码。");
  if (symbols.length > 100) throw new Error("一次最多分析 100 只股票。");
  return symbols;
}

export function finiteNumberOrNull(value) {
  if (value === null || value === undefined ||
      (typeof value !== "number" && typeof value !== "string") ||
      (typeof value === "string" && !value.trim())) return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

export function formatPrice(value, digits = 2) {
  const number = finiteNumberOrNull(value);
  if (number === null) return "—";
  return number.toLocaleString("zh-CN", { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

export function formatCompact(value) {
  const number = finiteNumberOrNull(value);
  if (number === null) return "—";
  return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 2 }).format(number);
}

export function formatPercent(value, digits = 2) {
  const number = finiteNumberOrNull(value);
  if (number === null) return "—";
  return `${number > 0 ? "+" : ""}${number.toFixed(digits)}%`;
}

export function formatDateTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  }).format(date);
}

export function formatDate(value) {
  if (!value) return "—";
  const date = String(value).slice(0, 10);
  return /^\d{4}-\d{2}-\d{2}$/.test(date) ? date.replaceAll("-", ".") : String(value);
}

export function qualityLabel(status) {
  const map = {
    valid: "质量正常",
    degraded: "质量降级",
    stale: "数据过期",
    invalid: "数据无效",
    unknown: "质量未知"
  };
  return map[String(status ?? "").toLowerCase()] || "尚未查询";
}

export function taskStatusLabel(status) {
  const map = {
    queued: "排队中",
    running: "执行中",
    succeeded: "执行成功",
    partial: "部分完成",
    failed: "执行失败"
  };
  return map[String(status ?? "").toLowerCase()] || String(status || "未知状态");
}

export function runStatusLabel(status) {
  const map = {
    running: "分析中",
    completed: "分析完成",
    blocked_data_quality: "数据质量阻断",
    rejected_request: "请求被拒绝",
    partial: "部分结果",
    failed: "分析失败"
  };
  return map[String(status ?? "").toLowerCase()] || String(status || "未知运行状态");
}


// Retry network failures and HTTP conditions that are commonly transient.
// Other 4xx responses are permanent until the user fixes configuration or the resource ID.
export function isRetryableApiError(error) {
  const status = Number(error?.status);
  if (!Number.isFinite(status) || status <= 0) return true;
  if ([408, 409, 425, 429].includes(status)) return true;
  return status >= 500;
}

export function describeApiError(error, fallback = "请求失败") {
  const status = Number(error?.status);
  let message;
  if (status === 401) {
    message = "接口认证失败，请检查当前服务的认证与反向代理配置。";
  } else if (status === 403) {
    message = "当前请求被服务拒绝，请检查访问权限。";
  } else if (status === 404) {
    message = "接口或资源不存在，请检查 API 路径、服务版本及资源 ID。";
  } else if (status === 502 || status === 503 || status === 504) {
    message = error?.message || "后端服务暂不可用。";
  } else {
    message = error?.message || fallback;
  }

  const details = [];
  if (Number.isFinite(status) && status > 0) details.push("HTTP " + status);
  const code = String(error?.code || "").trim();
  if (code && code !== "http_error") details.push("code=" + code);
  const requestId = String(error?.requestId || "").trim();
  if (requestId) details.push("request_id=" + requestId);
  return details.length ? message + "（" + details.join("；") + "）" : message;
}

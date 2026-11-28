import {
  getCapitalFlow, getChipDistribution, getDailyBars, getMarketBundle, getMarketValuations,
  getQuote, getStockAnnouncements, getStockBoards, getStockFundamentals, getStockPriceLimitRule,
  getStockProfile, getStockTradingStatus, runDataSourceProbe
} from "./api.js";
import { describeApiError, finiteNumberOrNull, formatDateTime, normalizeSymbol } from "./format.js";

// 用于确认浏览器是否加载了最新前端；页面与导出 JSON 都会带上。
export const DIAGNOSTICS_BUILD = "2026-11-28.3";

const QUALITY_WORD = { valid: "正常", degraded: "降级", stale: "过期", invalid: "无效", unknown: "未知" };
const DEGRADED_QUALITY = new Set(["degraded", "stale", "invalid", "unknown"]);
const LEVEL_WORD = { ok: "通过", degraded: "降级", failed: "失败" };

export function qualityWord(status) {
  return QUALITY_WORD[String(status || "").toLowerCase()] || "未知";
}

export function levelWord(level) {
  return LEVEL_WORD[level] || "未知";
}

function arrayLength(value) {
  return Array.isArray(value) ? value.length : null;
}

// 每个数据项只摘取“这次到底拿到了什么”的关键证据，不复制完整响应，
// 避免诊断页变成第二个业务详情页。
export function describeDiagnosticBody(key, body) {
  if (!body || typeof body !== "object") return "无响应体";
  switch (key) {
    case "quote":
      return [
        `价格 ${finiteNumberOrNull(body.current_price) ?? "—"}`,
        `来源 ${body.source || "—"}`,
        body.quote_time ? `行情时间 ${formatDateTime(body.quote_time)}` : "行情时间未报告"
      ].join(" · ");
    case "bundle": {
      const bars = arrayLength(body.daily_bars) ?? arrayLength(body.bars);
      const sources = body.sources || {};
      return `日线 ${bars ?? "—"} 根 · 行情源 ${sources.realtime_quote || "—"} · 日线源 ${sources.daily_bars || "—"}`;
    }
    case "bars": {
      const bars = Array.isArray(body.bars) ? body.bars : [];
      const last = bars.length ? bars[bars.length - 1].trade_date : "";
      return `${bars.length} 根${last ? " · 最新 " + last : ""}`;
    }
    case "profile":
      return [
        body.name || "—",
        `行业 ${body.industry || "未提供"}`,
        `PE ${finiteNumberOrNull(body.pe_dynamic) ?? "—"}`,
        `PB ${finiteNumberOrNull(body.pb) ?? "—"}`
      ].join(" · ");
    case "boards":
      return `${arrayLength(body.items) ?? 0} 个板块`;
    case "tradingStatus":
      return `${body.trade_date || "—"} · 停牌 ${body.suspension === true ? "是" : body.suspension === false ? "否" : "未确认"}`;
    case "priceLimitRule":
      return `${body.board || "—"} · 规则 ${body.rule_version || "—"} · 判定 ${body.status || "—"}`;
    case "announcements":
      return `${arrayLength(body.items) ?? 0} 条公告`;
    case "fundamentals":
      return `${arrayLength(body.sections) ?? 0} 个指标分区 · has_data=${body.has_data === true}`;
    case "chip":
      return body.has_data === false ? "数据源未确认筹码分布" : `区间 ${arrayLength(body.bins ?? body.items) ?? "—"}`;
    case "capitalFlow":
      return body.has_data === false ? "数据源未返回资金流" : `记录 ${arrayLength(body.items ?? body.points) ?? "—"}`;
    case "valuations":
      return `${arrayLength(body.items) ?? 0} 条估值记录`;
    default:
      return Object.keys(body).slice(0, 5).join(", ");
  }
}

// 只有 HTTP 成功且质量不落在降级集合、且未显式 has_data=false 才算通过。
// 同时接受原始 body 与已提取的 quality：导出与渲染拿到的可能是同一行的不同投影。
// 各端点的降级理由分散在 quality_reasons（quote/profile 等）与 warnings
// （bars/bundle）两个字段；诊断必须把两者都取出来，否则只显示"降级"没有原因。
export function extractQualityReasons(body) {
  const own = (value) => [
    ...(Array.isArray(value?.quality_reasons) ? value.quality_reasons : []),
    ...(Array.isArray(value?.warnings) ? value.warnings : [])
  ];
  // fundamentals 把原因放在 sections 子项上，快照层本身为空，必须下探一层。
  const nested = Array.isArray(body?.sections)
    ? body.sections.flatMap((section) => (Array.isArray(section?.quality_reasons) ? section.quality_reasons : []))
    : [];
  return [...new Set([...own(body), ...nested].filter((value) => typeof value === "string" && value.trim()))];
}

// 原因码保留英文原样供排查；页面上再叠一句中文，避免只看到一屏 code。
const REASON_TEXT = Object.freeze({
  daily_bar_provider_fallback_used: "主源失败，已改用备选源",
  quote_older_than_24_hours_outside_trading_clock: "报价超过 24 小时，非实时",
  no_provider_returned_a_fresh_timestamped_quote: "没有数据源返回新鲜报价",
  volume_unit_inferred_from_turnover_metrics: "成交量单位由换手率推断",
  volume_ratio_unsupported_by_tencent_quote_contract: "该源不提供量比",
  realtime_quote_quality_stale: "实时报价已过期",
  market_profile_timestamp_not_exposed: "数据源未提供快照时间",
  provider_snapshot_timestamp_not_exposed: "数据源未提供快照时间",
  market_profile_quote_supplement_unavailable: "行情补充不可用（行情域名不可达）",
  one_or_more_metrics_missing: "部分指标缺失",
  security_trading_status_provider_unavailable: "交易状态数据源不可用",
  security_trading_status_evidence_missing: "缺少交易状态证据",
  price_limit_event_evidence_missing: "缺少涨跌停事件证据",
  price_limit_special_exception_status_unverified: "特殊例外状态未验证",
  eastmoney_valuation_pe_mrq_not_supported: "该源不提供 PE(MRQ)",
  eastmoney_valuation_turnover_rate_not_supported: "该源不提供换手率",
  eastmoney_valuation_volume_ratio_not_supported: "该源不提供量比",
  eastmoney_valuation_dividend_yield_not_supported: "该源不提供股息率",
  eastmoney_valuation_provider_snapshot_time_not_exposed: "该源未提供快照时间",
  eastmoney_valuation_missing_symbols: "部分标的无估值记录"
});

export function explainReason(code) {
  const text = REASON_TEXT[String(code || "").trim()];
  return text ? text + "（" + code + "）" : String(code || "");
}

export function classifyDiagnosticRow({ ok, body, quality }) {
  if (!ok) return "failed";
  if (DEGRADED_QUALITY.has(String(quality || body?.quality_status || "").toLowerCase())) return "degraded";
  if (body?.has_data === false) return "degraded";
  return "ok";
}

export function summarizeDiagnosticRows(rows) {
  const summary = { ok: 0, degraded: 0, failed: 0, total: 0, degradedItems: [], failedItems: [] };
  for (const row of Array.isArray(rows) ? rows : []) {
    const level = classifyDiagnosticRow(row);
    summary[level] += 1;
    summary.total += 1;
    if (level === "degraded") summary.degradedItems.push(row.label || row.key);
    if (level === "failed") summary.failedItems.push(row.label || row.key);
  }
  return summary;
}

export const STOCK_DIAGNOSTIC_CASES = Object.freeze([
  { key: "quote", label: "实时行情", invoke: (api, ctx) => api.getQuote(ctx.settings, ctx.symbol, ctx.signal) },
  { key: "bundle", label: "聚合快照", invoke: (api, ctx) => api.getMarketBundle(ctx.settings, ctx.symbol, { lookbackDays: 20, adjustment: "none" }, ctx.signal) },
  { key: "bars", label: "日线", invoke: (api, ctx) => api.getDailyBars(ctx.settings, ctx.symbol, { limit: 20, adjustment: "none" }, ctx.signal) },
  { key: "profile", label: "公司概况", invoke: (api, ctx) => api.getStockProfile(ctx.settings, ctx.symbol, ctx.signal) },
  { key: "boards", label: "所属板块", invoke: (api, ctx) => api.getStockBoards(ctx.settings, ctx.symbol, ctx.signal) },
  { key: "tradingStatus", label: "交易状态", invoke: (api, ctx) => api.getStockTradingStatus(ctx.settings, ctx.symbol, ctx.tradeDate, { signal: ctx.signal }) },
  { key: "priceLimitRule", label: "涨跌停规则", invoke: (api, ctx) => api.getStockPriceLimitRule(ctx.settings, ctx.symbol, ctx.tradeDate, ctx.signal) },
  { key: "announcements", label: "公告", invoke: (api, ctx) => api.getStockAnnouncements(ctx.settings, ctx.symbol, { page: 1, pageSize: 5, signal: ctx.signal }) },
  { key: "fundamentals", label: "财务指标", invoke: (api, ctx) => api.getStockFundamentals(ctx.settings, ctx.symbol, { year: ctx.year, quarter: ctx.quarter, signal: ctx.signal }) },
  { key: "chip", label: "筹码分布", invoke: (api, ctx) => api.getChipDistribution(ctx.settings, ctx.symbol, ctx.signal) },
  { key: "capitalFlow", label: "个股资金流", invoke: (api, ctx) => api.getCapitalFlow(ctx.settings, ctx.symbol, ctx.signal) },
  { key: "valuations", label: "估值指标", invoke: (api, ctx) => api.getMarketValuations(ctx.settings, [ctx.symbol], ctx.signal) }
]);

// 诊断默认取最近一个工作日（跳过周六周日）；节假日由服务端健康性检查兜底。
export function recentTradingDay(from = new Date()) {
  const day = new Date(from.getFullYear(), from.getMonth(), from.getDate());
  for (let i = 0; i < 10; i += 1) {
    const weekday = day.getDay();
    if (weekday !== 0 && weekday !== 6) {
      const month = String(day.getMonth() + 1).padStart(2, "0");
      const date = String(day.getDate()).padStart(2, "0");
      return `${day.getFullYear()}-${month}-${date}`;
    }
    day.setDate(day.getDate() - 1);
  }
  return "";
}

// 报告期使用“最近一个已结束季度”：交易日所在季度通常尚未披露，
// 直接传当前季度会被服务端以“报告期不得晚于当前”拒绝。
export function quarterOfDate(tradeDate, fallback = new Date()) {
  const match = /^(\d{4})-(\d{2})-\d{2}$/.exec(String(tradeDate || ""));
  const year = match ? Number(match[1]) : fallback.getUTCFullYear();
  const month = match ? Number(match[2]) : fallback.getUTCMonth() + 1;
  const previous = Math.floor((month - 1) / 3);
  return previous === 0 ? { year: year - 1, quarter: 4 } : { year, quarter: previous };
}

export async function collectStockDiagnostics({
  api, settings, symbol, tradeDate, signal, cases = STOCK_DIAGNOSTIC_CASES, now = () => Date.now()
}) {
  const normalized = normalizeSymbol(symbol);
  if (!normalized) throw new Error("股票代码无效，需为六位代码加 .SH/.SZ/.BJ 后缀");
  const { year, quarter } = quarterOfDate(tradeDate);
  const ctx = { settings, symbol: normalized, tradeDate, year, quarter, signal };
  const rows = await Promise.all(cases.map(async (item) => {
    const started = now();
    try {
      const body = await item.invoke(api, ctx);
      return {
        key: item.key, label: item.label, ok: true, status: 200,
        durationMs: Math.max(0, now() - started), body,
        detail: describeDiagnosticBody(item.key, body), errorText: "",
        quality: String(body?.quality_status || ""), reasons: extractQualityReasons(body)
      };
    } catch (error) {
      return {
        key: item.key, label: item.label, ok: false, status: Number(error?.status) || 0,
        durationMs: Math.max(0, now() - started), body: null, detail: "",
        errorText: describeApiError(error, "请求失败") + (error?.code ? " (" + error.code + ")" : ""),
        quality: "", reasons: []
      };
    }
  }));
  return { symbol: normalized, tradeDate, rows, summary: summarizeDiagnosticRows(rows) };
}

// 必须与后端 service.isStockScopedProbeCapability 保持一致：只有这些能力允许带
// symbols；trading_calendar 等市场级能力带 symbols 会被服务端以 400 拒绝。
export const STOCK_SCOPED_PROBE_CAPABILITIES = Object.freeze([
  "quote", "daily_bars", "stock_basic", "stock_search", "security_trading_status",
  "price_limit_event", "stock_boards", "announcements", "fundamentals",
  "valuations", "capital_flow", "chip_distribution"
]);

const DEFAULT_DIAGNOSTIC_PROBE_CAPABILITIES = Object.freeze([
  "quote", "daily_bars", "security_trading_status", "stock_basic",
  "valuations", "chip_distribution", "capital_flow"
]);

// 请求里只保留股票范围能力，避免把市场级能力混进来触发 400。
export function buildStockProbeRequest(
  symbol, tradeDate, capabilities = DEFAULT_DIAGNOSTIC_PROBE_CAPABILITIES
) {
  const allowed = (Array.isArray(capabilities) ? capabilities : [])
    .filter((capability) => STOCK_SCOPED_PROBE_CAPABILITIES.includes(capability));
  return {
    capabilities: allowed.length ? allowed : [...DEFAULT_DIAGNOSTIC_PROBE_CAPABILITIES],
    symbols: [normalizeSymbol(symbol)],
    timeout_seconds: 20,
    daily_bar_start_date: shiftDays(tradeDate, -20),
    daily_bar_end_date: tradeDate
  };
}

function shiftDays(date, delta) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(date || ""));
  if (!match) return "";
  const base = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])));
  base.setUTCDate(base.getUTCDate() + delta);
  return base.toISOString().slice(0, 10);
}

export function summarizeProbeRun(run) {
  const results = Array.isArray(run?.check_results) ? run.check_results : [];
  const byProvider = new Map();
  for (const item of results) {
    const name = item.provider || "未知";
    if (!byProvider.has(name)) byProvider.set(name, []);
    byProvider.get(name).push(item);
  }
  return [...byProvider.entries()].map(([provider, checks]) => ({
    provider,
    capability: checks[0]?.capability || "",
    status: checks.some((check) => check.status === "failed") ? "failed"
      : checks.some((check) => check.status === "degraded") ? "degraded"
        : checks.every((check) => check.status === "not_supported") ? "not_supported" : "passed",
    checks: checks.map((check) => ({
      check: check.check, status: check.status,
      errorCode: check.error_code || "", errorMessage: check.error_message || ""
    }))
  }));
}

// 导出只包含诊断结论与错误，不含任何凭据；粘贴给维护者即可复现问题。
export function buildDiagnosticExport({ report, probe = null, generatedAt = "now" } = {}) {
  const rows = Array.isArray(report?.rows) ? report.rows : [];
  const summary = report?.summary || {};
  return JSON.stringify({
    kind: "stock-data-diagnostic",
    version: 1,
    build: DIAGNOSTICS_BUILD,
    generated_at: generatedAt === "now" ? new Date().toISOString() : generatedAt,
    symbol: report?.symbol || "",
    trade_date: report?.tradeDate || "",
    summary: {
      total: summary.total ?? rows.length,
      ok: summary.ok ?? 0,
      degraded: summary.degraded ?? 0,
      failed: summary.failed ?? 0,
      degraded_items: summary.degradedItems || [],
      failed_items: summary.failedItems || []
    },
    checks: rows.map((row) => ({
      key: row.key, label: row.label, ok: Boolean(row.ok),
      http_status: row.status || 0, duration_ms: row.durationMs ?? null,
      quality: row.quality || "", level: classifyDiagnosticRow(row),
      detail: row.detail || "", error: row.errorText || "",
      quality_reasons: Array.isArray(row.reasons) ? row.reasons : extractQualityReasons(row.body)
    })),
    probe: probe
      ? {
        run_id: probe.run_id || "",
        requested_capabilities: probe.requested_capabilities || [],
        requested_symbols: probe.requested_symbols || [],
        groups: summarizeProbeRun(probe)
      }
      : null
  }, null, 2);
}

function el(tag, cls, text) {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
}

function qualityPill(level, label) {
  const pill = el("span", "supplement-quality supplement-quality-" + (level === "ok" ? "valid" : level === "degraded" ? "degraded" : "invalid"), label);
  return pill;
}

export function bindDataDiagnostics({ root = document, getSettings, getSymbol, getTradeDate } = {}) {
  const symbolInput = root.querySelector("#diagnostics-symbol");
  const dateInput = root.querySelector("#diagnostics-date");
  const runButton = root.querySelector("#run-diagnostics");
  const probeButton = root.querySelector("#run-diagnostic-probe");
  const rowsBody = root.querySelector("#diagnostics-rows");
  const summaryNode = root.querySelector("#diagnostics-summary");
  const overallNode = root.querySelector("#diagnostics-overall");
  const probeNode = root.querySelector("#diagnostics-probe-result");
  const statsNode = root.querySelector("#diagnostics-stats");
  const exportNode = root.querySelector("#diagnostics-export");
  const exportText = root.querySelector("#diagnostics-export-text");
  const exportStatus = root.querySelector("#diagnostics-export-status");
  const copyButton = root.querySelector("#copy-diagnostics-json");
  void getSymbol;
  void getTradeDate;
  let lastReport = null;
  let lastProbe = null;

  function readInputs() {
    const symbol = String(symbolInput?.value || "").trim().toUpperCase();
    const tradeDate = String(dateInput?.value || "").trim();
    if (!normalizeSymbol(symbol)) throw new Error("请输入有效股票代码，例如 600519.SH");
    if (!/^\d{4}-\d{2}-\d{2}$/.test(tradeDate)) throw new Error("请选择交易日，格式 YYYY-MM-DD");
    return { symbol, tradeDate };
  }

  function setOverall(level, label) {
    if (!overallNode) return;
    overallNode.className = "availability-pill " + (level === "failed" ? "is-unavailable" : level === "degraded" ? "is-readonly" : "is-local");
    overallNode.textContent = label;
  }

  function renderRows(report) {
    if (!rowsBody) return;
    rowsBody.replaceChildren();
    for (const row of report.rows) {
      const level = classifyDiagnosticRow(row);
      const tr = el("tr");
      tr.append(el("td", "", row.label));
      const resultCell = el("td");
      const resultWrap = el("span", "diagnostics-result");
      resultWrap.dataset.level = level;
      resultWrap.append(el("span", "diagnostics-dot"), el("span", "", `${row.status || "—"} · ${levelWord(level)}`));
      resultCell.append(resultWrap);
      tr.append(resultCell);
      tr.append(el("td", "diagnostics-duration", `${row.durationMs} ms`));
      const qualityCell = el("td");
      qualityCell.append(qualityPill(level, row.quality ? qualityWord(row.quality) : level === "failed" ? "失败" : "未报告"));
      tr.append(qualityCell);
      tr.append(el("td", "", row.detail || "—"));
      const reasonCell = el("td", row.ok ? "" : "diagnostics-error");
      if (!row.ok) reasonCell.textContent = row.errorText || "—";
      else if (row.reasons && row.reasons.length) reasonCell.append(el("span", "diagnostics-reason", row.reasons.map(explainReason).join("；")));
      else reasonCell.textContent = "—";
      tr.append(reasonCell);
      if (!row.ok) tr.classList.add("diagnostics-row-failed");
      else if (level === "degraded") tr.classList.add("diagnostics-row-degraded");
      rowsBody.append(tr);
    }
  }

  function renderStats(summary) {
    if (!statsNode) return;
    statsNode.hidden = false;
    for (const key of ["total", "ok", "degraded", "failed"]) {
      const node = root.querySelector("#diagnostics-stat-" + key);
      if (node) node.textContent = String(summary?.[key] ?? 0);
    }
  }

  function refreshExport() {
    if (!exportNode || !exportText) return;
    if (!lastReport) { exportNode.hidden = true; return; }
    exportNode.hidden = false;
    exportText.value = buildDiagnosticExport({ report: lastReport, probe: lastProbe });
  }

  function setExportStatus(text) {
    if (exportStatus) exportStatus.textContent = text;
  }

  async function copyExport() {
    if (!exportText) return;
    const text = exportText.value;
    if (!text) { setExportStatus("尚无结果可复制"); return; }
    try {
      if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(text);
      setExportStatus("已复制到剪贴板");
    } catch {
      exportText.focus();
      exportText.select();
      setExportStatus("已选中文本，请按 Ctrl/Cmd+C 复制");
    }
  }

  function renderSummary(report) {
    if (!summaryNode) return;
    const { summary } = report;
    const parts = [`共 ${summary.total} 项`, `通过 ${summary.ok}`];
    if (summary.degraded) parts.push(`降级 ${summary.degraded}：${summary.degradedItems.join("、")}`);
    if (summary.failed) parts.push(`失败 ${summary.failed}：${summary.failedItems.join("、")}`);
    summaryNode.textContent = parts.join(" · ");
    setOverall(summary.failed ? "failed" : summary.degraded ? "degraded" : "ok", summary.failed ? "存在失败项" : summary.degraded ? "存在降级项" : "全部通过");
  }

  async function run() {
    let inputs;
    try {
      inputs = readInputs();
    } catch (error) {
      if (summaryNode) summaryNode.textContent = error.message;
      setOverall("failed", "输入无效");
      return null;
    }
    if (runButton) { runButton.disabled = true; runButton.textContent = "诊断中…"; }
    setOverall("degraded", "运行中");
    if (summaryNode) summaryNode.textContent = "正在并发调用该标的相关的全部数据端点…";
    try {
      const report = await collectStockDiagnostics({
        api: { getQuote, getMarketBundle, getDailyBars, getStockProfile, getStockBoards, getStockTradingStatus, getStockPriceLimitRule, getStockAnnouncements, getStockFundamentals, getChipDistribution, getCapitalFlow, getMarketValuations },
        settings: getSettings ? getSettings() : null,
        symbol: inputs.symbol,
        tradeDate: inputs.tradeDate
      });
      lastReport = report;
      renderRows(report);
      renderStats(report.summary);
      renderSummary(report);
      refreshExport();
      return report;
    } catch (error) {
      if (summaryNode) summaryNode.textContent = error.message || "诊断执行失败";
      setOverall("failed", "执行失败");
      return null;
    } finally {
      if (runButton) { runButton.disabled = false; runButton.textContent = "运行诊断"; }
    }
  }

  async function runProbe() {
    let inputs;
    try {
      inputs = readInputs();
    } catch (error) {
      if (probeNode) probeNode.textContent = error.message;
      return null;
    }
    if (probeButton) { probeButton.disabled = true; probeButton.textContent = "探测中…"; }
    if (probeNode) probeNode.textContent = "正在对行情、日线、交易日历与交易状态执行 Provider 级探测…";
    try {
      const run = await runDataSourceProbe(getSettings ? getSettings() : null, buildStockProbeRequest(inputs.symbol, inputs.tradeDate));
      lastProbe = run;
      refreshExport();
      const groups = summarizeProbeRun(run);
      if (probeNode) {
        probeNode.replaceChildren();
        if (!groups.length) {
          probeNode.append(el("p", "", "探测未返回任何 Provider 结果。"));
        } else {
          const table = el("table", "data-table");
          const head = el("tr");
          ["Provider", "能力", "结论", "明细"].forEach((label) => head.append(el("th", "", label)));
          const thead = el("thead");
          thead.append(head);
          const tbody = el("tbody");
          for (const group of groups) {
            const tr = el("tr");
            tr.append(el("td", "", group.provider));
            tr.append(el("td", "", group.capability));
            const statusCell = el("td");
            statusCell.append(qualityPill(group.status === "passed" ? "ok" : group.status === "degraded" ? "degraded" : "failed", group.status));
            tr.append(statusCell);
            tr.append(el("td", "", group.checks.map((check) => {
              const reason = check.errorMessage || check.errorCode;
              return check.check + ":" + check.status + (reason ? "/" + reason : "");
            }).join(" · ")));
            tbody.append(tr);
          }
          table.append(thead, tbody);
          probeNode.append(table);
        }
      }
      return run;
    } catch (error) {
      if (probeNode) probeNode.textContent = describeApiError(error, "探测失败");
      return null;
    } finally {
      if (probeButton) { probeButton.disabled = false; probeButton.textContent = "运行探测"; }
    }
  }

  runButton?.addEventListener("click", () => { void run(); });
  probeButton?.addEventListener("click", () => { void runProbe(); });
  copyButton?.addEventListener("click", () => { void copyExport(); });

  const buildNode = root.querySelector("#diagnostics-build");
  if (buildNode) buildNode.textContent = "前端版本 " + DIAGNOSTICS_BUILD;

  function ensureDefaults() {
    if (dateInput && !String(dateInput.value || "").trim()) dateInput.value = recentTradingDay();
  }

  return { run, runProbe, ensureDefaults, setSymbol: (value) => { if (symbolInput) symbolInput.value = value; }, setTradeDate: (value) => { if (dateInput) dateInput.value = value; } };
}

import test from "node:test";
import assert from "node:assert/strict";
import {
  buildDiagnosticExport,
  buildStockProbeRequest,
  explainReason,
  extractQualityReasons,
  classifyDiagnosticRow,
  collectStockDiagnostics,
  describeDiagnosticBody,
  quarterOfDate,
  recentTradingDay,
  summarizeDiagnosticRows,
  summarizeProbeRun
} from "../src/data-diagnostics.js";

test("classifyDiagnosticRow 区分通过、降级与失败", () => {
  assert.equal(classifyDiagnosticRow({ ok: true, body: { quality_status: "valid" } }), "ok");
  assert.equal(classifyDiagnosticRow({ ok: false, body: null }), "failed");
  for (const quality of ["degraded", "stale", "invalid", "unknown"]) {
    assert.equal(classifyDiagnosticRow({ ok: true, body: { quality_status: quality } }), "degraded", quality);
  }
  assert.equal(classifyDiagnosticRow({ ok: true, body: { has_data: false } }), "degraded");
  assert.equal(classifyDiagnosticRow({ ok: true, body: { has_data: true, quality_status: "valid" } }), "ok");
});

test("summarizeDiagnosticRows 汇总计数并保留问题项名称", () => {
  const summary = summarizeDiagnosticRows([
    { key: "quote", label: "实时行情", ok: true, body: { quality_status: "valid" } },
    { key: "bars", label: "日线", ok: true, body: { quality_status: "degraded" } },
    { key: "chip", label: "筹码分布", ok: false, body: null },
    { key: "valuations", label: "估值指标", ok: false, body: null }
  ]);
  assert.deepEqual(
    { ok: summary.ok, degraded: summary.degraded, failed: summary.failed, total: summary.total },
    { ok: 1, degraded: 1, failed: 2, total: 4 }
  );
  assert.deepEqual(summary.degradedItems, ["日线"]);
  assert.deepEqual(summary.failedItems, ["筹码分布", "估值指标"]);
});

test("describeDiagnosticBody 摘取每类端点的关键证据", () => {
  assert.match(describeDiagnosticBody("quote", { current_price: 1472.36, source: "tencent", quote_time: "2026-11-27T08:14:54Z" }), /价格 1472\.36/);
  assert.match(describeDiagnosticBody("quote", { current_price: 1472.36, source: "tencent" }), /来源 tencent/);
  assert.match(describeDiagnosticBody("profile", { name: "贵州茅台", industry: "白酒", pe_dynamic: 22.4, pb: 6.78 }), /行业 白酒 · PE 22\.4 · PB 6\.78/);
  assert.match(describeDiagnosticBody("bars", { bars: [{ trade_date: "2026-11-26" }, { trade_date: "2026-11-27" }] }), /2 根 · 最新 2026-11-27/);
  assert.match(describeDiagnosticBody("boards", { items: new Array(32) }), /32 个板块/);
  assert.match(describeDiagnosticBody("announcements", { items: new Array(2) }), /2 条公告/);
  assert.match(describeDiagnosticBody("fundamentals", { sections: new Array(6), has_data: true }), /6 个指标分区/);
  assert.match(describeDiagnosticBody("priceLimitRule", { board: "main_board", rule_version: "cn-mainboard-10-percent-current-v2", status: "unknown" }), /main_board · 规则 cn-mainboard-10-percent-current-v2 · 判定 unknown/);
  assert.match(describeDiagnosticBody("chip", { has_data: false }), /未确认筹码分布/);
  assert.match(describeDiagnosticBody("bundle", { daily_bars: new Array(20), sources: { realtime_quote: "tencent", daily_bars: "baostock" } }), /日线 20 根 · 行情源 tencent · 日线源 baostock/);
  assert.equal(describeDiagnosticBody("quote", null), "无响应体");
});

test("quarterOfDate 由交易日推导报告期，非法输入回退当前日期", () => {
  assert.deepEqual(quarterOfDate("2026-11-27"), { year: 2026, quarter: 3 });
  assert.deepEqual(quarterOfDate("2026-01-05"), { year: 2025, quarter: 4 });
  const fallback = new Date("2026-07-15T00:00:00Z");
  assert.deepEqual(quarterOfDate("", fallback), { year: 2026, quarter: 2 });
});

test("recentTradingDay 跳过周六周日", () => {
  assert.equal(recentTradingDay(new Date(2026, 10, 28)), "2026-11-27");
  assert.equal(recentTradingDay(new Date(2026, 10, 27)), "2026-11-27");
  assert.equal(recentTradingDay(new Date(2026, 10, 29)), "2026-11-27");
  assert.equal(recentTradingDay(new Date(2026, 10, 1)), "2026-10-30");
});

test("buildStockProbeRequest 只带股票范围能力并给出日线窗口", () => {
  const request = buildStockProbeRequest("600519.SH", "2026-11-27");
  assert.deepEqual(request.symbols, ["600519.SH"]);
  // trading_calendar 是市场级能力，带 symbols 会被服务端 400 拒绝。
  assert.equal(request.capabilities.includes("trading_calendar"), false);
  assert.equal(request.capabilities.includes("security_trading_status"), true);
  assert.equal(request.daily_bar_end_date, "2026-11-27");
  assert.equal(request.daily_bar_start_date, "2026-11-07");
});

test("buildStockProbeRequest 过滤掉非股票范围能力", () => {
  const request = buildStockProbeRequest("600519.SH", "2026-11-27", ["quote", "trading_calendar", "sector_rankings"]);
  assert.deepEqual(request.capabilities, ["quote"]);
});

test("collectStockDiagnostics 并发采集、区分成功与失败并保留错误码", async () => {
  const calls = [];
  const api = {
    getQuote: async (settings, symbol) => { calls.push(["quote", symbol]); return { current_price: 10, source: "tencent", quality_status: "valid" }; },
    getChipDistribution: async () => { calls.push(["chip"]); throw Object.assign(new Error("上游返回无效响应"), { status: 502, code: "PROVIDER_BAD_RESPONSE" }); }
  };
  const report = await collectStockDiagnostics({
    api,
    settings: {},
    symbol: "600519.SH",
    tradeDate: "2026-11-27",
    cases: [
      { key: "quote", label: "实时行情", invoke: (candidate, ctx) => candidate.getQuote(ctx.settings, ctx.symbol, ctx.signal) },
      { key: "chip", label: "筹码分布", invoke: (candidate) => candidate.getChipDistribution() }
    ]
  });
  assert.equal(report.symbol, "600519.SH");
  assert.equal(report.rows.length, 2);
  const quote = report.rows.find((row) => row.key === "quote");
  const chip = report.rows.find((row) => row.key === "chip");
  assert.equal(classifyDiagnosticRow(quote), "ok");
  assert.match(quote.detail, /价格 10/);
  assert.equal(chip.ok, false);
  assert.equal(chip.status, 502);
  assert.match(chip.errorText, /PROVIDER_BAD_RESPONSE/);
  assert.deepEqual(report.summary.failedItems, ["筹码分布"]);
  assert.deepEqual(calls, [["quote", "600519.SH"], ["chip"]]);
});

test("collectStockDiagnostics 拒绝无效股票代码", async () => {
  await assert.rejects(
    () => collectStockDiagnostics({ api: {}, settings: {}, symbol: "茅台", tradeDate: "2026-11-27" }),
    /股票代码/
  );
});

test("summarizeProbeRun 按 Provider 聚合检查结论", () => {
  const groups = summarizeProbeRun({
    check_results: [
      { provider: "baostock", capability: "daily_bars", check: "connectivity", status: "passed" },
      { provider: "baostock", capability: "daily_bars", check: "quality", status: "degraded" },
      { provider: "eastmoney", capability: "daily_bars", check: "connectivity", status: "failed", error_code: "provider_error" }
    ]
  });
  const baostock = groups.find((group) => group.provider === "baostock");
  const eastmoney = groups.find((group) => group.provider === "eastmoney");
  assert.equal(baostock.status, "degraded");
  assert.equal(eastmoney.status, "failed");
  assert.match(eastmoney.checks[0].errorCode, /provider_error/);
});

test("buildDiagnosticExport 输出可直接粘贴的结构化结论", () => {
  const report = {
    symbol: "600519.SH", tradeDate: "2026-11-27",
    rows: [
      { key: "quote", label: "实时行情", ok: true, status: 200, durationMs: 870, quality: "stale", detail: "价格 1472.36", errorText: "" },
      { key: "chip", label: "筹码分布", ok: false, status: 502, durationMs: 278, quality: "", detail: "", errorText: "upstream invalid (PROVIDER_BAD_RESPONSE)" }
    ],
    summary: { total: 2, ok: 0, degraded: 1, failed: 1, degradedItems: ["实时行情"], failedItems: ["筹码分布"] }
  };
  const parsed = JSON.parse(buildDiagnosticExport({ report, generatedAt: "2026-11-28T14:00:00Z" }));
  assert.equal(parsed.kind, "stock-data-diagnostic");
  assert.equal(parsed.version, 1);
  assert.equal(parsed.symbol, "600519.SH");
  assert.equal(parsed.trade_date, "2026-11-27");
  assert.equal(parsed.checks.length, 2);
  assert.equal(parsed.checks[0].level, "degraded");
  assert.equal(parsed.checks[1].level, "failed");
  assert.match(parsed.checks[1].error, /PROVIDER_BAD_RESPONSE/);
  assert.deepEqual(parsed.summary.failed_items, ["筹码分布"]);
  assert.equal(parsed.probe, null);
  assert.doesNotMatch(JSON.stringify(parsed), /cookie|authorization/i);
});

test("buildDiagnosticExport 附带探测分组与 provider 错误原因", () => {
  const parsed = JSON.parse(buildDiagnosticExport({
    report: { symbol: "600519.SH", tradeDate: "2026-11-27", rows: [], summary: { total: 0 } },
    probe: {
      run_id: "r1", requested_capabilities: ["quote"], requested_symbols: ["600519.SH"],
      check_results: [{ provider: "tushare", capability: "security_trading_status", check: "connectivity", status: "failed", error_code: "provider_error", error_message: "permission_required (40203)" }]
    },
    generatedAt: "2026-11-28T14:00:00Z"
  }));
  assert.equal(parsed.probe.groups.length, 1);
  assert.equal(parsed.probe.groups[0].checks[0].errorMessage, "permission_required (40203)");
});

test("extractQualityReasons 同时取 quality_reasons 与 warnings", () => {
  assert.deepEqual(
    extractQualityReasons({ quality_reasons: ["a", "b"], warnings: ["b", "c"] }),
    ["a", "b", "c"]
  );
  assert.deepEqual(extractQualityReasons({ warnings: ["daily_bar_provider_fallback_used"] }), ["daily_bar_provider_fallback_used"]);
  assert.deepEqual(extractQualityReasons({}), []);
  assert.deepEqual(extractQualityReasons(null), []);
});

test("collectStockDiagnostics 把 bars 的 warnings 记为降级原因", async () => {
  const report = await collectStockDiagnostics({
    api: {
      getDailyBars: async () => ({ quality_status: "degraded", warnings: ["daily_bar_provider_fallback_used"], bars: [{ trade_date: "2026-11-27" }] })
    },
    settings: {}, symbol: "600519.SH", tradeDate: "2026-11-27",
    cases: [{ key: "bars", label: "日线", invoke: (api) => api.getDailyBars() }]
  });
  assert.deepEqual(report.rows[0].reasons, ["daily_bar_provider_fallback_used"]);
  const parsed = JSON.parse(buildDiagnosticExport({ report, generatedAt: "2026-11-28T14:00:00Z" }));
  assert.deepEqual(parsed.checks[0].quality_reasons, ["daily_bar_provider_fallback_used"]);
  assert.equal(typeof parsed.build, "string");
});

test("extractQualityReasons 下探 fundamentals 的 sections 层", () => {
  assert.deepEqual(
    extractQualityReasons({ quality_status: "degraded", sections: [{ quality_reasons: ["one_or_more_metrics_missing"] }, {}] }),
    ["one_or_more_metrics_missing"]
  );
});

test("explainReason 保留原因码并附中文说明", () => {
  assert.match(explainReason("daily_bar_provider_fallback_used"), /主源失败.*daily_bar_provider_fallback_used/);
  assert.equal(explainReason("some_unknown_code"), "some_unknown_code");
  assert.equal(explainReason(""), "");
});

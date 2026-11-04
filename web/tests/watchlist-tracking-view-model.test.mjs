import test from "node:test";
import assert from "node:assert/strict";
import { buildWatchlistTrackingFields } from "../src/watchlist-tracking-view-model.js";

const fixedNow = new Date("2026-11-04T09:00:00");
const formatters = {
  normalizeSymbol: (value) => String(value || "").trim().toUpperCase(),
  dateKey: (value) => {
    const date = value instanceof Date ? value : new Date(String(value || ""));
    if (!Number.isFinite(date.getTime())) return "";
    return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, "0"), String(date.getDate()).padStart(2, "0")].join("-");
  },
  formatDateTime: (value) => value ? "DT(" + value + ")" : "",
  formatPrice: (value) => "P(" + value + ")",
  formatPercent: (value) => "C(" + value + ")",
  qualityLabel: (value) => "Q(" + (value || "unknown") + ")",
  taskStatusLabel: (value) => "T(" + (value || "unknown") + ")",
  runStatusLabel: (value) => "R(" + (value || "unknown") + ")"
};
const fields = ["updated", "quote", "today", "task", "result"];
const record = { symbol: "600000.SH", updated_at: "2026-11-03T10:00:00Z" };
function build(overrides = {}) {
  return buildWatchlistTrackingFields({
    record,
    batchQuote: null,
    currentQuote: null,
    quoteStatus: "idle",
    recentTasks: [],
    currentTask: {},
    runResult: null,
    currentAnalysisBase: "https://api.new",
    displayOrder: fields,
    displayFields: fields,
    now: fixedNow,
    formatters,
    ...overrides
  });
}
function byKey(result, key) { return result.find((field) => field.key === key); }

test("批量行情错误优先展示错误码与质量状态，不覆盖旧快照文案", () => {
  const result = build({
    batchQuote: { errorCode: "PROVIDER_TIMEOUT", qualityStatus: "degraded", quote: { current_price: 10 } },
    currentQuote: { current_price: 10, change_percent: 1, quote_time: "2026-11-04T08:00:00Z" },
    quoteStatus: "error"
  });
  assert.equal(byKey(result, "quote").text, "批量行情不可用：PROVIDER_TIMEOUT · Q(degraded)");
});

test("批量刷新失败时保留行情快照并明确标记上次成功快照", () => {
  const result = build({
    batchQuote: { qualityStatus: "good", fetchedAt: "2026-11-04T08:00:00Z", quote: { current_price: 10 } },
    currentQuote: { current_price: 10, change_percent: 1, quote_time: "2026-11-04T08:00:00Z", quality_status: "good" },
    quoteStatus: "error"
  });
  assert.match(byKey(result, "quote").text, /^上次成功快照：/);
  assert.match(byKey(result, "quote").title, /批量采集/);
});

test("行情缺失、加载中与本次批量响应缺项保持不同提示", () => {
  assert.match(byKey(build({ quoteStatus: "loading" }), "quote").text, /批量行情读取中/);
  assert.match(byKey(build({ quoteStatus: "loaded" }), "quote").text, /未包含该证券，质量未知/);
  assert.match(byKey(build(), "quote").text, /未加载/);
});

test("无本地任务引用不推断服务端从未分析", () => {
  const result = build();
  assert.match(byKey(result, "today").text, /未知（当前浏览器没有该股票的任务引用）/);
  assert.match(byKey(result, "task").text, /不代表从未分析/);
});

test("今日任务、旧分析 API 与任务状态标签保留", () => {
  const task = { task_id: "task-1", run_id: "run-1", symbols: ["600000"], created_at: "2026-11-04T01:00:00Z", updated_at: "2026-11-04T01:00:00Z", status: "running", analysis_base: "https://api.old" };
  const result = build({ recentTasks: [task] });
  assert.match(byKey(result, "today").text, /今日分析：有本地记录（旧分析 API 地址）/);
  assert.match(byKey(result, "task").text, /旧分析 API 地址/);
});

test("旧任务缺少服务端创建时间时今日状态未知", () => {
  const result = build({ recentTasks: [{ task_id: "old", symbols: ["600000"], updated_at: "2026-11-03", status: "failed" }] });
  assert.match(byKey(result, "today").text, /未知（部分旧任务缺少服务端创建时间）/);
});

test("当前任务状态读取中和读取失败保留上次状态及重试语义", () => {
  const base = { taskId: "task-current", symbols: ["600000"], status: "running" };
  assert.match(byKey(build({ currentTask: { ...base, statusLoading: true } }), "task").text, /正在确认服务端状态（上次状态：T\(running\)）/);
  assert.match(byKey(build({ currentTask: { ...base, statusError: "timeout", statusErrorRetrying: true } }), "task").text, /状态读取异常（上次状态：T\(running\)，自动重试中）/);
});

test("Run 结果匹配任务、Run、连接和证券；重读失败时明确保留旧结果", () => {
  const recentTask = { task_id: "task-current", run_id: "run-current", symbols: ["600000"], created_at: "2026-11-04T01:00:00Z", updated_at: "2026-11-04T01:00:00Z", status: "succeeded", analysis_base: "https://api.new" };
  const currentTask = { taskId: "task-current", runId: "run-current", analysisBase: "https://api.new", symbols: ["600000"], status: "succeeded" };
  const runResult = { run_id: "run-current", status: "completed", quality: { core_pass: false }, warnings: ["warning"] };
  const matching = build({ recentTasks: [recentTask], currentTask, runResult, runResultReadStatus: "error", runResultReadError: "read failed" });
  assert.match(byKey(matching, "result").text, /本页保留旧结果（本次读取失败）/);
  assert.match(byKey(matching, "result").text, /核心质量门禁未通过/);
  assert.match(byKey(matching, "result").text, /重读提示：read failed/);

  const wrongRun = build({ recentTasks: [recentTask], currentTask, runResult: { ...runResult, run_id: "other-run" } });
  assert.equal(byKey(wrongRun, "result").text, "结果详情：尚未载入当前页面");
  const wrongConnection = build({ recentTasks: [recentTask], currentTask: { ...currentTask, analysisBase: "https://api.old" }, runResult });
  assert.equal(byKey(wrongConnection, "result").text, "结果详情：尚未载入当前页面");
  const wrongSymbol = build({ recentTasks: [recentTask], currentTask: { ...currentTask, symbols: ["000001"] }, runResult });
  assert.equal(byKey(wrongSymbol, "result").text, "结果详情：尚未载入当前页面");
});

test("当前 Run 正在读取/重试/失败时，即使旧任务没有已载入结果也明确提示", () => {
  const recentTask = { task_id: "task-current", run_id: "run-current", symbols: ["600000"], analysis_base: "https://api.new" };
  const currentTask = { taskId: "task-current", analysisBase: "https://api.new", symbols: ["600000"] };
  assert.match(byKey(build({ recentTasks: [recentTask], currentTask, runResultReadStatus: "loading" }), "result").text, /正在读取服务端结果/);
  assert.match(byKey(build({ recentTasks: [recentTask], currentTask, runResultReadStatus: "retrying" }), "result").text, /自动重试/);
  assert.match(byKey(build({ recentTasks: [recentTask], currentTask, runResultReadStatus: "error", runResultReadError: "unavailable" }), "result").text, /读取失败 · unavailable/);
});

test("展示顺序和可见性仍由调用方偏好控制", () => {
  const result = build({ displayOrder: ["result", "quote", "today"], displayFields: ["quote", "today"] });
  assert.deepEqual(result.map((field) => field.key), ["quote", "today"]);
});

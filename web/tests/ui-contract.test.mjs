import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [html, app, format, domain, openapi, recentTaskModule, stylesheet, watchlistModule] = await Promise.all([
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("../src/format.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/analysis/domain/types.go", import.meta.url), "utf8"),
  readFile(new URL("../../docs/architecture/analysis-tasks.openapi.yaml", import.meta.url), "utf8"),
  readFile(new URL("../src/recent-tasks.js", import.meta.url), "utf8"),
  readFile(new URL("../styles.css", import.meta.url), "utf8"),
  readFile(new URL("../src/watchlist.js", import.meta.url), "utf8")
]);

test("static DOM IDs referenced by the WebUI exist and are unique", () => {
  const htmlIds = [...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]);
  assert.equal(htmlIds.length, new Set(htmlIds).size, "HTML must not contain duplicate id attributes");

  const idSet = new Set(htmlIds);
  const staticSelectors = [...app.matchAll(/\$\(["']#([a-zA-Z][\w-]*)["']\)/g)].map((match) => match[1]);
  const missing = [...new Set(staticSelectors.filter((id) => !idSet.has(id)))];
  assert.deepEqual(missing, [], "app.js references missing static DOM IDs: " + missing.join(", "));
});

test("structured result renderer uses fields declared by Go RunResult", () => {
  const requiredFields = [
    "market_state",
    "factors",
    "signals",
    "tradability",
    "risk",
    "plans",
    "planning_rejections",
    "execution_assessment",
    "failure",
    "warnings"
  ];
  for (const field of requiredFields) {
    assert.match(domain, new RegExp('json:"' + field + '(?:,[^"]*)?"'), "Go domain model must declare " + field);
    assert.ok(app.includes("result." + field), "WebUI must render result." + field);
  }
  assert.doesNotMatch(app, /signal\.(score|confidence|rationale|reason)\b/, "do not display fields absent from CandidateSignal");
  assert.doesNotMatch(app, /completed_stages\s*\?\?\s*0/, "missing stage counts must not be fabricated as zero");
  assert.match(html, /id="task-create-feedback"/, "task submission errors should be shown inline");
});

test("task/event endpoint parameters and status vocabulary remain grounded in the API contract", () => {
  assert.match(openapi, /name:\s*after_sequence/);
  assert.match(openapi, /name:\s*limit/);
  for (const status of ["queued", "running", "succeeded", "partial", "failed"]) {
    assert.match(format, new RegExp("\\b" + status + "\\s*:"), "task status " + status + " must have a UI label");
  }
  for (const status of ["running", "completed", "blocked_data_quality", "rejected_request", "partial", "failed"]) {
    assert.match(format, new RegExp("\\b" + status + "\\s*:"), "run status " + status + " must have a UI label");
  }
});


test("task restoration validates local references and guards stale asynchronous responses", () => {
  assert.match(app, /function restoreLastTask\(\)[\s\S]*?JSON\.parse\(localStorage\.getItem\(TASK_REFERENCE_KEY\) \|\| "null"\)[\s\S]*?catch \{[\s\S]*?return false;[\s\S]*?return activateTaskReference\(record\);/);
  assert.match(app, /function activateTaskReference\(record\)[\s\S]*?if \(!record \|\| typeof record\.task_id !== "string" \|\| !record\.task_id\.trim\(\)\) return false;[\s\S]*?stopTaskPolling\(\);[\s\S]*?state\.taskEvents\.clear\(\);[\s\S]*?void pollTask\(state\.pollGeneration\);/);
  assert.match(app, /if \(generation !== state\.pollGeneration \|\| taskId !== state\.taskId\) return;/);
  assert.match(app, /if \(generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId\) return;/);
  assert.match(app, /if \(TERMINAL_TASK_STATUSES\.has\(task\.status\)\) \{[\s\S]*?stopTaskPolling\(\);[\s\S]*?if \(runId\) void loadTaskRunResult\(runId, taskId\);/);
  assert.match(recentTaskModule, /return \{[\s\S]*?task_id: record\.task_id\.trim\(\)[\s\S]*?status: typeof record\.status === "string" \? record\.status : ""/);
});


test("polling pauses on permanent API errors and Run reads use bounded, stale-safe retries", () => {
  assert.match(app, /if \(!isRetryableApiError\(error\)\) \{[\s\S]*?自动刷新已暂停[\s\S]*?return;[\s\S]*?state\.pollTimer = window\.setTimeout\(\(\) => pollTask\(generation\), 5000\);/);
  assert.match(app, /const RUN_RESULT_RETRY_DELAYS_MS = \[1000, 2500, 5000\]/);
  assert.match(app, /generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId/);
  assert.match(app, /state\.runResultGeneration \+= 1/);
  assert.match(app, /isRetryableApiError\(error\)/);
});


test("structured analysis results use readable typography and mobile-safe layouts", () => {
  assert.match(stylesheet, /\.task-result-metric small \{[^}]*font-size: 11px/);
  assert.match(stylesheet, /\.task-result-metric strong \{[^}]*font-size: 14px/);
  assert.match(stylesheet, /\.analysis-result-block > h3 \{[^}]*font-size: 14px/);
  assert.match(stylesheet, /\.analysis-result-fields dt \{[^}]*font-size: 11px/);
  assert.match(stylesheet, /\.analysis-result-fields dd \{[^}]*font-size: 12px/);
  assert.match(stylesheet, /\.task-result-actions \.text-button \{[^}]*font-size: 11px/);
  assert.match(stylesheet, /@media \(max-width: 600px\)[\s\S]*?\.analysis-result-grid \{ grid-template-columns: minmax\(0, 1fr\); \}/);
  assert.match(stylesheet, /overflow-wrap: anywhere/);
});


test("browser-local watchlist is wired to existing query/analysis flows and stores only references", () => {
  for (const id of ["add-to-watchlist-button", "watchlist-count", "watchlist-feedback", "watchlist-list"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "watchlist control must exist: " + id);
  }
  assert.match(app, /function renderWatchlist\(/);
  assert.match(app, /addWatchlistSymbol\(symbol, defaultName, localStorage\)/);
  assert.match(app, /removeWatchlistSymbol\(symbol, localStorage\)/);
  assert.match(app, /renameWatchlistSymbol\(record\.symbol, aliasInput\.value, localStorage\)/);
  assert.match(app, /function startAnalysisForSymbol\(input\)/);
  assert.match(app, /state\.isQuerying[\s\S]*?当前行情查询尚未完成/);
  assert.match(watchlistModule, /WATCHLIST_KEY/);
  assert.doesNotMatch(watchlistModule, /apiKey|runResult|report\s*:/i, "watchlist persistence must not include secrets or report payloads");
  assert.match(stylesheet, /\.watchlist-item\s*\{/);
  assert.match(stylesheet, /@media \(max-width: 800px\)[\s\S]*?\.watchlist-list \{ grid-template-columns: minmax\(0, 1fr\); \}/);
});


test("daily chart exposes close/candle modes, optional date filters and adjustment basis", () => {
  for (const id of ["bar-start-date", "bar-end-date", "chart-title", "chart-view-close", "chart-view-candles", "chart-legend-label"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "chart/date control must exist: " + id);
  }
  assert.match(app, /function renderChartView\(\)/);
  assert.match(app, /renderCandlestickVolumeChart\(\$\("#close-chart"\), state\.bars, state\.chartMeta\)/);
  assert.match(app, /function shanghaiDateKey\(/);
  assert.match(app, /开始日期不能晚于结束日期/);
  assert.match(app, /日 K 查询失败，旧标的图表已清除/);
  assert.match(stylesheet, /\.chart-candle-body/);
  assert.match(stylesheet, /\.chart-volume-bar/);
  assert.match(stylesheet, /\.chart-view-button\.is-active/);
});

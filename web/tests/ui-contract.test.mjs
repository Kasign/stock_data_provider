import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [html, app, format, domain, openapi, recentTaskModule, stylesheet, watchlistModule, apiClient, taskQueue, marketActivity, stockResearch, marketOverview, serviceWiring] = await Promise.all([
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("../src/format.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/analysis/domain/types.go", import.meta.url), "utf8"),
  readFile(new URL("../../docs/architecture/analysis-tasks.openapi.yaml", import.meta.url), "utf8"),
  readFile(new URL("../src/recent-tasks.js", import.meta.url), "utf8"),
  readFile(new URL("../styles.css", import.meta.url), "utf8"),
  readFile(new URL("../src/watchlist.js", import.meta.url), "utf8"),
  readFile(new URL("../src/api.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/platform/taskqueue/queue.go", import.meta.url), "utf8"),
  readFile(new URL("../src/market-activity.js", import.meta.url), "utf8"),
  readFile(new URL("../src/stock-research.js", import.meta.url), "utf8"),
  readFile(new URL("../src/market-overview.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/service/wiring.go", import.meta.url), "utf8")
]);

function finalCssRuleBody(selector) {
  const start = stylesheet.lastIndexOf(selector);
  assert.notEqual(start, -1, "stylesheet should contain " + selector);
  const open = stylesheet.indexOf("{", start + selector.length);
  const close = stylesheet.indexOf("}", open + 1);
  assert.ok(open > start && close > open, "stylesheet should have a complete rule for " + selector);
  return stylesheet.slice(open + 1, close);
}

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

test("Go rejected Run status maps to a failed task instead of false success", () => {
  assert.match(domain, /RunStatusRejected\s+RunStatus\s*=\s*"rejected_request"/);
  const mapper = taskQueue.match(/func taskStatusForRun\(status domain\.RunStatus\) string \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(mapper, "Go task queue must have an explicit Run-to-Task status mapper");
  assert.match(mapper, /case domain\.RunStatusFailed,\s*domain\.RunStatusRejected:\s*return "failed"/);
});


test("task restoration validates local references and guards stale asynchronous responses", () => {
  assert.match(app, /function restoreLastTask\(\)[\s\S]*?JSON\.parse\(localStorage\.getItem\(TASK_REFERENCE_KEY\) \|\| "null"\)[\s\S]*?catch \{[\s\S]*?return false;[\s\S]*?return activateTaskReference\(record\);/);
  assert.match(app, /function activateTaskReference\(record\)[\s\S]*?if \(!record \|\| typeof record\.task_id !== "string" \|\| !record\.task_id\.trim\(\)\) return false;[\s\S]*?stopTaskPolling\(\);[\s\S]*?state\.taskEvents\.clear\(\);[\s\S]*?void pollTask\(state\.pollGeneration\);/);
  assert.match(app, /if \(generation !== state\.pollGeneration \|\| taskId !== state\.taskId \|\| endpointGeneration !== state\.analysisConnectionGeneration \|\| state\.taskAnalysisBase !== analysisBase \|\| state\.settings\.analysisBase !== analysisBase\) return;/);
  assert.match(app, /if \(generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\| endpointGeneration !== state\.analysisConnectionGeneration \|\| state\.taskAnalysisBase !== analysisBase \|\| state\.settings\.analysisBase !== analysisBase\) return;/);
  assert.match(app, /if \(task && TERMINAL_TASK_STATUSES\.has\(task\.status\)\) \{[\s\S]*?stopTaskPolling\(\);[\s\S]*?const runId = task\.run_id \|\| state\.runId;[\s\S]*?if \(runId\) void loadTaskRunResult\(runId, taskId\);/);
  assert.match(recentTaskModule, /return \{[\s\S]*?task_id: record\.task_id\.trim\(\)[\s\S]*?status: typeof record\.status === "string" \? record\.status : ""/);
});


test("polling pauses on permanent API errors and Run reads use bounded, stale-safe retries", () => {
  assert.match(app, /if \(taskError && !isRetryableApiError\(taskError\)\) \{[\s\S]*?自动刷新已暂停[\s\S]*?return;[\s\S]*?state\.pollTimer = window\.setTimeout\(\(\) => pollTask\(generation\), 5000\);/);
  assert.match(app, /const RUN_RESULT_RETRY_DELAYS_MS = \[1000, 2500, 5000\]/);
  assert.match(app, /generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\| endpointGeneration !== state\.analysisConnectionGeneration/);
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
  assert.match(app, /state\.isQuerying[\s\S]*?主行情查询尚未完成/);
  assert.match(watchlistModule, /WATCHLIST_KEY/);
  assert.doesNotMatch(watchlistModule, /apiKey|runResult|report\s*:/i, "watchlist persistence must not include secrets or report payloads");
  assert.match(stylesheet, /\.watchlist-item\s*\{/);
  assert.match(stylesheet, /@media \(max-width: 800px\)[\s\S]*?\.watchlist-list \{ grid-template-columns: minmax\(0, 1fr\); \}/);
});


test("analysis tasks can load the browser watchlist and withhold unavailable position review", () => {
  assert.match(html, /id="load-local-watchlist"/);
  assert.match(html, /id="task-symbol-help"[^>]*role="status"/);
  assert.match(html, /<option value="watchlist_scan">自选股扫描（使用下方清单）<\/option>/);
  assert.match(html, /<option value="position_review" disabled>持仓复核（持仓账本接入后开放）<\/option>/);
  assert.match(app, /function fillTaskSymbolsFromWatchlist\(\)[\s\S]*?const records = safeReadWatchlist\(\)[\s\S]*?parseAnalysisSymbols\(records\.map\(\(record\) => record\.symbol\)\.join\(","\)\)[\s\S]*?\$\("#task-type"\)\.value = "watchlist_scan"/);
  assert.match(app, /\$\("#load-local-watchlist"\)\.addEventListener\("click", fillTaskSymbolsFromWatchlist\)/);
  assert.match(serviceWiring, /Version: "unconfigured-no-paper-ledger-v1"/);
  assert.match(serviceWiring, /AvailableCash: 0, TotalEquity: 0/);
  assert.match(app, /当前没有可载入的本地自选股/);
});

test("daily chart exposes close/candle modes, optional date filters and adjustment basis", () => {
  for (const id of ["bar-start-date", "bar-end-date", "chart-title", "chart-view-close", "chart-view-candles", "chart-legend-label"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "chart/date control must exist: " + id);
  }
  assert.match(app, /function renderChartView\(\)/);
  assert.match(app, /renderCandlestickVolumeChart\(\$\("#close-chart"\),state\.bars,chartOptions\)/);
  assert.match(app, /function shanghaiDateKey\(/);
  assert.match(app, /开始日期不能晚于结束日期/);
  assert.match(app, /日 K 查询失败，旧标的图表已清除/);
  assert.match(stylesheet, /\.chart-candle-body/);
  assert.match(stylesheet, /\.chart-volume-bar/);
  assert.match(stylesheet, /\.chart-view-button\.is-active/);
});


test("supplemental chip and capital-flow research remains opt-in and independently reports each result", () => {
  for (const id of ["load-supplemental-button", "supplemental-feedback", "chip-quality", "chip-error", "chip-profit-ratio", "flow-quality", "flow-error", "flow-main-inflow", "flow-inflow-5d", "flow-inflow-10d"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "supplemental field must exist: " + id);
  }
  assert.match(apiClient, /\/api\/v1\/stocks\/\$\{encodeURIComponent\(symbol\)\}\/chip-distribution/);
  assert.match(apiClient, /\/api\/v1\/stocks\/\$\{encodeURIComponent\(symbol\)\}\/capital-flow/);
  assert.match(app, /Promise\.allSettled\(\[[\s\S]*?getChipDistribution\(state\.settings, symbol\)[\s\S]*?getCapitalFlow\(state\.settings, symbol\)/);
  assert.match(app, /function formatRatioPercent\(value\)/);
  assert.match(app, /function renderSupplementError\(kind, error\)/);
  assert.match(app, /resetSupplementPanel\(symbol\)/);
  assert.match(stylesheet, /\.supplemental-grid/);
  assert.match(stylesheet, /\.supplement-quality-degraded/);
});


test("market overview refresh summary is protected from an older overlapping refresh", () => {
  assert.match(marketOverview, /refreshGeneration=0/);
  assert.match(marketOverview, /const currentGeneration=\+\+refreshGeneration/);
  assert.match(marketOverview, /if\(currentGeneration!==refreshGeneration\)return/);
});

test("stock research clears stale snapshots on reload, request errors, and missing API key", () => {
  assert.match(stockResearch, /function clearPanel\(/);
  assert.match(stockResearch, /function markApiKeyRequired\(/);
  assert.match(stockResearch, /function invalidate\(/);
  assert.match(stockResearch, /clearPanel\(key,"正在读取本次请求，尚无新结果。"/);
  assert.match(stockResearch, /setQuality\(panel\.quality,"invalid","读取失败"\)/);
  assert.match(stockResearch, /if\(!settings\?\.apiKey\)\{markApiKeyRequired\(\);return false;\}/);
  assert.match(stockResearch, /clearPanel\("announcements","正在读取本次公告，尚无新结果。"/);
  assert.match(stockResearch, /valuations-reasons/);
  assert.match(app, /stockResearch\.invalidate\(/);
  assert.match(stockResearch, /if\(!append\)setQuality\("announcements-quality","invalid","读取失败"\)/);
});

test("changing market API credentials invalidates old stock-search suggestions", () => {
  assert.match(app, /function applySettings\(nextSettings\) \{[\s\S]*?previousSettings\.apiKey !== nextSettings\.apiKey \|\| previousSettings\.marketBase !== nextSettings\.marketBase[\s\S]*?cancelStockSearchRequest\(\);[\s\S]*?state\.stockSearchResults = \[\];[\s\S]*?\$\("#stock-search-results"\)\.replaceChildren\(\);[\s\S]*?hideStockSearch\(\);/);
  assert.match(app, /function cancelStockSearchRequest\(\)[\s\S]*?state\.stockSearchGeneration \+= 1;/);
});

test("recent-task suppression and migration are scoped to the analysis API base", () => {
  assert.match(app, /suppressedHistoryTaskBase: ""/);
  assert.match(app, /state\.suppressedHistoryTaskId === state\.taskId && state\.suppressedHistoryTaskBase === taskAnalysisBase/);
  assert.match(app, /historyItem = safeReadRecentTasks\(\)\.find\(\(item\) =>\s*item\.task_id === record\.task_id &&\s*String\(item\.analysis_base \|\| taskAnalysisBase\) === taskAnalysisBase/);
  assert.match(app, /analysis_base: historyItem\.analysis_base \|\| taskAnalysisBase/);
  assert.match(app, /state\.suppressedHistoryTaskBase = state\.taskAnalysisBase \|\| state\.settings\.analysisBase/);
});

test("task polling and saved task references are scoped to the analysis API base", () => {
  assert.match(app, /taskAnalysisBase: ""/);
  assert.match(app, /analysisConnectionGeneration: 0/);
  assert.match(app, /function invalidateActiveTaskForAnalysisBaseChange\(/);
  assert.match(app, /if \(analysisBaseChanged\) invalidateActiveTaskForAnalysisBaseChange\(/);
  assert.match(app, /endpointGeneration !== state\.analysisConnectionGeneration/);
  assert.match(app, /const taskAnalysisBase = state\.taskAnalysisBase \|\| state\.settings\.analysisBase;[\s\S]*?analysis_base: taskAnalysisBase/);
  assert.match(app, /旧任务不再自动轮询/);
  assert.match(app, /if \(requestedBase !== state\.settings\.analysisBase\)/);
  assert.match(recentTaskModule, /analysis_base: typeof record\.analysis_base === "string"/);
});

test("service health and provider-list refreshes ignore superseded responses", () => {
  assert.match(app, /let serviceRefreshGeneration = 0;/);
  assert.match(app, /let sourceRefreshGeneration = 0;/);
  assert.match(app, /async function refreshServices\(\) \{[\s\S]*?const generation = \+\+serviceRefreshGeneration;[\s\S]*?if \(generation !== serviceRefreshGeneration\) return \{ marketOK, analysisOK, stale: true \};/);
  assert.match(app, /async function refreshSources\(\) \{[\s\S]*?const generation = \+\+sourceRefreshGeneration;[\s\S]*?if \(generation !== sourceRefreshGeneration\) return;/);
  assert.match(app, /if \(generation !== serviceRefreshGeneration\) return \{ marketOK, analysisOK, stale: true \};[\s\S]*?setServiceStatus\("market"/);
});

test("filter changes clear stale errors even when a prior query returned no data", () => {
  assert.match(app, /function handleQuoteCriteriaChange\([^)]*\) \{\s*cancelQuoteQuery\(\{ clearResults: true,[\s\S]*?\}\);\s*\}/);
});

test("main quote query invalidates old snapshots when the symbol or filters change", () => {
  assert.match(app, /quoteQueryGeneration: 0/);
  assert.match(app, /function cancelQuoteQuery\(/);
  assert.match(app, /function clearBarsDisplay\(/);
  assert.match(app, /handleSymbolInputChange\(\)/);
  assert.match(app, /handleQuoteCriteriaChange\(/);
  assert.match(app, /controller\.signal\.aborted \|\| generation !== state\.quoteQueryGeneration/);
  assert.match(app, /getDailyBars\(state\.settings, symbol, \{ limit, adjustment \}, controller\.signal\)/);
  assert.match(app, /clearMarketQueryResults\(symbol, "本次行情结果无法展示，旧快照已清除。"\)/);
  assert.match(app, /行情连接配置已变更，旧快照已清除，请重新加载。/);
});

test("dynamic API and browser-local data are not rendered through HTML strings", () => {
  assert.doesNotMatch(app, /\.innerHTML\s*=|insertAdjacentHTML|outerHTML|document\.write/, "use textContent and DOM nodes instead of HTML string sinks");
  assert.doesNotMatch(watchlistModule, /innerHTML|insertAdjacentHTML|outerHTML/);
});

test("analysis task submission prevents concurrent duplicate requests and releases the guard", () => {
  assert.match(app, /isCreatingTask: false/);
  assert.match(app, /async function createTask\(event\)[\s\S]*?event\.preventDefault\(\);[\s\S]*?if \(state\.isCreatingTask\)[\s\S]*?分析任务正在提交[\s\S]*?return;[\s\S]*?state\.isCreatingTask = true;[\s\S]*?parseAnalysisSymbols/);
  assert.match(app, /catch \(error\) \{\s*state\.isCreatingTask = false;[\s\S]*?return;/);
  assert.match(app, /finally \{\s*state\.isCreatingTask = false;[\s\S]*?button\.disabled = false;/);
  assert.match(apiClient, /headersKey \? \{ "Idempotency-Key": headersKey \} : \{\}/);
});

test("market API key help identifies who generates it and where it must be configured", () => {
  assert.match(html, /id="market-key-help"/);
  assert.match(html, /部署管理员自行生成/);
  assert.match(html, /MARKET_DATA_API_KEY/);
  assert.match(html, /openssl rand -hex 32/);
  assert.match(html, /不会自动修改服务器配置或生成后端密钥/);
  assert.match(stylesheet, /\.field-hint \{[^}]*font-size: 13px;[^}]*line-height: 1\.75/);
  assert.equal((stylesheet.match(/Explain the service\/database boundary directly beside connection settings/g) || []).length, 1, "settings boundary CSS should not be duplicated");
});

test("market API key visibility is opt-in and resets when settings close", () => {
  assert.match(html, /id="market-key-input" type="password"/);
  assert.match(html, /id="toggle-market-key-visibility"[^>]*aria-controls="market-key-input"[^>]*aria-pressed="false"[^>]*aria-label="切换行情 API Key 明文显示"/);
  assert.match(app, /function setMarketKeyVisibility\(visible\)[\s\S]*?input\.type = visible \? "text" : "password"/);
  assert.match(app, /\$\("#toggle-market-key-visibility"\)\.addEventListener\("click",[\s\S]*?setMarketKeyVisibility/);
  assert.match(app, /\$\("#settings-dialog"\)\.addEventListener\("close", \(\) => \{[\s\S]*?setMarketKeyVisibility\(false\);[\s\S]*?initialConnectionDraft = null;[\s\S]*?\}\)/);
  const visibilityHandler = app.match(/function setMarketKeyVisibility\(visible\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.doesNotMatch(visibilityHandler, /setAttribute\("aria-label"/, "toggle button accessible name should remain stable while aria-pressed reports its state");
  assert.match(app, /function openSettings\(\)[\s\S]*?setMarketKeyVisibility\(false\)/);
  assert.match(stylesheet, /\.secret-input-row/);
  assert.match(stylesheet, /\.secret-visibility-toggle:focus-visible/);
});

test("global typography has a readable baseline and key UI text is not undersized", () => {
  assert.match(stylesheet, /Global readability pass/);
  assert.match(stylesheet, /body \{ font-size: 15px; line-height: 1\.65; \}/);
  assert.match(finalCssRuleBody(".brand-copy small"), /font-size: 12px !important;/, "brand subtitle should not be shrunk below the readability baseline");
  assert.match(finalCssRuleBody(".task-result-metric small"), /font-size: 13px !important;/, "check the final rule, not an earlier CSS declaration");
  assert.match(finalCssRuleBody(".analysis-result-fields dt"), /font-size: 12px !important;/);
  assert.match(finalCssRuleBody(".task-result-actions .text-button"), /font-size: 13px !important;/);
  assert.match(stylesheet, /\.table-scroll td \{ font-size: 13px !important; \}/);
  assert.match(stylesheet, /\.analysis-result-fields dd \{ font-size: 13px !important; \}/);
  for (const id of ["storage-boundary-title", "market-base-input", "market-key-input", "analysis-base-input"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "settings section must include " + id);
  }
  assert.match(stylesheet, /\.storage-boundary p \{[^}]*font-size: 13px;[^}]*line-height: 1\.75/);
});

test("recent analysis history supports search, single removal and local-only clearing", () => {
  for (const id of ["recent-analysis-search", "clear-recent-analysis", "recent-analysis-feedback", "recent-analysis-list"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "recent-history control must exist: " + id);
  }
  assert.match(app, /function renderRecentTasks\([\s\S]*?recent-analysis-search[\s\S]*?record\.task_id[\s\S]*?taskStatusLabel\(record\.status\)/);
  assert.match(app, /function removeRecentTaskReference\(taskId, analysisBase = ""\)[\s\S]*?removeRecentTask\(taskId, localStorage, undefined, analysisBase\)[\s\S]*?服务端任务和报告不会被删除/);
  assert.match(app, /function clearRecentTaskHistory\(\)[\s\S]*?window\.confirm\([\s\S]*?clearRecentTasks\(localStorage\)[\s\S]*?服务端记录未删除/);
  assert.match(app, /state\.suppressedHistoryTaskId === state\.taskId/);
  assert.match(app, /if \(state\.taskId === taskId && \(!analysisBase \|\| state\.taskAnalysisBase === analysisBase\)\) \{[\s\S]*?state\.suppressedHistoryTaskId = taskId/);
  assert.match(recentTaskModule, /export function removeRecentTask\(/);
  assert.match(recentTaskModule, /export function clearRecentTasks\(/);
  assert.match(stylesheet, /\.recent-analysis-search input:focus/);
  assert.match(stylesheet, /\.recent-analysis-remove:hover/);
});


test("light/dark theme is accessible, persisted locally, and independent of backend APIs", async () => {
  const module = await readFile(new URL("../src/theme.js", import.meta.url), "utf8");
  assert.match(html, /name="color-scheme" content="light dark"/);
  assert.match(html, /id="theme-toggle"[^>]*aria-label="切换主题"[^>]*aria-pressed="false"/);
  assert.match(app, /setAttribute\("aria-label", "切换主题"\)/);
  assert.match(html, /id="theme-toggle-icon"/);
  assert.match(html, /id="theme-toggle-label"/);
  assert.match(app, /import \{ applyTheme, persistTheme, readTheme \} from "\.\/theme\.js"/);
  assert.match(app, /\$\("#theme-toggle"\)\.addEventListener\("click"/);
  assert.match(stylesheet, /:root\[data-theme="light"\]\s*\{/);
  assert.match(stylesheet, /--bg:\s*#f3f6f9/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.settings-dialog/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.analysis-result-card/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.raw-result pre/);
  assert.match(module, /stock-provider-webui\.theme\.v1/);
  assert.doesNotMatch(module, /fetch\(|\/api\//, "theme preference must not call backend APIs");
});

test("mobile navigation drawer restores readable labels and can close via overlay or Escape", () => {
  for (const id of ["mobile-menu-toggle", "mobile-nav-overlay", "primary-sidebar"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "mobile navigation element must exist: " + id);
  }
  assert.match(html, /id="mobile-menu-toggle"[^>]*aria-expanded="false"[^>]*aria-controls="primary-sidebar"/);
  assert.match(app, /function setMobileNavigationOpen\(open, \{ restoreFocus = false \} = \{\}\)/);
  assert.match(app, /\$\("#mobile-nav-overlay"\)\.addEventListener\("click", \(\) => setMobileNavigationOpen\(false, \{ restoreFocus: true \}\)\)/);
  assert.match(app, /if \(event\.key === "Escape"\) setMobileNavigationOpen\(false, \{ restoreFocus: true \}\)/);
  assert.match(stylesheet, /@media \(max-width: 600px\)[\s\S]*?\.sidebar\.is-open \{ transform: translateX\(0\); \}/);
  assert.match(stylesheet, /\.mobile-nav-overlay:not\(\[hidden\]\)/);
});


test("global navigation exposes all planned pages with hash routing and accessible active state", () => {
  const views = ["market", "tasks", "notifications", "chat", "screening", "alerts", "settings"];
  for (const view of views) {
    assert.match(html, new RegExp('data-view="' + view + '"'), "navigation link missing: " + view);
    assert.match(html, new RegExp('id="' + view + '-view"'), "page shell missing: " + view);
  }
  assert.match(app, /const VIEW_META = Object\.freeze\(/);
  assert.match(app, /window\.addEventListener\("hashchange"/);
  assert.match(app, /setAttribute\("aria-current", "page"\)/);
  assert.match(html, /id="topbar-notifications"/, "topbar shortcut must exist without a fabricated count");
  assert.doesNotMatch(html, /id="notification-count"/, "no unread badge without a real notification API");
});

test("new feature pages display unavailable states rather than production mock records", () => {
  for (const id of ["notifications-view", "chat-view", "screening-view", "alerts-view", "settings-view"]) {
    assert.match(html, new RegExp('id="' + id + '"'));
  }
  assert.match(html, /通知收件箱暂不可用/);
  assert.match(html, /不会(?:展示|生成)模拟通知/);
  assert.match(html, /不会用单次分析报告冒充多轮 AI 对话/);
  assert.match(html, /不能提交扫描，也不会用普通分析任务伪装成选股/);
  assert.match(html, /不会假保存规则/);
  assert.match(html, /服务端配置 API 待接入/);
  assert.match(html, /服务端密码、权限、\.env、备份和恢复涉及认证/);
  assert.match(html, /data-theme-value="dark"/);
  assert.match(html, /data-theme-value="light"/);
  assert.match(app, /persistTheme\(button\.dataset\.themeValue\)/);
  assert.match(app, /state\.view === "tasks" && state\.taskId/);
});

test("new page layouts have responsive grids and disabled-state affordances", () => {
  assert.match(stylesheet, /\.feature-card-grid\s*\{[^}]*grid-template-columns: repeat\(3, minmax\(0, 1fr\)\)/);
  assert.match(stylesheet, /@media \(max-width: 800px\)[\s\S]*?\.settings-card-grid \{ grid-template-columns: minmax\(0, 1fr\); \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\)[\s\S]*?\.feature-form-grid \{ grid-template-columns: minmax\(0, 1fr\); \}/);
  assert.match(stylesheet, /button:disabled, input:disabled, select:disabled, textarea:disabled/);
});


test("task detail opens an accessible run-flow drawer with separate Task and Run states", () => {
  for (const id of ["run-flow-dialog", "run-flow-title", "close-run-flow-dialog", "task-status-pill", "task-run-status-pill", "task-created", "task-started", "task-finished", "task-symbols-value"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "run-flow field is missing: " + id);
  }
  assert.match(html, /<dialog class="run-flow-dialog" id="run-flow-dialog"/);
  assert.match(app, /function openRunFlowDrawer\(\)[\s\S]*?dialog\.showModal\(\)/);
  assert.match(app, /function activateTaskReference\(record\)[\s\S]*?openRunFlowDrawer\(\)[\s\S]*?pollTask\(state\.pollGeneration\)/);
  assert.match(app, /function renderTask\(task\)[\s\S]*?task\.started_at[\s\S]*?task\.finished_at/);
  assert.match(app, /function renderRunStatus\(status = ""\)[\s\S]*?runStatusLabel\(normalized\)/);
  assert.match(app, /function renderTaskResult\(result\)[\s\S]*?renderRunStatus\(result\.status \|\| ""\)/);
  assert.match(app, /close-run-flow-dialog/);
});

test("run-flow events expose only server-provided durations and diagnostics", () => {
  assert.match(app, /event\.duration_ms !== undefined && event\.duration_ms !== null/);
  assert.match(app, /event\.error_code/);
  assert.match(app, /metaParts\.join\(" · "\)/);
  assert.match(apiClient, /after_sequence/);
  assert.match(app, /\.sort\(\(a, b\) => a\.sequence - b\.sequence\)/);
  assert.doesNotMatch(app, /completed_stages\s*\/\s*total_stages|progressPercent|fakeDAG/i);
});


test("run-flow keeps cached events explicit during read errors and exports diagnostic context", () => {
  assert.match(html, /id="task-events-read-status"[^>]*role="status"[^>]*aria-live="polite"/);
  assert.match(app, /const readStatus = \$\("#task-events-read-status"\)/);
  assert.match(app, /已保留当前可用事件/);
  assert.match(app, /eventsCount\.textContent = state\.taskEventsError[\s\S]*?读取失败/);
  assert.match(app, /task_error:\s*\$\("#task-error"\)\.textContent\.trim\(\) \|\| null/);
  assert.match(app, /task_poll_status:\s*\$\("#task-poll-label"\)\.textContent \|\| null/);
  assert.match(app, /event_read_status:\s*\{[\s\S]*?last_error:\s*state\.taskEventsError \|\| null[\s\S]*?known_event_count:\s*state\.taskEvents\.size/);
  assert.match(stylesheet, /\.task-events-read-status/);
});


test("market activity clears stale calendar state for invalid input, missing key, loading, and failure", () => {
  assert.match(marketActivity, /function clearReasons\(id\)/);
  assert.match(marketActivity, /日期无效[\s\S]*?meta\("trading-calendar-meta",""\)[\s\S]*?clearReasons\("trading-calendar-reasons"\)/);
  assert.match(marketActivity, /需配置 Key[\s\S]*?数据未读取 · 需要 API Key[\s\S]*?clearReasons\("trading-calendar-reasons"\)/);
  assert.match(marketActivity, /badge\("trading-calendar-quality","加载中","unknown"\)/);
  assert.match(marketActivity, /正在读取交易日历…/);
  assert.match(marketActivity, /本次未返回数据[\s\S]*?clearReasons\("trading-calendar-reasons"\)/);
  assert.match(marketActivity, /if\(!append\)[\s\S]*?加载中[\s\S]*?meta\("dragon-tiger-meta",""\)/);
  assert.match(marketActivity, /龙虎榜暂不可用/);
  assert.match(marketActivity, /meta\("dragon-tiger-meta",""\)/);
});


test("provider capability definitions expose each backend capability once", () => {
  const definitionBlock = app.match(/const capabilityDefinitions = \[([\s\S]*?)\n  \];/);
  assert.ok(definitionBlock, "provider capability definitions must remain explicit");
  const keys = [...definitionBlock[1].matchAll(/\["([^"]+)",\s*"[^"]+"\]/g)].map((match) => match[1]);
  assert.equal(new Set(keys).size, keys.length, "one capability must not render multiple labels");
  assert.equal(keys.filter((key) => key === "chip_capable").length, 1);
  assert.equal(keys.filter((key) => key === "capital_flow_capable").length, 1);
});

test("run-flow event details share normalized status labels and tones with the timeline", () => {
  assert.match(app, /function taskEventStatusKey\(status\)/);
  assert.match(app, /function taskEventStatusLabel\(status, statusKey = taskEventStatusKey\(status\)\)/);
  assert.match(app, /function taskEventStatusTone\(statusKey\)/);
  assert.match(app, /status\.textContent = statusLabel/);
  assert.match(app, /\["事件状态", statusLabel\]/);
  assert.match(app, /const statusKey = taskEventStatusKey\(event\.status\)/);
  assert.match(stylesheet, /\.availability-pill\.is-error/);
  assert.match(stylesheet, /\.availability-pill\.is-warning/);
});


test("settings center groups configuration by ownership and prevents fake server writes", () => {
  for (const id of [
    "settings-display-section", "settings-connection-section", "settings-basic-section",
    "settings-data-source-section", "settings-ai-section", "settings-notification-section",
    "settings-scheduler-section", "settings-screening-section", "settings-security-section"
  ]) assert.match(html, new RegExp('id="' + id + '"'));
  assert.match(html, /data-settings-target="settings-security-section"/);
  assert.match(app, /connectionDraftChanged\(initialConnectionDraft, readConnectionDraft\(\)\)/);
  assert.match(app, /存在未保存的连接配置更改/);
  assert.match(app, /closeSettingsWithGuard/);
  assert.match(app, /event\.preventDefault\(\);[\s\S]*?closeSettingsWithGuard\(\)/);
  assert.match(app, /mergeConnectionDraft\(readConnectionDraft\(\), state\.settings\)/);
  assert.match(app, /async function checkConnectionFromSettings\(\)[\s\S]*?const settingsToCheck = mergeConnectionDraft\(testedDraft, state\.settings\);[\s\S]*?Promise\.allSettled\(\[[\s\S]*?marketHealth\(settingsToCheck\)[\s\S]*?analysisHealth\(settingsToCheck\)/);
  assert.match(app, /currentDraft\.marketBase !== testedDraft\.marketBase \|\| currentDraft\.analysisBase !== testedDraft\.analysisBase/);
  assert.match(app, /不会更改全局连接状态/);
  assert.match(html, /服务端配置 API 待接入/);
  assert.match(html, /服务器 \.env/);
  assert.match(html, /导出服务器配置（待接入）/);
  assert.match(html, /更改管理员密码（待接入）/);
  assert.match(html, /id="market-key-input" type="password"/);
  assert.match(app, /Never echo the persisted API key into the input DOM/);
  assert.doesNotMatch(app, /\$\("#market-key-input"\)\.value = state\.settings\.apiKey/);
});


test("notification center exposes explicit five-state UI without fake unread counts or records", () => {
  for (const state of ["loading", "empty", "unavailable", "error", "loaded"]) {
    assert.match(html, new RegExp('data-notification-state="' + state + '"'));
  }
  assert.match(html, /id="notification-category-filter"[^>]*disabled/);
  assert.match(html, /id="notification-status-filter"[^>]*disabled/);
  assert.match(html, /id="notification-search" type="search" disabled/);
  assert.match(html, /id="refresh-notifications"[^>]*disabled/);
  assert.match(html, /id="notification-detail-empty"/);
  assert.match(html, /id="notification-detail-content"[^>]*hidden/);
  assert.match(html, /不会生成模拟通知、虚假未读角标/);
  assert.match(html, /不会只在浏览器里假保存/);
  assert.match(app, /function renderNotificationState\(snapshot\)[\s\S]*?resolveNotificationViewState\(snapshot\)/);
  assert.match(app, /renderNotificationState\(\{ supported: false \}\)/);
  assert.doesNotMatch(html, /notification-count|unread-count|未读\\s*\\d+/i);
});


test("chat workbench has context isolation and no production mock replies", () => {
  for (const id of [
    "chat-session-list-state", "chat-new-session", "chat-stock-context", "chat-clear-context",
    "chat-mode-select", "chat-message-list", "chat-composer-input", "chat-send", "chat-stop",
    "chat-copy-message", "chat-export-session", "chat-retry-message"
  ]) assert.match(html, new RegExp('id="' + id + '"'), "missing chat control: " + id);
  for (const state of ["loading", "empty", "unavailable", "error", "loaded"]) {
    assert.match(html, new RegExp('data-chat-state="' + state + '"'));
  }
  assert.match(html, /不会用单次分析报告冒充多轮 AI 对话/);
  assert.match(html, /不会自动复用历史上下文/);
  assert.match(app, /function renderChatState\(snapshot\)[\s\S]*?resolveChatViewState\(snapshot\)/);
  assert.match(app, /function syncChatStockContextFromMarket\(\)[\s\S]*?state\.quote && typeof state\.quote\.symbol/);
  assert.match(app, /if \(nextView === "chat"\) syncChatStockContextFromMarket\(\)/);
  assert.match(app, /state\.chatStockContext = null;[\s\S]*?renderChatStockContext\(\)/);
  assert.match(app, /renderChatState\(\{ supported: false \}\)/);
  assert.match(html, /id="chat-send" type="button" disabled/);
  assert.match(html, /id="chat-stop" type="button" disabled/);
  assert.match(html, /id="chat-mode-select" disabled/);
  assert.doesNotMatch(app, /CHAT_SESSION_KEY|localStorage.*chat/i);
});


test("screening workbench distinguishes unavailable capability from disabled and empty results", () => {
  for (const id of [
    "screening-capability-pill", "screening-summary-capability", "screening-summary-run",
    "screening-summary-candidates", "screening-summary-last-run", "screening-market",
    "screening-universe", "screening-strategy", "screening-candidate-limit", "screening-min-change",
    "screening-min-turnover", "screening-submit", "screening-refresh-run", "screening-candidate-search",
    "screening-quality-filter", "screening-candidate-list", "screening-history-list",
    "screening-history-prev", "screening-history-next"
  ]) assert.match(html, new RegExp('id="' + id + '"'), "missing screening element: " + id);
  for (const state of ["loading", "idle", "queued", "running", "completed", "partial", "failed", "unknown", "error", "unavailable"]) {
    assert.match(html, new RegExp('data-screening-run-state="' + state + '"'));
  }
  for (const state of ["loading", "empty", "error", "unavailable", "loaded"]) {
    assert.match(html, new RegExp('data-screening-results-state="' + state + '"'));
    assert.match(html, new RegExp('data-screening-history-state="' + state + '"'));
  }
  const start = html.indexOf('id="screening-view"');
  const end = html.indexOf('id="alerts-view"', start);
  const screeningView = html.slice(start, end);
  assert.doesNotMatch(screeningView, /600519|贵州茅台|示例候选|模拟分数/);
  assert.match(app, /function renderScreeningCapability\(snapshot\)[\s\S]*?resolveScreeningCapability\(snapshot\)/);
  assert.match(app, /function renderScreeningTask\(snapshot\)[\s\S]*?resolveScreeningTaskState\(snapshot\)/);
  assert.match(app, /function renderScreeningCandidates\(snapshot\)[\s\S]*?resolveScreeningCollectionState\(snapshot, "candidates"\)/);
  assert.match(app, /function renderScreeningHistory\(snapshot\)[\s\S]*?resolveScreeningCollectionState\(snapshot, "items"\)/);
  assert.match(app, /renderScreeningCapability\(\{ supported: false \}\)/);
  assert.match(app, /renderScreeningTask\(\{ supported: false \}\)/);
  assert.match(app, /renderScreeningCandidates\(\{ supported: false \}\)/);
  assert.match(app, /renderScreeningHistory\(\{ supported: false \}\)/);
  assert.match(screeningView, /id="screening-submit" type="button" disabled/);
  assert.doesNotMatch(app, /screening.*localStorage/i);
});


test("alert center separates rule CRUD, evaluation state, trigger history and delivery attempts", () => {
  for (const id of [
    "alert-capability-pill", "alert-rules-status-pill", "alert-rule-name", "alert-rule-scope", "alert-rule-type",
    "alert-rule-threshold", "alert-rule-severity", "alert-rule-enabled", "alert-rule-submit",
    "alert-rule-test", "alert-evaluation-status-pill", "alert-history-symbol", "alert-history-rule",
    "alert-history-status", "alert-history-list", "alert-delivery-status-pill", "alert-delivery-list"
  ]) assert.match(html, new RegExp('id="' + id + '"'), "missing alert control: " + id);
  for (const state of ["loading", "empty", "error", "unavailable", "loaded"]) {
    assert.match(html, new RegExp('data-alert-rules-state="' + state + '"'));
    assert.match(html, new RegExp('data-alert-history-state="' + state + '"'));
    assert.match(html, new RegExp('data-alert-delivery-state="' + state + '"'));
  }
  for (const state of ["loading", "idle", "triggered", "not-triggered", "evaluation-error", "degraded", "skipped", "error", "unknown", "unavailable"]) {
    assert.match(html, new RegExp('data-alert-evaluation-state="' + state + '"'));
  }
  const start = html.indexOf('id="alerts-view"');
  const end = html.indexOf('id="settings-view"', start);
  const alertsView = html.slice(start, end);
  assert.match(alertsView, /不会假保存规则/);
  assert.match(alertsView, /不能静默当作未触发/);
  assert.match(alertsView, /不会展示模拟的“发送成功”/);
  assert.doesNotMatch(alertsView, /600519|贵州茅台|示例规则|示例触发/);
  assert.match(app, /function renderAlertCapability\(snapshot\)[\s\S]*?resolveAlertCapability\(snapshot\)/);
  assert.match(app, /function renderAlertEvaluation\(snapshot\)[\s\S]*?resolveAlertEvaluationState\(snapshot\)/);
  assert.match(app, /renderAlertCapability\(\{ supported: false \}\)/);
  assert.match(app, /renderAlertEvaluation\(\{ supported: false \}\)/);
  assert.match(html, /id="alert-rule-submit" type="button" disabled/);
  assert.match(html, /id="alert-rule-test" type="button" disabled/);
  assert.doesNotMatch(app, /alert.*localStorage/i);
});


test("stock-name autocomplete uses the real market API and accessible, stale-safe controls", () => {
  assert.match(html, /aria-autocomplete="list"/);
  assert.match(html, /id="stock-search-results"[^>]*role="listbox"/);
  assert.match(html, /id="stock-search-status"[^>]*role="status"/);
  assert.match(apiClient, /export function searchStocks\(/);
  assert.match(app, /function scheduleStockSearch\(\)/);
  assert.match(app, /new AbortController\(\)/);
  assert.match(app, /generation !== state\.stockSearchGeneration/);
  assert.match(app, /addEventListener\("compositionstart"/);
  assert.match(app, /addEventListener\("compositionend"/);
  assert.match(app, /function chooseStockSearchResult\(/);
  assert.match(app, /error\.status === 404/);
  assert.match(stylesheet, /\.stock-search-option/);
  assert.doesNotMatch(app, /localStorage[^\n]*stockSearch/i, "stock results must come from the API, not browser-side mock data");
});


test("Run Flow summary and event diagnostics remain grounded in persisted Task, StageEvent and RunResult data", () => {
  for (const id of [
    "task-summary-event-count",
    "task-summary-latest-event",
    "task-summary-quality",
    "task-summary-warning-count",
    "task-event-detail",
    "task-event-detail-fields",
    "task-event-json"
  ]) {
    assert.match(html, new RegExp('id="' + id + '"'), "Run Flow must include " + id);
  }
  assert.match(app, /function renderTaskFlowSummary\(\)/);
  assert.match(app, /function renderSelectedTaskEvent\(item\)/);
  assert.match(app, /button\[data-task-event-sequence\]/);
  assert.match(app, /state\.taskEventsLoaded/);
  assert.match(app, /event\.duration_ms/);
  assert.match(app, /event\.error_code/);
  assert.match(app, /result\?\.quality\?\.core_pass/);
  assert.match(app, /Array\.isArray\(warnings\)/);
  assert.match(stylesheet, /\.run-flow-summary/);
  assert.match(stylesheet, /\.event-row\[aria-pressed="true"\]/);
  assert.doesNotMatch(app, /completed_stages\s*\/\s*\w+|progressPercent|100\s*\*\s*.*completed_stages/i,
    "Run Flow must not infer or fabricate progress percentages");
});


test("market overview and activity use readable secondary text and touch-sized controls", () => {
  assert.match(finalCssRuleBody(".market-overview-updated"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-overview-subheading p"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-index-identity small"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-index-changes span"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-widget-meta"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-breadth-metric small"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-table-primary"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-table-secondary"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-touch-time"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-activity-heading p"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-activity-controls label"), /font-size: 12px/);
  assert.match(finalCssRuleBody(".market-activity-controls input"), /min-height: 38px/);
  assert.match(finalCssRuleBody(".market-activity-controls .button"), /min-height: 38px/);
  assert.match(finalCssRuleBody(".calendar-month-navigation .icon-button"), /min-width: 38px; min-height: 38px/);
  assert.match(finalCssRuleBody(".trading-calendar-weekdays span"), /font-size: 11px/);
  assert.match(stylesheet, /\.trading-calendar-day span \{ font-size: 11px; line-height: 1\.25; text-align: center; overflow-wrap: anywhere; \}/);
  assert.match(finalCssRuleBody(".trading-calendar-day span"), /font-size: 10px/);
  assert.match(stylesheet, /@media \(max-width: 700px\) \{[\s\S]*?\.market-overview-actions \.button \{ width: 100%; min-height: 40px; \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.trading-calendar-day span \{ font-size: 10px; \}/);
  assert.match(finalCssRuleBody(".market-activity-table"), /-webkit-overflow-scrolling: touch/);
});


test("primary workspace tabs keep market modules on the homepage without quick-locate UI", () => {
  const home = html.slice(html.indexOf('<section class="view" id="market-view">'), html.indexOf('<section class="view" id="stocks-view"'));
  for (const id of ["market-overview-panel", "market-lists-panel", "market-activity-panel"]) {
    assert.ok(home.includes('id="' + id + '"'), "homepage should include " + id);
  }
  assert.ok(home.indexOf('id="market-overview-panel"') < home.indexOf('id="market-lists-panel"'));
  assert.ok(home.indexOf('id="market-lists-panel"') < home.indexOf('id="market-activity-panel"'));
  for (const view of ["market", "stocks", "watchlist"]) {
    assert.match(html, new RegExp('class="workspace-tab[^"]*"[^>]*data-view="' + view + '"'));
  }
  assert.match(html, /aria-label="核心工作区"/);
  assert.doesNotMatch(html, /class="page-jump-nav"|data-scroll-target=/);
  assert.doesNotMatch(app, /pageJumpButtons|syncPageJumpLocation|scroll-target/);
  assert.doesNotMatch(stylesheet, /\.page-jump-nav|\.page-jump-button/);
  assert.match(stylesheet, /\.workspace-tabs\s*\{/);
  assert.match(stylesheet, /\.workspace-tab\.active/);
});

test("collapsed sidebar navigation remains accessible and unavailable modules disclose their status", () => {
  const navButtons = [...html.matchAll(/<button class="nav-link[^"]*"[^>]*data-view="([^"]+)"[^>]*>/g)];
  assert.deepEqual(navButtons.map((match) => match[1]), [
    "research", "tasks", "notifications", "chat", "screening", "alerts", "settings"
  ], "sidebar should expose secondary tools while primary destinations live in workspace tabs");
  const workspaceTabs = [...html.matchAll(/<button class="workspace-tab[^"]*"[^>]*data-view="([^"]+)"[^>]*>/g)];
  assert.deepEqual(workspaceTabs.map((match) => match[1]), ["market", "stocks", "watchlist"]);
  for (const view of ["market", "stocks", "watchlist"]) {
    assert.match(html, new RegExp('id="' + view + '-view"'));
  }
  assert.doesNotMatch(html, /id="market-data-view"/);
  for (const match of navButtons) {
    const view = match[1];
    assert.match(match[0], /aria-label="/, view + " should have an accessible name when its text is hidden");
    assert.match(match[0], /title="/, view + " should expose a hover label in the collapsed sidebar");
  }
  assert.match(html, /id="stocks-view"/);
  assert.match(html, /id="watchlist-view"/);
  assert.match(html, /id="research-view"/);
  assert.equal((html.match(/class="nav-link-state" title="后端接口待接入">待接入<\/span>/g) || []).length, 4,
    "notifications, chat, screening, and alerts should disclose backend dependency before navigation");
  assert.match(stylesheet, /\.nav-link-state\s*\{/);
  assert.match(stylesheet, /@media \(max-width: 900px\) \{[\s\S]*?\.nav-link-state\s*\{\s*display: none;/);
});

function parseThemeTokens(selector) {
  const start = stylesheet.indexOf(selector);
  assert.notEqual(start, -1, "CSS should declare theme token block: " + selector);
  const open = stylesheet.indexOf("{", start);
  const close = stylesheet.indexOf("}", open + 1);
  assert.ok(open > start && close > open, "theme token block should be closed: " + selector);
  return Object.fromEntries(
    [...stylesheet.slice(open + 1, close).matchAll(/--([A-Za-z0-9_-]+)[ \t]*:[ \t]*(#[0-9a-fA-F]{6})[ \t]*;/g)]
      .map((match) => [match[1], match[2]])
  );
}

function relativeLuminance(hexColor) {
  const channels = hexColor.match(/[0-9a-f]{2}/gi).map((channel) => parseInt(channel, 16) / 255);
  const linear = channels.map((value) => value <= 0.04045
    ? value / 12.92
    : ((value + 0.055) / 1.055) ** 2.4);
  return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2];
}

function contrastRatio(foreground, background) {
  const a = relativeLuminance(foreground);
  const b = relativeLuminance(background);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

test("深浅主题的辅助文字在常用背景上达到 WCAG AA 对比度", () => {
  const themes = [
    { name: "深色", selector: ":root {", surfaces: ["bg", "bg-elevated", "panel", "panel-raised"] },
    { name: "浅色", selector: ':root[data-theme="light"] {', surfaces: ["bg", "bg-elevated", "panel", "panel-raised"] }
  ];

  for (const theme of themes) {
    const tokens = parseThemeTokens(theme.selector);
    for (const surface of theme.surfaces) {
      assert.ok(tokens["text-dim"], theme.name + "主题必须声明辅助文字颜色");
      assert.ok(tokens[surface], theme.name + "主题必须声明背景 token: " + surface);
      const ratio = contrastRatio(tokens["text-dim"], tokens[surface]);
      assert.ok(
        ratio >= 4.5,
        theme.name + "主题的 --text-dim 在 --" + surface + " 上对比度为 " + ratio.toFixed(2) + ":1，低于 4.5:1"
      );
    }
  }
});

function baseCssDeclarationBlock(selector) {
  const escaped = selector.replace(/[.*+?^$()|[\]\\]/g, "\\$&");
  const match = stylesheet.match(new RegExp("(?:^|\\n)\\s*" + escaped + "\\s*\\{([^}]*)\\}"));
  assert.ok(match, "stylesheet should declare " + selector);
  return match[1];
}

test("辅助说明、任务元信息和输入占位文字统一使用主题感知的辅助文字 token", () => {
  for (const selector of [
    ".side-label",
    ".sidebar-footnote",
    ".task-facts small",
    ".event-row > small",
    ".task-actions > span"
  ]) {
    assert.match(
      baseCssDeclarationBlock(selector),
      /color:\s*var\(--text-dim\)\s*;/,
      selector + " should inherit contrast-safe text color from the current theme"
    );
  }

  assert.match(
    stylesheet,
    /\.field input::placeholder,\s*\.field textarea::placeholder,\s*\.recent-analysis-search input::placeholder,\s*\.watchlist-name-input::placeholder\s*\{[^}]*color:\s*var\(--text-dim\)/,
    "form and local-search placeholders should use the shared contrast-safe token"
  );
  assert.match(
    stylesheet,
    /:root\[data-theme="light"\] \.field input::placeholder,[\s\S]*?\.watchlist-name-input::placeholder\s*\{\s*color:\s*var\(--text-dim\);/,
    "light-theme placeholder overrides must not restore a low-contrast hard-coded color"
  );
  assert.match(stylesheet, /:root\[data-theme="light"\] \.side-label \{ color: var\(--text-dim\); \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.sidebar-footnote \{ color: var\(--text-dim\); \}/);
});


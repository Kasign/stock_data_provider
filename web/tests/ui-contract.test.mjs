import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [html, app, format, domain, openapi, recentTaskModule, stylesheet, watchlistModule, apiClient, taskQueue, marketActivity, stockResearch, marketOverview, serviceWiring, dataCenter, healthCheck] = await Promise.all([
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
  readFile(new URL("../../internal/service/wiring.go", import.meta.url), "utf8"),
  readFile(new URL("../src/data-center.js", import.meta.url), "utf8"),
  readFile(new URL("../src/health-check.js", import.meta.url), "utf8")
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
  assert.match(app, /if \(taskError && \(!isRetryableApiError\(taskError\) \|\| taskError\.responseIdentityMismatch\)\) \{[\s\S]*?自动刷新已暂停[\s\S]*?return;[\s\S]*?state\.pollTimer = window\.setTimeout\(\(\) => pollTask\(generation\), 5000\);/);
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
  assert.match(app, /function startAnalysisForSymbol\(input\) \{[\s\S]*?symbol = normalizeAnalysisSymbol\(input\);[\s\S]*?\$\("#task-symbols"\)\.value = symbol;[\s\S]*?setView\("tasks"\);[\s\S]*?\$\("#task-form"\)\.requestSubmit\(\);/);
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
  assert.match(app, /function fillTaskSymbolsFromWatchlist\(\)[\s\S]*?const watchlistState = safeReadWatchlistState\(\);[\s\S]*?const records = watchlistState\.items;[\s\S]*?if \(!watchlistState\.storageReadable \|\| !watchlistState\.storageValid\)[\s\S]*?parseAnalysisSymbols\(records\.map\(\(record\) => record\.symbol\)\.join\(","\)\)[\s\S]*?\$\("#task-type"\)\.value = "watchlist_scan"/);
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


test("individual stock research uses one detail workspace without duplicate chip/flow fetches", () => {
  for (const id of ["research-stock-list-body", "research-overview-panel", "research-overview-price", "research-overview-ma5", "research-overview-quality", "research-open-chart", "research-add-watchlist", "research-start-analysis", "stock-research-panel", "chip-quality", "load-chip-distribution", "capital-flow-quality", "load-capital-flow"]) {
    assert.ok(html.includes('id="' + id + '"'), "research field must exist: " + id);
  }
  assert.ok(!html.includes('id="stock-supplemental-panel"'), "old duplicate chip/flow panel must be gone");
  assert.ok(app.includes("function renderResearchOverview()"));
  assert.ok(app.includes('function openResearchStock(symbol, name = "")'));
  assert.ok(app.includes("await stockResearch.refresh()"));
  assert.ok(!app.includes("loadSupplementalData") && !app.includes("resetSupplementPanel"));
  assert.ok(apiClient.includes("/chip-distribution") && apiClient.includes("/capital-flow"));
  assert.ok(stockResearch.includes("getChipDistribution") && stockResearch.includes("getCapitalFlow"));
  assert.ok(stockResearch.includes("async function refresh()"));
  assert.ok(stylesheet.includes(".research-overview-grid") && stylesheet.includes(".supplement-quality-degraded"));
});
test("market overview refresh summaries are isolated by status region and guarded against stale runs", () => {
  assert.match(marketOverview, /refreshGenerations=Object\.create\(null\)/);
  assert.match(marketOverview, /const currentGeneration=\(refreshGenerations\[statusSelector\]\|\|0\)\+1/);
  assert.match(marketOverview, /refreshGenerations\[statusSelector\]=currentGeneration/);
  assert.match(marketOverview, /if\(currentGeneration!==refreshGenerations\[statusSelector\]\)return/);
});

test("an older stock-research load cannot unlock the current stock's loading button", () => {
  assert.match(stockResearch, /loadResearchGeneration:0/);
  assert.match(stockResearch, /function reset\(symbol\)\{[\s\S]*?state\.loadResearchGeneration\+=1;[\s\S]*?\$\("#load-stock-research"\)/);
  assert.match(stockResearch, /async function loadResearch\(\)\{[\s\S]*?const requestGeneration=\+\+state\.loadResearchGeneration;[\s\S]*?await Promise\.allSettled\([\s\S]*?requestGeneration!==state\.loadResearchGeneration\|\|symbol!==state\.symbol[\s\S]*?finally\{[\s\S]*?requestGeneration===state\.loadResearchGeneration&&symbol===state\.symbol/);
});

test("stale stock research detail requests cannot unlock buttons for a different symbol", () => {
  assert.match(stockResearch, /function reset\(symbol\)[\s\S]*?\["#load-valuations","读取估值指标"\][\s\S]*?\["#load-chip-distribution","读取筹码分布"\][\s\S]*?\["#load-capital-flow","读取资金流"\]/);
  assert.match(stockResearch, /async function runSupplementRequest\(buttonId,idleText,key,request,render,successMessage\)[\s\S]*?const operation=fetchOne\(key,request,render\)[\s\S]*?const generation=state\.generation\[key\][\s\S]*?generation===state\.generation\[key\]&&symbol===state\.symbol/);
  assert.match(stockResearch, /async function loadValuations\(\)\{[\s\S]*?runSupplementRequest\("#load-valuations"/);
  assert.match(stockResearch, /async function loadChipDistribution\(\)\{[\s\S]*?runSupplementRequest\("#load-chip-distribution"/);
  assert.match(stockResearch, /async function loadCapitalFlow\(\)\{[\s\S]*?runSupplementRequest\("#load-capital-flow"/);
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
  assert.match(stockResearch, /const replacingInFlight=state\.announcementLoading;[\s\S]*?if\(replacingInFlight&&append\)return false;[\s\S]*?if\(replacingInFlight\)\{abort\("announcements"\);state\.announcementLoading=false;\}/);
  assert.match(stockResearch, /if\(!replacingInFlight\)abort\("announcements"\);\s*const seq=state\.generation\.announcements/);
});

test("changing market API credentials invalidates old stock-search suggestions", () => {
  assert.match(app, /function applySettings\(nextSettings, \{ persistPreviousActiveTask = true \} = \{\}\) \{[\s\S]*?previousSettings\.apiKey !== nextSettings\.apiKey \|\| previousSettings\.marketBase !== nextSettings\.marketBase[\s\S]*?cancelStockSearchRequest\(\);[\s\S]*?state\.stockSearchResults = \[\];[\s\S]*?\$\("#stock-search-results"\)\.replaceChildren\(\);[\s\S]*?hideStockSearch\(\);/);
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

test("topbar service health checks have bounded timeouts and pass abort signals", () => {
  assert.ok(healthCheck.includes("export async function runHealthCheck(check, settings, timeoutMs = 6000)"));
  assert.ok(healthCheck.includes("controller.abort()"));
  assert.match(app, /async function refreshServices\(\)[\s\S]*?Promise\.all\(\[[\s\S]*?runHealthCheck\(marketHealth, settings\)[\s\S]*?runHealthCheck\(analysisHealth, settings\)/);
});

test("data-center checks are invalidated when a saved API connection changes", () => {
  assert.match(dataCenter, /let generation = 0;/);
  assert.match(dataCenter, /if \(current !== generation\) return;/);
  assert.match(dataCenter, /function invalidate\(message = "连接配置已变更，之前的检查结果已作废。请重新检查。"\) \{[\s\S]*?generation \+= 1;[\s\S]*?checking = false;[\s\S]*?setBadge\("#data-center-summary-status", "unknown", "需重新检查"\)/);
  assert.match(dataCenter, /finally \{[\s\S]*?if \(current === generation\) \{[\s\S]*?checking = false;/);
  assert.match(dataCenter, /return \{ refresh, invalidate \};/);
  assert.match(app, /const marketConnectionChanged = previousSettings\.apiKey !== nextSettings\.apiKey \|\| previousSettings\.marketBase !== nextSettings\.marketBase;[\s\S]*?if \(analysisBaseChanged \|\| marketConnectionChanged\) \{\s*dataCenter\.invalidate\(/);
});

test("connection-check failures ignore errors from a superseded configuration", () => {
  const checkConnection = app.match(/async function checkConnectionFromSettings\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(checkConnection, "settings connection check must exist");
  assert.match(checkConnection, /let settingsToCheck = null/);
  assert.match(checkConnection, /catch \(error\) \{[\s\S]*?const currentDraft = readConnectionDraft\(\);[\s\S]*?connectionDraftChanged\(testedDraft, currentDraft\)/);
  assert.match(checkConnection, /if \(configurationChanged\) \{[\s\S]*?旧错误已忽略，请使用当前配置重新检查/);
  assert.match(checkConnection, /连接检查失败：[\s\S]*?finally \{/);
});

test("market connection changes invalidate all in-flight stock-research panels", () => {
  assert.match(app, /const marketConnectionChanged = previousSettings\.apiKey !== nextSettings\.apiKey \|\| previousSettings\.marketBase !== nextSettings\.marketBase;[\s\S]*?if \(marketConnectionChanged\) \{[\s\S]*?cancelStockSearchRequest\(\);[\s\S]*?stockResearch\.invalidate\("行情连接配置已变更，旧研究请求已作废，请重新加载对应资料。"\);\s*\}/);
  assert.match(stockResearch, /function invalidate\(message="行情连接配置已变更，请重新加载个股研究资料。"\)\{\s*reset\(resolvedSymbol\(\)\);[\s\S]*?Object\.keys\(state\.generation\)\.forEach\(abort\)/);
});

test("service health and provider-list refreshes ignore superseded responses", () => {
  assert.match(app, /let serviceRefreshGeneration = 0;/);
  assert.match(app, /let sourceRefreshGeneration = 0;/);
  assert.match(app, /async function refreshServices\(\) \{[\s\S]*?const generation = \+\+serviceRefreshGeneration;[\s\S]*?if \(generation !== serviceRefreshGeneration\) return \{ marketOK, analysisOK, marketState, analysisState, stale: true \};/);
  assert.match(app, /async function refreshSources\(\) \{[\s\S]*?const generation = \+\+sourceRefreshGeneration;[\s\S]*?if \(generation !== sourceRefreshGeneration\) return;/);
  assert.match(app, /if \(generation !== serviceRefreshGeneration\) return \{ marketOK, analysisOK, marketState, analysisState, stale: true \};[\s\S]*?setServiceStatus\("market"/);
});

test("provider capability refresh clears old connection data and owns its loading state", () => {
  const refreshSources = app.match(/async function refreshSources\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(refreshSources, "refreshSources implementation must exist");
  assert.match(refreshSources, /list\.replaceChildren\(\);[\s\S]*?setText\("#source-count", "—"\);[\s\S]*?state\.sourceCount = 0/);
  assert.match(refreshSources, /button\.disabled = true;[\s\S]*?button\.setAttribute\("aria-busy", "true"\)/);
  assert.match(refreshSources, /正在读取当前连接配置对应的数据源能力/);
  assert.match(refreshSources, /showMessage\(errorMessage\(error\), "empty-row error-text"\)/);
  assert.match(refreshSources, /finally \{[\s\S]*?if \(generation === sourceRefreshGeneration && button\)[\s\S]*?button\.disabled = false;[\s\S]*?button\.removeAttribute\("aria-busy"\)/);
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
  assert.match(app, /function removeRecentTaskReference\(taskId, analysisBase = ""\)[\s\S]*?removeRecentTask\(taskId, localStorage, undefined, analysisBase\)[\s\S]*?const persistedState = safeReadRecentTasksState\(\)[\s\S]*?targetStillExists[\s\S]*?服务端任务和报告不会被删除/);
  assert.match(app, /function removeRecentTaskReference[\s\S]*?无法确认最近任务引用已移除/);
  assert.match(app, /function clearRecentTaskHistory\(\)[\s\S]*?clearRecentTasks\(localStorage\)[\s\S]*?const clearedState = safeReadRecentTasksState\(\)[\s\S]*?无法确认最近任务历史已清空[\s\S]*?服务端记录未删除/);
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
  assert.match(app, /import \{ applyTheme, persistTheme, readTheme(?:, THEME_STORAGE_KEY)? \} from "\.\/theme\.js"/);
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
  assert.match(html, /服务器配置 API 待接入/);
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


test("market hot-rank actions accept the symbol or code field as the stock identifier", () => {
  assert.match(marketOverview, /const stock=el\("td"\),button=el\("button","market-hot-stock-link"\),symbol=String\(item\.symbol\|\|item\.code\|\|""\)\.trim\(\);/);
  assert.match(marketOverview, /button\.dataset\.hotSymbol=symbol;button\.dataset\.hotName=String\(item\.name\|\|symbol\|\|""\);button\.disabled=!symbol/);
});

test("dragon-tiger rows only render stock navigation when a usable code exists", () => {
  assert.match(marketActivity, /const symbol=String\(item\.symbol\|\|item\.code\|\|""\)\.trim\(\);/);
  assert.match(marketActivity, /if\(symbol\)\{\s*const button=el\("button","activity-stock-link","查看行情"\);button\.type="button";button\.dataset\.dragonSymbol=symbol/);
  assert.match(marketActivity, /button\[data-dragon-symbol\]/);
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


test("trading calendar aligns actual dates and keeps missing status distinct from market closure", () => {
  assert.match(marketActivity, /function calendarTimestamp\(value\)/);
  assert.match(marketActivity, /for\(let timestamp=startTime;timestamp<=endTime;timestamp\+=86400000\)/);
  assert.match(marketActivity, /function renderCalendarDay\(root,date,item\)/);
  assert.match(marketActivity, /item\.is_trading_day===true\?"is-trading-day":item\.is_trading_day===false\?"is-closed-day":"is-unknown-day"/);
  assert.match(marketActivity, /is-missing-day[\s\S]*?接口未返回该日期的交易日状态/);
  assert.match(marketActivity, /missingDates>0\|\|reportedMismatch\|\|rangeMismatch/);
  assert.match(stylesheet, /\.trading-calendar-day\.is-unknown-day span,[\s\S]*?\.trading-calendar-day\.is-missing-day span/);
});

test("market activity date filter changes cancel old requests and clear mismatched results", () => {
  assert.match(marketActivity, /function invalidateDragonFilter\(\)[\s\S]*?abort\("dragon"\);[\s\S]*?dragonHasMore=false[\s\S]*?交易日期已变化/);
  assert.match(marketActivity, /function invalidateCalendarFilter\(\)[\s\S]*?abort\("calendar"\);[\s\S]*?日期筛选已变更[\s\S]*?日期范围已变化/);
  assert.match(marketActivity, /\$\("#dragon-tiger-date"\)\.addEventListener\("change",invalidateDragonFilter\)/);
  assert.match(marketActivity, /\$\("#calendar-start-date"\)\.addEventListener\("change",invalidateCalendarFilter\)/);
  assert.match(marketActivity, /\$\("#calendar-end-date"\)\.addEventListener\("change",invalidateCalendarFilter\)/);
});

test("changing fundamentals report period cancels old results until reloaded", () => {
  assert.match(stockResearch, /function invalidateFundamentalsPeriod\(\)[\s\S]*?abort\("fundamentals"\)[\s\S]*?clearPanel\("fundamentals","财务报告期已变更[\s\S]*?setQuality\("fundamentals-quality","unknown","待重新加载"\)/);
  assert.match(stockResearch, /\$\("#fundamentals-year"\)\?\.addEventListener\("change",invalidateFundamentalsPeriod\)/);
  assert.match(stockResearch, /\$\("#fundamentals-quarter"\)\?\.addEventListener\("change",invalidateFundamentalsPeriod\)/);
});

test("manual trading-calendar date ranges keep month navigation in sync", () => {
  assert.ok(marketActivity.includes("function syncCalendarMonthNavigation()"));
  assert.ok(marketActivity.includes("monthOffset=selectedYear*12+selectedMonth-(Number(current.year)*12+Number(current.month)-1)"));
  assert.match(marketActivity, /function invalidateCalendarFilter\(\)[\s\S]*?syncCalendarMonthNavigation\(\)/);
  assert.match(marketActivity, /meta\("calendar-month-label",label\)/);
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
  assert.match(app, /async function checkConnectionFromSettings\(\)/);
  assert.match(app, /let settingsToCheck = null;[\s\S]*?settingsToCheck = mergeConnectionDraft\(testedDraft, state\.settings\);/);
  assert.match(app, /runHealthCheck\(marketHealth, settingsToCheck\)/);
  assert.match(app, /runHealthCheck\(analysisHealth, settingsToCheck\)/);
  assert.ok(healthCheck.includes("export async function runHealthCheck(check, settings, timeoutMs = 6000)"));
  assert.ok(healthCheck.includes("Promise.race([request, timeout])"));
  assert.match(app, /const marketState = serviceHealthState\(marketResult\);/);
  assert.match(app, /const analysisState = serviceHealthState\(analysisResult\);/);
  assert.match(app, /API 状态未知（回包未提供可识别的健康状态）/);
  assert.match(app, /const currentSettingsToCheck = mergeConnectionDraft\(currentDraft, state\.settings\);/);
  assert.match(app, /connectionDraftChanged\(testedDraft, currentDraft\)/);
  assert.match(app, /currentSettingsToCheck\.marketBase !== settingsToCheck\.marketBase/);
  assert.match(app, /currentSettingsToCheck\.analysisBase !== settingsToCheck\.analysisBase/);
  assert.match(app, /currentSettingsToCheck\.apiKey !== settingsToCheck\.apiKey/);
  assert.match(app, /连接地址、API Key 或已保存配置发生变化，已忽略旧结果/);
  assert.match(app, /不会更改全局连接状态/);
  assert.match(html, /服务器配置 API 待接入/);
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


test("stock research toolbar controls use readable labels and touch-sized targets", () => {
  assert.match(finalCssRuleBody(".stock-research-toolbar label"), /font-size:12px/);
  assert.match(finalCssRuleBody(".stock-research-toolbar select"), /min-height:38px/);
  assert.match(finalCssRuleBody(".stock-research-toolbar select"), /font-size:12px/);
  assert.match(finalCssRuleBody(".stock-research-toolbar .button"), /min-height:38px/);
  assert.match(finalCssRuleBody(".stock-research-toolbar .button"), /font-size:12px/);
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
  assert.match(finalCssRuleBody(".activity-stock-link"), /min-height: 38px/);
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
    "research", "tasks", "data", "notifications", "chat", "screening", "alerts", "settings"
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
  assert.doesNotMatch(
    stylesheet,
    /\.watchlist-name-input::placeholder\s*\{\s*color:\s*#[0-9a-fA-F]{3,8}\s*;/,
    "a later hard-coded watchlist placeholder rule must not override the shared theme token"
  );
});



test("Run Flow 与结构化结果在浅色主题下使用可读的前景文字", () => {
  const mainTextSelectors = [
    ".task-empty strong",
    ".task-id-row code",
    ".task-facts strong",
    ".event-body strong",
    ".boundary-grid strong",
    ".signal-result-card strong",
    ".analysis-result-block > h3",
    ".analysis-result-card-heading strong",
    ".source-summary strong",
    ".supplemental-card-heading h3"
  ];
  for (const selector of mainTextSelectors) {
    assert.match(
      baseCssDeclarationBlock(selector),
      /color:\s*var\(--text\)\s*;/,
      selector + " must use the theme-aware primary text token"
    );
  }

  const dimTextSelectors = [
    ".task-empty",
    ".task-id-row > span",
    ".event-body p",
    ".boundary-grid p",
    ".task-result-note",
    ".task-result-metric small",
    ".signal-result-card p",
    ".analysis-result-block > p",
    ".analysis-result-fields dt",
    ".analysis-result-empty",
    ".raw-result",
    ".empty-row",
    ".table-empty",
    ".query-hint",
    ".metric-foot",
    ".chart-empty",
    ".chart-foot",
    ".source-version",
    ".source-main small",
    ".table-footer",
    ".page-footer",
    ".recent-analysis-item-main small",
    ".watchlist-item-meta",
    ".supplemental-context",
    ".supplemental-card-heading small",
    ".supplemental-fields dt",
    ".supplemental-reasons",
    ".section-heading h2 span",
    ".recent-analysis-feedback"
  ];
  for (const selector of dimTextSelectors) {
    assert.match(
      baseCssDeclarationBlock(selector),
      /color:\s*var\(--text-dim\)\s*;/,
      selector + " must use the theme-aware secondary text token"
    );
  }

  assert.match(baseCssDeclarationBlock(".chart-axis-label"), /fill:\s*var\(--text-dim\)\s*;/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.analysis-result-badge\.is-allowed \{ color: #08765e; border-color: #b9e6d6; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.analysis-result-badge\.is-blocked \{ color: #b92f41; border-color: #f2c3c9; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.task-result-warnings/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.inline-callout strong \{ color: #80540b; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.settings-feedback,?\s*[\s\S]*?:root\[data-theme="light"\] \.recent-analysis-feedback\.is-error \{ color: #b92f41; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.supplemental-feedback\.is-warning \{ color: #80540b; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.supplemental-feedback\.is-error \{ color: #b92f41; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.soft-badge \{ color: #245d95; background: #eaf3ff; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.text-button:hover \{ color: var\(--accent-strong\); \}/);
});

test("新增市场、数据中心与研究区块保持 12px 辅助文字基线", () => {
  const readableRules = [
    [/\.market-extension-metric small,\s*\.market-extension-metric span\s*\{[^}]*font-size:\s*12px;/, "market extension labels"],
    [/\.market-planned-state p\s*\{[^}]*font-size:\s*12px;/, "market planned-state copy"],
    [/\.market-global-placeholder span\s*\{[^}]*font-size:\s*12px;/, "global market placeholder copy"],
    [/\.data-center-summary-counts small\s*\{[^}]*font-size:\s*12px;/, "data-center summary labels"],
    [/\.data-center-service-heading p\s*\{[^}]*font-size:\s*12px;/, "data-center service descriptions"],
    [/\.data-center-service-icon\s*\{[^}]*font-size:\s*12px;/, "data-center service icon labels"],
    [/\.data-center-check-row\s*\{[^}]*font-size:\s*12px;/, "data-center check details"],
    [/\.data-center-service-detail\s*\{[^}]*font-size:\s*12px;/, "data-center service details"],
    [/\.data-center-capability-status\s*\{[^}]*font-size:\s*12px;/, "capability status labels"],
    [/\.data-center-provider-row small\s*\{[^}]*font-size:\s*12px;/, "provider descriptions"],
    [/\.data-center-capability-tag\s*\{[^}]*font-size:\s*12px;/, "capability tags"],
    [/\.data-center-diagnostic-note\s*\{[^}]*font-size:\s*12px;/, "diagnostic notes"],
    [/\.research-list-symbol\s*\{[^}]*font-size:\s*12px;/, "research list symbols"],
    [/\.research-list-action\s*\{[^}]*font-size:\s*12px;/, "research list actions"],
    [/\.watchlist-tracking-line\s*\{[^}]*font-size:\s*12px;/, "watchlist tracking details"],
    [/\.research-overview-context span\s*\{[^}]*font-size:\s*12px;/, "research overview context"],
    [/\.research-overview-metric span\s*\{[^}]*font-size:\s*12px;/, "research overview labels"],
    [/\.research-overview-note\s*\{[^}]*font-size:\s*12px;/, "research overview notes"],
    [/\.research-detail-nav-label\s*\{[^}]*font-size:\s*12px;/, "research detail navigation labels"]
  ];
  for (const [rule, description] of readableRules) {
    assert.match(stylesheet, rule, description + " must respect the 12px support-text baseline");
  }
});

test("研究与设置分区导航会移动键盘焦点并按全部可见区块同步高亮", () => {
  assert.match(
    app,
    /function bindSectionNavigation\([\s\S]*?focusTarget\.focus\(\{ preventScroll: true \}\)[\s\S]*?const current = targets[\s\S]*?getBoundingClientRect\(\)/
  );
});

test("分区导航的程序化焦点在研究和设置页中可见", () => {
  assert.match(
    stylesheet,
    /#settings-view \.settings-section h2:focus,[\s\S]*?#research-view \.supplemental-card h3:focus\s*\{[^}]*outline:\s*2px solid var\(--accent\)/
  );
});

test("导入浏览器工作区不会把导入前的活动任务重新写回任务引用", () => {
  assert.match(
    app,
    /function applySettings\(nextSettings, \{ persistPreviousActiveTask = true \} = \{\}\)/
  );
  const importBody = app.match(/async function importBrowserDataFile\(file\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(
    importBody,
    /localStorage\.removeItem\(TASK_REFERENCE_KEY\);[\s\S]*?applySettings\(nextSettings, \{ persistPreviousActiveTask: false \}\);[\s\S]*?applyTheme\(imported\.theme\)/
  );
  const applyIndex = importBody.indexOf("applySettings(nextSettings, { persistPreviousActiveTask: false })");
  assert.ok(applyIndex >= 0, "workspace import must apply the validated settings");
  const rollbackIndex = importBody.indexOf("\n  } catch (error) {", applyIndex);
  assert.ok(rollbackIndex > applyIndex, "storage rollback must remain in the failure path");
  assert.doesNotMatch(
    importBody.slice(applyIndex, rollbackIndex),
    /localStorage\.(?:setItem|removeItem)\(|persistTheme\(/,
    "the successful import path must finish storage writes before applying in-memory settings"
  );
  assert.match(importBody.slice(rollbackIndex), /storageKeys\.forEach\([\s\S]*?localStorage\.(?:setItem|removeItem)\(/,
    "the failure path should attempt to restore the pre-import storage snapshot"
  );
});

test("切换到尚未接入的板块维度时，旧请求不能覆盖能力占位", () => {
  assert.match(
    marketOverview,
    /async function load\(key\)\{cancel\(key\);const seq=generation\[key\],[\s\S]*?if\(key==="sectors"&&sectorMetric!=="change"\)\{showSectorMetricUnavailable\(\);return "unsupported";\}/
  );
  assert.match(
    marketOverview,
    /sectorType=button\.dataset\.sectorType==="concept"\?"concept":"industry"[\s\S]*?void load\("sectors"\)/
  );
  assert.match(
    marketOverview,
    /sectorMetric=\["change","amount","five-day","flow"\][\s\S]*?void load\("sectors"\)/
  );
});

test("快照来源、估值与补充指标使用可读的 12px 辅助文字", () => {
  const readableRules = [
    [/\.bundle-snapshot-metrics small\s*\{[^}]*font-size:\s*12px;/, "snapshot metadata labels"],
    [/\.bundle-snapshot-source small\s*\{[^}]*font-size:\s*12px;/, "snapshot source labels"],
    [/\.bundle-snapshot-source strong\s*\{[^}]*font-size:\s*12px;/, "snapshot source names"],
    [/\.stock-valuation-metric > span\s*\{[^}]*font-size:\s*12px;/, "valuation metric labels"],
    [/\.stock-supplement-metric > span\s*\{[^}]*font-size:\s*12px;/, "stock supplement metric labels"]
  ];
  for (const [rule, description] of readableRules) {
    assert.match(stylesheet, rule, description + " must use at least 12px text");
  }
});

test("数据中心超时后即使探针迟到返回，也不能覆盖为成功", () => {
  assert.match(
    healthCheck,
    /\.then\(\(data\) => timedOut\s*\? \{ ok: false, error: new Error\("检查超时"\), timedOut: true/
  );
});

test("服务健康刷新不会覆盖行情查询区域的独立提示", () => {
  const refreshServices = app.match(/async function refreshServices\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(refreshServices, "refreshServices implementation must exist");
  assert.doesNotMatch(
    refreshServices,
    /#query-hint/,
    "service health feedback belongs to service indicators, not the current stock query hint"
  );
});

test("侧栏与顶栏服务状态只有在明确健康回包时才显示正常", () => {
  assert.ok(app.includes('if (!result?.ok) return "failed";'));
  assert.ok(app.includes('const status = String(result.data?.status || "").trim().toLowerCase();'));
  assert.ok(app.includes('if (!status) return "unknown";'));
  assert.match(app, /function setServiceStatus\(prefix, status, text\)[\s\S]*?status === "healthy"[\s\S]*?status === "unknown" \|\| status === "degraded"/);
  assert.match(stylesheet, /\.status-dot\.is-unknown\s*\{[^}]*background:\s*var\(--text-dim\)/);
});

test("数据中心不会把缺少 status 的 2xx 回包标记为健康", () => {
  assert.match(dataCenter, /if \(!raw\) return "unknown";/);
  assert.match(
    dataCenter,
    /current === "degraded" \|\| current === "unknown" \? "is-warning" : "is-error"/
  );
  assert.match(dataCenter, /const unknownServices = serviceStates\.filter\(\(status\) => status === "unknown"\)\.length;/);
  assert.match(dataCenter, /const unknown = unknownServices \+ unknownDependencies/);
});


test("最近任务本地存储不可读或损坏时不覆盖原始历史，导出不会静默遗漏记录", () => {
  assert.match(recentTaskModule, /export function readRecentTasksState\(storage, limit = MAX_RECENT_TASKS\)/);
  assert.match(recentTaskModule, /if \(!currentState\.storageReadable \|\| !currentState\.storageValid\) return currentState\.items/);
  assert.match(app, /function safeReadRecentTasksState\(\)/);
  assert.match(app, /function renderRecentTasks\(records\)[\s\S]*?historyState\.storageReadable[\s\S]*?historyState\.storageValid[\s\S]*?历史修改已暂停/);
  assert.match(app, /recent_tasks: \(\(\) => \{[\s\S]*?recentTasksState\.storageReadable[\s\S]*?recentTasksState\.storageValid[\s\S]*?导出已取消/);
});


test("估值请求补齐交易所后缀，并只展示与当前标的匹配的记录", () => {
  const renderer = stockResearch.match(/function renderValuations\(data,symbol\)\{[\s\S]*?\n \}\n function fractionPercent/)?.[0] || "";
  assert.ok(renderer, "估值渲染器应存在");
  assert.match(stockResearch, /function normalizeValuationSymbol\(input\)/);
  assert.ok(stockResearch.includes('if(value.startsWith("6"))return value+".SH";'));
  assert.ok(stockResearch.includes('if(/^[03]/.test(value))return value+".SZ";'));
  assert.ok(stockResearch.includes('return value+".BJ";'));
  assert.ok(stockResearch.includes("getMarketValuations(settings,[normalizeValuationSymbol(sym)],signal)"));
  assert.match(renderer, /const requestedSymbol=String\(symbol\|\|data\.requested_symbols\?\.\[0\]\|\|"\)\.trim\(\)/);
  assert.match(renderer, /const targetSymbol=normalizeValuationSymbol\(requestedSymbol\)/);
  assert.match(renderer, /items\.find\(value=>sameValuationSymbol\(value\.symbol,targetSymbol\)\)/);
  assert.doesNotMatch(renderer, /items\[0\]/, "目标股票未匹配时不得退回展示列表第一条记录");
  assert.match(renderer, /估值接口返回的证券代码与当前标的不匹配，已忽略这些记录/);
});

 
test("个股研究响应代码不匹配时忽略结果", () => {
  assert.match(stockResearch, /function assertResponseSymbol\(data,requestedSymbol\)/);
  assert.match(stockResearch, /if\(responseSymbol&&!sameValuationSymbol\(responseSymbol,requestedSymbol\)\)/);
  assert.match(stockResearch, /服务端返回证券代码 \+responseSymbol\+ 与请求标的 \+requestedSymbol\+ 不匹配，已忽略结果/);
  assert.match(stockResearch, /async function fetchOne\(key,request,render\)[\s\S]*?assertResponseSymbol\(data,symbol\);[\s\S]*?render\(data,symbol\)/);
  assert.match(stockResearch, /async function loadAnnouncements\(append=false\)[\s\S]*?getStockAnnouncements\(settings,symbol,[\s\S]*?assertResponseSymbol\(data,symbol\);[\s\S]*?const root=\$\("#announcement-list"\)/);
});

 
test("主行情、日 K 与统一快照拒绝证券代码不匹配的响应", () => {
  assert.match(app, /function comparableMarketSymbol\(input\)/);
  assert.match(app, /function responseSymbolMismatch\(payload, requestedSymbol, label\)/);
  assert.match(app, /responseSymbolMismatch\(bundleResponse, symbol, "统一行情快照"\)/);
  assert.match(app, /responseSymbolMismatch\(bundleResponse\.quote, symbol, "统一快照中的实时行情"\)/);
  assert.match(app, /responseSymbolMismatch\(quoteResult\.value, symbol, "实时行情"\)/);
  assert.match(app, /responseSymbolMismatch\(barsResult\.value, symbol, "日 K"\)/);
  assert.match(app, /quote: null,[\s\S]*?warnings: \[\.\.\.\(Array\.isArray\(bundleResponse\.warnings\)/);
  assert.ok(app.includes('return label + "返回的证券代码 " + responseSymbol + " 与请求标的 " + requestedSymbol + " 不匹配，已忽略结果。"'));
});

 
test("分析任务、阶段事件与 Run 响应必须匹配当前请求标识", () => {
  assert.match(app, /function responseIdentityError\(message\)[\s\S]*?error\.responseIdentityMismatch = true/);
  assert.match(app, /candidate\.task_id !== taskId[\s\S]*?任务状态接口未返回 task_id/);
  assert.match(app, /eventPayload\.task_id !== taskId[\s\S]*?阶段事件接口未返回 task_id/);
  assert.match(app, /if \(taskError && \(!isRetryableApiError\(taskError\) \|\| taskError\.responseIdentityMismatch\)\)/);
  assert.match(app, /if \(isRetryableApiError\(eventError\) && !eventError\.responseIdentityMismatch\)/);
  assert.match(app, /if \(result\?\.run_id !== runId\) \{[\s\S]*?Run 结果接口未返回 run_id/);
  assert.match(app, /!error\.responseIdentityMismatch && isRetryableApiError\(error\)/);
});

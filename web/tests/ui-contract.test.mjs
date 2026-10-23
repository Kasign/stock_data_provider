import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [html, app, format, domain, openapi, recentTaskModule, stylesheet, watchlistModule, apiClient, taskQueue] = await Promise.all([
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("../src/format.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/analysis/domain/types.go", import.meta.url), "utf8"),
  readFile(new URL("../../docs/architecture/analysis-tasks.openapi.yaml", import.meta.url), "utf8"),
  readFile(new URL("../src/recent-tasks.js", import.meta.url), "utf8"),
  readFile(new URL("../styles.css", import.meta.url), "utf8"),
  readFile(new URL("../src/watchlist.js", import.meta.url), "utf8"),
  readFile(new URL("../src/api.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/platform/taskqueue/queue.go", import.meta.url), "utf8")
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
  assert.match(app, /\$\("#settings-dialog"\)\.addEventListener\("close", \(\) => setMarketKeyVisibility\(false\)\)/);
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
  assert.match(app, /function removeRecentTaskReference\(taskId\)[\s\S]*?removeRecentTask\(taskId, localStorage\)[\s\S]*?服务端任务和报告不会被删除/);
  assert.match(app, /function clearRecentTaskHistory\(\)[\s\S]*?window\.confirm\([\s\S]*?clearRecentTasks\(localStorage\)[\s\S]*?服务端记录未删除/);
  assert.match(app, /state\.suppressedHistoryTaskId === state\.taskId/);
  assert.match(app, /if \(state\.taskId === taskId\) state\.suppressedHistoryTaskId = taskId/);
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
  assert.match(app, /function setMobileNavigationOpen\(open\)/);
  assert.match(app, /\$\("#mobile-nav-overlay"\)\.addEventListener\("click", \(\) => setMobileNavigationOpen\(false\)\)/);
  assert.match(app, /if \(event\.key === "Escape"\) setMobileNavigationOpen\(false\)/);
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
  assert.match(html, /不会展示模拟通知/);
  assert.match(html, /不会用单次分析报告冒充多轮 AI 对话/);
  assert.match(html, /不能提交，不会产生模拟结果/);
  assert.match(html, /不会在浏览器里假保存规则/);
  assert.match(html, /服务端配置 API 待接入/);
  assert.match(html, /密码\/权限、服务器 \.env 导入导出需要认证/);
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

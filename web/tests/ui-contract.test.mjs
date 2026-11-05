import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [html, appSource, format, domain, openapi, recentTaskModule, stylesheet, watchlistModule, apiClient, taskQueue, marketActivity, stockResearch, marketOverview, serviceWiring, dataCenter, healthCheck, marketHTTPServer, marketOpenAPI, marketStocksModule, notificationPreferences, taskWorkflow, localAlertDrafts, taskEventFormat, taskEventView, taskEventNode, taskResultView, watchlistItemView] = await Promise.all([
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("../src/format.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/analysis/domain/types.go", import.meta.url), "utf8"),
  readFile(new URL("../../doc/api/分析任务.openapi.yaml", import.meta.url), "utf8"),
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
  readFile(new URL("../src/health-check.js", import.meta.url), "utf8"),
  readFile(new URL("../../services/market-data/internal/httpapi/server.go", import.meta.url), "utf8"),
  readFile(new URL("../../services/market-data/openapi.yaml", import.meta.url), "utf8"),
  readFile(new URL("../src/market-stocks.js", import.meta.url), "utf8"),
  readFile(new URL("../src/notification-preferences.js", import.meta.url), "utf8"),
  readFile(new URL("../src/task-workflow.js", import.meta.url), "utf8"),
  readFile(new URL("../src/local-alert-drafts.js", import.meta.url), "utf8"),
  readFile(new URL("../src/task-event-format.js", import.meta.url), "utf8"),
  readFile(new URL("../src/task-event-view.js", import.meta.url), "utf8"),
  readFile(new URL("../src/task-event-node.js", import.meta.url), "utf8"),
  readFile(new URL("../src/task-result-view.js", import.meta.url), "utf8"),
  readFile(new URL("../src/watchlist-item-view.js", import.meta.url), "utf8")
]);
const settingsNavigation = await readFile(new URL("../src/settings-navigation.js", import.meta.url), "utf8");
const recentTaskView = await readFile(new URL("../src/recent-task-view.js", import.meta.url), "utf8");
const stockSearchView = await readFile(new URL("../src/stock-search-view.js", import.meta.url), "utf8");
const watchlistImportPreviewView = await readFile(new URL("../src/watchlist-import-preview-view.js", import.meta.url), "utf8");
const watchlistTrackingViewModel = await readFile(new URL("../src/watchlist-tracking-view-model.js", import.meta.url), "utf8");
const watchlistListViewModel = await readFile(new URL("../src/watchlist-list-view-model.js", import.meta.url), "utf8");
const taskEventListViewModel = await readFile(new URL("../src/task-event-list-view-model.js", import.meta.url), "utf8");
const taskFlowSummaryViewModel = await readFile(new URL("../src/task-flow-summary-view-model.js", import.meta.url), "utf8");

// UI 控制器和功能模块分拆后，跨模块契约应检查实际参与运行的源码集合。
const app = [appSource, recentTaskModule, watchlistModule, marketOverview, marketActivity, stockResearch, dataCenter, healthCheck, marketStocksModule, taskWorkflow, watchlistItemView, watchlistImportPreviewView, watchlistTrackingViewModel, watchlistListViewModel].join("\n");

function finalCssRuleBody(selector) {
  const start = stylesheet.lastIndexOf(selector);
  assert.notEqual(start, -1, "stylesheet should contain " + selector);
  const open = stylesheet.indexOf("{", start + selector.length);
  const close = stylesheet.indexOf("}", open + 1);
  assert.ok(open > start && close > open, "stylesheet should have a complete rule for " + selector);
  return stylesheet.slice(open + 1, close);
}

function baseCssDeclarationBlock(selector) {
  const rules = [...stylesheet.matchAll(/([^{}]+)[{]([^{}]*)[}]/g)];
  const rule = rules.find((match) => match[1].split(",").some((part) => part.trim() === selector));
  return rule?.[2] || "";
}

function exactCssDeclarationBlock(selector) {
  const rules = [...stylesheet.matchAll(/([^{}]+)[{]([^{}]*)[}]/g)];
  const rule = rules.find((match) => match[1].trim() === selector);
  return rule?.[2] || "";
}

function sourceBetween(source, startMarker, endMarker, label) {
  const start = source.indexOf(startMarker);
  const end = source.indexOf(endMarker, start + startMarker.length);
  assert.ok(start >= 0 && end > start, label + " source boundaries must exist");
  return source.slice(start, end);
}

test("notification quiet hours disclose local-time and overnight semantics", () => {
  assert.match(html, /时间按当前设备本地时钟记录/);
  assert.match(html, /结束时间早于开始时间表示跨午夜/);
  assert.match(notificationPreferences, /equal-quiet-hours/);
});

test("static DOM IDs referenced by the WebUI exist and are unique", () => {
  const htmlIds = [...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]);
  assert.equal(htmlIds.length, new Set(htmlIds).size, "HTML must not contain duplicate id attributes");

  const idSet = new Set(htmlIds);
  const staticSelectors = [...app.matchAll(/\$\(["']#([a-zA-Z][\w-]*)["']\)/g)].map((match) => match[1]);
  const missing = [...new Set(staticSelectors.filter((id) => !idSet.has(id)))];
  assert.deepEqual(missing, [], "WebUI runtime JavaScript references missing static DOM IDs: " + missing.join(", "));
});

test("自选股辅助字段顺序与工作区导入预览契约完整", () => {
  assert.equal([...html.matchAll(/data-watchlist-field-order="/g)].length, 5);
  assert.equal([...html.matchAll(/data-watchlist-field-order-move="(?:up|down)"/g)].length, 10);
  assert.match(app, /watchlistDisplayOrder: \[\.\.\.document\.querySelectorAll\("\[data-watchlist-field-order\]"\)\]/);
  assert.match(app, /buildWatchlistTrackingFields\(\{/);
  assert.match(watchlistTrackingViewModel, /const displayFieldNodes = \{/);
  assert.match(watchlistTrackingViewModel, /updated: \{ className: "watchlist-item-meta"/);
  assert.match(watchlistTrackingViewModel, /\.filter\(\(field\) => Array\.isArray\(displayFields\) && displayFields\.includes\(field\)\)/);
  assert.match(watchlistItemView, /trackingFields\.forEach\(\(field\)/);
  assert.match(watchlistItemView, /className = "watchlist-tracking-summary"/);
  assert.match(app, /const BROWSER_DATA_VERSION = 3/);
  assert.match(app, /!\[1, 2, BROWSER_DATA_VERSION\]\.includes\(payload\.version\)/);
  assert.match(html, /<dialog[^>]+id="browser-data-import-dialog"/);
  assert.match(app, /watchlistConflicts[\s\S]*?taskConflicts[\s\S]*?showModal\(\)/);
  assert.match(app, /browser-data-import-replace/);
  assert.match(app, /browser-data-import-merge/);
  assert.match(app, /const currentWatchlistItems = currentWatchlist\.storageReadable/);
  assert.match(app, /rollbackFailures/);
  assert.match(app, /apiKey: state\.settings\.apiKey/);
  assert.doesNotMatch(app, /window\.prompt\(preview/);
});

test("自选股导入预览视图模块只构建 DOM，不接管页面状态或存储", () => {
  assert.match(appSource, /import \{ buildWatchlistImportPreviewView \} from "\.\/watchlist-import-preview-view\.js"/);
  assert.match(appSource, /buildWatchlistImportPreviewView\(\{ document, preview \}\)/);
  assert.match(appSource, /root\.replaceChildren\(\.\.\.view\.nodes\)/);
  assert.match(appSource, /state\.watchlistImportPreview = preview/);
  assert.match(watchlistImportPreviewView, /document\.createElement\("table"\)/);
  assert.match(watchlistImportPreviewView, /cell\.textContent = String\(value\)/);
  assert.doesNotMatch(watchlistImportPreviewView, /localStorage|document\.querySelector|addEventListener|fetch\s*\(/);
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
    assert.ok(taskResultView.includes("result." + field), "Run result adapter must render result." + field);
  }
  assert.doesNotMatch(taskResultView, /signal\.(score|confidence|rationale|reason)\b/, "do not display fields absent from CandidateSignal");
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
  assert.match(app, /function activateTaskReference\(record\)[\s\S]*?if \(!record \|\| typeof record\.task_id !== "string" \|\| !record\.task_id\.trim\(\)\) return false;[\s\S]*?stopTaskPolling\(\);[\s\S]*?state\.taskEvents\.clear\(\);[\s\S]*?void pollTaskSafely\(state\.pollGeneration\);/);
  assert.match(app, /if \(generation !== state\.pollGeneration \|\| taskId !== state\.taskId \|\|\s*endpointGeneration !== state\.analysisConnectionGeneration \|\|\s*state\.taskAnalysisBase !== analysisBase \|\|\s*state\.settings\.analysisBase !== analysisBase\) return;/);
  assert.match(app, /if \(generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\|\s*endpointGeneration !== state\.analysisConnectionGeneration \|\|\s*state\.taskAnalysisBase !== analysisBase \|\|\s*state\.settings\.analysisBase !== analysisBase\) return;/);
  assert.match(app, /if \(task && TERMINAL_TASK_STATUSES\.has\(task\.status\)\) \{[\s\S]*?stopTaskPolling\(\);[\s\S]*?const runId = task\.run_id \|\| state\.runId;[\s\S]*?if \(runId\) void loadTaskRunResult\(runId, taskId\);/);
  assert.match(recentTaskModule, /return \{[\s\S]*?task_id: record\.task_id\.trim\(\)[\s\S]*?status: typeof record\.status === "string" \? record\.status : ""/);
});


test("restoring a recent task clears the previous task status badge while waiting", () => {
  const start = app.indexOf("function activateTaskReference(record) {");
  const end = app.indexOf("\nfunction ", start + 1);
  const restore = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(restore.includes('state.taskStatusLoading = true;'), "恢复任务期间标记状态待确认");
  assert.ok(restore.includes('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");'), "恢复期间不沿用上一条任务的状态徽标");
  assert.ok(restore.indexOf('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");') < restore.indexOf("void pollTaskSafely(state.pollGeneration);"), "先显示恢复中，再请求服务端");
});

test("failed Run refresh distinguishes the last verified same-Run result from a new read", () => {
  const start = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const end = app.indexOf("\nfunction stopTaskPolling()", start);
  const loader = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(loader.includes("const retainedPreviousResult = Boolean(state.lastRunResult && state.lastRunResult.run_id === runId);"));
  assert.ok(loader.includes("页面保留的是此前已成功读取的同一 Run 结果，本次读取尚未确认。"));
  assert.ok(loader.includes("本次读取未成功，不代表结果已更新。"));
  assert.ok(loader.includes("renderTaskResult(result);"), "只有本次 Run 身份和标的校验通过才替换报告内容");
});

test("Run 结果重读采用单次请求锁并只由当前代际解锁按钮", () => {
  assert.match(app, /function setRunResultReloadBusy\(busy\) \{[\s\S]*?button\.disabled = Boolean\(busy\)[\s\S]*?button\.setAttribute\("aria-busy", "true"\)[\s\S]*?button\.textContent = "重新读取结果"/);
  const loaderStart = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const loaderEnd = app.indexOf("\nfunction stopTaskPolling()", loaderStart);
  const loader = loaderStart >= 0 && loaderEnd > loaderStart ? app.slice(loaderStart, loaderEnd) : "";
  assert.match(loader, /reload-run-result.*disabled[\s\S]*?\["loading", "retrying"\][\s\S]*?return;/);
  assert.match(loader, /setRunResultReloadBusy\(true\)/);
  assert.match(loader, /finally \{[\s\S]*?if \(generation === state\.runResultGeneration\) \{[\s\S]*?setRunResultReloadBusy\(false\);[\s\S]*?\}\s*\}/);
  assert.equal((app.match(/state\.runResultGeneration \+= 1;\s*releaseRunResultReloadBusy\(\);/g) || []).length, 3, "新建任务、恢复任务和分析地址切换都必须通过安全入口解除旧请求锁");
});

test("watchlist marks a retained Run result when re-reading fails and refreshes on recovery", () => {
  assert.ok(app.includes('runResultReadStatus: ""'));
  assert.ok(app.includes('runResultReadError: ""'));
  const watchlistStart = app.indexOf("function renderWatchlist(");
  const watchlistEnd = app.indexOf("\nfunction addCurrentSymbolToWatchlist(", watchlistStart);
  const watchlist = app.slice(watchlistStart, watchlistEnd);
  assert.match(watchlistTrackingViewModel, /正在重读，尚未确认/);
  assert.match(watchlistTrackingViewModel, /重试中，尚未确认/);
  assert.match(watchlistTrackingViewModel, /本次读取失败/);
  const resultStart = app.indexOf("function renderTaskResult(result) {");
  const resultEnd = app.indexOf("\nfunction ", resultStart + 10);
  assert.match(app.slice(resultStart, resultEnd), /state\.runResultReadStatus = "";[\s\S]*?state\.runResultReadError = ""/);
  const loaderStart = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const loaderEnd = app.indexOf("\nfunction stopTaskPolling()", loaderStart);
  const loader = app.slice(loaderStart, loaderEnd);
  assert.match(watchlistTrackingViewModel, /当前 Run 结果：正在读取服务端结果/);
  assert.match(watchlistTrackingViewModel, /当前 Run 结果：暂时不可读，正在自动重试/);
  assert.match(watchlistTrackingViewModel, /当前 Run 结果：读取失败/);
  assert.match(loader, /state\.runResultReadStatus = "loading"/);
  assert.match(loader, /state\.runResultReadStatus = "retrying"[\s\S]*?if \(state\.view === "watchlist"\) renderWatchlist\(\)/);
  assert.match(loader, /state\.runResultReadStatus = "error"[\s\S]*?if \(state\.view === "watchlist"\) renderWatchlist\(\)/);
});

test("creation, contract confirmation, and history restoration use distinct task status labels", () => {
  const createStart = app.indexOf("async function createTask(event) {");
  const createEnd = app.indexOf("\nfunction startAnalysisForSymbol(", createStart);
  const creator = app.slice(createStart, createEnd);
  assert.ok(creator.includes('setQualityPill($("#task-status-pill"), "unknown", "正在创建");'));
  assert.ok(creator.includes('setQualityPill($("#task-status-pill"), "unknown", "正在确认");'));
  assert.ok(!creator.includes('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");'));
  const restoreStart = app.indexOf("function activateTaskReference(record) {");
  const restoreEnd = app.indexOf("\nfunction restoreLastTask(", restoreStart);
  assert.ok(app.slice(restoreStart, restoreEnd).includes('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");'));
});

test("task restoration and manual refresh mark cached status as unconfirmed", () => {
  assert.ok(app.includes("taskStatusLoading: false"));
  const pollStart = app.indexOf("async function pollTask(");
  const pollEnd = app.indexOf("\nasync function createTask(", pollStart);
  const poller = app.slice(pollStart, pollEnd);
  assert.match(poller, /task = candidate;\s*state\.taskStatusLoading = false;\s*state\.taskStatusError = ""/);
  assert.match(poller, /if \(taskError\) \{\s*state\.taskStatusLoading = false;/);
  const watchStart = app.indexOf("function renderWatchlist(");
  const watchEnd = app.indexOf("\nfunction addCurrentSymbolToWatchlist(", watchStart);
  const watchlist = app.slice(watchStart, watchEnd);
  assert.match(watchlistTrackingViewModel, /currentTask\.statusLoading \|\| Boolean\(currentTask\.statusError\)/);
  assert.match(watchlistTrackingViewModel, /正在确认服务端状态（上次状态：/);
  const manualStart = app.indexOf('$("#refresh-task").addEventListener("click"');
  const manualEnd = app.indexOf('$("#recent-analysis-search")', manualStart);
  const manual = app.slice(manualStart, manualEnd);
  assert.match(manual, /state\.taskStatusLoading = true;[\s\S]*?setQualityPill\(\$\("#task-status-pill"\), "unknown", "正在刷新"\)/);
});

test("立即刷新状态使用忙碌锁阻止重复 Task/事件轮询", () => {
  assert.match(app, /function setTaskRefreshBusy\(busy\) \{[\s\S]*?button\.disabled = Boolean\(busy\)[\s\S]*?button\.setAttribute\("aria-busy", "true"\)[\s\S]*?button\.textContent = "立即刷新状态"/);
  const manualStart = app.indexOf('$("#refresh-task").addEventListener("click"');
  const manualEnd = app.indexOf('$("#recent-analysis-search")', manualStart);
  const manual = manualStart >= 0 && manualEnd > manualStart ? app.slice(manualStart, manualEnd) : "";
  assert.match(manual, /if \(!state\.taskId \|\| \$\("#refresh-task"\)\.disabled\) return/);
  assert.match(manual, /stopTaskPolling\(\);[\s\S]*?setTaskRefreshBusy\(true\)/);
  assert.match(manual, /pollTaskSafely\(generation\)\.finally\(\(\) => \{\s*if \(generation === state\.pollGeneration\) \{[\s\S]*?setTaskRefreshBusy\(false\);[\s\S]*?\} catch \{/);
  assert.match(app, /function stopTaskPolling\(\) \{[\s\S]*?state\.pollGeneration \+= 1;[\s\S]*?try \{\s*setTaskRefreshBusy\(false\);\s*\} catch \{/);
});


test("刷新当前页面会隔离刷新错误并安全释放工具栏按钮", () => {
  const start = app.indexOf('$("#refresh-button").addEventListener("click", async () => {');
  const end = app.indexOf('\n});', start);
  const handler = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(handler, "刷新按钮必须绑定当前页面刷新流程");
  assert.match(handler, /let ownsBusyState = false/);
  assert.match(handler, /if \(button\?\.disabled\) return;[\s\S]*?ownsBusyState = true/);
  assert.match(handler, /await refreshServices\(\)/);
  assert.match(handler, /finally[\s\S]*?ownsBusyState[\s\S]*?button\.disabled = false/);
  assert.match(handler, /appendFailure\("当前页面刷新失败："/);
});
test("数据源能力刷新初始化或最终解锁异常不会产生未处理 rejection", () => {
  const start = app.indexOf("async function refreshSources() {");
  const end = app.indexOf("\nfunction shanghaiDateKey(", start);
  const refresh = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.match(refresh, /let button = null;[\s\S]*?let list = null;[\s\S]*?try \{/);
  assert.match(refresh, /showMessage\("正在读取当前连接配置对应的数据源能力…"/);
  assert.match(refresh, /try \{\s*if \(button\) \{\s*button\.disabled = true;[\s\S]*?catch \{/);
  assert.ok(refresh.includes("const payload = await getDataSources(settings, controller.signal);"));
  assert.ok(refresh.includes("controller.signal.aborted || generation !== sourceRefreshGeneration"));
  assert.ok(refresh.includes("if (controller.signal.aborted || generation !== sourceRefreshGeneration) return;"));
  assert.ok(refresh.includes('showMessage(message, "empty-row error-text")'));
  assert.ok(refresh.includes("if (generation === sourceRefreshGeneration && sourceRefreshController === controller) {"));
  assert.ok(refresh.includes("button.disabled = false;"));

});

test("已保存连接检查与草稿检查的初始化/解锁异常不会产生未处理 rejection", () => {
  const saved = app.match(/async function checkConnectionFromSettings\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  const draft = app.match(/async function checkDraftConnectionFromSettings\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(saved, /generation !== savedConnectionCheckGeneration/);
  assert.match(saved, /savedConnectionCheckController = controller/);
  assert.match(saved, /runHealthCheck\(marketHealth, settingsToCheck, 6000, controller\.signal\)/);
  assert.match(saved, /finally[\s\S]*?savedConnectionCheckController === controller/);
  assert.match(saved, /finally[\s\S]*?releaseSettingsCheckButtonSafely/);
  assert.match(draft, /generation !== draftConnectionCheckGeneration/);
  assert.match(draft, /draftConnectionCheckController = controller/);
  assert.match(draft, /runHealthCheck\(marketHealth, settingsToCheck, 6000, controller\.signal\)/);
  assert.match(draft, /finally[\s\S]*?draftConnectionCheckController === controller/);
  assert.match(draft, /finally[\s\S]*?releaseSettingsCheckButtonSafely/);
  assert.match(draft, /testedDraft = readConnectionDraft\(\)/);
});
test("applySettings 的单项清理失败不会阻断其余配置应用步骤", () => {
  const start = app.indexOf("function applySettings(");
  const end = app.indexOf("\nfunction saveSettingsFromForm(", start);
  const apply = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(apply);
  assert.match(apply, /runSettingsApplyStep\("清理旧分析任务失败："/);
  assert.match(apply, /runSettingsApplyStep\("失效市场总览请求失败："/);
  assert.match(apply, /savedConnectionCheckController\?\.abort\(\)/);
  assert.match(apply, /draftConnectionCheckController\?\.abort\(\)/);
  assert.match(apply, /恢复草稿连接检查按钮失败：[\s\S]*?releaseSettingsCheckButtonSafely\(\$\("#settings-check-draft-connection"\), "检查未保存配置"\)/);
  assert.match(apply, /runSettingsApplyStep\("刷新服务健康状态失败："/);
  assert.match(apply, /reportSettingsApplyFailures\(failures\)/);
});
test("任务切换与分析 API 切换不会被按钮解锁异常中断", () => {
  assert.match(app, /function releaseRunResultReloadBusy\(\) \{[\s\S]*?try \{\s*setRunResultReloadBusy\(false\);\s*\} catch[\s\S]*?继续完成上下文切换/);

  const stop = app.match(/function stopTaskPolling\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(stop, "停止轮询共用函数必须存在");
  assert.match(stop, /state\.pollGeneration \+= 1;[\s\S]*?try \{[\s\S]*?setTaskRefreshBusy\(false\);[\s\S]*?catch/);
  assert.ok(stop.includes("const timer = state.pollTimer;") && stop.includes("state.pollTimer = null"));

  const contexts = [
    ["创建任务", "async function createTask(event) {", "function startAnalysisForSymbol("],
    ["恢复最近任务", "function activateTaskReference(record) {", "function restoreLastTask("],
    ["分析 API 地址切换", "function invalidateActiveTaskForAnalysisBaseChange(", "function applySettings("]
  ];
  for (const [label, startMarker, endMarker] of contexts) {
    const start = app.indexOf(startMarker);
    const end = app.indexOf(endMarker, start + startMarker.length);
    const body = start >= 0 && end > start ? app.slice(start, end) : "";
    assert.ok(body.includes("releaseRunResultReloadBusy();"), label + " 应使用不会冒泡的 Run 按钮解锁");
    assert.ok(body.includes("stopTaskPolling();"), label + " 应停止旧轮询");
    assert.doesNotMatch(body, /setRunResultReloadBusy\(false\);/, label + " 不应直接调用可能中断清理的按钮解锁");
  }
});

test("恢复最近任务时界面渲染失败仍会启动安全轮询", () => {
  const start = app.indexOf("function activateTaskReference(record) {");
  const end = app.indexOf("\nfunction restoreLastTask(", start);
  const activate = start >= 0 && end > start ? app.slice(start, end) : "";
  const taskBinding = activate.indexOf("state.taskId = record.task_id;");
  const uiTry = activate.indexOf("try {", taskBinding);
  const uiCatch = activate.indexOf("catch (error)", uiTry);
  const safePoll = activate.indexOf("void pollTaskSafely(state.pollGeneration);", uiCatch);
  assert.ok(taskBinding >= 0 && uiTry > taskBinding && uiCatch > uiTry, "先绑定任务上下文，再保护初始视图渲染");
  assert.ok(safePoll > uiCatch, "即使初始视图渲染失败，仍应发起安全轮询");
  assert.match(activate.slice(uiCatch), /恢复任务界面时发生错误：[\s\S]*?正在继续读取服务端状态[\s\S]*?void pollTaskSafely\(state\.pollGeneration\)/);
  assert.match(activate, /state\.taskId = record\.task_id;[\s\S]*?state\.taskEvents\.clear\(\);[\s\S]*?try \{/);
});

test("任务轮询意外异常会暂停自动刷新并保留手动重试入口", () => {
  const safeStart = app.indexOf("function pollTaskSafely(");
  const safeEnd = app.indexOf("\nasync function createTask(", safeStart);
  const safePoll = safeStart >= 0 && safeEnd > safeStart ? app.slice(safeStart, safeEnd) : "";
  assert.match(safePoll, /return pollTask\(generation\)\.catch\(\(error\) => \{/);
  assert.match(safePoll, /generation !== state\.pollGeneration \|\| !state\.taskId\) return/);
  assert.match(safePoll, /state\.taskStatusLoading = false;[\s\S]*?state\.taskStatusError = message;[\s\S]*?state\.taskStatusErrorRetrying = false/);
  assert.match(safePoll, /stopTaskPolling\(\);[\s\S]*?自动刷新已暂停：处理任务状态时发生意外错误[\s\S]*?点击“立即刷新状态”重试/);

  const pollStart = app.indexOf("async function pollTask(");
  const poller = pollStart >= 0 && safeStart > pollStart ? app.slice(pollStart, safeStart) : "";
  assert.doesNotMatch(poller, /setTimeout\(\(\) => pollTask\(generation\)/);
  assert.match(poller, /setTimeout\(\(\) => pollTaskSafely\(generation\)/);
  assert.ok(app.includes("void pollTaskSafely(state.pollGeneration);"), "恢复任务使用异常收敛入口");
  assert.ok(app.includes("await pollTaskSafely(generation);"), "工具栏刷新使用异常收敛入口");
  assert.ok(app.includes("void pollTaskSafely(generation).finally("), "手动刷新使用异常收敛入口并仍收尾忙碌状态");
});


test("轮询异常收敛中的诊断渲染失败不会再次拒绝 Promise", () => {
  const safeStart = app.indexOf("function pollTaskSafely(");
  const safeEnd = app.indexOf("\nasync function createTask(", safeStart);
  const safePoll = safeStart >= 0 && safeEnd > safeStart ? app.slice(safeStart, safeEnd) : "";
  assert.ok(safePoll.includes("return pollTask(generation).catch((error) => {"));
  assert.ok(safePoll.includes("generation !== state.pollGeneration || !state.taskId"));
  assert.ok(safePoll.includes('appendRecoveryError("状态徽标更新失败：", renderError);'));
  assert.ok(safePoll.includes('appendRecoveryError("轮询诊断更新失败：", renderError);'));
  assert.ok(safePoll.includes('appendRecoveryError("自选股卡片刷新失败：", renderError);'));
  assert.ok(safePoll.includes("异常收敛中的异常也不得形成新的未处理 Promise rejection"));
  assert.ok(safePoll.includes("try {\n        stopTaskPolling();\n      } catch {}"));
});

test("手动刷新前置渲染异常仍会发起安全轮询并收尾按钮状态", () => {
  const start = app.indexOf('$("#refresh-task").addEventListener("click"');
  const end = app.indexOf('$("#recent-analysis-search")', start);
  const handler = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.match(handler, /try \{[\s\S]*?setQualityPill[\s\S]*?renderWatchlist\(\);[\s\S]*?catch \(setupError\)/);
  assert.match(handler, /catch \(setupError\)[\s\S]*?仍将继续读取服务端状态[\s\S]*?void pollTaskSafely\(generation\)\.finally\(/);
  assert.match(handler, /void pollTaskSafely\(generation\)\.finally\(\(\) => \{\s*if \(generation === state\.pollGeneration\) \{\s*try \{\s*setTaskRefreshBusy\(false\);\s*\} catch \{/);
  assert.match(handler, /finally 的按钮解锁不得再次形成未处理的 Promise rejection/);
});

test("Run 结果读取准备阶段异常也会复位忙碌锁", () => {
  const start = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const end = app.indexOf("\nfunction stopTaskPolling()", start);
  const loader = start >= 0 && end > start ? app.slice(start, end) : "";
  const outerTry = loader.indexOf("try {");
  const busy = loader.indexOf("setRunResultReloadBusy(true);");
  const setupRender = loader.indexOf('if (state.view === "watchlist") renderWatchlist();', busy);
  const whileStart = loader.indexOf("while (true)", busy);
  const outerCatch = loader.lastIndexOf("} catch (error) {");
  const outerFinally = loader.lastIndexOf("} finally {");
  assert.ok(outerTry >= 0 && busy > outerTry && busy < setupRender, "忙碌锁设置、初始化和自选股重绘必须位于外层 try 内");
  assert.ok(whileStart > setupRender && outerCatch > whileStart && outerFinally > outerCatch, "外层 catch/finally 必须包住完整读取生命周期");
  const errorFlow = loader.slice(outerCatch);
  assert.match(errorFlow, /generation !== state\.runResultGeneration[\s\S]*?state\.taskId !== expectedTaskId/);
  assert.match(errorFlow, /describeErrorSafely[\s\S]*?try \{\s*return errorMessage\(value\);\s*\} catch/);
  assert.match(errorFlow, /state\.runResultReadStatus = "error";[\s\S]*?读取 Run 结果时发生意外错误/);
  assert.match(errorFlow, /appendReadDiagnostic\("错误提示更新失败：[\s\S]*?appendReadDiagnostic\("自选股卡片刷新失败：/);
  assert.match(errorFlow, /finally \{[\s\S]*?if \(generation === state\.runResultGeneration\) \{\s*try \{\s*setRunResultReloadBusy\(false\);\s*\} catch \(unlockError\)/);
  assert.match(errorFlow, /最终收尾本身必须保持不抛错/);
});

test("Run 结果的异常诊断与按钮解锁失败不会产生二次拒绝", () => {
  const start = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const end = app.indexOf("\nfunction stopTaskPolling()", start);
  const loader = start >= 0 && end > start ? app.slice(start, end) : "";
  const tail = loader.slice(loader.lastIndexOf("} finally {"));
  assert.match(tail, /generation === state\.runResultGeneration[\s\S]*?try \{\s*setRunResultReloadBusy\(false\);\s*\} catch \(unlockError\)/);
  assert.match(tail, /重新启用结果读取按钮失败：[\s\S]*?catch \{\s*\/\/ 最终收尾本身必须保持不抛错/);
  assert.match(loader, /if \(generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\|[\s\S]*?state\.settings\.analysisBase !== analysisBase\) return;/);
});

test("task polling errors mark last-known status stale and successful refresh clears it", () => {
  assert.ok(app.includes('taskStatusError: ""'));
  assert.ok(app.includes('taskStatusErrorRetrying: false'));
  const pollStart = app.indexOf("async function pollTask(");
  const pollEnd = app.indexOf("\nasync function createTask(", pollStart);
  const poller = pollStart >= 0 && pollEnd > pollStart ? app.slice(pollStart, pollEnd) : "";
  assert.ok(poller.includes("state.taskStatusError = errorMessage(taskError);"), "任务状态读取失败时保存提示");
  assert.ok(poller.includes("state.taskStatusErrorRetrying = isRetryableApiError(taskError) && !taskError.responseContractError;"), "区分自动重试与永久暂停");
  assert.ok(poller.includes('state.taskStatusError = "";'), "下一次有效 Task 回包清除旧错误");
  assert.ok(poller.includes('setQualityPill($("#task-status-pill"), "unknown", state.taskStatusErrorRetrying ? "状态刷新失败" : "状态读取失败")'), "任务状态徽标不得继续冒充为已确认的 running/succeeded");
  const watchlistRenderer = app.slice(app.indexOf("function renderWatchlist("), app.indexOf("\nfunction addCurrentSymbolToWatchlist("));
  assert.ok(watchlistTrackingViewModel.includes("状态读取异常（上次状态："), "自选卡片应将上次状态标为未验证");
  assert.ok(watchlistTrackingViewModel.includes("自动重试中") && watchlistTrackingViewModel.includes("自动刷新已暂停"), "自选卡片应区分临时错误与永久错误");
  const manualStart = app.indexOf('$("#refresh-task").addEventListener("click"');
  const manualEnd = app.indexOf('$("#recent-analysis-search")', manualStart);
  const manualRefresh = app.slice(manualStart, manualEnd);
  assert.ok(manualRefresh.includes("state.taskEventsUnavailable = false") && manualRefresh.includes("pollTaskSafely(generation)"), "手动刷新重新尝试任务与事件读取");
  assert.ok(manualRefresh.includes("正在手动刷新任务状态与阶段事件"), "手动刷新开始后应立即反馈，避免旧错误提示看起来未响应");
});

test("创建任务的前置 UI 异常也会释放提交锁和按钮", () => {
  const start = app.indexOf("async function createTask(event) {");
  const end = app.indexOf("\nfunction startAnalysisForSymbol(", start);
  const creator = start >= 0 && end > start ? app.slice(start, end) : "";
  const declarations = creator.indexOf("let button = null;");
  const protectedTry = creator.indexOf("try {", declarations);
  const buttonSetup = creator.indexOf('button = $("#create-task-button");', protectedTry);
  const networkCall = creator.indexOf("await createAnalysisTask(", protectedTry);
  const outerCatch = creator.lastIndexOf("} catch (error) {");
  const outerFinally = creator.lastIndexOf("} finally {");
  assert.ok(declarations >= 0 && protectedTry > declarations && protectedTry < buttonSetup, "按钮查询与初始化必须位于统一 try 内");
  assert.ok(protectedTry < networkCall && outerCatch > networkCall && outerFinally > outerCatch, "准备阶段和网络请求必须共享统一 catch/finally");
  assert.match(creator.slice(outerCatch), /catch \(recoveryError\)[\s\S]*?无法更新原始错误提示[\s\S]*?finally/);
  assert.match(creator.slice(outerFinally), /state\.isCreatingTask = false;[\s\S]*?try \{[\s\S]*?button\.disabled = false;[\s\S]*?catch \(unlockError\)/);
  assert.match(creator.slice(outerFinally), /finally 中的反馈或 DOM 异常不得形成未处理的 Promise rejection/);
  assert.match(creator, /creationContextGeneration = state\.taskContextGeneration/);
  assert.match(creator, /creationHistoryGeneration = state\.taskHistoryGeneration/);
});

test("new task creation responses cannot poison requested symbols or run/status identity", () => {
  const createStart = app.indexOf("async function createTask(event) {");
  const createEnd = app.indexOf("\nfunction startAnalysisForSymbol(", createStart);
  const creator = createStart >= 0 && createEnd > createStart ? app.slice(createStart, createEnd) : "";
  assert.match(creator, /const responseSymbolsMatch = task\?\.symbols === undefined \|\|[\s\S]*?sameTaskSymbolSet\(symbols, task\.symbols\)/);
  assert.match(creator, /const responseRunId = responseSymbolsMatch && responseRunIdValid \? task\.run_id : ""/);
  assert.match(creator, /const responseStatus = responseSymbolsMatch && responseStatusValid \? task\.status : ""/);
  assert.match(creator, /const normalizedTask = \{[\s\S]*?task_id: task\.task_id,[\s\S]*?run_id: responseRunId,[\s\S]*?status: responseStatus,[\s\S]*?task_type: taskType,[\s\S]*?symbols,/);
  assert.match(creator, /state\.pollTimer = window\.setTimeout\(\(\) => pollTaskSafely\(generation\), 500\);/);
  assert.doesNotMatch(creator, /void loadTaskRunResult\(responseRunId, task\.task_id\)/, "终态创建/幂等回包也必须先确认阶段事件");
  assert.match(creator, /current_stage: responseSymbolsMatch \? task\.current_stage : "等待服务端确认"/);
});

test("late task creation responses cannot replace the task selected while submission is pending", () => {
  assert.ok(app.includes("taskContextGeneration: 0"), "task selection context has its own generation");
  const createStart = app.indexOf("async function createTask(event) {");
  const createEnd = app.indexOf("\nfunction startAnalysisForSymbol(", createStart);
  const creator = createStart >= 0 && createEnd > createStart ? app.slice(createStart, createEnd) : "";
  assert.match(creator, /state\.taskContextGeneration \+= 1;\s*creationContextGeneration = state\.taskContextGeneration;/);
  assert.match(creator, /if \(creationContextGeneration !== state\.taskContextGeneration\) \{[\s\S]*?persistRecentTaskHistory\(endpointRecord\)[\s\S]*?当前选中的任务不会被覆盖/);
  assert.match(creator, /catch \(error\) \{\s*try \{\s*if \(creationContextGeneration !== state\.taskContextGeneration\)/, "late create errors must not hide the currently selected task");
  const activateStart = app.indexOf("function activateTaskReference(record) {");
  const activateEnd = app.indexOf("\nfunction restoreLastTask(", activateStart);
  assert.match(app.slice(activateStart, activateEnd), /state\.taskContextGeneration \+= 1;/);
  const invalidateStart = app.indexOf("function invalidateActiveTaskForAnalysisBaseChange(");
  const invalidateEnd = app.indexOf("\nfunction applySettings(", invalidateStart);
  assert.match(app.slice(invalidateStart, invalidateEnd), /state\.taskContextGeneration \+= 1;/);
});

test("polling pauses on permanent API errors and Run reads use bounded, stale-safe retries", () => {
  assert.match(app, /if \(taskError && \(!isRetryableApiError\(taskError\) \|\| taskError\.responseContractError\)\) \{[\s\S]*?自动刷新已暂停[\s\S]*?return;[\s\S]*?state\.pollTimer = window\.setTimeout\(\(\) => pollTaskSafely\(generation\), 5000\);/);
  assert.match(app, /const RUN_RESULT_RETRY_DELAYS_MS = \[1000, 2500, 5000\]/);
  assert.match(app, /generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\|\s*endpointGeneration !== state\.analysisConnectionGeneration/);
  assert.match(app, /state\.runResultGeneration \+= 1/);
  assert.match(app, /isRetryableApiError\(error\)/);
});


test("分析结果与诊断文本字号由组件基础规则负责", () => {
  const expected = [
    [".task-result-note", "12px"],
    [".task-result-metric small", "13px"],
    [".signal-result-card strong", "12px"],
    [".signal-result-card p", "12px"],
    [".task-result-warnings p", "13px"],
    [".raw-result", "12px"],
    [".raw-result pre", "12px"],
    [".analysis-result-block > p", "13px"],
    [".analysis-result-badge", "12px"],
    [".analysis-result-fields dt", "12px"],
    [".analysis-result-fields dd", "13px"],
    [".analysis-result-empty", "13px"]
  ];

  for (const [selector, size] of expected) {
    const declarations = exactCssDeclarationBlock(selector);
    assert.ok(declarations.includes("font-size: " + size + ";"), selector + " should own its readability baseline");
    assert.doesNotMatch(declarations, /!important/);
  }

  const actionDeclarations = exactCssDeclarationBlock(".task-result-actions .text-button");
  assert.ok(actionDeclarations.includes("font-size: 11px;"), "task result actions retain compact typography");
  assert.doesNotMatch(actionDeclarations, /!important/);
  assert.ok(!stylesheet.includes(':root[data-theme="light"] .task-result-actions .text-button'));
  assert.ok(!stylesheet.includes(':root[data-theme="light"] .task-result-metric small'));
});

test("最近分析与自选股文字字号由组件基础规则负责", () => {
  const expected = [
    [".recent-analysis-item-main strong", "12px"],
    [".recent-analysis-item-main small", "12px"],
    [".recent-analysis-item-status", "12px"],
    [".watchlist-panel .panel-heading p", "13px"],
    [".watchlist-name-field", "13px"],
    [".watchlist-item-meta", "12px"],
    [".watchlist-action", "13px"],
    [".watchlist-feedback", "13px"]
  ];

  for (const [selector, size] of expected) {
    const declarations = exactCssDeclarationBlock(selector);
    assert.ok(declarations.includes("font-size: " + size + ";"), selector + " should own its readability baseline");
    assert.doesNotMatch(declarations, /!important/);
  }

  assert.ok(!stylesheet.includes(".recent-analysis-item-main strong { font-size: 12px !important; }"));
  assert.ok(!stylesheet.includes(".watchlist-panel .panel-heading p { font-size: 13px !important; }"));
});

test("个股补充资料文字字号由组件基础规则负责", () => {
  const expected = [
    [".supplemental-heading p", "13px"],
    [".supplemental-heading .button", "13px"],
    [".supplemental-context", "12px"],
    [".supplemental-feedback", "13px"],
    [".supplemental-card-heading small", "12px"],
    [".supplement-quality", "12px"],
    [".supplemental-fields dt", "13px"],
    [".supplemental-fields dd", "13px"],
    [".supplemental-reasons", "12px"],
    [".supplemental-error", "13px"]
  ];

  for (const [selector, size] of expected) {
    const declarations = exactCssDeclarationBlock(selector);
    assert.ok(declarations.includes("font-size: " + size + ";"), selector + " should own its readability baseline");
    assert.doesNotMatch(declarations, /!important/);
  }

  assert.match(stylesheet, /@media \(max-width: 800px\) \{ \.supplemental-grid \{ grid-template-columns: minmax\(0, 1fr\); \} \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\)[\s\S]*?\.supplemental-fields \{ grid-template-columns:/);
});

test("structured analysis results use readable typography and mobile-safe layouts", () => {
  assert.match(stylesheet, /\.task-result-metric small \{[^}]*font-size: 13px/);
  assert.match(stylesheet, /\.task-result-metric strong \{[^}]*font-size: 14px/);
  assert.match(stylesheet, /\.analysis-result-block > h3 \{[^}]*font-size: 14px/);
  assert.match(stylesheet, /\.analysis-result-fields dt \{[^}]*font-size: 12px/);
  assert.match(stylesheet, /\.analysis-result-fields dd \{[^}]*font-size: 13px/);
  assert.match(stylesheet, /\.task-result-actions \.text-button \{[^}]*font-size: 11px/);
  assert.match(stylesheet, /@media \(max-width: 600px\)[\s\S]*?\.analysis-result-grid \{ grid-template-columns: minmax\(0, 1fr\); \}/);
  assert.match(stylesheet, /overflow-wrap: anywhere/);
});


test("watchlist list projection is pure and reuses the existing sort domain", () => {
  assert.match(appSource, /buildWatchlistListViewModel\(\{/);
  assert.match(watchlistListViewModel, /import \{ sortWatchlistItems \} from "\.\/watchlist-sort\.js"/);
  assert.match(watchlistListViewModel, /export function buildWatchlistListViewModel\(/);
  assert.doesNotMatch(watchlistListViewModel, /\bstate\.|localStorage|document\.|querySelector|addEventListener|fetch\(/);
  assert.match(watchlistListViewModel, /groupAssignments|assignments/);
  assert.match(watchlistListViewModel, /filterCountText/);
  assert.match(appSource, /syncWatchlistQuoteMembership\(records\)/);
  assert.match(appSource, /renderResearchStockList\(\)/);
});

test("browser-local watchlist is wired to existing query/analysis flows and stores only references", () => {
  for (const id of ["add-to-watchlist-button", "watchlist-count", "watchlist-feedback", "watchlist-list"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "watchlist control must exist: " + id);
  }
  assert.match(app, /function renderWatchlist\(/);
  assert.match(app, /addWatchlistSymbol\(symbol, defaultName, localStorage\)/);
  assert.match(app, /removeWatchlistSymbol\(symbol, localStorage\)/);
  assert.match(appSource, /function handleWatchlistNameChange\(aliasInput\)/);
  assert.match(appSource, /renameWatchlistSymbol\(symbol, aliasInput\.value, localStorage\)/);
  assert.match(appSource, /input\[data-watchlist-name-symbol\]/);
  assert.match(appSource, /buildWatchlistItemNode\(/);
  assert.match(watchlistItemView, /export function buildWatchlistItemNode\(/);
  assert.doesNotMatch(watchlistItemView, /state[.]|localStorage|fetch[(]|addEventListener|querySelector/);
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
  const researchView = html.match(/<section class="view" id="research-view"[\s\S]*?(?=<section class="view")/)?.[0] || "";
  for (const id of ["daily-chart-panel", "technical-indicators-panel", "historical-bars-panel", "research-detail-profile", "research-detail-fundamentals", "research-detail-valuations", "research-detail-capital-flow", "research-detail-chip", "research-detail-announcements"]) {
    assert.ok(researchView.includes('id="' + id + '"'), "unified research workspace must contain " + id);
  }
  assert.match(researchView, /data-research-detail-target="daily-chart-panel"/);
  assert.match(researchView, /data-research-detail-target="technical-indicators-panel"/);
  assert.match(app, /function openResearchChartView\(\)[\s\S]*?setView\("research"\)[\s\S]*?daily-chart-panel/);
  assert.match(stockResearch, /runSupplementRequest\("#load-fundamentals"/);
  assert.match(stockResearch, /接口请求成功，但没有返回可展示的筹码指标/);
  assert.match(stockResearch, /接口请求成功，但没有返回可展示的资金流指标/);

  assert.ok(app.includes("function renderResearchOverview()"));
  const researchOverviewRenderer = app.match(/function renderResearchOverview\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(researchOverviewRenderer, /sameMarketSymbol\(barsSymbol,\s*symbol\)/, "研究摘要 MA 必须复用 A 股证券代码比较规则");
  assert.doesNotMatch(researchOverviewRenderer, /barsSymbol\s*===\s*symbol/, "不得用原始代码字符串判断均线是否属于当前标的");
  assert.ok(app.includes('function openResearchStock(symbol, name = "")'));
  assert.ok(app.includes("await stockResearch.refresh()"));
  assert.ok(!app.includes("loadSupplementalData") && !app.includes("resetSupplementPanel"));
  assert.ok(apiClient.includes("/chip-distribution") && apiClient.includes("/capital-flow"));
  assert.ok(stockResearch.includes("getChipDistribution") && stockResearch.includes("getCapitalFlow"));
  assert.ok(stockResearch.includes("async function refresh()"));
  assert.ok(stylesheet.includes(".research-overview-grid") && stylesheet.includes(".supplement-quality-degraded"));
});
test("restoring a task resets the previous stage-event selection", () => {
  const restoreStart = app.indexOf("function activateTaskReference(record) {");
  const restoreEnd = app.indexOf("\nfunction ", restoreStart + 1);
  const restoreTask = restoreStart >= 0 && restoreEnd > restoreStart ? app.slice(restoreStart, restoreEnd) : "";
  assert.ok(restoreTask.includes("state.taskEvents.clear();"), "恢复任务前清空上一任务事件");
  assert.ok(restoreTask.includes("state.selectedTaskEventSequence = null;"), "恢复任务时清空旧事件选中序号");
  assert.ok(restoreTask.indexOf("state.selectedTaskEventSequence = null;") < restoreTask.indexOf("renderTaskEvents();"), "先重置事件选择，再渲染新任务事件");
});

test("watchlist labels task history from a different analysis API address", () => {
  assert.ok(watchlistTrackingViewModel.includes("function isOtherAnalysisBase(task, currentAnalysisBase)"));
  assert.ok(watchlistTrackingViewModel.includes('todayText = "今日分析：有本地记录" + (todayTaskUsesOtherEndpoint ? "（旧分析 API 地址）" : "")'));
  assert.ok(watchlistTrackingViewModel.includes('(recentTaskUsesOtherEndpoint ? " · 旧分析 API 地址" : "")'));
  const todayTaskDeclaration = watchlistTrackingViewModel.indexOf("const todayTask = [...matchingTasks]");
  const todayEndpointUse = watchlistTrackingViewModel.indexOf("const todayTaskUsesOtherEndpoint = isOtherAnalysisBase(todayTask");
  assert.ok(todayTaskDeclaration >= 0 && todayTaskDeclaration < todayEndpointUse, "必须先计算 todayTask 再读取其来源地址，避免暂时性死区");
});

test("watchlist task and Run summaries refresh when background state changes", () => {
  const taskRenderer = app.match(/function renderTask\(task\) \{[\s\S]*?\n\}/)?.[0] || "";
  const runRenderer = app.match(/function renderTaskResult\(result\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(taskRenderer, /if \(state\.view === "watchlist"\) renderWatchlist\(\);/, "Task 状态更新时刷新可见自选卡片");
  assert.match(runRenderer, /if \(state\.view === "watchlist"\) renderWatchlist\(\);/, "Run 结果更新时刷新可见自选卡片");
});

test("市场总览与活跃度请求的准备阶段和 finally 异常会收敛", () => {
  assert.match(marketOverview, /function safeOverviewUi\(operation\)/);
  assert.match(marketOverview, /function cancel\(key\)\{const controller=controllers\[key\];delete controllers\[key\];generation\[key\]\+\+;try\{controller\?\.abort\(\);\}catch\{\}\}/);
  assert.match(marketOverview, /async function load\(key\)\{[\s\S]*?try\{\s*cancel\(key\);seq=generation\[key\];settings=typeof getSettings/);
  assert.match(marketOverview, /async function refresh\(keys=KEYS,statusSelector="#market-overview-updated"\)\{[\s\S]*?try\{[\s\S]*?await Promise\.all\(requested\.map\(load\)\)[\s\S]*?catch\(error\)/);
  assert.match(marketOverview, /Object\.keys\(refreshGenerations\)\.forEach\(selector=>\{refreshGenerations\[selector\]\+=1;\}\);[\s\S]*?KEYS\.forEach/);
  assert.match(marketActivity, /function safeActivityUi\(operation\)/);
  assert.match(marketActivity, /async function loadDragon\(\{append=false\}=\{\}\)\{[\s\S]*?try\{\s*abort\("dragon"\);seq=generation\.dragon;cfg=settings\(\)/);
  assert.match(marketActivity, /finally\{[\s\S]*?if\(seq!==null&&generation\.dragon===seq\)[\s\S]*?safeActivityUi\(\(\)=>\{const next=\$\("#dragon-tiger-next"\)/);
  assert.match(marketActivity, /async function loadCalendar\(\)\{[\s\S]*?try\{\s*abort\("calendar"\);seq=generation\.calendar;cfg=settings\(\)/);
  assert.match(marketActivity, /finally\{[\s\S]*?if\(seq!==null&&generation\.calendar===seq\)safeActivityUi/);
});

test("market overview refresh summaries are isolated by status region and guarded against stale runs", () => {
  assert.match(marketOverview, /refreshGenerations=Object\.create\(null\)/);
  assert.match(marketOverview, /const currentGeneration=\(refreshGenerations\[statusSelector\]\|\|0\)\+1/);
  assert.match(marketOverview, /refreshGenerations\[statusSelector\]=currentGeneration/);
  assert.match(marketOverview, /if\(currentGeneration!==refreshGenerations\[statusSelector\]\)return/);
});

test("market connection changes invalidate all market overview requests and old summaries", () => {
  const start = marketOverview.indexOf("function invalidateConnection(){");
  const end = marketOverview.indexOf("\n }", start);
  const body = start >= 0 && end > start ? marketOverview.slice(start, end) : "";
  assert.ok(body.includes("KEYS.forEach(key=>{"));
  assert.ok(body.includes("safeOverviewUi(()=>cancel(key));"), "连接变更应通过安全入口中止各市场模块请求并推进 generation");
  assert.ok(body.includes('safeOverviewUi(()=>pending(key,"行情连接配置已变更，旧结果已清除，请重新刷新。","warning"));'));
  assert.ok(body.includes("refreshGenerations[selector]+=1;"), "作废旧刷新摘要回写");
  assert.ok(body.indexOf("Object.keys(refreshGenerations)") < body.indexOf("KEYS.forEach(key=>{"), "必须先使所有旧汇总响应失效，再逐项清理模块");
  assert.ok(body.includes('"#market-overview-updated","#market-data-updated"'));
  assert.ok(marketOverview.includes("return {refresh,invalidateConnection};"));
  const settingsStart = app.indexOf("function applySettings(nextSettings,");
  const settingsEnd = app.indexOf("\nfunction saveSettingsFromForm", settingsStart);
  const settingsApplication = settingsStart >= 0 && settingsEnd > settingsStart ? app.slice(settingsStart, settingsEnd) : "";
  assert.ok(settingsApplication.includes("marketOverview.invalidateConnection()"), "连接变更通过安全步骤失效首页请求");
});

test("market connection changes invalidate in-flight dragon-tiger and calendar requests", () => {
  const invalidateStart = marketActivity.indexOf("function invalidateConnection() {");
  const invalidateEnd = marketActivity.indexOf("\n }", invalidateStart);
  const invalidateBody = invalidateStart >= 0 && invalidateEnd > invalidateStart
    ? marketActivity.slice(invalidateStart, invalidateEnd)
    : "";
  assert.ok(invalidateBody.includes('safeActivityUi(()=>abort("dragon"))'), "连接配置变更应通过安全入口中止龙虎榜请求");
  assert.ok(invalidateBody.includes('safeActivityUi(()=>abort("calendar"))'), "连接配置变更应通过安全入口中止交易日历请求");
  assert.ok(invalidateBody.includes('$("#dragon-tiger-body")'), "应清除旧龙虎榜结果");
  assert.ok(invalidateBody.includes('$("#trading-calendar-grid").replaceChildren'), "应清除旧交易日历结果");
  assert.ok(marketActivity.includes("return {refresh:()=>Promise.all([loadDragon(),loadCalendar()]),invalidateConnection};"));
  const settingsStart = app.indexOf("function applySettings(nextSettings,");
  const settingsEnd = app.indexOf("\nfunction saveSettingsFromForm", settingsStart);
  const settingsApplication = settingsStart >= 0 && settingsEnd > settingsStart
    ? app.slice(settingsStart, settingsEnd)
    : "";
  assert.ok(settingsApplication.includes("marketActivity.invalidateConnection()"), "连接变更通过安全步骤失效龙虎榜和交易日历请求");
});

test("an older stock-research load cannot unlock the current stock's loading button", () => {
  assert.match(stockResearch, /loadResearchGeneration:0/);
  assert.match(stockResearch, /function reset\(symbol\)\{[\s\S]*?state\.loadResearchGeneration\+=1;[\s\S]*?\$\("#load-stock-research"\)/);
  assert.match(stockResearch, /async function loadResearch\(\)\{[\s\S]*?requestGeneration=\+\+state\.loadResearchGeneration;[\s\S]*?await Promise\.allSettled\([\s\S]*?requestGeneration!==state\.loadResearchGeneration\|\|symbol!==state\.symbol[\s\S]*?finally\{[\s\S]*?requestGeneration===state\.loadResearchGeneration&&symbol===state\.symbol/);
});

test("stale stock research detail requests cannot unlock buttons for a different symbol", () => {
  assert.match(stockResearch, /function reset\(symbol\)[\s\S]*?\["#load-valuations","读取估值指标"\][\s\S]*?\["#load-chip-distribution","读取筹码分布"\][\s\S]*?\["#load-capital-flow","读取资金流"\]/);
  assert.match(stockResearch, /async function runSupplementRequest\(buttonId,idleText,key,request,render,successMessage\)[\s\S]*?const operation=fetchOne\(key,request,render\)[\s\S]*?generation=state\.generation\[key\][\s\S]*?generation===state\.generation\[key\]&&symbol===state\.symbol/);
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
  assert.match(stockResearch, /if\(replacingInFlight&&append\)return false;[\s\S]*?if\(replacingInFlight\)\{abort\("announcements"\);state\.announcementLoading=false;\}\s*else abort\("announcements"\);\s*seq=state\.generation\.announcements/);
});

test("changing market API credentials invalidates old stock-search suggestions", () => {
  assert.match(app, /function applySettings\(nextSettings, \{ persistPreviousActiveTask = true \} = \{\}\) \{[\s\S]*?previousSettings\.apiKey !== nextSettings\.apiKey \|\|\s*previousSettings\.marketBase !== nextSettings\.marketBase[\s\S]*?runSettingsApplyStep\("取消股票搜索请求失败：", \(\) => cancelStockSearchRequest\(\), failures\);[\s\S]*?state\.stockSearchResults = \[\];[\s\S]*?runSettingsApplyStep\("清理股票搜索结果失败：", \(\) => \$\("#stock-search-results"\)\.replaceChildren\(\), failures\);[\s\S]*?runSettingsApplyStep\("关闭股票搜索面板失败：", \(\) => hideStockSearch\(\), failures\);/);
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
  assert.match(app, /if \(analysisBaseChanged\) \{[\s\S]*?invalidateActiveTaskForAnalysisBaseChange\(/);
  assert.match(app, /endpointGeneration !== state\.analysisConnectionGeneration/);
  assert.match(app, /const taskAnalysisBase = state\.taskAnalysisBase \|\| state\.settings\.analysisBase;[\s\S]*?analysis_base: taskAnalysisBase/);
  assert.match(app, /旧任务不再自动轮询/);
  const baseInvalidation = app.match(/function invalidateActiveTaskForAnalysisBaseChange\([\s\S]*?\n\}/)?.[0] || "";
  assert.match(baseInvalidation, /safelyUpdateTaskContextUi\("重绘自选股卡片失败：", \(\) => renderWatchlist\(\)\)/, "切换分析地址后应在安全清理步骤中重绘自选卡片");
  assert.match(app, /if \(requestedBase !== state\.settings\.analysisBase\)/);
  assert.match(recentTaskModule, /analysis_base: typeof record\.analysis_base === "string"/);
});

test("topbar service health checks have bounded timeouts and pass abort signals", () => {
  assert.ok(healthCheck.includes("export async function runHealthCheck(check, settings, timeoutMs = 6000, externalSignal = null)"));
  assert.match(healthCheck, /externalSignal\.addEventListener\("abort", abortListener, \{ once: true \}\)/);
  assert.ok(healthCheck.includes("controller.abort()"));
  assert.match(app, /async function refreshServices\(\)[\s\S]*?Promise\.all\(\[[\s\S]*?runHealthCheck\(marketHealth, settings\)[\s\S]*?runHealthCheck\(analysisHealth, settings\)/);
});

test("data-center checks are invalidated when a saved API connection changes", () => {
  assert.match(dataCenter, /let generation = 0;[\s\S]*?let refreshController = null;/);
  assert.match(dataCenter, /const controller = new AbortController\(\);[\s\S]*?refreshController = controller;/);
  assert.match(dataCenter, /runHealthCheck\(marketHealth, settings, 6000, controller\.signal\)/);
  assert.match(dataCenter, /refreshController\?\.abort\(\);[\s\S]*?refreshController = null;/);
  assert.match(dataCenter, /if \(current !== generation\) return;/);
  assert.match(dataCenter, /function invalidate\(message = "连接配置已变更，之前的检查结果已作废。请重新检查。"\) \{[\s\S]*?generation \+= 1;[\s\S]*?checking = false;[\s\S]*?setBadge\("#data-center-summary-status", "unknown", "需重新检查"\)/);
  assert.match(dataCenter, /finally \{[\s\S]*?if \(current === generation\) \{[\s\S]*?checking = false;/);
  assert.match(dataCenter, /return \{ refresh, invalidate \};/);
  assert.match(app, /const marketConnectionChanged = previousSettings\.apiKey !== nextSettings\.apiKey \|\|\s*previousSettings\.marketBase !== nextSettings\.marketBase;[\s\S]*?if \(analysisBaseChanged \|\| marketConnectionChanged\) \{[\s\S]*?runSettingsApplyStep\("失效数据中心检查结果失败：", \(\) =>\s*dataCenter\.invalidate\(/);
});

test("connection-check results are isolated between saved settings and unsaved draft", () => {
  const saved = app.match(/async function checkConnectionFromSettings\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  const draft = app.match(/async function checkDraftConnectionFromSettings\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(saved, /const generation = \+\+savedConnectionCheckGeneration/);
  assert.match(saved, /currentSettings\.marketBase !== settingsToCheck\.marketBase/);
  assert.match(draft, /const generation = \+\+draftConnectionCheckGeneration/);
  assert.match(draft, /connectionDraftChanged\(testedDraft, currentDraft\)/);
  assert.match(draft, /表单地址检查结果（尚未保存）/);
  assert.doesNotMatch(draft, /saveSettings\(/);
});
test("market connection changes invalidate all in-flight stock-research panels", () => {
  assert.match(app, /const marketConnectionChanged = previousSettings\.apiKey !== nextSettings\.apiKey \|\|\s*previousSettings\.marketBase !== nextSettings\.marketBase;[\s\S]*?if \(marketConnectionChanged\) \{[\s\S]*?runSettingsApplyStep\("取消股票搜索请求失败：", \(\) => cancelStockSearchRequest\(\), failures\);[\s\S]*?runSettingsApplyStep\("失效个股研究请求失败：", \(\) =>\s*stockResearch\.invalidate\("行情连接配置已变更，旧研究请求已作废，请重新加载对应资料。"\), failures\);/);
  assert.match(stockResearch, /function invalidate\(message="行情连接配置已变更，请重新加载个股研究资料。"\)\{\s*reset\(resolvedSymbol\(\)\);[\s\S]*?if\(!settings\?\.apiKey\)\{markApiKeyRequired\(\);return;\}/);
  assert.match(stockResearch, /function reset\(symbol\)\{[\s\S]*?Object\.keys\(state\.generation\)\.forEach\(abort\)/);
});

test("service health and provider-list refreshes ignore superseded responses", () => {
  assert.match(app, /let serviceRefreshGeneration = 0;/);
  assert.match(app, /let sourceRefreshGeneration = 0;/);
  assert.match(app, /let sourceRefreshController = null;/);
  assert.ok(app.includes("function invalidateSourceRefresh("));
  assert.ok(app.includes("sourceRefreshGeneration += 1;"));
  assert.ok(app.includes("controller?.abort()"));
  assert.ok(app.includes("runSettingsApplyStep(\"失效数据源能力读取失败：\", () => invalidateSourceRefresh(), failures)"));
  assert.match(app, /async function refreshServices\(\) \{[\s\S]*?const generation = \+\+serviceRefreshGeneration;[\s\S]*?if \(generation !== serviceRefreshGeneration\) \{\s*return \{ marketOK, analysisOK, marketState, analysisState, stale: true \};\s*\}/);
  assert.match(app, /async function refreshSources\(\) \{[\s\S]*?const generation = \+\+sourceRefreshGeneration;[\s\S]*?if \(generation !== sourceRefreshGeneration\) return;/);
  assert.match(app, /if \(generation !== serviceRefreshGeneration\) \{\s*return \{ marketOK, analysisOK, marketState, analysisState, stale: true \};\s*\}[\s\S]*?setText\("#connection-label", connectionLabel\)/);
});

test("provider capability refresh clears old connection data and owns its loading state", () => {
  const refreshSources = app.match(/async function refreshSources\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(refreshSources, "refreshSources implementation must exist");
  assert.match(refreshSources, /showMessage\("正在读取当前连接配置对应的数据源能力…"\);[\s\S]*?setSourceTextSafely\("#source-count", "—"\);[\s\S]*?state\.sourceCount = 0/);
  assert.match(refreshSources, /button\.disabled = true;[\s\S]*?button\.setAttribute\("aria-busy", "true"\)/);
  assert.match(refreshSources, /正在读取当前连接配置对应的数据源能力/);
  assert.match(refreshSources, /showMessage\(message, "empty-row error-text"\)/);
  assert.ok(refreshSources.includes("if (generation === sourceRefreshGeneration && sourceRefreshController === controller) {"));
  assert.ok(refreshSources.includes("sourceRefreshController = null;"));
  assert.ok(refreshSources.includes('button.removeAttribute("aria-busy")'));
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
  assert.doesNotMatch(app, /\.innerHTML\s*=/, "dynamic views should build DOM nodes and use textContent");
  assert.match(app, /setupSettingsNavigation\(document\)/);
  assert.match(settingsNavigation, /settings-nav-search/);
  assert.match(settingsNavigation, /settings-section-toggle/);
  assert.doesNotMatch(app, /function setupSettingsNavigation\(/);
  assert.doesNotMatch(settingsNavigation, /\bstate\.|localStorage|fetch\(|addEventListener\("storage"/);
});
test("analysis task submission prevents concurrent duplicate requests and releases the guard", () => {
  assert.match(app, /isCreatingTask: false/);
  assert.match(app, /async function createTask\(event\)[\s\S]*?event\.preventDefault\(\);[\s\S]*?if \(state\.isCreatingTask\)[\s\S]*?分析任务正在提交[\s\S]*?return;[\s\S]*?state\.isCreatingTask = true;[\s\S]*?parseAnalysisSymbols/);
  assert.match(app, /catch \(error\) \{\s*state\.isCreatingTask = false;[\s\S]*?return;/);
  assert.match(app, /finally \{[\s\S]*?state\.isCreatingTask = false;[\s\S]*?button\.disabled = false;/);
  assert.match(apiClient, /headersKey \? \{ "Idempotency-Key": headersKey \} : \{\}/);
});

test("market API key help identifies who generates it and where it must be configured", () => {
  assert.match(html, /部署管理员自行生成并配置/);
  assert.match(html, /MARKET_DATA_API_KEY/);
  assert.match(html, /openssl rand -hex 32/);
  assert.match(html, /X-API-Key/);
  assert.match(html, /不是第三方 Provider 密钥/);
});
test("market API key visibility is opt-in and resets when leaving settings", () => {
  assert.match(html, /id="market-key-input" type="password"/);
  assert.match(html, /id="toggle-market-key-visibility"[^>]*aria-pressed="false"/);
  assert.match(app, /function openSettings\(\)[\s\S]*?setMarketKeyVisibility\(false\)/);
  assert.match(app, /previousView === "settings" && nextView !== "settings"\) setMarketKeyVisibility\(false\)/);
});
test("global typography has a readable baseline and key UI text is not undersized", () => {
  assert.match(html, /id="storage-boundary-title"/);
  assert.match(stylesheet, /\.settings-section-heading h2/);
  assert.match(stylesheet, /font-size:\s*12px/);
  assert.match(stylesheet, /line-height:\s*1\.[456]/);
});
test("watchlist bulk selection keeps select-all checked and indeterminate state synchronized", () => {
  assert.match(app, /selectVisible\.checked = inputs\.length > 0 && count === inputs\.length/);
  assert.match(app, /selectVisible\.indeterminate = count > 0 && count < inputs\.length/);
  assert.match(app, /renderWatchlist\(records, storageReadable, storageValid\)[\s\S]*?updateWatchlistBulkControls\(\)/);
});

test("recent analysis history exposes historical-only task types", () => {
  assert.match(html, /<option value="position_review">持仓复核（历史记录）<\/option>/);
  assert.ok(recentTaskView.includes("recentTaskTypeLabel(record.task_type)"));
  assert.match(recentTaskModule, /export function recentTaskTypeLabel\(/);
});

test("recent analysis history supports search, single removal and local-only clearing", () => {
  for (const id of ["recent-analysis-search", "clear-recent-analysis", "recent-analysis-feedback", "recent-analysis-list"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "recent-history control must exist: " + id);
  }
  assert.ok(app.includes("function renderRecentTasks(records)"));
  assert.ok(app.includes("renderRecentTaskList({"));
  assert.ok(recentTaskView.includes("export function renderRecentTaskList("));
  assert.ok(recentTaskView.includes("record.task_id"));
  assert.ok(recentTaskView.includes("taskStatusLabel(record.status)"));
  assert.match(app, /function removeRecentTaskReference\(taskId, analysisBase = ""\)[\s\S]*?removeRecentTask\(taskId, localStorage, undefined, analysisBase\)[\s\S]*?const persistedState = safeReadRecentTasksState\(\)[\s\S]*?targetStillExists[\s\S]*?服务端任务和报告不会被删除/);
  assert.match(app, /function removeRecentTaskReference[\s\S]*?无法确认最近任务引用已移除/);
  assert.match(app, /function clearRecentTaskHistory\(\)[\s\S]*?clearRecentTasks\(localStorage\)[\s\S]*?const clearedState = safeReadRecentTasksState\(\)[\s\S]*?未能确认最近任务历史已清空[\s\S]*?服务端记录未删除/);
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
  assert.match(app, /setAttribute\("aria-label", persistenceWarning \? "切换主题；当前主题未能保存" : "切换主题"\)/);
  assert.match(app, /const warning = persistenceWarning \? "；当前页面已切换，但未能确认主题偏好已保存" : ""/);
  assert.match(html, /id="theme-toggle-icon"/);
  assert.match(html, /id="theme-toggle-label"/);
  assert.match(app, /import \{ applyTheme, persistTheme, readTheme(?:, THEME_STORAGE_KEY)? \} from "\.\/theme\.js"/);
  assert.match(app, /\$\("#theme-toggle"\)\.addEventListener\("click"/);
  assert.match(stylesheet, /:root\[data-theme="light"\]\s*\{/);
  assert.match(stylesheet, /--bg:\s*#f3f6f9/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.settings-dialog/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.analysis-result-card/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.raw-result pre/);
  const rawResultPreBase = baseCssDeclarationBlock(".raw-result pre");
  assert.match(rawResultPreBase, /line-height:\s*1\.7\s*;/);
  assert.doesNotMatch(rawResultPreBase, /line-height:\s*1\.6\s*;/);
  assert.match(rawResultPreBase, /font-size:\s*12px\s*;/);
   assert.doesNotMatch(rawResultPreBase, /!important/);
  const rawResultPreLineHeightRules = [...stylesheet.matchAll(/\.raw-result pre\s*\{([^}]*)\}/g)]
    .filter((match) => /line-height:\s*1\.7\s*;/.test(match[1]));
  assert.equal(rawResultPreLineHeightRules.length, 1, "raw result pre line-height must be declared once in the base rule");
  assert.match(stylesheet, /:root\[data-theme="light"\] \.raw-result pre\s*\{[^}]*color:\s*#334b5a[^}]*background:\s*#f5f8fa[^}]*border-color:\s*#d6e1e8/);
  assert.match(module, /stock-provider-webui\.theme\.v1/);
  assert.doesNotMatch(module, /fetch\(|\/api\//, "theme preference must not call backend APIs");
});

test("mobile navigation drawer restores readable labels and can close via overlay or Escape", () => {
  const drawer = app.match(/function setMobileNavigationOpen\(open, \{ restoreFocus = false \} = \{\}\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(drawer, /aria-expanded/);
  assert.match(drawer, /isOpen \? "关闭导航" : "打开导航"/);
  assert.match(drawer, /if \(wasOpen && !isOpen && restoreFocus\) \$\("#mobile-menu-toggle"\)\.focus\(\)/);
  assert.match(app, /event\.key === "Escape"[\s\S]*?setMobileNavigationOpen\(false, \{ restoreFocus: true \}\)/);
  assert.match(app, /#mobile-nav-overlay[\s\S]*?setMobileNavigationOpen\(false, \{ restoreFocus: true \}\)/);
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
  assert.match(html, /通知 API 尚未接入/);
  assert.match(html, /Agent API 待接入/);
  assert.match(html, /选股 API 尚未接入/);
  assert.match(html, /后端待接入/);
  assert.match(html, /disabled aria-disabled="true"/);
  assert.match(html, /不会在浏览器里假建、删除或保存会话/);
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
  assert.match(app, /function openRunFlowDrawer\(\)[\s\S]*?dialog\.show\(\)/);
  assert.match(stylesheet, /\.run-flow-dialog\[open\]\s*\{[^}]*pointer-events:\s*none/);
  assert.match(stylesheet, /\.run-flow-dialog\[open\]\s*\.task-status-panel\s*\{[^}]*pointer-events:\s*auto/);
  assert.match(app, /function renderSelectedTaskEvent\(item\)[\s\S]*?renderSelectedTaskEventView\(/);
  assert.match(taskEventView, /export function renderSelectedTaskEventView\([\s\S]*?detailLayout\?\.classList\.remove\("has-selected-event"\)[\s\S]*?detailLayout\?\.classList\.add\("has-selected-event"\)/);
  assert.match(stylesheet, /\.run-flow-dialog #task-detail:not\(\[hidden\]\):not\(\.has-selected-event\)\s*\{\s*grid-template-columns:\s*minmax\(0,\s*1fr\);\s*\}/);
  assert.match(app, /function activateTaskReference\(record\)[\s\S]*?openRunFlowDrawer\(\)[\s\S]*?pollTaskSafely\(state\.pollGeneration\)/);
  assert.match(app, /function renderTask\(task\)[\s\S]*?task\.started_at[\s\S]*?task\.finished_at/);
  assert.match(app, /function renderRunStatus\(status = ""\)[\s\S]*?renderRunStatusView\(/);
  assert.match(taskEventView, /export function renderRunStatusView\([\s\S]*?runStatusLabel\(normalized\)/);
  assert.match(app, /function renderTaskResult\(result\)[\s\S]*?renderRunStatus\(result\.status \|\| ""\)/);
  assert.match(app, /close-run-flow-dialog/);
});

test("Run Flow 字号覆盖由组件级联负责，不依赖全局 important", () => {
  for (const [selector, size] of [
    [".event-heading h3", "13px"],
    [".event-heading span", "12px"],
    [".event-status", "12px"],
    [".event-body strong", "12px"],
    [".event-body p", "12px"],
    [".event-row > small", "12px"]
  ]) {
    const declarations = exactCssDeclarationBlock(selector);
    assert.ok(declarations.includes("font-size: " + size + ";"), selector + " should own its readable base font size");
    assert.doesNotMatch(declarations, /!important/);
  }

  for (const [selector, size] of [
    [".run-flow-dialog .event-heading h3", "14px"],
    [".run-flow-dialog .event-heading span", "12px"],
    [".run-flow-dialog .event-row > small", "11px"],
    [".run-flow-dialog .event-row-message", "12px"]
  ]) {
    const declarations = exactCssDeclarationBlock(selector);
    assert.ok(declarations.includes("font-size: " + size + ";"), selector + " should retain its component font size");
    assert.doesNotMatch(declarations, /!important/);
  }

  assert.match(stylesheet, /\.run-flow-dialog \.event-row > small \{ grid-column: 2; max-width: none; font-size: 11px; \}/);
  assert.doesNotMatch(stylesheet, /\.run-flow-dialog \.event-row > small \{[^}]*!important/);
});

test("Run result DOM adapter owns structured result rendering while app retains task state", () => {
  const rendererStart = app.indexOf("function renderTaskResult(result) {");
  const rendererEnd = app.indexOf("\nfunction normalizedTaskSymbolSet(", rendererStart);
  const renderer = rendererStart >= 0 && rendererEnd > rendererStart
    ? app.slice(rendererStart, rendererEnd)
    : "";
  assert.ok(renderer.includes("state.lastRunResult = result"));
  assert.ok(renderer.includes('state.runResultReadStatus = ""'));
  assert.ok(renderer.includes("persistTaskReference()"));
  assert.ok(renderer.includes('renderRunStatus(result.status || "")'));
  assert.ok(renderer.includes("renderTaskResultView({ document, result, runStatusLabel, fallbackRunId: state.runId })"));
  assert.ok(renderer.includes('if (state.view === "watchlist") renderWatchlist()'));
  assert.ok(taskResultView.includes("export function renderTaskResultView("));
  assert.ok(taskResultView.includes('document.querySelector("#task-result-structured")'));
  assert.doesNotMatch(taskResultView, /\bstate\b|localStorage|fetch\(/);
});

test("run-flow events expose only server-provided durations and diagnostics", () => {
  assert.match(taskEventFormat, /export function taskEventDurationLabel\(value\)/);
  assert.match(taskEventNode, /taskEventDurationLabel\(event\.duration_ms\)/);
  assert.match(taskEventView, /taskEventDurationLabel\(event\.duration_ms\)/);
  assert.match(taskEventNode, /event\.error_code/);
  assert.match(taskEventNode, /parts\.join\(" · "\)/);
  assert.match(apiClient, /after_sequence/);
  assert.match(taskEventListViewModel, /\.sort\(\(a, b\) => Number\(a\.sequence\) - Number\(b\.sequence\)\)/);
  assert.doesNotMatch(app, /completed_stages\s*\/\s*total_stages|progressPercent|fakeDAG/i);
});


test("run-flow keeps cached events explicit during read errors and exports diagnostic context", () => {
  assert.match(html, /id="task-events-read-status"[^>]*role="status"[^>]*aria-live="polite"/);
  assert.match(app, /const readStatus = \$\("#task-events-read-status"\)/);
  assert.match(app, /已保留当前可用事件/);
  assert.match(taskFlowSummaryViewModel, /taskEventsError[\s\S]*?读取失败/);
  assert.match(app, /task_error:\s*\$\("#task-error"\)\.textContent\.trim\(\) \|\| null/);
  assert.match(app, /task_poll_status:\s*\$\("#task-poll-label"\)\.textContent \|\| null/);
  assert.match(app, /event_read_status:\s*\{[\s\S]*?last_error:\s*state\.taskEventsError \|\| null[\s\S]*?known_event_count:\s*state\.taskEvents\.size/);
  assert.match(stylesheet, /\.task-events-read-status/);
});


test("market hot-rank actions accept the symbol or code field as the stock identifier", () => {
  assert.match(marketOverview, /const stock=el\("td"\),button=el\("button","market-hot-stock-link"\),symbol=String\(item\.symbol\|\|item\.code\|\|""\)\.trim\(\),name=String\(item\.name\|\|symbol\|\|"名称未报告"\)/);
  assert.match(marketOverview, /button\.dataset\.hotSymbol=symbol;button\.dataset\.hotName=name;button\.dataset\.hotAction="stocks";button\.disabled=!symbol/);
  assert.match(marketOverview, /research\.dataset\.hotSymbol=symbol;research\.dataset\.hotName=name;research\.dataset\.hotAction="research"/);
});

test("dragon-tiger rows only render stock navigation when a usable code exists", () => {
  assert.match(marketActivity, /const symbol=String\(item\.symbol\|\|item\.code\|\|""\)\.trim\(\);/);
  assert.match(marketActivity, /if\(symbol\)\{\s*const actions=el\("div","market-stock-entry-actions"\)/);
  assert.match(marketActivity, /button\.dataset\.dragonSymbol=symbol;button\.dataset\.dragonName=item\.name\|\|symbol;button\.dataset\.dragonAction=target/);
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
  assert.match(taskEventFormat, /export function taskEventStatusKey\(status\)/);
  assert.match(taskEventFormat, /export function taskEventStatusLabel\(status, statusKey = taskEventStatusKey\(status\)\)/);
  assert.match(taskEventFormat, /export function taskEventStatusTone\(statusKey\)/);
  assert.match(taskEventView, /status\.textContent = statusText/);
  assert.match(taskEventView, /\["事件状态", statusText\]/);
  assert.match(taskEventView, /const statusKey = taskEventStatusKey\(event\.status\)/);
  assert.match(stylesheet, /\.availability-pill\.is-error/);
  assert.match(stylesheet, /\.availability-pill\.is-warning/);
});


test("settings center groups configuration by ownership and prevents fake server writes", () => {
  const targets = [...html.matchAll(/data-settings-target="([^"]+)"/g)].map((match) => match[1]);
  assert.equal(targets.length, 10);
  for (const target of targets) assert.match(html, new RegExp('id="' + target + '"'));
  assert.match(html, /存在未保存的连接配置更改/);
  assert.match(html, /浏览器本地/);
  assert.match(html, /只读/);
  assert.match(html, /待接入/);
  assert.match(html, /当前浏览器/);
  assert.doesNotMatch(html, /provider-secret-input|save-provider-secret/);
  assert.match(app, /setMarketKeyVisibility\(false\)/);
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
  assert.match(app, /chatStockContextCleared: false/);
  const syncStart = app.indexOf("function syncChatStockContextFromMarket() {");
  const syncEnd = app.indexOf(String.fromCharCode(10) + "function ", syncStart);
  const contextSync = app.slice(syncStart, syncEnd);
  assert.match(contextSync, /if \(state\.chatStockContextCleared\) \{[\s\S]*?state\.chatStockContext = null;[\s\S]*?return;/);
  assert.match(contextSync, /state\.quote && typeof state\.quote\.symbol/);
  assert.match(app, /if \(nextView === "chat" && previousView !== "chat"\) syncChatStockContextFromMarket\(\)/);
  assert.match(app, /state\.chatStockContextCleared = true;\s*state\.chatStockContext = null;[\s\S]*?renderChatStockContext\(\)/);
  const quoteStart = app.indexOf("async function runQuoteQuery(event) {");
  const quoteEnd = app.indexOf(String.fromCharCode(10) + "function ", quoteStart);
  const quoteQuery = app.slice(quoteStart, quoteEnd);
  assert.match(quoteQuery, /const generation = \+\+state\.quoteQueryGeneration;[\s\S]*?try \{\s*state\.chatStockContextCleared = false;/);
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
    "screening-history-prev", "screening-history-next", "screening-apply-current-page",
    "screening-local-results-summary", "screening-local-results-body"
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
  assert.match(screeningView, /当前页本地筛选/);
  assert.match(screeningView, /id="screening-ma-capability-hint"/);
  for (const period of ["ma60", "ma120", "ma250"]) {
    assert.match(screeningView, new RegExp(`<option value="${period}" disabled>.*待数据侧支持`), period + " must remain disabled until the market page API provides it");
  }
  assert.match(screeningView, /不会为每只股票额外请求日 K/);
  for (const id of ["screening-condition-logic", "screening-condition-field", "screening-condition-operator", "screening-condition-value", "screening-condition-add", "screening-plan-save", "screening-plan-save-as", "screening-saved-plan-list"]) {
    assert.match(screeningView, new RegExp('id="' + id + '"'), "missing local screening control: " + id);
  }
  assert.match(app, /function loadScreeningPlan\(id\)[\s\S]*?screeningDraftLogic = plan\.logic[\s\S]*?renderScreeningDraftConditions\(\)/);
  assert.match(app, /saveCurrentScreeningPlan\(true\)/);
  assert.match(app, /createScreeningCondition\(\$\("#screening-condition-field"\)/);
  assert.match(app, /researchNotes\.createFromReport\(item\)/);
  assert.match(app, /onCreateNote:/);
  assert.match(app, /getCurrentPageSnapshot\(\)/);
  assert.match(app, /evaluateScreeningConditions\(snapshot\.items, screeningDraftConditions, screeningDraftLogic, \{ expectedAdjustment: snapshot\.adjustment \}\)/);
  assert.match(app, /不代表全市场扫描/);
  assert.doesNotMatch(app, /screening.*localStorage/i);
});


test("当前页选股结果 DOM adapter 只构建行节点，分页快照与选择状态仍由 app 协调", async () => {
  const view = await readFile(new URL("../src/screening-result-view.js", import.meta.url), "utf8");
  assert.match(app, /renderScreeningLocalResultRowsView\(\{[\s\S]*?document, body, rows, filter, snapshot/);
  assert.match(app, /function isCurrentScreeningSnapshot\(snapshot\)[\s\S]*?generation === snapshot\.generation/);
  assert.match(app, /screeningSelectedSymbols\.clear\(\);[\s\S]*?renderScreeningLocalResultRowsView/);
  assert.match(app, /screeningSelectedSymbols\.add\(checkbox\.value\)/);
  assert.match(app, /buildScreeningCSV\(/);
  assert.match(app, /addScreeningResultsToWatchlist\(/);
  assert.match(view, /export function renderScreeningLocalResultRowsView\(/);
  assert.match(view, /screeningLocalSelectSymbol/);
  assert.match(view, /screeningLocalSymbol/);
  assert.match(view, /质量原因未报告/);
  assert.doesNotMatch(view, /\bstate\.|localStorage|fetch\(|addEventListener|screeningSelectedSymbols|isCurrentScreeningSnapshot/);
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
  assert.match(html, /id="alert-rule-submit" type="submit"/);
  assert.match(html, /id="alert-rule-test" type="button" disabled/);
  assert.doesNotMatch(app, /alert.*localStorage/i);
  assert.match(html, /id="alert-rule-symbol"/, "单只股票草稿必须有独立代码输入");
  assert.match(html, /id="alert-rule-symbol-field"/);
  assert.match(html, /id="alert-rule-scope"/);
  assert.match(appSource, /bindLocalAlertDrafts/);

});


test("stock-name autocomplete uses the real market API and accessible, stale-safe controls", () => {
  assert.match(html, /aria-autocomplete="list"/);
  assert.match(html, /id="stock-search-results"[^>]*role="listbox"/);
  assert.match(html, /id="stock-search-status"[^>]*role="status"/);
  assert.match(apiClient, /export function searchStocks\(/);
  assert.match(appSource, /import \{ normalizeStockSearchItems, renderStockSearchResultsView, updateStockSearchActiveOptionView \} from "\.\/stock-search-view\.js"/);
  assert.match(app, /function scheduleStockSearch\(\)/);
  assert.match(app, /new AbortController\(\)/);
  assert.match(app, /generation !== state\.stockSearchGeneration/);
  assert.match(stockSearchView, /export function normalizeStockSearchItems/);
  assert.match(stockSearchView, /export function renderStockSearchResultsView/);
  assert.match(stockSearchView, /export function updateStockSearchActiveOptionView/);
  assert.doesNotMatch(stockSearchView, /localStorage|fetch\s*\(|AbortController|setTimeout|state\./);
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
  assert.match(taskEventNode, /select\.dataset\.taskEventSequence = String\(item\.sequence\)/);
  assert.match(app, /button\[data-task-event-sequence\]/);
  assert.match(app, /state\.taskEventsLoaded/);
  assert.match(taskEventNode, /event\.duration_ms/);
  assert.match(taskEventNode, /event\.error_code/);
  assert.match(taskFlowSummaryViewModel, /lastRunResult\?\.quality\?\.core_pass/);
  assert.match(taskFlowSummaryViewModel, /Array\.isArray\(warnings\)/);
  assert.match(stylesheet, /\.run-flow-summary/);
  assert.match(stylesheet, /\.event-row\[aria-pressed="true"\]/);
  assert.doesNotMatch(app, /completed_stages\s*\/\s*\w+|progressPercent|100\s*\*\s*.*completed_stages/i,
    "Run Flow must not infer or fabricate progress percentages");
});


test("stock research toolbar controls use readable labels and touch-sized targets", () => {
  assert.match(finalCssRuleBody(".stock-research-toolbar label"), /font-size:12px/);
  assert.match(finalCssRuleBody(".stock-research-toolbar select"), /min-height:38px/);
  assert.match(finalCssRuleBody(".stock-research-toolbar select"), /font-size:12px/);
  const toolbarButtonRules = [...stylesheet.matchAll(/\.stock-research-toolbar \.button\s*\{([^}]*)\}/g)].map((match) => match[1]);
  assert.ok(toolbarButtonRules.some((body) => /min-height:\s*38px/.test(body)), "toolbar button must have a touch-sized minimum height");
  assert.ok(toolbarButtonRules.some((body) => /font-size:\s*12px/.test(body)), "toolbar button label must remain readable");
});

test("market overview and activity use readable secondary text and touch-sized controls", () => {
  assert.match(finalCssRuleBody(".market-overview-updated"), /font-size: 12px/);
  const marketOverviewHeadingDescriptionBase = baseCssDeclarationBlock(".market-overview-subheading p");
  const marketOverviewWidgetDescriptionBase = baseCssDeclarationBlock(".market-overview-widget-heading p");
  assert.equal(marketOverviewHeadingDescriptionBase, marketOverviewWidgetDescriptionBase, "both heading description selectors should share one base declaration block");
  assert.match(marketOverviewHeadingDescriptionBase, /font-size:\s*12px\s*;/);
  assert.match(marketOverviewHeadingDescriptionBase, /line-height:\s*1\.65\s*;?/);
  assert.match(marketOverviewHeadingDescriptionBase, /margin:\s*5px 0 0\s*;/);
  assert.match(marketOverviewHeadingDescriptionBase, /color:\s*var\(--text-dim\)\s*;/);
  assert.equal((stylesheet.match(/\.market-overview-subheading p,\.market-overview-widget-heading p\s*\{/g) || []).length, 1, "market overview heading descriptions should share one base rule");
  assert.doesNotMatch(stylesheet, /market-overview-subheading p,\s*\.market-overview-widget-heading p\s*\{\s*font-size:\s*12px/);
  const marketIndexSecondaryBase = baseCssDeclarationBlock(".market-index-identity small");
  assert.match(marketIndexSecondaryBase, /font-size:\s*12px\s*;/);
  assert.match(marketIndexSecondaryBase, /line-height:\s*1\.5\s*;?/);
  assert.match(marketIndexSecondaryBase, /color:\s*var\(--text-dim\)\s*;/);
  assert.equal((stylesheet.match(/\.market-index-identity small,\.market-index-card-footer small\s*\{/g) || []).length, 1, "index identity and footer metadata should share one base rule");
  assert.doesNotMatch(stylesheet, /market-index-identity small,\s*\.market-index-card-footer small\s*\{\s*font-size:\s*12px/);
  const marketIndexChangeBase = baseCssDeclarationBlock(".market-index-changes span");
  assert.match(marketIndexChangeBase, /font-size:\s*12px\s*;/);
  assert.match(marketIndexChangeBase, /color:var\(--text-soft\)/);
  assert.match(marketIndexChangeBase, /font-variant-numeric:tabular-nums/);
  assert.equal((stylesheet.match(/\.market-index-changes span\s*\{/g) || []).length, 1, "index change detail text should have one base rule");
  const marketWidgetMetaBase = baseCssDeclarationBlock(".market-widget-meta");
  assert.match(marketWidgetMetaBase, /font-size:\s*12px\s*;/);
  assert.match(marketWidgetMetaBase, /line-height:\s*1\.65\s*;/);
  assert.match(marketWidgetMetaBase, /margin:10px 0 0/);
  assert.match(marketWidgetMetaBase, /overflow-wrap:anywhere/);
  assert.equal((stylesheet.match(/\.market-widget-meta\s*\{/g) || []).length, 1, "market widget metadata should have one base rule");
  const stockResearchFootnoteBase = baseCssDeclarationBlock(".stock-research-footnote");
  assert.match(stockResearchFootnoteBase, /font-size:\s*12px\s*;/);
  assert.match(stockResearchFootnoteBase, /line-height:\s*1\.65\s*;?/);
  assert.match(stockResearchFootnoteBase, /margin:\s*9px 0 0\s*;/);
  assert.match(stockResearchFootnoteBase, /color:\s*var\(--text-dim\)\s*;/);
  assert.doesNotMatch(stylesheet, /market-widget-meta,[\\s\\S]{0,120}stock-research-footnote/);
  const marketBreadthMetricLabelBase = baseCssDeclarationBlock(".market-breadth-metric small");
  assert.match(marketBreadthMetricLabelBase, /font-size:\s*12px\s*;/);
  assert.match(marketBreadthMetricLabelBase, /line-height:\s*1\.45\s*;?/);
  assert.match(marketBreadthMetricLabelBase, /color:\s*var\(--text-dim\)\s*;/);
  assert.equal((stylesheet.match(/\.market-breadth-metric small\s*\{/g) || []).length, 1, "market breadth metric labels should have one base rule");
  const marketTablePrimaryBase = exactCssDeclarationBlock(".market-table-primary");
  assert.match(marketTablePrimaryBase, /font-size:\s*12px\s*;/);
  assert.match(marketTablePrimaryBase, /line-height:\s*1\.5\s*;?/);
  assert.match(marketTablePrimaryBase, /font-weight:\s*650\s*;?/);
  assert.match(marketTablePrimaryBase, /color:\s*var\(--text\)\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.market-table-primary\s*\{/g) || []).length, 1, "market table primary text should have one exact base rule");
  const marketTableSecondaryBase = exactCssDeclarationBlock(".market-table-secondary");
  assert.match(marketTableSecondaryBase, /font-size:\s*12px\s*;/);
  assert.match(marketTableSecondaryBase, /line-height:\s*1\.45\s*;?/);
  assert.match(marketTableSecondaryBase, /margin-top:\s*4px\s*;?/);
  assert.match(marketTableSecondaryBase, /color:\s*var\(--text-dim\)\s*;?/);
  assert.match(marketTableSecondaryBase, /font-variant-numeric:\s*tabular-nums\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.market-table-secondary\s*\{/g) || []).length, 1, "market table secondary text should have one exact base rule");
  const marketTouchTimeBase = exactCssDeclarationBlock(".market-touch-time");
  assert.match(marketTouchTimeBase, /font-size:\s*12px\s*;/);
  assert.match(marketTouchTimeBase, /line-height:\s*1\.5\s*;?/);
  assert.match(marketTouchTimeBase, /color:\s*var\(--text-soft\)\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.market-touch-time\s*\{/g) || []).length, 1, "market touch time should have one exact base rule");
  const marketQualityReasonsBase = exactCssDeclarationBlock(".market-quality-reasons");
  assert.match(marketQualityReasonsBase, /margin-top:\s*9px\s*;?/);
  assert.match(marketQualityReasonsBase, /color:\s*var\(--text-dim\)\s*;?/);
  assert.match(marketQualityReasonsBase, /font-size:\s*12px\s*;/);
  assert.match(marketQualityReasonsBase, /line-height:\s*1\.7\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.market-quality-reasons\s*\{/g) || []).length, 1, "market quality reasons should have one exact base rule");
  assert.match(finalCssRuleBody(".market-quality-reasons li"), /overflow-wrap:\s*anywhere/);
  assert.match(finalCssRuleBody(".market-activity-widget .market-quality-reasons"), /padding-left:\s*18px/);
  const marketActivityHeadingDescriptionBase = exactCssDeclarationBlock(".market-activity-heading p");
  assert.match(marketActivityHeadingDescriptionBase, /margin:\s*5px 0 0\s*;?/);
  assert.match(marketActivityHeadingDescriptionBase, /color:\s*var\(--text-dim\)\s*;?/);
  assert.match(marketActivityHeadingDescriptionBase, /font-size:\s*12px\s*;/);
  assert.match(marketActivityHeadingDescriptionBase, /line-height:\s*1\.65\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.market-activity-heading p\s*\{/g) || []).length, 1, "market activity heading descriptions should have one exact base rule");
  assert.match(finalCssRuleBody(".market-activity-heading"), /flex-direction:\s*column/);
  const marketActivityControlLabelBase = exactCssDeclarationBlock(".market-activity-controls label");
  assert.match(marketActivityControlLabelBase, /display:\s*flex\s*;?/);
  assert.match(marketActivityControlLabelBase, /flex-direction:\s*column\s*;?/);
  assert.match(marketActivityControlLabelBase, /gap:\s*5px\s*;?/);
  assert.match(marketActivityControlLabelBase, /color:\s*var\(--text-soft\)\s*;?/);
  assert.match(marketActivityControlLabelBase, /font-size:\s*12px\s*;/);
  assert.match(marketActivityControlLabelBase, /line-height:\s*1\.4\s*;?/);
  assert.equal((stylesheet.match(/\.market-activity-controls label\s*\{/g) || []).length, 3, "market activity control labels should have one base rule and two responsive layout rules");
  assert.match(stylesheet, /\.market-activity-controls label \{\s*flex:1;\s*min-width:130px\s*\}/);
  assert.match(stylesheet, /\.market-activity-controls label \{ min-width: 0; flex: 1 1 140px; \}/);
  assert.ok(stylesheet.includes(".market-activity-controls label { min-width: 0; flex: 1 1 140px; }"), "600px mobile label layout should remain without typography overrides");
  const marketActivityControlInputBase = exactCssDeclarationBlock(".market-activity-controls input");
  assert.match(marketActivityControlInputBase, /min-height:\s*38px\s*;?/);
  assert.match(marketActivityControlInputBase, /max-width:\s*170px\s*;?/);
  assert.match(marketActivityControlInputBase, /padding:\s*6px 8px\s*;?/);
  assert.equal((stylesheet.match(/\.market-activity-controls input\s*\{/g) || []).length, 2, "market activity date inputs should have one base rule and one responsive width rule");
  assert.match(stylesheet, /\.market-activity-controls input \{\s*width:100%;\s*max-width:none\s*\}/);
  const marketActivityControlButtonBase = exactCssDeclarationBlock(".market-activity-controls .button");
  assert.match(marketActivityControlButtonBase, /min-height:\s*38px\s*;?/);
  assert.match(marketActivityControlButtonBase, /padding:\s*6px 10px\s*;?/);
  assert.match(marketActivityControlButtonBase, /font-size:\s*12px\s*;?/);
  assert.equal((stylesheet.match(/\.market-activity-controls \.button\s*\{/g) || []).length, 2, "market activity controls should have one base button rule and one mobile flex rule");
  assert.ok(stylesheet.includes(".market-activity-controls .button { flex: 1 1 100%; }"), "mobile button width behavior should remain without typography overrides");
  const marketActivityFooterMetaBase = exactCssDeclarationBlock(".market-activity-footer span");
  assert.match(marketActivityFooterMetaBase, /color:\s*var\(--text-dim\)\s*;?/);
  assert.match(marketActivityFooterMetaBase, /font-size:\s*12px\s*;?/);
  assert.equal((stylesheet.match(/\.market-activity-footer span\s*\{/g) || []).length, 1, "market activity footer metadata should have one exact base rule");
  assert.match(exactCssDeclarationBlock(".market-activity-footer"), /flex-wrap:\s*wrap/);
  const marketActivityFooterButtonBase = exactCssDeclarationBlock(".market-activity-footer .button");
  assert.match(marketActivityFooterButtonBase, /min-height:\s*36px\s*;?/);
  assert.match(marketActivityFooterButtonBase, /padding:\s*5px 9px\s*;?/);
  assert.match(marketActivityFooterButtonBase, /font-size:\s*12px\s*;?/);
  assert.equal((stylesheet.match(/\.market-activity-footer \.button\s*\{/g) || []).length, 1, "market activity footer button should have one exact base rule");
  const readabilityStart = stylesheet.indexOf("/* 2026-10-27 readability pass: market overview, activity table, and calendar. */");
  const mobileOverrideStart = stylesheet.indexOf("@media (max-width: 700px)", readabilityStart);
  assert.ok(readabilityStart >= 0 && mobileOverrideStart > readabilityStart, "the general readability pass should precede responsive overrides");
  const generalRules = stylesheet.slice(readabilityStart, mobileOverrideStart);
  assert.match(marketActivityControlLabelBase, /font-size:\s*12px\s*;/, "desktop labels need a readable base font size");
  assert.match(marketActivityControlLabelBase, /line-height:\s*1\.4\s*;?/, "desktop labels need a readable base line height");
  assert.match(marketActivityControlInputBase, /min-height:\s*38px\s*;?/, "date inputs need a 38px base touch target");
  assert.match(marketActivityControlButtonBase, /min-height:\s*38px\s*;?/, "market activity buttons need a 38px base touch target");
  assert.match(marketActivityControlButtonBase, /font-size:\s*12px\s*;?/, "market activity button labels need a readable base font size");
  const activityStockLinkBase = exactCssDeclarationBlock(".activity-stock-link");
  assert.match(activityStockLinkBase, /display:\s*block\s*;?/);
  assert.match(activityStockLinkBase, /min-height:\s*38px\s*;?/);
  assert.match(activityStockLinkBase, /margin-top:\s*5px\s*;?/);
  assert.match(activityStockLinkBase, /padding:\s*5px 0\s*;?/);
  assert.match(activityStockLinkBase, /font-size:\s*12px\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.activity-stock-link\s*\{/g) || []).length, 1, "activity stock links should have one exact base rule");
  assert.match(finalCssRuleBody(".activity-stock-link:hover"), /text-decoration:\s*underline/);
  const calendarMonthButtonBase = exactCssDeclarationBlock(".calendar-month-navigation .icon-button");
  assert.match(calendarMonthButtonBase, /min-width:\s*38px\s*;?/);
  assert.match(calendarMonthButtonBase, /min-height:\s*38px\s*;?/);
  assert.match(calendarMonthButtonBase, /padding:\s*4px 8px\s*;?/);
  assert.match(calendarMonthButtonBase, /font-size:\s*13px\s*;?/);
  assert.equal((stylesheet.match(/\.calendar-month-navigation \.icon-button\s*\{/g) || []).length, 1, "calendar month buttons should have one exact base rule");
  const tradingCalendarWeekdayBase = exactCssDeclarationBlock(".trading-calendar-weekdays span");
  assert.match(tradingCalendarWeekdayBase, /font-size:\s*11px\s*;?/);
  assert.match(tradingCalendarWeekdayBase, /font-weight:\s*650\s*;?/);
  assert.match(tradingCalendarWeekdayBase, /padding:\s*5px 0\s*;?/);
  assert.equal((stylesheet.match(/\.trading-calendar-weekdays span\s*\{/g) || []).length, 1, "trading calendar weekdays should have one exact base rule");
  const tradingCalendarDayBase = exactCssDeclarationBlock(".trading-calendar-day");
  assert.match(tradingCalendarDayBase, /min-height:\s*56px\s*;?/);
  assert.match(tradingCalendarDayBase, /padding:\s*6px 2px\s*;?/);
  assert.match(tradingCalendarDayBase, /gap:\s*5px\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.trading-calendar-day\s*\{/g) || []).length, 2, "trading calendar day should have one base rule and one mobile override");
  const tradingCalendarDayLabelBase = exactCssDeclarationBlock(".trading-calendar-day span");
  assert.match(tradingCalendarDayLabelBase, /font-size:\s*11px\s*;?/);
  assert.match(tradingCalendarDayLabelBase, /line-height:\s*1\.25\s*;?/);
  assert.match(tradingCalendarDayLabelBase, /text-align:\s*center\s*;?/);
  assert.match(tradingCalendarDayLabelBase, /overflow-wrap:\s*anywhere\s*;?/);
  assert.equal((stylesheet.match(/(?:^|})\s*\.trading-calendar-day span\s*\{/g) || []).length, 2, "trading calendar day label should have one base rule and one mobile override");
  assert.match(stylesheet, /\.trading-calendar-day \{ min-height: 50px; padding: 5px 1px; \}/, "mobile calendar day sizing must remain in the exact responsive rule");
  assert.match(finalCssRuleBody(".trading-calendar-day span"), /font-size:\s*10px/);
  assert.match(stylesheet, /@media \(max-width: 700px\) \{[\s\S]*?\.market-overview-actions \.button \{ width: 100%; min-height: 40px; \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.trading-calendar-day span \{ font-size: 10px; \}/);
  assert.match(baseCssDeclarationBlock(".table-scroll"), /-webkit-overflow-scrolling:\s*touch\s*;/);
});


test("primary navigation keeps market modules on the homepage without quick-locate UI", () => {
  for (const view of ["market", "stocks", "watchlist"]) {
    assert.match(html, new RegExp('class="nav-link"[^>]*data-view="' + view + '"'));
  }
  assert.match(html, /id="market-view"/);
  assert.match(html, /id="stocks-view"/);
  assert.match(html, /id="watchlist-view"/);
  assert.doesNotMatch(html, /id="quick-locate"|class="quick-locate"|data-view="quick-locate"/);
  assert.match(html, /market-sector-body/);
});
test("collapsed sidebar navigation remains accessible and unavailable modules disclose their status", () => {
  assert.match(html, /class="nav-link"[^>]*data-view="screening"[^>]*aria-label="选股与筛选"/);
  assert.match(html, /class="nav-link"[^>]*data-view="alerts"[^>]*aria-label="告警"/);
  assert.match(html, /class="nav-link-state" title="后端接口待接入">待接入/);
  assert.match(stylesheet, /nav-link-state/);
});
test("深浅主题的辅助文字在常用背景上达到 WCAG AA 对比度", () => {
  const parseHex = (value) => {
    const hex = value.trim().replace(/^#/, "");
    const full = hex.length === 3 ? hex.split("").map((part) => part + part).join("") : hex;
    const number = Number.parseInt(full, 16);
    return [(number >> 16) & 255, (number >> 8) & 255, number & 255].map((channel) => {
      const normalized = channel / 255;
      return normalized <= 0.04045 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
    });
  };
  const luminance = (value) => {
    const [r, g, b] = parseHex(value);
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const contrast = (foreground, background) => {
    const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
    return (values[0] + 0.05) / (values[1] + 0.05);
  };
  assert.match(stylesheet, /--text-dim:\s*#[0-9a-f]{3,6}/i);
  assert.match(stylesheet, /--panel:\s*#[0-9a-f]{3,6}/i);
  assert.match(html, /id="storage-boundary-title"/);
  assert.ok(contrast("#666666", "#ffffff") >= 5, "baseline secondary text contrast helper should use WCAG AA threshold");
});
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

test("研究与设置分区导航会移动键盘焦点并按全部可见区块同步高亮", async () => {
  const sectionNavigation = await readFile(new URL("../src/section-navigation.js", import.meta.url), "utf8");
  assert.match(sectionNavigation, /focusTarget\.focus\(\{ preventScroll: true \}\)/);
  assert.match(sectionNavigation, /const current = targets[\s\S]*?getBoundingClientRect\(\)/);
  assert.match(sectionNavigation, /window\.matchMedia/);
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
  const importBody = sourceBetween(
    app,
    "async function applyBrowserDataImport(imported, currentWatchlist, currentRecentTasks, currentWatchlistGroups, strategy, previewStorageSnapshot) {",
    "\n\nsyncThemeControl(readTheme());",
    "工作区导入"
  );
  assert.match(
    importBody,
    /localStorage\.removeItem\(TASK_REFERENCE_KEY\);[\s\S]*?applySettings\(nextSettings, \{ persistPreviousActiveTask: false \}\);[\s\S]*?applyTheme\(imported\.theme\)/
  );
  assert.match(importBody, /const importedActiveTaskId = state\.taskId;[\s\S]*?const importedActiveTaskBase = state\.taskAnalysisBase \|\| state\.settings\.analysisBase;/);
  assert.match(importBody, /applySettings\(nextSettings, \{ persistPreviousActiveTask: false \}\);[\s\S]*?state\.suppressedHistoryTaskId = importedActiveTaskId;[\s\S]*?state\.suppressedHistoryTaskBase = importedActiveTaskId \? importedActiveTaskBase : "";/,
    "导入工作区后，旧活动任务不能在后续轮询中重新写回本地历史");
  const applyIndex = importBody.indexOf("applySettings(nextSettings, { persistPreviousActiveTask: false })");
  assert.ok(applyIndex >= 0, "workspace import must apply the validated settings");
  const rollbackIndex = importBody.indexOf("const rollbackFailures = []");
  assert.ok(rollbackIndex > 0 && rollbackIndex < applyIndex,
    "storage rollback must finish before the in-memory settings are applied");
  assert.match(importBody.slice(rollbackIndex, applyIndex),
    /localStorage\.(?:setItem|removeItem)\([\s\S]*?localStorage\.getItem\(key\) !== \(value \?\? null\)[\s\S]*?rollbackFailures\.push\(key\)/,
    "rollback must read back every restored key and report silent failures");
  const uiFailureIndex = importBody.indexOf("页面状态刷新未完成", applyIndex);
  assert.ok(uiFailureIndex > applyIndex, "UI refresh failures must be reported separately after storage commit");
  assert.doesNotMatch(
    importBody.slice(applyIndex, uiFailureIndex),
    /localStorage\.(?:setItem|removeItem)\(|persistTheme\(/,
    "post-commit UI refresh must not mutate or roll back the imported storage"
  );
  assert.match(
    importBody,
    /const expectedStorage = new Map\(\[[\s\S]*?\[TASK_REFERENCE_KEY, null\][\s\S]*?for \(const \[key, expectedValue\] of expectedStorage\)[\s\S]*?localStorage\.getItem\(key\) !== expectedValue[\s\S]*?throw new Error\("浏览器存储未能确认写入："/,
    "workspace import must verify every stored value before changing in-memory state"
  );
  assert.match(importBody.slice(rollbackIndex), /storageKeys\.forEach\([\s\S]*?localStorage\.(?:setItem|removeItem)\(/,
    "the failure path should attempt to restore the pre-import storage snapshot"
  );
});

test("仅板块资金净流入保留能力占位，已接入维度透传真实排序参数", () => {
  assert.match(
    marketOverview,
    /async function load\(key\)\s*\{[\s\S]*?cancel\(key\);\s*seq=generation\[key\];[\s\S]*?if\(key==="sectors"&&sectorMetric==="flow"\)\{showSectorMetricUnavailable\(\);return "unsupported";\}/
  );
  assert.match(
    marketOverview,
    /getMarketSectors\(settings,\{type:sectorType,limit:sectorLimit,direction:sectorDirection,sortBy:sectorMetricSortBy\(sectorMetric\),signal:ctl\.signal\}\)/
  );
  assert.match(
    marketOverview,
    /sectorType=button\.dataset\.sectorType==="concept"\?"concept":"industry"[\s\S]*?void load\("sectors"\)/
  );
  assert.match(
    marketOverview,
    /sectorMetric=\["change","amount","five-day","flow"\][\s\S]*?void load\("sectors"\)/
  );
  assert.match(marketOverview, /data-sector-direction/);
  assert.match(marketOverview, /sectorLimit=10/);
  assert.match(marketOverview, /sectorLimit=\[10,20,50,100\]\.includes\(value\)\?value:10/);
  assert.match(marketOverview, /sectorLimitControl\?\.addEventListener\("change"/);
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

test("数据中心检查初始化与 finally 解锁异常不会破坏 generation 收尾", () => {
  const refresh = dataCenter.match(/async function refresh\(\) \{[\s\S]*?\n  \}/)?.[0] || "";
  assert.ok(refresh);
  assert.match(refresh, /checking = true;[\s\S]*?try \{\s*settings = typeof getSettings/);
  assert.match(refresh, /按钮 DOM 更新失败不能阻止服务探测[\s\S]*?await Promise\.all/);
  assert.match(refresh, /catch \(error\) \{\s*if \(current !== generation\) return;[\s\S]*?safeError\(error\)/);
  assert.match(refresh, /finally \{[\s\S]*?checking = false;[\s\S]*?button\.disabled = false;[\s\S]*?finally 产生新的 rejection/);
  const invalidate = dataCenter.match(/function invalidate\(message = [\s\S]*?\n  \}/)?.[0] || "";
  assert.match(invalidate, /generation \+= 1;[\s\S]*?safeInvalidateStep/);
  assert.match(invalidate, /safeInvalidateStep\(\(\) => setBadge\("#data-center-summary-status"/);
  assert.match(invalidate, /forEach\(\(selector\) => safeInvalidateStep/);
});

test("数据中心超时后即使探针迟到返回，也不能覆盖为成功", () => {
  assert.match(
    healthCheck,
    /\.then\(\(data\) => timedOut\s*\? \{ ok: false, error: new Error\("检查超时"\), timedOut: true/
  );
});

test("行情查询初始化与 finally 渲染异常不会遗留查询按钮锁", () => {
  const start = app.indexOf("async function runQuoteQuery(event) {");
  const end = app.indexOf("\nfunction ", start + 20);
  const query = start >= 0 && end > start ? app.slice(start, end) : "";
  const tryStart = query.indexOf("try {", query.indexOf("const generation = ++state.quoteQueryGeneration;"));
  assert.ok(tryStart >= 0, "请求生命周期必须有主 try");
  assert.ok(query.indexOf('state.quoteQueryController = controller;', tryStart) > tryStart);
  assert.ok(query.indexOf('$("#quote-form").classList.add("is-loading");', tryStart) > tryStart);
  assert.match(query, /catch \(error\) \{[\s\S]*?清理旧行情显示失败：[\s\S]*?finally \{/);
  const finallyStart = query.lastIndexOf("} finally {");
  const finalizer = query.slice(finallyStart);
  assert.match(finalizer, /state\.isQuerying = false;[\s\S]*?try \{\s*renderResearchOverview\(\);\s*\} catch/);
  assert.match(finalizer, /catch \(renderError\)[\s\S]*?try \{\s*releaseQuoteQueryControls\(\);\s*\} catch \(unlockError\)/);
  assert.match(finalizer, /行情请求已结束，但查询按钮状态恢复失败/);
});

test("市场工作台各模块刷新失败会独立收敛并显示诊断", () => {
  const refresh = app.match(/function refreshMarketWorkspace\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(refresh, "refreshMarketWorkspace implementation must exist");
  assert.match(refresh, /const startModule = \(label, operation\) => \{[\s\S]*?Promise\.resolve\(operation\(\)\)\.then/);
  assert.match(refresh, /\(error\) => \(\{ label, ok: false, error \}\)/);
  assert.match(refresh, /catch \(error\) \{[\s\S]*?return Promise\.resolve\(\{ label, ok: false, error \}\)/);
  assert.match(refresh, /startModule\("市场总览"[\s\S]*?startModule\("资金与活跃度"/);
  assert.match(refresh, /Promise\.all\(requests\)\.then\(\(results\) => \{[\s\S]*?result\.ok[\s\S]*?市场工作台部分模块刷新失败/);
  assert.match(refresh, /市场工作台刷新汇总异常：[\s\S]*?return \[\]/);
});

test("服务健康刷新把请求与 DOM 异常收敛为结果", () => {
  const refresh = app.match(/async function refreshServices\(\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(refresh, "refreshServices implementation must exist");
  assert.match(refresh, /try \{[\s\S]*?await Promise\.all\(\[[\s\S]*?runHealthCheck\(marketHealth, settings\)/);
  assert.match(refresh, /catch \(renderError\)[\s\S]*?状态显示更新失败[\s\S]*?return \{ marketOK, analysisOK, marketState, analysisState, stale: false, renderError: message \}/);
  assert.match(refresh, /catch \(error\)[\s\S]*?服务健康检查流程异常[\s\S]*?return \{ \.\.\.staleResult, stale: false, error: message \}/);
  assert.match(refresh, /generation !== serviceRefreshGeneration/);
  assert.match(refresh, /label\.title = message/);
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
  assert.match(app, /function serviceHealthState\(result\)[\s\S]*?return "unknown";/);
  assert.match(app, /setText\("#connection-label", connectionLabel\)/);
  assert.doesNotMatch(app, /setServiceStatus\(/, "service refresh must not reference removed per-service sidebar elements");
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
  assert.match(recentTaskView, /historyState\.storageReadable[\s\S]*?historyState\.storageValid[\s\S]*?历史修改已暂停/);
  assert.match(app, /recent_tasks: \(\(\) => \{[\s\S]*?recentTasksState\.storageReadable[\s\S]*?recentTasksState\.storageValid[\s\S]*?导出已取消/);
});


test("最近任务新增与删除必须确认完整记录已持久化，失败时保留原列表", () => {
  assert.match(recentTaskModule, /storage\.setItem\(RECENT_TASKS_KEY, JSON\.stringify\(records\)\)[\s\S]*?const persistedState = readRecentTasksState\(storage, limit\)[\s\S]*?return persisted \? persistedState\.items : currentState\.items/);
  assert.match(recentTaskModule, /if \(JSON\.stringify\(persistedState\.items\) !== JSON\.stringify\(records\)\) return currentState\.items/);
  assert.match(recentTaskModule, /删除未成功持久化时保留原列表/);
  assert.match(app, /function persistRecentTaskHistory\(record\)[\s\S]*?const expected = \{[\s\S]*?JSON\.stringify\(item\.symbols\) === JSON\.stringify\(expected\.symbols\)[\s\S]*?最近任务引用未能确认持久化/);
});


test("活动任务引用静默写入失败时仍独立保存最近任务历史并明确提示", () => {
  assert.match(
    app,
    /const serializedReference = JSON\.stringify\(record\);[\s\S]*?localStorage\.setItem\(TASK_REFERENCE_KEY, serializedReference\);[\s\S]*?taskReferencePersisted = localStorage\.getItem\(TASK_REFERENCE_KEY\) === serializedReference/
  );
  assert.match(
    app,
    /const historyPersisted = persistRecentTaskHistory\(record\);[\s\S]*?if \(!taskReferencePersisted\)[\s\S]*?最近任务已保存，但活动任务引用未能确认持久化/
  );
});

test("估值请求补齐交易所后缀，并只展示与当前标的匹配的记录", () => {
  const renderer = stockResearch.match(/function renderValuations\(data,symbol\)\{[\s\S]*?\n \}\n function fractionPercent/)?.[0] || "";
  assert.ok(renderer, "估值渲染器应存在");
  assert.match(stockResearch, /function normalizeValuationSymbol\(input\)/);
  assert.match(stockResearch, /import \{ comparableMarketSymbol, .* from "\.\/format\.js"/);
  assert.match(stockResearch, /const normalized=comparableMarketSymbol\(value\)/);
  assert.match(stockResearch, /if\(!value\.includes\("\."\)&&normalized===value\)[\s\S]*?无法从该六位代码判断交易所/);
  assert.ok(stockResearch.includes("getMarketValuations(settings,[normalizeValuationSymbol(sym)],signal)"));
  assert.ok(renderer.includes('const requestedSymbol=String(symbol||data.requested_symbols?.[0]||"").trim();'));
  assert.match(renderer, /const targetSymbol=normalizeValuationSymbol\(requestedSymbol\)/);
  assert.match(renderer, /items\.find\(value=>sameValuationSymbol\(value\.symbol,targetSymbol\)\)/);
  assert.doesNotMatch(renderer, /items\[0\]/, "目标股票未匹配时不得退回展示列表第一条记录");
  assert.match(renderer, /估值接口返回的证券代码与当前标的不匹配，已忽略这些记录/);
});

 
test("季度财务指标读取的报告期校验与成功提示异常不会向外拒绝", () => {
  const loader = stockResearch.match(/async function loadFundamentals\(\)\{[\s\S]*?\n \}/)?.[0] || "";
  assert.ok(loader);
  assert.match(loader, /try\{[\s\S]*?syncSymbol\(\)/);
  assert.match(loader, /if\(!Number\.isInteger\(year\)[\s\S]*?safeResearchFeedback\("财务报告期不正确。"\)/);
  assert.match(loader, /return runSupplementRequest\("#load-fundamentals","读取财务指标","fundamentals"[\s\S]*?季度财务指标已返回/);
  assert.match(loader, /catch\(error\)\{\s*safeResearchFeedback\("季度财务指标读取流程异常："\+safeResearchError\(error\)\);\s*return false;/);
});

test("个股研究准备阶段异常与 finally 异常不会遗留按钮锁", () => {
  assert.match(stockResearch, /function safeResearchButton\(button,disabled,label(?:,busy=false)?\)[\s\S]*?catch\{return false;\}/);
  assert.match(stockResearch, /if\(busy\)button\.setAttribute\("aria-busy","true"\);else button\.removeAttribute\("aria-busy"\)/);
  assert.match(stockResearch, /\]\s*\.forEach\(\(\[selector,label\]\)=>safeResearchButton\(\$\(selector\),false,label\)\)/);
  assert.match(stockResearch, /async function fetchOne\(key,request,render\)[\s\S]*?try\{\s*abort\(key\);[\s\S]*?await request\(settings,symbol,ctl\.signal\)[\s\S]*?finally\{/);
  assert.match(stockResearch, /async function loadResearch\(\)[\s\S]*?safeResearchButton\(button,true,"正在加载…"[\s\S]*?finally\{[\s\S]*?safeResearchButton\(button,false,"重新加载公司资料"\)/);
  assert.match(stockResearch, /async function runSupplementRequest\([\s\S]*?let symbol="",button=null,generation=null,ownsButton=false;[\s\S]*?try\{[\s\S]*?const operation=fetchOne\(key,request,render\)[\s\S]*?finally\{[\s\S]*?safeResearchButton\(button,false,idleText\)/);
  assert.match(stockResearch, /async function loadAnnouncements\(append=false\)[\s\S]*?try\{[\s\S]*?safeResearchButton\(\$\("#load-more-announcements"\),true,"正在加载…"[\s\S]*?finally\{[\s\S]*?safeResearchButton\([\s\S]*?"加载更多公告"/);
});

test("个股研究响应代码不匹配时忽略结果", () => {
  assert.match(stockResearch, /function assertResponseSymbol\(data,requestedSymbol\)/);
  assert.match(stockResearch, /if\(responseSymbol&&!sameValuationSymbol\(responseSymbol,requestedSymbol\)\)/);
  assert.ok(stockResearch.includes('"服务端返回证券代码 "+responseSymbol+" 与请求标的 "+requestedSymbol+" 不匹配，已忽略结果。'));
  assert.match(stockResearch, /async function fetchOne\(key,request,render\)[\s\S]*?assertResponseSymbol\(data,symbol\);[\s\S]*?render\(data,symbol\)/);
  assert.match(stockResearch, /async function loadAnnouncements\(append=false\)[\s\S]*?getStockAnnouncements\(settings,symbol,[\s\S]*?assertResponseSymbol\(data,symbol\);[\s\S]*?const root=\$\("#announcement-list"\)/);
});

 
test("主行情、日 K 与统一快照拒绝证券代码不匹配的响应", () => {
  assert.match(format, /export function comparableMarketSymbol\(input\)/);
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
  assert.match(taskWorkflow, /payload\.task_id !== taskId[\s\S]*?阶段事件接口未返回 task_id/);
  assert.match(app, /if \(taskError && \(!isRetryableApiError\(taskError\) \|\| taskError\.responseContractError\)\)/);
  assert.match(app, /if \(isRetryableApiError\(eventError\) && !eventError\.responseContractError\)/);
  assert.match(app, /if \(result\?\.run_id !== runId\) \{[\s\S]*?Run 结果接口未返回 run_id/);
  assert.match(app, /!error\.responseContractError && isRetryableApiError\(error\)/);
});

test("mobile navigation drawer moves focus inside and traps Tab navigation", () => {
  assert.match(app, /if \(!wasOpen && isOpen\) \$\("\.sidebar"\)\.querySelector\("\.nav-link:not\(\[disabled\]\)"\)\?\.focus\(\)/);
  assert.match(app, /if \(event\.key !== "Tab"\) return;[\s\S]*?if \(window\.innerWidth > 600\) return;[\s\S]*?const firstFocusable = focusable\[0\];[\s\S]*?const lastFocusable = focusable\[focusable\.length - 1\]/);
  assert.match(app, /event\.shiftKey[\s\S]*?lastFocusable\.focus\(\)[\s\S]*?firstFocusable\.focus\(\)/);
});
test("移动侧栏打开连接设置时关闭抽屉并进入设置中心", () => {
  assert.match(html, /class="nav-link"[^>]*data-view="settings"/);
  assert.match(app, /if \(nextView === "settings" && previousView !== nextView\) openSettings\(\)/);
  assert.match(app, /button\.addEventListener\("click", \(\) => \{\s*setView\(button\.dataset\.view\);\s*setMobileNavigationOpen\(false, \{ restoreFocus: true \}\);/);
  assert.doesNotMatch(app, /settings-dialog/);
});
test("numeric UI presentation rejects boolean or structured market fields", () => {
  assert.match(format, /export function finiteNumberOrNull\(value\)/);
  assert.match(format, /typeof value !== "number" && typeof value !== "string"/);
  assert.match(marketOverview, /function count\(v\) \{ const n=finiteNumberOrNull\(v\)/);
  assert.match(marketOverview, /function dir\(v\) \{ const n=finiteNumberOrNull\(v\)/);
  assert.match(marketActivity, /function amount\(v\)\{const n=finiteNumberOrNull\(v\)/);
  assert.match(stockResearch, /function formatNumber\(value\)\{const number=finiteNumberOrNull\(value\)/);
  assert.match(app, /const changeValue = finiteNumberOrNull\(quote\.change\)/);
  assert.match(app, /const changePercentValue = matched \? finiteNumberOrNull\(quote\.change_percent\) : null/);
  assert.match(app, /const ratio = finiteNumberOrNull\(v\)/);
  assert.doesNotMatch(app, /Number\(quote\.(?:change|change_percent|volume_ratio)\)|Number\(bar\.change_percent\)/, "market values must not use coercive Number() for display or direction styling");
  assert.match(app, /const changePercent = finiteNumberOrNull\(bar\.change_percent\)/);
});

 
test("任务创建成功响应缺少 task_id 时提示结果未确认，避免误报失败", () => {
  assert.match(app, /if \(!task \|\| typeof task\.task_id !== "string" \|\| !task\.task_id\.trim\(\)\) \{[\s\S]*?error\.creationOutcomeUnknown = true/);
  assert.match(app, /typeof task\?\.task_id === "string" && task\.task_id\.trim\(\)/);
  assert.ok(app.includes("error?.creationOutcomeUnknown ? error.message"));
  assert.match(app, /无法确认任务是否已创建[\s\S]*?避免盲目重复提交/);
});

 
test("任务状态和阶段事件响应结构无效时暂停重试，未知状态不进入轮询循环", () => {
  assert.match(app, /const TASK_STATUSES = new Set\(\["queued", "running", "succeeded", "partial", "failed"\]\)/);
  assert.match(app, /function responseContractError\(message\)[\s\S]*?error\.responseContractError = true/);
  assert.match(app, /typeof candidate\.status !== "string" \|\| !TASK_STATUSES\.has\(candidate\.status\)/);
  assert.match(app, /typeof candidate\.run_id !== "string" \|\| !candidate\.run_id\.trim\(\)/);
  assert.match(app, /state\.runId && candidate\.run_id !== state\.runId[\s\S]*?当前任务已关联的 Run 不一致/);
  assert.match(app, /任务状态接口返回了未支持的状态/);
  assert.match(taskWorkflow, /阶段事件接口返回的数据结构无效/);
  assert.match(app, /eventError = eventPage\.identityMismatch[\s\S]*?eventPage\.error/);
  assert.match(app, /if \(!isRetryableApiError\(eventError\) \|\| eventError\.responseContractError\) state\.taskEventsUnavailable = true/);
  assert.match(app, /if \(delay !== undefined && !error\.responseContractError && isRetryableApiError\(error\)\)/);
});

test("Run 结果必须与当前任务标的集合一致后才允许渲染", () => {
  assert.match(app, /function runResultSymbolMismatch\(result\)/);
  assert.match(app, /const expectedRaw = Array\.isArray\(state\.taskSymbols\) \? state\.taskSymbols : \[\]/);
  assert.match(app, /Run 结果未返回请求标的列表，无法确认是否属于当前任务，已忽略/);
  assert.match(app, /function sameTaskSymbolSet\(expectedRaw, actualRaw\)[\s\S]*?normalizedTaskSymbolSet\(expectedRaw\)[\s\S]*?normalizedTaskSymbolSet\(actualRaw\)[\s\S]*?JSON\.stringify\(expected\) === JSON\.stringify\(actual\)/);
  assert.match(app, /const symbolMismatch = runResultSymbolMismatch\(result\);[\s\S]*?if \(symbolMismatch\) throw responseIdentityError\(symbolMismatch\);[\s\S]*?renderTaskResult\(result\)/);
  assert.match(app, /generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\|\s*endpointGeneration !== state\.analysisConnectionGeneration \|\|\s*state\.taskAnalysisBase !== analysisBase/);
});

 
test("任务状态轮询不允许更换已建立的 task_id/run_id 关联", () => {
  const poller = app.match(/async function pollTask\([\s\S]*?\n\}/)?.[0] || "";
  assert.ok(poller, "task poller must exist");
  assert.match(poller, /candidate\.task_id !== taskId/);
  assert.match(poller, /typeof candidate\.run_id !== "string" \|\| !candidate\.run_id\.trim\(\)/);
  assert.match(poller, /state\.runId && candidate\.run_id !== state\.runId/);
  assert.match(poller, /任务状态接口返回的 run_id 与当前任务已关联的 Run 不一致/);
  assert.match(poller, /taskError = responseIdentityError\([\s\S]*?run_id 与当前任务已关联的 Run 不一致/);
});

 
test("个股行情、研究摘要与均线匹配时兼容纯六位代码和带交易所后缀代码", () => {
  assert.match(app, /function sameMarketSymbol\(left, right\)/);
  assert.match(app, /items\.some\(\(item\) => sameMarketSymbol\(item\.symbol, currentSymbol\)\)/);
  assert.match(app, /technicalSymbol && sameMarketSymbol\(technicalSymbol, currentSymbol\)/);
  assert.match(app, /!sameMarketSymbol\(technicalSymbol, currentSymbol\)/);
  assert.match(app, /sameMarketSymbol\(item\.symbol, currentSymbol\) && Boolean\(quote\)/);
  assert.match(app, /state\.quote && sameMarketSymbol\(state\.quote\.symbol \|\| "", symbol\)/);
});

 
test("自选股报价和任务引用的股票代码比较兼容交易所后缀差异", () => {
  assert.match(app, /function normalizeWatchlistQuoteSymbol\(value\) \{\s*try \{ return comparableMarketSymbol\(value\);/);
  assert.match(app, /const normalizeLocalSymbol = normalizeWatchlistQuoteSymbol/);
  assert.match(app, /const quoteMatches = Boolean\(state\.quote\?\.symbol\) && normalizeLocalSymbol\(state\.quote\.symbol\) === itemSymbol/);
  assert.match(watchlistTrackingViewModel, /task\.symbols\.some\(\(value\) => normalizeSymbol\(value\) === itemSymbol\)/);
  assert.match(watchlistTrackingViewModel, /currentTask\.symbols\.some\(\(value\) => normalizeSymbol\(value\) === itemSymbol\)/);
  assert.match(app, /state\.quote\?\.name && state\.quote\?\.symbol && sameMarketSymbol\(state\.quote\.symbol, symbol\)/);
});

 
test("阶段事件严格校验数值序号与 event 对象结构", () => {
  assert.match(taskWorkflow, /const sequence = typeof item\?\.sequence === "number" \? item\.sequence : NaN/);
  assert.match(taskWorkflow, /!Number\.isSafeInteger\(sequence\) \|\| sequence < 1/);
  assert.match(taskWorkflow, /!isRecord\(item\.event\)/);
  assert.match(app, /字段无效的阶段事件（序号或 event 结构不符合契约），已忽略/);
  assert.doesNotMatch(taskWorkflow, /const sequence = Number\(item\?\.sequence\)/, "boolean sequence must not coerce to integer 1");
});

test("阶段事件的 Run ID 若存在必须匹配当前任务关联的 Run", () => {
  assert.match(taskWorkflow, /Object\.prototype\.hasOwnProperty\.call\(item\.event, "run_id"\)/);
  assert.match(taskWorkflow, /const eventRunId = typeof item\.event\.run_id === "string" \? item\.event\.run_id\.trim\(\) : ""/);
  assert.match(taskWorkflow, /!runId \|\| !eventRunId \|\| eventRunId !== runId/);
  assert.match(taskWorkflow, /domain\.StageEvent 的 run_id 并非必填字段/);
  assert.match(app, /Run ID 与当前任务关联的 Run 不一致，已忽略/);
  assert.match(taskWorkflow, /rejected\.mismatchedRun \+= 1/);
});

test("增量阶段事件不会覆盖已读取的相同或更旧序号", () => {
  assert.match(taskWorkflow, /sequence <= highestSequence \|\| seenSequences\.has\(sequence\) \|\| knownSequences\.has\(sequence\)/);
  assert.match(taskWorkflow, /events\.push\(\{ \.\.\.item, sequence \}\)/);
  assert.match(app, /state\.taskEvents\.set\(item\.sequence, item\)/);
  assert.match(app, /重复或旧序号的阶段事件，已忽略/);
});

test("终态任务按 next_after_sequence 排空阶段事件分页后才读取 Run 报告", () => {
  assert.match(app, /taskEventCursor: 0/);
  const pollStart = app.indexOf("async function pollTask(");
  const pollEnd = app.indexOf("\nasync function createTask(", pollStart);
  const poller = pollStart >= 0 && pollEnd > pollStart ? app.slice(pollStart, pollEnd) : "";
  assert.match(poller, /const requestedEventCursor = state\.taskEventCursor/);
  assert.ok(poller.includes("getAnalysisTaskEvents(requestSettings, taskId, requestedEventCursor, { signal: controller.signal })"));
  assert.match(taskWorkflow, /!Number\.isSafeInteger\(nextAfterSequence\)/);
  assert.match(poller, /state\.taskEventCursor = eventPage\.nextAfterSequence/);
  assert.match(taskWorkflow, /hasMore: payload\.events\.length >= TASK_EVENT_PAGE_SIZE && nextAfterSequence > afterSequence/);
  assert.match(poller, /if \(task && TERMINAL_TASK_STATUSES\.has\(task\.status\)\) \{[\s\S]*?if \(eventPageHasMore && !eventError && !eventsSkipped\) \{[\s\S]*?正在继续读取剩余阶段事件[\s\S]*?setTimeout\(\(\) => pollTaskSafely\(generation\), 0\)[\s\S]*?return;[\s\S]*?stopTaskPolling\(\)[\s\S]*?loadTaskRunResult/);
  assert.match(poller, /if \(eventError && isRetryableApiError\(eventError\) && !eventError\.responseContractError\) \{[\s\S]*?5 秒后自动重试[\s\S]*?setTimeout\(\(\) => pollTaskSafely\(generation\), 5000\)/);
  assert.match(poller, /if \(eventError\) \{[\s\S]*?Run 报告暂不加载[\s\S]*?return;[\s\S]*?if \(eventsSkipped\) \{[\s\S]*?Run 报告暂不加载[\s\S]*?return;[\s\S]*?loadTaskRunResult/,
    "阶段事件读取失败或被跳过时，不得提前读取 Run 报告");
  const refreshStart = app.indexOf('$("#refresh-task").addEventListener("click"');
  const refreshEnd = app.indexOf('$("#recent-analysis-search")', refreshStart);
  const refreshHandler = refreshStart >= 0 && refreshEnd > refreshStart ? app.slice(refreshStart, refreshEnd) : "";
  assert.ok(refreshHandler.includes("state.taskEventsUnavailable = false"), "手动刷新必须恢复事件读取");
  assert.ok(refreshHandler.includes("pollTaskSafely(generation)"), "手动刷新必须立即重新发起 Task/事件读取");
  assert.doesNotMatch(refreshHandler, /state\.taskEventCursor\s*=\s*0/, "手动重试应保留已推进的服务端游标，避免重新读取第一页");
  assert.equal((app.match(/state\.taskSequence = 0;\s*state\.taskEventCursor = 0;/g) || []).length, 3, "新建、恢复和清空任务时都重置独立事件游标");
});

test("分析 API 切换期间的 UI 清理异常不会中断配置应用", () => {
  const helper = app.match(/function safelyUpdateTaskContextUi\(label, update\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(helper, /try \{\s*update\(\);\s*\} catch \(error\)/);
  assert.match(helper, /部分界面清理失败[\s\S]*?catch/);

  const invalidateStart = app.indexOf("function invalidateActiveTaskForAnalysisBaseChange(");
  const invalidateEnd = app.indexOf("\nfunction applySettings(", invalidateStart);
  const invalidate = invalidateStart >= 0 && invalidateEnd > invalidateStart
    ? app.slice(invalidateStart, invalidateEnd) : "";
  assert.match(invalidate, /state\.taskId = "";[\s\S]*?safelyUpdateTaskContextUi\("清理任务详情失败：[\s\S]*?safelyUpdateTaskContextUi\("重绘阶段事件失败：[\s\S]*?safelyUpdateTaskContextUi\("重绘自选股卡片失败：/);
  assert.doesNotMatch(invalidate, /(^|\n)\s*renderTaskEvents\(\);|(^|\n)\s*renderTaskFlowSummary\(\);|(^|\n)\s*renderRecentTasks\(\);/);

  const applyStart = app.indexOf("function applySettings(");
  const applyEnd = app.indexOf("\nfunction saveSettingsFromForm(", applyStart);
  const apply = applyStart >= 0 && applyEnd > applyStart ? app.slice(applyStart, applyEnd) : "";
  assert.match(apply, /if \(analysisBaseChanged\) \{[\s\S]*?runSettingsApplyStep\("清理旧分析任务失败：", \(\) =>\s*invalidateActiveTaskForAnalysisBaseChange\(previousSettings\.analysisBase, nextSettings\.analysisBase\), failures\);/);
  assert.match(apply, /if \(marketConnectionChanged\) \{[\s\S]*?runSettingsApplyStep\("失效市场总览请求失败：", \(\) => marketOverview\.invalidateConnection\(\), failures\);[\s\S]*?runSettingsApplyStep\("失效市场活动请求失败：", \(\) => marketActivity\.invalidateConnection\(\), failures\);/);
  assert.match(apply, /if \(analysisBaseChanged \|\| marketConnectionChanged\) \{[\s\S]*?runSettingsApplyStep\("失效数据中心检查结果失败：", \(\) =>\s*dataCenter\.invalidate\(/);
  assert.match(apply, /runSettingsApplyStep\("重置连接配置表单失败：", \(\) => \{/);
  assert.match(apply, /initialConnectionDraft = null;[\s\S]*?syncConnectionDraftState\(\)/);
});

test("任务切换和分析连接切换会重置事件游标并隔离迟到轮询", () => {
  const contexts = [
    ["new task creation", "async function createTask(event) {", "function startAnalysisForSymbol("],
    ["recent task restoration", "function activateTaskReference(record) {", "function restoreLastTask("],
    ["analysis API change", "function invalidateActiveTaskForAnalysisBaseChange(", "function applySettings("]
  ];
  for (const [label, startMarker, endMarker] of contexts) {
    const start = app.indexOf(startMarker);
    const end = app.indexOf(endMarker, start + startMarker.length);
    assert.ok(start >= 0 && end > start, label + " state-reset function must exist");
    const body = app.slice(start, end);
    assert.match(body, /state\.taskSequence = 0;\s*state\.taskEventCursor = 0;\s*state\.taskEvents\.clear\(\)/, label + " resets both event cursors and stored events");
  }
  assert.match(app, /function stopTaskPolling\(\) \{[\s\S]*?state\.pollGeneration \+= 1/);
  assert.match(app, /if \(generation !== state\.pollGeneration \|\| taskId !== state\.taskId \|\| endpointGeneration !== state\.analysisConnectionGeneration \|\| state\.taskAnalysisBase !== analysisBase \|\| state\.settings\.analysisBase !== analysisBase\) return;/);
});

 
test("任务状态回包不能替换任务标的集合，Run 与任务复用同一规范化比较", () => {
  assert.match(app, /function sameTaskSymbolSet\(expectedRaw, actualRaw\)/);
  assert.match(app, /candidate\.symbols !== undefined && !Array\.isArray\(candidate\.symbols\)/);
  assert.match(app, /Array\.isArray\(candidate\.symbols\) && state\.taskSymbols\.length &&[\s\S]*?!sameTaskSymbolSet\(state\.taskSymbols, candidate\.symbols\)/);
  assert.match(app, /任务状态接口返回的标的列表与当前任务不一致，已忽略/);
  assert.match(app, /function runResultSymbolMismatch\(result\)[\s\S]*?sameTaskSymbolSet\(expectedRaw, actualRaw\)/);
});
test("switching task context or analysis API endpoint resets the stage-event filter", () => {
  const contexts = [
    ["recent task activation", "function activateTaskReference(record) {", "function restoreLastTask("],
    ["task creation", "async function createTask(event) {", "function startAnalysisForSymbol("],
    ["analysis API endpoint change", "function invalidateActiveTaskForAnalysisBaseChange(", "function applySettings("]
  ];
  for (const [label, startMarker, endMarker] of contexts) {
    const start = app.indexOf(startMarker);
    const end = app.indexOf(endMarker, start + startMarker.length);
    assert.ok(start >= 0 && end > start, label + " state-reset function must exist");
    const body = app.slice(start, end);
    assert.match(
      body,
      /state\.taskEvents\.clear\(\);\s*state\.taskEventFilter = "all";\s*state\.taskEventsLoaded = false;/,
      label + " must clear prior task events and restore the default filter"
    );
  }
});


test("工作区导入与自选股存储使用同一规则识别带后缀的重复证券", () => {
  assert.match(format, /export function comparableMarketSymbol\(input\)/);
  const importBody = app.match(/function validateBrowserDataPayload\(payload\) \{[\s\S]*?\n\}/)?.[0] || "";
  assert.ok(importBody, "工作区数据导入必须先完成结构校验");
  assert.match(importBody, /const symbolIdentity = comparableMarketSymbol\(symbol\)/);
  assert.match(importBody, /seenSymbols\.has\(symbolIdentity\)/);
  assert.match(importBody, /自选股文件包含重复证券/);
  assert.match(watchlistModule, /import \{ comparableMarketSymbol \} from "\.\/format\.js"/);
  assert.match(watchlistModule, /const identity = comparableMarketSymbol\(normalized\.symbol\)/);
  assert.match(watchlistModule, /seen\.has\(identity\)/);
});


test("清空或导入本地任务历史会隔离在途创建请求的晚到响应", () => {
  assert.match(app, /taskHistoryGeneration: 0/);
  const createStart = app.indexOf("async function createTask(event) {");
  const createEnd = app.indexOf("function startAnalysisForSymbol(", createStart + 1);
  const creator = createStart >= 0 && createEnd > createStart ? app.slice(createStart, createEnd) : "";
  assert.ok(creator, "任务创建方法必须存在");
  assert.match(creator, /creationHistoryGeneration = state\.taskHistoryGeneration;/);
  assert.match(creator, /const taskHistoryWasReplaced = creationHistoryGeneration !== state\.taskHistoryGeneration;/);
  assert.match(creator, /if \(taskHistoryWasReplaced\) \{[\s\S]*?未重新写入该引用[\s\S]*?\} else \{[\s\S]*?persistRecentTaskHistory\(endpointRecord\)/);
  const clearStart = app.indexOf("function clearRecentTaskHistory() {");
  const clearEnd = app.indexOf("function setWatchlistFeedback(", clearStart + 1);
  const clearBody = clearStart >= 0 && clearEnd > clearStart ? app.slice(clearStart, clearEnd) : "";
  assert.match(clearBody, /state\.taskHistoryGeneration \+= 1;\s*state\.taskContextGeneration \+= 1;/);
  const importBody = sourceBetween(
    app,
    "async function applyBrowserDataImport(imported, currentWatchlist, currentRecentTasks, currentWatchlistGroups, strategy, previewStorageSnapshot) {",
    "\n\nsyncThemeControl(readTheme());",
    "工作区导入"
  );
  assert.match(importBody, /applySettings\(nextSettings, \{ persistPreviousActiveTask: false \}\);[\s\S]*?state\.taskHistoryGeneration \+= 1;\s*state\.taskContextGeneration \+= 1;/);
});

test("个股行情默认请求复用统一快照；旧服务仅在 404 时回退，显式日期范围尊重独立日 K 参数", () => {
  const start = app.indexOf("async function runQuoteQuery(event) {");
  const end = app.indexOf("\nfunction ", start + 20);
  const query = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(query, "主行情查询流程必须存在");

  const defaultRange = query.indexOf("if (!startDate && !endDate)");
  const bundleRequest = query.indexOf("bundleResponse = await getMarketBundle", defaultRange);
  assert.ok(defaultRange >= 0 && bundleRequest > defaultRange, "默认日期范围应先请求统一行情快照");

  const fallbackStart = query.indexOf("if (error.status === 404)", bundleRequest);
  const fallbackEnd = query.indexOf("if (bundleResponse)", fallbackStart);
  const fallback = query.slice(fallbackStart, fallbackEnd);
  assert.ok(fallbackStart > bundleRequest && fallbackEnd > fallbackStart);
  assert.match(fallback, /usedLegacyBundleFallback = true/);
  assert.match(fallback, /Promise\.allSettled\(\[[\s\S]*?getQuote\([\s\S]*?getDailyBars\(/);
  assert.doesNotMatch(fallback, /else\s*\{[\s\S]*?getQuote\(/, "Bundle 非 404 失败不能再次触发两个 Provider 请求");

  const explicitRange = query.indexOf("Bundle has no start/end query parameters");
  assert.ok(explicitRange >= 0, "显式日期范围应走独立报价与日 K 请求");
  const explicitQuery = query.slice(explicitRange);
  assert.match(explicitQuery, /getQuote\(state\.settings, symbol, controller\.signal\)/);
  assert.match(explicitQuery, /getDailyBars\(state\.settings, symbol, \{ limit, adjustment, start: startDate, end: endDate \}/);
});


test("数据中心读取并展示真实 Provider 运行诊断，且不将未观测标记为健康", () => {
  assert.match(apiClient, /export function getDataSourceDiagnostics\(settings, signal\)/);
  assert.match(dataCenter, /runHealthCheck\(getDataSourceDiagnostics, settings, 6000, controller\.signal\)/);
  assert.match(dataCenter, /function renderProviderDiagnostics\(payload\)/);
  assert.match(dataCenter, /not_observed: "尚无业务观测"/);
  assert.match(dataCenter, /DIAGNOSTIC_STATUS\[status\]/);
  assert.match(dataCenter, /index_quotes: "指数行情"/);
  assert.match(dataCenter, /market_breadth: "市场广度"/);
  assert.match(dataCenter, /sector_rankings: "板块排行"/);
  assert.match(dataCenter, /不会测试同花顺 API Key 或请求上游/);
  assert.match(dataCenter, /累计成功/);
  assert.match(dataCenter, /累计失败/);
  assert.match(dataCenter, /连续失败/);
  assert.match(dataCenter, /运行诊断接口读取失败/);
  assert.match(dataCenter, /data-center-hithink-status/);
  assert.match(html, /MARKET_DATA_HITHINK_ENABLED/);
  assert.match(html, /id="data-center-hithink-title"/);
  assert.match(html, /MARKET_DATA_HITHINK_API_KEY/);
  assert.match(html, /id="data-center-diagnostics-list"/);
  assert.match(html, /主动探测由下方独立操作触发/);
  assert.match(html, /WebUI 不读取、不保存或回显第三方 API Key/);
  assert.match(html, /id="data-center-probe-run"/);
  assert.match(html, /id="data-center-probe-history-refresh"/);
  assert.match(html, /发送测试（待接入）/);
  assert.match(html, /通知渠道管理、订阅配置和发送测试接口/);
  assert.doesNotMatch(html, /当前没有逐 Provider \/ capability 的受控真实探测 API/);
});


test("自选股批量行情只请求当前本地清单并在连接变更时清除旧快照", () => {
  assert.match(apiClient, /export function getMarketQuotes\(settings, symbols, signal, \{ timeoutMs = 30000 \} = \{\}\)/);
  assert.match(apiClient, /timeoutMs = 30000/);
  assert.match(app, /function refreshWatchlistQuotes\(\)/);
  assert.match(app, /getMarketQuotes\(state\.settings, symbols, controller\.signal\)/);
  assert.match(app, /state\.watchlistQuotes = nextQuotes/);
  assert.match(app, /state\.watchlistQuoteSymbols = new Set\(requestedSymbols\)/);
  assert.match(app, /function syncWatchlistQuoteMembership\(records\)/);
  assert.match(app, /invalidateWatchlistQuotes\(\)/);
  assert.match(html, /id="refresh-watchlist-quotes"/);
  assert.match(html, /id="watchlist-quote-refresh-status"/);
});


test("运行流采用全屏双栏工作区，节点和质量诊断可折叠", () => {
  assert.match(html, /class="run-flow-workspace"/);
  assert.match(html, /class="run-flow-result-column"/);
  assert.match(html, /class="run-flow-event-inspector"/);
  assert.match(app, /expandedTaskEventSequences/);
  assert.match(app, /node\.addEventListener\("toggle"/);
  assert.match(taskResultView, /title === "数据质量与策略门禁"[\s\S]*?analysis-result-disclosure/);
  assert.match(stylesheet, /grid-template-columns:minmax\(300px,\.82fr\) minmax\(0,1\.18fr\)/);
  assert.match(stylesheet, /\.run-flow-node\[open\]/);
  assert.match(stylesheet, /@media \(max-width:900px\)[\s\S]*?run-flow-workspace/);
});

test("个股研究列表复用自选股批量行情并区分失败与旧快照", () => {
  const start = app.indexOf("function renderResearchStockList() {");
  const end = app.indexOf("\nfunction safeReadWatchlistState()", start);
  const renderer = app.slice(start, end);
  assert.match(renderer, /state\.watchlistQuotes\.get\(normalizeWatchlistQuoteSymbol\(item\.symbol\)\)/);
  assert.match(renderer, /const itemQuote = isCurrent \? quote : batchRecord\?\.quote/);
  assert.match(renderer, /batchQuoteFailed \? "行情失败" : "未加载"/);
  assert.match(renderer, /旧快照 · /);
  assert.match(renderer, /\["loading", "error"\]\.includes\(state\.watchlistQuoteStatus\)/);
  assert.match(renderer, /旧快照 · 刷新中/);
  assert.match(renderer, /最近一次批量刷新失败，当前保留此前成功快照/);
  assert.match(renderer, /formatResearchMA\(5, isCurrent\)/);
  assert.match(renderer, /formatResearchMA\(20, isCurrent\)/);
});

test("全市场分页行情只调用分页概览 API 并正确展示覆盖与质量状态", () => {
  assert.match(app, /import \{ bindMarketStocks \} from "\.\/market-stocks\.js"/);
  assert.match(app, /bindMarketStocks\([\s\S]*?selectStockFromMarket\(item, source, targetView\)/);
  assert.match(marketStocksModule, /getMarketStocks\(settings, \{/);
  assert.match(marketStocksModule, /coverage_ratio/);
  assert.match(marketStocksModule, /total_is_lower_bound/);
  assert.match(marketStocksModule, /item\?\.ma5/);
  assert.match(marketStocksModule, /item\?\.ma20/);
  assert.match(marketStocksModule, /quote\?\.quality_status/);
  assert.match(marketStocksModule, /controller\.signal\.aborted \|\| generation !== state\.generation/);
  assert.match(marketStocksModule, /state\.page \* state\.pageSize < total/);
  assert.match(app, /失效全市场分页行情失败：[\s\S]*?marketStocks\.invalidate\(\)/);
  assert.match(marketStocksModule, /qualityPill\.title = qualityReasons\.join/);
  assert.match(marketStocksModule, /prev\.disabled = busy \|\| state\.page <= 1/);
  assert.match(html, /id="market-stocks-page-size"/);
  assert.match(html, /id="market-stocks-adjustment"/);
  assert.match(html, /id="market-stocks-prev"/);
  assert.match(html, /id="market-stocks-next"/);
  assert.match(html, /当前页行情覆盖/);
  assert.match(apiClient, /export function getMarketStocks/);
  assert.match(marketOpenAPI, /coverage_ratio: \{type: number/);
  assert.match(marketOpenAPI, /total_is_lower_bound: \{type: boolean/);
});

test("市场股票与情绪历史 API 封装保持真实接口契约", () => {
  assert.match(apiClient, /export function getMarketStocks/);
  assert.match(apiClient, /\/api\/v1\/market\/stocks\?/);
  assert.match(apiClient, /page_size: String\(normalizedPageSize\)/);
  assert.match(apiClient, /export function getMarketSentiment\(/);
  assert.match(apiClient, /\/api\/v1\/market\/sentiment/);
  assert.match(apiClient, /export function getMarketSentimentHistory/);
  assert.match(apiClient, /\/api\/v1\/market\/sentiment\/history\?/);
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/market\/stocks"/);
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/market\/sentiment"/);
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/market\/sentiment\/history"/);
  assert.match(marketOpenAPI, /\/api\/v1\/market\/stocks:/);
  assert.match(marketOpenAPI, /\/api\/v1\/market\/sentiment:/);
  assert.match(marketOpenAPI, /\/api\/v1\/market\/sentiment\/history:/);
  assert.match(html, /id="market-sentiment-history-start"/);
  assert.match(html, /id="market-sentiment-history-end"/);
  assert.match(app, /getMarketSentimentHistory/);
  assert.match(app, /不会用当前行情补造历史/);
});

test("工作区备份必须包含自选股分组并在导入事务中回读", () => {
  assert.match(app, /watchlist_groups:/);
  assert.match(app, /normalizeWatchlistGroups\(data\.watchlist_groups\)/);
  assert.match(app, /\[WATCHLIST_GROUPS_KEY, JSON\.stringify\(imported\.watchlistGroups\)\]/);
  assert.match(app, /syncWatchlistGroupControls\(\);[\s\S]*?renderWatchlist\(\)/);
});

test("设置中心导航、区块和本地配置交互保持一致", () => {
  const targets = [...html.matchAll(/data-settings-target="([^"]+)"/g)].map((match) => match[1]);
  const sections = [...html.matchAll(/<section[^>]*class="[^"]*settings-section[^"]*"[^>]*id="([^"]+)"/g)].map((match) => match[1]);
  assert.equal(targets.length, 10, "设置中心应保留 10 个已规划分组");
  assert.deepEqual([...targets].sort(), [...sections].sort(), "每个设置导航必须唯一对应一个内容区块");
  assert.equal(new Set(targets).size, targets.length, "设置导航目标不能重复");
  assert.doesNotMatch(html, /<\/(?:section|div|main|form|nav|article|aside|footer)>>/, "HTML 不能包含多余的闭合标签字符");
  assert.match(app, /resetPreferences\(\)/, "恢复本地偏好必须调用统一的本地偏好存储模块");
  assert.match(app, /bindNotificationPreferences\(/, "通知本地偏好必须通过独立模块绑定");
  assert.match(html, /保存本地偏好/);
  assert.match(html, /保存渠道（待接入）/);
  assert.match(html, /发送测试（待接入）/);
});

test("未接入的服务端设置必须保持禁用并明确配置边界", () => {
  const targets = [
    "settings-data-source-section", "settings-ai-section",
    "settings-scheduler-section", "settings-screening-section", "settings-security-section"
  ];
  for (const target of targets) {
    const start = html.indexOf('id="' + target + '"');
    const end = html.indexOf("</section>", start);
    assert.ok(start >= 0 && end > start, target + " must exist");
    const section = html.slice(start, end);
    assert.match(section, /待接入|只读/);
    const controls = [...section.matchAll(/<(?:input|select)\b[^>]*>/g)].map(match => match[0]);
    assert.ok(controls.length > 0, target + " should expose its planned controls");
    for (const control of controls) assert.match(control, /\bdisabled(?:\s|>|\/)/, target + " must not accept fake browser-side server configuration");
  }
  assert.match(html, /不能仅在浏览器隐藏数据源来代表服务端停用/);
  assert.match(html, /凭据不能写入浏览器 localStorage/);
  assert.match(html, /没有周期调度 CRUD/);
  assert.match(html, /不会伪造候选结果或扫描记录/);
  assert.match(html, /管理员登录态与行情服务 API Key 是两套独立机制/);
});

test("行情服务接口文档、Handler 与 WebUI 调用边界明确区分", () => {
  assert.match(marketHTTPServer, /mux\.HandleFunc\("GET \/health\/live"/);
  assert.match(marketHTTPServer, /mux\.HandleFunc\("GET \/health\/ready"/);
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/data-sources\/diagnostics"/);
  assert.match(marketHTTPServer, /mux\.Handle\("POST \/api\/v1\/data-sources\/probe"/);
  assert.match(apiClient, /getDataSourceDiagnostics/);
  assert.match(apiClient, /export function runDataSourceProbe/, "主动探测必须通过独立 API 客户端显式调用");
  assert.match(apiClient, /export function getDataSourceProbeHistory/, "探测历史必须使用独立只读接口");
  assert.match(dataCenter, /probeButton\?\.addEventListener\("click", \(\) => \{ void runProviderProbe\(\); \}\)/,
    "只有用户点击主动探测按钮才发起 Provider 主动探测");
  const passiveDataCenterCheck = dataCenter.slice(dataCenter.indexOf("async function refresh()"), dataCenter.indexOf("function invalidate("));
  assert.doesNotMatch(passiveDataCenterCheck, /runDataSourceProbe/,
    "普通数据中心检查不得隐式触发 Provider 上游探测");
  assert.match(marketOpenAPI, /\/api\/v1\/market\/capital-flow:/, "OpenAPI 当前声明了市场级资金流接口");
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/market\/capital-flow"/, "市场级资金流 Handler 已在合并后的数据分支注册");
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/stocks\/\{symbol\}\/capital-flow"/, "个股资金流 Handler 已注册");
  assert.match(apiClient, /export function getCapitalFlow/);
  assert.match(apiClient, /export function getMarketCapitalFlow/, "已注册的市场级资金流接口应由 WebUI 显式封装");
});

test("告警草稿与通知偏好跨标签页同步，并在编辑冲突时阻止旧数据覆盖", () => {
  assert.match(appSource, /const localAlertDrafts = bindLocalAlertDrafts/);
  assert.match(appSource, /const notificationPreferences = bindNotificationPreferences/);
  assert.match(appSource, /localAlertDrafts\?\.handleStorageChange\(event\)/);
  assert.match(appSource, /notificationPreferences\?\.handleStorageChange\(event\)/);
  assert.match(localAlertDrafts, /handleStorageChange\(event\)/);
  assert.match(localAlertDrafts, /itemSnapshot\(latest\) !== editingSnapshot/);
  assert.match(localAlertDrafts, /当前编辑未保存/);
  assert.match(notificationPreferences, /canonical\(current\.value\) !== baselineSnapshot/);
  assert.match(notificationPreferences, /当前表单已保留且未保存/);
  assert.match(notificationPreferences, /if \(isDirty\(\)\)/);
});


test("run-flow event node builder only constructs DOM and leaves state/event wiring to app", () => {
  assert.match(appSource, /buildTaskEventNode\(\{[\s\S]*?expanded: state\.expandedTaskEventSequences\.has\(sequence\)/);
  assert.match(appSource, /node\.addEventListener\("toggle"/);
  assert.match(appSource, /state\.selectedTaskEventSequence = Number\(row\.dataset\.taskEventSequence\)/);
  assert.match(taskEventNode, /export function buildTaskEventNode\(/);
  assert.match(taskEventNode, /node\.dataset\.eventSequence = String\(item\.sequence\)/);
  assert.match(taskEventNode, /node\.setAttribute\("aria-pressed", String\(selected\)\)/);
  assert.match(taskEventNode, /node\.open = selected \|\| expanded/);
  assert.doesNotMatch(taskEventNode, /\bstate\.|localStorage|addEventListener|querySelector|fetch\(/);
  assert.match(taskEventNode, /textContent = event\.message \|\| "服务端事件未提供描述"/);
  assert.match(taskEventNode, /diagnostic\.textContent = "错误代码：" \+ String\(event\.error_code\)/);
});


test("导入偏好规范化由 preferences 领域模块负责，不留在 app 协调层", async () => {
  const preferencesModule = await readFile(new URL("../src/preferences.js", import.meta.url), "utf8");
  assert.match(preferencesModule, /export function normalizeImportedPreferences\(/);
  assert.match(appSource, /normalizeImportedPreferences/);
  assert.doesNotMatch(appSource, /function normalizeImportedPreferences\(/);
});


test("偏好控件同步由独立 DOM adapter 管理，持久化仍留在 app 协调层", async () => {
  const preferencesView = await readFile(new URL("../src/settings-preferences-view.js", import.meta.url), "utf8");
  assert.match(appSource, /syncPreferencesControls\(document, preferences\)/);
  assert.match(preferencesView, /export function syncPreferencesControls\(/);
  assert.doesNotMatch(preferencesView, /localStorage|savePreferences|fetch\(|\bstate\./);
  assert.doesNotMatch(preferencesView, /\$\(/);
  assert.match(appSource, /function persistLocalPreferences\(/);
});


test("task event list projection is pure and app retains DOM/state coordination", () => {
  assert.match(appSource, /buildTaskEventListViewModel\(/);
  assert.match(appSource, /node\.addEventListener\("toggle"/);
  assert.match(taskEventListViewModel, /export function buildTaskEventListViewModel\(/);
  assert.doesNotMatch(taskEventListViewModel, /\bstate\.|localStorage|document\.|addEventListener|fetch\(/);
});


test("task flow summary projection is pure and render adapter only applies its output", async () => {
  const summaryViewModel = await readFile(new URL("../src/task-flow-summary-view-model.js", import.meta.url), "utf8");
  assert.match(appSource, /buildTaskFlowSummaryViewModel\(/);
  assert.match(appSource, /quality\.className = "run-flow-summary-value " \+ viewModel\.qualityClass/);
  assert.match(summaryViewModel, /export function buildTaskFlowSummaryViewModel\(/);
  assert.doesNotMatch(summaryViewModel, /\bstate\.|localStorage|document\.|addEventListener|fetch\(/);
});


test("section navigation is a closed DOM behavior module and app only configures it", async () => {
  const sectionNavigation = await readFile(new URL("../src/section-navigation.js", import.meta.url), "utf8");
  assert.match(appSource, /setupSectionNavigation\(\{ document, window, IntersectionObserver: window\.IntersectionObserver, linkSelector:/);
  assert.doesNotMatch(appSource, /function bindSectionNavigation\(/);
  assert.match(sectionNavigation, /export function setupSectionNavigation\(/);
  assert.doesNotMatch(sectionNavigation, /\bstate\.|localStorage|fetch\(|\$\(/);
});


test("UIR-2 基础布局间距使用语义 token", () => {
  assert.match(stylesheet, /--space-1:\s*4px/);
  assert.match(stylesheet, /--space-2:\s*8px/);
  assert.match(stylesheet, /--space-3:\s*12px/);
  assert.match(stylesheet, /--space-4:\s*16px/);
  assert.match(stylesheet, /--space-6:\s*24px/);
  assert.match(stylesheet, /\.sidebar\s*\{[^}]*padding:\s*var\(--space-6\) 15px 17px/);
  assert.match(stylesheet, /\.search-panel\s*\{[^}]*gap:\s*var\(--space-6\)/);
  assert.match(stylesheet, /\.button\s*\{[^}]*gap:\s*var\(--space-2\)/);
  assert.match(stylesheet, /\.metric-grid\s*\{[^}]*gap:\s*var\(--space-3\)/);
  assert.match(stylesheet, /\.content-grid\s*\{[^}]*margin-top:\s*var\(--space-4\)/);
});


test("UIR-2 typography roles use semantic tokens without changing their baseline values", () => {
  assert.match(stylesheet, /--font-size-support:\s*12px/);
  assert.match(stylesheet, /--font-size-secondary:\s*13px/);
  assert.match(stylesheet, /--font-size-control:\s*13px/);
  assert.match(stylesheet, /--font-size-input:\s*14px/);
  assert.match(stylesheet, /\.page-subtitle \{ font-size: var\(--font-size-secondary\) !important; \}/);
  assert.match(baseCssDeclarationBlock(".panel-muted"), /font-size:\s*12px\s*;/);
  assert.doesNotMatch(baseCssDeclarationBlock(".panel-muted"), /!important/);
  assert.match(stylesheet, /\.button \{ font-size: var\(--font-size-control\) !important; \}/);
  assert.match(stylesheet, /\.field input, \.field select, \.field textarea, \.recent-analysis-search input \{ font-size: var\(--font-size-input\); \}/);
});


test("UIR-2 全局减少动态效果规则只保留一份", () => {
  const globalMotionRules = stylesheet.match(/\*,\s*\*::before,\s*\*::after\s*\{/g) || [];
  assert.equal(globalMotionRules.length, 1, "global reduced-motion declarations should have one authoritative rule");
  assert.match(stylesheet, /@media \(prefers-reduced-motion: reduce\) \{\s*\*, \*::before, \*::after \{[\s\S]*?scroll-behavior: auto !important;[\s\S]*?animation-duration: \.01ms !important;[\s\S]*?animation-iteration-count: 1 !important;[\s\S]*?transition-duration: \.01ms !important;/);
});

test("UIR-2 正文与常规控件使用统一的最终计算基线", () => {
  assert.ok(stylesheet.includes("--control-height-base: 40px;"));
  assert.ok(stylesheet.includes("--font-size-body-readable: 14px;"));
  assert.ok(stylesheet.includes("--line-height-body: 1.55;"));
  assert.ok(stylesheet.includes("font-size: var(--font-size-body-readable);"));
  assert.ok(stylesheet.includes("line-height: var(--line-height-body);"));
  assert.ok(stylesheet.includes(".button {") && stylesheet.includes("min-height: var(--control-height-base);"));
  assert.ok(stylesheet.includes(".field input,") && stylesheet.includes("min-height: var(--control-height-base);"));
  assert.ok(stylesheet.includes(".settings-dialog .field input { min-height: var(--control-height-base); }"));
});


test("UIR-2 shared page frame rules merge only redundant global overrides", () => {
  const newline = String.fromCharCode(10);
  assert.ok(stylesheet.includes(".main-area { min-height: 100vh; margin-left: var(--layout-sidebar-width); padding: 0 var(--space-page); }"));
  assert.ok(stylesheet.includes(".view { width: 100%; max-width: 1680px; min-width: 0; margin-inline: auto; padding-top: var(--layout-page-top); padding-bottom: var(--layout-page-bottom); }"));
  assert.ok(stylesheet.includes([".panel {", "   min-width: 0;", "   border: 1px solid var(--border);"].join(newline)));
  assert.ok(!stylesheet.includes([".main-area {", "  padding-left: var(--space-page);"].join(newline)));
  assert.ok(!stylesheet.includes([".view {", "  max-width: 1680px;", "  margin-inline: auto;"].join(newline)));
  assert.ok(!stylesheet.includes([".panel {", "  box-shadow: 0 8px 26px rgba(0, 0, 0, .10);"].join(newline)));
  assert.ok(stylesheet.includes("--space-page: clamp(16px, 2.1vw, 30px);"));
  assert.ok(stylesheet.includes("transition: border-color .16s ease, box-shadow .16s ease;"));
});

test("UIR-2 panel heading base rules include their final shared layout constraints", () => {
  const newline = String.fromCharCode(10);
  assert.ok(stylesheet.includes(["  gap: 14px;", "  min-width: 0;"].join(newline)));
  assert.ok(stylesheet.includes(["  letter-spacing: -.2px;", "  line-height: 1.4;", "  overflow-wrap: anywhere;"].join(newline)));
  assert.ok(!stylesheet.includes(".panel-heading { min-width: 0; }"));
  assert.ok(!stylesheet.includes([".panel-heading {", "  gap: 14px;", "}"].join(newline)));
  assert.ok(!stylesheet.includes([".panel-heading h2 {", "  letter-spacing: -.2px;", "}"].join(newline)));
  assert.ok(!stylesheet.includes([".panel-heading h2 {", "  min-width: 0;", "  line-height: 1.4;"].join(newline)));
});

test("UIR-2 settings section heading final values are consolidated", () => {
  const newline = String.fromCharCode(10);
  assert.ok(stylesheet.includes([".settings-section-heading {", "  display: flex;", "  align-items: center;", "  justify-content: space-between;", "  flex-wrap: wrap;", "  gap: 14px;", "  margin: 0 0 11px;", "}"].join(newline)));
  assert.ok(stylesheet.includes([".settings-section-heading h2 {", "  margin: 0 0 5px;", "  color: var(--text);", "  font-size: 17px;", "}"].join(newline)));
  assert.ok(stylesheet.includes([".settings-section-heading p {", "  font-size: 12px;", "  line-height: 1.65;", "}"].join(newline)));
  assert.ok(stylesheet.includes(".settings-section { margin: 0 0 22px; }"));
  assert.ok(!stylesheet.includes(".settings-section-heading { display:flex;"));
  assert.ok(!stylesheet.includes(".settings-section-heading { align-items:center; }"));
  assert.ok(!stylesheet.includes(".settings-section { margin-bottom:22px; }"));
  assert.ok(stylesheet.includes("@media (max-width:640px) {"));
  assert.ok(stylesheet.includes(".settings-section-heading { align-items:flex-start; }"));
});

test("UIR-2 text button height is consolidated with responsive and task-action exceptions", () => {
  const newline = String.fromCharCode(10);
  assert.ok(stylesheet.includes(".text-button { min-height: 32px;"));
  assert.ok(!stylesheet.includes([".text-button {", "  min-height: 32px;", "  font-size: 12px;", "}"].join(newline)));
  assert.ok(stylesheet.includes(["  .text-button {", "    min-height: 36px;", "  }"].join(newline)));
  assert.ok(stylesheet.includes(".task-result-actions .text-button { min-height: 30px; font-size: 11px; }"));
  assert.doesNotMatch(stylesheet, /\.task-result-actions \.text-button \{[^}]*!important/);
  assert.ok(!stylesheet.includes(".task-result-actions .text-button { font-size: 13px !important; }"));
});

test("UIR-2 settings feedback layout constraints are consolidated", () => {
  assert.ok(stylesheet.includes(".settings-feedback { min-height: 1.5em; margin: 9px 0 0; overflow-wrap: anywhere;"));
  assert.ok(!stylesheet.includes(".settings-feedback { min-height: 1.5em; overflow-wrap: anywhere; }"));
  assert.ok(stylesheet.includes(".settings-connection-form .settings-feedback { margin: 0; }"));
  assert.match(baseCssDeclarationBlock(".settings-feedback"), /font-size:\s*12px\s*;/);
  assert.doesNotMatch(baseCssDeclarationBlock(".settings-feedback"), /!important/);
  assert.ok(stylesheet.includes(":root[data-theme=\"light\"] .settings-feedback"));
  const inlineStatus = exactCssDeclarationBlock(".settings-inline-status");
  assert.match(inlineStatus, /margin:\s*11px 0 0\s*;/);
  assert.match(inlineStatus, /color:\s*var\(--text-dim\)\s*;/);
  assert.match(inlineStatus, /font-size:\s*12px\s*;/);
  assert.doesNotMatch(inlineStatus, /!important/);
  assert.match(stylesheet, /\.settings-browser-data-panel > \.settings-inline-status,\s*\.settings-browser-data-panel > \.field-hint \{[^}]*margin:\s*0\s*;/, "browser data panel status and field hint share the zero-margin grid rule");
  const browserErrorStatus = exactCssDeclarationBlock(".settings-browser-data-panel .settings-inline-status.is-error");
  assert.match(browserErrorStatus, /color:\s*var\(--danger\)\s*;/);
});


test("UIR-2 图表 HTML 与 SVG 文字字号归并到组件基础规则", () => {
  const expectedSizes = [
    [".chart-legend", /font-size:\s*12px\s*;/],
    [".chart-summary span", /font-size:\s*12px\s*;/],
    [".chart-axis-label", /font-size:\s*12px\s*;/],
    [".chart-empty", /font-size:\s*13px\s*;/],
    [".chart-foot", /font-size:\s*12px\s*;/],
    [".chart-view-button", /font-size:\s*13px\s*;/],
    [".chart-volume-unavailable", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.match(stylesheet, /:root\[data-theme="light"\] \.chart-axis-label, :root\[data-theme="light"\] \.chart-volume-unavailable \{ fill: var\(--text-dim\); \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.chart-view-button\.is-active/);
  assert.match(stylesheet, /@media \(max-width: 800px\) \{[\s\S]*?\.chart-heading-actions/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.chart-view-button \{ flex: 1 1 50%; \}/);
});


test("UIR-2 侧栏品牌、分组标签与服务状态文字使用组件基础字号", () => {
  const expectedSizes = [
    [".brand-copy small", /font-size:\s*12px\s*;/],
    [".side-label", /font-size:\s*12px\s*;/],
    [".mini-service strong", /font-size:\s*13px\s*;/],
    [".mini-service small", /font-size:\s*12px\s*;/],
    [".sidebar-footnote", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.doesNotMatch(stylesheet, /:root\[data-theme="light"\] \.brand-copy small \{[^}]*font-size/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.side-label \{ color: var\(--text-dim\); \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.sidebar-footnote \{ color: var\(--text-dim\); \}/);
  assert.match(stylesheet, /@media \(max-width: 900px\) \{[\s\S]*?\.brand-copy, \.side-label[\s\S]*?\.sidebar-footnote \{ display: none; \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.sidebar \.side-label, \.sidebar \.sidebar-footnote \{ display: block; \}/);
});


test("UIR-2 表格元信息、质量标记与页面辅助区域使用组件基础字号", () => {
  const expectedSizes = [
    [".table-meta", /font-size:\s*12px\s*;/],
    [".mini-quality", /font-size:\s*12px\s*;/],
    [".table-footer", /font-size:\s*12px\s*;/],
    [".page-footer", /font-size:\s*12px\s*;/],
    [".soft-badge", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.chart-foot, \.table-footer, \.page-footer \{ align-items: flex-start; flex-direction: column; gap: 6px; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.soft-badge \{ color: #245d95; background: #eaf3ff; \}/);
});


test("UIR-2 任务空态、任务标识与边界说明使用组件基础字号", () => {
  const expectedSizes = [
    [".task-empty strong", /font-size:\s*13px\s*;/],
    [".task-empty span", /font-size:\s*12px\s*;/],
    [".task-id-row > span", /font-size:\s*12px\s*;/],
    [".task-id-row code", /font-size:\s*12px\s*;/],
    [".task-error", /font-size:\s*12px\s*;/],
    [".task-actions > span", /font-size:\s*12px\s*;/],
    [".boundary-grid span", /font-size:\s*12px\s*;/],
    [".boundary-grid strong", /font-size:\s*13px\s*;/],
    [".boundary-grid p", /font-size:\s*12px\s*;/],
    [".empty-row", /font-size:\s*12px\s*;/],
    [".inline-callout", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.match(stylesheet, /:root\[data-theme="light"\] \.task-error/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.inline-callout strong \{ color: #80540b; \}/);
  assert.match(stylesheet, /\.task-id-row code \{[^}]*overflow:\s*hidden;[^}]*text-overflow:\s*ellipsis;/);
  assert.match(stylesheet, /\.task-facts small \{[^}]*font-size:\s*9px;/, "Run Flow compact fact labels retain their component-specific sizing");
  assert.match(stylesheet, /\.run-flow-dialog \.task-facts strong \{\s*font-size:\s*12px;/, "Run Flow compact fact values retain their component-specific sizing");
});


test("UIR-2 最近分析搜索、反馈和移除操作使用组件基础字号", () => {
  const expectedSizes = [
    [".recent-analysis-search", /font-size:\s*13px\s*;/],
    [".recent-analysis-feedback", /font-size:\s*13px\s*;/],
    [".recent-analysis-remove", /font-size:\s*12px\s*;/],
    [".quote-heading-actions .button", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.match(stylesheet, /\.recent-analysis-search input \{[^}]*font: inherit;/, "search input keeps inheriting the label font family and size");
  assert.match(stylesheet, /\.recent-analysis-feedback\.is-error \{ color: #f0a2a9; \}/, "error feedback retains its semantic color");
  assert.match(stylesheet, /\.recent-analysis-open:focus-visible, \.recent-analysis-remove:focus-visible \{ outline: 2px solid #8fb6cf;/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.recent-analysis-tools \{ align-items: stretch; flex-direction: column; \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.quote-heading-actions \.button \{ padding: 0 9px; \}/);
});


test("UIR-2 Run Flow 事件标题与事件区辅助文字由组件选择器控制字号", () => {
  assert.match(stylesheet, /\.run-flow-dialog \.event-body strong \{[\s\S]*?font-size:\s*13px\s*;/);
  assert.match(stylesheet, /\.run-flow-dialog \.run-flow-events-column \.event-heading h3 \{\s*font-size:\s*14px\s*;/);
  assert.match(stylesheet, /\.run-flow-dialog \.run-flow-events-column \.event-heading span \{\s*color:\s*var\(--text-dim\);\s*font-size:\s*11px\s*;/);
  assert.doesNotMatch(stylesheet, /\.run-flow-dialog \.event-body strong \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.run-flow-dialog \.run-flow-events-column \.event-heading h3 \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.run-flow-dialog \.run-flow-events-column \.event-heading span \{[^}]*!important/);
  assert.match(stylesheet, /\.run-flow-dialog \.event-row-succeeded \.event-state-label,[\s\S]*?\.run-flow-dialog \.event-row-completed \.event-state-label \{[\s\S]*?color:\s*var\(--accent\);/);
  assert.match(stylesheet, /\.run-flow-dialog \.event-row-failed \.event-state-label,[\s\S]*?\.run-flow-dialog \.event-row-rejected \.event-state-label \{[\s\S]*?color:\s*var\(--danger\);/);
  assert.match(stylesheet, /\.run-flow-dialog \.event-row-running \.event-state-label,[\s\S]*?\.run-flow-dialog \.event-row-processing \.event-state-label \{[\s\S]*?color:\s*var\(--blue\);/);
  assert.match(stylesheet, /\.run-flow-dialog \.event-row-pending \.event-state-label \{[\s\S]*?color:\s*var\(--warning\);/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.run-flow-dialog \.event-heading \{ align-items: flex-start; flex-direction: column;/);
});


test("UIR-2 Provider 数据源卡片字号由组件基础规则控制", () => {
  const expectedSizes = [
    [".source-summary", /font-size:\s*12px\s*;/],
    [".source-version", /font-size:\s*12px\s*;/],
    [".source-main strong", /font-size:\s*13px\s*;/],
    [".source-main small", /font-size:\s*12px\s*;/],
    [".source-state", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.match(stylesheet, /:root\[data-theme="light"\] \.source-state-on, :root\[data-theme="light"\] \.mini-quality-valid \{ color: #08765e; background: #e3f6ef; \}/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.source-state-off, :root\[data-theme="light"\] \.mini-quality-unknown \{ color: #607383; background: #eef2f5; \}/);
  assert.match(stylesheet, /\.source-main strong \{[^}]*text-overflow:\s*ellipsis;[^}]*white-space:\s*nowrap;/);
});


test("UIR-2 行情质量标记与指标辅助文字由组件基础规则控制", () => {
  const expectedSizes = [
    [".quality-pill", /font-size:\s*12px\s*;/],
    [".metric-label", /font-size:\s*12px\s*;/],
    [".metric-foot", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.doesNotMatch(stylesheet, /\.quality-pill \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.metric-label \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.metric-foot \{[^}]*!important/);
  assert.match(stylesheet, /\.section-heading p, \.panel-heading p \{ font-size: var\(--font-size-support\) !important; \}/);
  assert.match(stylesheet, /\.quality-valid \{[^}]*color:[^}]*background:/);
  assert.match(stylesheet, /\.quality-degraded \{[^}]*color:[^}]*background:/);
  assert.match(stylesheet, /\.quality-stale \{[^}]*color:[^}]*background:/);
  assert.match(stylesheet, /\.quality-invalid \{[^}]*color:[^}]*background:/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.metric-grid \{ grid-template-columns: repeat\(2, minmax\(0, 1fr\)\);/);
});


test("UIR-2 页面眉题、查询辅助与设置提示字号由组件基础规则控制", () => {
  const expectedSizes = [
    [".eyebrow", /font-size:\s*12px\s*;/],
    [".panel-muted", /font-size:\s*12px\s*;/],
    [".query-hint", /font-size:\s*12px\s*;/],
    [".security-note", /font-size:\s*12px\s*;/],
    [".settings-feedback", /font-size:\s*12px\s*;/]
  ];
  for (const [selector, sizePattern] of expectedSizes) {
    const declaration = baseCssDeclarationBlock(selector);
    assert.match(declaration, sizePattern, selector + " must own its readable base font size");
    assert.doesNotMatch(declaration, /!important/);
  }
  assert.doesNotMatch(stylesheet, /\.eyebrow \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.panel-muted \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.query-hint \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.security-note \{[^}]*!important/);
  assert.doesNotMatch(stylesheet, /\.settings-feedback \{[^}]*!important/);
  assert.match(stylesheet, /\.settings-feedback \{[^}]*color: #f0a2a9/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.settings-feedback[^}]*color: #b92f41/);
  assert.match(stylesheet, /\.security-note \{[\s\S]*?color: #c0aa85/);
  assert.match(stylesheet, /\.query-hint \{[^}]*padding-left: 150px/);
  assert.match(stylesheet, /@media \(max-width: 1200px\) \{[\s\S]*?\.query-hint \{ padding-left: 0; \}/);
});


test("UIR-2 page timestamp and panel kicker typography use component baselines", () => {
  const lastUpdated = baseCssDeclarationBlock(".last-updated");
  const lastUpdatedValue = baseCssDeclarationBlock(".last-updated strong");
  const panelKicker = baseCssDeclarationBlock(".panel-kicker");
  assert.match(lastUpdated, /font-size:\s*12px\s*;/);
  assert.match(lastUpdatedValue, /font-size:\s*12px\s*;/);
  assert.match(panelKicker, /font-size:\s*var\(--font-size-secondary\)\s*;/);
  assert.doesNotMatch(lastUpdated, /!important/);
  assert.doesNotMatch(lastUpdatedValue, /!important/);
  assert.doesNotMatch(panelKicker, /!important/);
  assert.match(stylesheet, /@media \(max-width: 640px\) \{[\s\S]*?\.last-updated \{[\s\S]*?white-space: normal;/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.last-updated strong[^}]*color: var\(--text\)/);
  assert.match(stylesheet, /:root\[data-theme="light"\] h2, :root\[data-theme="light"\] \.panel-kicker/);
});;


test("UIR-2 form field labels use a readable component baseline without forced priority", () => {
  const field = baseCssDeclarationBlock(".field");
  assert.match(field, /font-size:\s*12px\s*;/);
  assert.doesNotMatch(field, /!important/);
  assert.match(stylesheet, /\.field > span \{\s*font-size: 12px;\s*line-height: 1\.5;/);
  assert.match(stylesheet, /:root\[data-theme="light"\] \.field \{ color: #536879; \}/);
});


test("UIR-2 table typography uses component baselines and scoped readable table overrides", () => {
  const header = baseCssDeclarationBlock("th");
  const cell = baseCssDeclarationBlock("td");
  assert.match(header, /font-size:\s*11px\s*;/);
  assert.match(header, /padding-block:\s*11px\s*;/);
  assert.doesNotMatch(header, /!important/);
  assert.match(cell, /font-size:\s*12px\s*;/);
  assert.match(cell, /line-height:\s*1\.5\s*;/);
  assert.doesNotMatch(cell, /!important/);
  assert.match(stylesheet, /\.table-scroll th \{ font-size: 12px; \}/);
  assert.match(stylesheet, /\.table-scroll td \{ font-size: 13px; \}/);
  assert.doesNotMatch(stylesheet, /(?:^|\n)th \{ font-size: 12px !important; \}/);
  assert.doesNotMatch(stylesheet, /(?:^|\n)td \{ font-size: 12px !important; \}/);
  assert.match(stylesheet, /\.watchlist-import-preview th, \.watchlist-import-preview td \{[^}]*font-size: 12px;/);
  assert.match(stylesheet, /@media \(max-width: 600px\)[\s\S]*?th, td \{ padding: 9px 10px; \}/);
});


test("UIR-2 table white-space behavior is inherited from the shared table baseline", () => {
  assert.match(stylesheet, /table \{[^}]*white-space: nowrap;/);
  assert.doesNotMatch(stylesheet, /\.market-overview-table-scroll th\s*,\s*\.market-overview-table-scroll td\s*\{[^}]*white-space\s*:/);
  assert.doesNotMatch(stylesheet, /\.research-list-table-scroll th\s*,\s*\.research-list-table-scroll td\s*\{[^}]*white-space\s*:/);
  assert.match(stylesheet, /\.market-overview-table-scroll td \{\s*vertical-align:middle\s*\}/);
  assert.match(stylesheet, /\.market-activity-table td \{\s*vertical-align:middle\s*\}/);
});


test("UIR-2 shared table scrollers contain horizontal chaining and enable touch momentum", () => {
  const tableScroller = baseCssDeclarationBlock(".table-scroll");
  assert.match(tableScroller, /overflow-x:\s*auto\s*;/);
  assert.match(tableScroller, /overscroll-behavior-x:\s*contain\s*;/);
  assert.match(tableScroller, /-webkit-overflow-scrolling:\s*touch\s*;/);
  assert.match(stylesheet, /\.market-activity-table \{\s*overscroll-behavior: contain;\s*\}/);
  assert.doesNotMatch(stylesheet, /\.market-activity-table \{[^}]*-webkit-overflow-scrolling:/);
});


test("UIR-2 table viewport constraints keep wide data inside local scrollers", () => {
  assert.match(stylesheet, /body \{[^}]*min-width:\s*320px/);
  assert.match(stylesheet, /\.main-area \{[^}]*min-width:\s*0/);
  assert.match(stylesheet, /\.view \{[^}]*width:\s*100%[^}]*min-width:\s*0/);
  assert.match(stylesheet, /\.panel \{[^}]*min-width:\s*0/);
  assert.match(stylesheet, /\.table-scroll \{[^}]*overflow-x:\s*auto/);
  assert.match(stylesheet, /\.market-overview-table-scroll \{[^}]*overflow:\s*auto/);
  assert.match(stylesheet, /\.market-overview-table-scroll table \{\s*min-width:\s*500px\s*\}/);
  assert.match(stylesheet, /\.market-stocks-table-scroll table\{min-width:980px\}/);
  assert.match(stylesheet, /\.existing-api-results-grid > section \{ min-width: 0;/);
  assert.match(stylesheet, /@media \(max-width: 900px\) \{ \.existing-api-results-grid \{ grid-template-columns: minmax\(0, 1fr\); \} \}/);
  assert.match(html, /class="table-scroll research-list-table-scroll"/);
  assert.match(html, /class="table-scroll market-stocks-table-scroll"/);
  assert.match(html, /class="existing-api-results-grid"/);
});


test("UIR-2 market stocks pagination controls wrap within narrow panels", () => {
  assert.match(stylesheet, /\.market-stocks-pagination\{[^}]*display:flex;[^}]*flex-wrap:wrap/);
  assert.match(stylesheet, /\.market-stocks-pagination>div\{[^}]*display:flex;[^}]*flex-wrap:wrap/);
  assert.match(html, /class="market-stocks-pagination"><span id="market-stocks-page-label"/);
  assert.match(html, /id="market-stocks-prev"/);
  assert.match(html, /id="market-stocks-next"/);
});


test("UIR-2 local alert draft text wraps inside flex cards", () => {
  assert.match(stylesheet, /\.local-alert-draft-card > div\s*\{[^}]*min-width:\s*0;[^}]*overflow-wrap:\s*anywhere;/);
  assert.match(stylesheet, /\.local-alert-draft-card h4,\s*\.local-alert-draft-card p\s*\{[^}]*overflow-wrap:\s*anywhere;/);
  assert.match(localAlertDrafts, /const copy = document\.createElement\("div"\);[\s\S]*?title\.textContent = item\.name;/);
  assert.match(localAlertDrafts, /name\.length > 80/);
});


test("UIR-2 dialogs and Run Flow contain scrolling and long content on narrow viewports", () => {
  assert.match(html, /<dialog class="run-flow-dialog" id="run-flow-dialog"/);
  assert.match(stylesheet, /\.settings-dialog\s*\{\s*max-height:\s*calc\(100vh - 24px\);\s*overflow-y:\s*auto;/);
  assert.match(stylesheet, /\.run-flow-dialog \{[^}]*position:\s*fixed;[^}]*inset:\s*2vh 2vw;[^}]*overflow:\s*hidden;/);
  assert.match(stylesheet, /\.run-flow-dialog \.task-status-panel \{[^}]*height:\s*100%;[^}]*max-height:\s*100%;[^}]*overflow-y:\s*auto;[^}]*overscroll-behavior:\s*contain;/);
  assert.match(stylesheet, /\.event-body p \{[^}]*overflow-wrap:\s*anywhere;/);
  assert.match(stylesheet, /\.raw-result pre \{[^}]*max-height:\s*320px;[^}]*overflow:\s*auto;[\s\S]*?white-space:\s*pre-wrap;[^}]*overflow-wrap:\s*anywhere;/);
  assert.match(stylesheet, /#close-chart \{[^}]*width:\s*100%;[^}]*height:\s*auto;/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.run-flow-header \{ flex-direction: column; \}[\s\S]*?\.run-flow-dialog \.task-facts \{ grid-template-columns: repeat\(2, minmax\(0, 1fr\)\); \}/);
});


test("UIR-2 narrow Run Flow workspace allocates remaining height to independent panes", () => {
  assert.match(stylesheet, /@media\s*\(max-width:\s*900px\)\s*\{[\s\S]*?\.run-flow-dialog \.run-flow-workspace\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\);[^}]*grid-template-rows:\s*minmax\(0,1fr\) minmax\(0,1\.2fr\);/);
  assert.match(stylesheet, /\.run-flow-dialog \.run-flow-events-column, \.run-flow-dialog \.run-flow-result-column\s*\{[^}]*max-height:\s*none;[^}]*min-height:\s*0;/);
  assert.doesNotMatch(stylesheet.slice(stylesheet.lastIndexOf("@media (max-width:900px)"), stylesheet.length), /\.run-flow-events-column\s*\{[^}]*max-height:\s*4[26]dvh/);
  assert.match(stylesheet, /\.browser-data-import-dialog\{[^}]*max-height:calc\(100dvh - 24px\);overflow-y:auto;/);
  assert.match(html, /<dialog class="browser-data-import-dialog" id="browser-data-import-dialog"/);
});

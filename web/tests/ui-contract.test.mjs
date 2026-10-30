import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [html, appSource, format, domain, openapi, recentTaskModule, stylesheet, watchlistModule, apiClient, taskQueue, marketActivity, stockResearch, marketOverview, serviceWiring, dataCenter, healthCheck, marketHTTPServer, marketOpenAPI, marketStocksModule] = await Promise.all([
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("../src/format.js", import.meta.url), "utf8"),
  readFile(new URL("../../internal/analysis/domain/types.go", import.meta.url), "utf8"),
  readFile(new URL("../../doc/接口规范/分析任务.openapi.yaml", import.meta.url), "utf8"),
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
  readFile(new URL("../src/market-stocks.js", import.meta.url), "utf8")
]);

// UI 控制器和功能模块分拆后，跨模块契约应检查实际参与运行的源码集合。
const app = [appSource, recentTaskModule, watchlistModule, marketOverview, marketActivity, stockResearch, dataCenter, healthCheck, marketStocksModule].join("
");

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
  assert.match(app, /const visibleTrackingNodes = \(preferences\.watchlistDisplayOrder/);
  assert.match(app, /updated: meta/);
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
  const mapper = taskQueue.match(/func taskStatusForRun\(status domain\.RunStatus\) string \{[\s\S]*?
\}/)?.[0] || "";
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
  const end = app.indexOf("
function ", start + 1);
  const restore = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(restore.includes('state.taskStatusLoading = true;'), "恢复任务期间标记状态待确认");
  assert.ok(restore.includes('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");'), "恢复期间不沿用上一条任务的状态徽标");
  assert.ok(restore.indexOf('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");') < restore.indexOf("void pollTaskSafely(state.pollGeneration);"), "先显示恢复中，再请求服务端");
});

test("failed Run refresh distinguishes the last verified same-Run result from a new read", () => {
  const start = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const end = app.indexOf("
function stopTaskPolling()", start);
  const loader = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(loader.includes("const retainedPreviousResult = Boolean(state.lastRunResult && state.lastRunResult.run_id === runId);"));
  assert.ok(loader.includes("页面保留的是此前已成功读取的同一 Run 结果，本次读取尚未确认。"));
  assert.ok(loader.includes("本次读取未成功，不代表结果已更新。"));
  assert.ok(loader.includes("renderTaskResult(result);"), "只有本次 Run 身份和标的校验通过才替换报告内容");
});

test("Run 结果重读采用单次请求锁并只由当前代际解锁按钮", () => {
  assert.match(app, /function setRunResultReloadBusy\(busy\) \{[\s\S]*?button\.disabled = Boolean\(busy\)[\s\S]*?button\.setAttribute\("aria-busy", "true"\)[\s\S]*?button\.textContent = "重新读取结果"/);
  const loaderStart = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const loaderEnd = app.indexOf("
function stopTaskPolling()", loaderStart);
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
  const watchlistEnd = app.indexOf("
function addCurrentSymbolToWatchlist(", watchlistStart);
  const watchlist = app.slice(watchlistStart, watchlistEnd);
  assert.match(watchlist, /readState === "loading"[\s\S]*?正在重读，尚未确认/);
  assert.match(watchlist, /readState === "retrying"[\s\S]*?重试中，尚未确认/);
  assert.match(watchlist, /readState === "error"[\s\S]*?本次读取失败/);
  const resultStart = app.indexOf("function renderTaskResult(result) {");
  const resultEnd = app.indexOf("
function ", resultStart + 10);
  assert.match(app.slice(resultStart, resultEnd), /state\.runResultReadStatus = "";[\s\S]*?state\.runResultReadError = ""/);
  const loaderStart = app.indexOf("async function loadTaskRunResult(runId, expectedTaskId) {");
  const loaderEnd = app.indexOf("
function stopTaskPolling()", loaderStart);
  const loader = app.slice(loaderStart, loaderEnd);
  assert.match(watchlist, /activeRunReadMatches[\s\S]*?当前 Run 结果：正在读取服务端结果/);
  assert.match(watchlist, /当前 Run 结果：暂时不可读，正在自动重试/);
  assert.match(watchlist, /当前 Run 结果：读取失败/);
  assert.match(loader, /state\.runResultReadStatus = "loading"/);
  assert.match(loader, /state\.runResultReadStatus = "retrying"[\s\S]*?if \(state\.view === "watchlist"\) renderWatchlist\(\)/);
  assert.match(loader, /state\.runResultReadStatus = "error"[\s\S]*?if \(state\.view === "watchlist"\) renderWatchlist\(\)/);
});

test("creation, contract confirmation, and history restoration use distinct task status labels", () => {
  const createStart = app.indexOf("async function createTask(event) {");
  const createEnd = app.indexOf("
function startAnalysisForSymbol(", createStart);
  const creator = app.slice(createStart, createEnd);
  assert.ok(creator.includes('setQualityPill($("#task-status-pill"), "unknown", "正在创建");'));
  assert.ok(creator.includes('setQualityPill($("#task-status-pill"), "unknown", "正在确认");'));
  assert.ok(!creator.includes('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");'));
  const restoreStart = app.indexOf("function activateTaskReference(record) {");
  const restoreEnd = app.indexOf("
function restoreLastTask(", restoreStart);
  assert.ok(app.slice(restoreStart, restoreEnd).includes('setQualityPill($("#task-status-pill"), "unknown", "正在恢复");'));
});

test("task restoration and manual refresh mark cached status as unconfirmed", () => {
  assert.ok(app.includes("taskStatusLoading: false"));
  const pollStart = app.indexOf("async function pollTask(");
  const pollEnd = app.indexOf("
async function createTask(", pollStart);
  const poller = app.slice(pollStart, pollEnd);
  assert.match(poller, /task = candidate;\s*state\.taskStatusLoading = false;\s*state\.taskStatusError = ""/);
  assert.match(poller, /if \(taskError\) \{\s*state\.taskStatusLoading = false;/);
  const watchStart = app.indexOf("function renderWatchlist(");
  const watchEnd = app.indexOf("
function addCurrentSymbolToWatchlist(", watchStart);
  const watchlist = app.slice(watchStart, watchEnd);
  assert.match(watchlist, /state\.taskStatusLoading \|\| Boolean\(state\.taskStatusError\)/);
  assert.match(watchlist, /正在确认服务端状态（上次状态：/);
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
  const end = app.indexOf('
});', start);
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
  const end = app.indexOf("
function shanghaiDateKey(", start);
  const refresh = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.match(refresh, /let button = null;[\s\S]*?let list = null;[\s\S]*?try \{/);
  assert.match(refresh, /showMessage\("正在读取当前连接配置对应的数据源能力…"/);
  assert.match(refresh, /try \{\s*if \(button\) \{\s*button\.disabled = true;[\s\S]*?catch \{/);
  assert.match(refresh, /const payload = await getDataSources\(settings\);[\s\S]*?generation !== sourceRefreshGeneration/);
  assert.match(refresh, /catch \(error\) \{\s*if \(generation !== sourceRefreshGeneration\) return;[\s\S]*?showMessage\(message, "empty-row error-text"\)/);
  assert.match(refresh, /finally \{[\s\S]*?if \(generation === sourceRefreshGeneration\) \{[\s\S]*?try \{[\s\S]*?button\.disabled = false;[\s\S]*?catch \{/);
});

test("已保存连接检查与草稿检查的初始化/解锁异常不会产生未处理 rejection", () => {
  const saved = app.match(/async function checkConnectionFromSettings\(\) \{[\s\S]*?
\}/)?.[0] || "";
  const draft = app.match(/async function checkDraftConnectionFromSettings\(\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.match(saved, /generation !== savedConnectionCheckGeneration/);
  assert.match(saved, /finally[\s\S]*?releaseSettingsCheckButtonSafely/);
  assert.match(draft, /generation !== draftConnectionCheckGeneration/);
  assert.match(draft, /finally[\s\S]*?releaseSettingsCheckButtonSafely/);
  assert.match(draft, /testedDraft = readConnectionDraft\(\)/);
});
test("applySettings 的单项清理失败不会阻断其余配置应用步骤", () => {
  const start = app.indexOf("function applySettings(");
  const end = app.indexOf("
function saveSettingsFromForm(", start);
  const apply = start >= 0 && end > start ? app.slice(start, end) : "";
  assert.ok(apply);
  assert.match(apply, /runSettingsApplyStep\("清理旧分析任务失败："/);
  assert.match(apply, /runSettingsApplyStep\("失效市场总览请求失败："/);
  assert.match(apply, /runSettingsApplyStep\("刷新服务健康状态失败："/);
  assert.match(apply, /reportSettingsApplyFailures\(failures\)/);
});
test("任务切换与分析 API 切换不会被按钮解锁异常中断", () => {
  assert.match(app, /function releaseRunResultReloadBusy\(\) \{[\s\S]*?try \{\s*setRunResultReloadBusy\(false\);\s*\} catch[\s\S]*?继续完成上下文切换/);

  const stop = app.match(/function stopTaskPolling\(\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.ok(stop, "停止轮询共用函数必须存在");
  assert.match(stop, /state\.pollGeneration \+= 1;[\s\S]*?try \{[\s\S]*?setTaskRefreshBusy\(false\);[\s\S]*?catch/);
  assert.match(stop, /const timer = state\.pollTimer;\s*state\.pollTimer = null/);

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
  const end = app.indexOf("
function restoreLastTask(", start);
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
  const safeEnd = app.indexOf("
async function createTask(", safeStart);
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
  const safeEnd = app.indexOf("
async function createTask(", safeStart);
  const safePoll = safeStart >= 0 && safeEnd > safeStart ? app.slice(safeStart, safeEnd) : "";
  assert.ok(safePoll.includes("return pollTask(generation).catch((error) => {"));
  assert.ok(safePoll.includes("generation !== state.pollGeneration || !state.taskId"));
  assert.ok(safePoll.includes('appendRecoveryError("状态徽标更新失败：", renderError);'));
  assert.ok(safePoll.includes('appendRecoveryError("轮询诊断更新失败：", renderError);'));
  assert.ok(safePoll.includes('appendRecoveryError("自选股卡片刷新失败：", renderError);'));
  assert.ok(safePoll.includes("异常收敛中的异常也不得形成新的未处理 Promise rejection"));
  assert.ok(safePoll.includes("try {
        stopTaskPolling();
      } catch {}"));
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
  const end = app.indexOf("
function stopTaskPolling()", start);
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
  const end = app.indexOf("
function stopTaskPolling()", start);
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
  const pollEnd = app.indexOf("
async function createTask(", pollStart);
  const poller = pollStart >= 0 && pollEnd > pollStart ? app.slice(pollStart, pollEnd) : "";
  assert.ok(poller.includes("state.taskStatusError = errorMessage(taskError);"), "任务状态读取失败时保存提示");
  assert.ok(poller.includes("state.taskStatusErrorRetrying = isRetryableApiError(taskError) && !taskError.responseContractError;"), "区分自动重试与永久暂停");
  assert.ok(poller.includes('state.taskStatusError = "";'), "下一次有效 Task 回包清除旧错误");
  assert.ok(poller.includes('setQualityPill($("#task-status-pill"), "unknown", state.taskStatusErrorRetrying ? "状态刷新失败" : "状态读取失败")'), "任务状态徽标不得继续冒充为已确认的 running/succeeded");
  const watchlistRenderer = app.slice(app.indexOf("function renderWatchlist("), app.indexOf("
function addCurrentSymbolToWatchlist("));
  assert.ok(watchlistRenderer.includes("状态读取异常（上次状态："), "自选卡片应将上次状态标为未验证");
  assert.ok(watchlistRenderer.includes("自动重试中") && watchlistRenderer.includes("自动刷新已暂停"), "自选卡片应区分临时错误与永久错误");
  const manualStart = app.indexOf('$("#refresh-task").addEventListener("click"');
  const manualEnd = app.indexOf('$("#recent-analysis-search")', manualStart);
  const manualRefresh = app.slice(manualStart, manualEnd);
  assert.ok(manualRefresh.includes("state.taskEventsUnavailable = false") && manualRefresh.includes("pollTaskSafely(generation)"), "手动刷新重新尝试任务与事件读取");
  assert.ok(manualRefresh.includes("正在手动刷新任务状态与阶段事件"), "手动刷新开始后应立即反馈，避免旧错误提示看起来未响应");
});

test("创建任务的前置 UI 异常也会释放提交锁和按钮", () => {
  const start = app.indexOf("async function createTask(event) {");
  const end = app.indexOf("
function startAnalysisForSymbol(", start);
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
  const createEnd = app.indexOf("
function startAnalysisForSymbol(", createStart);
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
  const createEnd = app.indexOf("
function startAnalysisForSymbol(", createStart);
  const creator = createStart >= 0 && createEnd > createStart ? app.slice(createStart, createEnd) : "";
  assert.match(creator, /state\.taskContextGeneration \+= 1;\s*creationContextGeneration = state\.taskContextGeneration;/);
  assert.match(creator, /if \(creationContextGeneration !== state\.taskContextGeneration\) \{[\s\S]*?persistRecentTaskHistory\(endpointRecord\)[\s\S]*?当前选中的任务不会被覆盖/);
  assert.match(creator, /catch \(error\) \{\s*try \{\s*if \(creationContextGeneration !== state\.taskContextGeneration\)/, "late create errors must not hide the currently selected task");
  const activateStart = app.indexOf("function activateTaskReference(record) {");
  const activateEnd = app.indexOf("
function restoreLastTask(", activateStart);
  assert.match(app.slice(activateStart, activateEnd), /state\.taskContextGeneration \+= 1;/);
  const invalidateStart = app.indexOf("function invalidateActiveTaskForAnalysisBaseChange(");
  const invalidateEnd = app.indexOf("
function applySettings(", invalidateStart);
  assert.match(app.slice(invalidateStart, invalidateEnd), /state\.taskContextGeneration \+= 1;/);
});

test("polling pauses on permanent API errors and Run reads use bounded, stale-safe retries", () => {
  assert.match(app, /if \(taskError && \(!isRetryableApiError\(taskError\) \|\| taskError\.responseContractError\)\) \{[\s\S]*?自动刷新已暂停[\s\S]*?return;[\s\S]*?state\.pollTimer = window\.setTimeout\(\(\) => pollTaskSafely\(generation\), 5000\);/);
  assert.match(app, /const RUN_RESULT_RETRY_DELAYS_MS = \[1000, 2500, 5000\]/);
  assert.match(app, /generation !== state\.runResultGeneration \|\| state\.taskId !== expectedTaskId \|\|\s*endpointGeneration !== state\.analysisConnectionGeneration/);
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
  const researchOverviewRenderer = app.match(/function renderResearchOverview\(\) \{[\s\S]*?
\}/)?.[0] || "";
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
  const restoreEnd = app.indexOf("
function ", restoreStart + 1);
  const restoreTask = restoreStart >= 0 && restoreEnd > restoreStart ? app.slice(restoreStart, restoreEnd) : "";
  assert.ok(restoreTask.includes("state.taskEvents.clear();"), "恢复任务前清空上一任务事件");
  assert.ok(restoreTask.includes("state.selectedTaskEventSequence = null;"), "恢复任务时清空旧事件选中序号");
  assert.ok(restoreTask.indexOf("state.selectedTaskEventSequence = null;") < restoreTask.indexOf("renderTaskEvents();"), "先重置事件选择，再渲染新任务事件");
});

test("watchlist labels task history from a different analysis API address", () => {
  assert.ok(app.includes('const isOtherAnalysisBase = (task) => Boolean(task && String(task.analysis_base || state.settings.analysisBase) !== String(state.settings.analysisBase));'));
  assert.ok(app.includes('todayLine.textContent = "今日分析：有本地记录" + (todayTaskUsesOtherEndpoint ? "（旧分析 API 地址）" : "")'));
  assert.ok(app.includes('(recentTaskUsesOtherEndpoint ? " · 旧分析 API 地址" : "")'));
  const todayTaskDeclaration = app.indexOf("const todayTask = [...matchingTasks]");
  const todayEndpointUse = app.indexOf("const todayTaskUsesOtherEndpoint = isOtherAnalysisBase(todayTask)");
  assert.ok(todayTaskDeclaration >= 0 && todayTaskDeclaration < todayEndpointUse, "必须先计算 todayTask 再读取其来源地址，避免暂时性死区");
});

test("watchlist task and Run summaries refresh when background state changes", () => {
  const taskRenderer = app.match(/function renderTask\(task\) \{[\s\S]*?
\}/)?.[0] || "";
  const runRenderer = app.match(/function renderTaskResult\(result\) \{[\s\S]*?
\}/)?.[0] || "";
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
  const end = marketOverview.indexOf("
 }", start);
  const body = start >= 0 && end > start ? marketOverview.slice(start, end) : "";
  assert.ok(body.includes("KEYS.forEach(key=>{"));
  assert.ok(body.includes("safeOverviewUi(()=>cancel(key));"), "连接变更应通过安全入口中止各市场模块请求并推进 generation");
  assert.ok(body.includes('safeOverviewUi(()=>pending(key,"行情连接配置已变更，旧结果已清除，请重新刷新。","warning"));'));
  assert.ok(body.includes("refreshGenerations[selector]+=1;"), "作废旧刷新摘要回写");
  assert.ok(body.indexOf("Object.keys(refreshGenerations)") < body.indexOf("KEYS.forEach(key=>{"), "必须先使所有旧汇总响应失效，再逐项清理模块");
  assert.ok(body.includes('"#market-overview-updated","#market-data-updated"'));
  assert.ok(marketOverview.includes("return {refresh,invalidateConnection};"));
  const settingsStart = app.indexOf("function applySettings(nextSettings,");
  const settingsEnd = app.indexOf("
function saveSettingsFromForm", settingsStart);
  const settingsApplication = settingsStart >= 0 && settingsEnd > settingsStart ? app.slice(settingsStart, settingsEnd) : "";
  assert.ok(settingsApplication.includes("marketOverview.invalidateConnection()"), "连接变更通过安全步骤失效首页请求");
});

test("market connection changes invalidate in-flight dragon-tiger and calendar requests", () => {
  const invalidateStart = marketActivity.indexOf("function invalidateConnection() {");
  const invalidateEnd = marketActivity.indexOf("
 }", invalidateStart);
  const invalidateBody = invalidateStart >= 0 && invalidateEnd > invalidateStart
    ? marketActivity.slice(invalidateStart, invalidateEnd)
    : "";
  assert.ok(invalidateBody.includes('safeActivityUi(()=>abort("dragon"))'), "连接配置变更应通过安全入口中止龙虎榜请求");
  assert.ok(invalidateBody.includes('safeActivityUi(()=>abort("calendar"))'), "连接配置变更应通过安全入口中止交易日历请求");
  assert.ok(invalidateBody.includes('$("#dragon-tiger-body")'), "应清除旧龙虎榜结果");
  assert.ok(invalidateBody.includes('$("#trading-calendar-grid").replaceChildren'), "应清除旧交易日历结果");
  assert.ok(marketActivity.includes("return {refresh:()=>Promise.all([loadDragon(),loadCalendar()]),invalidateConnection};"));
  const settingsStart = app.indexOf("function applySettings(nextSettings,");
  const settingsEnd = app.indexOf("
function saveSettingsFromForm", settingsStart);
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
  const baseInvalidation = app.match(/function invalidateActiveTaskForAnalysisBaseChange\([\s\S]*?
\}/)?.[0] || "";
  assert.match(baseInvalidation, /safelyUpdateTaskContextUi\("重绘自选股卡片失败：", \(\) => renderWatchlist\(\)\)/, "切换分析地址后应在安全清理步骤中重绘自选卡片");
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
  assert.match(app, /const marketConnectionChanged = previousSettings\.apiKey !== nextSettings\.apiKey \|\|\s*previousSettings\.marketBase !== nextSettings\.marketBase;[\s\S]*?if \(analysisBaseChanged \|\| marketConnectionChanged\) \{[\s\S]*?runSettingsApplyStep\("失效数据中心检查结果失败：", \(\) =>\s*dataCenter\.invalidate\(/);
});

test("connection-check results are isolated between saved settings and unsaved draft", () => {
  const saved = app.match(/async function checkConnectionFromSettings\(\) \{[\s\S]*?
\}/)?.[0] || "";
  const draft = app.match(/async function checkDraftConnectionFromSettings\(\) \{[\s\S]*?
\}/)?.[0] || "";
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
  assert.match(app, /async function refreshServices\(\) \{[\s\S]*?const generation = \+\+serviceRefreshGeneration;[\s\S]*?if \(generation !== serviceRefreshGeneration\) \{\s*return \{ marketOK, analysisOK, marketState, analysisState, stale: true \};\s*\}/);
  assert.match(app, /async function refreshSources\(\) \{[\s\S]*?const generation = \+\+sourceRefreshGeneration;[\s\S]*?if \(generation !== sourceRefreshGeneration\) return;/);
  assert.match(app, /if \(generation !== serviceRefreshGeneration\) \{\s*return \{ marketOK, analysisOK, marketState, analysisState, stale: true \};\s*\}[\s\S]*?setText\("#connection-label", connectionLabel\)/);
});

test("provider capability refresh clears old connection data and owns its loading state", () => {
  const refreshSources = app.match(/async function refreshSources\(\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.ok(refreshSources, "refreshSources implementation must exist");
  assert.match(refreshSources, /showMessage\("正在读取当前连接配置对应的数据源能力…"\);[\s\S]*?setSourceTextSafely\("#source-count", "—"\);[\s\S]*?state\.sourceCount = 0/);
  assert.match(refreshSources, /button\.disabled = true;[\s\S]*?button\.setAttribute\("aria-busy", "true"\)/);
  assert.match(refreshSources, /正在读取当前连接配置对应的数据源能力/);
  assert.match(refreshSources, /showMessage\(message, "empty-row error-text"\)/);
  assert.match(refreshSources, /finally \{[\s\S]*?if \(generation === sourceRefreshGeneration\) \{[\s\S]*?if \(!button\) button = \$\("#reload-sources"\);[\s\S]*?if \(button\) \{[\s\S]*?button\.disabled = false;[\s\S]*?button\.removeAttribute\("aria-busy"\)/);
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
  assert.match(app, /document\.createElement\("label"\)[\s\S]*?settings-nav-search/);
  assert.match(app, /textContent = "搜索设置分组"/);
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
test("recent analysis history supports search, single removal and local-only clearing", () => {
  for (const id of ["recent-analysis-search", "clear-recent-analysis", "recent-analysis-feedback", "recent-analysis-list"]) {
    assert.match(html, new RegExp('id="' + id + '"'), "recent-history control must exist: " + id);
  }
  assert.match(app, /function renderRecentTasks\([\s\S]*?recent-analysis-search[\s\S]*?record\.task_id[\s\S]*?taskStatusLabel\(record\.status\)/);
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
  assert.match(module, /stock-provider-webui\.theme\.v1/);
  assert.doesNotMatch(module, /fetch\(|\/api\//, "theme preference must not call backend APIs");
});

test("mobile navigation drawer restores readable labels and can close via overlay or Escape", () => {
  const drawer = app.match(/function setMobileNavigationOpen\(open, \{ restoreFocus = false \} = \{\}\) \{[\s\S]*?
\}/)?.[0] || "";
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
  assert.match(app, /function openRunFlowDrawer\(\)[\s\S]*?dialog\.showModal\(\)/);
  assert.match(app, /function renderSelectedTaskEvent\(item\)[\s\S]*?detailLayout\?\.classList\.remove\("has-selected-event"\)[\s\S]*?detailLayout\?\.classList\.add\("has-selected-event"\)/);
  assert.match(stylesheet, /\.run-flow-dialog #task-detail:not\(\[hidden\]\):not\(\.has-selected-event\)\s*\{\s*grid-template-columns:\s*minmax\(0,\s*1fr\);\s*\}/);
  assert.match(app, /function activateTaskReference\(record\)[\s\S]*?openRunFlowDrawer\(\)[\s\S]*?pollTaskSafely\(state\.pollGeneration\)/);
  assert.match(app, /function renderTask\(task\)[\s\S]*?task\.started_at[\s\S]*?task\.finished_at/);
  assert.match(app, /function renderRunStatus\(status = ""\)[\s\S]*?runStatusLabel\(normalized\)/);
  assert.match(app, /function renderTaskResult\(result\)[\s\S]*?renderRunStatus\(result\.status \|\| ""\)/);
  assert.match(app, /close-run-flow-dialog/);
});

test("run-flow events expose only server-provided durations and diagnostics", () => {
  assert.match(app, /event\.duration_ms !== undefined && event\.duration_ms !== null/);
  assert.match(app, /event\.error_code/);
  assert.match(app, /parts\.join\(" · "\)/);
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
  const definitionBlock = app.match(/const capabilityDefinitions = \[([\s\S]*?)
  \];/);
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
  assert.match(html, /id="alert-rule-submit" type="submit"/);
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
  assert.doesNotMatch(app, /localStorage[^
]*stockSearch/i, "stock results must come from the API, not browser-side mock data");
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
  const toolbarButtonRules = [...stylesheet.matchAll(/\.stock-research-toolbar \.button\s*\{([^}]*)\}/g)].map((match) => match[1]);
  assert.ok(toolbarButtonRules.some((body) => /min-height:\s*38px/.test(body)), "toolbar button must have a touch-sized minimum height");
  assert.ok(toolbarButtonRules.some((body) => /font-size:\s*12px/.test(body)), "toolbar button label must remain readable");
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
  const readabilityStart = stylesheet.indexOf("/* 2026-10-27 readability pass: market overview, activity table, and calendar. */");
  const mobileOverrideStart = stylesheet.indexOf("@media (max-width: 700px)", readabilityStart);
  assert.ok(readabilityStart >= 0 && mobileOverrideStart > readabilityStart, "the general readability pass should precede responsive overrides");
  const generalRules = stylesheet.slice(readabilityStart, mobileOverrideStart);
  assert.match(generalRules, /\.market-activity-controls label \{[^}]*font-size: 12px;/, "desktop labels need a readable baseline");
  assert.match(generalRules, /\.market-activity-controls input \{ min-height: 38px; \}/, "desktop date inputs need a 38px target");
  assert.match(generalRules, /\.market-activity-controls \.button \{ min-height: 38px; font-size: 12px; \}/, "desktop buttons need a readable 38px baseline");
  assert.match(finalCssRuleBody(".activity-stock-link"), /min-height: 38px/);
  assert.match(finalCssRuleBody(".calendar-month-navigation .icon-button"), /min-width: 38px; min-height: 38px/);
  assert.match(finalCssRuleBody(".trading-calendar-weekdays span"), /font-size: 11px/);
  assert.match(stylesheet, /\.trading-calendar-day span \{ font-size: 11px; line-height: 1\.25; text-align: center; overflow-wrap: anywhere; \}/);
  assert.match(finalCssRuleBody(".trading-calendar-day span"), /font-size: 10px/);
  assert.match(stylesheet, /@media \(max-width: 700px\) \{[\s\S]*?\.market-overview-actions \.button \{ width: 100%; min-height: 40px; \}/);
  assert.match(stylesheet, /@media \(max-width: 600px\) \{[\s\S]*?\.trading-calendar-day span \{ font-size: 10px; \}/);
  assert.match(finalCssRuleBody(".market-activity-table"), /-webkit-overflow-scrolling: touch/);
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
  const importBody = app.match(/async function applyBrowserDataImport\(imported, currentWatchlist, currentRecentTasks, strategy\) \{[\s\S]*?
\}/)?.[0] || "";
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

test("切换到尚未接入的板块维度时，旧请求不能覆盖能力占位", () => {
  assert.match(
    marketOverview,
    /async function load\(key\)\s*\{[\s\S]*?cancel\(key\);\s*seq=generation\[key\];[\s\S]*?if\(key==="sectors"&&sectorMetric!=="change"\)\{showSectorMetricUnavailable\(\);return "unsupported";\}/
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

test("数据中心检查初始化与 finally 解锁异常不会破坏 generation 收尾", () => {
  const refresh = dataCenter.match(/async function refresh\(\) \{[\s\S]*?
  \}/)?.[0] || "";
  assert.ok(refresh);
  assert.match(refresh, /checking = true;[\s\S]*?try \{\s*settings = typeof getSettings/);
  assert.match(refresh, /按钮 DOM 更新失败不能阻止服务探测[\s\S]*?await Promise\.all/);
  assert.match(refresh, /catch \(error\) \{\s*if \(current !== generation\) return;[\s\S]*?safeError\(error\)/);
  assert.match(refresh, /finally \{[\s\S]*?checking = false;[\s\S]*?button\.disabled = false;[\s\S]*?finally 产生新的 rejection/);
  const invalidate = dataCenter.match(/function invalidate\(message = [\s\S]*?
  \}/)?.[0] || "";
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
  const end = app.indexOf("
function ", start + 20);
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
  const refresh = app.match(/function refreshMarketWorkspace\(\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.ok(refresh, "refreshMarketWorkspace implementation must exist");
  assert.match(refresh, /const startModule = \(label, operation\) => \{[\s\S]*?Promise\.resolve\(operation\(\)\)\.then/);
  assert.match(refresh, /\(error\) => \(\{ label, ok: false, error \}\)/);
  assert.match(refresh, /catch \(error\) \{[\s\S]*?return Promise\.resolve\(\{ label, ok: false, error \}\)/);
  assert.match(refresh, /startModule\("市场总览"[\s\S]*?startModule\("资金与活跃度"/);
  assert.match(refresh, /Promise\.all\(requests\)\.then\(\(results\) => \{[\s\S]*?result\.ok[\s\S]*?市场工作台部分模块刷新失败/);
  assert.match(refresh, /市场工作台刷新汇总异常：[\s\S]*?return \[\]/);
});

test("服务健康刷新把请求与 DOM 异常收敛为结果", () => {
  const refresh = app.match(/async function refreshServices\(\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.ok(refresh, "refreshServices implementation must exist");
  assert.match(refresh, /try \{[\s\S]*?await Promise\.all\(\[[\s\S]*?runHealthCheck\(marketHealth, settings\)/);
  assert.match(refresh, /catch \(renderError\)[\s\S]*?状态显示更新失败[\s\S]*?return \{ marketOK, analysisOK, marketState, analysisState, stale: false, renderError: message \}/);
  assert.match(refresh, /catch \(error\)[\s\S]*?服务健康检查流程异常[\s\S]*?return \{ \.\.\.staleResult, stale: false, error: message \}/);
  assert.match(refresh, /generation !== serviceRefreshGeneration/);
  assert.match(refresh, /label\.title = message/);
});

test("服务健康刷新不会覆盖行情查询区域的独立提示", () => {
  const refreshServices = app.match(/async function refreshServices\(\) \{[\s\S]*?
\}/)?.[0] || "";
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
  assert.match(app, /function renderRecentTasks\(records\)[\s\S]*?historyState\.storageReadable[\s\S]*?historyState\.storageValid[\s\S]*?历史修改已暂停/);
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
  const renderer = stockResearch.match(/function renderValuations\(data,symbol\)\{[\s\S]*?
 \}
 function fractionPercent/)?.[0] || "";
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
  const loader = stockResearch.match(/async function loadFundamentals\(\)\{[\s\S]*?
 \}/)?.[0] || "";
  assert.ok(loader);
  assert.match(loader, /try\{[\s\S]*?syncSymbol\(\)/);
  assert.match(loader, /if\(!Number\.isInteger\(year\)[\s\S]*?safeResearchFeedback\("财务报告期不正确。"\)/);
  assert.match(loader, /const ok=await fetchOne\("fundamentals"[\s\S]*?季度财务指标已返回/);
  assert.match(loader, /catch\(error\)\{\s*safeResearchFeedback\("季度财务指标读取流程异常："\+safeResearchError\(error\)\);\s*return false;/);
});

test("个股研究准备阶段异常与 finally 异常不会遗留按钮锁", () => {
  assert.match(stockResearch, /function safeResearchButton\(button,disabled,label\)[\s\S]*?catch\{return false;\}/);
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
  assert.match(app, /eventPayload\.task_id !== taskId[\s\S]*?阶段事件接口未返回 task_id/);
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
  assert.match(app, /taskError = responseContractError\("任务状态接口返回了无法识别的响应结构。"\)/);
  assert.match(app, /eventError = responseContractError\("阶段事件接口返回的数据结构无效。"\)/);
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
  const poller = app.match(/async function pollTask\([\s\S]*?
\}/)?.[0] || "";
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
  assert.match(app, /task\.symbols\.some\(\(value\) => normalizeLocalSymbol\(value\) === itemSymbol\)/);
  assert.match(app, /state\.taskSymbols\.some\(\(value\) => normalizeLocalSymbol\(value\) === itemSymbol\)/);
  assert.match(app, /state\.quote\?\.name && state\.quote\?\.symbol && sameMarketSymbol\(state\.quote\.symbol, symbol\)/);
});

 
test("阶段事件严格校验数值序号与 event 对象结构", () => {
  assert.match(app, /const sequence = typeof item\?\.sequence === "number" \? item\.sequence : NaN/);
  assert.match(app, /!Number\.isSafeInteger\(sequence\) \|\| sequence < 1/);
  assert.match(app, /!item\.event \|\| typeof item\.event !== "object" \|\| Array\.isArray\(item\.event\)/);
  assert.match(app, /字段无效的阶段事件（序号或 event 结构不符合契约），已忽略/);
  assert.doesNotMatch(app, /const sequence = Number\(item\?\.sequence\)/, "boolean sequence must not coerce to integer 1");
});

 
test("阶段事件的 run_id 必须匹配当前任务关联的 Run", () => {
  assert.match(app, /const expectedEventRunId = state\.runId \|\| task\?\.run_id \|\| ""/);
  assert.match(app, /const eventRunId = typeof item\.event\.run_id === "string" \? item\.event\.run_id\.trim\(\) : ""/);
  assert.match(app, /!expectedEventRunId \|\| !eventRunId \|\| eventRunId !== expectedEventRunId/);
  assert.match(app, /Run ID 缺失或与当前任务关联的 Run 不一致，已忽略/);
  assert.match(app, /字段无效的阶段事件（序号或 event 结构不符合契约），已忽略/);
});

test("增量阶段事件不会覆盖已读取的相同或更旧序号", () => {
  assert.match(app, /const previousSequence = state\.taskSequence/);
  assert.match(app, /sequence <= previousSequence \|\| state\.taskEvents\.has\(sequence\)/);
  assert.match(app, /state\.taskEvents\.set\(sequence, normalized\)/);
  assert.match(app, /重复或旧序号的阶段事件，已忽略/);
});

test("终态任务按 next_after_sequence 排空阶段事件分页后才读取 Run 报告", () => {
  assert.match(app, /taskEventCursor: 0/);
  const pollStart = app.indexOf("async function pollTask(");
  const pollEnd = app.indexOf("
async function createTask(", pollStart);
  const poller = pollStart >= 0 && pollEnd > pollStart ? app.slice(pollStart, pollEnd) : "";
  assert.match(poller, /const requestedEventCursor = state\.taskEventCursor/);
  assert.match(poller, /getAnalysisTaskEvents\(requestSettings, taskId, requestedEventCursor\)/);
  assert.match(poller, /!Number\.isSafeInteger\(nextAfterSequence\)/);
  assert.match(poller, /state\.taskEventCursor = nextAfterSequence/);
  assert.match(poller, /eventPageHasMore = eventPayload\.events\.length >= TASK_EVENT_PAGE_SIZE &&[\s\S]*?nextAfterSequence > requestedEventCursor/);
  assert.match(poller, /if \(task && TERMINAL_TASK_STATUSES\.has\(task\.status\)\) \{[\s\S]*?if \(eventPageHasMore && !eventError && !eventsSkipped\) \{[\s\S]*?正在继续读取剩余阶段事件[\s\S]*?setTimeout\(\(\) => pollTaskSafely\(generation\), 0\)[\s\S]*?return;[\s\S]*?stopTaskPolling\(\)[\s\S]*?loadTaskRunResult/);
  assert.match(poller, /if \(eventError && isRetryableApiError\(eventError\) && !eventError\.responseContractError\) \{[\s\S]*?5 秒后自动重试[\s\S]*?setTimeout\(\(\) => pollTaskSafely\(generation\), 5000\)/);
  const refreshStart = app.indexOf('$("#refresh-task").addEventListener("click"');
  const refreshEnd = app.indexOf('$("#recent-analysis-search")', refreshStart);
  const refreshHandler = refreshStart >= 0 && refreshEnd > refreshStart ? app.slice(refreshStart, refreshEnd) : "";
  assert.ok(refreshHandler.includes("state.taskEventsUnavailable = false"), "手动刷新必须恢复事件读取");
  assert.ok(refreshHandler.includes("pollTaskSafely(generation)"), "手动刷新必须立即重新发起 Task/事件读取");
  assert.doesNotMatch(refreshHandler, /state\.taskEventCursor\s*=\s*0/, "手动重试应保留已推进的服务端游标，避免重新读取第一页");
  assert.equal((app.match(/state\.taskSequence = 0;\s*state\.taskEventCursor = 0;/g) || []).length, 3, "新建、恢复和清空任务时都重置独立事件游标");
});

test("分析 API 切换期间的 UI 清理异常不会中断配置应用", () => {
  const helper = app.match(/function safelyUpdateTaskContextUi\(label, update\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.match(helper, /try \{\s*update\(\);\s*\} catch \(error\)/);
  assert.match(helper, /部分界面清理失败[\s\S]*?catch/);

  const invalidateStart = app.indexOf("function invalidateActiveTaskForAnalysisBaseChange(");
  const invalidateEnd = app.indexOf("
function applySettings(", invalidateStart);
  const invalidate = invalidateStart >= 0 && invalidateEnd > invalidateStart
    ? app.slice(invalidateStart, invalidateEnd) : "";
  assert.match(invalidate, /state\.taskId = "";[\s\S]*?safelyUpdateTaskContextUi\("清理任务详情失败：[\s\S]*?safelyUpdateTaskContextUi\("重绘阶段事件失败：[\s\S]*?safelyUpdateTaskContextUi\("重绘自选股卡片失败：/);
  assert.doesNotMatch(invalidate, /(^|
)\s*renderTaskEvents\(\);|(^|
)\s*renderTaskFlowSummary\(\);|(^|
)\s*renderRecentTasks\(\);/);

  const applyStart = app.indexOf("function applySettings(");
  const applyEnd = app.indexOf("
function saveSettingsFromForm(", applyStart);
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
  const importBody = app.match(/function validateBrowserDataPayload\(payload\) \{[\s\S]*?
\}/)?.[0] || "";
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
  const importBody = app.match(/async function applyBrowserDataImport\(imported, currentWatchlist, currentRecentTasks, strategy\) \{[\s\S]*?
\}/)?.[0] || "";
  assert.match(importBody, /applySettings\(nextSettings, \{ persistPreviousActiveTask: false \}\);[\s\S]*?state\.taskHistoryGeneration \+= 1;\s*state\.taskContextGeneration \+= 1;/);
});

test("个股行情默认请求复用统一快照；旧服务仅在 404 时回退，显式日期范围尊重独立日 K 参数", () => {
  const start = app.indexOf("async function runQuoteQuery(event) {");
  const end = app.indexOf("
function ", start + 20);
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
  assert.match(dataCenter, /runHealthCheck\(getDataSourceDiagnostics, settings\)/);
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
  assert.match(html, /不主动调用同花顺接口，不验证第三方 API Key/);
  assert.match(html, /发送测试（待接入）/);
  assert.match(html, /通知渠道管理、订阅配置和发送测试接口/);
  assert.doesNotMatch(html, /当前没有逐 Provider \/ capability 的受控真实探测 API/);
});


test("自选股批量行情只请求当前本地清单并在连接变更时清除旧快照", () => {
  assert.match(apiClient, /export function getMarketQuotes\(settings, symbols, signal\)/);
  assert.match(app, /function refreshWatchlistQuotes\(\)/);
  assert.match(app, /getMarketQuotes\(state\.settings, symbols, controller\.signal\)/);
  assert.match(app, /state\.watchlistQuotes = nextQuotes/);
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
  assert.match(app, /title === "数据质量与策略门禁"[\s\S]*?analysis-result-disclosure/);
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
  assert.match(renderer, /最近一次批量刷新失败，当前保留此前成功快照/);
  assert.match(renderer, /formatResearchMA\(5, isCurrent\)/);
  assert.match(renderer, /formatResearchMA\(20, isCurrent\)/);
});

test("全市场分页行情只调用分页概览 API 并正确展示覆盖与质量状态", () => {
  assert.match(app, /import \{ bindMarketStocks \} from "\.\/market-stocks\.js"/);
  assert.match(app, /bindMarketStocks\([\s\S]*?selectStockFromMarket\(item, "全市场行情"\)/);
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

test("行情服务接口文档、Handler 与 WebUI 调用边界明确区分", () => {
  assert.match(marketHTTPServer, /mux\.HandleFunc\("GET \/health\/live"/);
  assert.match(marketHTTPServer, /mux\.HandleFunc\("GET \/health\/ready"/);
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/data-sources\/diagnostics"/);
  assert.match(marketHTTPServer, /mux\.Handle\("POST \/api\/v1\/data-sources\/probe"/);
  assert.match(apiClient, /getDataSourceDiagnostics/);
  assert.doesNotMatch(apiClient, /data-sources\/probe/, "WebUI 不应为同花顺诊断调用主动探测接口");
  assert.match(marketOpenAPI, /\/api\/v1\/market\/capital-flow:/, "OpenAPI 当前声明了市场级资金流接口");
  assert.doesNotMatch(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/market\/capital-flow"/, "市场级资金流尚无 Handler 注册");
  assert.match(marketHTTPServer, /mux\.Handle\("GET \/api\/v1\/stocks\/\{symbol\}\/capital-flow"/, "个股资金流 Handler 已注册");
  assert.match(apiClient, /export function getCapitalFlow/);
  assert.doesNotMatch(apiClient, /export function getMarketCapitalFlow/, "WebUI 不应把未注册的市场级资金流接口当作可用能力");
});

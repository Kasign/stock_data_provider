import { applyTheme, persistTheme, readTheme, THEME_STORAGE_KEY } from "./theme.js";
import { DEFAULT_PREFERENCES, PREFERENCES_KEY, WATCHLIST_DISPLAY_FIELDS, readPreferences, resetPreferences, savePreferences } from "./preferences.js";
import { connectionDraftChanged, mergeConnectionDraft } from "./settings-draft.js";
import { resolveNotificationViewState } from "./notification-state.js";
import { resolveChatViewState } from "./chat-state.js";
import { resolveScreeningCapability, resolveScreeningTaskState, resolveScreeningCollectionState } from "./screening-state.js";
import { buildScreeningDraftSummary, createScreeningCondition, formatScreeningCondition } from "./screening-draft.js";
import { resolveAlertCapability, resolveAlertEvaluationState, resolveAlertCollectionState } from "./alert-state.js";
import {
  analysisHealth,
  clearSettings,
  createAnalysisTask,
  getAnalysisRun,
  getAnalysisTask,
  getAnalysisTaskEvents,
  getDailyBars,
  getDataSources,
  getMarketBundle,
  getMarketQuotes,
  getQuote,
  searchStocks,
  loadSettings,
  marketHealth,
  saveSettings,
  SETTINGS_KEY
} from "./api.js";
import { calculateTechnicalIndicators, normalizeDailyBars, renderCandlestickVolumeChart, renderCloseChart, renderMACDChart, renderRSIChart } from "./chart.js";
import { bindMarketOverview } from "./market-overview.js";
import { bindMarketStocks } from "./market-stocks.js";
import { taskEventMatchesFilter } from "./task-event-filter.js";
import { bindStockResearch } from "./stock-research.js";
import { bindDataCenter } from "./data-center.js";
import { bindLocalAlertDrafts } from "./local-alert-drafts.js";
import { bindResearchNotes } from "./research-notes.js";
import { bindNotificationPreferences } from "./notification-preferences.js";
import { runHealthCheck } from "./health-check.js";
import { bindMarketActivity } from "./market-activity.js";
import { bindExistingApiTools } from "./existing-api-tools.js";
import { clearRecentTasks, filterRecentTasks, readRecentTasks, readRecentTasksState, removeRecentTask, upsertRecentTask, RECENT_TASKS_KEY, MAX_RECENT_TASKS } from "./recent-tasks.js";
import { addWatchlistSymbol, importWatchlistSymbols, MAX_WATCHLIST_ITEMS, readWatchlistState, removeWatchlistSymbol, renameWatchlistSymbol, WATCHLIST_KEY } from "./watchlist.js";
import { assignWatchlistGroup, createWatchlistGroup, readWatchlistGroups, removeWatchlistGroup, DEFAULT_WATCHLIST_GROUP } from "./watchlist-groups.js";
import { buildRunResultFilename, buildRunResultMarkdown } from "./report.js";
import {
  comparableMarketSymbol,
  describeApiError,
  formatCompact,
  formatDate,
  formatDateTime,
  finiteNumberOrNull,
  formatPercent,
  formatPrice,
  isRetryableApiError,
  normalizeAnalysisSymbol,
  normalizeBasePath,
  parseAnalysisSymbols,
  normalizeSymbol,
  qualityLabel,
  runStatusLabel,
  taskStatusLabel
} from "./format.js";

const $ = (selector) => document.querySelector(selector);
const TASK_REFERENCE_KEY = "stock-provider-webui.last-analysis-task.v1";
const BROWSER_DATA_FORMAT = "stock-provider-webui-browser-data";
const BROWSER_DATA_VERSION = 3;
const TERMINAL_TASK_STATUSES = new Set(["succeeded", "partial", "failed"]);
const TASK_STATUSES = new Set(["queued", "running", "succeeded", "partial", "failed"]);
const TASK_EVENT_STATUS_LABELS = Object.freeze({
  succeeded: "已完成",
  success: "已完成",
  completed: "已完成",
  failed: "失败",
  error: "异常",
  blocked: "已阻断",
  rejected: "已拒绝",
  degraded: "已降级",
  partial: "部分完成",
  running: "运行中",
  started: "已启动",
  "in-progress": "运行中",
  in_progress: "运行中",
  processing: "处理中",
  queued: "排队中",
  pending: "等待中",
  skipped: "已跳过",
  unknown: "状态未报告",
  other: "其他状态"
});

function responseContractError(message) {
  const error = new Error(message);
  error.responseContractError = true;
  return error;
}

function responseIdentityError(message) {
  const error = responseContractError(message);
  error.responseIdentityMismatch = true;
  return error;
}

function taskEventStatusKey(status) {
  const normalized = String(status || "").trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-");
  return Object.prototype.hasOwnProperty.call(TASK_EVENT_STATUS_LABELS, normalized)
    ? normalized
    : normalized ? "other" : "unknown";
}

function taskEventStatusLabel(status, statusKey = taskEventStatusKey(status)) {
  return statusKey === "other" ? String(status) : TASK_EVENT_STATUS_LABELS[statusKey];
}

function taskEventStatusTone(statusKey) {
  if (["succeeded", "success", "completed"].includes(statusKey)) return "is-local";
  if (["failed", "error", "blocked", "rejected"].includes(statusKey)) return "is-error";
  if (["degraded", "partial"].includes(statusKey)) return "is-warning";
  return "is-readonly";
}

const RUN_RESULT_RETRY_DELAYS_MS = [1000, 2500, 5000];
const TASK_EVENT_PAGE_SIZE = 100;
let initialConnectionDraft = null;
let serviceRefreshGeneration = 0;
let sourceRefreshGeneration = 0;
let savedConnectionCheckGeneration = 0;
let draftConnectionCheckGeneration = 0;
let pendingBrowserImport = null;

const state = {
  settings: loadSettings(),
  view: "market",
  quote: null,
  bundleSnapshot: null,
  bars: [],
  chartView: "close",
  movingAverages: [5, 20],
  technicalRows: [],
  technicalSymbol: "",
  chartMeta: { adjustmentBasis: "unknown", volumeUnit: "", amountUnit: "", qualityStatus: "unknown" },
  // 当前任务选择代际与最近任务历史代际分别隔离不同来源的晚到响应。
  taskContextGeneration: 0,
  taskHistoryGeneration: 0,
  taskId: "",
  runId: "",
  taskAnalysisBase: "",
  taskCreatedAt: "",
  analysisConnectionGeneration: 0,
  taskSymbols: [],
  taskType: "post_market_scan",
  taskStatus: "",
  taskStatusLoading: false,
  taskStatusError: "",
  taskStatusErrorRetrying: false,
  isCreatingTask: false,
  lastRunResult: null,
  runResultReadStatus: "",
  runResultReadError: "",
  runResultGeneration: 0,
  taskSequence: 0,
  taskEventCursor: 0,
  taskEvents: new Map(),
  taskEventsLoaded: false,
  taskEventsUnavailable: false,
  taskEventsError: "",
  selectedTaskEventSequence: null,
  expandedTaskEventSequences: new Set(),
  taskEventFilter: "all",
  pollTimer: null,
  pollGeneration: 0,
  isQuerying: false,
  quoteQueryGeneration: 0,
  quoteQueryController: null,
  quoteQueryKey: "",
  quoteQueryInput: "",
  lastQueryInput: "",
  stockSearchTimer: null,
  stockSearchController: null,
  stockSearchGeneration: 0,
  stockSearchComposing: false,
  stockSearchResults: [],
  stockSearchActiveIndex: -1,
  selectedStockSearchItem: null,
  sourceCount: 0,
  suppressedHistoryTaskId: "",
  suppressedHistoryTaskBase: "",
  chatStockContext: null,
  // 用户主动清除后，禁止旧行情与页面切换重新注入股票上下文，直到开始新的行情查询。
  chatStockContextCleared: false,
  watchlistQuotes: new Map(),
  watchlistQuoteStatus: "idle",
  watchlistQuoteMessage: "批量行情按需读取，不会在打开自选股时自动请求。",
  watchlistQuoteGeneration: 0,
  watchlistQuoteController: null,
  watchlistImportPreview: null
};

const marketOverview = bindMarketOverview(() => state.settings, (item, source) => selectStockFromMarket(item, source));
const stockResearch = bindStockResearch(() => state.settings, () => getQuerySymbol());
const dataCenter = bindDataCenter(() => state.settings);
bindLocalAlertDrafts({ root: document.querySelector("#alerts-view"), getContext: () => ({ symbol: state.quote?.symbol || state.selectedStockSearchItem?.symbol || "", name: state.quote?.name || state.selectedStockSearchItem?.name || "" }) });
bindResearchNotes({ root: document.querySelector("#research-view"), getContext: () => ({ symbol: state.quote?.symbol || state.selectedStockSearchItem?.symbol || "", name: state.quote?.name || state.selectedStockSearchItem?.name || "" }) });
bindNotificationPreferences({ root: document.querySelector("#settings-notification-section") });
bindExistingApiTools(() => state.settings);
const marketActivity = bindMarketActivity(
  () => state.settings,
  (item) => selectStockFromMarket(item, "龙虎榜")
);
const marketStocks = bindMarketStocks(
  () => state.settings,
  (item) => selectStockFromMarket(item, "全市场行情")
);

function setText(selector, value) {
  const target = $(selector);
  if (target) target.textContent = value == null || value === "" ? "—" : String(value);
}

function selectStockFromMarket(item, source = "市场榜单") {
  const symbol = String(item?.symbol || "").trim();
  if (!symbol) return;
  setView("stocks");
  $("#symbol-input").value = symbol;
  state.selectedStockSearchItem = { symbol, name: item.name || symbol };
  cancelStockSearchRequest();
  hideStockSearch();
  stockResearch.syncSymbol();
  setText("#query-hint", "已从" + source + "选择 " + (item.name || symbol) + "，正在加载实时行情与日 K。");
  void runQuoteQuery();
}

function syncThemeControl(theme, persistenceWarning = false) {
  const current = theme === "light" ? "light" : "dark";
  const target = current === "dark" ? "light" : "dark";
  const targetLabel = target === "light" ? "浅色" : "深色";
  const warning = persistenceWarning ? "；当前页面已切换，但未能确认主题偏好已保存" : "";
  $("#theme-toggle-icon").textContent = target === "light" ? "☼" : "☾";
  $("#theme-toggle-label").textContent = targetLabel;
  $("#theme-toggle").setAttribute("aria-label", persistenceWarning ? "切换主题；当前主题未能保存" : "切换主题");
  $("#theme-toggle").setAttribute("aria-pressed", String(current === "light"));
  $("#theme-toggle").title = "当前" + (current === "light" ? "浅色" : "深色") + "主题；点击切换至" + targetLabel + "主题" + warning;
}

function setMobileNavigationOpen(open, { restoreFocus = false } = {}) {
  const wasOpen = $(".sidebar").classList.contains("is-open");
  const isOpen = Boolean(open) && window.innerWidth <= 600;
  $(".sidebar").classList.toggle("is-open", isOpen);
  $("#mobile-nav-overlay").hidden = !isOpen;
  document.body.classList.toggle("mobile-nav-open", isOpen);
  $("#mobile-menu-toggle").setAttribute("aria-expanded", String(isOpen));
  $("#mobile-menu-toggle").setAttribute("aria-label", isOpen ? "关闭导航" : "打开导航");
  $("#mobile-menu-toggle").title = isOpen ? "关闭导航" : "打开导航";
  if (!wasOpen && isOpen) $(".sidebar").querySelector(".nav-link:not([disabled])")?.focus();
  if (wasOpen && !isOpen && restoreFocus) $("#mobile-menu-toggle").focus();
}

function persistTaskReference() {
  const taskAnalysisBase = state.taskAnalysisBase || state.settings.analysisBase;
  if (!state.taskId || (state.suppressedHistoryTaskId === state.taskId && state.suppressedHistoryTaskBase === taskAnalysisBase)) return;
  const record = {
    task_id: state.taskId,
    run_id: state.runId || "",
    created_at: state.taskCreatedAt || "",
    symbols: state.taskSymbols,
    task_type: state.taskType,
    status: state.taskStatus,
    analysis_base: taskAnalysisBase
  };
  const comparable = JSON.stringify(record);
  try {
    const previous = JSON.parse(localStorage.getItem(TASK_REFERENCE_KEY) || "null");
    const previousComparable = previous ? JSON.stringify({
      task_id: previous.task_id,
      run_id: previous.run_id || "",
      created_at: previous.created_at || "",
      symbols: Array.isArray(previous.symbols) ? previous.symbols : [],
      task_type: previous.task_type || "post_market_scan",
      status: previous.status || "",
      analysis_base: previous.analysis_base || state.taskAnalysisBase || state.settings.analysisBase
    }) : "";
    if (previousComparable === comparable) {
      // Migrate the legacy single-task reference into the new local history once.
      const historyItem = safeReadRecentTasks().find((item) =>
        item.task_id === record.task_id &&
        String(item.analysis_base || taskAnalysisBase) === taskAnalysisBase
      );
      const historyComparable = historyItem ? JSON.stringify({
        task_id: historyItem.task_id,
        run_id: historyItem.run_id || "",
        created_at: historyItem.created_at || "",
        symbols: Array.isArray(historyItem.symbols) ? historyItem.symbols : [],
        task_type: historyItem.task_type || "post_market_scan",
        status: historyItem.status || "",
        analysis_base: historyItem.analysis_base || taskAnalysisBase
      }) : "";
      if (historyComparable !== comparable) {
        record.updated_at = previous?.updated_at || new Date().toISOString();
        persistRecentTaskHistory(record);
      }
      return;
    }
    record.updated_at = new Date().toISOString();
    let taskReferencePersisted = false;
    try {
      const serializedReference = JSON.stringify(record);
      localStorage.setItem(TASK_REFERENCE_KEY, serializedReference);
      taskReferencePersisted = localStorage.getItem(TASK_REFERENCE_KEY) === serializedReference;
    } catch {
      // 活动任务引用与最近任务历史分别确认；单个键写入失败不应阻止历史列表尝试保存。
    }
    const historyPersisted = persistRecentTaskHistory(record);
    if (!taskReferencePersisted) {
      setRecentTasksFeedback(
        historyPersisted
          ? "最近任务已保存，但活动任务引用未能确认持久化；刷新后可能无法自动恢复当前任务。"
          : "任务已创建，但活动任务引用与最近任务历史均未能确认持久化；刷新后可能无法从本地找回。",
        "warning"
      );
    }
  } catch {
    // 本地引用写入失败不阻断当前任务，但必须明确提示刷新后可能无法找回。
    setRecentTasksFeedback(
      "任务已创建，但浏览器存储不可用，任务引用未保存；刷新后可能无法从本地列表找回。",
      "warning"
    );
  }
}

function renderRecentTasks(records) {
  const root = $("#recent-analysis-list");
  root.replaceChildren();
  const historyState = safeReadRecentTasksState();
  if (!historyState.storageReadable || !historyState.storageValid) {
    setText("#recent-analysis-count", "不可用 / " + MAX_RECENT_TASKS);
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = !historyState.storageReadable
      ? "无法读取浏览器最近任务存储。为避免误报为空，当前历史暂不可用；请检查浏览器存储权限后重新载入。"
      : "本地最近任务数据格式异常。为避免覆盖原始记录，历史修改已暂停；请先检查或备份浏览器数据。";
    root.append(empty);
    setRecentTasksFeedback(empty.textContent, "error");
    renderResearchStockList();
    return;
  }
  setRecentTasksFeedback("");
  const allItems = Array.isArray(records) ? records : historyState.items;
  const items = filterRecentTasks(allItems, {
    search: $("#recent-analysis-search").value,
    status: $("#recent-analysis-status").value,
    taskType: $("#recent-analysis-type").value
  });
  setText("#recent-analysis-count", items.length + " / " + allItems.length + "（最多 10）");

  if (!allItems.length || !items.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = !allItems.length
      ? "完成一次分析后，这里会显示最近任务。"
      : "没有匹配项，请调整关键词或清除搜索。";
    root.append(empty);
    renderResearchStockList();
    return;
  }

  items.forEach((record) => {
    const item = document.createElement("article");
    item.className = "recent-analysis-item";

    const open = document.createElement("button");
    open.type = "button";
    open.className = "recent-analysis-open";
    open.dataset.openTaskId = record.task_id;
    const main = document.createElement("span");
    main.className = "recent-analysis-item-main";
    const title = document.createElement("strong");
    title.textContent = record.symbols?.join(", ") || "标的未记录";
    const recordAnalysisBase = String(record.analysis_base || state.settings.analysisBase);
    const isOtherEndpoint = recordAnalysisBase !== state.settings.analysisBase;
    open.dataset.openTaskBase = recordAnalysisBase;
    const detail = document.createElement("small");
    detail.textContent = (record.updated_at ? formatDateTime(record.updated_at) : "时间未记录") + " · " + record.task_id + (isOtherEndpoint ? " · 旧分析 API 地址" : "");
    const status = document.createElement("span");
    status.className = "recent-analysis-item-status";
    status.textContent = taskStatusLabel(record.status);
    main.append(title, detail);
    open.append(main, status);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "recent-analysis-remove";
    remove.dataset.removeTaskId = record.task_id;
    remove.dataset.removeTaskBase = recordAnalysisBase;
    remove.textContent = "移除";
    remove.setAttribute("aria-label", "从本地列表移除任务 " + record.task_id);

    item.append(open, remove);
    root.append(item);
  });
}

function safeReadRecentTasksState() {
  try {
    return readRecentTasksState(localStorage);
  } catch {
    return { items: [], storageReadable: false, storageValid: false, reason: "unavailable" };
  }
}

function safeReadRecentTasks() {
  return safeReadRecentTasksState().items;
}

function persistRecentTaskHistory(record) {
  const history = upsertRecentTask(record, localStorage);
  renderRecentTasks(history);

  const storageState = safeReadRecentTasksState();
  if (!storageState.storageReadable || !storageState.storageValid) return false;
  const expected = {
    task_id: String(record.task_id || "").trim(),
    run_id: typeof record.run_id === "string" ? record.run_id : "",
    created_at: typeof record.created_at === "string" ? record.created_at : "",
    symbols: Array.isArray(record.symbols) ? record.symbols.filter((item) => typeof item === "string") : [],
    task_type: typeof record.task_type === "string" ? record.task_type : "post_market_scan",
    status: typeof record.status === "string" ? record.status : "",
    analysis_base: typeof record.analysis_base === "string" ? record.analysis_base.trim() : "",
    updated_at: typeof record.updated_at === "string" ? record.updated_at : ""
  };
  const persisted = history.some((item) =>
    item.task_id === expected.task_id &&
    item.run_id === expected.run_id &&
    item.created_at === expected.created_at &&
    item.task_type === expected.task_type &&
    item.status === expected.status &&
    item.analysis_base === expected.analysis_base &&
    item.updated_at === expected.updated_at &&
    JSON.stringify(item.symbols) === JSON.stringify(expected.symbols)
  );
  if (!persisted) {
    setRecentTasksFeedback(
      "任务已创建，但最近任务引用未能确认持久化；刷新后可能无法从本地列表找回。",
      "warning"
    );
  }
  return persisted;
}

function setRecentTasksFeedback(message, kind = "info") {
  const target = $("#recent-analysis-feedback");
  target.hidden = !message;
  target.className = "recent-analysis-feedback" + (message ? " is-" + kind : "");
  target.textContent = message;
}

function removeRecentTaskReference(taskId, analysisBase = "") {
  const historyState = safeReadRecentTasksState();
  if (!historyState.storageReadable || !historyState.storageValid) {
    renderRecentTasks(historyState.items);
    setRecentTasksFeedback(
      !historyState.storageReadable
        ? "无法读取浏览器最近任务存储，未移除任何记录。请检查浏览器存储权限后重试。"
        : "本地最近任务数据格式异常，未修改历史记录。请先检查或备份浏览器数据。",
      "error"
    );
    return;
  }

  // 先清理兼容旧版本的单任务引用，确认成功后再删除历史列表，避免刷新后旧任务重新出现。
  try {
    const last = JSON.parse(localStorage.getItem(TASK_REFERENCE_KEY) || "null");
    if (last?.task_id === taskId && (!analysisBase || !last.analysis_base || last.analysis_base === analysisBase)) {
      localStorage.removeItem(TASK_REFERENCE_KEY);
      if (localStorage.getItem(TASK_REFERENCE_KEY) !== null) throw new Error("旧任务引用仍存在");
    }
  } catch (error) {
    renderRecentTasks();
    setRecentTasksFeedback("无法确认旧任务引用已移除，最近任务历史未修改。请检查浏览器存储权限后重试。" +
      (error?.message ? "（" + error.message + "）" : ""), "error");
    return;
  }

  try {
    removeRecentTask(taskId, localStorage, undefined, analysisBase);
  } catch (error) {
    renderRecentTasks();
    setRecentTasksFeedback("移除最近任务失败，已保留当前存储内容。" +
      (error?.message ? "（" + error.message + "）" : ""), "error");
    return;
  }

  // removeRecentTask 会尽量处理存储异常；必须回读确认目标记录确实不再存在，才能反馈成功。
  const persistedState = safeReadRecentTasksState();
  const targetStillExists = persistedState.items.some((item) =>
    item.task_id === taskId && (!analysisBase || !item.analysis_base || item.analysis_base === analysisBase)
  );
  if (!persistedState.storageReadable || !persistedState.storageValid || targetStillExists) {
    renderRecentTasks();
    setRecentTasksFeedback("无法确认最近任务引用已移除。浏览器存储可能不可写，已保留可读取的记录，请检查后重试。", "error");
    return;
  }

  if (state.taskId === taskId && (!analysisBase || state.taskAnalysisBase === analysisBase)) {
    state.suppressedHistoryTaskId = taskId;
    state.suppressedHistoryTaskBase = state.taskAnalysisBase || state.settings.analysisBase;
  }
  renderRecentTasks(persistedState.items);
  setRecentTasksFeedback("已从当前浏览器移除该任务引用；服务端任务和报告不会被删除。");
}

function clearRecentTaskHistory() {
  if (!window.confirm("清空当前浏览器保存的最近任务引用？不会删除服务端任务或报告。")) return;
  const currentTaskId = state.taskId;
  let remaining = [];
  try {
    localStorage.removeItem(TASK_REFERENCE_KEY);
    if (localStorage.getItem(TASK_REFERENCE_KEY) !== null) throw new Error("旧任务引用仍存在");
    clearRecentTasks(localStorage);
    const clearedState = safeReadRecentTasksState();
    if (!clearedState.storageReadable || !clearedState.storageValid || clearedState.items.length !== 0) {
      throw new Error("最近任务历史回读后仍有记录，无法确认清空完成");
    }
    remaining = clearedState.items;
  } catch (error) {
    renderRecentTasks();
    setRecentTasksFeedback("未能确认最近任务历史已清空。浏览器存储可能不可写，请检查后重试。" +
      (error?.message ? "（" + error.message + "）" : ""), "error");
    return;
  }
  // 使清空之前发出的创建请求失效，防止晚到响应把历史重新写入。
  state.taskHistoryGeneration += 1;
  state.taskContextGeneration += 1;
  state.suppressedHistoryTaskId = currentTaskId;
  state.suppressedHistoryTaskBase = currentTaskId ? (state.taskAnalysisBase || state.settings.analysisBase) : "";
  renderRecentTasks(remaining);
  setRecentTasksFeedback("本地最近任务列表已清空；当前页面中的任务不会被取消，服务端记录未删除。");
}


function setWatchlistFeedback(message, kind = "info") {
  const target = $("#watchlist-feedback");
  target.hidden = !message;
  target.textContent = message;
  target.className = "watchlist-feedback" + (message ? " watchlist-feedback-" + kind : "");
}

function renderResearchStockList() {
  const root = $("#research-stock-list-body");
  if (!root) return;
  root.replaceChildren();
  const watchlistState = safeReadWatchlistState();
  const records = watchlistState.items;
  const quote = state.quote && typeof state.quote === "object" ? state.quote : null;
  let currentSymbol = "";
  try { currentSymbol = quote ? normalizeSymbol(quote.symbol || "") : ""; } catch { currentSymbol = ""; }
  const items = records.map((record) => {
    let symbol = String(record.symbol || "");
    try { symbol = normalizeSymbol(symbol); } catch { /* 不规范本地条目保留原始值并显示未加载。 */ }
    return { symbol, name: record.name || symbol };
  });
  if (currentSymbol && !items.some((item) => sameMarketSymbol(item.symbol, currentSymbol))) {
    items.unshift({ symbol: currentSymbol, name: quote.name || currentSymbol });
  }
  if (!items.length) {
    const tr = document.createElement("tr"), td = document.createElement("td");
    td.colSpan = 10; td.className = "table-empty";
    td.textContent = !watchlistState.storageReadable
      ? "无法读取当前浏览器的自选股存储。为避免误报为空，请检查浏览器存储权限后重新载入。"
      : !watchlistState.storageValid
        ? "本地自选数据格式异常。为避免覆盖原始数据，当前未修改列表；请先检查或备份浏览器存储。"
        : "当前没有自选股或已查询标的。可以先搜索股票，再回到本页查看研究详情。";
    tr.append(td); root.append(tr);
    setText("#research-list-scope", !watchlistState.storageReadable
      ? "本地自选不可读取"
      : !watchlistState.storageValid
        ? "本地自选格式异常"
        : "当前浏览器 · 无标的");
    renderResearchOverview();
    return;
  }
  const technicalSymbol = state.technicalSymbol;
  const latest = technicalSymbol && sameMarketSymbol(technicalSymbol, currentSymbol)
    ? [...state.technicalRows].reverse().find((row) => row && row.close !== null && row.close !== undefined) || null
    : null;
  const formatResearchMA = (period, isCurrent) => {
    if (!isCurrent) return "未加载";
    if (!technicalSymbol || !sameMarketSymbol(technicalSymbol, currentSymbol)) return state.isQuerying ? "读取中…" : "未加载";
    if (!latest) return "数据不足";
    const value = latest["ma" + period];
    return Number.isFinite(value) ? value.toFixed(2) : "数据不足";
  };
  items.forEach((item) => {
    const isCurrent = Boolean(currentSymbol) && sameMarketSymbol(item.symbol, currentSymbol) && Boolean(quote);
    const batchRecord = isCurrent ? null : state.watchlistQuotes.get(normalizeWatchlistQuoteSymbol(item.symbol));
    const itemQuote = isCurrent ? quote : batchRecord?.quote || null;
    const hasQuote = Boolean(itemQuote);
    const batchQuoteFailed = Boolean(batchRecord && !batchRecord.quote);
    const tr = document.createElement("tr"), nameCell = document.createElement("td");
    const name = document.createElement("strong"), symbol = document.createElement("small");
    name.className = "research-list-stock-name"; name.textContent = item.name || item.symbol;
    symbol.className = "research-list-symbol"; symbol.textContent = item.symbol; nameCell.append(name, symbol); tr.append(nameCell);
    const quoteValue = (value, formatter) => hasQuote
      ? (value === null || value === undefined || value === "" ? "—" : formatter(value))
      : (batchQuoteFailed ? "行情失败" : "未加载");
    const changePercent = hasQuote ? finiteNumberOrNull(itemQuote.change_percent) : null;
    const volumeRatio = hasQuote ? finiteNumberOrNull(itemQuote.volume_ratio) : null;
    const changeClass = changePercent !== null && changePercent > 0 ? "positive" : changePercent !== null && changePercent < 0 ? "negative" : "";
    tr.append(
      makeCell(quoteValue(itemQuote?.current_price, (value) => formatPrice(value, 2))),
      makeCell(quoteValue(itemQuote?.change_percent, formatPercent), changeClass),
      makeCell(quoteValue(itemQuote?.amount, formatCompact)),
      makeCell(quoteValue(itemQuote?.turnover_rate, formatPercent)),
      makeCell(hasQuote ? (volumeRatio === null ? "—" : volumeRatio.toFixed(2)) : (batchQuoteFailed ? "行情失败" : "未加载")),
      makeCell(formatResearchMA(5, isCurrent)),
      makeCell(formatResearchMA(20, isCurrent)),
      (() => {
        const cell = document.createElement("td");
        const qualityStatus = isCurrent ? itemQuote.quality_status : (batchRecord?.qualityStatus || itemQuote?.quality_status || "unknown");
        const pill = document.createElement("span");
        pill.className = "mini-quality mini-quality-" + String(qualityStatus || "unknown").toLowerCase();
        const stale = Boolean(batchRecord?.quote && state.watchlistQuoteStatus === "error");
        pill.textContent = stale ? "旧快照 · " + qualityLabel(qualityStatus) : (batchRecord && !batchRecord.quote ? "行情失败" : hasQuote ? qualityLabel(qualityStatus) : "未加载");
        const detail = [
          batchRecord?.errorCode || "",
          batchRecord?.fetchedAt ? "批量快照 " + formatDateTime(batchRecord.fetchedAt) : "",
          stale ? "最近一次批量刷新失败，当前保留此前成功快照。" : ""
        ].filter(Boolean);
        pill.title = detail.join("；") || (Array.isArray(itemQuote?.quality_reasons) ? itemQuote.quality_reasons.join("；") : "未加载行情");
        cell.append(pill);
        return cell;
      })()
    );
    const actionCell = document.createElement("td"), action = document.createElement("button");
    action.type = "button"; action.className = "button button-secondary research-list-action";
    action.dataset.researchOpenSymbol = item.symbol;
    action.dataset.researchOpenName = item.name || item.symbol;
    action.textContent = "查看详情";
    actionCell.append(action); tr.append(action); root.append(tr);
  });
  const quoteScope = state.watchlistQuoteStatus === "loaded"
    ? " · 已读取自选股批量行情"
    : state.watchlistQuoteStatus === "error" && state.watchlistQuotes.size
      ? " · 批量刷新失败，显示旧快照"
      : "";
  setText("#research-list-scope", !watchlistState.storageReadable
    ? items.length + " 只当前标的 · 自选不可读取" + quoteScope
    : !watchlistState.storageValid
      ? items.length + " 只当前标的 · 自选格式异常" + quoteScope
      : items.length + " 只 · 当前浏览器列表" + quoteScope);
  renderResearchOverview();
}
function safeReadWatchlistState() {
  try {
    return readWatchlistState(localStorage);
  } catch {
    return { items: [], storageReadable: false, storageValid: false, reason: "unavailable" };
  }
}


function localCalendarDateKey(value) {
  const date = value instanceof Date ? value : new Date(String(value || ""));
  if (!Number.isFinite(date.getTime())) return "";
  return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, "0"), String(date.getDate()).padStart(2, "0")].join("-");
}

function normalizeWatchlistQuoteSymbol(value) {
  try { return comparableMarketSymbol(value); } catch { return String(value || "").trim().toUpperCase(); }
}

function invalidateWatchlistQuotes() {
  state.watchlistQuoteGeneration += 1;
  try { state.watchlistQuoteController?.abort(); } catch {}
  state.watchlistQuoteController = null;
  state.watchlistQuotes.clear();
  state.watchlistQuoteStatus = "idle";
  state.watchlistQuoteMessage = "行情连接配置已变更，旧批量快照已清除，请重新刷新。";
  const button = $("#refresh-watchlist-quotes");
  if (button) {
    button.disabled = false;
    button.textContent = "刷新批量行情";
    button.removeAttribute("aria-busy");
  }
  if (state.view === "watchlist") renderWatchlist();
}

async function refreshWatchlistQuotes() {
  const button = $("#refresh-watchlist-quotes");
  const watchlistState = safeReadWatchlistState();
  if (!watchlistState.storageReadable || !watchlistState.storageValid) {
    state.watchlistQuoteStatus = "error";
    state.watchlistQuoteMessage = "本地自选清单不可读取或格式异常，未发送批量行情请求。";
    renderWatchlist();
    return;
  }
  const symbols = watchlistState.items.map((item) => item.symbol);
  if (!symbols.length) {
    state.watchlistQuoteStatus = "empty";
    state.watchlistQuoteMessage = "当前没有自选股，无需请求批量行情。";
    renderWatchlist();
    return;
  }
  if (!state.settings?.apiKey) {
    state.watchlistQuoteStatus = "needs-key";
    state.watchlistQuoteMessage = "请先在设置中心配置行情 API Key，再刷新批量行情。";
    renderWatchlist();
    return;
  }

  try { state.watchlistQuoteController?.abort(); } catch {}
  const controller = new AbortController();
  const generation = ++state.watchlistQuoteGeneration;
  state.watchlistQuoteController = controller;
  state.watchlistQuoteStatus = "loading";
  state.watchlistQuoteMessage = "正在请求 " + symbols.length + " 只自选股的批量行情…";
  if (button) {
    button.disabled = true;
    button.textContent = "刷新中…";
    button.setAttribute("aria-busy", "true");
  }
  renderWatchlist();

  try {
    const payload = await getMarketQuotes(state.settings, symbols, controller.signal);
    if (controller.signal.aborted || generation !== state.watchlistQuoteGeneration) return;
    const nextQuotes = new Map();
    const items = Array.isArray(payload?.items) ? payload.items : [];
    items.forEach((item) => {
      const key = normalizeWatchlistQuoteSymbol(item?.symbol);
      if (!key) return;
      nextQuotes.set(key, {
        quote: item.quote && typeof item.quote === "object" ? item.quote : null,
        errorCode: String(item.error_code || ""),
        qualityStatus: String(item.quality_status || item.quote?.quality_status || "unknown"),
        fetchedAt: String(payload.fetched_at || "")
      });
    });
    state.watchlistQuotes = nextQuotes;
    state.watchlistQuoteStatus = "loaded";
    state.watchlistQuoteMessage = "批量行情已返回 " + Number(payload.succeeded ?? items.filter((item) => item?.quote).length)
      + " / " + Number(payload.total ?? symbols.length) + " 只；失败 "
      + Number(payload.failed ?? items.filter((item) => !item?.quote).length) + " 只"
      + " · 整体质量 " + qualityLabel(payload.quality_status)
      + " · 快照 " + (payload.fetched_at ? formatDateTime(payload.fetched_at) : "时间未报告");
  } catch (error) {
    if (controller.signal.aborted || generation !== state.watchlistQuoteGeneration) return;
    state.watchlistQuoteStatus = "error";
    state.watchlistQuoteMessage = "批量行情读取失败：" + describeApiError(error, "请求失败。")
      + (state.watchlistQuotes.size ? "；保留上次成功快照并标记为旧数据。" : "");
  } finally {
    if (generation === state.watchlistQuoteGeneration) {
      state.watchlistQuoteController = null;
      if (button) {
        button.disabled = false;
        button.textContent = "刷新批量行情";
        button.removeAttribute("aria-busy");
      }
      renderWatchlist();
    }
  }
}

function renderWatchlist(records, storageReadable, storageValid) {
  const root = $("#watchlist-list");
  root.replaceChildren();
  setText("#watchlist-quote-refresh-status", state.watchlistQuoteMessage);
  if (records === undefined) {
    const readState = safeReadWatchlistState();
    records = readState.items;
    storageReadable = readState.storageReadable;
    storageValid = readState.storageValid;
  }
  const items = Array.isArray(records) ? records : [];
  const storageIsReadable = storageReadable !== false;
  const storageIsValid = storageValid !== false;
  setText("#watchlist-count", storageIsReadable && storageIsValid
    ? items.length + " / " + MAX_WATCHLIST_ITEMS
    : "不可用 / " + MAX_WATCHLIST_ITEMS);
  if (!storageIsReadable || !storageIsValid) {
    const empty = document.createElement("div");
    empty.className = "empty-row watchlist-storage-unavailable";
    empty.textContent = !storageIsReadable
      ? "无法读取浏览器自选股存储。为防止误清空数据，添加、改名和移除已暂停；恢复浏览器存储权限后请重新载入。"
      : "本地自选数据格式异常，已暂停修改以避免覆盖原始数据。请先检查或备份浏览器数据。";
    root.append(empty);
    renderResearchStockList();
    return;
  }
  if (!items.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "暂无自选股。可先在上方输入代码，再点击“加入自选”。";
    root.append(empty);
    renderResearchStockList();
    return;
  }

  const keyword = String($("#watchlist-search")?.value || "").trim().toLocaleLowerCase();
  const preferences = readPreferences();
  const sortMode = $("#watchlist-sort")?.value || preferences.watchlistSort;
  const groupState = readWatchlistGroups(safeWatchlistGroupStorage());
  const selectedGroup = $("#watchlist-group-filter")?.value || "all";
  const visibleItems = items.filter((record) => {
    const matchesKeyword = !keyword || String(record.symbol || "").toLocaleLowerCase().includes(keyword)
      || String(record.name || "").toLocaleLowerCase().includes(keyword);
    return matchesKeyword && (selectedGroup === "all" || (groupState.assignments[record.symbol] || DEFAULT_WATCHLIST_GROUP) === selectedGroup);
  });
  const direction = preferences.watchlistSortDirection === "asc" ? 1 : -1;
  if (sortMode === "symbol") visibleItems.sort((a, b) => direction * a.symbol.localeCompare(b.symbol, "zh-CN"));
  else if (sortMode === "name") visibleItems.sort((a, b) => direction * (a.name.localeCompare(b.name, "zh-CN") || a.symbol.localeCompare(b.symbol)));
  else visibleItems.sort((a, b) => direction * ((Date.parse(a.updated_at || "") || 0) - (Date.parse(b.updated_at || "") || 0)));
  setText("#watchlist-filter-count", keyword
    ? "匹配 " + visibleItems.length + " / " + items.length + " 只"
    : "显示全部 " + items.length + " 只");
  if (!visibleItems.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = keyword ? "没有匹配的自选股。可尝试股票代码或自定义名称。" : "暂无自选股。";
    root.append(empty);
    renderResearchStockList();
    return;
  }

  const recentTasks = safeReadRecentTasks();
  const normalizeLocalSymbol = normalizeWatchlistQuoteSymbol;
  visibleItems.forEach((record) => {
    const row = document.createElement("article");
    row.className = "watchlist-item";
    const main = document.createElement("div");
    main.className = "watchlist-item-main";

    const symbol = document.createElement("strong");
    symbol.className = "watchlist-symbol";
    symbol.textContent = record.symbol;

    const aliasLabel = document.createElement("label");
    aliasLabel.className = "watchlist-name-field";
    const aliasCaption = document.createElement("span");
    aliasCaption.textContent = "自定义名称";
    const aliasInput = document.createElement("input");
    aliasInput.type = "text";
    aliasInput.className = "watchlist-name-input";
    aliasInput.maxLength = 32;
    aliasInput.value = record.name === record.symbol ? "" : record.name;
    aliasInput.placeholder = "可选，最多 32 字";
    aliasInput.setAttribute("aria-label", record.symbol + " 的自定义名称");
    aliasInput.addEventListener("change", () => {
      try {
        const result = renameWatchlistSymbol(record.symbol, aliasInput.value, localStorage);
        renderWatchlist(result.items, result.storageReadable, result.storageValid);
        if (!result.storageReadable || !result.storageValid) {
          setWatchlistFeedback(!result.storageReadable
            ? "无法读取浏览器自选股存储，未修改名称。为避免数据丢失，已暂停更改。"
            : "本地自选数据格式异常，未修改名称；请先检查或备份浏览器数据。", "error");
        } else if (!result.valid) {
          setWatchlistFeedback("股票代码无效，未保存自定义名称。", "error");
        } else if (result.persisted) {
          setWatchlistFeedback("已保存 " + record.symbol + " 的自定义名称。");
        } else {
          setWatchlistFeedback(record.symbol + " 的名称已在当前页面临时更新，但浏览器存储不可用，刷新后可能丢失。", "warning");
        }
      } catch {
        setWatchlistFeedback("自选名称更新失败，浏览器存储不可用。", "error");
      }
    });

    const meta = document.createElement("small");
    meta.className = "watchlist-item-meta";
    meta.textContent = "添加/更新于 " + (record.updated_at ? formatDateTime(record.updated_at) : "时间未记录");
    aliasLabel.append(aliasCaption, aliasInput);
    main.append(symbol, aliasLabel);
    // 更新时间与其他辅助字段统一按用户配置顺序渲染。

    const tracking = document.createElement("div");
    tracking.className = "watchlist-tracking-summary";
    const itemSymbol = normalizeLocalSymbol(record.symbol);
    const quoteMatches = Boolean(state.quote?.symbol) && normalizeLocalSymbol(state.quote.symbol) === itemSymbol;
    const batchQuote = state.watchlistQuotes.get(itemSymbol);
    const currentQuote = batchQuote?.quote || (quoteMatches ? state.quote : null);
    const quoteLine = document.createElement("small");
    quoteLine.className = "watchlist-tracking-line";
    if (batchQuote?.errorCode) {
      quoteLine.textContent = "批量行情不可用：" + batchQuote.errorCode + " · " + qualityLabel(batchQuote.qualityStatus);
    } else if (currentQuote) {
      const oldSnapshot = ["error", "loading"].includes(state.watchlistQuoteStatus) && batchQuote;
      quoteLine.textContent = (oldSnapshot ? "上次成功快照：" : "行情快照：")
        + formatPrice(currentQuote.current_price) + " · " + formatPercent(currentQuote.change_percent)
        + " · " + (currentQuote.quote_time ? formatDateTime(currentQuote.quote_time) : "时间未报告")
        + " · " + qualityLabel(batchQuote?.qualityStatus || currentQuote.quality_status)
        + (batchQuote?.fetchedAt ? " · 批量采集 " + formatDateTime(batchQuote.fetchedAt) : "");
    } else if (state.watchlistQuoteStatus === "loading") {
      quoteLine.textContent = "批量行情读取中…";
    } else if (state.watchlistQuoteStatus === "loaded") {
      quoteLine.textContent = "本次批量响应未包含该证券，质量未知。";
    } else {
      quoteLine.textContent = "当前行情：未加载（可刷新批量行情或点击“查行情”读取）";
    }
    const matchingTasks = recentTasks.filter((task) =>
      Array.isArray(task.symbols) && task.symbols.some((value) => normalizeLocalSymbol(value) === itemSymbol)
    );
    const recentTask = [...matchingTasks]
      .sort((a, b) => (Date.parse(b.updated_at || "") || 0) - (Date.parse(a.updated_at || "") || 0))[0] || null;
    const todayKey = localCalendarDateKey(new Date());
    const todayTask = [...matchingTasks]
      .filter((task) => Boolean(todayKey) && localCalendarDateKey(task.created_at) === todayKey)
      .sort((a, b) => (Date.parse(b.created_at || "") || 0) - (Date.parse(a.created_at || "") || 0))[0] || null;
    const isOtherAnalysisBase = (task) => Boolean(task && String(task.analysis_base || state.settings.analysisBase) !== String(state.settings.analysisBase));
    const todayTaskUsesOtherEndpoint = isOtherAnalysisBase(todayTask);
    const recentTaskUsesOtherEndpoint = isOtherAnalysisBase(recentTask);
    const activeTaskMatches = Boolean(state.taskId) && Array.isArray(state.taskSymbols)
      && state.taskSymbols.some((value) => normalizeLocalSymbol(value) === itemSymbol)
      && (state.taskStatusLoading || Boolean(state.taskStatusError)
        || ["running", "started", "pending", "queued", "processing", "in-progress", "in_progress"].includes(String(state.taskStatus || "").toLowerCase()));
    const todayLine = document.createElement("small");
    todayLine.className = "watchlist-tracking-line" + (todayTask ? " is-today" : " is-unknown");
    if (todayTask) {
      todayLine.textContent = "今日分析：有本地记录" + (todayTaskUsesOtherEndpoint ? "（旧分析 API 地址）" : "") + " · " + taskStatusLabel(todayTask.status) + " · 创建于 " + formatDateTime(todayTask.created_at);
    } else if (!matchingTasks.length) {
      todayLine.textContent = "今日分析：未知（当前浏览器没有该股票的任务引用）";
    } else if (matchingTasks.some((task) => !localCalendarDateKey(task.created_at))) {
      todayLine.textContent = "今日分析：未知（部分旧任务缺少服务端创建时间）";
    } else {
      todayLine.textContent = "今日分析：本地最近任务未发现今日创建记录（不代表服务端无分析）";
    }

    const taskLine = document.createElement("small");
    taskLine.className = "watchlist-tracking-line";
    taskLine.textContent = activeTaskMatches
      ? state.taskStatusLoading
        ? "当前任务：正在确认服务端状态（上次状态：" + taskStatusLabel(state.taskStatus) + "） · " + state.taskId
        : state.taskStatusError
          ? "当前任务：状态读取异常（上次状态：" + taskStatusLabel(state.taskStatus) + (state.taskStatusErrorRetrying ? "，自动重试中" : "，自动刷新已暂停") + "） · " + state.taskId
          : "当前任务：" + taskStatusLabel(state.taskStatus) + " · " + state.taskId
      : recentTask
        ? "最近任务：" + taskStatusLabel(recentTask.status) + (recentTaskUsesOtherEndpoint ? " · 旧分析 API 地址" : "") + " · " + (recentTask.updated_at ? formatDateTime(recentTask.updated_at) : "时间未记录")
        : "最近任务：暂无本地任务记录（不代表从未分析）";
    const resultLine = document.createElement("small");
    resultLine.className = "watchlist-tracking-line";
    const latestResultMatches = Boolean(recentTask && state.lastRunResult
      && state.taskId === recentTask.task_id
      && recentTask.run_id
      && state.lastRunResult.run_id === recentTask.run_id
      && String(recentTask.analysis_base || state.settings.analysisBase) === String(state.taskAnalysisBase || state.settings.analysisBase)
      && Array.isArray(state.taskSymbols)
      && state.taskSymbols.some((value) => normalizeLocalSymbol(value) === itemSymbol));
    const activeRunReadMatches = Boolean(recentTask
      && state.taskId === recentTask.task_id
      && String(recentTask.analysis_base || state.settings.analysisBase) === String(state.taskAnalysisBase || state.settings.analysisBase)
      && Array.isArray(state.taskSymbols)
      && state.taskSymbols.some((value) => normalizeLocalSymbol(value) === itemSymbol)
      && ["loading", "retrying", "error"].includes(state.runResultReadStatus));
    if (latestResultMatches) {
      const corePass = state.lastRunResult.quality?.core_pass;
      const qualityState = corePass === true ? "核心质量门禁通过" : corePass === false ? "核心质量门禁未通过" : "核心质量门禁未报告";
      const warnings = Array.isArray(state.lastRunResult.warnings) ? " · 警告 " + state.lastRunResult.warnings.length + " 项" : "";
      const readState = state.runResultReadStatus;
      const readLabel = readState === "loading"
        ? "本页保留旧结果（正在重读，尚未确认）："
        : readState === "retrying"
          ? "本页保留旧结果（重试中，尚未确认）："
          : readState === "error"
            ? "本页保留旧结果（本次读取失败）："
            : "本页已载入结果：";
      const readError = ["retrying", "error"].includes(readState) && state.runResultReadError
        ? " · 重读提示：" + state.runResultReadError
        : "";
      resultLine.textContent = readLabel + runStatusLabel(state.lastRunResult.status) + " · " + qualityState + warnings + readError;
    } else if (activeRunReadMatches) {
      const readState = state.runResultReadStatus;
      const readLabel = readState === "loading"
        ? "当前 Run 结果：正在读取服务端结果"
        : readState === "retrying"
          ? "当前 Run 结果：暂时不可读，正在自动重试"
          : "当前 Run 结果：读取失败";
      const readError = state.runResultReadError ? " · " + state.runResultReadError : "";
      resultLine.textContent = readLabel + readError;
    } else {
      resultLine.textContent = recentTask ? "结果详情：尚未载入当前页面" : "分析结果：未知";
    }
    const displayFieldNodes = {
      updated: meta,
      quote: quoteLine,
      today: todayLine,
      task: taskLine,
      result: resultLine
    };
    const visibleTrackingNodes = (preferences.watchlistDisplayOrder || WATCHLIST_DISPLAY_FIELDS)
      .filter((field) => preferences.watchlistDisplayFields.includes(field))
      .map((field) => displayFieldNodes[field])
      .filter(Boolean);
    if (visibleTrackingNodes.length) {
      tracking.append(...visibleTrackingNodes);
      main.append(tracking);
    }

    const actions = document.createElement("div");
    actions.className = "watchlist-actions";
    for (const [action, label] of [["query", "查行情"], ["analyze", "分析"], ["remove", "移除"]]) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "watchlist-action" + (action === "remove" ? " watchlist-action-muted" : "");
      button.dataset.watchlistAction = action;
      button.dataset.symbol = record.symbol;
      button.textContent = label;
      if (action === "remove") button.setAttribute("aria-label", "移除自选 " + record.symbol);
      actions.append(button);
    }
    const groupSelect = document.createElement("select");
    groupSelect.className = "watchlist-item-group";
    groupSelect.dataset.watchlistGroupSymbol = record.symbol;
    groupSelect.setAttribute("aria-label", record.symbol + " 所属分组");
    groupState.groups.forEach((group) => {
      const option = document.createElement("option");
      option.value = group; option.textContent = group; groupSelect.append(option);
    });
    groupSelect.value = groupState.assignments[record.symbol] || DEFAULT_WATCHLIST_GROUP;
    actions.append(groupSelect);
    row.append(main, actions);
    root.append(row);
  });
  renderResearchStockList();
}

function parseWatchlistImportLine(line) {
  const cells = [];
  let value = "", quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (char === '"') {
      if (quoted && line[index + 1] === '"') { value += '"'; index += 1; }
      else quoted = !quoted;
    } else if ((char === "," || char === "\t" || char === ";") && !quoted) {
      cells.push(value.trim()); value = "";
    } else value += char;
  }
  cells.push(value.trim());
  return cells;
}

function buildWatchlistImportPreview(rawText) {
  const existingState = safeReadWatchlistState();
  if (!existingState.storageReadable || !existingState.storageValid) {
    return { rows: [], accepted: [], error: "当前本地自选数据不可读取或格式异常，已暂停导入。" };
  }
  const lines = String(rawText || "").replace(/^\uFEFF/, "").split(/\r?\n/).filter((line) => line.trim());
  const rows = [];
  const accepted = [];
  const seen = new Set(existingState.items.map((item) => comparableMarketSymbol(item.symbol)));
  const remaining = Math.max(0, MAX_WATCHLIST_ITEMS - existingState.items.length);
  let capacity = remaining;
  lines.forEach((line, index) => {
    const cells = parseWatchlistImportLine(line);
    const rawSymbol = String(cells[0] || "").trim().toUpperCase();
    const rawName = String(cells[1] || "").trim();
    if (index === 0 && /^(代码|股票代码|symbol|code)$/i.test(rawSymbol)) return;
    let symbol = rawSymbol;
    if (/^\d{6}$/.test(symbol)) {
      if (symbol.startsWith("6")) symbol += ".SH";
      else if (/^[03]/.test(symbol)) symbol += ".SZ";
      else if (/^(?:[48]|92)/.test(symbol)) symbol += ".BJ";
    }
    if (!/^\d{6}(?:\.(SH|SZ|BJ))?$/.test(symbol)) {
      rows.push({ line: index + 1, symbol: rawSymbol, name: rawName, status: "invalid", message: "代码格式无效" });
      return;
    }
    const identity = comparableMarketSymbol(symbol);
    if (seen.has(identity)) {
      rows.push({ line: index + 1, symbol, name: rawName, status: "duplicate", message: "已存在或文件内重复" });
      return;
    }
    seen.add(identity);
    if (capacity <= 0) {
      rows.push({ line: index + 1, symbol, name: rawName, status: "limit", message: "超过自选股上限" });
      return;
    }
    capacity -= 1;
    const record = { symbol, name: rawName || symbol };
    accepted.push(record);
    rows.push({ line: index + 1, ...record, status: "ready", message: "可导入" });
  });
  return {
    rows, accepted, remaining, counts: {
      total: rows.length,
      ready: accepted.length,
      duplicate: rows.filter((item) => item.status === "duplicate").length,
      invalid: rows.filter((item) => item.status === "invalid").length,
      limit: rows.filter((item) => item.status === "limit").length
    }
  };
}

function renderWatchlistImportPreview(preview) {
  const root = $("#watchlist-import-preview");
  const confirm = $("#watchlist-import-confirm");
  if (!root || !confirm) return;
  root.replaceChildren();
  state.watchlistImportPreview = preview;
  if (preview.error) {
    root.textContent = preview.error;
    confirm.disabled = true;
    setText("#watchlist-import-summary", preview.error);
    return;
  }
  const counts = preview.counts;
  setText("#watchlist-import-summary", "可导入 " + counts.ready + " 只 · 重复 " + counts.duplicate
    + " 只 · 无效 " + counts.invalid + " 条 · 超出上限 " + counts.limit + " 只");
  const table = document.createElement("table");
  const head = document.createElement("thead");
  const headerRow = document.createElement("tr");
  ["行", "代码", "名称", "结果"].forEach((label) => {
    const cell = document.createElement("th");
    cell.textContent = label;
    headerRow.append(cell);
  });
  head.append(headerRow);
  table.append(head);
  const body = document.createElement("tbody");
  preview.rows.forEach((item) => {
    const row = document.createElement("tr");
    [item.line, item.symbol, item.name || "—", item.message].forEach((value) => {
      const cell = document.createElement("td"); cell.textContent = String(value); row.append(cell);
    });
    body.append(row);
  });
  table.append(body);
  root.append(table);
  confirm.disabled = counts.ready === 0;
}

function previewWatchlistImport() {
  const preview = buildWatchlistImportPreview($("#watchlist-import-text").value);
  renderWatchlistImportPreview(preview);
}

function confirmWatchlistImport() {
  const preview = state.watchlistImportPreview;
  if (!preview || !Array.isArray(preview.accepted) || !preview.accepted.length) return;
  const result = importWatchlistSymbols(preview.accepted, localStorage);
  if (!result.storageReadable || !result.storageValid || !result.persisted) {
    setWatchlistFeedback("导入未能确认完整写入浏览器存储，请检查存储空间；不会报告导入成功。", "error");
    return;
  }
  $("#watchlist-import-text").value = "";
  $("#watchlist-import-preview").replaceChildren();
  $("#watchlist-import-confirm").disabled = true;
  setText("#watchlist-import-summary", "已导入 " + result.imported.length + " 只股票。");
  state.watchlistImportPreview = null;
  renderWatchlist();
  setWatchlistFeedback("已导入 " + result.imported.length + " 只股票；重复、无效及超出上限的记录未写入。");
}

function addCurrentSymbolToWatchlist() {
  let symbol;
  try {
    symbol = normalizeSymbol($("#symbol-input").value);
  } catch (error) {
    setWatchlistFeedback(error.message, "error");
    return;
  }

  let defaultName = symbol;
  if (state.selectedStockSearchItem?.symbol === symbol && state.selectedStockSearchItem.name) {
    defaultName = state.selectedStockSearchItem.name;
  }
  if (state.quote?.name && state.quote?.symbol && sameMarketSymbol(state.quote.symbol, symbol)) {
    defaultName = state.quote.name;
  }

  try {
    const result = addWatchlistSymbol(symbol, defaultName, localStorage);
    if (!result.valid) {
      setWatchlistFeedback("股票代码格式无效，未加入自选。", "error");
      return;
    }
    renderWatchlist(result.items, result.storageReadable, result.storageValid);
    if (!result.storageReadable || !result.storageValid) {
      setWatchlistFeedback(!result.storageReadable
        ? "无法读取浏览器自选股存储，未添加或覆盖任何数据。请检查浏览器存储权限后重试。"
        : "本地自选数据格式异常，未添加或覆盖任何数据；请先检查或备份浏览器数据。", "error");
      return;
    }
    if (!result.persisted) {
      setWatchlistFeedback(symbol + " 已在当前页面临时加入自选，但浏览器存储不可用，刷新后可能丢失。", "warning");
      return;
    }
    setWatchlistFeedback(result.added ? "已将 " + symbol + " 加入本地自选。" : symbol + " 已在自选列表中，已更新时间。");
  } catch {
    setWatchlistFeedback("浏览器本地存储不可用，无法保存自选股。", "error");
  }
}

function handleWatchlistAction(event) {
  const button = event.target.closest("button[data-watchlist-action]");
  if (!button) return;
  const symbol = button.dataset.symbol;
  const action = button.dataset.watchlistAction;

  if (action === "remove") {
    try {
      const result = removeWatchlistSymbol(symbol, localStorage);
      renderWatchlist(result.items, result.storageReadable, result.storageValid);
      if (!result.storageReadable || !result.storageValid) {
        setWatchlistFeedback(!result.storageReadable
          ? "无法读取浏览器自选股存储，未移除或覆盖任何数据。请检查浏览器存储权限后重试。"
          : "本地自选数据格式异常，未移除或覆盖任何数据；请先检查或备份浏览器数据。", "error");
      } else if (!result.valid) {
        setWatchlistFeedback("股票代码无效，未修改自选列表。", "error");
      } else if (!result.removed) {
        setWatchlistFeedback(symbol + " 已不在当前自选列表中。");
      } else if (result.persisted) {
        setWatchlistFeedback("已从本地自选中移除 " + symbol + "。");
      } else {
        setWatchlistFeedback(symbol + " 已在当前页面临时移除，但浏览器存储不可用，刷新后可能恢复。", "warning");
      }
    } catch {
      setWatchlistFeedback("自选股移除失败，浏览器存储不可用。", "error");
    }
    return;
  }

  if (action === "query") {
    $("#symbol-input").value = symbol;
    setView("stocks");
    void runQuoteQuery();
    return;
  }

  if (action === "analyze") startAnalysisForSymbol(symbol);
}

function fillTaskSymbolsFromWatchlist() {
  const help = $("#task-symbol-help");
  const watchlistState = safeReadWatchlistState();
  const records = watchlistState.items;
  if (!watchlistState.storageReadable || !watchlistState.storageValid) {
    help.textContent = !watchlistState.storageReadable
      ? "无法读取浏览器自选股存储，未载入任何股票。请检查浏览器存储权限后重试。"
      : "本地自选数据格式异常，未载入任何股票。请先检查或备份浏览器数据。";
    return;
  }
  if (!records.length) {
    help.textContent = "当前没有可载入的本地自选股，请先在自选股页添加股票。";
    return;
  }

  try {
    const symbols = parseAnalysisSymbols(records.map((record) => record.symbol).join(","));
    $("#task-symbols").value = symbols.join(", ");
    $("#task-type").value = "watchlist_scan";
    help.textContent = "已载入 " + symbols.length + " 只本地自选股到输入框；任务尚未提交。";
  } catch (error) {
    help.textContent = "无法载入本地自选股：" + error.message;
  }
}

function setNotice(message, kind = "info") {
  const notice = $("#market-notice");
  notice.textContent = message;
  notice.className = `notice notice-${kind}`;
  notice.hidden = !message;
}


function cancelStockSearchRequest() {
  if (state.stockSearchTimer) {
    clearTimeout(state.stockSearchTimer);
    state.stockSearchTimer = null;
  }
  if (state.stockSearchController) {
    state.stockSearchController.abort();
    state.stockSearchController = null;
  }
  state.stockSearchGeneration += 1;
}

function hideStockSearch() {
  const dropdown = $("#stock-search-dropdown");
  const input = $("#symbol-input");
  if (dropdown) dropdown.hidden = true;
  if (input) {
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
  }
  state.stockSearchActiveIndex = -1;
}

function setStockSearchMessage(message, { show = true } = {}) {
  const dropdown = $("#stock-search-dropdown");
  const status = $("#stock-search-status");
  const results = $("#stock-search-results");
  if (!dropdown || !status || !results) return;
  status.textContent = message;
  dropdown.hidden = !show;
  $("#symbol-input").setAttribute("aria-expanded", String(show));
}

function updateStockSearchActiveOption() {
  const input = $("#symbol-input");
  const results = $("#stock-search-results");
  if (!input || !results) return;
  const options = results.querySelectorAll("[data-stock-search-index]");
  options.forEach((option, index) => {
    option.setAttribute("aria-selected", String(index === state.stockSearchActiveIndex));
  });
  const active = results.querySelector('[aria-selected="true"]');
  if (active) {
    input.setAttribute("aria-activedescendant", active.id);
    active.scrollIntoView?.({ block: "nearest" });
  } else {
    input.removeAttribute("aria-activedescendant");
  }
}

function renderStockSearchResults(items, stale = false) {
  const root = $("#stock-search-results");
  if (!root) return;
  root.replaceChildren();
  state.stockSearchResults = Array.isArray(items) ? items.filter((item) =>
    item && typeof item.symbol === "string" && typeof item.code === "string"
    && typeof item.name === "string" && typeof item.exchange === "string"
  ) : [];
  state.stockSearchActiveIndex = -1;
  state.stockSearchResults.forEach((item, index) => {
    const option = document.createElement("button");
    option.type = "button";
    option.className = "stock-search-option";
    option.id = "stock-search-option-" + state.stockSearchGeneration + "-" + index;
    option.dataset.stockSearchIndex = String(index);
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", "false");

    const main = document.createElement("span");
    main.className = "stock-search-option-main";
    const name = document.createElement("span");
    name.className = "stock-search-option-name";
    name.textContent = item.name;
    const symbol = document.createElement("span");
    symbol.className = "stock-search-option-symbol";
    symbol.textContent = item.symbol;
    main.append(name, symbol);

    const exchange = document.createElement("span");
    exchange.className = "stock-search-option-exchange";
    exchange.textContent = item.exchange;
    option.append(main, exchange);
    root.append(option);
  });
  const status = stale
    ? "证券目录刷新暂时失败，以下为缓存目录结果，请留意数据时效。"
    : "找到 " + state.stockSearchResults.length + " 条匹配结果。";
  setStockSearchMessage(status, { show: true });
  updateStockSearchActiveOption();
}

async function loadStockSearch(query, generation) {
  const controller = new AbortController();
  state.stockSearchController = controller;
  try {
    const snapshot = await searchStocks(state.settings, query, { limit: 10, signal: controller.signal });
    if (generation !== state.stockSearchGeneration || $("#symbol-input").value.trim() !== query) return;
    const items = Array.isArray(snapshot?.items) ? snapshot.items : [];
    if (!items.length) {
      state.stockSearchResults = [];
      $("#stock-search-results").replaceChildren();
      setStockSearchMessage("没有匹配的股票，请检查名称或代码。", { show: true });
      return;
    }
    renderStockSearchResults(items, snapshot.quality_status === "stale");
  } catch (error) {
    if (controller.signal.aborted || generation !== state.stockSearchGeneration) return;
    state.stockSearchResults = [];
    $("#stock-search-results").replaceChildren();
    const message = error.status === 404
      ? "当前部署的行情服务尚未提供股票搜索接口，请更新 market-data 服务后再使用。"
      : error.status === 401 || error.status === 403
        ? "行情服务 API Key 验证失败，请检查连接与密钥设置。"
        : "股票搜索失败：" + errorMessage(error);
    setStockSearchMessage(message, { show: true });
  } finally {
    if (generation === state.stockSearchGeneration) state.stockSearchController = null;
  }
}

function scheduleStockSearch() {
  if (state.stockSearchTimer) clearTimeout(state.stockSearchTimer);
  if (state.stockSearchController) {
    state.stockSearchController.abort();
    state.stockSearchController = null;
  }
  const generation = ++state.stockSearchGeneration;
  state.stockSearchResults = [];
  state.stockSearchActiveIndex = -1;
  $("#stock-search-results").replaceChildren();

  const query = $("#symbol-input").value.trim();
  if (!query) {
    setStockSearchMessage("", { show: false });
    return;
  }
  if (!state.settings.apiKey) {
    setStockSearchMessage("请先在连接与密钥设置中配置行情服务 API Key。", { show: true });
    return;
  }
  setStockSearchMessage("正在搜索证券目录…", { show: true });
  state.stockSearchTimer = setTimeout(() => {
    state.stockSearchTimer = null;
    void loadStockSearch(query, generation);
  }, 260);
}

function chooseStockSearchResult(item) {
  if (!item) return;
  cancelStockSearchRequest();
  state.selectedStockSearchItem = item;
  $("#symbol-input").value = item.symbol;
  hideStockSearch();
  setText("#query-hint", "已选择 " + item.name + "（" + item.symbol + "），正在加载行情与日 K。");
  void runQuoteQuery();
}

function handleStockSearchKeydown(event) {
  if (event.key === "Escape") {
    if (!$("#stock-search-dropdown").hidden) {
      event.preventDefault();
      cancelStockSearchRequest();
      hideStockSearch();
    }
    return;
  }
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    if ($("#stock-search-dropdown").hidden || !state.stockSearchResults.length) return;
    event.preventDefault();
    const delta = event.key === "ArrowDown" ? 1 : -1;
    const length = state.stockSearchResults.length;
    state.stockSearchActiveIndex = state.stockSearchActiveIndex < 0
      ? (delta > 0 ? 0 : length - 1)
      : (state.stockSearchActiveIndex + delta + length) % length;
    updateStockSearchActiveOption();
    return;
  }
  if (event.key === "Enter" && !$("#stock-search-dropdown").hidden) {
    if (state.stockSearchResults.length === 1) {
      event.preventDefault();
      chooseStockSearchResult(state.stockSearchResults[0]);
    } else if (state.stockSearchActiveIndex >= 0) {
      event.preventDefault();
      chooseStockSearchResult(state.stockSearchResults[state.stockSearchActiveIndex]);
    } else if (state.stockSearchResults.length > 1) {
      event.preventDefault();
      setStockSearchMessage("找到多条匹配结果，请先用方向键选择一只股票。", { show: true });
    }
  }
}

function handleQuoteFormSubmit(event) {
  const raw = $("#symbol-input").value.trim();
  try {
    normalizeSymbol(raw);
    cancelStockSearchRequest();
    hideStockSearch();
    void runQuoteQuery(event);
  } catch {
    event.preventDefault();
    if (state.stockSearchResults.length === 1) {
      chooseStockSearchResult(state.stockSearchResults[0]);
    } else if (state.stockSearchResults.length > 1) {
      setStockSearchMessage("找到多条匹配结果，请选择要查询的股票。", { show: true });
    } else {
      scheduleStockSearch();
      setStockSearchMessage("请从搜索结果中选择股票，再查询行情。", { show: true });
    }
  }
}

function serviceHealthState(result) {
  if (!result?.ok) return "failed";
  const status = String(result.data?.status || "").trim().toLowerCase();
  if (["ok", "ready", "healthy", "alive", "running"].includes(status)) return "healthy";
  if (["failed", "unhealthy", "unavailable", "error", "down"].includes(status)) return "failed";
  if (["degraded", "partial"].includes(status)) return "degraded";
  return "unknown";
}

function setQualityPill(target, status, customText = "") {
  target.className = `quality-pill quality-${String(status || "unknown").toLowerCase()}`;
  target.textContent = customText || qualityLabel(status);
}

function renderNotificationState(snapshot) {
  const current = resolveNotificationViewState(snapshot);
  document.querySelectorAll("[data-notification-state]").forEach((node) => {
    node.hidden = node.dataset.notificationState !== current;
  });
  const status = $("#notification-list-status");
  status.textContent = current === "unavailable" ? "接口尚未接入"
    : current === "loading" ? "正在加载"
    : current === "error" ? "读取失败"
    : current === "empty" ? "无匹配记录" : "已加载";
  status.className = "availability-pill " + (current === "loaded" || current === "empty" ? "is-readonly" : "is-unavailable");
  return current;
}

function renderChatState(snapshot) {
  const current = resolveChatViewState(snapshot);
  document.querySelectorAll("[data-chat-state]").forEach((node) => {
    node.hidden = node.dataset.chatState !== current;
  });
  const status = $("#chat-capability-status");
  status.textContent = current === "unavailable" ? "Agent API 待接入"
    : current === "loading" ? "正在载入"
    : current === "error" ? "会话读取失败"
    : current === "empty" ? "空会话" : "会话已载入";
  status.className = "availability-pill " + (current === "loaded" || current === "empty" ? "is-readonly" : "is-unavailable");
  return current;
}

function renderChatStockContext() {
  const context = state.chatStockContext;
  setText("#chat-stock-context", context ? (context.name ? context.name + " · " : "") + context.symbol : "未选择股票上下文");
  setText("#chat-stock-context-status", context
    ? "来自当前页面最近一次成功的真实行情查询。此上下文仅用于本页临时 UI 状态，尚未发送到任何模型。"
    : "没有股票上下文；为避免串用旧会话，不会自动复用历史上下文。");
  $("#chat-clear-context").disabled = !context;
  $("#chat-clear-context").setAttribute("aria-disabled", String(!context));
}

function syncChatStockContextFromMarket() {
  if (state.chatStockContextCleared) {
    state.chatStockContext = null;
    renderChatStockContext();
    return;
  }
  state.chatStockContext = state.quote && typeof state.quote.symbol === "string" && state.quote.symbol.trim()
    ? { symbol: state.quote.symbol.trim(), name: String(state.quote.name || state.quote.symbol).trim() }
    : null;
  renderChatStockContext();
}

const screeningDraftConditions = [];

function renderScreeningDraftConditions() {
  const root = $("#screening-condition-list");
  const summary = buildScreeningDraftSummary(screeningDraftConditions);
  root.replaceChildren();
  if (!summary.count) {
    const empty = document.createElement("li");
    empty.className = "empty-row";
    empty.textContent = "尚未添加条件";
    root.append(empty);
  } else {
    summary.conditions.forEach((label, index) => {
      const item = document.createElement("li");
      item.className = "screening-condition-item";
      const text = document.createElement("span");
      text.textContent = label;
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "button button-secondary";
      remove.dataset.screeningConditionRemove = String(index);
      remove.textContent = "移除";
      remove.setAttribute("aria-label", "移除条件：" + label);
      item.append(text, remove);
      root.append(item);
    });
  }
  const summaryRoot = $("#screening-request-summary");
  if (summaryRoot) {
    const paragraph = summaryRoot.querySelector("p");
    if (paragraph) paragraph.textContent = summary.description;
  }
}

$("#screening-condition-add").addEventListener("click", () => {
  const result = createScreeningCondition(
    $("#screening-condition-field").value,
    $("#screening-condition-operator").value,
    $("#screening-condition-value").value
  );
  const feedback = $("#screening-condition-feedback");
  if (!result.valid) {
    feedback.textContent = result.reason === "range" ? "数值超出该指标允许范围，请调整后重试。" : "请选择有效指标并输入有效数值。";
    feedback.dataset.state = "error";
    return;
  }
  screeningDraftConditions.push(result.condition);
  $("#screening-condition-value").value = "";
  feedback.textContent = "已添加条件。当前仅为页面内存草稿。";
  feedback.dataset.state = "success";
  renderScreeningDraftConditions();
});
$("#screening-condition-list").addEventListener("click", (event) => {
  const button = event.target.closest("[data-screening-condition-remove]");
  if (!button) return;
  const index = Number(button.dataset.screeningConditionRemove);
  if (!Number.isInteger(index) || index < 0 || index >= screeningDraftConditions.length) return;
  screeningDraftConditions.splice(index, 1);
  $("#screening-condition-feedback").textContent = "已移除条件。";
  renderScreeningDraftConditions();
});
renderScreeningDraftConditions();

function renderScreeningCapability(snapshot) {
  const current = resolveScreeningCapability(snapshot);
  const labels = {
    enabled: "选股能力已启用", disabled: "选股功能已关闭", unknown: "选股能力未知",
    unavailable: "选股 API 尚未接入", loading: "正在检查选股能力", error: "选股能力检查失败"
  };
  const pill = $("#screening-capability-pill");
  pill.textContent = labels[current] || labels.unknown;
  pill.className = "availability-pill " + (current === "enabled" ? "is-local" : current === "disabled" ? "is-readonly" : "is-unavailable");
  setText("#screening-summary-capability", current === "enabled" ? "已启用" : current === "disabled" ? "已关闭" : current === "unknown" ? "未知" : current === "loading" ? "检查中" : current === "error" ? "检查失败" : "不可用");
  setText("#screening-capability-title", labels[current] || labels.unknown);
  const descriptions = {
    enabled: "服务端报告选股能力已启用。策略、表单 schema 和提交端点仍需分别确认。",
    disabled: "服务端明确报告选股功能已关闭；不会将请求错误当作功能关闭。",
    unknown: "服务端尚未返回选股能力状态。",
    unavailable: "当前 Go 服务没有策略列表、筛选配置、扫描任务、候选结果或选股历史 API。不能提交扫描，也不会用普通分析任务伪装成选股。",
    loading: "正在读取服务端选股能力声明。",
    error: "选股能力检查发生错误；错误不代表功能已关闭。"
  };
  setText("#screening-capability-description", descriptions[current] || descriptions.unknown);
  return current;
}

function renderScreeningTask(snapshot) {
  const current = resolveScreeningTaskState(snapshot);
  const labels = {
    idle: "尚未发起扫描", queued: "扫描已排队", running: "扫描运行中",
    completed: "扫描已完成", partial: "扫描部分完成", failed: "扫描失败",
    unavailable: "任务 API 尚未接入", loading: "正在读取任务", error: "读取失败", unknown: "状态未知"
  };
  document.querySelectorAll("[data-screening-run-state]").forEach((node) => {
    node.hidden = node.dataset.screeningRunState !== current;
  });
  const pill = $("#screening-run-status-pill");
  pill.textContent = labels[current] || labels.unknown;
  pill.className = "availability-pill " + (current === "completed" ? "is-local" : ["partial", "failed", "error"].includes(current) ? "is-unavailable" : "is-readonly");
  setText("#screening-summary-run", current === "idle" ? "未开始" : labels[current] || "未知");
  return current;
}

function renderScreeningCandidates(snapshot) {
  const current = resolveScreeningCollectionState(snapshot, "candidates");
  document.querySelectorAll("[data-screening-results-state]").forEach((node) => {
    node.hidden = node.dataset.screeningResultsState !== current;
  });
  const pill = $("#screening-results-status-pill");
  pill.textContent = current === "loaded" ? "候选已加载" : current === "empty" ? "无匹配候选" : current === "loading" ? "正在加载" : current === "error" ? "读取失败" : "候选 API 待接入";
  pill.className = "availability-pill " + (current === "loaded" || current === "empty" ? "is-readonly" : "is-unavailable");
  setText("#screening-summary-candidates", current === "loaded" ? String(snapshot.candidates.length) : current === "empty" ? "0" : "—");
  return current;
}

function renderScreeningHistory(snapshot) {
  const current = resolveScreeningCollectionState(snapshot, "items");
  document.querySelectorAll("[data-screening-history-state]").forEach((node) => {
    node.hidden = node.dataset.screeningHistoryState !== current;
  });
  const pill = $("#screening-history-status-pill");
  pill.textContent = current === "loaded" ? "历史已加载" : current === "empty" ? "无匹配历史" : current === "loading" ? "正在加载" : current === "error" ? "读取失败" : "历史 API 待接入";
  pill.className = "availability-pill " + (current === "loaded" || current === "empty" ? "is-readonly" : "is-unavailable");
  return current;
}

function renderAlertCapability(snapshot) {
  const current = resolveAlertCapability(snapshot);
  const labels = { enabled: "告警能力已启用", disabled: "告警功能已关闭", unknown: "告警能力未知", unavailable: "告警 API 尚未接入", loading: "正在检查告警能力", error: "告警能力检查失败" };
  const pill = $("#alert-capability-pill");
  pill.textContent = labels[current] || labels.unknown;
  pill.className = "availability-pill " + (current === "enabled" ? "is-local" : current === "disabled" ? "is-readonly" : "is-unavailable");
  setText("#alert-capability-title", labels[current] || labels.unknown);
  const descriptions = {
    enabled: "服务端报告告警能力已启用；具体规则类型、写权限和评估能力仍需分别确认。",
    disabled: "服务端明确报告告警功能已关闭；请求错误不能被解释为已关闭。",
    unknown: "服务端没有返回可识别的告警能力状态。",
    unavailable: "当前后端没有规则 CRUD、后台评估调度、触发记录或通知投递尝试查询 API。此页面不会假保存规则、显示模拟触发记录或虚构通知成功状态。",
    loading: "正在请求服务端告警能力声明。",
    error: "告警能力检查发生错误；错误不代表功能已经关闭。"
  };
  setText("#alert-capability-description", descriptions[current] || descriptions.unknown);
  return current;
}

function renderAlertEvaluation(snapshot) {
  const current = resolveAlertEvaluationState(snapshot);
  document.querySelectorAll("[data-alert-evaluation-state]").forEach((node) => {
    node.hidden = node.dataset.alertEvaluationState !== current;
  });
  const labels = {
    idle: "尚未执行评估", loading: "正在评估", triggered: "规则已触发",
    "not-triggered": "规则未触发", "evaluation-error": "评估执行错误",
    degraded: "评估结果降级", skipped: "本次评估已跳过", error: "评估请求失败",
    unavailable: "评估 API 待接入", unknown: "评估状态未知"
  };
  const pill = $("#alert-evaluation-status-pill");
  pill.textContent = labels[current] || labels.unknown;
  pill.className = "availability-pill " + (current === "triggered" ? "is-unavailable" : current === "not-triggered" ? "is-local" : "is-readonly");
  return current;
}

function renderAlertCollection(snapshot, field, selector, pillSelector, summarySelector, label) {
  const current = resolveAlertCollectionState(snapshot, field);
  document.querySelectorAll(selector).forEach((node) => {
    const stateKey = node.getAttribute("data-alert-" + label + "-state");
    node.hidden = stateKey !== current;
  });
  const pill = $(pillSelector);
  pill.textContent = current === "loaded" ? "记录已加载" : current === "empty" ? "无匹配记录" : current === "loading" ? "正在加载" : current === "error" ? "读取失败" : "API 待接入";
  pill.className = "availability-pill " + (current === "loaded" || current === "empty" ? "is-readonly" : "is-unavailable");
  if (summarySelector) setText(summarySelector, current === "loaded" ? String(snapshot[field].length) : current === "empty" ? "0" : "—");
  return current;
}

function errorMessage(error) {
  return describeApiError(error, "请求失败，请检查服务状态与浏览器网络面板。");
}

function getQuerySymbol() {
  return normalizeSymbol($("#symbol-input").value);
}

const VIEW_META = Object.freeze({
  market: { title: "首页" },
  stocks: { title: "个股行情" },
  watchlist: { title: "自选股" },
  research: { title: "个股研究" },
  data: { title: "数据中心" },
  tasks: { title: "分析任务" },
  notifications: { title: "通知中心" },
  chat: { title: "问股工作台" },
  screening: { title: "条件选股" },
  alerts: { title: "告警中心" },
  settings: { title: "设置中心" }
});

const REFRESH_BUTTON_CONTEXT = Object.freeze({
  market: { label: "刷新首页数据", title: "刷新核心指数、市场涨跌分布、行业/概念排行、涨跌停池、人气/飙升榜、龙虎榜与交易日历。" },
  stocks: { label: "重新查询个股行情", title: "使用当前输入的股票代码、日 K 数量、复权口径和日期范围重新查询。" },
  watchlist: { label: "刷新自选股列表", title: "重新读取当前浏览器本地保存的自选股，不会向服务器同步列表。" },
  research: { label: "刷新个股研究", title: "检查服务连接并重新加载当前标的的公司资料、财务、筹码与资金流。" },
  tasks: { label: "刷新任务与连接状态", title: "重新检查服务连接；若已选择任务，则重新读取任务状态与阶段事件。" },
  notifications: { label: "检查服务连接（通知接口待接入）", title: "通知中心 API 尚未接入；此操作只刷新行情与分析服务连接状态。" },
  chat: { label: "检查服务连接（问股接口待接入）", title: "问股 API 尚未接入；此操作只刷新行情与分析服务连接状态。" },
  screening: { label: "检查服务连接（选股接口待接入）", title: "选股任务与候选 API 尚未接入；此操作只刷新行情与分析服务连接状态。" },
  alerts: { label: "检查服务连接（告警接口待接入）", title: "告警规则与触发记录 API 尚未接入；此操作只刷新行情与分析服务连接状态。" },
  data: { label: "立即检查服务与数据源能力", title: "检查行情/分析服务存活和就绪状态，并读取 Provider 能力声明；不代表逐 Provider 实时探测。" },
  settings: { label: "检查服务连接", title: "检查当前已保存的行情/分析 API；未保存的连接草稿可在设置中心的连接与服务分组中检查。" }
});

function syncRefreshButtonContext() {
  const button = $("#refresh-button");
  if (!button) return;
  const context = REFRESH_BUTTON_CONTEXT[state.view] || REFRESH_BUTTON_CONTEXT.market;
  button.setAttribute("aria-label", context.label);
  button.title = context.title;
}

function refreshMarketWorkspace() {
  const startModule = (label, operation) => {
    try {
      return Promise.resolve(operation()).then(
        (value) => ({ label, ok: true, value }),
        (error) => ({ label, ok: false, error })
      );
    } catch (error) {
      // 捕获模块启动阶段同步异常，和 Promise rejection 使用同一结果结构。
      return Promise.resolve({ label, ok: false, error });
    }
  };
  const requests = [
    startModule("市场总览", () =>
      marketOverview.refresh(["indices", "breadth", "sectors", "pool", "hot"], "#market-data-updated")),
    startModule("资金与活跃度", () => marketActivity.refresh())
  ];

  // allSettled 语义：单个模块失败不取消等待其他模块，也不让 void 调用产生未处理 rejection。
  return Promise.all(requests).then((results) => {
    const failures = results.filter((result) => !result.ok);
    if (failures.length) {
      const details = failures.map((result) =>
        result.label + "：" + settingsCheckErrorMessage(result.error));
      try {
        setText("#market-data-updated", "市场工作台部分模块刷新失败：" + details.join("；"));
      } catch {
        // 元信息节点不可用时，保留逐模块捕获结果并避免再次抛错。
      }
    }
    return results;
  }).catch((error) => {
    // 最终汇总防线：异常只返回诊断，不向页面初始化或切换流程传播 rejection。
    try {
      setText("#market-data-updated", "市场工作台刷新汇总异常：" + settingsCheckErrorMessage(error));
    } catch {}
    return [];
  });
}

function setView(view, { updateHash = true, refreshData = true } = {}) {
  const legacyMarketView = view === "market-data";
  const requestedView = legacyMarketView ? "market" : view;
  const nextView = Object.hasOwn(VIEW_META, requestedView) ? requestedView : "market";
  const previousView = state.view;
  if (previousView === "settings" && nextView !== "settings") setMarketKeyVisibility(false);
  state.view = nextView;
  if (legacyMarketView && window.location.hash === "#market-data") window.history.replaceState(null, "", "#market");
  syncWatchlistGroupControls();
syncRefreshButtonContext();
  if (nextView === "settings" && previousView !== nextView) openSettings();
  if (refreshData && previousView !== nextView) {
    if (nextView === "market") void refreshMarketWorkspace();
    else if (nextView === "stocks") void refreshSources();
    else if (nextView === "watchlist") renderWatchlist();
    else if (nextView === "research") { stockResearch.syncSymbol(); renderResearchStockList(); }
    else if (nextView === "data") void dataCenter.refresh();
  }
  if (nextView === "chat" && previousView !== "chat") syncChatStockContextFromMarket();
  const runFlowDialog = $("#run-flow-dialog");
  if (nextView !== "tasks" && runFlowDialog?.open) runFlowDialog.close();
  document.querySelectorAll(".view").forEach((section) => {
    section.hidden = section.id !== nextView + "-view";
  });
  document.querySelectorAll(".nav-link, .workspace-tab").forEach((button) => {
    const active = button.dataset.view === nextView;
    button.classList.toggle("active", active);
    if (active) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  });
  const title = VIEW_META[nextView].title;
  setText("#breadcrumb-current", title);
  setText("#view-title", title);
  document.title = title + " · Stock Data Console";
  if (previousView !== nextView) {
    window.scrollTo(0, 0);
    const heading = document.querySelector("#" + nextView + "-view h1");
    if (heading) {
      heading.setAttribute("tabindex", "-1");
      heading.focus({ preventScroll: true });
    }
  }
  if (updateHash && window.location.hash !== "#" + nextView) window.location.hash = nextView;
}

function syncSettingsThemeButtons(theme = readTheme()) {
  document.querySelectorAll("[data-theme-value]").forEach((button) => {
    const active = button.dataset.themeValue === theme;
    button.setAttribute("aria-pressed", String(active));
    button.classList.toggle("is-selected", active);
  });
}


function renderResearchOverview() {
  if (!$("#research-overview-symbol")) return;
  let symbol = "";
  try { symbol = getQuerySymbol(); } catch {}
  const quote = symbol && state.quote && sameMarketSymbol(state.quote.symbol || "", symbol) ? state.quote : null;
  const matched = Boolean(quote);
  const quality = String(quote?.quality_status || "unknown").toLowerCase();
  const qualityNode = $("#research-overview-quality");
  qualityNode.className = "supplement-quality supplement-quality-" + (["valid","degraded","stale","invalid","unknown"].includes(quality) ? quality : "unknown");
  qualityNode.textContent = matched ? qualityLabel(quality) : (symbol ? (state.isQuerying ? "查询中" : "未加载") : "未选择");
  setText("#research-overview-symbol", symbol ? ((matched && quote.name ? quote.name + " · " : "") + symbol) : "尚未选择标的");
  const meta = [];
  if (matched) {
    meta.push(quote.quote_time ? "行情时间 " + formatDateTime(quote.quote_time) : "行情时间未报告");
    meta.push(quote.fetched_at ? "获取时间 " + formatDateTime(quote.fetched_at) : "获取时间未报告");
    meta.push("质量 " + qualityLabel(quality));
  } else meta.push(symbol ? (state.isQuerying ? "正在读取当前标的行情" : "尚无匹配报价") : "请先选择有效股票代码");
  setText("#research-overview-meta", meta.join(" · "));
  const display = (value, formatter) => !matched ? (symbol && state.isQuerying ? "读取中…" : "未加载") : (value == null || value === "" ? "未报告" : formatter(value));
  setText("#research-overview-price", display(quote?.current_price, formatPrice));
  const changeValue = matched ? finiteNumberOrNull(quote.change) : null;
  const changePercentValue = matched ? finiteNumberOrNull(quote.change_percent) : null;
  const change = changeValue === null ? "未报告" : formatPrice(changeValue);
  const percent = changePercentValue === null ? "未报告" : formatPercent(changePercentValue);
  setText("#research-overview-change", matched ? change + " / " + percent : (symbol && state.isQuerying ? "读取中…" : "未加载"));
  $("#research-overview-change").classList.toggle("positive", changePercentValue !== null && changePercentValue > 0);
  $("#research-overview-change").classList.toggle("negative", changePercentValue !== null && changePercentValue < 0);
  setText("#research-overview-amount", display(quote?.amount, formatCompact));
  setText("#research-overview-turnover", display(quote?.turnover_rate, formatPercent));
  setText("#research-overview-volume-ratio", display(quote?.volume_ratio, v => {
    const ratio = finiteNumberOrNull(v);
    return ratio === null ? "未报告" : ratio.toFixed(2);
  }));
  setText("#research-overview-source", matched ? (quote.source || "未报告") : (symbol && state.isQuerying ? "读取中…" : "未加载"));
  let barsSymbol = "";
  try { barsSymbol = normalizeSymbol(state.technicalSymbol || ""); } catch {}
  const hasBars = Boolean(symbol && barsSymbol && sameMarketSymbol(barsSymbol, symbol));
  const latest = hasBars ? [...state.technicalRows].reverse().find(row => row && row.close != null) : null;
  const technical = key => !hasBars ? (symbol && state.isQuerying ? "读取中…" : "未加载") : (Number.isFinite(latest?.[key]) ? latest[key].toFixed(2) : "数据不足");
  setText("#research-overview-ma5", technical("ma5"));
  setText("#research-overview-ma20", technical("ma20"));
  const note = $("#research-overview-note");
  const reasons = matched && Array.isArray(quote.quality_reasons) ? quote.quality_reasons.filter(Boolean) : [];
  note.classList.toggle("is-warning", reasons.length > 0 || (matched && quality !== "valid"));
  if (!symbol) note.textContent = "请先选择有效股票；未选择标的时不会展示上一只股票的快照。";
  else if (!matched && state.isQuerying) note.textContent = "正在读取 " + symbol + " 的实时行情与日 K。";
  else if (!matched) note.textContent = "当前标的尚无匹配行情。可从列表点击“查看详情”加载，或打开图表与日 K 页面查询。";
  else if (reasons.length) note.textContent = "行情质量提示：" + reasons.join("；");
  else if (quality !== "valid") note.textContent = "服务端未提供详细质量原因；当前质量为“" + qualityLabel(quality) + "”，请结合来源与时间判断。";
  else note.textContent = "当前报价与所选股票匹配。MA5/MA20 仅基于当前股票有效日 K 计算；历史数据不足时不估算。";
  ["research-open-chart","research-add-watchlist","research-start-analysis"].forEach(id => { const b = $("#"+id); if (b) b.disabled = !symbol; });
}
function openResearchStock(symbol, name = "") {
  let normalized;
  try { normalized = normalizeSymbol(symbol); } catch (error) { setNotice(error.message, "danger"); return; }
  let q = "", bars = "";
  try { q = state.quote ? normalizeSymbol(state.quote.symbol || "") : ""; } catch {}
  try { bars = state.technicalSymbol ? normalizeSymbol(state.technicalSymbol) : ""; } catch {}
  const quoteMatches = q === normalized && Boolean(state.quote), barsMatch = bars === normalized;
  $("#symbol-input").value = normalized;
  state.selectedStockSearchItem = {symbol:normalized,name:name || normalized};
  setView("research");
  if (!quoteMatches || !barsMatch) void runQuoteQuery();
}
function bindSectionNavigation(linkSelector, dataKey) {
  const links = Array.from(document.querySelectorAll(linkSelector));
  if (!links.length) return;

  const setActive = (targetId) => {
    links.forEach((link) => {
      const active = link.dataset[dataKey] === targetId;
      link.classList.toggle("is-active", active);
      if (active) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");
    });
  };

  const targets = links.map((link) => document.getElementById(link.dataset[dataKey] || "")).filter(Boolean);

  links.forEach((link) => {
    link.addEventListener("click", () => {
      const targetId = link.dataset[dataKey] || "";
      const target = targetId ? document.getElementById(targetId) : null;
      if (!target) return;
      setActive(targetId);
      const reduceMotion = Boolean(window.matchMedia?.("(prefers-reduced-motion: reduce)").matches);
      target.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "start" });

      // 将键盘焦点移动到目标标题，使导航不只改变滚动位置。
      const focusTarget = target.matches("h2, h3, h4")
        ? target
        : target.querySelector("h2, h3, h4") || target;
      if (!focusTarget.hasAttribute("tabindex")) focusTarget.setAttribute("tabindex", "-1");
      focusTarget.focus({ preventScroll: true });
    });
  });

  if (typeof IntersectionObserver === "undefined" || !targets.length) return;
  const observer = new IntersectionObserver(() => {
    // entries 只包含本次跨越观察阈值的元素；根据全部可见目标重算，避免高亮回跳。
    const referenceLine = Math.min(window.innerHeight * 0.28, 220);
    const current = targets
      .map((target) => ({ target, rect: target.getBoundingClientRect() }))
      .filter(({ target, rect }) => target.getClientRects().length > 0
        && rect.bottom >= 120
        && rect.top <= window.innerHeight - 120)
      .sort((a, b) => Math.abs(a.rect.top - referenceLine) - Math.abs(b.rect.top - referenceLine))[0];
    if (current) setActive(current.target.id);
  }, { rootMargin: "-120px 0px -120px 0px", threshold: 0 });
  targets.forEach((target) => observer.observe(target));
}

function openResearchChartView() {
  let symbol;
  try { symbol = getQuerySymbol(); } catch (error) { setNotice(error.message, "danger"); return; }
  let q = "", bars = "";
  try { q = state.quote ? normalizeSymbol(state.quote.symbol || "") : ""; } catch {}
  try { bars = state.technicalSymbol ? normalizeSymbol(state.technicalSymbol) : ""; } catch {}
  const quoteMatches = q === symbol && Boolean(state.quote), barsMatch = bars === symbol;
  setView("stocks");
  if (!quoteMatches || !barsMatch) void runQuoteQuery();
}

function clearBundleSnapshotMetadata() {
  state.bundleSnapshot = null;
  const root = $("#bundle-snapshot-strip");
  if (root) root.hidden = true;
  const feedback = $("#bundle-snapshot-feedback");
  if (feedback) {
    feedback.hidden = true;
    feedback.textContent = "";
  }
  const sources = $("#bundle-snapshot-sources");
  if (sources) sources.replaceChildren();
}

function renderBundleSnapshotMetadata(snapshot) {
  state.bundleSnapshot = snapshot && typeof snapshot === "object" ? snapshot : null;
  const root = $("#bundle-snapshot-strip");
  if (!root) return;
  if (!state.bundleSnapshot) {
    clearBundleSnapshotMetadata();
    return;
  }
  root.hidden = false;
  setText("#bundle-snapshot-id", state.bundleSnapshot.snapshot_id || "未报告");
  setText("#bundle-analysis-reference-time", state.bundleSnapshot.analysis_reference_time ? formatDateTime(state.bundleSnapshot.analysis_reference_time) : "未报告");
  setText("#bundle-fetched-at", state.bundleSnapshot.fetched_at ? formatDateTime(state.bundleSnapshot.fetched_at) : "未报告");
  setText("#bundle-quality-status", state.bundleSnapshot.quality_status ? qualityLabel(state.bundleSnapshot.quality_status) : "未报告");

  const sourceRoot = $("#bundle-snapshot-sources");
  sourceRoot.replaceChildren();
  const sources = state.bundleSnapshot.sources && typeof state.bundleSnapshot.sources === "object"
    ? Object.entries(state.bundleSnapshot.sources).filter(([key, value]) => key && value !== null && value !== undefined && String(value).trim())
    : [];
  if (!sources.length) {
    const empty = document.createElement("span");
    empty.className = "bundle-snapshot-source-empty";
    empty.textContent = "服务端未报告组件来源明细。";
    sourceRoot.append(empty);
  } else {
    const title = document.createElement("strong");
    title.textContent = "组件来源";
    sourceRoot.append(title);
    sources.slice(0, 8).forEach(([key, value]) => {
      const item = document.createElement("span");
      item.className = "bundle-snapshot-source";
      const label = document.createElement("small");
      label.textContent = String(key);
      const content = document.createElement("strong");
      content.textContent = String(value);
      item.append(label, content);
      sourceRoot.append(item);
    });
    if (sources.length > 8) {
      const more = document.createElement("span");
      more.className = "bundle-snapshot-source-empty";
      more.textContent = "另有 " + (sources.length - 8) + " 项来源，完整列表可通过复制元数据查看。";
      sourceRoot.append(more);
    }
  }
  const feedback = $("#bundle-snapshot-feedback");
  feedback.hidden = true;
  feedback.textContent = "";
}

function renderQuote(quote) {
  state.quote = quote;
  if (state.view === "chat") syncChatStockContextFromMarket();
  const symbol = quote.symbol || getQuerySymbol();
  setText("#quote-title", `${quote.name || symbol}  ${symbol}`);
  setText("#quote-subtitle", `来源：${quote.source || "未知"} · 标准化版本：${quote.normalization_version || "—"}`);
  setQualityPill($("#quote-quality"), quote.quality_status);
  setText("#metric-price", formatPrice(quote.current_price));
  const change = formatPrice(quote.change);
  const changePct = formatPercent(quote.change_percent);
  const changeValue = finiteNumberOrNull(quote.change);
  const changePercentValue = finiteNumberOrNull(quote.change_percent);
  setText("#metric-change", changeValue === null && changePercentValue === null ? "暂无涨跌数据" : `${change}（${changePct}）`);
  $("#metric-change").classList.toggle("positive", changeValue !== null && changeValue > 0);
  $("#metric-change").classList.toggle("negative", changeValue !== null && changeValue < 0);
  setText("#metric-amount", formatCompact(quote.amount));
  setText("#metric-volume", formatCompact(quote.volume));
  setText("#metric-turnover", quote.turnover_rate == null ? "—" : formatPercent(quote.turnover_rate));
  const volumeRatio = finiteNumberOrNull(quote.volume_ratio);
  setText("#metric-volume-ratio", `量比 ${volumeRatio === null ? "—" : volumeRatio.toFixed(2)}`);
  setText("#metric-source", quote.source || "未知");
  setText("#metric-quote-time", `行情时间 ${formatDateTime(quote.quote_time)}`);
  renderResearchStockList();

  const reasons = Array.isArray(quote.quality_reasons) ? quote.quality_reasons : [];
  if (reasons.length) {
    setNotice(`行情质量已降级：${reasons.join("、")}`, "warning");
  }
}

function clearQuoteDisplay(symbol, phase = "failed") {
  state.quote = null;
  if (state.view === "chat") syncChatStockContextFromMarket();

  const isLoading = phase === "loading";
  setText("#quote-title", `${symbol} · ${isLoading ? "正在查询行情" : "实时行情不可用"}`);
  setText("#quote-subtitle", isLoading
    ? "正在读取当前标的的实时快照。"
    : "本次请求未取得实时报价；不会沿用之前标的的价格数据。");
  setQualityPill($("#quote-quality"), "unknown", isLoading ? "查询中" : "未取得报价");
  setText("#metric-price", "—");
  setText("#metric-change", isLoading ? "等待本次行情" : "本次报价不可用");
  $("#metric-change").classList.remove("positive", "negative");
  setText("#metric-amount", "—");
  setText("#metric-volume", "—");
  setText("#metric-turnover", "—");
  setText("#metric-volume-ratio", "量比 —");
  setText("#metric-source", "—");
  setText("#metric-quote-time", "行情时间未报告");
  renderResearchStockList();
}

function makeCell(value, className = "") {
  const cell = document.createElement("td");
  if (className) cell.className = className;
  cell.textContent = value == null || value === "" ? "—" : String(value);
  return cell;
}

function renderTechnicalSummary(snapshot = calculateTechnicalIndicators(state.bars)) {
  state.technicalRows = snapshot.rows;
  const latest = snapshot.latest;
  const close = latest?.close;
  [5,20,60,120,250].forEach((period)=>{
    const value=latest?.["ma"+period],valueNode=$("#technical-ma-"+period),deltaNode=$("#technical-ma-"+period+"-change");
    valueNode.textContent=Number.isFinite(value)?value.toFixed(2):"样本不足";
    deltaNode.classList.remove("positive","negative");
    if(Number.isFinite(value)&&Number.isFinite(close)&&value!==0){
      const delta=(close/value-1)*100;
      deltaNode.textContent="收盘价距均线 "+(delta>0?"+":"")+delta.toFixed(2)+"%";
      if(delta>0)deltaNode.classList.add("positive");
      if(delta<0)deltaNode.classList.add("negative");
    } else deltaNode.textContent="至少需要 "+period+" 个有效收盘价";
  });
  const rsi=latest?.rsi14;
  $("#technical-rsi-value").textContent=Number.isFinite(rsi)?rsi.toFixed(2):"样本不足";
  $("#technical-rsi-state").textContent=!Number.isFinite(rsi)?"RSI(14) 至少需要 15 个有效收盘价":rsi>=70?"处于 70 参考线上方":rsi<=30?"处于 30 参考线下方":"位于 30–70 参考区间";
  $("#technical-rsi-state").classList.toggle("is-emphasis",Number.isFinite(rsi)&&(rsi>=70||rsi<=30));
  [["dif",latest?.macdDif],["signal",latest?.macdSignal],["histogram",latest?.macdHistogram]].forEach(([key,value])=>{
    $("#technical-macd-"+key).textContent=Number.isFinite(value)?value.toFixed(3):"—";
  });
  $("#technical-macd-state").textContent=Number.isFinite(latest?.macdHistogram)?"DIF / DEA / 柱值均已形成":"MACD 柱值通常需要至少 34 个有效收盘价";
  setText("#technical-sample-note",snapshot.validCloseCount+" 根有效收盘价 · 当前页面本地计算 · 不构成买卖信号");
  const hasRsi=renderRSIChart($("#technical-rsi-chart"),snapshot.rows);
  const hasMacd=renderMACDChart($("#technical-macd-chart"),snapshot.rows);
  $("#technical-rsi-chart-empty").hidden=hasRsi;
  $("#technical-macd-chart-empty").hidden=hasMacd;
  $("#technical-rsi-chart-empty").textContent=snapshot.validCloseCount<15?"有效收盘价不足 15 个，暂不能计算 RSI(14)。":"RSI 有效点不足，暂不能绘制曲线。";
  $("#technical-macd-chart-empty").textContent=snapshot.validCloseCount<34?"有效收盘价不足 34 个，暂不能形成完整 MACD(12,26,9)。":"MACD 有效点不足，暂不能绘制曲线。";
  $("#technical-indicator-status").textContent=snapshot.validCloseCount?snapshot.validCloseCount+" 根有效收盘价":"暂无日 K 数据";
}

function renderChartView() {
  const isCandles = state.chartView === "candles";
  $("#chart-view-close").classList.toggle("is-active", !isCandles);
  $("#chart-view-close").setAttribute("aria-pressed", String(!isCandles));
  $("#chart-view-candles").classList.toggle("is-active", isCandles);
  $("#chart-view-candles").setAttribute("aria-pressed", String(isCandles));
  $("#chart-title").textContent = isCandles ? "OHLC K 线与成交量" : "收盘价走势";
  $("#chart-legend").classList.toggle("chart-legend-candles", isCandles);
  setText("#chart-legend-label", isCandles ? "涨跌 K 线 · 成交量" : "收盘价");
  $("#close-chart").setAttribute("aria-label", isCandles ? "OHLC K 线与成交量" : "收盘价走势");
  setText("#chart-caption", `${state.chartMeta.adjustmentBasis} · ${state.bars.length} 根 · 按交易日期升序`);

  const chartOptions={...state.chartMeta,movingAverages:state.movingAverages,indicatorRows:state.technicalRows};
  const drawn=isCandles
    ? renderCandlestickVolumeChart($("#close-chart"),state.bars,chartOptions)
    : renderCloseChart($("#close-chart"),state.bars,chartOptions);
  $("#chart-empty").hidden = drawn;
  if (!drawn) {
    $("#chart-empty").textContent = !state.bars.length
      ? "查询行情后显示历史走势"
      : isCandles ? "没有可绘制的完整 OHLC K 线。" : "至少需要两根有效收盘价数据。";
  }

  const validBars = normalizeDailyBars(state.bars).filter((bar) => bar.close !== null);
  const latest = validBars[validBars.length - 1];
  setText("#chart-last-close", latest ? formatPrice(latest.close) : "—");
  setText("#chart-last-date", latest ? `截至 ${formatDate(latest.date)} · ${validBars.length} 根有效收盘数据` : "暂无有效收盘价");
  setText("#chart-quality-note", qualityLabel(state.chartMeta.qualityStatus));
}

function setChartView(view) {
  if (view !== "close" && view !== "candles") return;
  state.chartView = view;
  renderChartView();
}

function renderBars(response) {
  state.bars = Array.isArray(response.bars) ? response.bars : [];
  state.technicalSymbol = "";
  try {
    state.technicalSymbol = normalizeSymbol(response.symbol || getQuerySymbol());
  } catch {
    // 响应标识无法规范化时不关联任何个股均线，避免将错误标识当作当前股票。
  }
  state.chartMeta = {
    adjustmentBasis: String(response.adjustment_basis || "unknown"),
    volumeUnit: String(response.volume_unit || ""),
    amountUnit: String(response.amount_unit || ""),
    qualityStatus: response.quality_status || "unknown"
  };
  const bars = state.bars;
  const tbody = $("#bars-body");
  tbody.replaceChildren();
  setText("#bars-meta", `${bars.length} 根`);
  setText("#bars-caption", `标准化 OHLC · ${response.symbol || getQuerySymbol()} · ${qualityLabel(response.quality_status)}`);
  setText("#bars-adjustment", `复权口径 ${response.adjustment_basis || "unknown"}`);
  setText("#bars-warning", Array.isArray(response.warnings) && response.warnings.length ? response.warnings.join("；") : "显示最近交易日，最新日期在前");

  if (!bars.length) {
    const row = document.createElement("tr");
    const cell = makeCell("服务返回成功，但没有可展示的日 K 数据。");
    cell.colSpan = 9;
    cell.className = "table-empty";
    row.append(cell);
    tbody.append(row);
  } else {
    [...bars].reverse().slice(0, 40).forEach((bar) => {
      const row = document.createElement("tr");
      const changePercent = finiteNumberOrNull(bar.change_percent);
      row.append(
        makeCell(formatDate(bar.trade_date), "date-cell"),
        makeCell(formatPrice(bar.open)),
        makeCell(formatPrice(bar.high)),
        makeCell(formatPrice(bar.low)),
        makeCell(formatPrice(bar.close), "close-cell"),
        makeCell(formatPercent(bar.change_percent), changePercent !== null && changePercent > 0 ? "positive" : changePercent !== null && changePercent < 0 ? "negative" : ""),
        makeCell(formatCompact(bar.volume)),
        makeCell(bar.source || "—"),
      );
      const qualityCell = document.createElement("td");
      const pill = document.createElement("span");
      pill.className = `mini-quality mini-quality-${String(bar.quality_status || "unknown").toLowerCase()}`;
      pill.textContent = qualityLabel(bar.quality_status);
      qualityCell.append(pill);
      row.append(qualityCell);
      tbody.append(row);
    });
  }
  renderTechnicalSummary(calculateTechnicalIndicators(state.bars));
  renderResearchStockList();
  renderChartView();
}

function renderSources(payload) {
  const sources = Array.isArray(payload.sources) ? payload.sources : [];
  state.sourceCount = sources.length;
  setText("#source-count", sources.length);
  setText("#normalization-version", `归一化版本 ${payload.normalization_version || "—"}`);
  const list = $("#data-source-list");
  list.replaceChildren();

  if (!sources.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "服务未返回已注册的数据源。";
    list.append(empty);
    return;
  }

  const capabilityDefinitions = [
    ["quote_capable", "实时行情"],
    ["daily_bar_capable", "日 K"],
    ["stock_basic_capable", "证券档案"],
    ["sector_ranking_capable", "行业/概念排行"],
    ["index_quote_capable", "核心指数"],
    ["market_breadth_capable", "市场涨跌分布"],
    ["limit_pool_capable", "涨跌停池"],
    ["stock_boards_capable", "个股所属板块"],
    ["fundamentals_capable", "季度财务指标"],
    ["valuation_capable", "个股估值指标"],
    ["hot_ranking_capable", "个股人气榜/飙升榜"],
    ["chip_capable", "筹码分布"],
    ["capital_flow_capable", "个股资金流"],
    ["announcements_capable", "个股公告"],
    ["dragon_tiger_capable", "龙虎榜"],
    ["trading_calendar_capable", "交易日历"]
  ];

  sources.forEach((source) => {
    const row = document.createElement("div");
    row.className = "source-row";
    const main = document.createElement("div");
    main.className = "source-main";
    const indicator = document.createElement("span");
    indicator.className = `source-indicator ${source.enabled ? "enabled" : "disabled"}`;
    const name = document.createElement("strong");
    name.textContent = source.name || "unknown";
    const capabilities = capabilityDefinitions.filter(([key]) => source[key] === true);
    const detail = document.createElement("small");
    detail.textContent = capabilities.length ? `${capabilities.length} 项已声明能力` : "未声明查询能力";
    const disclosure = document.createElement("details");
    disclosure.className = "source-capabilities";
    const summary = document.createElement("summary");
    summary.textContent = "查看能力明细";
    const listOfCapabilities = document.createElement("div");
    listOfCapabilities.className = "source-capability-list";
    if (capabilities.length) {
      capabilities.forEach(([, label]) => {
        const tag = document.createElement("span");
        tag.className = "source-capability-tag";
        tag.textContent = label;
        listOfCapabilities.append(tag);
      });
    } else {
      const emptyCapability = document.createElement("span");
      emptyCapability.className = "source-capability-empty";
      emptyCapability.textContent = "当前 API 未声明能力；不代表服务整体不可用。";
      listOfCapabilities.append(emptyCapability);
    }
    disclosure.append(summary, listOfCapabilities);
    main.append(indicator, name, detail, disclosure);
    const badge = document.createElement("span");
    badge.className = `source-state ${source.enabled ? "source-state-on" : "source-state-off"}`;
    badge.textContent = source.enabled ? "已注册" : "已禁用";
    row.append(main, badge);
    list.append(row);
  });
}

async function refreshServices() {
  const generation = ++serviceRefreshGeneration;
  const settings = state.settings;
  const staleResult = {
    marketOK: false,
    analysisOK: false,
    marketState: "unknown",
    analysisState: "unknown",
    stale: true
  };

  try {
    const checks = await Promise.all([
      runHealthCheck(marketHealth, settings),
      runHealthCheck(analysisHealth, settings)
    ]);
    const marketState = serviceHealthState(checks[0]);
    const analysisState = serviceHealthState(checks[1]);
    const marketOK = marketState === "healthy";
    const analysisOK = analysisState === "healthy";
    if (generation !== serviceRefreshGeneration) {
      return { marketOK, analysisOK, marketState, analysisState, stale: true };
    }

    try {
      const connectionDot = $("#connection-dot");
      connectionDot.classList.toggle("is-online", marketOK && analysisOK);
      connectionDot.classList.toggle("is-offline", marketState === "failed" && analysisState === "failed");
      connectionDot.classList.toggle("is-unknown",
        ["unknown", "degraded"].includes(marketState) || ["unknown", "degraded"].includes(analysisState));
      const connectionLabel = marketOK && analysisOK ? "服务已连接"
        : marketState === "failed" && analysisState === "failed" ? "连接异常"
          : marketOK || analysisOK ? "部分连接"
            : marketState === "failed" || analysisState === "failed" ? "连接异常或状态未知" : "服务状态未知";
      setText("#connection-label", connectionLabel);
    } catch (renderError) {
      if (generation !== serviceRefreshGeneration) return staleResult;
      const message = "服务健康结果已获取，但状态显示更新失败：" + settingsCheckErrorMessage(renderError);
      try {
        const label = $("#connection-label");
        if (label) {
          label.textContent = "服务状态显示异常";
          label.title = message;
        }
      } catch {}
      return { marketOK, analysisOK, marketState, analysisState, stale: false, renderError: message };
    }
    return { marketOK, analysisOK, marketState, analysisState, stale: false };
  } catch (error) {
    if (generation !== serviceRefreshGeneration) return staleResult;
    const message = "服务健康检查流程异常：" + settingsCheckErrorMessage(error);
    // 即使健康请求或状态渲染意外失败，也将错误转换为结果，避免 void 调用产生未处理 rejection。
    try {
    } catch {}
    try {
      const connectionDot = $("#connection-dot");
      connectionDot.classList.remove("is-online", "is-offline");
      connectionDot.classList.add("is-unknown");
    } catch {}
    try {
      const label = $("#connection-label");
      if (label) {
        label.textContent = "服务状态未知";
        label.title = message;
      }
    } catch {}
    return { ...staleResult, stale: false, error: message };
  }
}


function describeConnectionHealthResults(marketResult, analysisResult) {
  const marketState = serviceHealthState(marketResult);
  const analysisState = serviceHealthState(analysisResult);
  const describe = (name, result, status) => {
    if (status === "healthy") return name + " API 就绪";
    if (status === "failed") return name + " API 不可用（" + errorMessage(result.error) + "）";
    if (status === "unknown") return name + " API 状态未知（回包未提供可识别的健康状态）";
    return name + " API 返回需关注的状态（" + String(result.data?.status || "未知") + "）";
  };
  return {
    text: describe("行情", marketResult, marketState) + "；" + describe("分析", analysisResult, analysisState),
    className: marketState === "healthy" && analysisState === "healthy"
      ? "settings-inline-status is-success"
      : "settings-inline-status is-warning"
  };
}

function settingsCheckErrorMessage(error) {
  try {
    return errorMessage(error);
  } catch {
    return "未知异常";
  }
}

function writeSettingsCheckOutputSafely(output, message, className = "settings-inline-status is-warning") {
  try {
    if (!output) return false;
    output.textContent = message;
    output.className = className;
    return true;
  } catch {
    return false;
  }
}

function releaseSettingsCheckButtonSafely(button, label) {
  try {
    if (!button) return;
    button.disabled = false;
    button.removeAttribute("aria-busy");
    button.textContent = label;
  } catch {
    // 控件不可写时也不能让检查流程的 finally 再次抛错。
  }
}

async function checkConnectionFromSettings() {
  const generation = ++savedConnectionCheckGeneration;
  let button = null;
  let output = null;
  let settingsToCheck = null;
  try {
    button = $("#settings-check-connection");
    output = $("#settings-connection-check");
    settingsToCheck = { ...state.settings };
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    button.textContent = "检查中…";
    writeSettingsCheckOutputSafely(output, "正在检查当前已保存的行情与分析 API 地址…", "settings-inline-status");

    const [marketResult, analysisResult] = await Promise.all([
      runHealthCheck(marketHealth, settingsToCheck),
      runHealthCheck(analysisHealth, settingsToCheck)
    ]);
    if (generation !== savedConnectionCheckGeneration) return;
    const currentSettings = state.settings;
    if (
      currentSettings.marketBase !== settingsToCheck.marketBase ||
      currentSettings.analysisBase !== settingsToCheck.analysisBase ||
      currentSettings.apiKey !== settingsToCheck.apiKey
    ) {
      writeSettingsCheckOutputSafely(
        output,
        "检查期间已保存的连接配置发生变化，旧结果已忽略，请使用当前配置重新检查。",
        "settings-inline-status is-warning"
      );
      return;
    }
    const summary = describeConnectionHealthResults(marketResult, analysisResult);
    writeSettingsCheckOutputSafely(
      output,
      "已保存连接配置检查结果：" + summary.text +
        "。健康检查不验证 Provider 的每次真实请求，也不会更改全局连接状态。",
      summary.className
    );
  } catch (error) {
    if (generation !== savedConnectionCheckGeneration) return;
    const currentSettings = state.settings;
    const configurationChanged = settingsToCheck && (
      currentSettings.marketBase !== settingsToCheck.marketBase ||
      currentSettings.analysisBase !== settingsToCheck.analysisBase ||
      currentSettings.apiKey !== settingsToCheck.apiKey
    );
    let message = "连接检查失败：" + settingsCheckErrorMessage(error);
    if (!settingsToCheck) message = "连接检查未能启动：" + settingsCheckErrorMessage(error);
    else if (configurationChanged) {
      message = "检查期间已保存的连接配置发生变化，旧错误已忽略，请使用当前配置重新检查。";
    }
    if (!output) {
      try { output = $("#settings-connection-check"); } catch {}
    }
    writeSettingsCheckOutputSafely(output, message, "settings-inline-status is-warning");
  } finally {
    if (generation === savedConnectionCheckGeneration) {
      if (!button) {
        try { button = $("#settings-check-connection"); } catch {}
      }
      releaseSettingsCheckButtonSafely(button, "检查连接");
    }
  }
}

async function checkDraftConnectionFromSettings() {
  const generation = ++draftConnectionCheckGeneration;
  let button = null;
  let output = null;
  let testedDraft = null;
  let settingsToCheck = null;
  try {
    button = $("#settings-check-draft-connection");
    output = $("#settings-draft-connection-check");
    testedDraft = readConnectionDraft();
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    button.textContent = "检查中…";
    writeSettingsCheckOutputSafely(
      output,
      "正在测试表单中的行情与分析 API 地址；本次检查不会保存配置…",
      "settings-inline-status"
    );

    // 草稿检查只验证设置页中的未保存配置，不写入全局设置。
    settingsToCheck = mergeConnectionDraft(testedDraft, state.settings);
    const [marketResult, analysisResult] = await Promise.all([
      runHealthCheck(marketHealth, settingsToCheck),
      runHealthCheck(analysisHealth, settingsToCheck)
    ]);
    if (generation !== draftConnectionCheckGeneration) return;
    const currentDraft = readConnectionDraft();
    const currentSettingsToCheck = mergeConnectionDraft(currentDraft, state.settings);
    if (
      connectionDraftChanged(testedDraft, currentDraft) ||
      currentSettingsToCheck.marketBase !== settingsToCheck.marketBase ||
      currentSettingsToCheck.analysisBase !== settingsToCheck.analysisBase ||
      currentSettingsToCheck.apiKey !== settingsToCheck.apiKey
    ) {
      writeSettingsCheckOutputSafely(
        output,
        "检查期间连接草稿或已保存配置发生变化，已忽略旧结果，请重新检查当前草稿。",
        "settings-inline-status is-warning"
      );
      return;
    }
    const summary = describeConnectionHealthResults(marketResult, analysisResult);
    writeSettingsCheckOutputSafely(
      output,
      "表单地址检查结果（尚未保存）：" + summary.text +
        "。健康检查不验证 Provider 的每次真实请求，也不会更改全局连接状态。",
      summary.className
    );
  } catch (error) {
    if (generation !== draftConnectionCheckGeneration) return;
    let configurationChanged = false;
    let currentDraft = null;
    if (testedDraft) {
      try {
        currentDraft = readConnectionDraft();
        configurationChanged = connectionDraftChanged(testedDraft, currentDraft);
      } catch {
        configurationChanged = true;
      }
    }
    if (!configurationChanged && settingsToCheck && currentDraft) {
      try {
        const currentSettingsToCheck = mergeConnectionDraft(currentDraft, state.settings);
        configurationChanged = currentSettingsToCheck.marketBase !== settingsToCheck.marketBase ||
          currentSettingsToCheck.analysisBase !== settingsToCheck.analysisBase ||
          currentSettingsToCheck.apiKey !== settingsToCheck.apiKey;
      } catch {
        configurationChanged = true;
      }
    }
    let message = testedDraft
      ? "连接检查失败：" + settingsCheckErrorMessage(error)
      : "草稿连接检查未能启动：" + settingsCheckErrorMessage(error);
    if (configurationChanged) {
      message = "检查期间连接草稿或已保存配置发生变化，旧错误已忽略，请重新检查当前草稿。";
    }
    if (!output) {
      try { output = $("#settings-draft-connection-check"); } catch {}
    }
    writeSettingsCheckOutputSafely(output, message, "settings-inline-status is-warning");
  } finally {
    if (generation === draftConnectionCheckGeneration) {
      if (!button) {
        try { button = $("#settings-check-draft-connection"); } catch {}
      }
      releaseSettingsCheckButtonSafely(button, "检查未保存配置");
    }
  }
}


async function refreshSources() {
  const generation = ++sourceRefreshGeneration;
  const settings = state.settings;
  let button = null;
  let list = null;

  const showMessage = (message, className = "empty-row") => {
    try {
      if (!list) list = $("#data-source-list");
      if (!list) return false;
      list.replaceChildren();
      const hint = document.createElement("div");
      hint.className = className;
      hint.textContent = message;
      list.append(hint);
      return true;
    } catch {
      return false;
    }
  };
  const setSourceTextSafely = (selector, value) => {
    try {
      setText(selector, value);
    } catch {
      // 列表或摘要节点更新失败时，仍应让请求和按钮收尾继续。
    }
  };

  try {
    try { button = $("#reload-sources"); } catch {}
    try { list = $("#data-source-list"); } catch {}

    // 初始化 UI 独立保护；局部 DOM 故障不能阻止读取当前配置能力。
    showMessage("正在读取当前连接配置对应的数据源能力…");
    setSourceTextSafely("#source-count", "—");
    setSourceTextSafely("#normalization-version", "归一化版本 —");
    state.sourceCount = 0;
    try {
      if (button) {
        button.disabled = true;
        button.textContent = "读取中…";
        button.setAttribute("aria-busy", "true");
      }
    } catch {
      // 若按钮无法更新，仍继续执行能力读取。
    }

    if (!settings.apiKey) {
      if (generation !== sourceRefreshGeneration) return;
      showMessage("尚未配置行情 API Key。密钥保存在当前浏览器本地。");
      return;
    }

    const payload = await getDataSources(settings);
    if (generation !== sourceRefreshGeneration) return;
    try {
      renderSources(payload);
    } catch (error) {
      let message = "未知异常";
      try { message = errorMessage(error); } catch {}
      showMessage("数据源能力已读取，但页面渲染失败：" + message, "empty-row error-text");
    }
  } catch (error) {
    if (generation !== sourceRefreshGeneration) return;
    let message = "未知异常";
    try { message = errorMessage(error); } catch {}
    showMessage(message, "empty-row error-text");
    setSourceTextSafely("#source-count", "—");
    setSourceTextSafely("#normalization-version", "归一化版本 —");
    state.sourceCount = 0;
  } finally {
    if (generation === sourceRefreshGeneration) {
      try {
        if (!button) button = $("#reload-sources");
        if (button) {
          button.disabled = false;
          button.removeAttribute("aria-busy");
          button.textContent = "刷新 ↻";
        }
      } catch {
        // 刷新按钮 DOM 不可用时，finally 不能再制造未处理的 Promise rejection。
      }
    }
  }
}


function shanghaiDateKey(value = new Date()) {
  const parts = new Intl.DateTimeFormat("en", {
    timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit"
  }).formatToParts(value);
  const fields = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return fields.year + "-" + fields.month + "-" + fields.day;
}

function configureDateInputs() {
  const today = shanghaiDateKey();
  $("#bar-start-date").max = today;
  $("#bar-end-date").max = today;
}

function clearBarsDisplay(symbol, message = "当前条件尚无日 K 结果。") {
  state.bars = [];
  state.technicalSymbol = "";
  state.chartMeta = { adjustmentBasis: "unknown", volumeUnit: "", amountUnit: "", qualityStatus: "unknown" };
  const tbody = $("#bars-body");
  tbody.replaceChildren();
  const row = document.createElement("tr");
  const cell = makeCell(message);
  cell.colSpan = 9;
  cell.className = "table-empty";
  row.append(cell);
  tbody.append(row);
  setText("#bars-meta", "未加载");
  setText("#bars-caption", `标准化 OHLC · ${symbol || "尚未选择"} · 未加载`);
  setText("#bars-adjustment", "复权口径 —");
  setText("#bars-warning", message);
  renderTechnicalSummary(calculateTechnicalIndicators([]));
  renderChartView();
  $("#chart-empty").textContent = message;
  $("#chart-empty").hidden = false;
  renderResearchStockList();
}

function clearMarketQueryResults(symbol, message) {
  const label = String(symbol || "").trim() || "当前标的";
  clearQuoteDisplay(label, "failed");
  clearBarsDisplay(label, message || "当前查询条件已变化，尚未读取新数据。");
  clearBundleSnapshotMetadata();
  setText("#last-updated", "—");
  setText("#query-hint", message || "当前查询条件已变化，旧行情快照已清除。");
  setNotice("", "info");
  stockResearch.invalidate(message || "当前查询条件已变化，请重新加载个股研究资料。");
}

function releaseQuoteQueryControls() {
  $("#quote-form").classList.remove("is-loading");
  $("#quote-form button[type='submit']").disabled = false;
}

function cancelQuoteQuery({ clearResults = false, symbol = "", message = "当前行情查询已取消。" } = {}) {
  const wasActive = state.isQuerying || Boolean(state.quoteQueryController);
  if (wasActive) state.quoteQueryGeneration += 1;
  const controller = state.quoteQueryController;
  state.quoteQueryController = null;
  state.quoteQueryKey = "";
  state.isQuerying = false;
  if (controller) controller.abort();
  releaseQuoteQueryControls();
  if (clearResults) {
    clearMarketQueryResults(symbol || $("#symbol-input").value.trim(), message);
    state.lastQueryInput = "";
  }
}

function quoteQueryKey(symbol, startDate, endDate, limit, adjustment) {
  return JSON.stringify([symbol, startDate, endDate, limit, adjustment, normalizeBasePath(state.settings.marketBase, "/market-api")]);
}

function responseSymbolMismatch(payload, requestedSymbol, label) {
  const responseSymbol = String(payload?.symbol ?? "").trim();
  if (!responseSymbol) return "";
  try {
    if (comparableMarketSymbol(responseSymbol) === comparableMarketSymbol(requestedSymbol)) return "";
  } catch {
    // 无法规范化的响应标识必须按不匹配处理，不将其显示为当前标的数据。
  }
  return label + "返回的证券代码 " + responseSymbol + " 与请求标的 " + requestedSymbol + " 不匹配，已忽略结果。";
}

function sameMarketSymbol(left, right) {
  try {
    return comparableMarketSymbol(left) === comparableMarketSymbol(right);
  } catch {
    return false;
  }
}


function handleQuoteCriteriaChange(message = "查询条件已变化，旧行情与日 K 已清除，请重新查询。") {
  cancelQuoteQuery({ clearResults: true, symbol: $("#symbol-input").value.trim(), message });
}

function handleSymbolInputChange() {
  const current = $("#symbol-input").value.trim();
  const activeInputChanged = state.isQuerying && current !== state.quoteQueryInput;
  const displayedResultChanged = Boolean(state.lastQueryInput) && current !== state.lastQueryInput;
  if (activeInputChanged || displayedResultChanged) {
    cancelQuoteQuery({
      clearResults: true,
      symbol: current,
      message: "标的输入已变化，旧行情、日 K 和相关研究快照已清除。请选择/确认新股票后重新查询。"
    });
  }
}

async function runQuoteQuery(event) {
  event?.preventDefault();
  let symbol;
  let startDate;
  let endDate;
  let limit;
  let adjustment;
  try {
    symbol = getQuerySymbol();
    state.settings.marketBase = normalizeBasePath(state.settings.marketBase, "/market-api");
    startDate = $("#bar-start-date").value;
    endDate = $("#bar-end-date").value;
    limit = Number($("#bar-limit").value) || 120;
    adjustment = $("#adjustment").value;
    const today = shanghaiDateKey();
    if (startDate && endDate && startDate > endDate) throw new Error("开始日期不能晚于结束日期。");
    if (startDate && startDate > today) throw new Error("开始日期不能晚于上海时区的今天。");
    if (endDate && endDate > today) throw new Error("结束日期不能晚于上海时区的今天。");
  } catch (error) {
    cancelQuoteQuery({ clearResults: true, symbol: $("#symbol-input").value.trim(), message: error.message });
    setNotice(error.message, "danger");
    return;
  }
  const requestKey = quoteQueryKey(symbol, startDate, endDate, limit, adjustment);
  if (state.isQuerying && state.quoteQueryKey === requestKey) return;
  if (state.isQuerying) cancelQuoteQuery();
  if (!state.settings.apiKey) {
    cancelQuoteQuery({ clearResults: true, symbol, message: "请先在连接与密钥设置中配置行情 API Key。" });
    setNotice("请先打开“连接与密钥设置”，填写行情服务的 MARKET_DATA_API_KEY。", "warning");
    setView("settings");
    return;
  }

  // 新的用户行情查询允许后续成功回包建立新的临时问股上下文。
  const generation = ++state.quoteQueryGeneration;
  const controller = new AbortController();
  let bundleResponse = null;
  let quoteResult;
  let barsResult;
  let usedLegacyBundleFallback = false;

  try {
    state.chatStockContextCleared = false;
    state.quoteQueryController = controller;
    state.quoteQueryKey = requestKey;
    state.quoteQueryInput = $("#symbol-input").value.trim();
    state.lastQueryInput = state.quoteQueryInput;
    state.isQuerying = true;
    $("#quote-form").classList.add("is-loading");
    $("#quote-form button[type='submit']").disabled = true;
    setNotice("", "info");
    setText("#query-hint", `正在查询 ${symbol} 的实时行情与日 K…`);

    clearQuoteDisplay(symbol, "loading");
    clearBarsDisplay(symbol, "正在读取本次日 K，尚无新结果。");
    clearBundleSnapshotMetadata();
    if (!startDate && !endDate) {
      // No custom date range: prefer one backend snapshot for a consistent quote + daily-bar view.
      try {
        bundleResponse = await getMarketBundle(state.settings, symbol, {
          lookbackDays: Math.min(2000, Math.max(20, limit)),
          adjustment
        }, controller.signal);
      } catch (error) {
      if (controller.signal.aborted || generation !== state.quoteQueryGeneration) return;
      if (error.status === 404) {
        // Older deployed servers may not expose Bundle yet. Only this case falls back to two requests.
        usedLegacyBundleFallback = true;
        [quoteResult, barsResult] = await Promise.allSettled([
          getQuote(state.settings, symbol, controller.signal),
          getDailyBars(state.settings, symbol, { limit, adjustment }, controller.signal)
        ]);
      } else {
        // Avoid retrying all providers through two endpoints after an aggregate Bundle failure.
        quoteResult = { status: "rejected", reason: error };
        barsResult = { status: "rejected", reason: error };
      }
    }
    if (bundleResponse) {
      const bundleMismatch = responseSymbolMismatch(bundleResponse, symbol, "统一行情快照");
      if (bundleMismatch) {
        bundleResponse = null;
        quoteResult = { status: "rejected", reason: new Error(bundleMismatch) };
        barsResult = { status: "rejected", reason: new Error(bundleMismatch) };
      } else if (bundleResponse.quote && typeof bundleResponse.quote === "object") {
        const quoteMismatch = responseSymbolMismatch(bundleResponse.quote, symbol, "统一快照中的实时行情");
        if (quoteMismatch) {
          bundleResponse = {
            ...bundleResponse,
            quote: null,
            warnings: [...(Array.isArray(bundleResponse.warnings) ? bundleResponse.warnings : []), quoteMismatch]
          };
        }
      }
    }
  } else {
    // Bundle has no start/end query parameters; honor explicit date ranges with the existing daily-bars API.
    [quoteResult, barsResult] = await Promise.allSettled([
      getQuote(state.settings, symbol, controller.signal),
      getDailyBars(state.settings, symbol, { limit, adjustment, start: startDate, end: endDate }, controller.signal)
    ]);
  }

  if (controller.signal.aborted || generation !== state.quoteQueryGeneration) return;
  if (!bundleResponse) {
    if (quoteResult?.status === "fulfilled") {
      const mismatch = responseSymbolMismatch(quoteResult.value, symbol, "实时行情");
      if (mismatch) quoteResult = { status: "rejected", reason: new Error(mismatch) };
    }
    if (barsResult?.status === "fulfilled") {
      const mismatch = responseSymbolMismatch(barsResult.value, symbol, "日 K");
      if (mismatch) barsResult = { status: "rejected", reason: new Error(mismatch) };
    }
  }
  if (bundleResponse) {
    renderBundleSnapshotMetadata(bundleResponse);
    const bundleBars = Array.isArray(bundleResponse.daily_bars) ? bundleResponse.daily_bars : [];
    if (bundleResponse.quote && typeof bundleResponse.quote === "object") {
      renderQuote(bundleResponse.quote);
    } else {
      clearQuoteDisplay(symbol, "failed");
    }

    const actualAdjustment = bundleBars.find((bar) => bar.adjustment_basis)?.adjustment_basis
      || (bundleBars.length ? adjustment : "unknown");
    renderBars({
      symbol: bundleResponse.symbol || symbol,
      bars: bundleBars,
      adjustment_basis: actualAdjustment,
      volume_unit: bundleBars[0]?.volume_unit || (bundleBars.length ? "shares" : ""),
      amount_unit: bundleBars[0]?.amount_unit || (bundleBars.length ? "CNY" : ""),
      quality_status: bundleResponse.quality_status || "unknown",
      warnings: Array.isArray(bundleResponse.warnings) ? bundleResponse.warnings : []
    });

    const warnings = Array.isArray(bundleResponse.warnings) ? bundleResponse.warnings : [];
    const quoteMissing = !(bundleResponse.quote && typeof bundleResponse.quote === "object");
    const needsWarning = warnings.length > 0
      || bundleResponse.quality_status !== "valid"
      || (bundleResponse.quote?.quality_status && bundleResponse.quote.quality_status !== "valid");
    if (quoteMissing) {
      setNotice(`统一快照已返回 ${bundleBars.length} 根日 K，但没有返回实时行情。${warnings.length ? "提示：" + warnings.join("；") : ""}`, "warning");
    } else if (needsWarning) {
      setNotice(`统一行情快照质量状态：${qualityLabel(bundleResponse.quality_status)}。${warnings.length ? "提示：" + warnings.join("；") : ""}`, "warning");
    }
  } else {
    if (quoteResult.status === "fulfilled") {
      renderQuote(quoteResult.value);
      if (quoteResult.value.quality_status && quoteResult.value.quality_status !== "valid") {
        setNotice(`行情接口已返回数据，但质量状态为“${qualityLabel(quoteResult.value.quality_status)}”。请同时查看质量原因及数据源验收记录。`, "warning");
      }
    } else {
      clearQuoteDisplay(symbol, "failed");
      setNotice(`实时行情查询失败：${errorMessage(quoteResult.reason)}`, "danger");
    }

    if (barsResult.status === "fulfilled") {
      renderBars(barsResult.value);
      const warnings = barsResult.value.warnings || [];
      if (warnings.length || barsResult.value.quality_status !== "valid") {
        setNotice(`日 K 已返回 ${(barsResult.value.bars || []).length} 根，质量状态：${qualityLabel(barsResult.value.quality_status)}。${warnings.length ? `提示：${warnings.join("；")}` : ""}`, "warning");
      }
    } else {
      renderBars({ symbol, bars: [], adjustment_basis: adjustment, quality_status: "unknown", warnings: ["日 K 查询失败：" + errorMessage(barsResult.reason)] });
      setText("#bars-meta", "查询失败");
      $("#chart-empty").textContent = "日 K 查询失败，旧标的图表已清除。";
      $("#chart-empty").hidden = false;
      if (quoteResult.status === "fulfilled") setNotice(`实时行情已返回，日 K 查询失败：${errorMessage(barsResult.reason)}`, "warning");
    }
  }

  setText("#last-updated", new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(new Date()));
  const compatibilityHint = usedLegacyBundleFallback
    ? " · 当前服务未提供 Bundle 接口，已回退为两次独立请求"
    : "";
  setText("#query-hint", `请求标的：${symbol} · 日 K 上限：${limit} · 复权口径：${adjustment} · 日期：${startDate || "起始不限"} 至 ${endDate || "截至今天"}${compatibilityHint}`);
  } catch (error) {
    if (!controller.signal.aborted && generation === state.quoteQueryGeneration) {
      let message = "未知异常";
      try { message = errorMessage(error); } catch {}
      try {
        clearMarketQueryResults(symbol, "本次行情结果无法展示，旧快照已清除。");
      } catch (renderError) {
        try {
          message += "；清理旧行情显示失败：" + errorMessage(renderError);
        } catch {}
      }
      try {
        setNotice("行情查询结果处理失败：" + message, "danger");
      } catch {
        try { setText("#query-hint", "行情查询失败，且错误提示更新失败：" + message); } catch {}
      }
    }
  } finally {
    if (generation === state.quoteQueryGeneration) {
      // 先释放逻辑锁，再分别尽力更新摘要和表单控件，避免任一渲染失败跳过按钮解锁。
      state.isQuerying = false;
      state.quoteQueryController = null;
      state.quoteQueryKey = "";
      try {
        renderResearchOverview();
      } catch (renderError) {
        try {
          setText("#query-hint", "行情请求已结束，但个股研究摘要刷新失败：" + errorMessage(renderError));
        } catch {}
      }
      try {
        releaseQuoteQueryControls();
      } catch (unlockError) {
        try {
          setNotice("行情请求已结束，但查询按钮状态恢复失败：" + errorMessage(unlockError), "warning");
        } catch {
          try { setText("#query-hint", "行情请求已结束，请重新载入页面恢复查询控件。"); } catch {}
        }
      }
    }
  }
}
function renderRunStatus(status = "") {
  const target = $("#task-run-status-pill");
  const normalized = String(status || "").toLowerCase();
  const label = normalized ? runStatusLabel(normalized) : state.runId ? "等待 Run 结果" : "尚未关联 Run";
  const tone = normalized === "completed"
    ? "is-local"
    : ["partial", "blocked_data_quality", "rejected_request", "failed"].includes(normalized)
      ? "is-unavailable"
      : "is-readonly";
  target.className = "availability-pill " + tone;
  target.textContent = label;
  target.title = normalized || "Run 尚未返回状态";
  target.dataset.runStatus = normalized || "unknown";
}

function openRunFlowDrawer() {
  const dialog = $("#run-flow-dialog");
  if (!dialog.open) dialog.showModal();
}


function renderTaskFlowSummary() {
  const eventsCount = $("#task-summary-event-count");
  const latestEvent = $("#task-summary-latest-event");
  const quality = $("#task-summary-quality");
  const warningCount = $("#task-summary-warning-count");
  if (!eventsCount || !latestEvent || !quality || !warningCount) return;

  if (!state.taskEventsLoaded) {
    eventsCount.textContent = state.taskId ? "等待事件读取" : "未选择任务";
    latestEvent.textContent = "未报告";
  } else {
    const events = [...state.taskEvents.values()].sort((a, b) => Number(a.sequence) - Number(b.sequence));
    eventsCount.textContent = state.taskEventsError
      ? events.length ? `${events.length} 条（读取异常）` : "读取失败"
      : events.length ? String(events.length) : "暂无持久化事件";
    const last = events.length ? events[events.length - 1] : null;
    const lastEvent = last?.event && typeof last.event === "object" ? last.event : last;
    latestEvent.textContent = lastEvent?.at ? formatDateTime(lastEvent.at) : "未报告";
  }
  const result = state.lastRunResult;
  const corePass = result?.quality?.core_pass;
  quality.textContent = corePass === true ? "通过" : corePass === false ? "未通过" : result ? "未报告" : "等待 Run 结果";
  quality.className = "run-flow-summary-value " + (corePass === true ? "is-valid" : corePass === false ? "is-invalid" : "is-unknown");
  const warnings = result?.warnings;
  warningCount.textContent = Array.isArray(warnings) ? String(warnings.length) : result ? "未报告" : "等待 Run 结果";
}

function renderSelectedTaskEvent(item) {
  const panel = $("#task-event-detail");
  const detailLayout = $("#task-detail");
  if (!panel) return;
  if (!item) {
    panel.hidden = true;
    detailLayout?.classList.remove("has-selected-event");
    return;
  }
  const event = item.event && typeof item.event === "object" ? item.event : item;
  panel.hidden = false;
  detailLayout?.classList.add("has-selected-event");
  const sequence = Number(item.sequence);
  const label = event.stage || event.name || "阶段事件";
  $("#task-event-detail-title").textContent = label + " · #" + (Number.isFinite(sequence) ? sequence : "未报告");
  $("#task-event-detail-message").textContent = event.message || "服务端未提供事件说明。";
  const status = $("#task-event-detail-status");
  const statusKey = taskEventStatusKey(event.status);
  const statusLabel = taskEventStatusLabel(event.status, statusKey);
  status.textContent = statusLabel;
  status.className = "availability-pill " + taskEventStatusTone(statusKey);
  status.title = event.status || "服务端未报告状态";
  status.dataset.eventStatus = statusKey;

  const fields = [
    ["服务端序号", Number.isFinite(sequence) ? sequence : null],
    ["阶段键", event.stage || event.name],
    ["事件状态", statusLabel],
    ["服务端时间", event.at ? formatDateTime(event.at) : null],
    ["错误代码", event.error_code]
  ];
  if (event.duration_ms !== undefined && event.duration_ms !== null && event.duration_ms !== "" &&
      Number.isFinite(Number(event.duration_ms)) && Number(event.duration_ms) >= 0) {
    const durationMs = Number(event.duration_ms);
    fields.push(["服务端耗时", durationMs >= 1000 ? (durationMs / 1000).toFixed(2) + " 秒" : durationMs + " 毫秒"]);
  }
  const root = $("#task-event-detail-fields");
  root.replaceChildren();
  fields.forEach(([labelText, value]) => {
    if (value === undefined || value === null || value === "") return;
    const field = document.createElement("div");
    field.className = "event-detail-field";
    const caption = document.createElement("small");
    caption.textContent = labelText;
    const content = document.createElement("strong");
    content.textContent = String(value);
    field.append(caption, content);
    root.append(field);
  });
  $("#task-event-json").textContent = JSON.stringify(item, null, 2);
}
function renderTask(task) {
  state.taskStatus = task.status || state.taskStatus;
  if (typeof task.created_at === "string" && task.created_at) state.taskCreatedAt = task.created_at;
  if (task.run_id) state.runId = task.run_id;
  if (task.task_type) state.taskType = task.task_type;
  if (Array.isArray(task.symbols)) state.taskSymbols = task.symbols;
  persistTaskReference();
  setQualityPill($("#task-status-pill"), task.status === "succeeded" ? "valid" : task.status === "partial" ? "degraded" : task.status === "failed" ? "invalid" : "unknown", taskStatusLabel(task.status));
  renderRunStatus(state.lastRunResult?.status || "");
  $("#task-empty").hidden = true;
  $("#task-detail").hidden = false;
  setText("#task-id-value", task.task_id || state.taskId);
  setText("#task-symbols-value", (state.taskSymbols.length ? state.taskSymbols.join(", ") : "标的未报告") + " · " + (state.taskType || "任务类型未报告"));
  setText("#task-run-id", task.run_id || state.runId);
  setText("#task-current-stage", task.current_stage || (task.status === "queued" ? "等待 Worker" : "未报告"));
  setText("#task-completed", task.completed_stages ?? "未报告");
  setText("#task-created", formatDateTime(task.created_at));
  setText("#task-started", formatDateTime(task.started_at));
  setText("#task-finished", formatDateTime(task.finished_at));
  const failureCode = task.failure_code || "";
  const message = task.error_message || "";
  const diagnostics = [message, failureCode ? "错误代码：" + failureCode : ""].filter(Boolean).join(" · ");
  $("#task-error").hidden = !diagnostics;
  $("#task-error").textContent = diagnostics;
  $("#task-poll-label").textContent = TERMINAL_TASK_STATUSES.has(task.status)
    ? "任务已结束"
    : state.taskEventsUnavailable
      ? "任务状态每 2.5 秒刷新；阶段事件接口暂不可用，点击“立即刷新状态”可重新尝试。"
      : "每 2.5 秒轮询服务端状态";
  renderTaskFlowSummary();
  // 自选股页可能在后台继续跟踪任务；同步最新的 Task 状态。
  if (state.view === "watchlist") renderWatchlist();
}

function renderTaskEvents() {
  const readStatus = $("#task-events-read-status");
  if (readStatus) {
    readStatus.hidden = !state.taskEventsError;
    readStatus.textContent = state.taskEventsError
      ? "阶段事件读取提示：" + state.taskEventsError + "。已保留当前可用事件；任务状态可独立刷新。"
      : "";
  }
  const list = $("#task-events");
  list.replaceChildren();
  const allEvents = [...state.taskEvents.values()].sort((a, b) => Number(a.sequence) - Number(b.sequence));
  const filterLabels = { all: "全部", issues: "异常 / 降级", active: "运行中", completed: "成功 / 跳过" };
  if (!Object.prototype.hasOwnProperty.call(filterLabels, state.taskEventFilter)) state.taskEventFilter = "all";
  const statusKeyFor = (item) => {
    const event = item?.event && typeof item.event === "object" ? item.event : item;
    return taskEventStatusKey(event?.status);
  };
  const filterCounts = { all: allEvents.length, issues: 0, active: 0, completed: 0 };
  allEvents.forEach((item) => {
    for (const key of ["issues", "active", "completed"]) {
      if (taskEventMatchesFilter(statusKeyFor(item), key)) filterCounts[key]++;
    }
  });
  document.querySelectorAll("[data-task-event-filter]").forEach((button) => {
    const key = button.dataset.taskEventFilter || "all";
    button.setAttribute("aria-pressed", String(key === state.taskEventFilter));
    const count = button.querySelector("[data-filter-count]");
    if (count) count.textContent = String(filterCounts[key] ?? 0);
  });
  const events = allEvents.filter((item) => taskEventMatchesFilter(statusKeyFor(item), state.taskEventFilter));
  const summary = $("#task-event-filter-summary");
  if (summary) summary.textContent = "筛选：" + filterLabels[state.taskEventFilter] + " · 显示 " + events.length + "/" + allEvents.length + " 条阶段事件";
  if (!allEvents.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = state.taskEventsError
      ? "当前没有可显示的阶段事件。请查看上方读取提示；任务状态仍可独立刷新，点击“立即刷新状态”可重试。"
      : state.taskEventsLoaded
        ? "服务端目前没有持久化阶段事件。排队期间事件数组可以为空。"
        : "等待服务端阶段事件读取。";
    list.append(empty);
    renderSelectedTaskEvent(null);
    renderTaskFlowSummary();
    return;
  }
  if (!events.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "当前筛选条件下没有匹配事件（已读取 " + allEvents.length + " 条）。选择“全部”查看所有阶段。";
    list.append(empty);
    renderSelectedTaskEvent(null);
    renderTaskFlowSummary();
    return;
  }

  const hasSelected = events.some((item) => Number(item.sequence) === Number(state.selectedTaskEventSequence));
  if (!hasSelected) {
    const priority = [...events].reverse().find((item) => {
      const event = item.event && typeof item.event === "object" ? item.event : item;
      const status = taskEventStatusKey(event.status);
      return ["failed", "error", "blocked", "rejected", "degraded", "partial", "running", "started", "in-progress", "in_progress", "processing"].includes(status);
    });
    state.selectedTaskEventSequence = Number((priority || events[events.length - 1]).sequence);
  }

  events.forEach((item) => {
    const event = item.event && typeof item.event === "object" ? item.event : item;
    const sequence = Number(item.sequence);
    const selected = sequence === Number(state.selectedTaskEventSequence);
    const statusKey = taskEventStatusKey(event.status);
    const statusLabel = taskEventStatusLabel(event.status, statusKey);
    const node = document.createElement("details");
    node.className = "run-flow-node event-row event-row-" + statusKey;
    node.dataset.eventSequence = String(item.sequence);
    node.setAttribute("aria-pressed", String(selected));
    node.open = selected || state.expandedTaskEventSequences.has(sequence);
    const heading = document.createElement("summary");
    heading.className = "run-flow-node-heading";
    const icon = document.createElement("span");
    icon.className = "event-status event-status-" + statusKey;
    icon.textContent = ["succeeded", "success", "completed"].includes(statusKey) ? "✓"
      : ["failed", "error", "blocked", "rejected"].includes(statusKey) ? "!"
        : statusKey === "skipped" ? "–"
          : ["running", "started", "in-progress", "in_progress", "processing"].includes(statusKey) ? "↻" : "·";
    const body = document.createElement("span");
    body.className = "event-body";
    const title = document.createElement("strong");
    title.textContent = event.stage || event.name || "分析阶段";
    const titleLine = document.createElement("span");
    titleLine.className = "event-row-heading";
    const badge = document.createElement("span");
    badge.className = "event-state-label";
    badge.textContent = statusLabel;
    titleLine.append(title, badge);
    const meta = document.createElement("small");
    const parts = ["#" + item.sequence, formatDateTime(event.at)];
    if (event.duration_ms !== undefined && event.duration_ms !== null && event.duration_ms !== "" &&
        Number.isFinite(Number(event.duration_ms)) && Number(event.duration_ms) >= 0) {
      const duration = Number(event.duration_ms);
      parts.push(duration >= 1000 ? (duration / 1000).toFixed(2) + " 秒" : duration + " 毫秒");
    }
    meta.textContent = parts.join(" · ");
    body.append(titleLine, meta);
    heading.append(icon, body);
    node.append(heading);
    const content = document.createElement("div");
    content.className = "run-flow-node-content";
    const message = document.createElement("p");
    message.textContent = event.message || "服务端事件未提供描述";
    content.append(message);
    if (event.error_code) {
      const diagnostic = document.createElement("span");
      diagnostic.className = "event-diagnostic";
      diagnostic.textContent = "错误代码：" + String(event.error_code);
      content.append(diagnostic);
    }
    const select = document.createElement("button");
    select.type = "button";
    select.className = "text-button run-flow-node-detail";
    select.dataset.taskEventSequence = String(item.sequence);
    select.textContent = selected ? "当前节点详情已显示" : "查看节点详情";
    content.append(select);
    node.append(content);
    node.addEventListener("toggle", () => {
      if (node.open) state.expandedTaskEventSequences.add(sequence);
      else state.expandedTaskEventSequences.delete(sequence);
    });
    list.append(node);
  });

  const selected = events.find((item) => Number(item.sequence) === Number(state.selectedTaskEventSequence));
  renderSelectedTaskEvent(selected || null);
  renderTaskFlowSummary();
}

function resultDisplayValue(value) {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) return value.length ? value.map(resultDisplayValue).join("、") : "无";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function createResultSection(root, title, description = "") {
  const section = document.createElement("section");
  section.className = "analysis-result-block";
  const heading = document.createElement("h3");
  heading.textContent = title;
  section.append(heading);
  if (description) {
    const detail = document.createElement("p");
    detail.textContent = description;
    section.append(detail);
  }
  const grid = document.createElement("div");
  grid.className = "analysis-result-grid";
  section.append(grid);
  if (title === "数据质量与策略门禁") {
    const disclosure = document.createElement("details");
    disclosure.className = "analysis-result-disclosure";
    const summary = document.createElement("summary");
    summary.textContent = "数据质量与策略门禁 · 展开诊断";
    disclosure.append(summary, section);
    root.append(disclosure);
  } else {
    root.append(section);
  }
  return grid;
}

function appendResultEmpty(parent, message) {
  const empty = document.createElement("div");
  empty.className = "analysis-result-empty";
  empty.textContent = message;
  parent.append(empty);
}

function appendResultCard(parent, title, fields, badgeText = "", badgeTone = "") {
  const card = document.createElement("article");
  card.className = "analysis-result-card";
  const heading = document.createElement("div");
  heading.className = "analysis-result-card-heading";
  const titleNode = document.createElement("strong");
  titleNode.textContent = title;
  heading.append(titleNode);
  if (badgeText) {
    const badge = document.createElement("span");
    badge.className = "analysis-result-badge" + (badgeTone ? " " + badgeTone : "");
    badge.textContent = badgeText;
    heading.append(badge);
  }
  card.append(heading);
  const details = document.createElement("dl");
  details.className = "analysis-result-fields";
  fields.forEach(([label, value]) => {
    const term = document.createElement("dt");
    term.textContent = label;
    const description = document.createElement("dd");
    description.textContent = resultDisplayValue(value);
    details.append(term, description);
  });
  if (fields.length) card.append(details);
  parent.append(card);
  return card;
}

function resultPairs(record, fields) {
  return fields.map(([label, key, transform]) => [
    label,
    typeof transform === "function" ? transform(record?.[key]) : record?.[key]
  ]);
}

function renderTaskResult(result) {
  $("#run-flow-result-empty")?.setAttribute("hidden", "");
  state.lastRunResult = result;
  state.runResultReadStatus = "";
  state.runResultReadError = "";
  state.runId = result.run_id || state.runId;
  renderRunStatus(result.status || "");
  renderTaskFlowSummary();
  persistTaskReference();

  const quality = result.quality || {};
  const factors = Array.isArray(result.factors) ? result.factors : [];
  const signals = Array.isArray(result.signals) ? result.signals : [];
  const plans = Array.isArray(result.plans) ? result.plans : [];
  const warnings = Array.isArray(result.warnings) ? result.warnings : [];
  const metrics = [
    ["运行状态", runStatusLabel(result.status)],
    ["核心质量门禁", quality.core_pass === true ? "通过" : quality.core_pass === false ? "未通过" : "未报告"],
    ["分析时点", result.request?.as_of || result.market_state?.as_of || result.started_at || "未报告"],
    ["因子数量", Array.isArray(result.factors) ? factors.length : "未报告"],
    ["候选信号", Array.isArray(result.signals) ? signals.length : "未报告"],
    ["计划数量", Array.isArray(result.plans) ? plans.length : "未报告"],
    ["警告数量", Array.isArray(result.warnings) ? warnings.length : "未报告"]
  ];
  const metricRoot = $("#task-result-metrics");
  metricRoot.replaceChildren();
  metrics.forEach(([label, value]) => {
    const card = document.createElement("div");
    card.className = "task-result-metric";
    const caption = document.createElement("small");
    caption.textContent = label;
    const amount = document.createElement("strong");
    amount.textContent = String(value);
    card.append(caption, amount);
    metricRoot.append(card);
  });

  const root = $("#task-result-structured");
  root.replaceChildren();

  const overview = createResultSection(root, "本次分析概况");
  appendResultCard(overview, "分析请求", [
    ["标的", result.request?.symbols],
    ["任务类型", result.request?.task_type],
    ["市场", result.request?.market_id],
    ["分析时点", result.request?.as_of],
    ["时点模式", result.request?.as_of_mode],
    ["策略", result.request?.strategy_ids],
    ["策略配置版本", result.request?.strategy_config_version],
    ["风险配置版本", result.request?.risk_config_version]
  ]);
  appendResultCard(overview, "运行信息", [
    ["Run ID", result.run_id],
    ["状态", runStatusLabel(result.status)],
    ["开始时间", result.started_at],
    ["结束时间", result.finished_at],
    ["Universe Snapshot", result.universe?.id],
    ["数据快照", result.raw_snapshot_id],
    ["市场快照", result.market_snapshot_id],
    ["质量策略版本", quality.policy_version]
  ]);

  const marketGrid = createResultSection(root, "市场状态", "展示后端返回的市场状态、指标与证据；不会在前端自行推导大盘结论。");
  if (result.market_state && Object.keys(result.market_state).length) {
    const market = result.market_state;
    appendResultCard(marketGrid, market.regime || "市场状态", [
      ["状态", market.regime],
      ["有效时点", market.as_of],
      ["算法版本", market.algorithm_version],
      ["指标", market.metrics],
      ["证据", market.evidence]
    ]);
  } else {
    appendResultEmpty(marketGrid, "本次 Run 未返回 market_state。");
  }

  const qualityGrid = createResultSection(root, "数据质量与策略门禁", "缺失字段保留为未报告，不把未知值当作 0 或通过。");
  appendResultCard(qualityGrid, "质量报告", [
    ["核心门禁", quality.core_pass === true ? "通过" : quality.core_pass === false ? "未通过" : "未报告"],
    ["部分数据", quality.partial],
    ["检查时间", quality.checked_at],
    ["策略资格", quality.strategy_eligible],
    ["策略阻断原因", quality.strategy_blockers],
    ["质量策略版本", quality.policy_version]
  ], quality.core_pass === true ? "核心门禁通过" : quality.core_pass === false ? "核心门禁未通过" : "状态未知",
  quality.core_pass === true ? "is-allowed" : quality.core_pass === false ? "is-blocked" : "");
  const issues = Array.isArray(quality.issues) ? quality.issues : [];
  if (issues.length) {
    issues.forEach((issue, index) => {
      appendResultCard(qualityGrid, issue.message || issue.code || ("质量问题 " + (index + 1)), resultPairs(issue, [
        ["严重程度", "severity"], ["问题代码", "code"], ["数据集", "dataset"],
        ["标的", "symbol"], ["字段", "field"], ["策略", "strategy_id"]
      ]), issue.severity || "");
    });
  } else {
    appendResultEmpty(qualityGrid, "没有返回 quality.issues；这不等同于所有可选数据都完整。");
  }

  const factorGrid = createResultSection(root, "分析因子", "显示实际计算结果、输入质量和证据引用。");
  if (factors.length) {
    factors.forEach((factor, index) => {
      appendResultCard(factorGrid, factor.factor_id || ("因子 " + (index + 1)), resultPairs(factor, [
        ["标的", "symbol"], ["因子版本", "factor_version"], ["因子值", "value"],
        ["单位", "unit"], ["有效时点", "as_of"], ["输入质量", "input_quality"],
        ["缺失原因", "missing_reason"], ["证据引用", "evidence_refs"]
      ]), factor.input_quality || "");
    });
  } else {
    appendResultEmpty(factorGrid, "本次 Run 未返回分析因子。");
  }

  const signalGrid = createResultSection(root, "候选信号", "这些是分析引擎生成的候选信号，不是自动交易指令。");
  if (signals.length) {
    signals.forEach((signal, index) => {
      appendResultCard(signalGrid, (signal.symbol || "标的未报告") + " · " + (signal.direction || "方向未报告"), resultPairs(signal, [
        ["信号 ID", "id"], ["策略", "strategy_id"], ["策略版本", "strategy_version"],
        ["信号时间", "signal_at"], ["参考价格", "reference_price"],
        ["有效截止", "valid_until"], ["预期持有天数", "expected_hold_days"],
        ["入场规则", "entry_rule"], ["退出规则", "exit_rule"],
        ["失效规则", "invalidation_rule"], ["证据引用", "evidence_refs"],
        ["扩展属性", "attributes"]
      ]), signal.strategy_id || ("候选 " + (index + 1)));
    });
  } else {
    appendResultEmpty(signalGrid, "本次 Run 未生成候选信号；请同时查看质量门禁、拒绝原因及警告。");
  }

  const renderDecisions = (title, values) => {
    const grid = createResultSection(root, title);
    if (!Array.isArray(values) || !values.length) {
      appendResultEmpty(grid, "本次 Run 未返回 " + title + " 决策。");
      return;
    }
    values.forEach((decision, index) => {
      appendResultCard(grid, decision.signal_id || (title + " " + (index + 1)), resultPairs(decision, [
        ["允许", "allowed"], ["阻断/通过原因", "reasons"],
        ["规则 ID", "rule_ids"], ["规则版本", "rule_refs"]
      ]), decision.allowed === true ? "允许" : decision.allowed === false ? "阻断" : "未报告",
      decision.allowed === true ? "is-allowed" : decision.allowed === false ? "is-blocked" : "");
    });
  };
  renderDecisions("可交易性检查", result.tradability);
  renderDecisions("风险检查", result.risk);

  const plansGrid = createResultSection(root, "交易计划", "仅呈现后端实际生成的计划；页面不会自行补造价格或数量。");
  if (plans.length) {
    plans.forEach((plan, index) => {
      appendResultCard(plansGrid, (plan.symbol || "标的未报告") + " · " + (plan.side || "方向未报告"), resultPairs(plan, [
        ["信号 ID", "signal_id"], ["数量", "quantity"], ["参考价格", "reference_price"],
        ["止损价", "stop_price"], ["目标价", "target_price"],
        ["最早执行时间", "earliest_execute_at"], ["过期时间", "expire_at"],
        ["风险预算", "risk_budget"], ["成本估算", "cost_estimate"], ["规则版本", "rule_version"]
      ]), "计划 " + (index + 1));
    });
  } else {
    appendResultEmpty(plansGrid, "没有返回交易计划。请检查下方的计划拒绝、质量和风险决策。");
  }

  const rejectionsGrid = createResultSection(root, "计划拒绝原因");
  const rejections = Array.isArray(result.planning_rejections) ? result.planning_rejections : [];
  if (rejections.length) {
    rejections.forEach((rejection, index) => {
      appendResultCard(rejectionsGrid, rejection.signal_id || ("拒绝项 " + (index + 1)), resultPairs(rejection, [
        ["信号 ID", "signal_id"], ["拒绝原因", "reasons"]
      ]), "未生成计划", "is-blocked");
    });
  } else {
    appendResultEmpty(rejectionsGrid, "本次 Run 未返回 planning_rejections。");
  }

  const executionGrid = createResultSection(root, "执行评估");
  const execution = result.execution_assessment || {};
  appendResultCard(executionGrid, "评估模型", resultPairs(execution, [["模型版本", "model_version"], ["警告", "warnings"]]));
  const orders = Array.isArray(execution.orders) ? execution.orders : [];
  if (orders.length) {
    orders.forEach((order, index) => {
      appendResultCard(executionGrid, order.signal_id || ("订单评估 " + (index + 1)), resultPairs(order, [
        ["状态", "status"], ["请求数量", "requested_quantity"], ["已成交数量", "filled_quantity"],
        ["剩余数量", "remaining_quantity"], ["原因", "reason"]
      ]), order.status || "");
    });
  } else {
    appendResultEmpty(executionGrid, "没有返回订单执行评估记录；不应将其理解为已成交。");
  }

  const failure = result.failure;
  if (failure) {
    const failureGrid = createResultSection(root, "运行失败信息");
    appendResultCard(failureGrid, failure.message || "分析失败", resultPairs(failure, [
      ["错误代码", "code"], ["失败阶段", "stage"], ["消息", "message"]
    ]), failure.code || "Failure", "is-blocked");
  }

  const warningRoot = $("#task-result-warnings");
  warningRoot.replaceChildren();
  warningRoot.hidden = warnings.length === 0;
  warnings.forEach((warning) => {
    const line = document.createElement("p");
    line.textContent = String(warning);
    warningRoot.append(line);
  });

  $("#task-result-json").textContent = JSON.stringify(result, null, 2);
  $("#task-result-note").textContent = "Run ID：" + (result.run_id || state.runId || "—") + " · 状态：" + runStatusLabel(result.status) + "。结果以 Go API 返回为准，仅供研究与工程验证，不构成交易建议。";
  $("#task-result-section").hidden = false;
  // Run 结果可能在用户停留自选股页面时到达，刷新卡片中的结果摘要。
  if (state.view === "watchlist") renderWatchlist();
}

function normalizedTaskSymbolSet(symbols) {
  return [...new Set(symbols.map(normalizeAnalysisSymbol))].sort();
}

function sameTaskSymbolSet(expectedRaw, actualRaw) {
  if (!Array.isArray(expectedRaw) || !Array.isArray(actualRaw)) return false;
  try {
    const expected = normalizedTaskSymbolSet(expectedRaw);
    const actual = normalizedTaskSymbolSet(actualRaw);
    return expected.length > 0 && JSON.stringify(expected) === JSON.stringify(actual);
  } catch {
    return false;
  }
}

function runResultSymbolMismatch(result) {
  const expectedRaw = Array.isArray(state.taskSymbols) ? state.taskSymbols : [];
  if (!expectedRaw.length) return "";
  const actualRaw = result?.request?.symbols;
  if (!Array.isArray(actualRaw)) {
    return "Run 结果未返回请求标的列表，无法确认是否属于当前任务，已忽略。";
  }
  return sameTaskSymbolSet(expectedRaw, actualRaw)
    ? ""
    : "Run 结果中的标的列表与当前任务不一致，已忽略。";
}

function setTaskRefreshBusy(busy) {
  const button = $("#refresh-task");
  if (!button) return;
  button.disabled = Boolean(busy);
  if (busy) {
    button.setAttribute("aria-busy", "true");
    button.textContent = "刷新中…";
  } else {
    button.removeAttribute("aria-busy");
    button.textContent = "立即刷新状态";
  }
}

function setRunResultReloadBusy(busy) {
  const button = $("#reload-run-result");
  if (!button) return;
  button.disabled = Boolean(busy);
  if (busy) {
    button.setAttribute("aria-busy", "true");
    button.textContent = "读取中…";
  } else {
    button.removeAttribute("aria-busy");
    button.textContent = "重新读取结果";
  }
}

function releaseRunResultReloadBusy() {
  try {
    setRunResultReloadBusy(false);
  } catch (error) {
    // 结果按钮的 DOM 更新失败不能中断任务/连接上下文的状态清理。
    try {
      const message = typeof error?.message === "string" ? error.message : "未知异常";
      state.runResultReadError = [
        state.runResultReadError,
        "恢复结果读取按钮失败：" + message
      ].filter(Boolean).join("；");
    } catch {
      // 状态记录也失败时仍必须继续完成上下文切换。
    }
  }
}

async function loadTaskRunResult(runId, expectedTaskId) {
  const analysisBase = state.taskAnalysisBase || state.settings.analysisBase;
  if (analysisBase !== state.settings.analysisBase || state.taskId !== expectedTaskId) return;
  if ($("#reload-run-result")?.disabled && ["loading", "retrying"].includes(state.runResultReadStatus)) return;
  const requestSettings = { ...state.settings, analysisBase };
  const endpointGeneration = state.analysisConnectionGeneration;
  const generation = ++state.runResultGeneration;
  let retries = 0;
  const retainedPreviousResult = Boolean(state.lastRunResult && state.lastRunResult.run_id === runId);
  const retainedResultNotice = retainedPreviousResult
    ? "页面保留的是此前已成功读取的同一 Run 结果，本次读取尚未确认。"
    : "";
  try {
    // 忙碌锁自身也可能遇到 DOM 异常，必须处于统一 catch/finally 保护内。
    setRunResultReloadBusy(true);
    state.runResultReadStatus = "loading";
    state.runResultReadError = "";
    if (state.view === "watchlist") renderWatchlist();
    $("#task-result-section").hidden = false;
    $("#task-result-note").textContent =
      "正在读取已保存的 Run 结果…" + (retainedResultNotice ? " " + retainedResultNotice : "");

    while (true) {
      if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId ||
          endpointGeneration !== state.analysisConnectionGeneration || state.taskAnalysisBase !== analysisBase ||
          state.settings.analysisBase !== analysisBase) return;

      try {
        const result = await getAnalysisRun(requestSettings, runId);
        if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId ||
            endpointGeneration !== state.analysisConnectionGeneration || state.taskAnalysisBase !== analysisBase ||
            state.settings.analysisBase !== analysisBase) return;
        if (result?.run_id !== runId) {
          throw responseIdentityError(result?.run_id
            ? "Run 结果接口返回了其他 Run 的结果，已忽略。"
            : "Run 结果接口未返回 run_id，已忽略该响应。");
        }
        const symbolMismatch = runResultSymbolMismatch(result);
        if (symbolMismatch) throw responseIdentityError(symbolMismatch);
        state.runId = result.run_id;
        persistTaskReference();
        renderTaskResult(result);
        return;
      } catch (error) {
        if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId ||
            endpointGeneration !== state.analysisConnectionGeneration || state.taskAnalysisBase !== analysisBase ||
            state.settings.analysisBase !== analysisBase) return;
        const delay = RUN_RESULT_RETRY_DELAYS_MS[retries];
        if (delay !== undefined && !error.responseContractError && isRetryableApiError(error)) {
          retries += 1;
          state.runResultReadStatus = "retrying";
          state.runResultReadError = errorMessage(error);
          if (state.view === "watchlist") renderWatchlist();
          $("#task-result-note").textContent =
            "Run 结果暂时不可读（" + errorMessage(error) + "），将在 " + delay / 1000 +
            " 秒后重试（" + retries + "/" + RUN_RESULT_RETRY_DELAYS_MS.length + "）…" +
            (retainedResultNotice ? " " + retainedResultNotice : "");
          await new Promise((resolve) => window.setTimeout(resolve, delay));
          continue;
        }
        $("#task-result-note").textContent =
          "读取结果失败：" + errorMessage(error) + "。可点击“重新读取结果”或“立即刷新状态”重试。" +
          (retainedPreviousResult ? " 页面保留的是此前已成功读取的同一 Run 结果，本次读取未成功，不代表结果已更新。" : "");
        state.runResultReadStatus = "error";
        state.runResultReadError = errorMessage(error);
        if (state.view === "watchlist") renderWatchlist();
        return;
      }
    }
  } catch (error) {
    if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId ||
        endpointGeneration !== state.analysisConnectionGeneration || state.taskAnalysisBase !== analysisBase ||
        state.settings.analysisBase !== analysisBase) return;

    const describeErrorSafely = (value) => {
      try {
        return errorMessage(value);
      } catch {
        return "未知异常";
      }
    };
    const appendReadDiagnostic = (message) => {
      try {
        state.runResultReadError = [state.runResultReadError, message].filter(Boolean).join("；");
      } catch {
        // 错误状态本身无法写入时，不再让诊断路径产生第二个异常。
      }
    };
    const message = describeErrorSafely(error);
    try {
      state.runResultReadStatus = "error";
    } catch {}
    try {
      state.runResultReadError = message;
    } catch {}

    try {
      const note = $("#task-result-note");
      if (note) {
        note.textContent =
          "读取 Run 结果时发生意外错误：" + message + "。请检查页面状态后重新读取。";
      }
    } catch (noteError) {
      appendReadDiagnostic("错误提示更新失败：" + describeErrorSafely(noteError));
    }

    if (state.view === "watchlist") {
      try {
        renderWatchlist();
      } catch (renderError) {
        appendReadDiagnostic("自选股卡片刷新失败：" + describeErrorSafely(renderError));
      }
    }
  } finally {
    // 旧任务的迟到 finally 不能解锁新任务的读取按钮；解锁异常也不得向外传播。
    if (generation === state.runResultGeneration) {
      try {
        setRunResultReloadBusy(false);
      } catch (unlockError) {
        try {
          state.runResultReadError = [
            state.runResultReadError,
            "重新启用结果读取按钮失败：" + (typeof unlockError?.message === "string" ? unlockError.message : "未知异常")
          ].filter(Boolean).join("；");
        } catch {
          // 最终收尾本身必须保持不抛错。
        }
      }
    }
  }
}
function stopTaskPolling() {
  const timer = state.pollTimer;
  state.pollTimer = null;
  // 先失效当前代际，再进行定时器与按钮清理；UI 故障不能让旧回包继续生效。
  state.pollGeneration += 1;
  try {
    if (timer) window.clearTimeout(timer);
  } catch (error) {
    try {
      const message = typeof error?.message === "string" ? error.message : "未知异常";
      state.taskStatusError = [state.taskStatusError, "清除任务轮询定时器失败：" + message]
        .filter(Boolean).join("；");
    } catch {}
  }
  try {
    setTaskRefreshBusy(false);
  } catch (error) {
    try {
      const message = typeof error?.message === "string" ? error.message : "未知异常";
      state.taskStatusError = [state.taskStatusError, "恢复任务刷新按钮失败：" + message]
        .filter(Boolean).join("；");
    } catch {
      // 按钮 DOM 不可用时仍保持代际已失效，不能中断调用方的上下文清理。
    }
  }
}

async function pollTask(generation = state.pollGeneration) {
  if (!state.taskId || generation !== state.pollGeneration) return;
  const taskId = state.taskId;
  const analysisBase = state.taskAnalysisBase || state.settings.analysisBase;
  if (analysisBase !== state.settings.analysisBase) return;
  const requestSettings = { ...state.settings, analysisBase };
  const endpointGeneration = state.analysisConnectionGeneration;
  const requestedEventCursor = state.taskEventCursor;
  const eventRequest = state.taskEventsUnavailable
    ? Promise.resolve({ skipped: true })
    : getAnalysisTaskEvents(requestSettings, taskId, requestedEventCursor);
  const [taskResult, eventResult] = await Promise.allSettled([
    getAnalysisTask(requestSettings, taskId),
    eventRequest
  ]);
  if (generation !== state.pollGeneration || taskId !== state.taskId || endpointGeneration !== state.analysisConnectionGeneration || state.taskAnalysisBase !== analysisBase || state.settings.analysisBase !== analysisBase) return;

  let task = null;
  let taskError = null;
  let eventError = null;
  let eventsSkipped = false;
  let eventPageHasMore = false;

  if (taskResult.status === "fulfilled") {
    const payload = taskResult.value;
    const candidate = payload?.task || payload;
    if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) {
      taskError = responseContractError("任务状态接口返回了无法识别的响应结构。");
    } else if (candidate.task_id !== taskId) {
      taskError = responseIdentityError(candidate.task_id
        ? "任务状态接口返回了其他任务的结果，已忽略。"
        : "任务状态接口未返回 task_id，已忽略该响应。");
    } else if (typeof candidate.run_id !== "string" || !candidate.run_id.trim()) {
      taskError = responseContractError("任务状态接口未返回有效 run_id，已暂停自动刷新。");
    } else if (state.runId && candidate.run_id !== state.runId) {
      taskError = responseIdentityError("任务状态接口返回的 run_id 与当前任务已关联的 Run 不一致，已忽略。");
    } else if (candidate.symbols !== undefined && !Array.isArray(candidate.symbols)) {
      taskError = responseContractError("任务状态接口返回的标的列表结构无效，已暂停自动刷新。");
    } else if (Array.isArray(candidate.symbols) && state.taskSymbols.length &&
               !sameTaskSymbolSet(state.taskSymbols, candidate.symbols)) {
      taskError = responseIdentityError("任务状态接口返回的标的列表与当前任务不一致，已忽略。");
    } else if (typeof candidate.status !== "string" || !TASK_STATUSES.has(candidate.status)) {
      taskError = responseContractError(candidate.status
        ? "任务状态接口返回了未支持的状态“" + String(candidate.status) + "”，已暂停自动刷新。"
        : "任务状态接口未返回有效状态，已暂停自动刷新。");
    } else {
      task = candidate;
      state.taskStatusLoading = false;
      state.taskStatusError = "";
      state.taskStatusErrorRetrying = false;
      renderTask(task);
    }
  } else {
    taskError = taskResult.reason;
  }

  if (eventResult.status === "fulfilled" && eventResult.value?.skipped === true) {
    eventsSkipped = true;
  } else if (eventResult.status === "fulfilled") {
    const eventPayload = eventResult.value;
    if (!eventPayload || typeof eventPayload !== "object" || Array.isArray(eventPayload) || !Array.isArray(eventPayload.events)) {
      eventError = responseContractError("阶段事件接口返回的数据结构无效。");
    } else if (eventPayload.task_id !== taskId) {
      eventError = responseIdentityError(eventPayload.task_id
        ? "阶段事件接口返回了其他任务的事件，已忽略。"
        : "阶段事件接口未返回 task_id，已忽略该响应。");
    } else {
      const nextAfterSequence = eventPayload.next_after_sequence;
      const highestReportedSequence = eventPayload.events.reduce((maximum, item) => {
        const sequence = typeof item?.sequence === "number" ? item.sequence : NaN;
        return Number.isSafeInteger(sequence) && sequence >= 1 ? Math.max(maximum, sequence) : maximum;
      }, requestedEventCursor);
      if (!Number.isSafeInteger(nextAfterSequence) ||
          nextAfterSequence < requestedEventCursor ||
          (eventPayload.events.length > 0 && nextAfterSequence <= requestedEventCursor) ||
          nextAfterSequence < highestReportedSequence) {
        eventError = responseContractError("阶段事件接口返回的 next_after_sequence 无效或未前进。");
      } else {
        // 请求游标独立于可显示的事件序号推进；单条坏事件不能卡住后续分页。
        state.taskEventCursor = nextAfterSequence;
        eventPageHasMore = eventPayload.events.length >= TASK_EVENT_PAGE_SIZE &&
          nextAfterSequence > requestedEventCursor;
        state.taskEventsLoaded = true;
        state.taskEventsError = "";
        let invalidStructureCount = 0;
        let mismatchedRunCount = 0;
        let ignoredSequenceCount = 0;
        const previousSequence = state.taskSequence;
        const expectedEventRunId = state.runId || task?.run_id || "";
        for (const item of eventPayload.events) {
          const sequence = typeof item?.sequence === "number" ? item.sequence : NaN;
          if (!item || typeof item !== "object" || Array.isArray(item) ||
              !Number.isSafeInteger(sequence) || sequence < 1 ||
              !item.event || typeof item.event !== "object" || Array.isArray(item.event)) {
            invalidStructureCount += 1;
            continue;
          }
          const eventRunId = typeof item.event.run_id === "string" ? item.event.run_id.trim() : "";
          if (!expectedEventRunId || !eventRunId || eventRunId !== expectedEventRunId) {
            mismatchedRunCount += 1;
            continue;
          }
          if (sequence <= previousSequence || state.taskEvents.has(sequence)) {
            ignoredSequenceCount += 1;
            continue;
          }
          const normalized = { ...item, sequence };
          state.taskEvents.set(sequence, normalized);
          state.taskSequence = Math.max(state.taskSequence, sequence);
        }
        const ignoredReasons = [];
        if (invalidStructureCount) {
          ignoredReasons.push("字段无效的阶段事件（序号或 event 结构不符合契约），已忽略 " + invalidStructureCount + " 条");
        }
        if (mismatchedRunCount) {
          ignoredReasons.push("Run ID 缺失或与当前任务关联的 Run 不一致，已忽略 " + mismatchedRunCount + " 条");
        }
        if (ignoredSequenceCount) {
          ignoredReasons.push("重复或旧序号的阶段事件，已忽略 " + ignoredSequenceCount + " 条");
        }
        if (ignoredReasons.length) state.taskEventsError = ignoredReasons.join("；") + "。";
        renderTaskEvents();
      }
    }
  } else {
    eventError = eventResult.reason;
  }

  if (eventError) {
    state.taskEventsError = errorMessage(eventError);
    if (!isRetryableApiError(eventError) || eventError.responseContractError) state.taskEventsUnavailable = true;
    renderTaskEvents();
  }

  if (taskError) {
    state.taskStatusLoading = false;
    state.taskStatusError = errorMessage(taskError);
    state.taskStatusErrorRetrying = isRetryableApiError(taskError) && !taskError.responseContractError;
    setQualityPill($("#task-status-pill"), "unknown", state.taskStatusErrorRetrying ? "状态刷新失败" : "状态读取失败");
    if (state.view === "watchlist") renderWatchlist();
  }

  // A permanent task lookup failure means the current task reference cannot be polled safely.
  if (taskError && (!isRetryableApiError(taskError) || taskError.responseContractError)) {
    stopTaskPolling();
    $("#task-poll-label").textContent =
      "自动刷新已暂停：任务状态读取失败：" + errorMessage(taskError) +
      (eventError ? "；阶段事件读取失败：" + errorMessage(eventError) : "") +
      "。请检查权限、API 配置或任务 ID，再点击“立即刷新状态”重试。";
    return;
  }

  if (task && TERMINAL_TASK_STATUSES.has(task.status)) {
    if (eventError && isRetryableApiError(eventError) && !eventError.responseContractError) {
      $("#task-poll-label").textContent =
        "任务已结束；阶段事件暂时读取失败：" + errorMessage(eventError) +
        "，5 秒后自动重试；续读完成前不会加载 Run 报告。";
      state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 5000);
      return;
    }
    if (eventPageHasMore && !eventError && !eventsSkipped) {
      $("#task-poll-label").textContent = "任务已结束，正在继续读取剩余阶段事件…";
      state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 0);
      return;
    }
    stopTaskPolling();
    if (eventError) {
      $("#task-poll-label").textContent =
        "任务已结束；阶段事件读取失败：" + errorMessage(eventError) +
        "。可点击“立即刷新状态”重新尝试读取事件。";
    } else if (eventsSkipped) {
      $("#task-poll-label").textContent = "任务已结束；阶段事件读取此前被暂停，可手动刷新重试。";
    }
    const runId = task.run_id || state.runId;
    if (runId) void loadTaskRunResult(runId, taskId);
    return;
  }

  if (taskError) {
    $("#task-poll-label").textContent =
      "任务状态刷新失败：" + errorMessage(taskError) +
      (eventError ? "；阶段事件刷新失败：" + errorMessage(eventError) : "") +
      "，5 秒后自动重试…";
    state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 5000);
    return;
  }

  if (eventError) {
    if (isRetryableApiError(eventError) && !eventError.responseContractError) {
      $("#task-poll-label").textContent =
        "任务状态已更新，但阶段事件读取失败：" + errorMessage(eventError) + "，5 秒后重试…";
      state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 5000);
    } else {
      $("#task-poll-label").textContent =
        "任务状态正常刷新；阶段事件接口暂不可用：" + errorMessage(eventError) +
        "。已暂停事件请求以避免重复失败；点击“立即刷新状态”可重试。";
      state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 2500);
    }
    return;
  }

  if (eventPageHasMore) {
    $("#task-poll-label").textContent = "阶段事件较多，正在继续读取后续事件…";
    state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 0);
    return;
  }
  if (eventsSkipped) {
    state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 2500);
    return;
  }
  state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 2500);
}


function pollTaskSafely(generation = state.pollGeneration) {
  return pollTask(generation).catch((error) => {
    // 将异常恢复也视为不可信路径：其中任一 UI 更新失败都不能再次拒绝 Promise。
    try {
      // 旧任务的迟到异常不能覆盖当前任务状态或暂停新任务的轮询。
      if (generation !== state.pollGeneration || !state.taskId) return;

      let message = "未知异常";
      try {
        message = errorMessage(error);
      } catch {
        // 错误对象本身不可格式化时仍继续完成轮询收尾。
      }

      const appendRecoveryError = (label, recoveryError) => {
        try {
          state.taskStatusError = [state.taskStatusError, label + errorMessage(recoveryError)]
            .filter(Boolean)
            .join("；");
        } catch {
          // 诊断记录失败不能让异常处理路径再次抛错。
        }
      };

      try {
        state.taskStatusLoading = false;
      } catch (cleanupError) {
        appendRecoveryError("清除状态加载标记失败：", cleanupError);
      }
      try {
        state.taskStatusError = message;
      } catch (cleanupError) {
        appendRecoveryError("记录任务状态错误失败：", cleanupError);
      }
      try {
        state.taskStatusErrorRetrying = false;
      } catch (cleanupError) {
        appendRecoveryError("更新状态重试标记失败：", cleanupError);
      }

      // 先停轮询，再尽力更新诊断；即使某个 DOM 节点或视图渲染异常也不再冒泡。
      try {
        stopTaskPolling();
      } catch (cleanupError) {
        appendRecoveryError("停止自动轮询失败：", cleanupError);
      }
      try {
        setQualityPill($("#task-status-pill"), "unknown", "状态处理失败");
      } catch (renderError) {
        appendRecoveryError("状态徽标更新失败：", renderError);
      }
      try {
        const pollLabel = $("#task-poll-label");
        if (pollLabel) {
          pollLabel.textContent =
            "自动刷新已暂停：处理任务状态时发生意外错误：" + message +
            "。请检查页面状态；问题排除后可点击“立即刷新状态”重试。";
        }
      } catch (renderError) {
        appendRecoveryError("轮询诊断更新失败：", renderError);
      }
      if (state.view === "watchlist") {
        try {
          renderWatchlist();
        } catch (renderError) {
          appendRecoveryError("自选股卡片刷新失败：", renderError);
        }
      }
    } catch {
      // 最终兜底：异常收敛中的异常也不得形成新的未处理 Promise rejection。
      try {
        state.taskStatusLoading = false;
      } catch {}
      try {
        state.taskStatusErrorRetrying = false;
      } catch {}
      try {
        stopTaskPolling();
      } catch {}
    }
  });
}

async function createTask(event) {
  event.preventDefault();
  const feedback = $("#task-create-feedback");
  if (state.isCreatingTask) {
    feedback.hidden = false;
    feedback.textContent = "分析任务正在提交，请等待本次请求完成后再创建下一项。";
    return;
  }
  state.isCreatingTask = true;

  let symbols;
  try {
    symbols = parseAnalysisSymbols($("#task-symbols").value);
  } catch (error) {
    state.isCreatingTask = false;
    feedback.hidden = false;
    feedback.textContent = error.message;
    return;
  }
  let button = null;
  let taskType = "";
  let payload = null;
  let idempotencyKey = "";
  let requestAnalysisBase = state.settings.analysisBase;
  let requestAnalysisGeneration = state.analysisConnectionGeneration;
  let requestSettings = { ...state.settings, analysisBase: requestAnalysisBase };
  let creationContextGeneration = state.taskContextGeneration;
  let creationHistoryGeneration = state.taskHistoryGeneration;

  try {
  feedback.hidden = true;
  feedback.textContent = "";
  button = $("#create-task-button");
  button.disabled = true;
  button.textContent = "正在写入任务队列…";
  taskType = $("#task-type").value;
  payload = { task_type: taskType, symbols };
  idempotencyKey = $("#idempotency-key").value.trim();
  requestAnalysisBase = state.settings.analysisBase;
  requestAnalysisGeneration = state.analysisConnectionGeneration;
  requestSettings = { ...state.settings, analysisBase: requestAnalysisBase };
  state.taskContextGeneration += 1;
  creationContextGeneration = state.taskContextGeneration;
  creationHistoryGeneration = state.taskHistoryGeneration;
  state.runResultGeneration += 1;
  releaseRunResultReloadBusy();
  stopTaskPolling();
  state.taskId = "";
  state.taskCreatedAt = "";
  state.runId = "";
  state.taskAnalysisBase = requestAnalysisBase;
  state.taskSymbols = symbols;
  state.taskType = taskType;
  state.taskStatus = "";
  state.taskStatusLoading = false;
  state.taskStatusError = "";
  state.taskStatusErrorRetrying = false;
  setQualityPill($("#task-status-pill"), "unknown", "正在创建");
  state.lastRunResult = null;
  state.runResultReadStatus = "";
  state.runResultReadError = "";
  state.taskSequence = 0;
  state.taskEventCursor = 0;
  state.taskEvents.clear();
  state.taskEventFilter = "all";
  state.taskEventsLoaded = false;
  state.taskEventsUnavailable = false;
  state.taskEventsError = "";
  state.selectedTaskEventSequence = null;
  state.expandedTaskEventSequences.clear();
  $("#task-result-section").hidden = true;
  $("#run-flow-result-empty")?.removeAttribute("hidden");
  $("#task-result-metrics").replaceChildren();
  $("#task-result-structured").replaceChildren();
  $("#task-result-warnings").replaceChildren();

    const response = await createAnalysisTask(requestSettings, payload, idempotencyKey);
    const task = response.task || response;
    const responseSymbolsMatch = task?.symbols === undefined ||
      (Array.isArray(task.symbols) && sameTaskSymbolSet(symbols, task.symbols));
    const responseStatusValid = typeof task?.status === "string" && TASK_STATUSES.has(task.status);
    const responseRunIdValid = typeof task?.run_id === "string" && Boolean(task.run_id.trim());
    // POST 回包不完整时保留请求上下文，随后由 Task 状态接口补齐权威字段。
    const responseRunId = responseSymbolsMatch && responseRunIdValid ? task.run_id : "";
    const responseStatus = responseSymbolsMatch && responseStatusValid ? task.status : "";
    const responseCreatedAt = typeof task?.created_at === "string" ? task.created_at : "";
    const creationResponseWarnings = [];
    if (!responseStatusValid) creationResponseWarnings.push("状态缺失或不受支持");
    if (!responseRunIdValid) creationResponseWarnings.push("Run ID 缺失或格式无效");
    if (!responseSymbolsMatch) creationResponseWarnings.push("回包标的列表无效或与本次请求不一致");
    const taskHistoryWasReplaced = creationHistoryGeneration !== state.taskHistoryGeneration;
    if (requestAnalysisGeneration !== state.analysisConnectionGeneration || requestAnalysisBase !== state.settings.analysisBase) {
      if (typeof task?.task_id === "string" && task.task_id.trim()) {
        const endpointRecord = {
          task_id: task.task_id,
          run_id: responseRunId,
          created_at: responseCreatedAt,
          symbols,
          task_type: taskType,
          status: responseStatus,
          updated_at: new Date().toISOString(),
          analysis_base: requestAnalysisBase
        };
        if (taskHistoryWasReplaced) {
          setRecentTasksFeedback(
            "任务已在旧分析 API 地址创建，但本地最近任务历史在提交期间已被清空或替换；按当前操作未重新写入该引用。任务 ID：" + task.task_id + "。切回原地址后可手动核实。",
            "warning"
          );
          feedback.hidden = false;
          feedback.textContent = "旧连接已返回任务 ID，但本地历史已被替换；本次未重新写入任务引用。任务 ID：" + task.task_id + "。";
        } else {
          const historyPersisted = persistRecentTaskHistory(endpointRecord);
          setRecentTasksFeedback(
            historyPersisted
              ? "任务已在旧分析 API 地址 " + requestAnalysisBase + " 创建，并已保存到最近分析列表。当前地址已改变；切回原地址后才能继续读取。"
              : "任务已在旧分析 API 地址创建，但最近任务引用未能确认持久化。任务 ID：" + task.task_id + "；请切回原地址核实。",
            "warning"
          );
          feedback.hidden = false;
          feedback.textContent = historyPersisted
            ? "任务已在旧连接创建并记录；当前界面不会把它绑定到新分析 API。切回原地址后可从最近分析恢复。"
            : "任务已在旧连接创建，但本地历史未能确认保存。任务 ID：" + task.task_id + "。";
        }
      } else {
        feedback.hidden = false;
        feedback.textContent = "提交期间分析 API 地址已变化，返回中没有可用 task_id。无法在当前连接确认任务状态；请核实旧服务，避免盲目重复提交。";
      }
      return;
    }
    // 创建请求等待期间可能切换到历史任务；晚到的有效结果只保留引用，不接管当前页面。
    if (creationContextGeneration !== state.taskContextGeneration) {
      if (typeof task?.task_id === "string" && task.task_id.trim()) {
        const endpointRecord = {
          task_id: task.task_id,
          run_id: responseRunId,
          created_at: responseCreatedAt,
          symbols,
          task_type: taskType,
          status: responseStatus,
          updated_at: new Date().toISOString(),
          analysis_base: requestAnalysisBase
        };
        if (taskHistoryWasReplaced) {
          setRecentTasksFeedback(
            "新建任务已创建，但本地最近任务历史在提交期间已被清空或替换；按当前操作未重新写入该引用。任务 ID：" + task.task_id + "。",
            "warning"
          );
          feedback.hidden = false;
          feedback.textContent = "提交前的任务请求已返回任务 ID，但本地历史已被替换；本次未重新写入任务引用。任务 ID：" + task.task_id + "。";
        } else {
          const historyPersisted = persistRecentTaskHistory(endpointRecord);
          setRecentTasksFeedback(
            historyPersisted
              ? "新建任务已创建并保存到最近分析列表；当前选中的任务不会被覆盖。"
              : "新建任务已创建，但最近任务引用未能确认持久化。任务 ID：" + task.task_id + "；当前选中的任务不会被覆盖。",
            "warning"
          );
          feedback.hidden = false;
          feedback.textContent = historyPersisted
            ? "新建任务 " + task.task_id + " 已记录到本地历史；页面继续跟踪当前选中的任务。"
            : "新建任务 " + task.task_id + " 已创建，但本地历史未能确认保存。页面继续跟踪当前选中的任务。";
        }
      } else {
        feedback.hidden = false;
        feedback.textContent = "提交期间当前任务已切换，但新建任务响应没有有效 task_id，无法安全绑定或记录。请核实服务端任务记录，避免盲目重复提交。";
      }
      return;
    }
    if (!task || typeof task.task_id !== "string" || !task.task_id.trim()) {
      const error = new Error("分析服务返回成功响应，但缺少有效 task_id，无法确认任务是否已创建。请检查服务端任务记录后再决定是否重试，避免盲目重复提交。");
      error.creationOutcomeUnknown = true;
      throw error;
    }
    state.suppressedHistoryTaskId = "";
    state.suppressedHistoryTaskBase = "";
    state.taskId = task.task_id;
    state.taskCreatedAt = responseCreatedAt;
    state.runId = responseRunId;
    state.taskAnalysisBase = requestAnalysisBase;
    state.taskSymbols = symbols;
    state.taskType = taskType;
    state.taskStatus = responseStatus;
    state.taskStatusLoading = creationResponseWarnings.length > 0;
    persistTaskReference();
    feedback.hidden = true;
    const normalizedTask = {
      ...task,
      task_id: task.task_id,
      run_id: responseRunId,
      status: responseStatus,
      task_type: taskType,
      symbols,
      current_stage: responseSymbolsMatch ? task.current_stage : "等待服务端确认",
      completed_stages: responseSymbolsMatch ? task.completed_stages : undefined,
      failure_code: responseSymbolsMatch ? task.failure_code : "",
      error_message: responseSymbolsMatch ? task.error_message : ""
    };
    renderTask(normalizedTask);
    if (state.taskStatusLoading) {
      setQualityPill($("#task-status-pill"), "unknown", "正在确认");
    }
    if (creationResponseWarnings.length) {
      $("#task-poll-label").textContent = "创建任务已确认；回包" + creationResponseWarnings.join("、") +
        "。已保留本次请求标的，正在通过服务端状态接口核实。";
    }
    renderTaskEvents();
    openRunFlowDrawer();
    const generation = state.pollGeneration;
    // 即使幂等命中已结束任务，也先读取并排空阶段事件，再加载最终 Run 报告。
    state.pollTimer = window.setTimeout(() => pollTaskSafely(generation), 500);
  } catch (error) {
    try {
    if (creationContextGeneration !== state.taskContextGeneration) {
      feedback.hidden = false;
      feedback.textContent = requestAnalysisGeneration !== state.analysisConnectionGeneration || requestAnalysisBase !== state.settings.analysisBase
        ? "提交期间分析 API 地址已变化且当前任务上下文已切换，无法确认旧连接是否创建了任务：" + errorMessage(error) + "。请核实旧服务后再决定是否重试。"
        : "提交期间当前任务已切换；本次创建失败或结果未知，没有覆盖当前任务界面：" + errorMessage(error);
      return;
    }
    $("#task-empty").hidden = false;
    $("#task-detail").hidden = true;
    feedback.hidden = false;
    feedback.textContent = requestAnalysisGeneration !== state.analysisConnectionGeneration || requestAnalysisBase !== state.settings.analysisBase
      ? "分析 API 地址在提交期间已变化，无法确认旧连接是否创建了任务：" + errorMessage(error) + "。请核实旧服务后再决定是否重试。"
      : error?.creationOutcomeUnknown ? error.message : "创建任务失败：" + errorMessage(error);
    } catch (recoveryError) {
      try {
        feedback.hidden = false;
        const message = typeof recoveryError?.message === "string" ? recoveryError.message : "未知异常";
        feedback.textContent = "处理任务提交结果时发生异常，且无法更新原始错误提示：" + message;
      } catch {
        // 错误反馈不可用时，仍必须进入 finally 恢复提交锁。
      }
    }
  } finally {
    // 先释放逻辑提交锁，再尽力恢复 DOM；恢复失败也不能让 Promise 再次拒绝。
    state.isCreatingTask = false;
    try {
      if (button) {
        button.disabled = false;
        button.removeAttribute("aria-busy");
        button.replaceChildren(document.createTextNode("创建并跟踪任务 "));
        const arrow = document.createElement("span");
        arrow.textContent = "→";
        button.append(arrow);
      }
    } catch (unlockError) {
      try {
        feedback.hidden = false;
        const message = typeof unlockError?.message === "string" ? unlockError.message : "未知异常";
        feedback.textContent = "任务提交流程已结束，但创建按钮状态恢复失败：" + message + "。请重新载入页面后重试。";
      } catch {
        // finally 中的反馈或 DOM 异常不得形成未处理的 Promise rejection。
      }
    }
  }
}

function startAnalysisForSymbol(input) {
  let symbol;
  try {
    symbol = normalizeAnalysisSymbol(input);
  } catch (error) {
    setNotice(error.message, "danger");
    return;
  }
  $("#task-symbols").value = symbol;
  $("#task-type").value = "post_market_scan";
  setView("tasks");
  $("#task-form").requestSubmit();
}

function startQuoteAnalysis() {
  startAnalysisForSymbol(state.quote?.symbol || $("#symbol-input").value);
}

function activateTaskReference(record) {
  if (!record || typeof record.task_id !== "string" || !record.task_id.trim()) return false;
  const requestedBase = String(record.analysis_base || state.settings.analysisBase);
  if (requestedBase !== state.settings.analysisBase) {
    setRecentTasksFeedback(
      "该任务属于分析 API 地址 " + requestedBase + "，当前地址是 " + state.settings.analysisBase +
      "。已阻止向新地址读取旧 task_id；切回原地址后可以恢复此任务。",
      "warning"
    );
    return false;
  }
  state.suppressedHistoryTaskId = "";
  state.suppressedHistoryTaskBase = "";
  state.taskContextGeneration += 1;
  state.runResultGeneration += 1;
  releaseRunResultReloadBusy();
  stopTaskPolling();
  state.taskId = record.task_id;
  state.taskCreatedAt = typeof record.created_at === "string" ? record.created_at : "";
  state.runId = typeof record.run_id === "string" ? record.run_id : "";
  state.taskAnalysisBase = requestedBase;
  state.taskSymbols = Array.isArray(record.symbols) ? record.symbols : [];
  state.taskType = record.task_type || "post_market_scan";
  state.taskStatus = record.status || "";
  state.taskStatusLoading = true;
  state.taskStatusError = "";
  state.taskStatusErrorRetrying = false;
  state.lastRunResult = null;
  state.runResultReadStatus = "";
  state.runResultReadError = "";
  state.taskSequence = 0;
  state.taskEventCursor = 0;
  state.taskEvents.clear();
  state.taskEventFilter = "all";
  state.taskEventsLoaded = false;
  state.taskEventsUnavailable = false;
  state.taskEventsError = "";
  state.selectedTaskEventSequence = null;
  state.expandedTaskEventSequences.clear();
  try {
    $("#task-symbols").value = state.taskSymbols.join(", ") || "600519";
    if (["post_market_scan", "watchlist_scan", "position_review"].includes(state.taskType)) {
      $("#task-type").value = state.taskType;
    }
    $("#task-empty").hidden = true;
    $("#task-detail").hidden = false;
    $("#task-result-section").hidden = true;
    $("#run-flow-result-empty")?.removeAttribute("hidden");
    $("#task-result-metrics").replaceChildren();
    $("#task-result-structured").replaceChildren();
    $("#task-result-warnings").replaceChildren();
    setText("#task-id-value", state.taskId);
    setText("#task-run-id", state.runId);
    setText("#task-current-stage", "正在恢复任务");
    setQualityPill($("#task-status-pill"), "unknown", "正在恢复");
    $("#task-poll-label").textContent = "正在从服务端恢复状态与阶段事件…";
    setView("tasks");
    renderRunStatus("");
    openRunFlowDrawer();
    renderTaskEvents();
    persistTaskReference();
  } catch (error) {
    let message = "未知异常";
    try {
      message = errorMessage(error);
    } catch {
      // 恢复视图自身发生异常时，仍必须启动当前任务的权威状态读取。
    }
    try {
      state.taskStatusError = "恢复任务界面时发生错误：" + message;
    } catch {}
    try {
      const pollLabel = $("#task-poll-label");
      if (pollLabel) {
        pollLabel.textContent =
          "任务引用已切换，但部分界面更新失败；正在继续读取服务端状态。详情：" + message;
      }
    } catch {
      // 诊断节点不可用时仍继续发起安全轮询。
    }
  }
  void pollTaskSafely(state.pollGeneration);
  return true;
}
function restoreLastTask() {
  let record;
  try {
    record = JSON.parse(localStorage.getItem(TASK_REFERENCE_KEY) || "null");
  } catch {
    return false;
  }
  return activateTaskReference(record);
}

async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through when the clipboard API exists without permission.
    }
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.append(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) throw new Error("浏览器未允许复制，请手动展开原始 JSON 后复制。");
}

function setMarketKeyVisibility(visible) {
  const input = $("#market-key-input");
  const button = $("#toggle-market-key-visibility");
  input.type = visible ? "text" : "password";
  button.textContent = visible ? "隐藏" : "显示";
  button.setAttribute("aria-pressed", String(visible));
}

function readConnectionDraft() {
  return {
    marketBase: $("#market-base-input").value,
    analysisBase: $("#analysis-base-input").value,
    apiKey: $("#market-key-input").value
  };
}

function connectionDraftIsDirty() {
  return connectionDraftChanged(initialConnectionDraft, readConnectionDraft());
}

function syncConnectionDraftState() {
  const dirty = connectionDraftIsDirty();
  $("#connection-dirty-state").hidden = !dirty;
  $("#connection-dirty-state").textContent = dirty
    ? "存在未保存的连接配置更改。保存只会修改当前浏览器连接配置，不会更改服务器 .env。"
    : "没有未保存的连接配置更改。";
  $("#save-settings").disabled = !dirty;
  return dirty;
}

function setupSettingsNavigation() {
  const nav = document.querySelector(".settings-nav");
  if (!nav || nav.dataset.enhanced === "true") return;
  nav.dataset.enhanced = "true";

  const tools = document.createElement("div");
  tools.className = "settings-nav-tools";
  const searchLabel = document.createElement("label");
  searchLabel.className = "settings-nav-search";
  const searchHint = document.createElement("span");
  searchHint.className = "sr-only";
  searchHint.textContent = "搜索设置分组";
  const searchInput = document.createElement("input");
  searchInput.type = "search";
  searchInput.id = "settings-section-search";
  searchInput.placeholder = "搜索设置分组";
  searchInput.autocomplete = "off";
  searchLabel.append(searchHint, searchInput);

  const actions = document.createElement("div");
  actions.className = "settings-nav-actions";
  const expandAll = document.createElement("button");
  expandAll.type = "button";
  expandAll.dataset.settingsExpandAll = "";
  expandAll.textContent = "全部展开";
  const collapseAll = document.createElement("button");
  collapseAll.type = "button";
  collapseAll.dataset.settingsCollapseAll = "";
  collapseAll.textContent = "全部收起";
  actions.append(expandAll, collapseAll);

  const resultStatus = document.createElement("p");
  resultStatus.className = "settings-nav-result";
  resultStatus.id = "settings-nav-result";
  resultStatus.setAttribute("aria-live", "polite");
  tools.append(searchLabel, actions, resultStatus);
  nav.prepend(tools);

  const links = Array.from(nav.querySelectorAll(".settings-nav-link"));
  const sections = links.map((link) => document.getElementById(link.dataset.settingsTarget)).filter(Boolean);
  sections.forEach((section) => {
    const heading = section.querySelector(":scope > .settings-section-heading");
    if (!heading || heading.querySelector("[data-settings-section-toggle]")) return;
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "settings-section-toggle";
    toggle.dataset.settingsSectionToggle = "true";
    toggle.setAttribute("aria-expanded", "true");
    toggle.textContent = "收起内容";
    toggle.setAttribute("aria-label", "收起" + (heading.querySelector("h2")?.textContent || "设置") + "内容");
    heading.append(toggle);
  });

  const setCollapsed = (section, collapsed) => {
    const heading = section.querySelector(":scope > .settings-section-heading");
    const toggle = heading?.querySelector("[data-settings-section-toggle]");
    if (!heading || !toggle) return;
    Array.from(section.children).filter((child) => child !== heading).forEach((child) => {
      child.hidden = collapsed;
    });
    toggle.setAttribute("aria-expanded", String(!collapsed));
    toggle.textContent = collapsed ? "展开内容" : "收起内容";
    toggle.setAttribute("aria-label", (collapsed ? "展开" : "收起") +
      (heading.querySelector("h2")?.textContent || "设置") + "内容");
    section.classList.toggle("is-collapsed", collapsed);
  };

  nav.addEventListener("click", (event) => {
    const toggle = event.target.closest("[data-settings-section-toggle]");
    if (toggle) {
      const section = toggle.closest(".settings-section");
      if (section) setCollapsed(section, toggle.getAttribute("aria-expanded") === "true");
      return;
    }
    if (event.target.closest("[data-settings-expand-all]")) {
      sections.forEach((section) => setCollapsed(section, false));
      return;
    }
    if (event.target.closest("[data-settings-collapse-all]")) {
      sections.forEach((section) => setCollapsed(section, true));
    }
  });

  const search = tools.querySelector("#settings-section-search");
  const result = tools.querySelector("#settings-nav-result");
  search.addEventListener("input", () => {
    const query = search.value.trim().toLocaleLowerCase();
    let visible = 0;
    links.forEach((link) => {
      const section = document.getElementById(link.dataset.settingsTarget);
      const matched = !query || (link.textContent + " " + (section?.textContent || ""))
        .toLocaleLowerCase().includes(query);
      link.hidden = !matched;
      if (matched) visible += 1;
    });
    result.textContent = query ? "找到 " + visible + " 个设置分组" : "共 " + links.length + " 个设置分组";
  });
  result.textContent = "共 " + links.length + " 个设置分组";
}

function openSettings() {
  $("#market-base-input").value = state.settings.marketBase;
  // Never echo the persisted API key into the input DOM, even though it is password-masked.
  $("#market-key-input").value = "";
  $("#market-key-input").placeholder = state.settings.apiKey
    ? "已配置 Key；留空保留当前值"
    : "填写 MARKET_DATA_API_KEY";
  setMarketKeyVisibility(false);
  $("#analysis-base-input").value = state.settings.analysisBase;
  $("#settings-feedback").textContent = state.settings.apiKey
    ? "已保存的 Key 不会回显；留空会保留它。清除操作会删除当前浏览器保存的 Key。"
    : "连接配置仅保存在当前浏览器。";
  initialConnectionDraft = readConnectionDraft();
  syncConnectionDraftState();
  $("#settings-check-draft-connection").disabled = false;
  $("#settings-check-draft-connection").removeAttribute("aria-busy");
  $("#settings-check-draft-connection").textContent = "检查未保存配置";
  $("#settings-draft-connection-check").textContent = "尚未检查未保存配置。";
  $("#settings-draft-connection-check").className = "settings-inline-status";

}

function safelyUpdateTaskContextUi(label, update) {
  try {
    update();
  } catch (error) {
    let message = "未知异常";
    try {
      message = errorMessage(error);
    } catch {
      // 诊断格式化失败时仍需允许设置保存流程继续。
    }
    try {
      state.taskStatusError = [state.taskStatusError, label + message].filter(Boolean).join("；");
    } catch {}
    try {
      const feedback = $("#task-create-feedback");
      if (feedback) {
        feedback.hidden = false;
        feedback.textContent =
          "分析 API 地址已变化，旧任务状态已作废；部分界面清理失败：" + label + message +
          "。请重新载入页面核对显示。";
      }
    } catch {
      // 视图与反馈节点均不可用时，不能阻断配置应用流程。
    }
  }
}

function invalidateActiveTaskForAnalysisBaseChange(previousBase, nextBase) {
  state.taskContextGeneration += 1;
  state.analysisConnectionGeneration += 1;
  state.runResultGeneration += 1;
  releaseRunResultReloadBusy();
  stopTaskPolling();
  state.taskId = "";
  state.taskCreatedAt = "";
  state.runId = "";
  state.taskAnalysisBase = "";
  state.taskSymbols = [];
  state.taskType = "post_market_scan";
  state.taskStatus = "";
  state.taskStatusLoading = false;
  state.taskStatusError = "";
  state.taskStatusErrorRetrying = false;
  state.lastRunResult = null;
  state.runResultReadStatus = "";
  state.runResultReadError = "";
  state.taskSequence = 0;
  state.taskEventCursor = 0;
  state.taskEvents.clear();
  state.taskEventFilter = "all";
  state.taskEventsLoaded = false;
  state.taskEventsUnavailable = false;
  state.taskEventsError = "";
  state.selectedTaskEventSequence = null;
  state.expandedTaskEventSequences.clear();
  safelyUpdateTaskContextUi("清理任务详情失败：", () => {
    $("#task-empty").hidden = false;
    $("#task-detail").hidden = true;
    $("#task-result-section").hidden = true;
  $("#run-flow-result-empty")?.removeAttribute("hidden");
    $("#task-result-metrics").replaceChildren();
    $("#task-result-structured").replaceChildren();
    $("#task-result-warnings").replaceChildren();
  });
  safelyUpdateTaskContextUi("更新 API 地址切换提示失败：", () => {
    $("#task-poll-label").textContent = "分析 API 地址已变化，旧任务不再自动轮询。";
    $("#task-create-feedback").hidden = false;
    $("#task-create-feedback").textContent = "分析 API 地址已从 " + previousBase + " 改为 " + nextBase +
      "。旧任务引用仍保留在最近分析列表；切回原地址后才能恢复，当前不会向新地址发送旧 task_id。";
  });
  safelyUpdateTaskContextUi("重绘阶段事件失败：", () => renderTaskEvents());
  safelyUpdateTaskContextUi("重绘任务流程摘要失败：", () => renderTaskFlowSummary());
  safelyUpdateTaskContextUi("重绘最近任务失败：", () => renderRecentTasks());
  safelyUpdateTaskContextUi("更新最近任务反馈失败：", () => setRecentTasksFeedback(
    "分析 API 地址已从 " + previousBase + " 改为 " + nextBase +
    "。旧任务引用仍保留并标注来源地址；切回原地址后可恢复。",
    "warning"
  ));
  // 地址切换已清空活动任务/Run；自选股卡片失败不阻断其他连接配置的失效处理。
  if (state.view === "watchlist") {
    safelyUpdateTaskContextUi("重绘自选股卡片失败：", () => renderWatchlist());
  }
}
function reportSettingsApplyFailures(failures) {
  if (!failures.length) return;
  const message = "配置已保存，但部分界面/缓存刷新步骤失败：" + failures.join("；") +
    "。请重新打开设置检查，并重新载入页面复核。";
  try {
    const output = $("#settings-connection-check");
    if (output) {
      output.textContent = message;
      output.className = "settings-inline-status is-warning";
    }
  } catch {}
  try {
    const feedback = $("#settings-feedback");
    if (feedback) feedback.textContent = message;
  } catch {}
  try {
    const label = $("#connection-label");
    if (label) label.title = message;
  } catch {}
}

function runSettingsApplyStep(label, operation, failures) {
  try {
    const result = operation();
    if (result && typeof result.then === "function") {
      return result.catch((error) => {
        let message = "未知异常";
        try {
          message = errorMessage(error);
        } catch {}
        failures.push(label + message);
        reportSettingsApplyFailures(failures);
      });
    }
    return result;
  } catch (error) {
    let message = "未知异常";
    try {
      message = errorMessage(error);
    } catch {}
    failures.push(label + message);
    reportSettingsApplyFailures(failures);
    return undefined;
  }
}

function applySettings(nextSettings, { persistPreviousActiveTask = true } = {}) {
  const previousSettings = state.settings;
  const analysisBaseChanged = previousSettings.analysisBase !== nextSettings.analysisBase;
  const marketConnectionChanged = previousSettings.apiKey !== nextSettings.apiKey ||
    previousSettings.marketBase !== nextSettings.marketBase;
  const failures = [];

  // 保存上一任务的引用属于辅助操作；存储异常不能阻止已持久化的新连接配置生效。
  if (analysisBaseChanged && state.taskId && persistPreviousActiveTask) {
    if (!state.taskAnalysisBase) state.taskAnalysisBase = previousSettings.analysisBase;
    runSettingsApplyStep("保存旧任务引用失败：", () => persistTaskReference(), failures);
  }

  state.settings = nextSettings;

  if (analysisBaseChanged) {
    runSettingsApplyStep("清理旧分析任务失败：", () =>
      invalidateActiveTaskForAnalysisBaseChange(previousSettings.analysisBase, nextSettings.analysisBase), failures);
  }

  if (marketConnectionChanged) {
    runSettingsApplyStep("失效自选股批量行情失败：", () => invalidateWatchlistQuotes(), failures);
    runSettingsApplyStep("失效市场总览请求失败：", () => marketOverview.invalidateConnection(), failures);
    runSettingsApplyStep("失效市场活动请求失败：", () => marketActivity.invalidateConnection(), failures);
    runSettingsApplyStep("失效全市场分页行情失败：", () => marketStocks.invalidate(), failures);
    runSettingsApplyStep("清理旧行情结果失败：", () => cancelQuoteQuery({
      clearResults: true,
      symbol: $("#symbol-input").value.trim(),
      message: "行情连接配置已变更，旧快照已清除，请重新加载。"
    }), failures);
    runSettingsApplyStep("取消股票搜索请求失败：", () => cancelStockSearchRequest(), failures);
    state.stockSearchResults = [];
    runSettingsApplyStep("清理股票搜索结果失败：", () => $("#stock-search-results").replaceChildren(), failures);
    runSettingsApplyStep("关闭股票搜索面板失败：", () => hideStockSearch(), failures);
    runSettingsApplyStep("失效个股研究请求失败：", () =>
      stockResearch.invalidate("行情连接配置已变更，旧研究请求已作废，请重新加载对应资料。"), failures);
  }

  if (analysisBaseChanged || marketConnectionChanged) {
    // 先让在途连接检查失效，再分别恢复按钮、结果提示和数据中心状态。
    savedConnectionCheckGeneration += 1;
    runSettingsApplyStep("恢复连接检查按钮失败：", () => {
      const button = $("#settings-check-connection");
      button.disabled = false;
      button.removeAttribute("aria-busy");
      button.textContent = "检查连接";
    }, failures);
    runSettingsApplyStep("更新连接检查提示失败：", () => {
      const output = $("#settings-connection-check");
      output.textContent = "连接配置已变化，请重新检查当前已保存的配置。";
      output.className = "settings-inline-status is-warning";
    }, failures);
    runSettingsApplyStep("失效数据中心检查结果失败：", () =>
      dataCenter.invalidate("连接配置已变更，旧的数据中心检查结果已作废。请使用新配置重新检查服务与 Provider 能力。"), failures);
  }

  initialConnectionDraft = null;
  runSettingsApplyStep("重置连接配置表单失败：", () => {
    $("#market-base-input").value = state.settings.marketBase;
    $("#analysis-base-input").value = state.settings.analysisBase;
    $("#market-key-input").value = "";
    $("#market-key-input").placeholder = state.settings.apiKey ? "已配置 Key；留空保留当前值" : "填写 MARKET_DATA_API_KEY";
    setMarketKeyVisibility(false);
    $("#settings-feedback").textContent = state.settings.apiKey
      ? "连接配置已保存。已保存的 Key 不会回显；留空会保留当前值。"
      : "连接配置已保存到当前浏览器。";
    initialConnectionDraft = readConnectionDraft();
    syncConnectionDraftState();
    $("#settings-draft-connection-check").textContent = "尚未检查未保存配置。";
    $("#settings-draft-connection-check").className = "settings-inline-status";
  }, failures);

  // 网络刷新与当前设置应用解耦，并显式收敛 Promise rejection。
  runSettingsApplyStep("刷新服务健康状态失败：", () => refreshServices(), failures);
  if (state.view === "market") {
    runSettingsApplyStep("刷新市场工作台失败：", () => refreshMarketWorkspace(), failures);
  } else if (state.view === "stocks") {
    runSettingsApplyStep("刷新数据源能力失败：", () => refreshSources(), failures);
    if (state.settings.apiKey) {
      runSettingsApplyStep("重新查询行情失败：", () => runQuoteQuery(), failures);
    }
  }
  reportSettingsApplyFailures(failures);
}


function saveSettingsFromForm() {
  const form = $("#settings-form");
  if (!form.reportValidity()) return;
  if (!connectionDraftIsDirty()) {
    $("#settings-feedback").textContent = "没有待保存的更改。";
    return;
  }
  try {
    const merged = mergeConnectionDraft(readConnectionDraft(), state.settings);
    const next = saveSettings(merged);
    applySettings(next);
  } catch (error) {
    $("#settings-feedback").textContent = error.message;
  }
}

function clearLocalSettings() {
  if (!window.confirm("确定清除当前浏览器保存的 API Key 与自定义 API 路径？这不会修改服务器 .env。")) return;
  try {
    const next = clearSettings();
    applySettings(next);
  } catch (error) {
    $("#settings-feedback").textContent = "清除本地设置失败：" + error.message;
  }
}


function setBrowserDataFeedback(message, kind = "info") {
  const output = $("#browser-data-feedback");
  if (!output) return;
  output.textContent = message;
  output.className = "settings-inline-status" + (kind === "error" ? " is-error" : kind === "success" ? " is-success" : "");
}

function buildBrowserDataPayload() {
  // 仅导出明确允许的浏览器偏好与引用；绝不包含 API Key、行情缓存或完整报告。
  return {
    format: BROWSER_DATA_FORMAT,
    version: BROWSER_DATA_VERSION,
    exported_at: new Date().toISOString(),
    data: {
      theme: readTheme(),
      preferences: readPreferences(),
      connection: { marketBase: state.settings.marketBase, analysisBase: state.settings.analysisBase },
      watchlist: (() => {
        const watchlistState = safeReadWatchlistState();
        if (!watchlistState.storageReadable) throw new Error("无法读取浏览器自选股存储，导出已取消。");
        if (!watchlistState.storageValid) throw new Error("本地自选数据格式异常，导出已取消以避免导出为空列表覆盖备份。");
        return watchlistState.items;
      })(),
      recent_tasks: (() => {
        const recentTasksState = safeReadRecentTasksState();
        if (!recentTasksState.storageReadable) throw new Error("无法读取浏览器最近任务存储，导出已取消。");
        if (!recentTasksState.storageValid) throw new Error("本地最近任务数据格式异常，导出已取消以避免遗漏历史记录。");
        return recentTasksState.items;
      })()
    }
  };
}

function downloadBrowserData() {
  try {
    const payload = buildBrowserDataPayload();
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "stock-provider-webui-local-data-" + new Date().toISOString().slice(0, 10) + ".json";
    link.hidden = true;
    document.body.append(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
    setBrowserDataFeedback("本地工作区数据已导出。文件不包含 API Key、行情快照或完整分析报告；任务引用可能包含标的与任务 ID。", "success");
  } catch (error) {
    setBrowserDataFeedback("导出失败：" + (error?.message || "浏览器无法读取本地数据。"), "error");
  }
}

function normalizeImportedPreferences(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("本地偏好字段必须是对象。");
  }
  const allowed = new Set(["watchlistSort", "watchlistSortDirection", "watchlistDisplayFields", "watchlistDisplayOrder", "marketColorScheme", "priceDigits", "percentDigits", "amountDisplayUnit", "dailyBarLimit", "dailyAdjustment"]);
  if (Object.keys(value).some((key) => !allowed.has(key))) {
    throw new Error("本地偏好包含不支持的字段。");
  }
  if (value.watchlistSort !== undefined && !["updated", "symbol", "name"].includes(value.watchlistSort)) {
    throw new Error("自选股默认排序字段无效。");
  }
  if (value.watchlistSortDirection !== undefined && !["asc", "desc"].includes(value.watchlistSortDirection)) {
    throw new Error("自选股排序方向无效。");
  }
  if (value.watchlistDisplayFields !== undefined && (!Array.isArray(value.watchlistDisplayFields)
    || value.watchlistDisplayFields.some((field) => !WATCHLIST_DISPLAY_FIELDS.includes(field)))) {
    throw new Error("自选股展示字段配置无效。");
  }
  if (value.watchlistDisplayOrder !== undefined && (!Array.isArray(value.watchlistDisplayOrder)
    || value.watchlistDisplayOrder.length !== WATCHLIST_DISPLAY_FIELDS.length
    || new Set(value.watchlistDisplayOrder).size !== WATCHLIST_DISPLAY_FIELDS.length
    || value.watchlistDisplayOrder.some((field) => !WATCHLIST_DISPLAY_FIELDS.includes(field)))) {
    throw new Error("自选股字段顺序配置无效。");
  }
  if (value.marketColorScheme !== undefined && !["cn", "reverse"].includes(value.marketColorScheme)) throw new Error("涨跌颜色方案无效。");
  if (value.priceDigits !== undefined && ![2, 3].includes(value.priceDigits)) throw new Error("价格精度无效。");
  if (value.percentDigits !== undefined && ![2, 3].includes(value.percentDigits)) throw new Error("涨跌幅精度无效。");
  if (value.amountDisplayUnit !== undefined && !["compact", "raw"].includes(value.amountDisplayUnit)) throw new Error("成交额单位显示无效。");
  if (value.dailyBarLimit !== undefined && ![60, 120, 250].includes(Number(value.dailyBarLimit))) throw new Error("默认日 K 条数无效。");
  if (value.dailyAdjustment !== undefined && !["forward_adjusted", "backward_adjusted", "none"].includes(value.dailyAdjustment)) throw new Error("默认复权方式无效。");
  return {
    watchlistSort: value.watchlistSort || DEFAULT_PREFERENCES.watchlistSort,
    watchlistSortDirection: value.watchlistSortDirection || DEFAULT_PREFERENCES.watchlistSortDirection,
    watchlistDisplayFields: value.watchlistDisplayFields === undefined
      ? [...DEFAULT_PREFERENCES.watchlistDisplayFields]
      : WATCHLIST_DISPLAY_FIELDS.filter((field) => value.watchlistDisplayFields.includes(field)),
    watchlistDisplayOrder: Array.isArray(value.watchlistDisplayOrder) ? [...value.watchlistDisplayOrder] : [...DEFAULT_PREFERENCES.watchlistDisplayOrder],
    marketColorScheme: value.marketColorScheme || DEFAULT_PREFERENCES.marketColorScheme,
    priceDigits: value.priceDigits || DEFAULT_PREFERENCES.priceDigits,
    percentDigits: value.percentDigits || DEFAULT_PREFERENCES.percentDigits,
    amountDisplayUnit: value.amountDisplayUnit || DEFAULT_PREFERENCES.amountDisplayUnit,
    dailyBarLimit: [60, 120, 250].includes(Number(value.dailyBarLimit)) ? Number(value.dailyBarLimit) : DEFAULT_PREFERENCES.dailyBarLimit,
    dailyAdjustment: value.dailyAdjustment || DEFAULT_PREFERENCES.dailyAdjustment
  };
}

function safeWatchlistGroupStorage() {
  try { return window.localStorage; } catch { return null; }
}

function syncWatchlistGroupControls() {
  const select = $("#watchlist-group-filter");
  if (!select) return;
  const previous = select.value || "all", result = readWatchlistGroups(safeWatchlistGroupStorage());
  select.replaceChildren();
  const all = document.createElement("option"); all.value = "all"; all.textContent = "全部分组"; select.append(all);
  result.groups.forEach((group) => { const option = document.createElement("option"); option.value = group; option.textContent = group; select.append(option); });
  select.value = result.groups.includes(previous) ? previous : "all";
  $("#watchlist-group-remove").disabled = select.value === "all" || select.value === DEFAULT_WATCHLIST_GROUP;
  if (!result.valid) setWatchlistFeedback("自选分组存储不可用或格式异常；分组修改已暂停。", "error");
}

function syncWatchlistPreferenceControls(preferences = readPreferences()) {
  const normalized = preferences;
  const sort = $("#settings-watchlist-sort");
  const direction = $("#settings-watchlist-direction");
  const listSort = $("#watchlist-sort");
  if (sort) sort.value = normalized.watchlistSort;
  if (direction) direction.value = normalized.watchlistSortDirection;
  if (listSort) listSort.value = normalized.watchlistSort;
  document.querySelectorAll("[data-watchlist-display-field]").forEach((input) => {
    input.checked = normalized.watchlistDisplayFields.includes(input.value);
  });
  const orderRoot = $("#watchlist-field-order-list");
  if (orderRoot) {
    (normalized.watchlistDisplayOrder || DEFAULT_PREFERENCES.watchlistDisplayOrder).forEach((field) => {
      const item = orderRoot.querySelector(`[data-watchlist-field-order="${field}"]`);
      if (item) orderRoot.append(item);
    });
    orderRoot.querySelectorAll("[data-watchlist-field-order-move]").forEach((button) => {
      const items = [...orderRoot.children];
      const index = items.indexOf(button.closest("[data-watchlist-field-order]"));
      button.disabled = button.dataset.watchlistFieldOrderMove === "up" ? index <= 0 : index >= items.length - 1;
    });
  }
  const controls = {
    "#settings-market-color-scheme": normalized.marketColorScheme,
    "#settings-price-digits": String(normalized.priceDigits),
    "#settings-percent-digits": String(normalized.percentDigits),
    "#settings-amount-unit": normalized.amountDisplayUnit
  };
  Object.entries(controls).forEach(([selector, value]) => {
    const control = $(selector);
    if (control) control.value = value;
  });
  document.documentElement.dataset.marketColorScheme = normalized.marketColorScheme;
  const barLimit = $("#settings-default-bar-limit");
  const adjustment = $("#settings-default-adjustment");
  if (barLimit) barLimit.value = String(normalized.dailyBarLimit);
  if (adjustment) adjustment.value = normalized.dailyAdjustment;
  const queryLimit = $("#bar-limit");
  const queryAdjustment = $("#adjustment");
  if (queryLimit) queryLimit.value = String(normalized.dailyBarLimit);
  if (queryAdjustment) queryAdjustment.value = normalized.dailyAdjustment;
}

function persistLocalPreferences() {
  try {
    const preferences = savePreferences({
      ...readPreferences(),
      watchlistSort: $("#watchlist-sort")?.value || $("#settings-watchlist-sort")?.value,
      watchlistSortDirection: $("#settings-watchlist-direction")?.value || readPreferences().watchlistSortDirection,
      watchlistDisplayFields: [...document.querySelectorAll("[data-watchlist-display-field]:checked")].map((input) => input.value),
      watchlistDisplayOrder: [...document.querySelectorAll("[data-watchlist-field-order]")].map((item) => item.dataset.watchlistFieldOrder),
      marketColorScheme: $("#settings-market-color-scheme")?.value || readPreferences().marketColorScheme,
      priceDigits: Number($("#settings-price-digits")?.value || readPreferences().priceDigits),
      percentDigits: Number($("#settings-percent-digits")?.value || readPreferences().percentDigits),
      amountDisplayUnit: $("#settings-amount-unit")?.value || readPreferences().amountDisplayUnit,
      dailyBarLimit: Number($("#settings-default-bar-limit")?.value || readPreferences().dailyBarLimit),
      dailyAdjustment: $("#settings-default-adjustment")?.value || readPreferences().dailyAdjustment
    });
    syncWatchlistPreferenceControls(preferences);
    $("#local-preferences-feedback").textContent = "本地展示与查询偏好已保存到当前浏览器。";
    return preferences;
  } catch (error) {
    $("#local-preferences-feedback").textContent = "偏好保存失败：" + (error?.message || "浏览器存储不可用。");
    return readPreferences();
  }
}

function validateBrowserDataPayload(payload) {
  if (!payload || payload.format !== BROWSER_DATA_FORMAT || ![1, 2, BROWSER_DATA_VERSION].includes(payload.version)) {
    throw new Error("文件格式或版本不受支持。请使用本 WebUI 导出的 JSON 文件。");
  }
  const data = payload.data;
  if (!data || typeof data !== "object" || Array.isArray(data)) throw new Error("缺少有效的 data 配置对象。");
  if (!["dark", "light"].includes(data.theme)) throw new Error("主题字段无效。");
  const preferences = payload.version === 1 ? { ...DEFAULT_PREFERENCES } : normalizeImportedPreferences(data.preferences);
  if (!data.connection || typeof data.connection !== "object") throw new Error("缺少连接路径配置。");
  // 仅接受同源绝对路径，避免将导入文件变成任意外部请求目标。
  const marketBase = normalizeBasePath(data.connection.marketBase, "/market-api");
  const analysisBase = normalizeBasePath(data.connection.analysisBase, "/analysis-api");

  if (!Array.isArray(data.watchlist) || data.watchlist.length > MAX_WATCHLIST_ITEMS) {
    throw new Error("自选股必须是数组，且不能超过 " + MAX_WATCHLIST_ITEMS + " 条。");
  }
  const seenSymbols = new Set();
  const symbolPattern = /^\d{6}(?:\.(?:SH|SZ|BJ))?$/;
  const watchlist = data.watchlist.map((record) => {
    if (!record || typeof record !== "object" || typeof record.symbol !== "string") throw new Error("自选股条目格式无效。");
    const symbol = record.symbol.trim().toUpperCase();
    if (!symbolPattern.test(symbol)) throw new Error("自选股代码格式无效：" + symbol);
    const symbolIdentity = comparableMarketSymbol(symbol);
    if (seenSymbols.has(symbolIdentity)) throw new Error("自选股文件包含重复证券：" + symbol + "（纯代码与交易所后缀代码按同一标的处理）");
    seenSymbols.add(symbolIdentity);
    const name = String(record.name ?? "").replace(/[\u0000-\u001f\u007f]/g, "").trim().slice(0, 32).trim() || symbol;
    return { symbol, name, updated_at: typeof record.updated_at === "string" ? record.updated_at.slice(0, 80) : "" };
  });

  if (!Array.isArray(data.recent_tasks) || data.recent_tasks.length > MAX_RECENT_TASKS) {
    throw new Error("最近任务引用必须是数组，且不能超过 " + MAX_RECENT_TASKS + " 条。");
  }
  const seenTasks = new Set();
  const recentTasks = data.recent_tasks.map((record) => {
    if (!record || typeof record !== "object" || typeof record.task_id !== "string" || !record.task_id.trim()) {
      throw new Error("最近任务引用缺少有效 task_id。");
    }
    const taskId = record.task_id.replace(/[\u0000-\u001f\u007f]/g, "").trim().slice(0, 200);
    if (!taskId) throw new Error("最近任务引用中的 task_id 无效。");
    if (record.symbols !== undefined && !Array.isArray(record.symbols)) throw new Error("任务标的必须是数组。");
    const rawSymbols = Array.isArray(record.symbols) ? record.symbols : [];
    if (rawSymbols.length > 100 || rawSymbols.some((item) => typeof item !== "string")) {
      throw new Error("任务标的数量或格式无效。");
    }
    const analysisBase = typeof record.analysis_base === "string" && record.analysis_base.trim()
      ? normalizeBasePath(record.analysis_base, "/analysis-api")
      : "";
    // 同一 task_id 可能由不同分析服务分别生成，身份由 task_id + analysis_base 共同决定。
    const taskIdentity = JSON.stringify([taskId, analysisBase]);
    if (seenTasks.has(taskIdentity)) throw new Error("最近任务引用包含重复的任务 ID 与服务地址：" + taskId);
    seenTasks.add(taskIdentity);
    const safeString = (value, max = 200) => typeof value === "string"
      ? value.replace(/[\u0000-\u001f\u007f]/g, "").slice(0, max)
      : "";
    return {
      task_id: taskId,
      run_id: safeString(record.run_id, 200),
      created_at: safeString(record.created_at, 80),
      symbols: rawSymbols.map((item) => item.trim().slice(0, 40)),
      task_type: safeString(record.task_type, 100) || "post_market_scan",
      status: safeString(record.status, 80),
      analysis_base: analysisBase,
      updated_at: safeString(record.updated_at, 80)
    };
  });
  return { theme: data.theme, preferences, marketBase, analysisBase, watchlist, recentTasks };
}

async function importBrowserDataFile(file) {
  if (!file) return;
  if (file.size > 1024 * 1024) { setBrowserDataFeedback("导入失败：文件超过 1 MiB 上限。", "error"); return; }
  let imported;
  try { imported = validateBrowserDataPayload(JSON.parse(await file.text())); }
  catch (error) { setBrowserDataFeedback("导入失败：" + (error?.message || "文件不是有效的工作区 JSON。"), "error"); return; }
  const currentWatchlist = safeReadWatchlistState();
  const currentRecentTasks = safeReadRecentTasksState();
  const currentWatchlistItems = currentWatchlist.storageReadable && currentWatchlist.storageValid ? currentWatchlist.items : [];
  const currentRecentTaskItems = currentRecentTasks.storageReadable && currentRecentTasks.storageValid ? currentRecentTasks.items : [];
  const watchlistIdentity = (record) => comparableMarketSymbol(record.symbol);
  const existingSymbols = new Set(currentWatchlistItems.map(watchlistIdentity));
  const existingTaskIds = new Set(currentRecentTaskItems.map((record) => record.task_id));
  const watchlistConflicts = imported.watchlist.filter((record) => existingSymbols.has(watchlistIdentity(record))).length;
  const taskConflicts = imported.recentTasks.filter((record) => existingTaskIds.has(record.task_id)).length;
  $("#browser-data-import-summary").textContent = "文件包含自选股 " + imported.watchlist.length + " 条（冲突 " + watchlistConflicts + " 条），最近任务 " + imported.recentTasks.length + " 条（冲突 " + taskConflicts + " 条）。";
  $("#browser-data-import-settings").textContent = "主题、本地偏好和 API 路径将采用文件中的设置；当前行情 API Key 保留。";
  $("#browser-data-import-current").textContent = "当前浏览器：" + (currentWatchlist.storageReadable && currentWatchlist.storageValid ? currentWatchlistItems.length : "不可读取") + " 条自选股，" + (currentRecentTasks.storageReadable && currentRecentTasks.storageValid ? currentRecentTaskItems.length : "不可读取") + " 条最近任务。";
  const mergeDisabled = !currentWatchlist.storageReadable || !currentWatchlist.storageValid || !currentRecentTasks.storageReadable || !currentRecentTasks.storageValid;
  $("#browser-data-import-merge").disabled = mergeDisabled;
  $("#browser-data-import-merge-hint").textContent = mergeDisabled ? "当前本地清单不可读取或格式异常，禁止合并以避免覆盖未知数据。" : "按股票代码和任务 ID 去重，导入文件中的同标识记录覆盖当前记录。";
  pendingBrowserImport = { imported, currentWatchlist, currentRecentTasks };
  if (!$("#browser-data-import-dialog").open) $("#browser-data-import-dialog").showModal();
}

async function applyBrowserDataImport(imported, currentWatchlist, currentRecentTasks, strategy) {
  const currentWatchlistItems = currentWatchlist.storageReadable && currentWatchlist.storageValid ? currentWatchlist.items : [];
  const currentRecentTaskItems = currentRecentTasks.storageReadable && currentRecentTasks.storageValid ? currentRecentTasks.items : [];
  const watchlistIdentity = (record) => comparableMarketSymbol(record.symbol);
  if (strategy === "merge") {
    if (!currentWatchlist.storageReadable || !currentWatchlist.storageValid
      || !currentRecentTasks.storageReadable || !currentRecentTasks.storageValid) {
      setBrowserDataFeedback("合并失败：当前自选股或最近任务存储不可读取/格式异常。为避免覆盖未知数据，请先检查本地存储。", "error");
      return;
    }
    const mergedWatchlist = new Map(currentWatchlistItems.map((record) => [watchlistIdentity(record), record]));
    imported.watchlist.forEach((record) => mergedWatchlist.set(watchlistIdentity(record), record));
    const mergedRecentTasks = new Map(currentRecentTaskItems.map((record) => [record.task_id, record]));
    imported.recentTasks.forEach((record) => mergedRecentTasks.set(record.task_id, record));
    if (mergedWatchlist.size > MAX_WATCHLIST_ITEMS || mergedRecentTasks.size > MAX_RECENT_TASKS) {
      setBrowserDataFeedback("合并失败：合并后自选股 " + mergedWatchlist.size + " 条、最近任务 " + mergedRecentTasks.size + " 条，超过本地上限（" + MAX_WATCHLIST_ITEMS + " / " + MAX_RECENT_TASKS + "）。未写入任何数据。", "error");
      return;
    }
    imported.watchlist = [...mergedWatchlist.values()];
    imported.recentTasks = [...mergedRecentTasks.values()];
  }

  const storageKeys = [SETTINGS_KEY, THEME_STORAGE_KEY, PREFERENCES_KEY, WATCHLIST_KEY, RECENT_TASKS_KEY, TASK_REFERENCE_KEY];
  const previous = new Map();
  try {
    storageKeys.forEach((key) => previous.set(key, localStorage.getItem(key)));
  } catch (error) {
    setBrowserDataFeedback("导入失败：无法读取当前浏览器配置，尚未执行写入。" + (error?.message || ""), "error");
    return;
  }
  // 导入完成后允许当前任务继续在页面中跟踪，但禁止它把引用重新写回被替换的本地历史。
  const importedActiveTaskId = state.taskId;
  const importedActiveTaskBase = state.taskAnalysisBase || state.settings.analysisBase;
  let nextSettings;
  try {
    nextSettings = saveSettings({
      marketBase: imported.marketBase,
      analysisBase: imported.analysisBase,
      apiKey: state.settings.apiKey
    });
    localStorage.setItem(THEME_STORAGE_KEY, imported.theme);
    localStorage.setItem(PREFERENCES_KEY, JSON.stringify(imported.preferences));
    localStorage.setItem(WATCHLIST_KEY, JSON.stringify(imported.watchlist));
    localStorage.setItem(RECENT_TASKS_KEY, JSON.stringify(imported.recentTasks));
    localStorage.removeItem(TASK_REFERENCE_KEY);

    // 浏览器存储适配层可能静默忽略写入；应用内存状态切换前必须逐项回读确认。
    const expectedStorage = new Map([
      [SETTINGS_KEY, JSON.stringify(nextSettings)],
      [THEME_STORAGE_KEY, imported.theme],
      [PREFERENCES_KEY, JSON.stringify(imported.preferences)],
      [WATCHLIST_KEY, JSON.stringify(imported.watchlist)],
      [RECENT_TASKS_KEY, JSON.stringify(imported.recentTasks)],
      [TASK_REFERENCE_KEY, null]
    ]);
    for (const [key, expectedValue] of expectedStorage) {
      if (localStorage.getItem(key) !== expectedValue) {
        throw new Error("浏览器存储未能确认写入：" + key);
      }
    }
  } catch (error) {
    const rollbackFailures = [];
    storageKeys.forEach((key) => {
      try {
        const value = previous.get(key);
        if (value === null || value === undefined) localStorage.removeItem(key);
        else localStorage.setItem(key, value);
        if (localStorage.getItem(key) !== (value ?? null)) {
          rollbackFailures.push(key);
        }
      } catch {
        rollbackFailures.push(key);
      }
    });
    const rollbackMessage = rollbackFailures.length
      ? "；且无法确认以下数据已恢复：" + [...new Set(rollbackFailures)].join("、") + "。请重新载入页面并检查本地数据。"
      : "；已回读确认导入前的本地存储已恢复。";
    setBrowserDataFeedback("导入失败" + rollbackMessage + " 原因：" + (error?.message || "本地存储不可用。"), "error");
    return;
  }

  // 存储事务已提交。界面刷新异常不能再回滚持久化数据，以免内存与已导入数据进一步分叉。
  try {
    applySettings(nextSettings, { persistPreviousActiveTask: false });
    // 将导入视为历史替换：在途的创建请求不能接管当前任务，也不能把新记录写回导入的列表。
    state.taskHistoryGeneration += 1;
    state.taskContextGeneration += 1;
    state.suppressedHistoryTaskId = importedActiveTaskId;
    state.suppressedHistoryTaskBase = importedActiveTaskId ? importedActiveTaskBase : "";
    applyTheme(imported.theme);
    syncThemeControl(imported.theme);
    syncSettingsThemeButtons(imported.theme);
    syncWatchlistPreferenceControls(imported.preferences);
    renderWatchlist();
    if (state.quote) renderQuote(state.quote);
    if (state.view === "research") renderResearchOverview();
    renderRecentTasks();
    setBrowserDataFeedback("导入完成：主题、本地偏好、同源 API 路径、自选股与本地任务引用已更新。原有 API Key 已保留。", "success");
  } catch (error) {
    setBrowserDataFeedback(
      "本地数据已导入，但页面状态刷新未完成；请重新载入页面。原因：" + (error?.message || "界面更新异常。"),
      "error"
    );
  }

}

syncThemeControl(readTheme());
syncWatchlistPreferenceControls();
$("#theme-toggle").addEventListener("click", () => {
  const nextTheme = readTheme() === "dark" ? "light" : "dark";
  const result = persistTheme(nextTheme);
  applyTheme(result.theme);
  syncThemeControl(result.theme, !result.persisted);
});
$("#mobile-menu-toggle").addEventListener("click", () => {
  setMobileNavigationOpen(!$(".sidebar").classList.contains("is-open"));
});
$("#mobile-nav-overlay").addEventListener("click", () => setMobileNavigationOpen(false, { restoreFocus: true }));
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    const openDialog = document.querySelector("dialog[open]");
    if (openDialog) {
      // 其他模态对话框由浏览器自身处理 Escape。
      return;
    }
    setMobileNavigationOpen(false, { restoreFocus: true });
    return;
  }
  if (event.key !== "Tab") return;

  // 模态对话框必须先处理焦点循环，不能让焦点落到遮罩后的页面。
  const dialog = document.querySelector("dialog[open]");
  if (dialog) {
    const focusable = [...dialog.querySelectorAll(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )].filter((element) => !element.hidden &&
      element.getAttribute("aria-hidden") !== "true" &&
      element.getClientRects().length > 0);
    if (!focusable.length) {
      event.preventDefault();
      return;
    }
    const firstFocusable = focusable[0];
    const lastFocusable = focusable[focusable.length - 1];
    if (event.shiftKey && (document.activeElement === firstFocusable || !dialog.contains(document.activeElement))) {
      event.preventDefault();
      lastFocusable.focus();
    } else if (!event.shiftKey && (document.activeElement === lastFocusable || !dialog.contains(document.activeElement))) {
      event.preventDefault();
      firstFocusable.focus();
    }
    return;
  }

  if (window.innerWidth > 600) return;
  const sidebar = $(".sidebar");
  if (!sidebar.classList.contains("is-open")) return;
  const focusable = [...sidebar.querySelectorAll(
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
  )].filter((element) => !element.hidden && element.getAttribute("aria-hidden") !== "true" && element.getClientRects().length > 0);
  if (!focusable.length) {
    event.preventDefault();
    return;
  }
  const firstFocusable = focusable[0];
  const lastFocusable = focusable[focusable.length - 1];
  if (event.shiftKey && (document.activeElement === firstFocusable || !sidebar.contains(document.activeElement))) {
    event.preventDefault();
    lastFocusable.focus();
  } else if (!event.shiftKey && (document.activeElement === lastFocusable || !sidebar.contains(document.activeElement))) {
    event.preventDefault();
    firstFocusable.focus();
  }
});
window.addEventListener("resize", () => {
  if (window.innerWidth > 600) setMobileNavigationOpen(false);
});
window.addEventListener("hashchange", () => {
  setView(window.location.hash.slice(1), { updateHash: false });
});

$("#topbar-notifications").addEventListener("click", () => setView("notifications"));
$("#research-stock-list-body").addEventListener("click", (event) => {
  const button = event.target.closest("button[data-research-open-symbol]");
  if (!button) return;
  openResearchStock(button.dataset.researchOpenSymbol || "", button.dataset.researchOpenName || "");
});
$("#chat-clear-context").addEventListener("click", () => {
  state.chatStockContextCleared = true;
  state.chatStockContext = null;
  renderChatStockContext();
});
$("#close-run-flow-dialog").addEventListener("click", () => $("#run-flow-dialog").close());
$("#run-flow-dialog").addEventListener("click", (event) => {
  if (event.target === event.currentTarget) event.currentTarget.close();
});
$("#research-open-chart").addEventListener("click", openResearchChartView);
$("#research-add-watchlist").addEventListener("click", () => {
  addCurrentSymbolToWatchlist();
  const feedback = $("#watchlist-feedback");
  if (feedback?.textContent) setText("#research-overview-note", feedback.textContent);
});
$("#research-start-analysis").addEventListener("click", () => startAnalysisForSymbol($("#symbol-input").value));
$("#settings-check-connection").addEventListener("click", checkConnectionFromSettings);
$("#settings-check-draft-connection").addEventListener("click", checkDraftConnectionFromSettings);
$("#settings-view-data-sources").addEventListener("click", () => {
  setView("data");
});
$("#export-browser-data").addEventListener("click", downloadBrowserData);
$("#import-browser-data").addEventListener("click", () => $("#browser-data-file").click());
$("#browser-data-import-dialog").addEventListener("close", () => {
  if (pendingBrowserImport) { pendingBrowserImport = null; setBrowserDataFeedback("已取消导入，现有本地数据未更改。"); }
});
$("#browser-data-import-replace").addEventListener("click", async () => {
  if (!pendingBrowserImport) return;
  const pending = pendingBrowserImport; pendingBrowserImport = null; $("#browser-data-import-dialog").close();
  try { await applyBrowserDataImport(pending.imported, pending.currentWatchlist, pending.currentRecentTasks, "replace"); }
  catch (error) { setBrowserDataFeedback("导入失败：" + (error?.message || "本地数据写入异常。"), "error"); }
});
$("#browser-data-import-merge").addEventListener("click", async () => {
  if (!pendingBrowserImport) return;
  const pending = pendingBrowserImport; pendingBrowserImport = null; $("#browser-data-import-dialog").close();
  try { await applyBrowserDataImport(pending.imported, pending.currentWatchlist, pending.currentRecentTasks, "merge"); }
  catch (error) { setBrowserDataFeedback("合并失败：" + (error?.message || "本地数据写入异常。"), "error"); }
});
document.querySelectorAll("[data-watchlist-field-order-move]").forEach((button) => {
  button.addEventListener("click", () => {
    const item = button.closest("[data-watchlist-field-order]");
    const list = $("#watchlist-field-order-list");
    const sibling = button.dataset.watchlistFieldOrderMove === "up" ? item.previousElementSibling : item.nextElementSibling;
    if (!item || !sibling) return;
    if (button.dataset.watchlistFieldOrderMove === "up") list.insertBefore(item, sibling); else list.insertBefore(sibling, item);
    const preferences = persistLocalPreferences(); syncWatchlistPreferenceControls(preferences); renderWatchlist();
  });
});
$("#browser-data-file").addEventListener("change", async (event) => {
  const input = event.currentTarget;
  try {
    await importBrowserDataFile(input.files?.[0]);
  } catch (error) {
    setBrowserDataFeedback("导入失败：" + (error?.message || "无法读取文件。"), "error");
  } finally {
    input.value = "";
  }
});
bindSectionNavigation("[data-settings-target]", "settingsTarget");
setupSettingsNavigation();
bindSectionNavigation("[data-research-detail-target]", "researchDetailTarget");
document.querySelectorAll("[data-theme-value]").forEach((button) => {
  button.addEventListener("click", () => {
    const result = persistTheme(button.dataset.themeValue);
    applyTheme(result.theme);
    syncThemeControl(result.theme, !result.persisted);
    syncSettingsThemeButtons(result.theme);
    $("#settings-feedback").textContent = result.persisted
      ? "主题偏好已保存。"
      : "主题已在当前页面切换，但未能确认保存；重新载入后可能恢复旧主题。";
  });
});
document.querySelectorAll(".nav-link, .workspace-tab").forEach((button) => {
  button.addEventListener("click", () => {
    setView(button.dataset.view);
    setMobileNavigationOpen(false, { restoreFocus: true });
  });
});
document.querySelectorAll("[data-open-view]").forEach((button) => {
  button.addEventListener("click", () => {
    setView(button.dataset.openView);
    setMobileNavigationOpen(false);
  });
});
document.querySelectorAll("[data-task-event-filter]").forEach((button) => {
  button.addEventListener("click", () => {
    state.taskEventFilter = button.dataset.taskEventFilter || "all";
    renderTaskEvents();
  });
});
$("#task-events").addEventListener("click", (event) => {
  const row = event.target.closest("button[data-task-event-sequence]");
  if (!row) return;
  state.selectedTaskEventSequence = Number(row.dataset.taskEventSequence);
  state.expandedTaskEventSequences.add(state.selectedTaskEventSequence);
  renderTaskEvents();
});
$("#quote-form").addEventListener("submit", handleQuoteFormSubmit);
$("#symbol-input").addEventListener("compositionstart", () => {
  state.stockSearchComposing = true;
  cancelStockSearchRequest();
});
$("#symbol-input").addEventListener("compositionend", () => {
  state.stockSearchComposing = false;
  scheduleStockSearch();
});
$("#symbol-input").addEventListener("input", () => {
  handleSymbolInputChange();
  const current = $("#symbol-input").value.trim();
  if (state.selectedStockSearchItem?.symbol !== current) state.selectedStockSearchItem = null;
  if (!state.stockSearchComposing) scheduleStockSearch();
});
["bar-start-date", "bar-end-date", "bar-limit", "adjustment"].forEach((id) => {
  $("#" + id).addEventListener("change", () => {
    handleQuoteCriteriaChange("日期范围、日 K 上限或复权口径已变化，旧行情与 K 线已清除，请重新查询。");
  });
});
$("#symbol-input").addEventListener("keydown", handleStockSearchKeydown);
$("#stock-search-results").addEventListener("mousedown", (event) => {
  if (event.target.closest("[data-stock-search-index]")) event.preventDefault();
});
$("#stock-search-results").addEventListener("click", (event) => {
  const option = event.target.closest("[data-stock-search-index]");
  if (!option) return;
  const index = Number(option.dataset.stockSearchIndex);
  chooseStockSearchResult(state.stockSearchResults[index]);
});
document.addEventListener("click", (event) => {
  if (event.target.closest(".symbol-field")) return;
  cancelStockSearchRequest();
  hideStockSearch();
});
$("#copy-bundle-snapshot").addEventListener("click", async () => {
  if (!state.bundleSnapshot) return;
  const feedback = $("#bundle-snapshot-feedback");
  const payload = {
    symbol: state.bundleSnapshot.symbol || state.quote?.symbol || "",
    snapshot_id: state.bundleSnapshot.snapshot_id || "",
    analysis_reference_time: state.bundleSnapshot.analysis_reference_time || "",
    fetched_at: state.bundleSnapshot.fetched_at || "",
    quality_status: state.bundleSnapshot.quality_status || "unknown",
    sources: state.bundleSnapshot.sources || {},
    warnings: Array.isArray(state.bundleSnapshot.warnings) ? state.bundleSnapshot.warnings : []
  };
  try {
    await copyText(JSON.stringify(payload, null, 2));
    feedback.textContent = "快照元数据已复制，可附在问题报告中。";
    feedback.hidden = false;
  } catch (error) {
    feedback.textContent = "复制失败：" + (error?.message || "浏览器未允许复制。");
    feedback.hidden = false;
  }
});
$("#chart-view-close").addEventListener("click", () => setChartView("close"));
$("#chart-view-candles").addEventListener("click", () => setChartView("candles"));
document.querySelectorAll("[data-ma-period]").forEach((control)=>{
  control.addEventListener("change",()=>{
    const period=Number(control.dataset.maPeriod);
    if(![5,20,60,120,250].includes(period))return;
    state.movingAverages=control.checked?[...new Set([...state.movingAverages,period])].sort((a,b)=>a-b):state.movingAverages.filter((item)=>item!==period);
    renderChartView();
  });
});
$("#analyze-quote-button").addEventListener("click", startQuoteAnalysis);
$("#add-to-watchlist-button").addEventListener("click", addCurrentSymbolToWatchlist);
$("#watchlist-list").addEventListener("click", handleWatchlistAction);
$("#watchlist-list").addEventListener("change", (event) => {
  const select = event.target.closest("select[data-watchlist-group-symbol]");
  if (!select) return;
  const result = assignWatchlistGroup(select.dataset.watchlistGroupSymbol, select.value, safeWatchlistGroupStorage());
  if (result.persisted) { setWatchlistFeedback("已调整 " + select.dataset.watchlistGroupSymbol + " 的分组。"); renderWatchlist(); }
  else setWatchlistFeedback("分组调整未能保存；请检查浏览器存储状态。", "error");
});
$("#watchlist-group-filter").addEventListener("change", () => renderWatchlist());
$("#watchlist-group-create").addEventListener("click", () => {
  const input = $("#watchlist-group-name"), result = createWatchlistGroup(input.value, safeWatchlistGroupStorage());
  if (!result.changed || !result.persisted) {
    const messages = { "invalid-storage": "分组存储异常，未修改。", "invalid-name": "请输入有效分组名称（最多 20 字）。", duplicate: "分组名称已存在。", limit: "最多创建 12 个分组。" };
    setWatchlistFeedback(messages[result.reason] || "分组未能保存。", "error"); return;
  }
  input.value = ""; syncWatchlistGroupControls(); setWatchlistFeedback("已创建分组。");
});
$("#watchlist-group-remove").addEventListener("click", () => {
  const name = $("#watchlist-group-filter").value;
  if (name === "all" || name === DEFAULT_WATCHLIST_GROUP) return;
  const result = removeWatchlistGroup(name, safeWatchlistGroupStorage());
  if (!result.changed || !result.persisted) { setWatchlistFeedback("分组删除失败；请检查浏览器存储状态。", "error"); return; }
  $("#watchlist-group-filter").value = "all"; syncWatchlistGroupControls(); renderWatchlist();
  setWatchlistFeedback("已删除分组，组内股票已归入“未分组”。");
});
$("#watchlist-import-preview-button").addEventListener("click", previewWatchlistImport);
$("#watchlist-import-confirm").addEventListener("click", confirmWatchlistImport);
$("#refresh-watchlist-quotes").addEventListener("click", () => { void refreshWatchlistQuotes(); });
$("#watchlist-search").addEventListener("input", () => renderWatchlist());
$("#watchlist-sort").addEventListener("change", () => {
  persistLocalPreferences();
  renderWatchlist();
});
$("#settings-watchlist-sort").addEventListener("change", () => {
  $("#watchlist-sort").value = $("#settings-watchlist-sort").value;
  persistLocalPreferences();
  renderWatchlist();
});
$("#settings-watchlist-direction").addEventListener("change", () => {
  persistLocalPreferences();
  renderWatchlist();
});
document.querySelectorAll("[data-watchlist-display-field]").forEach((input) => {
  input.addEventListener("change", () => {
    persistLocalPreferences();
    renderWatchlist();
  });
});
["#settings-market-color-scheme", "#settings-price-digits", "#settings-percent-digits", "#settings-amount-unit", "#settings-default-bar-limit", "#settings-default-adjustment"].forEach((selector) => {
  $(selector).addEventListener("change", () => {
    persistLocalPreferences();
    renderWatchlist();
    if (state.view === "stocks" && state.quote) renderQuote(state.quote);
    if (state.view === "research") renderResearchOverview();
  });
});
$("#reset-local-preferences").addEventListener("click", () => {
  try {
    const preferences = resetPreferences();
    syncWatchlistPreferenceControls(preferences);
    renderWatchlist();
    if (state.quote) renderQuote(state.quote);
    if (state.view === "research") renderResearchOverview();
    $("#local-preferences-feedback").textContent = "本地展示与查询偏好已恢复默认值；主题、连接密钥、自选股和任务引用均未修改。";
  } catch (error) {
    $("#local-preferences-feedback").textContent = "恢复默认值失败：" + (error?.message || "浏览器存储不可用。");
  }
});
$("#reload-sources").addEventListener("click", refreshSources);
syncRefreshButtonContext();
$("#refresh-button").addEventListener("click", async () => {
  let button = null;
  let ownsBusyState = false;
  const failures = [];
  const appendFailure = (label, error) => {
    let message = "未知异常";
    try { message = errorMessage(error); } catch {}
    failures.push(label + message);
  };

  try {
    button = $("#refresh-button");
    if (button?.disabled) return;
    ownsBusyState = true;

    try {
      if (button) {
        button.disabled = true;
        button.setAttribute("aria-busy", "true");
        button.title = "正在刷新当前页面…";
      }
    } catch (error) {
      appendFailure("刷新按钮状态更新失败：", error);
    }

    // 当前页面数据刷新与按钮外观相互隔离，单个视图异常不会跳过 finally。
    try {
      await refreshServices();
      if (state.view === "market") await refreshMarketWorkspace();
      else if (state.view === "stocks") {
        await refreshSources();
        if (state.settings.apiKey) await runQuoteQuery();
      } else if (state.view === "watchlist") renderWatchlist();
      else if (state.view === "research") {
        await stockResearch.refresh();
        renderResearchStockList();
      } else if (state.view === "data") {
        await dataCenter.refresh();
      } else if (state.view === "tasks" && state.taskId) {
        stopTaskPolling();
        const generation = state.pollGeneration;
        await pollTaskSafely(generation);
      }
    } catch (error) {
      appendFailure("当前页面刷新失败：", error);
    }
  } catch (error) {
    appendFailure("启动当前页面刷新失败：", error);
  } finally {
    if (ownsBusyState) {
      try {
        if (!button) button = $("#refresh-button");
        if (button) {
          button.disabled = false;
          button.removeAttribute("aria-busy");
          syncRefreshButtonContext();
          if (failures.length) {
            button.title = "刷新未完全成功：" + failures.join("；");
          }
        }
      } catch (error) {
        appendFailure("恢复刷新按钮失败：", error);
        try {
          if (button) button.title = "刷新收尾异常：" + failures.join("；");
        } catch {}
      }
    }
  }
});
$("#open-settings").addEventListener("click", () => setView("settings"));
$("#save-settings").addEventListener("click", saveSettingsFromForm);
$("#settings-form").addEventListener("input", syncConnectionDraftState);
$("#toggle-market-key-visibility").addEventListener("click", () => {
  setMarketKeyVisibility($("#market-key-input").type === "password");
});
$("#clear-settings").addEventListener("click", clearLocalSettings);
$("#task-form").addEventListener("submit", createTask);
$("#load-local-watchlist").addEventListener("click", fillTaskSymbolsFromWatchlist);
$("#task-symbols").addEventListener("input", () => {
  $("#task-symbol-help").textContent = "任务只分析输入框中的代码；载入本地自选股会替换当前输入内容。";
});
$("#refresh-task").addEventListener("click", () => {
  if (!state.taskId || $("#refresh-task").disabled) return;
  state.taskEventsUnavailable = false;
  state.taskStatusLoading = true;
  stopTaskPolling();
  const generation = state.pollGeneration;
  try {
    setTaskRefreshBusy(true);
    setQualityPill($("#task-status-pill"), "unknown", "正在刷新");
    const pollLabel = $("#task-poll-label");
    if (pollLabel) pollLabel.textContent = "正在手动刷新任务状态与阶段事件…";
    if (state.view === "watchlist") renderWatchlist();
  } catch (setupError) {
    let message = "未知异常";
    try {
      message = errorMessage(setupError);
    } catch {
      // 错误对象不可格式化时，仍继续发起服务端状态读取。
    }
    try {
      state.taskStatusError = "刷新前置界面更新失败：" + message;
    } catch {
      // 前置诊断写入失败不能阻止任务状态请求。
    }
    try {
      const pollLabel = $("#task-poll-label");
      if (pollLabel) {
        pollLabel.textContent =
          "刷新前置界面更新失败：" + message + "；仍将继续读取服务端状态。";
      }
    } catch {
      // 诊断节点不可更新时仍继续请求；不能让按钮永久停留在忙碌状态。
    }
  }
  void pollTaskSafely(generation).finally(() => {
    if (generation === state.pollGeneration) {
      try {
        setTaskRefreshBusy(false);
      } catch {
        // finally 的按钮解锁不得再次形成未处理的 Promise rejection。
      }
    }
  });
});
$("#recent-analysis-search").addEventListener("input", () => renderRecentTasks());
$("#recent-analysis-status").addEventListener("change", () => renderRecentTasks());
$("#recent-analysis-type").addEventListener("change", () => renderRecentTasks());
$("#clear-recent-analysis").addEventListener("click", clearRecentTaskHistory);
$("#recent-analysis-list").addEventListener("click", (event) => {
  const removeButton = event.target.closest("button[data-remove-task-id]");
  if (removeButton) {
    removeRecentTaskReference(removeButton.dataset.removeTaskId, removeButton.dataset.removeTaskBase || "");
    return;
  }
  const openButton = event.target.closest("button[data-open-task-id]");
  if (!openButton) return;
  const selectedBase = openButton.dataset.openTaskBase || state.settings.analysisBase;
  const record = safeReadRecentTasks().find((item) =>
    item.task_id === openButton.dataset.openTaskId &&
    String(item.analysis_base || state.settings.analysisBase) === selectedBase
  );
  if (record) activateTaskReference(record);
});
$("#reload-run-result").addEventListener("click", () => {
  if (state.taskId && state.runId) void loadTaskRunResult(state.runId, state.taskId);
});
$("#copy-run-json").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  if (!state.lastRunResult) return;
  try {
    await copyText(JSON.stringify(state.lastRunResult, null, 2));
    button.textContent = "已复制";
  } catch (error) {
    $("#task-result-note").textContent = error.message;
    return;
  }
  window.setTimeout(() => { button.textContent = "复制 JSON"; }, 1500);
});
$("#copy-task-diagnostic").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  const result = state.lastRunResult || {};
  const diagnostic = {
    captured_at_client: new Date().toISOString(),
    task_id: state.taskId || null,
    task_status: state.taskStatus || null,
    task_error: $("#task-error").textContent.trim() || null,
    task_poll_status: $("#task-poll-label").textContent || null,
    run_id: state.runId || null,
    run_status: result.status || null,
    run_result_read_status: $("#task-result-note").textContent || null,
    symbols: state.taskSymbols,
    task_type: state.taskType,
    as_of: result.request?.as_of || null,
    quality: result.quality || null,
    failure: result.failure || null,
    warnings: result.warnings || [],
    event_read_status: {
      loaded: state.taskEventsLoaded,
      unavailable: state.taskEventsUnavailable,
      last_error: state.taskEventsError || null,
      known_event_count: state.taskEvents.size,
      after_sequence: state.taskSequence
    },
    selected_event_sequence: state.selectedTaskEventSequence,
    events: [...state.taskEvents.values()].sort((a, b) => a.sequence - b.sequence)
  };
  try {
    await copyText(JSON.stringify(diagnostic, null, 2));
    button.textContent = "已复制";
  } catch (error) {
    $("#task-result-note").textContent = error.message;
    return;
  }
  window.setTimeout(() => { button.textContent = "复制诊断"; }, 1500);
});
function downloadTextFile(content, filename, mimeType) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.style.display = "none";
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

$("#download-run-json").addEventListener("click", () => {
  if (!state.lastRunResult) return;
  downloadTextFile(JSON.stringify(state.lastRunResult, null, 2), buildRunResultFilename(state.lastRunResult, "json"), "application/json;charset=utf-8");
});
$("#download-run-markdown").addEventListener("click", () => {
  if (!state.lastRunResult) return;
  downloadTextFile(buildRunResultMarkdown(state.lastRunResult), buildRunResultFilename(state.lastRunResult, "md"), "text/markdown;charset=utf-8");
});
$("#copy-task-id").addEventListener("click", async () => {
  if (!state.taskId) return;
  const button = $("#copy-task-id");
  try {
    await copyText(state.taskId);
    button.textContent = "已复制";
  } catch {
    button.textContent = "复制失败";
  }
  window.setTimeout(() => { button.textContent = "复制"; }, 1600);
});

renderNotificationState({ supported: false });
renderChatState({ supported: false });
renderChatStockContext();
renderScreeningCapability({ supported: false });
renderScreeningTask({ supported: false });
renderScreeningCandidates({ supported: false });
renderScreeningHistory({ supported: false });
renderAlertCapability({ supported: false });
renderAlertCollection({ supported: false }, "rules", "[data-alert-rules-state]", "#alert-rules-status-pill", "#alert-summary-rule-count", "rules");
renderAlertEvaluation({ supported: false });
renderAlertCollection({ supported: false }, "items", "[data-alert-history-state]", "#alert-history-status-pill", "#alert-summary-trigger-count", "history");
renderAlertCollection({ supported: false }, "attempts", "[data-alert-delivery-state]", "#alert-delivery-status-pill", "#alert-summary-delivery-count", "delivery");
renderRecentTasks();
renderWatchlist();
configureDateInputs();
// 每次重新进入应用都从首页开始；任务引用仍保留，但不强制跳转到运行流。
window.history.replaceState(null, "", window.location.pathname + window.location.search);
setView("market", { updateHash: false, refreshData: false });
syncSettingsThemeButtons(readTheme());
void refreshServices();
void refreshMarketWorkspace();

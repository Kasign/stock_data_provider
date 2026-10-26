import { applyTheme, persistTheme, readTheme } from "./theme.js";
import { connectionDraftChanged, mergeConnectionDraft } from "./settings-draft.js";
import { resolveNotificationViewState } from "./notification-state.js";
import { resolveChatViewState } from "./chat-state.js";
import {
  analysisHealth,
  clearSettings,
  createAnalysisTask,
  getAnalysisRun,
  getAnalysisTask,
  getAnalysisTaskEvents,
  getDailyBars,
  getChipDistribution,
  getCapitalFlow,
  getDataSources,
  getQuote,
  loadSettings,
  marketHealth,
  saveSettings
} from "./api.js";
import { normalizeDailyBars, renderCandlestickVolumeChart, renderCloseChart } from "./chart.js";
import { clearRecentTasks, readRecentTasks, removeRecentTask, upsertRecentTask } from "./recent-tasks.js";
import { addWatchlistSymbol, MAX_WATCHLIST_ITEMS, readWatchlist, removeWatchlistSymbol, renameWatchlistSymbol } from "./watchlist.js";
import { buildRunResultFilename, buildRunResultMarkdown } from "./report.js";
import {
  describeApiError,
  formatCompact,
  formatDate,
  formatDateTime,
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
const TERMINAL_TASK_STATUSES = new Set(["succeeded", "partial", "failed"]);
const RUN_RESULT_RETRY_DELAYS_MS = [1000, 2500, 5000];
let initialConnectionDraft = null;

const state = {
  settings: loadSettings(),
  view: "market",
  quote: null,
  bars: [],
  chartView: "close",
  chartMeta: { adjustmentBasis: "unknown", volumeUnit: "", amountUnit: "", qualityStatus: "unknown" },
  taskId: "",
  runId: "",
  taskSymbols: [],
  taskType: "post_market_scan",
  taskStatus: "",
  isCreatingTask: false,
  lastRunResult: null,
  runResultGeneration: 0,
  taskSequence: 0,
  taskEvents: new Map(),
  pollTimer: null,
  pollGeneration: 0,
  isQuerying: false,
  sourceCount: 0,
  supplementGeneration: 0,
  supplementSymbol: "",
  supplementLoading: false,
  suppressedHistoryTaskId: "",
  chatStockContext: null
};

function setText(selector, value) {
  const target = $(selector);
  if (target) target.textContent = value == null || value === "" ? "—" : String(value);
}


function syncThemeControl(theme) {
  const current = theme === "light" ? "light" : "dark";
  const target = current === "dark" ? "light" : "dark";
  const targetLabel = target === "light" ? "浅色" : "深色";
  $("#theme-toggle-icon").textContent = target === "light" ? "☼" : "☾";
  $("#theme-toggle-label").textContent = targetLabel;
  $("#theme-toggle").setAttribute("aria-label", "切换主题");
  $("#theme-toggle").setAttribute("aria-pressed", String(current === "light"));
  $("#theme-toggle").title = "当前" + (current === "light" ? "浅色" : "深色") + "主题；点击切换至" + targetLabel + "主题";
}

function setMobileNavigationOpen(open) {
  const isOpen = Boolean(open) && window.innerWidth <= 600;
  $(".sidebar").classList.toggle("is-open", isOpen);
  $("#mobile-nav-overlay").hidden = !isOpen;
  document.body.classList.toggle("mobile-nav-open", isOpen);
  $("#mobile-menu-toggle").setAttribute("aria-expanded", String(isOpen));
  $("#mobile-menu-toggle").setAttribute("aria-label", isOpen ? "关闭导航" : "打开导航");
  $("#mobile-menu-toggle").title = isOpen ? "关闭导航" : "打开导航";
}

function persistTaskReference() {
  if (!state.taskId || state.suppressedHistoryTaskId === state.taskId) return;
  const record = {
    task_id: state.taskId,
    run_id: state.runId || "",
    symbols: state.taskSymbols,
    task_type: state.taskType,
    status: state.taskStatus
  };
  const comparable = JSON.stringify(record);
  try {
    const previous = JSON.parse(localStorage.getItem(TASK_REFERENCE_KEY) || "null");
    const previousComparable = previous ? JSON.stringify({
      task_id: previous.task_id,
      run_id: previous.run_id || "",
      symbols: Array.isArray(previous.symbols) ? previous.symbols : [],
      task_type: previous.task_type || "post_market_scan",
      status: previous.status || ""
    }) : "";
    if (previousComparable === comparable) {
      // Migrate the legacy single-task reference into the new local history once.
      const historyItem = safeReadRecentTasks().find((item) => item.task_id === record.task_id);
      const historyComparable = historyItem ? JSON.stringify({
        task_id: historyItem.task_id,
        run_id: historyItem.run_id || "",
        symbols: Array.isArray(historyItem.symbols) ? historyItem.symbols : [],
        task_type: historyItem.task_type || "post_market_scan",
        status: historyItem.status || ""
      }) : "";
      if (historyComparable !== comparable) {
        record.updated_at = previous?.updated_at || new Date().toISOString();
        renderRecentTasks(upsertRecentTask(record, localStorage));
      }
      return;
    }
    record.updated_at = new Date().toISOString();
    localStorage.setItem(TASK_REFERENCE_KEY, JSON.stringify(record));
    renderRecentTasks(upsertRecentTask(record, localStorage));
  } catch {
    // Storage can be disabled or full; the current page session should still work.
  }
}

function renderRecentTasks(records = safeReadRecentTasks()) {
  const root = $("#recent-analysis-list");
  root.replaceChildren();
  const allItems = Array.isArray(records) ? records : [];
  const search = $("#recent-analysis-search").value.trim().toLocaleLowerCase();
  const items = search ? allItems.filter((record) => [
    record.task_id,
    record.run_id,
    ...(record.symbols || []),
    record.task_type,
    record.status,
    taskStatusLabel(record.status),
    record.updated_at
  ].some((value) => String(value || "").toLocaleLowerCase().includes(search))) : allItems;
  setText("#recent-analysis-count", allItems.length + " / 10");

  if (!allItems.length || !items.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = !allItems.length
      ? "完成一次分析后，这里会显示最近任务。"
      : "没有匹配项，请调整关键词或清除搜索。";
    root.append(empty);
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
    const detail = document.createElement("small");
    detail.textContent = (record.updated_at ? formatDateTime(record.updated_at) : "时间未记录") + " · " + record.task_id;
    const status = document.createElement("span");
    status.className = "recent-analysis-item-status";
    status.textContent = taskStatusLabel(record.status);
    main.append(title, detail);
    open.append(main, status);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "recent-analysis-remove";
    remove.dataset.removeTaskId = record.task_id;
    remove.textContent = "移除";
    remove.setAttribute("aria-label", "从本地列表移除任务 " + record.task_id);

    item.append(open, remove);
    root.append(item);
  });
}

function safeReadRecentTasks() {
  try {
    return readRecentTasks(localStorage);
  } catch {
    return [];
  }
}

function setRecentTasksFeedback(message, kind = "info") {
  const target = $("#recent-analysis-feedback");
  target.hidden = !message;
  target.className = "recent-analysis-feedback" + (message ? " is-" + kind : "");
  target.textContent = message;
}

function removeRecentTaskReference(taskId) {
  let remaining = [];
  try {
    remaining = removeRecentTask(taskId, localStorage);
  } catch {
    remaining = safeReadRecentTasks();
  }
  try {
    const last = JSON.parse(localStorage.getItem(TASK_REFERENCE_KEY) || "null");
    if (last?.task_id === taskId) localStorage.removeItem(TASK_REFERENCE_KEY);
  } catch {
    // Local history is optional and must not interrupt task controls.
  }
  if (state.taskId === taskId) state.suppressedHistoryTaskId = taskId;
  renderRecentTasks(remaining);
  setRecentTasksFeedback("已从当前浏览器移除该任务引用；服务端任务和报告不会被删除。");
}

function clearRecentTaskHistory() {
  if (!window.confirm("清空当前浏览器保存的最近任务引用？不会删除服务端任务或报告。")) return;
  const currentTaskId = state.taskId;
  let remaining = [];
  try {
    remaining = clearRecentTasks(localStorage);
    localStorage.removeItem(TASK_REFERENCE_KEY);
  } catch {
    remaining = [];
  }
  state.suppressedHistoryTaskId = currentTaskId;
  renderRecentTasks(remaining);
  setRecentTasksFeedback("本地最近任务列表已清空；当前页面中的任务不会被取消，服务端记录未删除。");
}


function setWatchlistFeedback(message, kind = "info") {
  const target = $("#watchlist-feedback");
  target.hidden = !message;
  target.textContent = message;
  target.className = "watchlist-feedback" + (message ? " watchlist-feedback-" + kind : "");
}

function safeReadWatchlist() {
  try {
    return readWatchlist(localStorage);
  } catch {
    return [];
  }
}

function renderWatchlist(records = safeReadWatchlist()) {
  const root = $("#watchlist-list");
  root.replaceChildren();
  const items = Array.isArray(records) ? records : [];
  setText("#watchlist-count", items.length + " / " + MAX_WATCHLIST_ITEMS);
  if (!items.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "暂无自选股。可先在上方输入代码，再点击“加入自选”。";
    root.append(empty);
    return;
  }

  items.forEach((record) => {
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
        renderWatchlist(renameWatchlistSymbol(record.symbol, aliasInput.value, localStorage));
        setWatchlistFeedback("已保存 " + record.symbol + " 的自定义名称。");
      } catch {
        setWatchlistFeedback("浏览器存储不可用，无法保存自选名称。", "error");
      }
    });

    const meta = document.createElement("small");
    meta.className = "watchlist-item-meta";
    meta.textContent = "添加/更新于 " + (record.updated_at ? formatDateTime(record.updated_at) : "时间未记录");
    aliasLabel.append(aliasCaption, aliasInput);
    main.append(symbol, aliasLabel, meta);

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
    row.append(main, actions);
    root.append(row);
  });
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
  if (state.quote?.name && state.quote?.symbol) {
    try {
      if (normalizeSymbol(state.quote.symbol) === symbol) defaultName = state.quote.name;
    } catch {
      // A malformed server display symbol must not block saving the user-entered symbol.
    }
  }

  try {
    const result = addWatchlistSymbol(symbol, defaultName, localStorage);
    if (!result.valid) {
      setWatchlistFeedback("股票代码格式无效，未加入自选。", "error");
      return;
    }
    renderWatchlist(result.items);
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
      renderWatchlist(removeWatchlistSymbol(symbol, localStorage));
      setWatchlistFeedback("已从本地自选中移除 " + symbol + "。");
    } catch {
      setWatchlistFeedback("浏览器本地存储不可用，无法移除自选股。", "error");
    }
    return;
  }

  if (action === "query") {
    if (state.isQuerying) {
      setWatchlistFeedback("当前行情查询尚未完成，请稍后再选择其他标的。", "error");
      return;
    }
    $("#symbol-input").value = symbol;
    setView("market");
    void runQuoteQuery();
    return;
  }

  if (action === "analyze") startAnalysisForSymbol(symbol);
}

function setNotice(message, kind = "info") {
  const notice = $("#market-notice");
  notice.textContent = message;
  notice.className = `notice notice-${kind}`;
  notice.hidden = !message;
}

function setServiceStatus(prefix, ok, text) {
  const dot = $(`#${prefix}-health-dot`);
  const label = $(`#${prefix}-health-text`);
  dot.classList.toggle("is-online", Boolean(ok));
  dot.classList.toggle("is-offline", !ok);
  label.textContent = text;
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
  state.chatStockContext = state.quote && typeof state.quote.symbol === "string" && state.quote.symbol.trim()
    ? { symbol: state.quote.symbol.trim(), name: String(state.quote.name || state.quote.symbol).trim() }
    : null;
  renderChatStockContext();
}

function errorMessage(error) {
  return describeApiError(error, "请求失败，请检查服务状态与浏览器网络面板。");
}

function getQuerySymbol() {
  return normalizeSymbol($("#symbol-input").value);
}

const VIEW_META = Object.freeze({
  market: { title: "行情总览" },
  tasks: { title: "任务与运行流" },
  notifications: { title: "通知中心" },
  chat: { title: "问股工作台" },
  screening: { title: "条件选股" },
  alerts: { title: "告警中心" },
  settings: { title: "设置中心" }
});

function setView(view, { updateHash = true } = {}) {
  const nextView = Object.hasOwn(VIEW_META, view) ? view : "market";
  state.view = nextView;
  if (nextView === "chat") syncChatStockContextFromMarket();
  const runFlowDialog = $("#run-flow-dialog");
  if (nextView !== "tasks" && runFlowDialog?.open) runFlowDialog.close();
  document.querySelectorAll(".view").forEach((section) => {
    section.hidden = section.id !== nextView + "-view";
  });
  document.querySelectorAll(".nav-link").forEach((button) => {
    const active = button.dataset.view === nextView;
    button.classList.toggle("active", active);
    if (active) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  });
  const title = VIEW_META[nextView].title;
  setText("#breadcrumb-current", title);
  setText("#view-title", title);
  document.title = title + " · Stock Data Console";
  if (updateHash && window.location.hash !== "#" + nextView) window.location.hash = nextView;
}

function syncSettingsThemeButtons(theme = readTheme()) {
  document.querySelectorAll("[data-theme-value]").forEach((button) => {
    const active = button.dataset.themeValue === theme;
    button.setAttribute("aria-pressed", String(active));
    button.classList.toggle("is-selected", active);
  });
}


function clearSupplementReasons(kind, message = "尚未返回质量原因。") {
  const root = $("#"+kind+"-reasons");
  root.replaceChildren();
  const line = document.createElement("p");
  line.textContent = message;
  root.append(line);
}

function setSupplementQuality(kind, status, label = "") {
  const node = $("#"+kind+"-quality");
  const normalized = String(status || "unknown").toLowerCase();
  node.className = "supplement-quality supplement-quality-" + (["valid","degraded","stale","invalid","unknown"].includes(normalized) ? normalized : "unknown");
  node.textContent = label || qualityLabel(normalized);
}

function clearSupplementCard(kind, symbolLabel = "尚未查询") {
  const ids = kind === "chip"
    ? ["chip-average-cost","chip-profit-ratio","chip-concentration-90","chip-concentration-70","chip-date","chip-source","chip-fetched-at"]
    : ["flow-main-inflow","flow-inflow-5d","flow-inflow-10d","flow-observations","flow-source","flow-fetched-at"];
  ids.forEach((id) => setText("#"+id, "—"));
  setText("#"+kind+"-symbol", symbolLabel);
  setSupplementQuality(kind, "unknown", "未加载");
  clearSupplementReasons(kind, "加载后显示后端返回的质量原因。");
  const errorNode = $("#"+kind+"-error");
  errorNode.hidden = true;
  errorNode.textContent = "";
}

function resetSupplementPanel(symbol) {
  state.supplementGeneration += 1;
  state.supplementSymbol = symbol || "";
  state.supplementLoading = false;
  setText("#supplemental-target", symbol ? "当前标的：" + symbol : "当前标的：尚未选择");
  setText("#supplemental-feedback", "");
  $("#supplemental-feedback").hidden = true;
  $("#load-supplemental-button").disabled = !symbol;
  $("#load-supplemental-button").textContent = "加载补充数据";
  clearSupplementCard("chip", symbol || "尚未查询");
  clearSupplementCard("flow", symbol || "尚未查询");
}

function formatRatioPercent(value) {
  if (value === null || value === undefined || value === "") return "—";
  const number = Number(value);
  if (!Number.isFinite(number) || number < 0 || number > 1) return "超出接口约定";
  return formatPercent(number * 100);
}

function formatSignedYuan(value) {
  if (value === null || value === undefined || value === "") return "—";
  const number = Number(value);
  if (!Number.isFinite(number)) return "—";
  const formatted = (number > 0 ? "+" : "") + formatCompact(number) + " 元";
  return formatted;
}

function renderSupplementReasons(kind, reasons) {
  const root = $("#"+kind+"-reasons");
  root.replaceChildren();
  if (!Array.isArray(reasons) || !reasons.length) {
    const line = document.createElement("p");
    line.textContent = "接口未返回质量原因；请结合质量状态与数据日期判断可用性。";
    root.append(line);
    return;
  }
  reasons.forEach((reason) => {
    const line = document.createElement("p");
    line.textContent = String(reason);
    root.append(line);
  });
}

function renderChipDistribution(data, symbol) {
  setText("#chip-symbol", data.symbol || symbol);
  setText("#chip-average-cost", Number(data.average_cost) > 0 ? formatPrice(data.average_cost) : "未报告");
  setText("#chip-profit-ratio", formatRatioPercent(data.profit_ratio));
  setText("#chip-concentration-90", formatRatioPercent(data.concentration_90));
  setText("#chip-concentration-70", formatRatioPercent(data.concentration_70));
  setText("#chip-date", data.date || data.provider_date ? formatDate(data.date || data.provider_date) : "未报告");
  setText("#chip-source", [data.source, data.source_version].filter(Boolean).join(" · ") || "未报告");
  setText("#chip-fetched-at", data.fetched_at ? formatDateTime(data.fetched_at) : "未报告");
  setSupplementQuality("chip", data.quality_status);
  renderSupplementReasons("chip", data.quality_reasons);
  $("#chip-error").hidden = true;
  $("#chip-error").textContent = "";
}

function renderCapitalFlow(data, symbol) {
  setText("#flow-symbol", data.symbol || symbol);
  setText("#flow-main-inflow", formatSignedYuan(data.main_net_inflow_yuan));
  setText("#flow-inflow-5d", formatSignedYuan(data.inflow_5d_yuan));
  setText("#flow-inflow-10d", formatSignedYuan(data.inflow_10d_yuan));
  setText("#flow-observations", Number.isFinite(Number(data.observations)) ? String(data.observations) : "未报告");
  setText("#flow-source", [data.source, data.source_version].filter(Boolean).join(" · ") || "未报告");
  setText("#flow-fetched-at", data.fetched_at ? formatDateTime(data.fetched_at) : "未报告");
  setSupplementQuality("flow", data.quality_status);
  renderSupplementReasons("flow", data.quality_reasons);
  $("#flow-error").hidden = true;
  $("#flow-error").textContent = "";
}

function renderSupplementError(kind, error) {
  const node = $("#"+kind+"-error");
  node.hidden = false;
  node.textContent = errorMessage(error);
  setSupplementQuality(kind, "unknown", "接口不可用");
  clearSupplementReasons(kind, "本次未获取到质量原因。");
}

async function loadSupplementalData() {
  if (!state.settings.apiKey) {
    const feedback = $("#supplemental-feedback");
    feedback.hidden = false;
    feedback.className = "supplemental-feedback is-warning";
    feedback.textContent = "请先配置行情服务 API Key。";
    $("#settings-dialog").showModal();
    return;
  }
  if (state.isQuerying) {
    const feedback = $("#supplemental-feedback");
    feedback.hidden = false;
    feedback.textContent = "主行情查询尚未完成，请稍后加载补充数据。";
    feedback.className = "supplemental-feedback is-warning";
    return;
  }

  let symbol;
  try {
    symbol = getQuerySymbol();
    state.settings.marketBase = normalizeBasePath(state.settings.marketBase, "/market-api");
  } catch (error) {
    const feedback = $("#supplemental-feedback");
    feedback.hidden = false;
    feedback.textContent = error.message;
    feedback.className = "supplemental-feedback is-error";
    return;
  }

  if (state.supplementLoading) return;
  if (state.supplementSymbol !== symbol) resetSupplementPanel(symbol);
  const generation = ++state.supplementGeneration;
  state.supplementSymbol = symbol;
  state.supplementLoading = true;
  $("#load-supplemental-button").disabled = true;
  $("#load-supplemental-button").textContent = "正在加载…";
  setText("#supplemental-target", "当前标的：" + symbol);
  const feedback = $("#supplemental-feedback");
  feedback.hidden = false;
  feedback.className = "supplemental-feedback";
  feedback.textContent = "正在分别读取筹码分布和资金流；任一接口失败不会阻塞另一项结果。";
  clearSupplementCard("chip", symbol);
  clearSupplementCard("flow", symbol);
  setSupplementQuality("chip", "unknown", "读取中…");
  setSupplementQuality("flow", "unknown", "读取中…");

  try {
    const results = await Promise.allSettled([
      getChipDistribution(state.settings, symbol),
      getCapitalFlow(state.settings, symbol)
    ]);
    if (generation !== state.supplementGeneration || symbol !== state.supplementSymbol) return;

    if (results[0].status === "fulfilled") renderChipDistribution(results[0].value, symbol);
    else renderSupplementError("chip", results[0].reason);

    if (results[1].status === "fulfilled") renderCapitalFlow(results[1].value, symbol);
    else renderSupplementError("flow", results[1].reason);

    const failed = results.filter((result) => result.status === "rejected").length;
    feedback.className = "supplemental-feedback" + (failed ? " is-warning" : "");
    feedback.textContent = failed === 0
      ? "补充数据已返回。请分别检查各接口的数据日期、来源和质量状态；数值不代表交易建议。"
      : failed === 1
        ? "一项补充接口不可用，另一项结果已独立显示。请检查对应错误、请求 ID 和后端 Provider 配置。"
        : "两项补充接口均未返回数据。请检查 API Key、服务版本和 Provider 是否启用；行情、日 K 与分析主流程不受此结果覆盖。";
  } finally {
    if (generation === state.supplementGeneration) {
      state.supplementLoading = false;
      $("#load-supplemental-button").disabled = !state.supplementSymbol;
      $("#load-supplemental-button").textContent = "重新加载补充数据";
    }
  }
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
  setText("#metric-change", quote.change == null && quote.change_percent == null ? "暂无涨跌数据" : `${change}（${changePct}）`);
  $("#metric-change").classList.toggle("positive", Number(quote.change) > 0);
  $("#metric-change").classList.toggle("negative", Number(quote.change) < 0);
  setText("#metric-amount", formatCompact(quote.amount));
  setText("#metric-volume", formatCompact(quote.volume));
  setText("#metric-turnover", quote.turnover_rate == null ? "—" : formatPercent(quote.turnover_rate));
  setText("#metric-volume-ratio", `量比 ${quote.volume_ratio == null ? "—" : Number(quote.volume_ratio).toFixed(2)}`);
  setText("#metric-source", quote.source || "未知");
  setText("#metric-quote-time", `行情时间 ${formatDateTime(quote.quote_time)}`);

  const reasons = Array.isArray(quote.quality_reasons) ? quote.quality_reasons : [];
  if (reasons.length) {
    setNotice(`行情质量已降级：${reasons.join("、")}`, "warning");
  }
}

function makeCell(value, className = "") {
  const cell = document.createElement("td");
  if (className) cell.className = className;
  cell.textContent = value == null || value === "" ? "—" : String(value);
  return cell;
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

  const drawn = isCandles
    ? renderCandlestickVolumeChart($("#close-chart"), state.bars, state.chartMeta)
    : renderCloseChart($("#close-chart"), state.bars);
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
      row.append(
        makeCell(formatDate(bar.trade_date), "date-cell"),
        makeCell(formatPrice(bar.open)),
        makeCell(formatPrice(bar.high)),
        makeCell(formatPrice(bar.low)),
        makeCell(formatPrice(bar.close), "close-cell"),
        makeCell(formatPercent(bar.change_percent), Number(bar.change_percent) > 0 ? "positive" : Number(bar.change_percent) < 0 ? "negative" : ""),
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

  sources.forEach((source) => {
    const row = document.createElement("div");
    row.className = "source-row";
    const main = document.createElement("div");
    main.className = "source-main";
    const indicator = document.createElement("span");
    indicator.className = `source-indicator ${source.enabled ? "enabled" : "disabled"}`;
    const name = document.createElement("strong");
    name.textContent = source.name || "unknown";
    const detail = document.createElement("small");
    const capabilities = [];
    if (source.quote_capable) capabilities.push("实时行情");
    if (source.daily_bar_capable) capabilities.push("日 K");
    if (source.chip_capable) capabilities.push("筹码");
    if (source.capital_flow_capable) capabilities.push("资金流");
    detail.textContent = capabilities.length ? capabilities.join(" · ") : "未声明查询能力";
    main.append(indicator, name, detail);
    const badge = document.createElement("span");
    badge.className = `source-state ${source.enabled ? "source-state-on" : "source-state-off"}`;
    badge.textContent = source.enabled ? "已注册" : "已禁用";
    row.append(main, badge);
    list.append(row);
  });
}

async function refreshServices() {
  const checks = await Promise.allSettled([
    marketHealth(state.settings),
    analysisHealth(state.settings)
  ]);
  const marketOK = checks[0].status === "fulfilled";
  const analysisOK = checks[1].status === "fulfilled";
  setServiceStatus("market", marketOK, marketOK ? "就绪" : "不可用");
  setServiceStatus("analysis", analysisOK, analysisOK ? "就绪" : "不可用");
  $("#connection-dot").classList.toggle("is-online", marketOK && analysisOK);
  $("#connection-dot").classList.toggle("is-offline", !marketOK && !analysisOK);
  setText("#connection-label", marketOK && analysisOK ? "服务已连接" : marketOK || analysisOK ? "部分连接" : "连接异常");
  if (!marketOK && checks[0].reason) setText("#query-hint", errorMessage(checks[0].reason));
  return { marketOK, analysisOK };
}

async function checkConnectionFromSettings() {
  const button = $("#settings-check-connection");
  const output = $("#settings-connection-check");
  button.disabled = true;
  button.textContent = "检查中…";
  output.textContent = "正在请求行情与分析服务健康检查…";
  output.className = "settings-inline-status";
  try {
    const result = await refreshServices();
    output.textContent = (result.marketOK ? "行情 API 就绪" : "行情 API 不可用") + "；" +
      (result.analysisOK ? "分析 API 就绪" : "分析 API 不可用") +
      "。健康检查不验证 Provider 的每次真实请求。";
    output.className = "settings-inline-status " + (result.marketOK && result.analysisOK ? "is-success" : "is-warning");
  } catch (error) {
    output.textContent = "连接检查失败：" + errorMessage(error);
    output.className = "settings-inline-status is-warning";
  } finally {
    button.disabled = false;
    button.textContent = "重新检查";
  }
}

async function refreshSources() {
  if (!state.settings.apiKey) {
    const list = $("#data-source-list");
    list.replaceChildren();
    const hint = document.createElement("div");
    hint.className = "empty-row";
    hint.textContent = "尚未配置行情 API Key。密钥保存在当前浏览器本地。";
    list.append(hint);
    setText("#source-count", "—");
    setText("#normalization-version", "归一化版本 —");
    return;
  }
  try {
    const payload = await getDataSources(state.settings);
    renderSources(payload);
  } catch (error) {
    const list = $("#data-source-list");
    list.replaceChildren();
    const hint = document.createElement("div");
    hint.className = "empty-row error-text";
    hint.textContent = errorMessage(error);
    list.append(hint);
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

async function runQuoteQuery(event) {
  event?.preventDefault();
  if (state.isQuerying) return;
  let symbol;
  let startDate;
  let endDate;
  try {
    symbol = getQuerySymbol();
    state.settings.marketBase = normalizeBasePath(state.settings.marketBase, "/market-api");
    startDate = $("#bar-start-date").value;
    endDate = $("#bar-end-date").value;
    const today = shanghaiDateKey();
    if (startDate && endDate && startDate > endDate) throw new Error("开始日期不能晚于结束日期。");
    if (startDate && startDate > today) throw new Error("开始日期不能晚于上海时区的今天。");
    if (endDate && endDate > today) throw new Error("结束日期不能晚于上海时区的今天。");
  } catch (error) {
    setNotice(error.message, "danger");
    return;
  }
  resetSupplementPanel(symbol);
  if (!state.settings.apiKey) {
    setNotice("请先打开“连接与密钥设置”，填写行情服务的 MARKET_DATA_API_KEY。", "warning");
    $("#settings-dialog").showModal();
    return;
  }

  state.isQuerying = true;
  $("#load-supplemental-button").disabled = true;
  $("#quote-form").classList.add("is-loading");
  $("#quote-form button[type='submit']").disabled = true;
  setNotice("", "info");
  setText("#query-hint", `正在查询 ${symbol} 的实时行情与日 K…`);

  const limit = Number($("#bar-limit").value) || 120;
  const adjustment = $("#adjustment").value;
  const [quoteResult, barsResult] = await Promise.allSettled([
    getQuote(state.settings, symbol),
    getDailyBars(state.settings, symbol, { limit, adjustment, start: startDate, end: endDate })
  ]);

  if (quoteResult.status === "fulfilled") {
    renderQuote(quoteResult.value);
    if (quoteResult.value.quality_status && quoteResult.value.quality_status !== "valid") {
      setNotice(`行情接口已返回数据，但质量状态为“${qualityLabel(quoteResult.value.quality_status)}”。请同时查看质量原因及数据源验收记录。`, "warning");
    }
  } else {
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

  setText("#last-updated", new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(new Date()));
  setText("#query-hint", `请求标的：${symbol} · 日 K 上限：${limit} · 复权口径：${adjustment} · 日期：${startDate || "起始不限"} 至 ${endDate || "截至今天"}`);
  state.isQuerying = false;
  $("#load-supplemental-button").disabled = !state.supplementSymbol || state.supplementLoading;
  $("#quote-form").classList.remove("is-loading");
  $("#quote-form button[type='submit']").disabled = false;
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

function renderTask(task) {
  state.taskStatus = task.status || state.taskStatus;
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
  $("#task-poll-label").textContent = TERMINAL_TASK_STATUSES.has(task.status) ? "任务已结束" : "每 2.5 秒轮询服务端状态";
}

function renderTaskEvents() {
  const list = $("#task-events");
  list.replaceChildren();
  const events = [...state.taskEvents.values()].sort((a, b) => a.sequence - b.sequence);
  if (!events.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "尚无持久化阶段事件。排队期间事件数组可以为空。";
    list.append(empty);
    return;
  }
  events.forEach((item) => {
    const event = item.event || {};
    const row = document.createElement("div");
    row.className = "event-row";
    const icon = document.createElement("span");
    icon.className = `event-status event-status-${String(event.status || "unknown").toLowerCase()}`;
    icon.textContent = event.status === "succeeded" ? "✓" : event.status === "failed" ? "!" : event.status === "skipped" ? "–" : "·";
    const body = document.createElement("div");
    body.className = "event-body";
    const title = document.createElement("strong");
    title.textContent = event.stage || event.name || "分析阶段";
    const detail = document.createElement("p");
    detail.textContent = event.message || event.status || "服务端事件";
    body.append(title, detail);
    if (event.error_code) {
      const diagnostic = document.createElement("p");
      diagnostic.className = "event-diagnostic";
      diagnostic.textContent = "错误代码：" + String(event.error_code);
      body.append(diagnostic);
    }
    const meta = document.createElement("small");
    const metaParts = [`#${item.sequence}`, formatDateTime(event.at)];
    if (event.duration_ms !== undefined && event.duration_ms !== null && event.duration_ms !== "" &&
        Number.isFinite(Number(event.duration_ms)) && Number(event.duration_ms) >= 0) {
      const durationMs = Number(event.duration_ms);
      metaParts.push(durationMs >= 1000 ? (durationMs / 1000).toFixed(2) + " 秒" : durationMs + " 毫秒");
    }
    meta.textContent = metaParts.join(" · ");
    row.append(icon, body, meta);
    list.append(row);
  });
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
  root.append(section);
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
  state.lastRunResult = result;
  state.runId = result.run_id || state.runId;
  renderRunStatus(result.status || "");
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
}

async function loadTaskRunResult(runId, expectedTaskId) {
  const generation = ++state.runResultGeneration;
  let retries = 0;
  $("#task-result-section").hidden = false;
  $("#task-result-note").textContent = "正在读取已保存的 Run 结果…";

  while (true) {
    if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId) return;
    try {
      const result = await getAnalysisRun(state.settings, runId);
      if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId) return;
      state.runId = result.run_id || runId;
      persistTaskReference();
      renderTaskResult(result);
      return;
    } catch (error) {
      if (generation !== state.runResultGeneration || state.taskId !== expectedTaskId) return;
      const delay = RUN_RESULT_RETRY_DELAYS_MS[retries];
      if (delay !== undefined && isRetryableApiError(error)) {
        retries += 1;
        $("#task-result-note").textContent =
          `Run 结果暂时不可读（${errorMessage(error)}），将在 ${delay / 1000} 秒后重试（${retries}/${RUN_RESULT_RETRY_DELAYS_MS.length}）…`;
        await new Promise((resolve) => window.setTimeout(resolve, delay));
        continue;
      }
      $("#task-result-note").textContent =
        "读取结果失败：" + errorMessage(error) + "。可点击“重新读取结果”或“立即刷新状态”重试。";
      return;
    }
  }
}

function stopTaskPolling() {
  if (state.pollTimer) window.clearTimeout(state.pollTimer);
  state.pollTimer = null;
  state.pollGeneration += 1;
}

async function pollTask(generation = state.pollGeneration) {
  if (!state.taskId || generation !== state.pollGeneration) return;
  const taskId = state.taskId;
  try {
    const [taskPayload, eventPayload] = await Promise.all([
      getAnalysisTask(state.settings, taskId),
      getAnalysisTaskEvents(state.settings, taskId, state.taskSequence)
    ]);
    if (generation !== state.pollGeneration || taskId !== state.taskId) return;
    const task = taskPayload.task || taskPayload;
    renderTask(task);
    for (const event of (eventPayload.events || [])) {
      state.taskEvents.set(event.sequence, event);
      state.taskSequence = Math.max(state.taskSequence, Number(event.sequence) || 0);
    }
    renderTaskEvents();
    if (TERMINAL_TASK_STATUSES.has(task.status)) {
      stopTaskPolling();
      const runId = task.run_id || state.runId;
      if (runId) void loadTaskRunResult(runId, taskId);
      return;
    }
    state.pollTimer = window.setTimeout(() => pollTask(generation), 2500);
  } catch (error) {
    if (generation !== state.pollGeneration) return;
    if (!isRetryableApiError(error)) {
      $("#task-poll-label").textContent =
        `自动刷新已暂停：${errorMessage(error)}。请检查权限、API 配置或任务 ID，再点击“立即刷新状态”重试。`;
      return;
    }
    $("#task-poll-label").textContent = `状态刷新失败：${errorMessage(error)}，5 秒后自动重试…`;
    state.pollTimer = window.setTimeout(() => pollTask(generation), 5000);
  }
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
  feedback.hidden = true;
  feedback.textContent = "";
  const button = $("#create-task-button");
  button.disabled = true;
  button.textContent = "正在写入任务队列…";
  const taskType = $("#task-type").value;
  const payload = { task_type: taskType, symbols };
  const idempotencyKey = $("#idempotency-key").value.trim();
  state.runResultGeneration += 1;
  stopTaskPolling();
  state.taskId = "";
  state.runId = "";
  state.taskSymbols = symbols;
  state.taskType = taskType;
  state.taskStatus = "";
  state.lastRunResult = null;
  state.taskSequence = 0;
  state.taskEvents.clear();
  $("#task-result-section").hidden = true;
  $("#task-result-metrics").replaceChildren();
  $("#task-result-structured").replaceChildren();
  $("#task-result-warnings").replaceChildren();

  try {
    const response = await createAnalysisTask(state.settings, payload, idempotencyKey);
    const task = response.task || response;
    if (!task.task_id) throw new Error("接口返回成功，但响应中缺少 task.task_id。");
    state.suppressedHistoryTaskId = "";
    state.taskId = task.task_id;
    state.runId = task.run_id || "";
    persistTaskReference();
    feedback.hidden = true;
    renderTask(task);
    renderTaskEvents();
    openRunFlowDrawer();
    const generation = state.pollGeneration;
    if (!TERMINAL_TASK_STATUSES.has(task.status)) {
      state.pollTimer = window.setTimeout(() => pollTask(generation), 500);
    } else if (task.run_id || state.runId) {
      void loadTaskRunResult(task.run_id || state.runId, task.task_id);
    }
  } catch (error) {
    $("#task-empty").hidden = false;
    $("#task-detail").hidden = true;
    feedback.hidden = false;
    feedback.textContent = "创建任务失败：" + errorMessage(error);
  } finally {
    state.isCreatingTask = false;
    button.disabled = false;
    button.replaceChildren(document.createTextNode("创建并跟踪任务 "));
    const arrow = document.createElement("span");
    arrow.textContent = "→";
    button.append(arrow);
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
  state.suppressedHistoryTaskId = "";
  state.runResultGeneration += 1;
  stopTaskPolling();
  state.taskId = record.task_id;
  state.runId = typeof record.run_id === "string" ? record.run_id : "";
  state.taskSymbols = Array.isArray(record.symbols) ? record.symbols : [];
  state.taskType = record.task_type || "post_market_scan";
  state.taskStatus = record.status || "";
  state.lastRunResult = null;
  state.taskSequence = 0;
  state.taskEvents.clear();
  $("#task-symbols").value = state.taskSymbols.join(", ") || "600519";
  if (["post_market_scan", "watchlist_scan", "position_review"].includes(state.taskType)) {
    $("#task-type").value = state.taskType;
  }
  $("#task-empty").hidden = true;
  $("#task-detail").hidden = false;
  $("#task-result-section").hidden = true;
  $("#task-result-metrics").replaceChildren();
  $("#task-result-structured").replaceChildren();
  $("#task-result-warnings").replaceChildren();
  setText("#task-id-value", state.taskId);
  setText("#task-run-id", state.runId);
  setText("#task-current-stage", "正在恢复任务");
  $("#task-poll-label").textContent = "正在从服务端恢复状态与阶段事件…";
  setView("tasks");
  renderRunStatus("");
  openRunFlowDrawer();
  renderTaskEvents();
  persistTaskReference();
  void pollTask(state.pollGeneration);
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
    ? "存在未保存更改。保存只会修改当前浏览器连接配置，不会更改服务器 .env。"
    : "没有未保存的连接配置更改。";
  $("#save-settings").disabled = !dirty;
  return dirty;
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
  $("#settings-dialog").showModal();
}

function closeSettingsWithGuard() {
  if (connectionDraftIsDirty() && !window.confirm("存在未保存的连接配置更改。确定放弃这些更改并关闭吗？")) return;
  $("#settings-dialog").close();
}

function applySettings(nextSettings) {
  state.settings = nextSettings;
  initialConnectionDraft = null;
  $("#settings-dialog").close();
  void refreshServices();
  void refreshSources();
  if (state.view === "market" && state.settings.apiKey) void runQuoteQuery();
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

syncThemeControl(readTheme());
$("#theme-toggle").addEventListener("click", () => {
  const nextTheme = readTheme() === "dark" ? "light" : "dark";
  persistTheme(nextTheme);
  applyTheme(nextTheme);
  syncThemeControl(nextTheme);
});
$("#mobile-menu-toggle").addEventListener("click", () => {
  setMobileNavigationOpen(!$(".sidebar").classList.contains("is-open"));
});
$("#mobile-nav-overlay").addEventListener("click", () => setMobileNavigationOpen(false));
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") setMobileNavigationOpen(false);
});
window.addEventListener("resize", () => {
  if (window.innerWidth > 600) setMobileNavigationOpen(false);
});
window.addEventListener("hashchange", () => {
  setView(window.location.hash.slice(1), { updateHash: false });
});
$("#topbar-notifications").addEventListener("click", () => setView("notifications"));
$("#chat-clear-context").addEventListener("click", () => {
  state.chatStockContext = null;
  renderChatStockContext();
});
$("#close-run-flow-dialog").addEventListener("click", () => $("#run-flow-dialog").close());
$("#run-flow-dialog").addEventListener("click", (event) => {
  if (event.target === event.currentTarget) event.currentTarget.close();
});
$("#settings-open-connection").addEventListener("click", openSettings);
$("#settings-check-connection").addEventListener("click", checkConnectionFromSettings);
$("#settings-view-data-sources").addEventListener("click", () => {
  setView("market");
  window.requestAnimationFrame(() => $("#data-source-list").scrollIntoView({ behavior: "smooth", block: "center" }));
});
document.querySelectorAll("[data-settings-target]").forEach((button) => {
  button.addEventListener("click", () => {
    const target = document.getElementById(button.dataset.settingsTarget);
    if (!target) return;
    document.querySelectorAll("[data-settings-target]").forEach((item) => {
      item.classList.toggle("is-active", item === button);
      if (item === button) item.setAttribute("aria-current", "location");
      else item.removeAttribute("aria-current");
    });
    target.scrollIntoView({ behavior: "smooth", block: "start" });
  });
});
document.querySelectorAll("[data-theme-value]").forEach((button) => {
  button.addEventListener("click", () => {
    const theme = persistTheme(button.dataset.themeValue);
    applyTheme(theme);
    syncThemeControl(theme);
    syncSettingsThemeButtons(theme);
  });
});
document.querySelectorAll(".nav-link").forEach((button) => {
  button.addEventListener("click", () => {
    setView(button.dataset.view);
    setMobileNavigationOpen(false);
  });
});
$("#quote-form").addEventListener("submit", runQuoteQuery);
$("#load-supplemental-button").addEventListener("click", loadSupplementalData);
$("#chart-view-close").addEventListener("click", () => setChartView("close"));
$("#chart-view-candles").addEventListener("click", () => setChartView("candles"));
$("#analyze-quote-button").addEventListener("click", startQuoteAnalysis);
$("#add-to-watchlist-button").addEventListener("click", addCurrentSymbolToWatchlist);
$("#watchlist-list").addEventListener("click", handleWatchlistAction);
$("#reload-sources").addEventListener("click", refreshSources);
$("#refresh-button").addEventListener("click", async () => {
  await refreshServices();
  if (state.view === "market") {
    await refreshSources();
    if (state.settings.apiKey) await runQuoteQuery();
  } else if (state.view === "tasks" && state.taskId) {
    stopTaskPolling();
    const generation = state.pollGeneration;
    await pollTask(generation);
  }
});
$("#open-settings").addEventListener("click", openSettings);
$("#open-settings-side").addEventListener("click", openSettings);
$("#save-settings").addEventListener("click", saveSettingsFromForm);
$("#close-settings-dialog").addEventListener("click", closeSettingsWithGuard);
$("#cancel-settings-dialog").addEventListener("click", closeSettingsWithGuard);
$("#settings-form").addEventListener("input", syncConnectionDraftState);
$("#settings-dialog").addEventListener("cancel", (event) => {
  if (!connectionDraftIsDirty()) return;
  event.preventDefault();
  closeSettingsWithGuard();
});
$("#toggle-market-key-visibility").addEventListener("click", () => {
  setMarketKeyVisibility($("#market-key-input").type === "password");
});
$("#settings-dialog").addEventListener("close", () => {
  setMarketKeyVisibility(false);
  initialConnectionDraft = null;
  $("#save-settings").disabled = false;
});
$("#clear-settings").addEventListener("click", clearLocalSettings);
$("#task-form").addEventListener("submit", createTask);
$("#refresh-task").addEventListener("click", () => {
  if (!state.taskId) return;
  stopTaskPolling();
  const generation = state.pollGeneration;
  void pollTask(generation);
});
$("#recent-analysis-search").addEventListener("input", () => renderRecentTasks());
$("#clear-recent-analysis").addEventListener("click", clearRecentTaskHistory);
$("#recent-analysis-list").addEventListener("click", (event) => {
  const removeButton = event.target.closest("button[data-remove-task-id]");
  if (removeButton) {
    removeRecentTaskReference(removeButton.dataset.removeTaskId);
    return;
  }
  const openButton = event.target.closest("button[data-open-task-id]");
  if (!openButton) return;
  const record = safeReadRecentTasks().find((item) => item.task_id === openButton.dataset.openTaskId);
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
    task_id: state.taskId || null,
    task_status: state.taskStatus || null,
    run_id: state.runId || null,
    symbols: state.taskSymbols,
    task_type: state.taskType,
    run_status: result.status || null,
    as_of: result.request?.as_of || null,
    quality: result.quality || null,
    failure: result.failure || null,
    warnings: result.warnings || [],
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
renderRecentTasks();
renderWatchlist();
configureDateInputs();
resetSupplementPanel("");
setView(window.location.hash.slice(1), { updateHash: false });
syncSettingsThemeButtons(readTheme());
void refreshServices();
void refreshSources();
const restoredTask = restoreLastTask();
if (!restoredTask && state.settings.apiKey) {
  void runQuoteQuery();
} else if (!restoredTask) {
  setText("#query-hint", "尚未配置行情 API Key。点击右上角设置按钮后填写 MARKET_DATA_API_KEY。");
}

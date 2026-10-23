import {
  analysisHealth,
  clearSettings,
  createAnalysisTask,
  getAnalysisRun,
  getAnalysisTask,
  getAnalysisTaskEvents,
  getDailyBars,
  getDataSources,
  getQuote,
  loadSettings,
  marketHealth,
  saveSettings
} from "./api.js";
import { renderCloseChart } from "./chart.js";
import {
  formatCompact,
  formatDate,
  formatDateTime,
  formatPercent,
  formatPrice,
  normalizeBasePath,
  normalizeSymbol,
  parseSymbols,
  qualityLabel,
  taskStatusLabel
} from "./format.js";

const $ = (selector) => document.querySelector(selector);
const state = {
  settings: loadSettings(),
  view: "market",
  quote: null,
  bars: [],
  taskId: "",
  taskSequence: 0,
  taskEvents: new Map(),
  pollTimer: null,
  pollGeneration: 0,
  isQuerying: false,
  sourceCount: 0
};

function setText(selector, value) {
  const target = $(selector);
  if (target) target.textContent = value == null || value === "" ? "—" : String(value);
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

function errorMessage(error) {
  if (error?.status === 401) return "行情 API Key 无效或未填写，请打开连接设置检查密钥。";
  if (error?.status === 404) return "接口不存在，请确认 API 路径及服务版本。";
  if (error?.status === 502 || error?.status === 503 || error?.status === 504) {
    return `${error.message || "服务暂不可用"}（HTTP ${error.status}）`;
  }
  return error?.message || "请求失败，请检查服务状态与浏览器网络面板。";
}

function getQuerySymbol() {
  return normalizeSymbol($("#symbol-input").value);
}

function setView(view) {
  state.view = view === "tasks" ? "tasks" : "market";
  $("#market-view").hidden = state.view !== "market";
  $("#tasks-view").hidden = state.view !== "tasks";
  document.querySelectorAll(".nav-link").forEach((button) => {
    button.classList.toggle("active", button.dataset.view === state.view);
  });
  setText("#breadcrumb-current", state.view === "market" ? "行情工作台" : "分析任务");
  setText("#view-title", state.view === "market" ? "行情工作台" : "分析任务");
}

function renderQuote(quote) {
  state.quote = quote;
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

function renderBars(response) {
  state.bars = Array.isArray(response.bars) ? response.bars : [];
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

  const drawn = renderCloseChart($("#close-chart"), bars);
  $("#chart-empty").hidden = drawn;
  setText("#chart-last-close", bars.length ? formatPrice(bars[bars.length - 1].close) : "—");
  setText("#chart-last-date", bars.length ? `截至 ${formatDate(bars[bars.length - 1].trade_date)}` : "暂无日 K 数据");
  setText("#chart-quality-note", qualityLabel(response.quality_status));
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

async function runQuoteQuery(event) {
  event?.preventDefault();
  if (state.isQuerying) return;
  let symbol;
  try {
    symbol = getQuerySymbol();
    state.settings.marketBase = normalizeBasePath(state.settings.marketBase, "/market-api");
  } catch (error) {
    setNotice(error.message, "danger");
    return;
  }
  if (!state.settings.apiKey) {
    setNotice("请先打开“连接与密钥设置”，填写行情服务的 MARKET_DATA_API_KEY。", "warning");
    $("#settings-dialog").showModal();
    return;
  }

  state.isQuerying = true;
  $("#quote-form").classList.add("is-loading");
  $("#quote-form button[type='submit']").disabled = true;
  setNotice("", "info");
  setText("#query-hint", `正在查询 ${symbol} 的实时行情与日 K…`);

  const limit = Number($("#bar-limit").value) || 120;
  const adjustment = $("#adjustment").value;
  const [quoteResult, barsResult] = await Promise.allSettled([
    getQuote(state.settings, symbol),
    getDailyBars(state.settings, symbol, { limit, adjustment })
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
    setText("#bars-meta", "查询失败");
    setText("#chart-quality-note", "日 K 查询失败");
    if (quoteResult.status === "fulfilled") setNotice(`实时行情已返回，日 K 查询失败：${errorMessage(barsResult.reason)}`, "warning");
  }

  setText("#last-updated", new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(new Date()));
  setText("#query-hint", `请求标的：${symbol} · 日 K 上限：${limit} · 复权口径：${adjustment}`);
  state.isQuerying = false;
  $("#quote-form").classList.remove("is-loading");
  $("#quote-form button[type='submit']").disabled = false;
}

function renderTask(task) {
  setQualityPill($("#task-status-pill"), task.status === "succeeded" ? "valid" : task.status === "partial" ? "degraded" : task.status === "failed" ? "invalid" : "unknown", taskStatusLabel(task.status));
  $("#task-empty").hidden = true;
  $("#task-detail").hidden = false;
  setText("#task-id-value", task.task_id || state.taskId);
  setText("#task-run-id", task.run_id);
  setText("#task-current-stage", task.current_stage || (task.status === "queued" ? "等待 Worker" : "未报告"));
  setText("#task-completed", task.completed_stages ?? 0);
  setText("#task-created", formatDateTime(task.created_at));
  const message = task.error_message || task.failure_code || "";
  $("#task-error").hidden = !message;
  $("#task-error").textContent = message;
  $("#task-poll-label").textContent = ["succeeded", "partial", "failed"].includes(task.status) ? "任务已结束" : "每 2.5 秒轮询服务端状态";
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
    const meta = document.createElement("small");
    meta.textContent = `#${item.sequence} · ${formatDateTime(event.at)}`;
    row.append(icon, body, meta);
    list.append(row);
  });
}

function renderTaskResult(result) {
  const metrics = [
    ["运行状态", result.status || "未报告"],
    ["质量门禁", result.quality?.core_pass === true ? "通过" : result.quality?.core_pass === false ? "未通过" : "未报告"],
    ["候选信号", Array.isArray(result.signals) ? result.signals.length : "—"],
    ["计划数量", Array.isArray(result.plans) ? result.plans.length : "—"],
    ["警告数量", Array.isArray(result.warnings) ? result.warnings.length : 0]
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

  const signalsRoot = $("#task-result-signals");
  signalsRoot.replaceChildren();
  const signals = Array.isArray(result.signals) ? result.signals : [];
  signals.forEach((signal, index) => {
    const item = document.createElement("article");
    item.className = "signal-result-card";
    const heading = document.createElement("strong");
    heading.textContent = signal.symbol || signal.stock_code || signal.code || signal.name || ("候选信号 " + (index + 1));
    const detail = document.createElement("p");
    detail.textContent = [
      signal.action || signal.direction || signal.signal_type || signal.decision,
      signal.score == null ? "" : ("评分 " + signal.score),
      signal.confidence == null ? "" : ("置信度 " + signal.confidence),
      signal.reason || signal.rationale || signal.message
    ].filter(Boolean).join(" · ") || "详细字段请查看完整 Run JSON。";
    item.append(heading, detail);
    signalsRoot.append(item);
  });
  if (!signals.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "本次运行没有返回候选信号；请结合质量门禁、警告与原始结果判断原因。";
    signalsRoot.append(empty);
  }

  const warningRoot = $("#task-result-warnings");
  warningRoot.replaceChildren();
  const warnings = Array.isArray(result.warnings) ? result.warnings : [];
  warningRoot.hidden = warnings.length === 0;
  warnings.forEach((warning) => {
    const line = document.createElement("p");
    line.textContent = String(warning);
    warningRoot.append(line);
  });
  $("#task-result-json").textContent = JSON.stringify(result, null, 2);
  $("#task-result-note").textContent = "Run ID：" + (result.run_id || "—") + " · 状态：" + (result.status || "未报告") + "。当前结果只用于工程验证与研究，不构成交易建议。";
  $("#task-result-section").hidden = false;
}

async function loadTaskRunResult(runId, expectedTaskId) {
  $("#task-result-section").hidden = false;
  $("#task-result-note").textContent = "正在读取已保存的 Run 结果…";
  try {
    const result = await getAnalysisRun(state.settings, runId);
    if (state.taskId !== expectedTaskId) return;
    renderTaskResult(result);
  } catch (error) {
    if (state.taskId !== expectedTaskId) return;
    $("#task-result-note").textContent = "读取结果失败：" + errorMessage(error) + "。可点击“立即刷新状态”重试。";
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
    if (["succeeded", "partial", "failed"].includes(task.status)) {
      stopTaskPolling();
      if (task.run_id) void loadTaskRunResult(task.run_id, taskId);
      return;
    }
    state.pollTimer = window.setTimeout(() => pollTask(generation), 2500);
  } catch (error) {
    if (generation !== state.pollGeneration) return;
    $("#task-poll-label").textContent = `状态刷新失败：${errorMessage(error)}`;
    state.pollTimer = window.setTimeout(() => pollTask(generation), 5000);
  }
}

async function createTask(event) {
  event.preventDefault();
  let symbols;
  try {
    symbols = parseSymbols($("#task-symbols").value);
  } catch (error) {
    window.alert(error.message);
    return;
  }
  const button = $("#create-task-button");
  button.disabled = true;
  button.textContent = "正在写入任务队列…";
  const taskType = $("#task-type").value;
  const payload = { task_type: taskType, symbols };
  const idempotencyKey = $("#idempotency-key").value.trim();
  stopTaskPolling();
  state.taskId = "";
  state.taskSequence = 0;
  state.taskEvents.clear();
  $("#task-result-section").hidden = true;
  $("#task-result-metrics").replaceChildren();
  $("#task-result-signals").replaceChildren();
  $("#task-result-warnings").replaceChildren();

  try {
    const response = await createAnalysisTask(state.settings, payload, idempotencyKey);
    const task = response.task || response;
    if (!task.task_id) throw new Error("接口返回成功，但响应中缺少 task.task_id。");
    state.taskId = task.task_id;
    renderTask(task);
    renderTaskEvents();
    const generation = state.pollGeneration;
    if (!["succeeded", "partial", "failed"].includes(task.status)) {
      state.pollTimer = window.setTimeout(() => pollTask(generation), 500);
    } else if (task.run_id) {
      void loadTaskRunResult(task.run_id, task.task_id);
    }
  } catch (error) {
    $("#task-empty").hidden = false;
    $("#task-detail").hidden = true;
    window.alert(`创建任务失败：${errorMessage(error)}`);
  } finally {
    button.disabled = false;
    button.innerHTML = '创建并跟踪任务 <span>→</span>';
  }
}

function openSettings() {
  $("#market-base-input").value = state.settings.marketBase;
  $("#market-key-input").value = state.settings.apiKey;
  $("#analysis-base-input").value = state.settings.analysisBase;
  $("#settings-feedback").textContent = "";
  $("#settings-dialog").showModal();
}

function applySettings(nextSettings) {
  state.settings = nextSettings;
  $("#settings-dialog").close();
  void refreshServices();
  void refreshSources();
  if (state.view === "market" && state.settings.apiKey) void runQuoteQuery();
}

function saveSettingsFromForm() {
  try {
    const next = saveSettings({
      marketBase: normalizeBasePath($("#market-base-input").value, "/market-api"),
      analysisBase: normalizeBasePath($("#analysis-base-input").value, "/analysis-api"),
      apiKey: $("#market-key-input").value
    });
    applySettings(next);
  } catch (error) {
    $("#settings-feedback").textContent = error.message;
  }
}

function clearLocalSettings() {
  const next = clearSettings();
  $("#market-base-input").value = next.marketBase;
  $("#analysis-base-input").value = next.analysisBase;
  $("#market-key-input").value = "";
  $("#settings-feedback").textContent = "已清除本浏览器保存的 API Key 与自定义路径。";
  applySettings(next);
}

document.querySelectorAll(".nav-link").forEach((button) => {
  button.addEventListener("click", () => setView(button.dataset.view));
});
$("#quote-form").addEventListener("submit", runQuoteQuery);
$("#reload-sources").addEventListener("click", refreshSources);
$("#refresh-button").addEventListener("click", async () => {
  await refreshServices();
  if (state.view === "market") {
    await refreshSources();
    if (state.settings.apiKey) await runQuoteQuery();
  } else if (state.taskId) {
    stopTaskPolling();
    const generation = state.pollGeneration;
    await pollTask(generation);
  }
});
$("#open-settings").addEventListener("click", openSettings);
$("#open-settings-side").addEventListener("click", openSettings);
$("#save-settings").addEventListener("click", saveSettingsFromForm);
$("#clear-settings").addEventListener("click", clearLocalSettings);
$("#task-form").addEventListener("submit", createTask);
$("#refresh-task").addEventListener("click", () => {
  if (!state.taskId) return;
  stopTaskPolling();
  const generation = state.pollGeneration;
  void pollTask(generation);
});
$("#copy-task-id").addEventListener("click", async () => {
  if (!state.taskId) return;
  try {
    await navigator.clipboard.writeText(state.taskId);
    $("#copy-task-id").textContent = "已复制";
    window.setTimeout(() => { $("#copy-task-id").textContent = "复制"; }, 1400);
  } catch {
    $("#copy-task-id").textContent = state.taskId;
  }
});

setView("market");
void refreshServices();
void refreshSources();
if (state.settings.apiKey) {
  void runQuoteQuery();
} else {
  setText("#query-hint", "尚未配置行情 API Key。点击右上角设置按钮后填写 MARKET_DATA_API_KEY。");
}

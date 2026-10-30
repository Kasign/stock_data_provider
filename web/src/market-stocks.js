import { getMarketStocks } from "./api.js";
import { describeApiError, finiteNumberOrNull, formatCompact, formatPercent, formatPrice, qualityLabel } from "./format.js";

const $ = (selector) => document.querySelector(selector);
const PAGE_SIZE_OPTIONS = new Set([20, 50, 100]);

function element(tag, className = "", value = "") {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (value !== null && value !== undefined) node.textContent = String(value);
  return node;
}

function displayNumber(value, formatter) {
  const number = finiteNumberOrNull(value);
  return number === null ? "—" : formatter(number);
}

function responseError(error) {
  if (Number(error?.status) === 401) return "行情 API Key 无效或未获授权，请检查连接与密钥设置。";
  if (Number(error?.status) === 503) return "当前没有已配置完整 A 股证券目录能力的 Provider。";
  if (Number(error?.status) === 502) return "完整 A 股证券目录获取失败；请检查 Provider 状态与服务端错误信息。";
  return describeApiError(error, "全市场行情读取失败。");
}

export function bindMarketStocks(getSettings, onSelectStock) {
  const state = { page: 1, pageSize: 50, adjustment: "forward_adjusted", busy: false, hasResult: false, hasNext: false, generation: 0, controller: null };

  function renderEmpty(message) {
    const body = $("#market-stocks-body");
    if (!body) return;
    body.replaceChildren();
    const row = element("tr");
    const cell = element("td", "table-empty", message);
    cell.colSpan = 10;
    row.append(cell);
    body.append(row);
  }

  function setBusy(busy) {
    state.busy = busy;
    ["#load-market-stocks", "#reload-market-stocks", "#market-stocks-page-size", "#market-stocks-adjustment"].forEach((selector) => {
      const node = $(selector);
      if (node) node.disabled = busy;
    });
    const prev = $("#market-stocks-prev"), next = $("#market-stocks-next");
    if (prev) prev.disabled = busy || state.page <= 1;
    if (next) next.disabled = busy || !state.hasNext;
    const load = $("#load-market-stocks"), reload = $("#reload-market-stocks");
    if (load) load.textContent = busy ? "读取中…" : (state.hasResult ? "重新加载全市场" : "加载全市场");
    if (reload) reload.disabled = busy || !state.hasResult;
  }

  function renderRows(items) {
    const body = $("#market-stocks-body");
    if (!body) return;
    body.replaceChildren();
    if (!items.length) { renderEmpty("当前页没有返回证券记录；请检查目录分页与数据源状态。"); return; }
    items.forEach((item) => {
      const quote = item?.quote && typeof item.quote === "object" ? item.quote : null;
      const symbol = String(item?.symbol || item?.code || quote?.symbol || "").trim();
      const name = String(item?.name || quote?.name || symbol || "未报告");
      const row = element("tr"), stockCell = element("td");
      stockCell.append(element("strong", "market-stocks-stock-name", name), element("small", "market-stocks-stock-symbol", symbol || "代码未报告"));
      row.append(stockCell);
      const change = finiteNumberOrNull(quote?.change_percent);
      const changeClass = change === null ? "" : change > 0 ? "positive" : change < 0 ? "negative" : "";
      [
        displayNumber(quote?.current_price, (value) => formatPrice(value, 2)),
        displayNumber(quote?.change_percent, formatPercent),
        displayNumber(quote?.amount, formatCompact),
        displayNumber(quote?.turnover_rate, formatPercent),
        displayNumber(quote?.volume_ratio, (value) => value.toFixed(2)),
        displayNumber(item?.ma5, (value) => formatPrice(value, 2)),
        displayNumber(item?.ma20, (value) => formatPrice(value, 2))
      ].forEach((value, index) => row.append(element("td", index === 1 ? changeClass : "", value)));
      const quality = item?.quality_status || quote?.quality_status || "unknown";
      const qualityCell = element("td");
      const qualityPill = element("span", "mini-quality mini-quality-" + String(quality).toLowerCase(), qualityLabel(quality));
      const qualityReasons = [
        ...(Array.isArray(item?.quality_reasons) ? item.quality_reasons : []),
        ...(Array.isArray(quote?.quality_reasons) ? quote.quality_reasons : [])
      ].filter(Boolean);
      qualityPill.title = qualityReasons.join("；") || item?.error_code || "服务端未提供详细质量原因";
      qualityPill.setAttribute("aria-label", qualityLabel(quality) + "：" + qualityPill.title);
      qualityCell.append(qualityPill);
      row.append(qualityCell);
      const actionCell = element("td"), action = element("button", "button button-secondary market-stocks-row-action", "查看详情");
      action.type = "button";
      action.dataset.marketStockSymbol = symbol;
      action.dataset.marketStockName = name;
      action.disabled = !symbol;
      actionCell.append(action);
      row.append(actionCell);
      body.append(row);
    });
  }

  function renderResult(data) {
    const items = Array.isArray(data?.items) ? data.items : [];
    const total = Number.isFinite(Number(data?.total)) ? Number(data.total) : null;
    const lowerBound = Boolean(data?.total_is_lower_bound);
    state.hasResult = true;
    state.hasNext = items.length >= state.pageSize && (lowerBound || total === null || state.page * state.pageSize < total);
    const coverage = finiteNumberOrNull(data?.coverage_ratio);
    const coverageText = coverage === null ? "未报告" : formatPercent(coverage * 100, 1);
    const totalText = total === null ? "未报告" : (lowerBound ? "至少 " : "") + total.toLocaleString("zh-CN");
    const summary = "第 " + state.page + " 页 · 返回 " + items.length + " 条 · 目录证券数 " + totalText +
      " · 当前页行情覆盖 " + coverageText + "（成功 " + (data?.quote_succeeded ?? "未报告") +
      " / 失败 " + (data?.quote_failed ?? "未报告") + "）";
    const status = $("#market-stocks-status");
    if (status) status.textContent = qualityLabel(data?.quality_status);
    const summaryNode = $("#market-stocks-summary");
    if (summaryNode) summaryNode.textContent = summary;
    const pageLabel = $("#market-stocks-page-label");
    if (pageLabel) pageLabel.textContent = "第 " + state.page + " 页 · 每页 " + state.pageSize + " 条";
    const meta = $("#market-stocks-meta");
    if (meta) {
      const reasons = Array.isArray(data?.quality_reasons) ? data.quality_reasons.filter(Boolean) : [];
      meta.textContent = "证券目录：" + (data?.universe_source || "来源未报告") + " / " + (data?.universe_source_version || "版本未报告") +
        " · 目录获取：" + (data?.universe_fetched_at || "未报告") + " · 行情获取：" + (data?.fetched_at || "未报告") +
        " · MA 复权：" + (data?.ma_adjustment_basis || "未报告") + (reasons.length ? " · 质量原因：" + reasons.join("；") : "");
    }
    renderRows(items);
    setBusy(false);
  }

  async function loadPage(page = state.page) {
    if (state.busy) return;
    const settings = typeof getSettings === "function" ? getSettings() : null;
    if (!settings?.apiKey) {
      const status = $("#market-stocks-status"), summary = $("#market-stocks-summary");
      if (status) status.textContent = "需配置 API Key";
      if (summary) summary.textContent = "请先在连接与密钥设置中配置行情 API Key。";
      renderEmpty("未发送请求：需要先配置行情 API Key。");
      return;
    }
    const size = Number($("#market-stocks-page-size")?.value || state.pageSize);
    state.pageSize = PAGE_SIZE_OPTIONS.has(size) ? size : 50;
    state.adjustment = $("#market-stocks-adjustment")?.value || "forward_adjusted";
    state.page = Math.max(1, Math.min(10000, Math.trunc(Number(page) || 1)));
    const generation = ++state.generation;
    try { state.controller?.abort(); } catch {}
    const controller = new AbortController();
    state.controller = controller;
    setBusy(true);
    const status = $("#market-stocks-status");
    if (status) status.textContent = "正在读取";
    try {
      const data = await getMarketStocks(settings, { page: state.page, pageSize: state.pageSize, includeMA: true, adjustment: state.adjustment, signal: controller.signal });
      if (controller.signal.aborted || generation !== state.generation) return;
      renderResult(data || {});
    } catch (error) {
      if (controller.signal.aborted || generation !== state.generation) return;
      if (status) status.textContent = "读取失败";
      const summary = $("#market-stocks-summary");
      if (summary) summary.textContent = responseError(error);
      const meta = $("#market-stocks-meta");
      if (meta) meta.textContent = "本次请求未取得新结果；列表已清空，避免将旧数据误认为当前连接的结果。";
      renderEmpty(responseError(error));
      state.hasResult = false;
      state.hasNext = false;
      setBusy(false);
    } finally {
      if (generation === state.generation && state.controller === controller) state.controller = null;
    }
  }

  $("#load-market-stocks")?.addEventListener("click", () => { void loadPage(1); });
  $("#reload-market-stocks")?.addEventListener("click", () => { void loadPage(state.page); });
  $("#market-stocks-prev")?.addEventListener("click", () => { if (state.page > 1) void loadPage(state.page - 1); });
  $("#market-stocks-next")?.addEventListener("click", () => { if (state.hasNext) void loadPage(state.page + 1); });
  $("#market-stocks-page-size")?.addEventListener("change", () => { void loadPage(1); });
  $("#market-stocks-adjustment")?.addEventListener("change", () => { if (state.hasResult) void loadPage(1); });
  $("#market-stocks-body")?.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-market-stock-symbol]");
    if (!button || !button.dataset.marketStockSymbol || typeof onSelectStock !== "function") return;
    onSelectStock({ symbol: button.dataset.marketStockSymbol, name: button.dataset.marketStockName || button.dataset.marketStockSymbol }, "全市场行情");
  });
  setBusy(false);
  return {
    invalidate() {
      state.generation += 1;
      try { state.controller?.abort(); } catch {}
      state.controller = null;
      state.hasResult = false;
      state.hasNext = false;
      renderEmpty("行情连接配置已变化，请重新加载。");
      const status = $("#market-stocks-status");
      if (status) status.textContent = "连接已变化";
      setBusy(false);
    }
  };
}

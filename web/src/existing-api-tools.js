import {
  getIndexCatalog,
  getIndexConstituents,
  getIndexDailyBars,
  getMarketSession,
  getStockPriceLimitRule,
  getStockTradingStatus
} from "./api.js";
import { formatDateTime, formatPrice, qualityLabel } from "./format.js";

const $ = (selector) => document.querySelector(selector);
const QUALITY = new Set(["valid", "degraded", "stale", "invalid", "unknown"]);
const safe = (value) => value === null || value === undefined || value === "" ? "—" : String(value);
function node(tag, text = "") {
  const element = document.createElement(tag);
  element.textContent = safe(text);
  return element;
}
function cell(row, value) { row.append(node("td", value)); }
function renderRows(selector, items, columns, emptyText) {
  const root = $(selector);
  if (!root) return;
  root.replaceChildren();
  if (!items.length) {
    const row = document.createElement("tr");
    const td = node("td", emptyText);
    td.colSpan = columns.length;
    td.className = "table-empty";
    row.append(td);
    root.append(row);
    return;
  }
  items.forEach((item) => {
    const row = document.createElement("tr");
    columns.forEach((column) => cell(row, column(item)));
    root.append(row);
  });
}
function setFeedback(selector, message, isError = false) {
  const target = $(selector);
  if (!target) return;
  target.textContent = message;
  target.classList.toggle("is-error", isError);
}
function setQuality(status) {
  const target = $("#index-tools-quality");
  if (!target) return;
  const value = QUALITY.has(String(status).toLowerCase()) ? String(status).toLowerCase() : "unknown";
  target.className = "quality-pill quality-" + value;
  target.textContent = qualityLabel(value);
}
function renderDetail(selector, values) {
  const root = $(selector);
  if (!root) return;
  root.replaceChildren();
  for (const [label, value] of values) {
    root.append(node("dt", label), node("dd", value));
  }
}
function errorText(error) {
  if (Number(error?.status) === 404) return "当前服务未找到对应数据或历史记录。";
  if (Number(error?.status) === 503 || Number(error?.status) === 502) return "上游 Provider 不可用或返回的数据不完整，请查看服务端诊断。";
  if (Number(error?.status) === 400) return "请求参数不符合接口要求，请检查股票代码、指数代码和日期范围。";
  if ([401, 403].includes(Number(error?.status))) return "API Key 验证失败，请检查设置中心的行情服务密钥。";
  return String(error?.message || "请求失败").slice(0, 300);
}
function qualityMeta(data) {
  return [
    "质量 " + qualityLabel(data?.quality_status),
    "来源 " + safe(data?.source),
    "获取时间 " + formatDateTime(data?.fetched_at),
    Array.isArray(data?.quality_reasons) && data.quality_reasons.length ? "说明 " + data.quality_reasons.join("；") : ""
  ].filter(Boolean).join(" · ");
}
function dateTodayChina() {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date());
  const map = Object.fromEntries(parts.filter((part) => part.type !== "literal").map((part) => [part.type, part.value]));
  return map.year + "-" + map.month + "-" + map.day;
}

export function bindExistingApiTools(getSettings) {
  const settings = () => getSettings();
  const today = dateTodayChina();
  const recentStart = (() => { const date = new Date(today + "T00:00:00Z"); date.setUTCDate(date.getUTCDate() - 30); return date.toISOString().slice(0, 10); })();
  for (const id of ["index-constituents-date", "index-daily-end", "stock-status-date"]) {
    const input = $("#" + id);
    if (input && !input.value) input.value = today;
  }
  const startInput = $("#index-daily-start");
  if (startInput && !startInput.value) startInput.value = recentStart;

  $("#load-index-catalog")?.addEventListener("click", async () => {
    const button = $("#load-index-catalog");
    button.disabled = true;
    try {
      const data = await getIndexCatalog(settings(), { tag: $("#index-catalog-tag").value });
      const items = Array.isArray(data?.items) ? data.items : [];
      renderRows("#index-catalog-body", items, [item => item.symbol, item => item.name, item => item.tag], "接口成功响应，但没有目录记录。");
      setQuality(data.quality_status);
      setFeedback("#index-tools-feedback", "指数目录 " + items.length + " 条；" + qualityMeta(data) + (data.total_is_lower_bound ? "；目录数量为下限，可能达到服务端安全上限。" : ""));
    } catch (error) {
      setFeedback("#index-tools-feedback", "读取指数目录失败：" + errorText(error), true);
      renderRows("#index-catalog-body", [], [x => x, x => x, x => x], "请求失败，未保留旧结果。");
    } finally { button.disabled = false; }
  });

  $("#load-index-constituents")?.addEventListener("click", async () => {
    const button = $("#load-index-constituents");
    const symbol = $("#index-tool-symbol").value.trim();
    const asOf = $("#index-constituents-date").value;
    button.disabled = true;
    try {
      const data = await getIndexConstituents(settings(), symbol, { asOf });
      const items = Array.isArray(data?.items) ? data.items : [];
      renderRows("#index-constituents-body", items, [item => item.name, item => item.symbol || item.code, item => item.weight_percent == null ? "未提供" : item.weight_percent + "%"], "接口成功响应，但没有成分记录。");
      setQuality(data.quality_status);
      setFeedback("#index-tools-feedback", "指数成分 " + items.length + " 条；报告日期 " + safe(data.provider_report_date || data.as_of_date) + "；" + qualityMeta(data) + (data.total_is_lower_bound ? "；结果可能被服务端上限截断。" : ""));
    } catch (error) {
      renderRows("#index-constituents-body", [], [x => x, x => x, x => x], "请求失败，未保留旧结果。");
      setFeedback("#index-tools-feedback", "读取指数成分失败：" + errorText(error), true);
    } finally { button.disabled = false; }
  });

  $("#load-index-daily")?.addEventListener("click", async () => {
    const button = $("#load-index-daily");
    const symbol = $("#index-tool-symbol").value.trim();
    const start = $("#index-daily-start").value;
    const end = $("#index-daily-end").value;
    button.disabled = true;
    try {
      const data = await getIndexDailyBars(settings(), symbol, { start, end });
      const items = Array.isArray(data?.items) ? data.items : [];
      renderRows("#index-daily-body", items, [
        item => item.trade_date,
        item => formatPrice(item.open),
        item => formatPrice(item.high),
        item => formatPrice(item.low),
        item => formatPrice(item.close),
        item => item.volume == null ? "未提供" : Number(item.volume).toLocaleString("zh-CN") + " " + safe(item.volume_unit)
      ], "接口成功响应，但没有日线记录。");
      setQuality(data.quality_status);
      setFeedback("#index-tools-feedback", "指数日线 " + items.length + " 条；" + qualityMeta(data));
    } catch (error) {
      renderRows("#index-daily-body", [], [x => x, x => x, x => x, x => x, x => x, x => x], "请求失败，未保留旧结果。");
      setFeedback("#index-tools-feedback", "读取指数日线失败：" + errorText(error), true);
    } finally { button.disabled = false; }
  });

  $("#load-market-session")?.addEventListener("click", async () => {
    const button = $("#load-market-session");
    button.disabled = true;
    try {
      const data = await getMarketSession(settings());
      const phase = {
        non_trading_day: "非交易日", pre_open: "盘前", opening_auction: "开盘集合竞价",
        auction_pause: "集合竞价暂停", continuous_morning: "上午连续竞价", lunch_break: "午间休市",
        continuous_afternoon: "下午连续竞价", closing_auction: "收盘集合竞价", closed: "已收盘"
      };
      $("#market-session-summary").textContent = "A 股交易时段：" + (phase[data.phase] || safe(data.phase)) +
        " · 日期 " + safe(data.local_date) + " " + safe(data.local_time) +
        " · 交易日 " + (data.is_trading_day ? "是" : data.is_trading_day === false ? "否" : "未知") +
        " · 当前交易时段 " + (data.is_trading_session ? "是" : data.is_trading_session === false ? "否" : "未知") +
        " · " + qualityMeta(data);
      setQuality(data.quality_status);
    } catch (error) {
      $("#market-session-summary").textContent = "交易时段读取失败：" + errorText(error);
    } finally { button.disabled = false; }
  });

  async function loadStockStatus(kind) {
    const symbol = $("#stock-status-symbol").value.trim();
    const date = $("#stock-status-date").value;
    const button = $("#load-stock-" + kind);
    button.disabled = true;
    try {
      const data = kind === "trading-status"
        ? await getStockTradingStatus(settings(), symbol, date)
        : await getStockPriceLimitRule(settings(), symbol, date);
      if (kind === "trading-status") {
        renderDetail("#stock-trading-status-result", [
          ["股票", safe(data.security_name) + " (" + safe(data.symbol) + ")"],
          ["交易日期", safe(data.trade_date)],
          ["风险警示", safe(data.risk_warning_status)],
          ["退市状态", safe(data.delisting_status)],
          ["上市状态可确认", data.listing_status_known ? "是" : "否"],
          ["交易日", data.trading_day_known ? (data.is_trading_day ? "是" : "否") : "未知"],
          ["停复牌证据", safe(data.suspension_status)],
          ["停牌时间 / 类型", [data.suspension_timing, data.suspension_type].filter(Boolean).join(" / ") || "未提供"],
          ["数据质量", qualityLabel(data.quality_status)],
          ["来源 / 获取时间", safe(data.source) + " / " + formatDateTime(data.fetched_at)],
          ["质量说明", Array.isArray(data.quality_reasons) && data.quality_reasons.length ? data.quality_reasons.join("；") : "无额外说明"]
        ]);
      } else {
        renderDetail("#stock-price-limit-result", [
          ["股票", safe(data.symbol)],
          ["交易日期", safe(data.trade_date)],
          ["规则状态", data.status === "limited" ? "有涨跌幅限制" : data.status === "no_limit" ? "无涨跌幅限制" : "未知"],
          ["限制比例", data.limit_percent == null ? "未确认" : data.limit_percent + "%"],
          ["风险警示", safe(data.risk_warning_status)],
          ["上市交易日序号", safe(data.listing_trading_day_number)],
          ["特殊事件证据", safe(data.special_exception_status)],
          ["规则版本", safe(data.rule_version)],
          ["规则来源", safe(data.rule_source)],
          ["数据质量", qualityLabel(data.quality_status)],
          ["质量说明", Array.isArray(data.quality_reasons) && data.quality_reasons.length ? data.quality_reasons.join("；") : "无额外说明"]
        ]);
        if (data.rule_source_url && /^https:\/\//.test(data.rule_source_url)) {
          const root = $("#stock-price-limit-result");
          const link = document.createElement("a");
          link.href = data.rule_source_url;
          link.target = "_blank";
          link.rel = "noopener noreferrer";
          link.textContent = "查看规则来源";
          root.append(node("dt", "规则链接"), Object.assign(node("dd"), { textContent: "" }));
          root.lastChild.append(link);
        }
      }
      setFeedback("#stock-status-feedback", "查询成功；质量 " + qualityLabel(data.quality_status) + "；" + (data.quality_reasons || []).join("；"));
    } catch (error) {
      renderDetail(kind === "trading-status" ? "#stock-trading-status-result" : "#stock-price-limit-result", [["请求状态", errorText(error)]]);
      setFeedback("#stock-status-feedback", "查询失败：" + errorText(error), true);
    } finally { button.disabled = false; }
  }
  $("#load-stock-trading-status")?.addEventListener("click", () => loadStockStatus("trading-status"));
  $("#load-stock-limit-rule")?.addEventListener("click", () => loadStockStatus("limit-rule"));

  // 首页默认展示服务端交易时段；没有 API Key 时不发起无效请求。
  if (settings().apiKey) $("#load-market-session")?.click();
}

const SVG_NS = "http://www.w3.org/2000/svg";
const CHART_WIDTH = 760;

function element(name, attributes = {}, text = "") {
  const node = document.createElementNS(SVG_NS, name);
  Object.entries(attributes).forEach(([key, value]) => node.setAttribute(key, String(value)));
  if (text !== "") node.textContent = text;
  return node;
}

function numeric(value) {
  if (value === null || value === undefined || value === "" ||
      (typeof value !== "number" && typeof value !== "string")) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function positivePrice(value) {
  const parsed = numeric(value);
  return parsed !== null && parsed > 0 ? parsed : null;
}

// Only fields in the existing DailyBar response are used. Missing values remain null.
export function normalizeDailyBars(bars = []) {
  if (!Array.isArray(bars)) return [];
  return bars.map((bar) => {
    if (!bar || typeof bar !== "object") return null;
    const date = String(bar.trade_date ?? "").slice(0, 10);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return null;
    const volume = numeric(bar.volume);
    const amount = numeric(bar.amount);
    return {
      date,
      open: positivePrice(bar.open),
      high: positivePrice(bar.high),
      low: positivePrice(bar.low),
      close: positivePrice(bar.close),
      volume: volume !== null && volume >= 0 ? volume : null,
      amount: amount !== null && amount >= 0 ? amount : null,
      changePercent: numeric(bar.change_percent),
      adjustmentBasis: String(bar.adjustment_basis || ""),
      source: String(bar.source || ""),
      qualityStatus: String(bar.quality_status || "unknown"),
      qualityReasons: Array.isArray(bar.quality_reasons) ? bar.quality_reasons.filter((item) => typeof item === "string") : [],
      isPartial: bar.is_partial === true,
      fallbackFrom: String(bar.fallback_from || ""),
      rawVolumeUnit: String(bar.volume_unit || ""),
      rawAmountUnit: String(bar.amount_unit || "")
    };
  }).filter(Boolean).sort((left, right) => left.date.localeCompare(right.date));
}

function paddedRange(low, high, ratio) {
  const spread = Math.max(high - low, Math.abs(high) * 0.015, 0.01);
  return { min: Math.max(0, low - spread * ratio), max: high + spread * ratio };
}

function addDateLabels(svg, points, x, y) {
  const indexes = [...new Set([0, Math.floor((points.length - 1) / 3), Math.floor(((points.length - 1) * 2) / 3), points.length - 1])];
  indexes.forEach((index) => {
    svg.append(element("text", {
      x: x(index), y,
      "text-anchor": index === 0 ? "start" : index === points.length - 1 ? "end" : "middle",
      class: "chart-axis-label"
    }, points[index].date.slice(5).replace("-", ".")));
  });
}

function compactNumber(value) {
  if (value >= 100000000) return (value / 100000000).toFixed(1).replace(/\.0$/, "") + "亿";
  if (value >= 10000) return (value / 10000).toFixed(1).replace(/\.0$/, "") + "万";
  return Math.round(value).toLocaleString("zh-CN");
}

function candleTitle(point, options = {}) {
  const price = (value) => value === null ? "未报告" : value.toFixed(2);
  const volumeUnit = point.rawVolumeUnit || options.volumeUnit || "";
  const amountUnit = point.rawAmountUnit || options.amountUnit || "";
  const details = [
    point.date, "开盘 " + price(point.open), "最高 " + price(point.high),
    "最低 " + price(point.low), "收盘 " + price(point.close),
    "成交量 " + (point.volume === null ? "未报告" : compactNumber(point.volume) + (volumeUnit ? " " + volumeUnit : "")),
    "成交额 " + (point.amount === null ? "未报告" : compactNumber(point.amount) + (amountUnit ? " " + amountUnit : "")),
    "复权口径 " + (point.adjustmentBasis || options.adjustmentBasis || "unknown"),
    "数据质量 " + point.qualityStatus
  ];
  if (point.changePercent !== null) details.push("涨跌幅 " + (point.changePercent > 0 ? "+" : "") + point.changePercent.toFixed(2) + "%");
  if (point.source) details.push("来源 " + point.source);
  if (point.isPartial) details.push("当前交易时段 K 线可能尚未完成");
  if (point.fallbackFrom) details.push("回退来源 " + point.fallbackFrom);
  if (point.qualityReasons.length) details.push("质量原因 " + point.qualityReasons.join("、"));
  return details.join(" | ");
}

export function renderCloseChart(svg, bars = []) {
  svg.replaceChildren();
  svg.setAttribute("viewBox", "0 0 760 260");
  const points = normalizeDailyBars(bars).filter((bar) => bar.close !== null);
  if (points.length < 2) return false;

  const height = 260;
  const margin = { top: 16, right: 20, bottom: 34, left: 58 };
  const plotWidth = CHART_WIDTH - margin.left - margin.right;
  const plotHeight = height - margin.top - margin.bottom;
  const values = points.map((point) => point.close);
  const range = paddedRange(Math.min(...values), Math.max(...values), 0.12);
  const x = (index) => margin.left + (index / (points.length - 1)) * plotWidth;
  const y = (value) => margin.top + ((range.max - value) / (range.max - range.min || 1)) * plotHeight;

  const gradient = element("linearGradient", { id: "close-area-gradient", x1: "0", y1: "0", x2: "0", y2: "1" });
  gradient.append(element("stop", { offset: "0%", "stop-color": "#43d6ae", "stop-opacity": "0.22" }));
  gradient.append(element("stop", { offset: "100%", "stop-color": "#43d6ae", "stop-opacity": "0" }));
  const defs = element("defs");
  defs.append(gradient);
  svg.append(defs);

  for (let tick = 0; tick <= 4; tick += 1) {
    const value = range.max - ((range.max - range.min) * tick) / 4;
    const positionY = y(value);
    svg.append(element("line", { x1: margin.left, y1: positionY, x2: CHART_WIDTH - margin.right, y2: positionY, class: "chart-grid-line" }));
    svg.append(element("text", { x: margin.left - 10, y: positionY + 4, "text-anchor": "end", class: "chart-axis-label" }, value.toFixed(2)));
  }

  const path = points.map((point, index) => `${index ? "L" : "M"} ${x(index).toFixed(2)} ${y(point.close).toFixed(2)}`).join(" ");
  const area = `${path} L ${x(points.length - 1).toFixed(2)} ${(height - margin.bottom).toFixed(2)} L ${x(0).toFixed(2)} ${(height - margin.bottom).toFixed(2)} Z`;
  svg.append(element("path", { d: area, fill: "url(#close-area-gradient)" }));
  svg.append(element("path", { d: path, class: "chart-close-line" }));

  points.forEach((point, index) => {
    const marker = element("circle", {
      cx: x(index), cy: y(point.close), r: 4, class: "chart-line-hit", tabindex: "0",
      "aria-label": point.date + " 收盘价 " + point.close.toFixed(2)
    });
    marker.append(element("title", {}, `${point.date} | 收盘价 ${point.close.toFixed(2)}${point.changePercent === null ? "" : " | 涨跌幅 " + point.changePercent.toFixed(2) + "%"}`));
    svg.append(marker);
  });
  addDateLabels(svg, points, x, height - 10);
  const lastIndex = points.length - 1;
  svg.append(element("line", { x1: x(lastIndex), y1: y(points[lastIndex].close), x2: CHART_WIDTH - margin.right, y2: y(points[lastIndex].close), class: "chart-last-guide" }));
  svg.append(element("circle", { cx: x(lastIndex), cy: y(points[lastIndex].close), r: 4.5, class: "chart-last-point" }));
  return true;
}

export function renderCandlestickVolumeChart(svg, bars = [], options = {}) {
  svg.replaceChildren();
  svg.setAttribute("viewBox", "0 0 760 300");
  const points = normalizeDailyBars(bars).filter((point) =>
    point.open !== null && point.high !== null && point.low !== null && point.close !== null &&
    point.high >= point.low && point.high >= Math.max(point.open, point.close) &&
    point.low <= Math.min(point.open, point.close)
  );
  if (!points.length) return false;

  const margin = { left: 58, right: 20 };
  const plotWidth = CHART_WIDTH - margin.left - margin.right;
  const priceTop = 17, priceBottom = 194, volumeTop = 222, volumeBottom = 260;
  const range = paddedRange(Math.min(...points.map((point) => point.low)), Math.max(...points.map((point) => point.high)), 0.08);
  const y = (value) => priceTop + ((range.max - value) / (range.max - range.min || 1)) * (priceBottom - priceTop);
  const x = (index) => margin.left + (points.length === 1 ? plotWidth / 2 : (index / (points.length - 1)) * plotWidth);
  const stepWidth = plotWidth / Math.max(1, points.length);
  const candleWidth = Math.min(13, Math.max(3, stepWidth * 0.58));

  for (let tick = 0; tick <= 4; tick += 1) {
    const value = range.max - ((range.max - range.min) * tick) / 4;
    const positionY = y(value);
    svg.append(element("line", { x1: margin.left, y1: positionY, x2: CHART_WIDTH - margin.right, y2: positionY, class: "chart-grid-line" }));
    svg.append(element("text", { x: margin.left - 10, y: positionY + 4, "text-anchor": "end", class: "chart-axis-label" }, value.toFixed(2)));
  }
  svg.append(element("line", { x1: margin.left, y1: 209, x2: CHART_WIDTH - margin.right, y2: 209, class: "chart-volume-divider" }));
  svg.append(element("text", { x: margin.left, y: volumeTop - 7, "text-anchor": "start", class: "chart-axis-label" }, options.volumeUnit ? "成交量（" + options.volumeUnit + "）" : "成交量"));

  const volumes = points.map((point) => point.volume).filter((value) => value !== null);
  const maxVolume = volumes.length ? Math.max(...volumes) : 0;
  if (maxVolume > 0) {
    points.forEach((point, index) => {
      if (point.volume === null || point.volume <= 0) return;
      const barHeight = Math.max(1, (point.volume / maxVolume) * (volumeBottom - volumeTop));
      const direction = point.close > point.open ? "up" : point.close < point.open ? "down" : "flat";
      svg.append(element("rect", {
        x: x(index) - candleWidth / 2, y: volumeBottom - barHeight,
        width: candleWidth, height: barHeight, class: "chart-volume-bar chart-volume-" + direction,
        "data-date": point.date
      }));
    });
  } else {
    svg.append(element("text", { x: margin.left + 7, y: volumeTop + 18, class: "chart-volume-unavailable" }, "成交量未报告"));
  }

  points.forEach((point, index) => {
    const centerX = x(index);
    const direction = point.close > point.open ? "up" : point.close < point.open ? "down" : "flat";
    const tooltip = candleTitle(point, options);
    const group = element("g", {
      class: "chart-candle chart-candle-" + direction, tabindex: "0", role: "graphics-symbol",
      "aria-label": tooltip, "data-date": point.date
    });
    group.append(element("title", {}, tooltip));
    group.append(element("line", { x1: centerX, y1: y(point.high), x2: centerX, y2: y(point.low), class: "chart-candle-wick" }));
    const bodyTop = Math.min(y(point.open), y(point.close));
    const equalBody = point.open === point.close;
    const bodyHeight = Math.max(2, Math.abs(y(point.open) - y(point.close)));
    group.append(element("rect", {
      x: centerX - candleWidth / 2, y: bodyTop - (equalBody ? 1 : 0),
      width: candleWidth, height: bodyHeight, class: "chart-candle-body"
    }));
    svg.append(group);
  });
  addDateLabels(svg, points, x, 289);
  return true;
}

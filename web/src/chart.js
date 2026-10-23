const SVG_NS = "http://www.w3.org/2000/svg";

function element(name, attributes = {}, text = "") {
  const node = document.createElementNS(SVG_NS, name);
  Object.entries(attributes).forEach(([key, value]) => node.setAttribute(key, String(value)));
  if (text !== "") node.textContent = text;
  return node;
}

export function renderCloseChart(svg, bars = []) {
  svg.replaceChildren();
  const points = bars
    .filter((bar) => Number.isFinite(Number(bar.close)) && bar.trade_date)
    .map((bar) => ({ date: String(bar.trade_date).slice(0, 10), close: Number(bar.close) }));
  if (points.length < 2) return false;

  const width = 760;
  const height = 260;
  const margin = { top: 16, right: 20, bottom: 34, left: 58 };
  const plotWidth = width - margin.left - margin.right;
  const plotHeight = height - margin.top - margin.bottom;
  const values = points.map((point) => point.close);
  const rawMin = Math.min(...values);
  const rawMax = Math.max(...values);
  const spread = Math.max(rawMax - rawMin, Math.abs(rawMax) * 0.015, 0.01);
  const min = Math.max(0, rawMin - spread * 0.12);
  const max = rawMax + spread * 0.12;
  const x = (index) => margin.left + (index / (points.length - 1)) * plotWidth;
  const y = (value) => margin.top + ((max - value) / (max - min || 1)) * plotHeight;

  const gradient = element("linearGradient", { id: "close-area-gradient", x1: "0", y1: "0", x2: "0", y2: "1" });
  gradient.append(element("stop", { offset: "0%", "stop-color": "#43d6ae", "stop-opacity": "0.22" }));
  gradient.append(element("stop", { offset: "100%", "stop-color": "#43d6ae", "stop-opacity": "0" }));
  const defs = element("defs");
  defs.append(gradient);
  svg.append(defs);

  for (let tick = 0; tick <= 4; tick += 1) {
    const value = max - ((max - min) * tick) / 4;
    const positionY = y(value);
    svg.append(element("line", { x1: margin.left, y1: positionY, x2: width - margin.right, y2: positionY, class: "chart-grid-line" }));
    svg.append(element("text", { x: margin.left - 10, y: positionY + 4, "text-anchor": "end", class: "chart-axis-label" }, value.toFixed(2)));
  }

  const path = points.map((point, index) => `${index ? "L" : "M"} ${x(index).toFixed(2)} ${y(point.close).toFixed(2)}`).join(" ");
  const area = `${path} L ${x(points.length - 1).toFixed(2)} ${(height - margin.bottom).toFixed(2)} L ${x(0).toFixed(2)} ${(height - margin.bottom).toFixed(2)} Z`;
  svg.append(element("path", { d: area, fill: "url(#close-area-gradient)" }));
  svg.append(element("path", { d: path, class: "chart-close-line" }));

  const sampleIndexes = [...new Set([0, Math.floor((points.length - 1) / 3), Math.floor(((points.length - 1) * 2) / 3), points.length - 1])];
  sampleIndexes.forEach((index) => {
    const label = points[index].date.slice(5).replace("-", ".");
    svg.append(element("text", { x: x(index), y: height - 10, "text-anchor": index === 0 ? "start" : index === points.length - 1 ? "end" : "middle", class: "chart-axis-label" }, label));
  });

  const lastIndex = points.length - 1;
  svg.append(element("line", { x1: x(lastIndex), y1: y(points[lastIndex].close), x2: width - margin.right, y2: y(points[lastIndex].close), class: "chart-last-guide" }));
  svg.append(element("circle", { cx: x(lastIndex), cy: y(points[lastIndex].close), r: 4.5, class: "chart-last-point" }));
  return true;
}

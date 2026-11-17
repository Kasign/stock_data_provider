import test from "node:test";
import assert from "node:assert/strict";
import { calculateTechnicalIndicators, normalizeDailyBars, renderCandlestickVolumeChart, renderCloseChart, renderMACDChart, renderRSIChart } from "../src/chart.js";

class FakeSvgNode {
  constructor(name) { this.name = name; this.attributes = {}; this.children = []; this.textContent = ""; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] ?? null; }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = [...nodes]; }
}
function withFakeDocument(run) {
  const original = globalThis.document;
  globalThis.document = { createElementNS(_namespace, name) { return new FakeSvgNode(name); } };
  try { return run(); } finally { globalThis.document = original; }
}
function flatten(node) { return [node, ...(node.children || []).flatMap(flatten)]; }

const sampleBars = [
  { trade_date: "2026-10-22", open: 102, high: 106, low: 100, close: 104, volume: 120000, amount: 12480000, change_percent: 1.96, adjustment_basis: "forward_adjusted", source: "fixture-provider", volume_unit: "shares", amount_unit: "CNY", quality_status: "valid", quality_reasons: [] },
  { trade_date: "2026-10-20", open: 100, high: 103, low: 98, close: 101, volume: 90000, amount: 9090000, change_percent: 1, adjustment_basis: "forward_adjusted", source: "fixture-provider", volume_unit: "shares", amount_unit: "CNY", quality_status: "valid", quality_reasons: [] },
  { trade_date: "2026-10-21", open: 101, high: 104, low: 99, close: 100, volume: null, amount: null, change_percent: null, adjustment_basis: "forward_adjusted", source: "fixture-provider", volume_unit: "shares", amount_unit: "CNY", quality_status: "degraded", quality_reasons: ["volume_missing"] }
];

test("normalizes bars in ascending date order and preserves absent values as null", () => {
  const normalized = normalizeDailyBars(sampleBars);
  assert.deepEqual(normalized.map((item) => item.date), ["2026-10-20", "2026-10-21", "2026-10-22"]);
  assert.equal(normalized[1].volume, null);
  assert.equal(normalized[1].amount, null);
  assert.equal(normalized[1].changePercent, null);
  assert.equal(normalizeDailyBars([{ trade_date: "2026-10-20", close: null }])[0].close, null);
});

test("close-line chart does not turn missing close into zero and provides hover titles", () => {
  withFakeDocument(() => {
    const svg = new FakeSvgNode("svg");
    assert.equal(renderCloseChart(svg, [
      { trade_date: "2026-10-22", close: 104 },
      { trade_date: "2026-10-21", close: null },
      { trade_date: "2026-10-20", close: 101 }
    ]), true);
    assert.equal(svg.getAttribute("viewBox"), "0 0 760 260");
    const nodes = flatten(svg);
    assert.equal(nodes.filter((node) => node.name === "circle" && node.getAttribute("class") === "chart-line-hit").length, 2);
    assert.ok(nodes.some((node) => node.name === "title" && node.textContent.includes("2026-10-20")));
  });
});

test("renders OHLC bodies, wicks, volume bars and per-candle hover detail", () => {
  withFakeDocument(() => {
    const svg = new FakeSvgNode("svg");
    assert.equal(renderCandlestickVolumeChart(svg, sampleBars, { adjustmentBasis: "forward_adjusted", volumeUnit: "shares", amountUnit: "CNY" }), true);
    assert.equal(svg.getAttribute("viewBox"), "0 0 760 300");
    const nodes = flatten(svg);
    assert.equal(nodes.filter((node) => node.name === "g" && (node.getAttribute("class") || "").startsWith("chart-candle ")).length, 3);
    assert.equal(nodes.filter((node) => node.name === "rect" && node.getAttribute("class") === "chart-candle-body").length, 3);
    assert.equal(nodes.filter((node) => node.name === "line" && node.getAttribute("class") === "chart-candle-wick").length, 3);
    assert.equal(nodes.filter((node) => node.name === "rect" && (node.getAttribute("class") || "").startsWith("chart-volume-bar ")).length, 2);
    assert.ok(nodes.some((node) => node.name === "title" && node.textContent.includes("开盘 100.00")));
    assert.ok(nodes.some((node) => node.name === "title" && node.textContent.includes("成交量 未报告")));
    assert.ok(nodes.some((node) => node.name === "text" && node.textContent === "成交量（shares）"));
  });
});

test("does not silently repair internally inconsistent OHLC bars", () => {
  withFakeDocument(() => {
    const svg = new FakeSvgNode("svg");
    assert.equal(renderCandlestickVolumeChart(svg, [{ trade_date: "2026-10-20", open: 100, high: 99, low: 98, close: 101, volume: 500 }]), false);
  });
});

test("can draw OHLC when volume is absent without inventing zero volume", () => {
  withFakeDocument(() => {
    const svg = new FakeSvgNode("svg");
    assert.equal(renderCandlestickVolumeChart(svg, [{ trade_date: "2026-10-20", open: 100, high: 103, low: 98, close: 101, volume: null }]), true);
    const nodes = flatten(svg);
    assert.equal(nodes.filter((node) => node.name === "rect" && (node.getAttribute("class") || "").startsWith("chart-volume-bar ")).length, 0);
    assert.ok(nodes.some((node) => node.name === "text" && node.textContent === "成交量未报告"));
    assert.ok(nodes.some((node) => node.name === "title" && node.textContent.includes("成交量 未报告")));
  });
});

function risingBars(count,start=100) {
  return Array.from({length:count},(_,index)=>{
    const tradeDate=new Date(Date.UTC(2026,0,1+index)).toISOString().slice(0,10);
    const close=start+index;
    return {trade_date:tradeDate,open:close-0.5,high:close+1,low:close-1,close,volume:1000+index,amount:(1000+index)*close,adjustment_basis:"forward_adjusted",quality_status:"valid"};
  });
}
test("computes moving averages, RSI and MACD only after sufficient valid close samples",()=>{
  const snapshot=calculateTechnicalIndicators(risingBars(40));
  assert.equal(snapshot.validCloseCount,40);
  assert.equal(snapshot.latest.ma5,137);
  assert.equal(snapshot.latest.ma20,129.5);
  assert.equal(snapshot.latest.ma60,null);
  assert.equal(snapshot.latest.rsi14,100);
  assert.ok(Number.isFinite(snapshot.latest.macdDif));
  assert.ok(Number.isFinite(snapshot.latest.macdSignal));
  assert.ok(Number.isFinite(snapshot.latest.macdHistogram));
  assert.equal(snapshot.requiredSamples.rsi14,15);
  assert.equal(snapshot.requiredSamples.macdSignal,34);
});
test("does not turn missing daily closes into zero for technical indicators",()=>{
  const bars=risingBars(16); bars[7].close=null;
  const snapshot=calculateTechnicalIndicators(bars);
  assert.equal(snapshot.validCloseCount,15);
  assert.equal(snapshot.rows[7].close,null);
  assert.equal(snapshot.rows[7].ma5,null);
  assert.ok(Number.isFinite(snapshot.latest.rsi14));
});
test("moving-average overlays are rendered only for selected periods",()=>{
  withFakeDocument(()=>{
    const svg=new FakeSvgNode("svg"),bars=risingBars(25),rows=calculateTechnicalIndicators(bars).rows;
    assert.equal(renderCloseChart(svg,bars,{movingAverages:[5,20],indicatorRows:rows}),true);
    const classes=flatten(svg).map((node)=>node.attributes.class||"");
    assert.ok(classes.some((value)=>value.includes("chart-moving-average-5")));
    assert.ok(classes.some((value)=>value.includes("chart-moving-average-20")));
    assert.ok(!classes.some((value)=>value.includes("chart-moving-average-60")));
  });
});
test("RSI and MACD charts render calculated rows and report insufficient samples",()=>{
  withFakeDocument(()=>{
    const rows=calculateTechnicalIndicators(risingBars(40)).rows,rsiSvg=new FakeSvgNode("svg"),macdSvg=new FakeSvgNode("svg");
    assert.equal(renderRSIChart(rsiSvg,rows),true);
    assert.equal(renderMACDChart(macdSvg,rows),true);
    assert.ok(flatten(rsiSvg).some((node)=>(node.attributes.class||"").includes("technical-rsi-line")));
    assert.ok(flatten(macdSvg).some((node)=>(node.attributes.class||"").includes("technical-macd-bar")));
    assert.equal(renderRSIChart(rsiSvg,calculateTechnicalIndicators(risingBars(10)).rows),false);
    assert.equal(renderMACDChart(macdSvg,calculateTechnicalIndicators(risingBars(30)).rows),false);
  });
});

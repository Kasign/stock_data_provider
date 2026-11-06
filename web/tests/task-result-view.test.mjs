import test from "node:test";
import assert from "node:assert/strict";
import { renderTaskResultView } from "../src/task-result-view.js";

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName;
    this.children = [];
    this.dataset = {};
    this.attributes = {};
    this.hidden = false;
    this.textContent = "";
    this.className = "";
    this.title = "";
  }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  setAttribute(name, value) { this.attributes[name] = value; }
}

class FakeDocument {
  constructor() {
    this.elements = new Map();
    for (const selector of [
      "#task-result-metrics", "#task-result-structured", "#task-result-warnings",
      "#task-result-json", "#task-result-note", "#task-result-section"
    ]) this.elements.set(selector, new FakeElement("root"));
  }
  querySelector(selector) {
    const element = this.elements.get(selector);
    if (!element) throw new Error("Missing selector: " + selector);
    return element;
  }
  createElement(tagName) { return new FakeElement(tagName); }
}

function descendants(node) {
  return [node, ...node.children.flatMap(descendants)];
}

function render(result) {
  const document = new FakeDocument();
  renderTaskResultView({
    document,
    result,
    fallbackRunId: "fallback-run",
    runStatusLabel: status => "状态：" + (status || "未知")
  });
  return document;
}

test("Run result view renders all report sections and preserves server values", () => {
  const result = {
    run_id: "run-2026-01",
    status: "completed",
    request: { symbols: ["600000.SH"], task_type: "daily_stock_analysis", market_id: "CN" },
    started_at: "2026-11-04T09:30:00+08:00",
    quality: {
      core_pass: false, partial: true, checked_at: "2026-11-04",
      strategy_eligible: false, strategy_blockers: ["missing-data"],
      policy_version: "v2", issues: [{ severity: "high", code: "missing-data", message: "数据缺失" }]
    },
    market_state: { regime: "震荡", metrics: { breadth: 0.4 }, evidence: ["server"] },
    factors: [{ factor_id: "momentum", value: 1.25, unit: "score" }],
    signals: [{ symbol: "600000.SH", direction: "long", strategy_id: "trend" }],
    plans: [],
    warnings: ["服务端警告"],
    tradability: [{ allowed: false, reasons: ["不可交易"] }],
    risk: [{ allowed: true, reasons: ["通过"] }],
    planning_rejections: [{ signal_id: "signal-1", reasons: ["质量门禁未通过"] }],
    execution_assessment: { model_version: "v1", warnings: [], orders: [] },
    failure: { code: "partial-run", message: "部分结果" }
  };
  const document = render(result);
  const metrics = document.querySelector("#task-result-metrics");
  const structured = document.querySelector("#task-result-structured");
  const allText = descendants(structured).map(node => node.textContent).join("|");
  assert.equal(metrics.children.length, 7);
  assert.match(allText, /本次分析概况/);
  assert.match(allText, /市场状态/);
  assert.match(allText, /数据质量与策略门禁/);
  assert.match(allText, /分析因子/);
  assert.match(allText, /候选信号/);
  assert.match(allText, /可交易性检查/);
  assert.match(allText, /风险检查/);
  assert.match(allText, /交易计划/);
  assert.match(allText, /计划拒绝原因/);
  assert.match(allText, /执行评估/);
  assert.match(allText, /运行失败信息/);
  assert.match(allText, /失败详情/);
  assert.equal((allText.match(/部分结果/g) || []).length, 1, "failure message appears once in the structured failure card");
  assert.equal(document.querySelector("#task-result-warnings").hidden, false);
  assert.equal(document.querySelector("#task-result-warnings").children[0].textContent, "服务端警告");
  assert.equal(document.querySelector("#task-result-json").textContent, JSON.stringify(result, null, 2));
  assert.match(document.querySelector("#task-result-note").textContent, /run-2026-01/);
  assert.equal(document.querySelector("#task-result-section").hidden, false);
});

test("Run result view distinguishes missing arrays and missing values without inventing data", () => {
  const result = { run_id: "run-empty", status: "unknown", quality: {} };
  const document = render(result);
  const structured = document.querySelector("#task-result-structured");
  const text = descendants(structured).map(node => node.textContent).join("|");
  assert.match(text, /本次 Run 未返回 market_state/);
  assert.match(text, /本次 Run 未返回分析因子/);
  assert.match(text, /本次 Run 未生成候选信号/);
  assert.match(text, /没有返回交易计划/);
  assert.match(text, /没有返回订单执行评估记录/);
  assert.equal(document.querySelector("#task-result-warnings").hidden, true);
  assert.match(document.querySelector("#task-result-note").textContent, /run-empty/);
});

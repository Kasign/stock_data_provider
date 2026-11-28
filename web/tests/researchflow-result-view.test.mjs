import test from "node:test";
import assert from "node:assert/strict";
import { renderResearchflowResultsView } from "../src/researchflow-result-view.js";

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName;
    this.children = [];
    this.dataset = {};
    this.attributes = {};
    this.hidden = false;
    this.textContent = "";
    this.className = "";
  }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  setAttribute(name, value) { this.attributes[name] = value; }
  removeAttribute(name) { delete this.attributes[name]; }
}

class FakeDocument {
  constructor() {
    this.elements = new Map();
    for (const selector of [
      "#task-result-metrics", "#task-result-structured", "#task-result-warnings",
      "#task-result-note", "#task-result-section"
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

function allText(doc) {
  return descendants(doc.querySelector("#task-result-structured"))
    .map((node) => node.textContent).join("\n");
}

function sampleView(overrides = {}) {
  return {
    isFullReport: false,
    resourceKind: "authorized_domain_results",
    runRef: "run-abc-1",
    items: [
      {
        resultType: "indicator", resultId: "r-ind", resultStatus: "succeeded",
        qualityStatus: "valid", scope: { symbol: "600519.SH" }, systemAsOf: "2026-11-27",
        authorizationRef: "auth-1", hasPayload: true,
        payload: { indicator_code: "sma20", value: 1680.5 }
      },
      {
        resultType: "factor", resultId: "r-fac", resultStatus: "succeeded",
        qualityStatus: "valid", scope: { symbol: "600519.SH" }, systemAsOf: "2026-11-27",
        authorizationRef: "auth-2", hasPayload: true,
        payload: { factor_code: "momentum_5d", value: 0.032 }
      },
      {
        resultType: "trend", resultId: "r-tre", resultStatus: "succeeded",
        qualityStatus: "valid", scope: { symbol: "600519.SH" }, systemAsOf: "2026-11-27",
        authorizationRef: "auth-3", hasPayload: true,
        payload: { direction: "up", strength: 0.6 }
      },
      {
        resultType: "risk", resultId: "r-rsk", resultStatus: "succeeded",
        qualityStatus: "restricted", scope: { symbol: "600519.SH" }, systemAsOf: "2026-11-27",
        authorizationRef: "auth-4", hasPayload: false, payload: undefined
      }
    ],
    ...overrides
  };
}

function render(view, runStatus = "succeeded") {
  const document = new FakeDocument();
  renderResearchflowResultsView({ document, view, runStatusLabel: () => runStatus });
  return document;
}

test("降级结果渲染器明确标注非完整报告，不冒充研究报告", () => {
  const doc = render(sampleView());
  const text = allText(doc);
  assert.match(text, /授权领域结果/);
  assert.match(text, /非完整研究报告/);
  assert.doesNotMatch(text, /完整研究报告\s*正文/);
});

test("降级结果渲染器渲染四类结果项为明细卡，risk 项标注无 payload", () => {
  const doc = render(sampleView());
  const structured = doc.querySelector("#task-result-structured");
  const cards = descendants(structured).filter((node) => node.className === "analysis-result-card");
  assert.equal(cards.length, 4);
  const texts = cards.map((card) => descendants(card).map((n) => n.textContent).join(""));
  assert.ok(texts.some((t) => t.includes("指标") && t.includes("sma20")), "indicator 项含指标代码");
  assert.ok(texts.some((t) => t.includes("因子") && t.includes("momentum_5d")), "factor 项含因子代码");
  assert.ok(texts.some((t) => t.includes("趋势") && t.includes("up")), "trend 项含方向");
  assert.ok(texts.some((t) => t.includes("风险") && t.includes("仅状态")), "risk 项标注仅状态");
  const metrics = descendants(doc.querySelector("#task-result-metrics")).map((n) => n.textContent).join("");
  assert.match(metrics, /4/, "固定指标区展示结果项数");
});

test("降级结果渲染器展示 run_ref 与授权信息，不泄露 risk payload", () => {
  const doc = render(sampleView());
  const structured = allText(doc);
  assert.match(structured, /run-abc-1/);
  assert.match(structured, /authorized_domain_results/);
  assert.doesNotMatch(structured, /risk.{0,40}\{/, "risk 项不得渲染 payload 对象");
});

test("降级结果渲染器对空 items 给出明确空态而非伪造数据", () => {
  const doc = render(sampleView({ items: [] }));
  const structured = allText(doc);
  assert.match(structured, /暂无已授权的领域结果/);
  const cards = descendants(doc.querySelector("#task-result-structured"))
    .filter((node) => node.className === "analysis-result-card");
  assert.equal(cards.length, 0);
});

test("降级结果渲染器只接受降级视图，拒绝完整报告视图", () => {
  assert.throws(() => render(sampleView({ isFullReport: true })), /完整报告/);
  assert.throws(() => render(sampleView({ resourceKind: "full_report" })), /resource_kind/);
});

test("降级结果渲染器对 item payload 结构异常 fail-closed", () => {
  const view = sampleView();
  view.items[0].payload = "not-an-object";
  assert.throws(() => render(view), /payload/);
});

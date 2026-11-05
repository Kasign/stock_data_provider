import test from "node:test";
import assert from "node:assert/strict";
import {
  normalizeStockSearchItems,
  renderStockSearchResultsView,
  updateStockSearchActiveOptionView
} from "../src/stock-search-view.js";

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName;
    this.children = [];
    this.dataset = {};
    this.attributes = {};
    this.className = "";
    this.id = "";
    this.type = "";
    this.textContent = "";
    this.scrollCalls = [];
  }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  setAttribute(name, value) { this.attributes[name] = value; }
  getAttribute(name) { return this.attributes[name] ?? null; }
  querySelectorAll(selector) {
    return selector === "[data-stock-search-index]" ? this.children.filter(node => node.dataset.stockSearchIndex !== undefined) : [];
  }
  querySelector(selector) {
    if (selector !== '[aria-selected="true"]') return null;
    return this.children.find(node => node.attributes["aria-selected"] === "true") || null;
  }
  scrollIntoView(options) { this.scrollCalls.push(options); }
}
class FakeDocument {
  createElement(tagName) { return new FakeElement(tagName); }
}

const valid = [
  { symbol: "600000.SH", code: "600000", name: "浦发银行", exchange: "上交所" },
  { symbol: "000001.SZ", code: "000001", name: "平安银行", exchange: "深交所" }
];

test("normalizes stock search results without accepting malformed records", () => {
  assert.deepEqual(normalizeStockSearchItems(valid.concat([null, {}, { symbol: "x", code: 1, name: "bad", exchange: "?" }])), valid);
  assert.deepEqual(normalizeStockSearchItems(null), []);
});

test("renders accessible stock options with generation-scoped IDs and text-only values", () => {
  const document = new FakeDocument(), root = new FakeElement("root");
  renderStockSearchResultsView({ document, root, items: valid, generation: 17 });
  assert.equal(root.children.length, 2);
  assert.equal(root.children[0].id, "stock-search-option-17-0");
  assert.equal(root.children[1].id, "stock-search-option-17-1");
  assert.equal(root.children[0].attributes.role, "option");
  assert.equal(root.children[0].children[0].children[0].textContent, "浦发银行");
  assert.equal(root.children[0].children[0].children[1].textContent, "600000.SH");
  assert.equal(root.children[0].children[1].textContent, "上交所");
  assert.equal(root.children[0].attributes["aria-selected"], "false");
  assert.deepEqual(root.children[0].children[0].children[0].children, []);
});

test("updates active descendant, selected option and nearest scrolling", () => {
  const input = new FakeElement("input"), root = new FakeElement("root");
  renderStockSearchResultsView({ document: new FakeDocument(), root, items: valid, generation: 2 });
  updateStockSearchActiveOptionView({ input, root, activeIndex: 1 });
  assert.equal(root.children[0].attributes["aria-selected"], "false");
  assert.equal(root.children[1].attributes["aria-selected"], "true");
  assert.equal(input.attributes["aria-activedescendant"], "stock-search-option-2-1");
  assert.deepEqual(root.children[1].scrollCalls, [{ block: "nearest" }]);
  updateStockSearchActiveOptionView({ input, root, activeIndex: -1 });
  assert.equal(input.attributes["aria-activedescendant"], undefined);
  assert.equal(root.children.every(node => node.attributes["aria-selected"] === "false"), true);
});

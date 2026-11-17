import test from "node:test";
import assert from "node:assert/strict";
import { renderScreeningLocalResultRowsView } from "../src/screening-result-view.js";

class FakeNode {
  constructor(tag) { this.tagName = tag; this.children = []; this.dataset = {}; this.attributes = {}; this.textContent = ""; }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = [...nodes]; }
  setAttribute(key, value) { this.attributes[key] = value; }
}
function fakeDocument() {
  return { createElement: tag => new FakeNode(tag), createTextNode: text => ({ textContent: text, tagName: "#text" }) };
}
const formatters = {
  formatPercent: value => value == null ? "—" : String(value) + "%",
  formatCompact: value => String(value),
  formatPrice: value => String(value),
  finiteNumberOrNull: value => Number.isFinite(Number(value)) && value !== null && value !== "" ? Number(value) : null,
  qualityLabel: value => "质量:" + value
};

test("builds safe result rows with selection and stock navigation data attributes", () => {
  const body = new FakeNode("tbody"), item = {
    symbol: "600000.SH", name: "浦发银行",
    quote: { change_percent: 1.2, turnover_rate: 2, amount: 1200000, amount_unit: "CNY" },
    ma5: 10.2, ma20: 9.8, quality_status: "valid", quality_reasons: ["字段完整"]
  };
  const result = renderScreeningLocalResultRowsView({
    document: fakeDocument(), body, rows: [{ item, status: "命中" }], filter: "matched",
    snapshot: { page: 2 }, ...formatters
  });
  assert.equal(result.rowCount, 1);
  const row = body.children[0];
  assert.equal(row.tagName, "tr");
  const checkbox = row.children[0].children[0];
  assert.equal(checkbox.dataset.screeningLocalSelectSymbol, "600000.SH");
  assert.equal(checkbox.disabled, false);
  const stockButton = row.children[1].children[0];
  assert.equal(stockButton.dataset.screeningLocalSymbol, "600000.SH");
  assert.equal(stockButton.dataset.screeningLocalName, "浦发银行");
  assert.equal(row.children[2].textContent, "1.2%");
  assert.equal(row.children[4].textContent, "1200000 元");
  assert.equal(row.children[7].title, "字段完整");
  assert.equal(row.children[8].textContent, "命中");
});

test("disables selection for invalid symbols and preserves missing quality reasons", () => {
  const body = new FakeNode("tbody");
  renderScreeningLocalResultRowsView({
    document: fakeDocument(), body, rows: [{ item: { symbol: "bad", quote: {} }, status: "待判断" }],
    filter: "unknown", snapshot: { page: 1 }, ...formatters
  });
  const row = body.children[0];
  assert.equal(row.children[0].children[0].disabled, true);
  assert.equal(row.children[7].textContent, "质量:unknown");
  assert.equal(row.children[7].children[1].textContent, "质量原因未报告");
});

test("renders filter-specific empty state without fabricating rows", () => {
  const body = new FakeNode("tbody");
  const result = renderScreeningLocalResultRowsView({
    document: fakeDocument(), body, rows: [], filter: "unknown", snapshot: { page: 1 }, ...formatters
  });
  assert.equal(result.rowCount, 0);
  assert.equal(body.children.length, 1);
  assert.equal(body.children[0].children[0].colSpan, 9);
  assert.match(body.children[0].children[0].textContent, /待判断的股票/);
});

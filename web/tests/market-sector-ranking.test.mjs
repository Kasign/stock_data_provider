import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [overview, html, apiSource] = await Promise.all([
  readFile(new URL("../src/market-overview.js", import.meta.url), "utf8"),
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/api.js", import.meta.url), "utf8")
]);

test("板块排行接入服务端成交额、五日涨幅和排序方向", () => {
  assert.match(apiSource, /direction = "up", sortBy = "change_percent"/);
  assert.match(apiSource, /sort_by:String\(sortBy\)/);
  assert.match(overview, /sectorMetricSortBy/);
  assert.match(overview, /change_percent_5d/);
  assert.match(overview, /item\.amount_unit==="CNY"/);
  assert.match(overview, /data-sector-direction/);
  assert.match(html, /data-sector-metric="amount"/);
  assert.match(html, /data-sector-metric="five-day"/);
  assert.match(html, /data-sector-direction="down"/);
});

test("板块资金净流入仍明确标记为未接入，不以成交额代替", () => {
  assert.match(overview, /板块资金净流入需要真实板块级资金流接口/);
  assert.match(overview, /不会用成交额替代资金净流入/);
});

import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [overview, html, apiSource, styles] = await Promise.all([
  readFile(new URL("../src/market-overview.js", import.meta.url), "utf8"),
  readFile(new URL("../index.html", import.meta.url), "utf8"),
  readFile(new URL("../src/api.js", import.meta.url), "utf8"),
  readFile(new URL("../styles.css", import.meta.url), "utf8")
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
  assert.match(html, /id="market-sector-limit"/);
  assert.match(html, /value="10" selected/);
  assert.match(html, /value="20"/);
  assert.match(html, /value="50"/);
  assert.match(html, /value="100"/);
  assert.match(overview, /sectorLimit=10/);
  assert.match(overview, /limit:sectorLimit/);
  assert.match(overview, /请求上限/);
  assert.match(overview, /实际返回/);
  assert.match(styles, /\.market-sector-limit-control/);
  assert.match(styles, /\.market-sector-limit-control select:focus-visible/);
});

test("板块资金净流入仍明确标记为未接入，不以成交额代替", () => {
  assert.match(overview, /板块资金净流入需要真实板块级资金流接口/);
  assert.match(overview, /不会用成交额替代资金净流入/);
});

test("近五日涨幅字段与服务端 Go JSON 契约一致", async () => {
  const model = await readFile(new URL("../../services/market-data/internal/model/model.go", import.meta.url), "utf8");
  assert.match(model, /ChangePercent5D\s+\*float64\s+`json:"change_percent_5d,omitempty"`/);
});


test("人气榜和飙升榜支持服务端结果数量选择", () => {
  assert.match(html, /id="market-hot-stocks-limit"/);
  assert.match(html, /value="20" selected/);
  assert.match(overview, /hotLimit=20/);
  assert.match(overview, /limit:hotLimit/);
  assert.match(overview, /function renderHotStocks\(data,requestedLimit=20\)/);
  assert.match(overview, /renderHotStocks\(data,hotLimit\)/);
  assert.match(overview, /请求上限 "\+count\(requestedLimit\)/);
  assert.match(overview, /请求上限/);
  assert.match(overview, /实际返回/);
  assert.match(overview, /hotLimit=\[10,20,50,100\]\.includes\(value\)\?value:20/);
});

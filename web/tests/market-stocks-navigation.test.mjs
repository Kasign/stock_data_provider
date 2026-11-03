import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [marketStocks, app] = await Promise.all([
  readFile(new URL("../src/market-stocks.js", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8")
]);

test("全市场分页行情同时提供行情详情和深度研究入口", () => {
  assert.match(marketStocks, /\["stocks", "查看行情"\]/);
  assert.match(marketStocks, /\["research", "深度研究"\]/);
  assert.match(marketStocks, /data\.marketStockAction === "research" \? "research" : "stocks"/);
  assert.match(app, /function selectStockFromMarket\(item, source = "市场榜单", targetView = "stocks"\)/);
  assert.match(app, /setView\(targetView === "research" \? "research" : "stocks"\)/);
});

test("全市场入口继续通过统一行情查询建立研究上下文", () => {
  assert.match(app, /stockResearch\.syncSymbol\(\);\s*setText\("#query-hint"/);
  assert.match(app, /void runQuoteQuery\(\);/);
});

import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [marketStocks, app, browserAcceptance] = await Promise.all([
  readFile(new URL("../src/market-stocks.js", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("./stock-research-browser.mjs", import.meta.url), "utf8")
]);

test("全市场分页行情同时提供行情详情和深度研究入口", () => {
  assert.match(marketStocks, /\["stocks", "查看行情"\]/);
  assert.match(marketStocks, /\["research", "深度研究"\]/);
  assert.match(marketStocks, /button\.dataset\.marketStockAction === "research" \? "research" : "stocks"/);
  assert.match(app, /function selectStockFromMarket\(item, source = "市场榜单", targetView = "stocks"\)/);
  assert.match(app, /setView\(targetView === "research" \? "research" : "stocks"\)/);
});

test("Chromium 专项覆盖全市场直达研究入口", () => {
  assert.match(browserAcceptance, /button\[data-market-stock-action="research"\]/);
  assert.match(browserAcceptance, /#research-overview-symbol/);
});

test("市场人气榜、涨跌停池和龙虎榜提供独立深度研究入口", async () => {
  const [overview, activity] = await Promise.all([
    readFile(new URL("../src/market-overview.js", import.meta.url), "utf8"),
    readFile(new URL("../src/market-activity.js", import.meta.url), "utf8")
  ]);
  assert.match(overview, /data-hot-action="research"|data\.hotAction="research"/);
  assert.match(overview, /data-pool-action|poolAction/);
  assert.match(overview, /targetView=button\.dataset\.hotAction==="research"\?"research":"stocks"/);
  assert.match(activity, /data-dragon-action|dragonAction/);
  assert.match(activity, /b\.dataset\.dragonAction==="research"\?"research":"stocks"/);
  assert.match(app, /\(item, source, targetView\) => selectStockFromMarket\(item, source, targetView\)/);
});

test("自选股列表提供直达研究入口并同步标的名称", async () => {
  const appSource = await readFile(new URL("../src/app.js", import.meta.url), "utf8");
  assert.match(appSource, /\["query", "查行情"\], \["research", "研究"\]/);
  assert.match(appSource, /action === "query" \|\| action === "research"/);
  assert.match(appSource, /setView\(action === "research" \? "research" : "stocks"\)/);
  assert.match(browserAcceptance, /button\[data-watchlist-action="research"\]/);
});

test("全市场入口继续通过统一行情查询建立研究上下文", () => {
  assert.match(app, /stockResearch\.syncSymbol\(\);\s*setText\("#query-hint"/);
  assert.match(app, /void runQuoteQuery\(\);/);
});

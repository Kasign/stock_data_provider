import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [marketStocks, app, browserAcceptance, screeningBrowserAcceptance] = await Promise.all([
  readFile(new URL("../src/market-stocks.js", import.meta.url), "utf8"),
  readFile(new URL("../src/app.js", import.meta.url), "utf8"),
  readFile(new URL("./stock-research-browser.mjs", import.meta.url), "utf8"),
  readFile(new URL("./screening-local-browser.mjs", import.meta.url), "utf8")
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
  assert.match(overview, /dataset\.hotAction="research"/);
  assert.match(overview, /dataset\.poolAction=target/);
  assert.match(overview, /button\.dataset\.hotAction==="research"\?"research":"stocks"/);
  assert.match(activity, /dataset\.dragonAction=target/);
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

test("市场入口拒绝无效证券代码，并统一规范化有效代码", () => {
  assert.match(app, /function selectStockFromMarket\(item, source = "市场榜单", targetView = "stocks"\) \{\s*let symbol = "";\s*try \{\s*symbol = normalizeSymbol\(item\?\.symbol \|\| ""\);/);
  assert.match(app, /市场入口未返回有效 A 股证券代码，未执行跳转/);
  assert.match(app, /const origin = String\(source \|\| "市场榜单"\)\.trim\(\) \|\| "市场榜单"/);
  assert.match(app, /state\.selectedStockSearchItem = \{ symbol, name \}/);
});

test("板块领涨股仅展示名称；API 未提供证券代码时不得伪造研究入口", async () => {
  const [overview, openApi] = await Promise.all([
    readFile(new URL("../src/market-overview.js", import.meta.url), "utf8"),
    readFile(new URL("../../services/market-data/openapi.yaml", import.meta.url), "utf8")
  ]);
  const sectorSchema = openApi.slice(openApi.indexOf("    SectorRanking:"), openApi.indexOf("    SectorRankingSnapshot:"));
  assert.match(sectorSchema, /leading_stock: \{type: string\}/);
  assert.doesNotMatch(sectorSchema, /leading_stock_(?:symbol|code)/);
  assert.match(overview, /cell\(tr,item\.leading_stock\|\|"—"\)/);
  assert.doesNotMatch(overview, /data-sector-leading-stock/);
});

test("全市场请求刷新期间使旧筛选快照失效", () => {
  assert.match(marketStocks, /hasResult: state\.hasResult, busy: state\.busy, generation: state\.generation/);
  assert.match(app, /!current\.busy && !snapshot\.busy/);
  assert.match(app, /current\.generation === snapshot\.generation/);
  assert.match(app, /全市场行情正在刷新，暂不能应用筛选条件/);
  assert.match(screeningBrowserAcceptance, /holdSecondMarketStocksResponse/);
  assert.match(screeningBrowserAcceptance, /旧快照不得继续用于筛选、导出或批量加入/);
});

test("全市场刷新保留旧快照时明确标记，连接切换后清空旧摘要和元数据", () => {
  assert.match(marketStocks, /正在刷新 · 保留上次成功快照/);
  assert.match(marketStocks, /表格暂时保留第 .* 页的上次成功快照/);
  assert.match(marketStocks, /旧连接下的全市场行情快照已清除/);
  assert.match(marketStocks, /旧连接下的分页行情元数据已失效/);
  assert.match(marketStocks, /state\.controller\?\.abort\(\)/);
  assert.match(marketStocks, /generation !== state\.generation/);
  assert.match(screeningBrowserAcceptance, /holdThirdMarketStocksResponse/);
  assert.match(screeningBrowserAcceptance, /旧连接下的分页行情元数据已失效/);
});

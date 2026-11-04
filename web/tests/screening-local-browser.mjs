import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(webRoot, "..");
const screenshotDir = path.join(repositoryRoot, "browser-screenshots", "screening-local");
const server = createServer(async (request, response) => {
  const pathname = decodeURIComponent(new URL(request.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) {
    response.writeHead(403); response.end("Forbidden"); return;
  }
  try {
    const body = await readFile(target), extension = path.extname(target);
    const contentType = extension === ".html" ? "text/html; charset=utf-8"
      : extension === ".css" ? "text/css; charset=utf-8"
        : [".js", ".mjs"].includes(extension) ? "text/javascript; charset=utf-8" : "application/octet-stream";
    response.writeHead(200, { "content-type": contentType, "cache-control": "no-store" }); response.end(body);
  } catch { response.writeHead(404); response.end("Not found"); }
});
await mkdir(screenshotDir, { recursive: true });
await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;
try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "screening-browser-key"
    }));
  });
  const page = await context.newPage(), pageErrors = [];
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  let marketStocksRequests = 0;
  let holdSecondMarketStocksResponse = false;
  let releaseSecondMarketStocksResponse;
  let markSecondMarketStocksStarted;
  const secondMarketStocksStarted = new Promise(resolve => { markSecondMarketStocksStarted = resolve; });
  const secondMarketStocksResponseGate = new Promise(resolve => { releaseSecondMarketStocksResponse = resolve; });
  await page.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/market-api/api/v1/market/stocks") {
      marketStocksRequests += 1;
      if (marketStocksRequests === 2 && holdSecondMarketStocksResponse) {
        markSecondMarketStocksStarted();
        await secondMarketStocksResponseGate;
      }
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
        items: [
          { symbol: "600001.SH", name: "测试命中", quality_status: "valid", quote: { symbol: "600001.SH", change_percent: 3, turnover_rate: 2, amount: 20000000, quality_status: "valid" }, ma5: 12, ma20: 11 },
          { symbol: "600002.SH", name: "测试未命中", quality_status: "valid", quote: { symbol: "600002.SH", change_percent: -1, turnover_rate: 5, amount: 5000000, quality_status: "valid" }, ma5: 8, ma20: 9 },
          { symbol: "600003.SH", name: "测试待判断", quality_status: "degraded", quality_reasons: ["MA5 字段缺失"], quote: { symbol: "600003.SH", change_percent: null, turnover_rate: 3, amount: 30000000, quality_status: "degraded", quality_reasons: ["行情涨跌幅未报告"] }, ma5: null, ma20: 10 }
        ],
        page: 1, page_size: 50, total: 3, total_is_lower_bound: false, coverage_ratio: 1,
        quote_succeeded: 3, quote_failed: 0, quality_status: "degraded",
        universe_source: "browser-fixture", universe_source_version: "test",
        universe_fetched_at: "2026-11-03T03:00:00Z", fetched_at: "2026-11-03T03:00:0" + marketStocksRequests + "Z",
        ma_adjustment_basis: "forward_adjusted"
      }) });
      return;
    }
    if (url.pathname.includes("/api/")) {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { message: "browser fixture unavailable" } }) });
      return;
    }
    await route.continue();
  });
  await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#load-market-stocks").click();
  await page.waitForFunction(() => document.querySelectorAll("#market-stocks-body tr").length === 3);
  await page.locator('.nav-link[data-view="screening"]').click();

  await page.locator("#screening-condition-field").selectOption("change_pct");
  await page.locator("#screening-condition-operator").selectOption("gte");
  await page.locator("#screening-condition-value").fill("1");
  await page.locator("#screening-condition-add").click();
  await page.locator("#screening-condition-field").selectOption("ma5");
  await page.locator("#screening-condition-value").fill("10");
  await page.locator("#screening-condition-add").click();
  await page.locator("#screening-apply-current-page").click();

  const summary = page.locator("#screening-local-results-summary");
  await page.waitForFunction(() => document.querySelector("#screening-local-results-summary")?.textContent.includes("命中 1 条"));
  assert.match(await summary.textContent(), /未命中 1 条/);
  assert.match(await summary.textContent(), /无法判断 1 条/);
  assert.match(await summary.textContent(), /第 1 页 \/ 每页 50 条/);
  assert.equal(await page.locator("#screening-local-results-body tr").count(), 1);
  assert.match(await page.locator("#screening-local-results-body").textContent(), /测试命中/);

  await page.locator("#screening-local-result-filter").selectOption("unknown");
  assert.match(await page.locator("#screening-local-results-body").textContent(), /测试待判断/);
  assert.match(await page.locator("#screening-local-results-body").textContent(), /MA5 字段缺失/);
  await page.locator("#screening-local-result-filter").selectOption("unmatched");
  assert.match(await page.locator("#screening-local-results-body").textContent(), /测试未命中/);
  await page.locator("#screening-local-result-filter").selectOption("all");
  assert.equal(await page.locator("#screening-local-results-body tr").count(), 3);
  assert.equal(marketStocksRequests, 1, "switching local result views must not re-fetch market data");
  const exportButton = page.locator("#screening-local-export-csv");
  assert.equal(await exportButton.isEnabled(), true);
  const [csvDownload] = await Promise.all([page.waitForEvent("download"), exportButton.click()]);
  assert.match(csvDownload.suggestedFilename(), /^当前页筛选-全部-\d{8}-\d{6}\.csv$/);
  const csvPath = await csvDownload.path();
  const csvContent = await readFile(csvPath, "utf8");
  assert.equal(csvContent.charCodeAt(0), 0xFEFF, "CSV must include UTF-8 BOM");
  assert.match(csvContent, /"测试命中"/);
  assert.match(csvContent, /"测试未命中"/);
  assert.match(csvContent, /"测试待判断"/);
  assert.match(csvContent, /MA5 字段缺失/);
  assert.match(csvContent, /"待判断"/);
  assert.equal(marketStocksRequests, 1, "CSV export must not re-fetch market data");

  // 批量加入只作用于当前命中视图；重复加入不重复插入，明确选组时才调整既有归属。
  await page.locator("#screening-local-result-filter").selectOption("matched");
  await page.locator("#screening-local-results-body input[data-screening-local-select-symbol]").check();
  assert.equal(await page.locator("#screening-local-select-count").textContent(), "已选择 1 只");
  await page.locator("#screening-local-add-watchlist").click();
  let watchlist = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.watchlist.v1") || "[]"));
  assert.equal(watchlist.length, 1);
  assert.equal(watchlist[0].symbol, "600001.SH");
  assert.match(await page.locator("#screening-local-batch-feedback").textContent(), /新增 1 只/);
  await page.evaluate(() => localStorage.setItem("stock-provider-webui.watchlist-groups.v1", JSON.stringify({
    groups: ["未分组", "重点观察"], assignments: {}
  })));
  await page.locator("#screening-local-group").focus();
  await page.locator("#screening-local-group").selectOption("重点观察");
  await page.locator("#screening-local-results-body input[data-screening-local-select-symbol]").check();
  await page.locator("#screening-local-add-watchlist").click();
  watchlist = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.watchlist.v1") || "[]"));
  const groups = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.watchlist-groups.v1") || "{}"));
  assert.equal(watchlist.length, 1, "adding an existing result must not create a duplicate");
  assert.equal(groups.assignments["600001.SH"], "重点观察");
  assert.match(await page.locator("#screening-local-batch-feedback").textContent(), /重复跳过 1 只/);
  assert.match(await page.locator("#screening-local-batch-feedback").textContent(), /调整分组 1 只/);
  assert.equal(await page.locator("#screening-local-select-count").textContent(), "已选择 0 只");
  assert.equal(marketStocksRequests, 1, "batch watchlist actions must not re-fetch market data");


  await page.locator("#screening-plan-name").fill("浏览器筛选方案");
  await page.locator("#screening-plan-save").click();
  const storedBeforeClear = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.screening-plans.v1") || "[]"));
  assert.equal(storedBeforeClear.length, 1);
  await page.locator("#screening-condition-clear").click();
  assert.match(await summary.textContent(), /条件已清空/);
  assert.match(await page.locator("#screening-local-results-body").textContent(), /尚未执行当前页筛选/);
  const storedAfterClear = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.screening-plans.v1") || "[]"));
  assert.equal(storedAfterClear.length, 1, "clearing the draft must not delete saved plans");

  await page.locator("#screening-condition-field").selectOption("change_pct");
  await page.locator("#screening-condition-value").fill("0");
  await page.locator("#screening-condition-add").click();
  await page.locator("#screening-apply-current-page").click();
  await page.locator("#screening-condition-value").fill("2");
  await page.locator("#screening-condition-add").click();
  assert.match(await summary.textContent(), /筛选条件已变化/);
  assert.match(await page.locator("#screening-local-results-body").textContent(), /尚未执行当前页筛选/);
  await page.locator("#screening-apply-current-page").click();

  // 全市场刷新进行中时，旧快照不得继续用于筛选、导出或批量加入。
  await page.locator('.nav-link[data-view="stocks"]').click();
  holdSecondMarketStocksResponse = true;
  await page.locator("#reload-market-stocks").click();
  await secondMarketStocksStarted;
  await page.locator('.nav-link[data-view="screening"]').click();
  assert.match(await summary.textContent(), /分页已变化或重新加载/);
  await page.locator("#screening-apply-current-page").click();
  assert.match(await summary.textContent(), /正在刷新，暂不能应用筛选条件/);
  assert.equal(await exportButton.isEnabled(), false);
  assert.equal(await page.locator("#screening-local-add-watchlist").isEnabled(), false);
  assert.equal(await page.locator("#screening-local-results-body tr").count(), 1);
  releaseSecondMarketStocksResponse();
  await page.waitForFunction(() => document.querySelector("#market-stocks-meta")?.textContent.includes("2026-11-03T03:00:02Z"));
  assert.equal(await exportButton.isEnabled(), false, "refresh completion must not silently revive the previous screening result");
  await page.locator("#screening-apply-current-page").click();
  await page.waitForFunction(() => document.querySelector("#screening-local-results-summary")?.textContent.includes("命中 1 条"));
  assert.deepEqual(pageErrors, []);
  await page.screenshot({ path: path.join(screenshotDir, "screening-local-results.png"), fullPage: true });
  console.log("Screening local filter browser acceptance passed: result views, tri-state quality, stale-result invalidation, draft clearing and saved-plan preservation.");
  await context.close();
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

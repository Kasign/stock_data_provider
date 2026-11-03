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
  await page.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/market-api/api/v1/market/stocks") {
      marketStocksRequests += 1;
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

  // 全市场分页刷新后，即使页码/页大小相同，也必须使上一批结果失效。
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#reload-market-stocks").click();
  await page.waitForFunction(() => document.querySelector("#market-stocks-meta")?.textContent.includes("2026-11-03T03:00:02Z"));
  await page.locator('.nav-link[data-view="screening"]').click();
  assert.match(await summary.textContent(), /分页已变化或重新加载/);
  assert.deepEqual(pageErrors, []);
  await page.screenshot({ path: path.join(screenshotDir, "screening-local-results.png"), fullPage: true });
  console.log("Screening local filter browser acceptance passed: result views, tri-state quality, stale-result invalidation, draft clearing and saved-plan preservation.");
  await context.close();
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

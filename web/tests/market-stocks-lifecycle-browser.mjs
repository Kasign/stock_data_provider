import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
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
    response.writeHead(200, { "content-type": contentType, "cache-control": "no-store" });
    response.end(body);
  } catch { response.writeHead(404); response.end("Not found"); }
});

await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;

try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "market-stocks-lifecycle-key"
    }));
  });

  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  const requests = [];
  let holdFifthResponse = false;
  let markFifthStarted;
  let releaseFifthResponse;
  const fifthStarted = new Promise(resolve => { markFifthStarted = resolve; });
  const fifthGate = new Promise(resolve => { releaseFifthResponse = resolve; });

  await page.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/market-api/api/v1/market/stocks") {
      const pageNumber = Number(url.searchParams.get("page") || 1);
      const pageSize = Number(url.searchParams.get("page_size") || 50);
      const adjustment = url.searchParams.get("adjustment") || "forward_adjusted";
      requests.push({ page: pageNumber, pageSize, adjustment });
      const requestNumber = requests.length;
      if (requestNumber === 5 && holdFifthResponse) {
        markFifthStarted();
        await fifthGate;
      }
      const total = 60;
      const count = Math.max(0, Math.min(pageSize, total - (pageNumber - 1) * pageSize));
      const items = Array.from({ length: count }, (_, index) => {
        const sequence = (pageNumber - 1) * pageSize + index + 1;
        const symbol = "60" + String(sequence).padStart(4, "0") + ".SH";
        return {
          symbol, name: "测试股票" + sequence, quality_status: "valid",
          quote: { symbol, name: "测试股票" + sequence, current_price: 100 + sequence, change_percent: 1, amount: 1000000, turnover_rate: 1, quality_status: "valid" },
          ma5: 99 + sequence, ma20: 98 + sequence
        };
      });
      try {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items, page: pageNumber, page_size: pageSize, total, total_is_lower_bound: false,
          coverage_ratio: 1, quote_succeeded: count, quote_failed: 0, quality_status: "valid",
          universe_source: "browser-fixture", universe_source_version: "lifecycle-test",
          universe_fetched_at: "2026-11-04T02:00:00Z", fetched_at: "lifecycle-request-" + requestNumber,
          ma_adjustment_basis: adjustment
        }) });
      } catch {
        // 若页面或连接生命周期取消请求，测试路由不再尝试提交响应。
      }
      return;
    }
    if (url.pathname.includes("/api/")) {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { message: "isolated browser fixture" } }) });
      return;
    }
    await route.continue();
  });

  await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#load-market-stocks").click();
  await page.waitForFunction(() => document.querySelectorAll("#market-stocks-body tr").length === 50);
  assert.deepEqual(requests[0], { page: 1, pageSize: 50, adjustment: "forward_adjusted" });
  assert.equal(await page.locator("#market-stocks-next").isEnabled(), true);

  // 翻页必须使用当前页大小和复权口径，且仅更新一次请求。
  await page.locator("#market-stocks-next").click();
  await page.waitForFunction(() => document.querySelector("#market-stocks-summary")?.textContent.includes("第 2 页 · 返回 10 条"));
  assert.deepEqual(requests[1], { page: 2, pageSize: 50, adjustment: "forward_adjusted" });
  assert.equal(await page.locator("#market-stocks-body tr").count(), 10);
  assert.equal(await page.locator("#market-stocks-next").isEnabled(), false);

  // 页大小变化回到第一页，并重新获取对应数量的记录。
  await page.locator("#market-stocks-page-size").selectOption("20");
  await page.waitForFunction(() => document.querySelector("#market-stocks-page-label")?.textContent === "第 1 页 · 每页 20 条");
  assert.deepEqual(requests[2], { page: 1, pageSize: 20, adjustment: "forward_adjusted" });
  assert.equal(await page.locator("#market-stocks-body tr").count(), 20);

  // 复权方式变化同样回到第一页，并将新口径传给接口。
  await page.locator("#market-stocks-adjustment").selectOption("backward_adjusted");
  await page.waitForFunction(() => document.querySelector("#market-stocks-meta")?.textContent.includes("MA 复权：backward_adjusted"));
  assert.deepEqual(requests[3], { page: 1, pageSize: 20, adjustment: "backward_adjusted" });

  // 刷新期间控件应锁定；切换页面不应遗留 busy 状态，响应完成后返回仍显示新快照。
  holdFifthResponse = true;
  await page.locator("#reload-market-stocks").click();
  await fifthStarted;
  assert.equal(await page.locator("#reload-market-stocks").isDisabled(), true);
  assert.equal(await page.locator("#market-stocks-page-size").isDisabled(), true);
  assert.equal(await page.locator("#market-stocks-adjustment").isDisabled(), true);
  await page.locator('.nav-link[data-view="market"]').click();
  releaseFifthResponse();
  await page.waitForFunction(() => document.querySelector("#market-stocks-meta")?.textContent.includes("lifecycle-request-5"));
  await page.locator('.nav-link[data-view="stocks"]').click();
  assert.equal(await page.locator("#market-stocks-body tr").count(), 20);
  assert.equal(await page.locator("#reload-market-stocks").isEnabled(), true);
  assert.deepEqual(requests[4], { page: 1, pageSize: 20, adjustment: "backward_adjusted" });
  assert.equal(requests.length, 5, "one refresh must issue exactly one request");
  assert.deepEqual(pageErrors, []);

  await context.close();
  console.log("Market stocks lifecycle browser acceptance passed: pagination, page size, adjustment, busy controls and leave/return response state.");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

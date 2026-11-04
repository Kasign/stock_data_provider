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
    const body = await readFile(target);
    const extension = path.extname(target);
    const contentType = extension === ".html" ? "text/html; charset=utf-8"
      : [".js", ".mjs"].includes(extension) ? "text/javascript; charset=utf-8"
        : extension === ".css" ? "text/css; charset=utf-8" : "application/octet-stream";
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
    if (!localStorage.getItem("stock-provider-webui.settings.v1")) {
      localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
        marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "watchlist-browser-key"
      }));
    }
    if (!localStorage.getItem("stock-provider-webui.watchlist.v1")) {
      localStorage.setItem("stock-provider-webui.watchlist.v1", JSON.stringify([
        { symbol: "600519.SH", name: "测试标的", updated_at: "2026-11-04T01:00:00Z" }
      ]));
    }
  });

  let quoteRequests = 0;
  let markFirstStarted, markSecondStarted, markThirdStarted, releaseFirst, releaseSecond, releaseThird;
  const firstStarted = new Promise(resolve => { markFirstStarted = resolve; });
  const secondStarted = new Promise(resolve => { markSecondStarted = resolve; });
  const thirdStarted = new Promise(resolve => { markThirdStarted = resolve; });
  const firstGate = new Promise(resolve => { releaseFirst = resolve; });
  const secondGate = new Promise(resolve => { releaseSecond = resolve; });
  const thirdGate = new Promise(resolve => { releaseThird = resolve; });
  const quotePayload = () => ({
    items: [{
      symbol: "600519.SH",
      quote: { symbol: "600519.SH", current_price: 1800, change_percent: 1.2, quote_time: "2026-11-04T01:00:00Z", quality_status: "valid" },
      quality_status: "valid"
    }],
    succeeded: 1, total: 1, failed: 0, quality_status: "valid", fetched_at: "2026-11-04T01:01:00Z"
  });

  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  await page.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/market-api/api/v1/market/quotes") {
      quoteRequests += 1;
      if (quoteRequests === 1) { markFirstStarted(); await firstGate; }
      if (quoteRequests === 2) { markSecondStarted(); await secondGate; }
      if (quoteRequests === 3) { markThirdStarted(); await thirdGate; }
      if (quoteRequests === 5) {
        return route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { message: "fixture unavailable" } }) });
      }
      if (quoteRequests === 6) {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items: [
            { symbol: "600519.SH", quote: { symbol: "600519.SH", current_price: 1800, change_percent: 1.2, quote_time: "2026-11-04T01:00:00Z", quality_status: "valid" }, quality_status: "valid" },
            { symbol: "000001.SZ", quote: null, error_code: "quote_unavailable", quality_status: "unknown" }
          ],
          succeeded: 1, total: 2, failed: 1, quality_status: "degraded", fetched_at: "2026-11-04T01:04:00Z"
        }) });
      }
      if (quoteRequests === 7) {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items: [], succeeded: 0, total: 2, failed: 2, quality_status: "unavailable", fetched_at: "2026-11-04T01:05:00Z"
        }) });
      }
      try { return await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(quotePayload()) }); }
      catch { /* 跨标签页成员变化应中止旧请求，路由不再提交旧响应。 */ }
    }
    if (url.pathname.includes("/api/") || /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(url.pathname)) {
      return route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { message: "isolated browser fixture" } }) });
    }
    return route.continue();
  });

  await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
  await page.locator('.nav-link[data-view="watchlist"]').click();
  const refresh = page.locator("#refresh-watchlist-quotes");
  const status = page.locator("#watchlist-quote-refresh-status");
  const list = page.locator("#watchlist-list");

  // 请求期间增加股票：响应属于旧成员集合，必须丢弃，不得只更新其中一部分。
  await refresh.click();
  await firstStarted;
  await page.evaluate(() => {
    const key = "stock-provider-webui.watchlist.v1";
    const items = JSON.parse(localStorage.getItem(key) || "[]");
    items.push({ symbol: "000001.SZ", name: "新增标的", updated_at: "2026-11-04T01:02:00Z" });
    localStorage.setItem(key, JSON.stringify(items));
  });
  releaseFirst();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("本次响应已丢弃"));
  assert.match(await list.textContent(), /000001\.SZ/);
  assert.doesNotMatch(await list.textContent(), /批量采集 11\/04 09:01/);
  assert.match(await status.textContent(), /请按当前清单重新刷新/);

  // 恢复原成员集合后，只改变分组：分组不改变行情请求范围，响应应正常应用。
  await page.evaluate(() => localStorage.setItem("stock-provider-webui.watchlist.v1", JSON.stringify([
    { symbol: "600519.SH", name: "测试标的", updated_at: "2026-11-04T01:00:00Z" }
  ])));
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.locator('.nav-link[data-view="watchlist"]').click();
  await page.locator("#refresh-watchlist-quotes").click();
  await secondStarted;
  await page.evaluate(() => localStorage.setItem("stock-provider-webui.watchlist-groups.v1", JSON.stringify({
    groups: ["未分组", "重点观察"], assignments: { "600519.SH": "重点观察" }
  })));
  releaseSecond();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("批量行情已返回 1 / 1"));
  assert.match(await list.textContent(), /批量采集/);

  // 第二个标签页修改自选股成员时，当前页应立即同步列表并中止在途批量行情。
  const otherPage = await context.newPage();
  await otherPage.goto(baseUrl, { waitUntil: "domcontentloaded" });
  await page.locator("#refresh-watchlist-quotes").click();
  await thirdStarted;
  await otherPage.evaluate(() => {
    const key = "stock-provider-webui.watchlist.v1";
    const items = JSON.parse(localStorage.getItem(key) || "[]");
    items.push({ symbol: "000001.SZ", name: "跨标签新增", updated_at: "2026-11-04T01:03:00Z" });
    localStorage.setItem(key, JSON.stringify(items));
  });
  await page.waitForFunction(() =>
    document.querySelector("#watchlist-count")?.textContent.includes("2 /") &&
    document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("在途请求已失效")
  );
  assert.match(await list.textContent(), /000001\.SZ/);
  assert.equal(await refresh.isEnabled(), true, "cross-tab membership change must restore the refresh button");
  assert.equal(await refresh.getAttribute("aria-busy"), null);
  releaseThird();
  await page.waitForTimeout(100);
  assert.doesNotMatch(await list.textContent(), /批量采集/);
  await otherPage.close();

  // 重新按当前成员集合成功刷新，再验证失败时保留快照的旧数据标识。
  await page.evaluate(() => localStorage.setItem("stock-provider-webui.watchlist.v1", JSON.stringify([
    { symbol: "600519.SH", name: "测试标的", updated_at: "2026-11-04T01:00:00Z" }
  ])));
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.locator('.nav-link[data-view="watchlist"]').click();
  await page.locator("#refresh-watchlist-quotes").click();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("批量行情已返回 1 / 1"));
  // 刷新失败保留上次成功快照时，行内状态必须明确标记为旧数据。
  await page.locator("#refresh-watchlist-quotes").click();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("标记为旧数据"));
  assert.match(await page.locator(".watchlist-tracking-line").first().textContent(), /上次成功快照/);

  // 部分成功：成功项正常展示，失败项显示明确错误码。
  await page.evaluate(() => localStorage.setItem("stock-provider-webui.watchlist.v1", JSON.stringify([
    { symbol: "600519.SH", name: "成功标的", updated_at: "2026-11-04T01:00:00Z" },
    { symbol: "000001.SZ", name: "失败标的", updated_at: "2026-11-04T01:00:00Z" }
  ])));
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.locator('.nav-link[data-view="watchlist"]').click();
  await page.locator("#refresh-watchlist-quotes").click();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("批量行情已返回 1 / 2"));
  assert.match(await page.locator('.watchlist-item').filter({ hasText: "600519.SH" }).locator(".watchlist-tracking-line").textContent(), /行情快照：/);
  assert.match(await page.locator('.watchlist-item').filter({ hasText: "000001.SZ" }).locator(".watchlist-tracking-line").textContent(), /批量行情不可用：quote_unavailable/);
  assert.match(await status.textContent(), /失败 1 只/);

  // 空 items：所有请求标的均须显示明确失败，不能退化为“未加载”。
  await page.locator("#refresh-watchlist-quotes").click();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("批量行情已返回 0 / 2"));
  assert.match(await status.textContent(), /失败 2 只/);
  for (const symbol of ["600519.SH", "000001.SZ"]) {
    const line = page.locator('.watchlist-item').filter({ hasText: symbol }).locator(".watchlist-tracking-line");
    assert.match(await line.textContent(), /批量行情不可用：missing_response_item/);
  }
  assert.deepEqual(pageErrors, []);
  await context.close();
  console.log("Watchlist quote snapshot acceptance passed: membership changes discard in-flight responses, group-only changes preserve quote scope, and failures label retained snapshots as old.");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

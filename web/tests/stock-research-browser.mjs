import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(webRoot, "..");
const screenshotDir = path.join(repositoryRoot, "browser-screenshots", "stock-research");
const server = createServer(async (request, response) => {
  const pathname = decodeURIComponent(new URL(request.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) {
    response.writeHead(403);
    response.end("Forbidden");
    return;
  }
  try {
    const body = await readFile(target);
    const extension = path.extname(target);
    const contentType = extension === ".html" ? "text/html; charset=utf-8"
      : extension === ".css" ? "text/css; charset=utf-8"
        : [".js", ".mjs"].includes(extension) ? "text/javascript; charset=utf-8"
          : "application/octet-stream";
    response.writeHead(200, { "content-type": contentType, "cache-control": "no-store" });
    response.end(body);
  } catch {
    response.writeHead(404);
    response.end("Not found");
  }
});

await mkdir(screenshotDir, { recursive: true });
await new Promise((resolve, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolve);
});
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;
let releaseOldRequest;
let oldRequestStarted;
try {
  browser = await chromium.launch({ headless: true });

  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "research-browser-key"
    }));
  });
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  oldRequestStarted = new Promise(resolve => {
    page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.pathname.includes("/api/v1/stocks/600519.SH/fundamentals")) {
        resolve();
        await new Promise(done => { releaseOldRequest = done; });
        try {
          await route.fulfill({
            status: 200, contentType: "application/json",
            body: JSON.stringify({
              symbol: "600519.SH", year: 2025, quarter: 4, quality_status: "valid",
              source: "browser-fixture",
              sections: [{ category: "profitability", report_date: "2025-12-31", publication_date: "2026-03-30",
                quality_status: "valid", has_data: true,
                metrics: [{ name: "延迟旧标的指标", value: 999, unit: "元" }] }]
            })
          });
        } catch {
          // 切换股票后旧请求应已被 AbortController 取消。
        }
        return;
      }
      if (url.pathname.includes("/api/")) {
        await route.fulfill({
          status: 503, contentType: "application/json",
          body: JSON.stringify({ error: { message: "browser fixture unavailable" } })
        });
        return;
      }
      await route.continue();
    });
  });

  await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await page.locator('.nav-link[data-view="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.locator("#symbol-input").fill("600519.SH");
  await page.locator("#load-fundamentals").click();
  await oldRequestStarted;

  // 切换标的后必须中止旧请求，并恢复所有请求按钮，包括财务指标按钮。
  await page.locator("#symbol-input").fill("000001.SZ");
  await page.waitForFunction(() => {
    const button = document.querySelector("#load-fundamentals");
    return button && !button.disabled && button.textContent.includes("读取财务指标");
  });
  releaseOldRequest();
  await page.waitForTimeout(100);
  assert.equal(await page.locator("#fundamentals-symbol").textContent(), "000001.SZ");
  assert.doesNotMatch(await page.locator("#fundamentals-content").textContent(), /延迟旧标的指标|999/);
  assert.equal(await page.locator("#load-fundamentals").isEnabled(), true);

  // 其它补充模块的 503 状态应清楚展示，且不锁死主资料按钮。
  await page.locator("#load-stock-research").click();
  await page.waitForFunction(() => document.querySelector("#stock-profile-error")?.textContent.includes("未启用或未配置"));
  assert.equal(await page.locator("#stock-profile-symbol").textContent(), "000001.SZ");
  assert.equal(await page.locator("#load-stock-research").isEnabled(), true);
  assert.deepEqual(pageErrors, [], pageErrors.join("\n"));
  await page.screenshot({
    path: path.join(screenshotDir, "1280x900-stock-research.png"),
    fullPage: false,
    animations: "disabled"
  });
  await context.close();

  // 缺少 API Key 时统一提示，但按钮保持可重试，配置完成后不会被永久禁用。
  const noKeyContext = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await noKeyContext.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: ""
    }));
  });
  const noKeyPage = await noKeyContext.newPage();
  await noKeyPage.route("**/api/**", route => route.fulfill({
    status: 503, contentType: "application/json",
    body: JSON.stringify({ error: { message: "not configured" } })
  }));
  await noKeyPage.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await noKeyPage.locator('.nav-link[data-view="research"]').click();
  await noKeyPage.locator("#symbol-input").fill("600519.SH");
  await noKeyPage.locator("#load-valuations").click();
  await noKeyPage.waitForFunction(() => document.querySelector("#valuations-quality")?.textContent === "需配置 Key");
  for (const selector of [
    "#load-stock-research", "#load-fundamentals", "#load-valuations",
    "#load-chip-distribution", "#load-capital-flow"
  ]) {
    assert.equal(await noKeyPage.locator(selector).isEnabled(), true, selector + " must remain retryable without API key");
  }
  await noKeyContext.close();
  console.log("PASS stock research request cancellation and button recovery");
} finally {
  if (releaseOldRequest) releaseOldRequest();
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

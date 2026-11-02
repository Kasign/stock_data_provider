import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(webRoot, "..");
const screenshotDir = path.resolve(process.env.BROWSER_SCREENSHOT_DIR || path.join(repositoryRoot, "browser-screenshots"));
const viewports = [
  { width: 320, height: 800 },
  { width: 375, height: 812 },
  { width: 600, height: 900 },
  { width: 768, height: 1024 },
  { width: 900, height: 900 },
  { width: 1280, height: 900 },
  { width: 1600, height: 1000 }
];
const viewNavigation = [
  { name: "market", selector: '.nav-link[data-view="market"]' },
  { name: "stocks", selector: '.nav-link[data-view="stocks"]' },
  { name: "watchlist", selector: '.nav-link[data-view="watchlist"]' },
  { name: "research", selector: '.nav-link[data-view="research"]' },
  { name: "tasks", selector: '.nav-link[data-view="tasks"]' },
  { name: "data", selector: '.nav-link[data-view="data"]' },
  { name: "notifications", selector: '.nav-link[data-view="notifications"]' },
  { name: "chat", selector: '.nav-link[data-view="chat"]' },
  { name: "screening", selector: '.nav-link[data-view="screening"]' },
  { name: "alerts", selector: '.nav-link[data-view="alerts"]' },
  { name: "settings", selector: '.nav-link[data-view="settings"]' }
];
const clickNav = async (targetPage, selector) => {
  const menu = targetPage.locator("#mobile-menu-toggle");
  if (await menu.isVisible() && (await menu.getAttribute("aria-expanded")) !== "true") await menu.click();
  await targetPage.locator(selector).click();
};

const contentTypes = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml"
};

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
    response.writeHead(200, {
      "content-type": contentTypes[path.extname(target)] || "application/octet-stream",
      "cache-control": "no-store"
    });
    response.end(body);
  } catch {
    response.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
    response.end("Not found");
  }
});

await mkdir(screenshotDir, { recursive: true });
await new Promise((resolve, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolve);
});

const address = server.address();
assert.ok(address && typeof address === "object", "static WebUI server must listen on an ephemeral port");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;

try {
  browser = await chromium.launch({ headless: true });

  for (const viewport of viewports) {
    const context = await browser.newContext({ viewport, reducedMotion: "reduce" });
    const page = await context.newPage();
    await page.addInitScript(() => {
      window.__pageErrorDetails = [];
      window.addEventListener("error", (event) => {
        window.__pageErrorDetails.push({
          message: event.message,
          filename: event.filename,
          line: event.lineno,
          column: event.colno,
          stack: event.error?.stack || ""
        });
      });
    });
    const pageErrors = [];
    const consoleErrors = [];
    const unexpectedNetworkFailures = [];
    page.on("pageerror", (error) => pageErrors.push(error.stack || error.message));
    page.on("console", (message) => {
      if (message.type() !== "error") return;
      const detail = message.text();
      // Mock API 4xx/5xx 与主动断网场景会产生浏览器资源错误；业务错误由页面状态和请求断言单独验收。
      if (/Failed to load resource|net::ERR_FAILED/.test(detail)) return;
      consoleErrors.push(detail);
    });
    page.on("requestfailed", (request) => {
      const pathname = new URL(request.url()).pathname;
      const expectedApiFailure = pathname.includes("/api/") ||
        /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(pathname);
      if (!expectedApiFailure) unexpectedNetworkFailures.push(request.method() + " " + pathname + ": " + (request.failure()?.errorText || "request failed"));
    });
    page.on("response", (response) => {
      const pathname = new URL(response.url()).pathname;
      const isApi = pathname.includes("/api/") ||
        /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(pathname);
      if (!isApi && response.status() >= 400) {
        unexpectedNetworkFailures.push(response.status() + " " + pathname);
      }
    });

    // API responses are deterministic fixtures only: one isolated viewport exercises failures, others stay unavailable.
    let quoteScenario = "success";
    let quoteRequestCount = 0;
    let marketScenario = "503";
    let marketIndexRequestCount = 0;
    let capitalFlowScenario = "success";
    let capitalFlowRequestCount = 0;
    let capitalFlowRequest = null;
    let dataSourceScenario = "success";
    let dataSourcesRequestCount = 0;
    let batchQuotesRequestCount = 0;
    let diagnosticsRequestCount = 0;
    let healthScenario = "unavailable";
    let settingsConnectionScenario = "default";
    let settingsConnectionRequestCount = 0;
    const quoteFixture = {
      symbol: "600519.SH",
      name: "贵州茅台",
      current_price: 123.45,
      change: 1.23,
      change_percent: 1.01,
      amount: 123450,
      volume: 1000,
      turnover_rate: 1.2,
      volume_ratio: 1.05,
      source: "browser-test-fixture",
      quote_time: "2026-10-28T07:00:00Z",
      quality_status: "valid",
      quality_reasons: []
    };
    const barsFixture = {
      symbol: "600519.SH",
      bars: [{
        trade_date: "2026-10-28",
        open: 122.5,
        high: 124,
        low: 121.8,
        close: 123.45,
        volume: 1000,
        amount: 123450,
        change_percent: 1.01
      }],
      adjustment_basis: "forward_adjusted",
      volume_unit: "shares",
      amount_unit: "CNY",
      quality_status: "valid",
      warnings: []
    };
    const quoteForSymbol = (symbol) => symbol === "000001.SH"
      ? {
          ...quoteFixture,
          symbol,
          name: "测试新标的",
          current_price: 10.25,
          change: -0.25,
          change_percent: -2.38,
          amount: 10125,
          volume: 985,
          turnover_rate: 0.82,
          volume_ratio: 0.95,
          quote_time: "2026-10-29T01:00:00Z"
        }
      : { ...quoteFixture, symbol };
    const maSeedBars = [121, 122, 123, 124, 125].map((close, index) => ({
      trade_date: ["2026-10-22", "2026-10-23", "2026-10-26", "2026-10-27", "2026-10-28"][index],
      open: close - 0.2,
      high: close + 0.5,
      low: close - 0.5,
      close,
      volume: 1000 + index,
      amount: close * (1000 + index),
      change_percent: index ? 0.8 : null,
      quality_status: "valid",
      source: "browser-test-fixture"
    }));
    const quoteErrors = {
      "401": { status: 401, code: "auth_required", message: "credentials required", requestId: "req-browser-401", expected: "接口认证失败" },
      "403": { status: 403, code: "access_denied", message: "permission denied", requestId: "req-browser-403", expected: "当前请求被服务拒绝" },
      "404": { status: 404, code: "quote_not_found", message: "quote does not exist", requestId: "req-browser-404", expected: "接口或资源不存在" },
      "503": { status: 503, code: "provider_unavailable", message: "上游 Provider 暂不可用", requestId: "req-browser-503", expected: "上游 Provider 暂不可用" },
      network: { status: 0, expected: "网络请求失败" }
    };
    const marketOverviewErrors = {
      "401": { status: 401, code: "auth_required", message: "credentials required", requestId: "req-overview-401", expected: "接口认证失败", summary: "鉴权失败 1 个" },
      "403": { status: 403, code: "access_denied", message: "permission denied", requestId: "req-overview-403", expected: "当前请求被服务拒绝", summary: "鉴权失败 1 个" },
      "404": { status: 404, code: "route_not_found", message: "route not found", requestId: "req-overview-404", expected: "接口或资源不存在", summary: "接口尚未提供 1 个" },
      "503": { status: 503, code: "provider_unavailable", message: "上游 Provider 暂不可用", requestId: "req-overview-503", expected: "上游 Provider 暂不可用", summary: "Provider 不可用 2 个" },
      network: { status: 0, expected: "网络请求失败", summary: "读取失败 1 个" }
    };
    const dataSourceErrors = {
      "401": { status: 401, code: "auth_required", message: "credentials required", requestId: "req-data-401", expected: "接口认证失败" },
      "403": { status: 403, code: "access_denied", message: "permission denied", requestId: "req-data-403", expected: "当前请求被服务拒绝" },
      "404": { status: 404, code: "route_not_found", message: "data source route not found", requestId: "req-data-404", expected: "接口或资源不存在" },
      "503": { status: 503, code: "provider_unavailable", message: "Provider 清单服务暂不可用", requestId: "req-data-503", expected: "Provider 清单服务暂不可用" },
      network: { status: 0, expected: "网络请求失败" }
    };
    const settingsConnectionErrors = {
      "401": { status: 401, code: "auth_required", message: "credentials required", requestId: "req-settings-401", expected: "接口认证失败" },
      "403": { status: 403, code: "access_denied", message: "permission denied", requestId: "req-settings-403", expected: "当前请求被服务拒绝" },
      "404": { status: 404, code: "route_not_found", message: "health route not found", requestId: "req-settings-404", expected: "接口或资源不存在" },
      "503": { status: 503, code: "probe_unavailable", message: "健康探针服务暂不可用", requestId: "req-settings-503", expected: "健康探针服务暂不可用" },
      network: { status: 0, expected: "网络请求失败" }
    };

    if (viewport.width === 320) {
      await context.addInitScript(() => {
        localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
          marketBase: "/market-api",
          analysisBase: "/analysis-api",
          apiKey: "browser-test-key"
        }));
      });
    }

    await page.route("**/*", async (route) => {
      const pathname = new URL(route.request().url()).pathname;
      const json = (status, body) => route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body)
      });

      // Settings tests intentionally change API prefixes; saved and unsaved probes share this deterministic error matrix.
      if (viewport.width === 320 && settingsConnectionScenario !== "default" &&
          (pathname.endsWith("/health/ready") || pathname.endsWith("/readyz"))) {
        const scenario = settingsConnectionScenario;
        settingsConnectionRequestCount += 1;
        if (scenario === "network") return route.abort("failed");
        const item = settingsConnectionErrors[scenario === "delayed" ? "503" : scenario];
        if (scenario === "delayed") await new Promise((resolve) => setTimeout(resolve, 1500));
        return json(item.status, {
          status: "unavailable",
          error: {
            code: item.code,
            message: item.message,
            request_id: item.requestId
          }
        });
      }
      if (pathname.endsWith("/health/ready") || pathname.endsWith("/readyz")) {
        if (viewport.width === 320 && healthScenario === "missing-status") {
          return json(200, { message: "HTTP success without an explicit health status" });
        }
        return json(503, { status: "unavailable", error: "browser test backend unavailable" });
      }

      if (viewport.width === 320 && pathname.includes("/api/v1/bundles/")) {
        // Exercise the supported legacy fallback so the successful quote can seed a prior snapshot.
        return json(404, { error: { code: "route_not_found", message: "bundle endpoint unavailable" } });
      }

      if (viewport.width === 320 && pathname.endsWith("/api/v1/market/indices")) {
        marketIndexRequestCount += 1;
        if (marketScenario === "success") {
          return json(200, {
            status: "ok",
            source: "browser-test-fixture",
            fetched_at: "2026-10-28T07:00:00Z",
            items: [{
              name: "测试指数",
              code: "TEST-INDEX",
              symbol: "000001.SH",
              current_price: 3000,
              change: 10,
              change_percent: 0.5,
              quote_time: "2026-10-28T07:00:00Z",
              source: "browser-test-fixture",
              quality_status: "valid",
              quality_reasons: []
            }]
          });
        }
        const item = marketOverviewErrors[marketScenario] || marketOverviewErrors["503"];
        if (marketScenario === "network") return route.abort("failed");
        return json(item.status, {
          error: {
            code: item.code,
            message: item.message,
            request_id: item.requestId
          }
        });
      }

      if (viewport.width === 320 && pathname.endsWith("/api/v1/market/capital-flow")) {
        capitalFlowRequestCount += 1;
        capitalFlowRequest = { url: route.request().url(), headers: route.request().headers() };
        if (capitalFlowScenario === "network") return route.abort("failed");
        if (capitalFlowScenario === "503") return json(503, { error: { code: "provider_unavailable", message: "Tushare Provider unavailable", request_id: "req-market-flow-503" } });
        return json(200, {
          start_date: "2026-10-01", end_date: "2026-10-30", total: 2,
          items: [
            { trade_date: "2026-10-30", close_sh: 3950.2, change_percent_sh: 0.5, close_sz: 12800.5, change_percent_sz: -0.2, net_amount_cny: 1500, net_amount_rate_percent: 0.35 },
            { trade_date: "2026-10-29", close_sh: null, change_percent_sh: null, close_sz: null, change_percent_sz: null, net_amount_cny: null, net_amount_rate_percent: null }
          ],
          source: "tushare", source_version: "moneyflow_mkt_dc", fetched_at: "2026-10-30T08:00:00Z",
          quality_status: "degraded", quality_reasons: ["源端时间精度不足"]
        });
      }

      if (viewport.width === 320 && pathname.endsWith("/api/v1/data-sources")) {
        dataSourcesRequestCount += 1;
        if (dataSourceScenario === "network") return route.abort("failed");
        if (dataSourceScenario === "delayed-success") {
          await new Promise((resolve) => setTimeout(resolve, 1600));
          return json(200, {
            sources: [{
              name: "测试 Provider",
              enabled: true,
              quote_capable: true,
              daily_bar_capable: true
            }]
          });
        }
        if (dataSourceScenario === "success") {
          return json(200, {
            sources: [{
              name: "测试 Provider",
              enabled: true,
              quote_capable: true,
              daily_bar_capable: true
            }]
          });
        }
        const item = dataSourceErrors[dataSourceScenario];
        return json(item.status, {
          error: {
            code: item.code,
            message: item.message,
            request_id: item.requestId
          }
        });
      }

      if (viewport.width === 320 && pathname.endsWith("/api/v1/data-sources/diagnostics")) {
        diagnosticsRequestCount += 1;
        return json(200, {
          items: [{
            provider: "测试 Provider",
            capabilities: [
              { capability: "quote", status: "healthy", consecutive_failures: 0, total_success: 3, total_failure: 0, last_success_at: "2026-10-30T07:00:00Z", last_failure_at: null, open_until: null },
              { capability: "daily_bars", status: "not_observed", consecutive_failures: 0, total_success: 0, total_failure: 0, last_success_at: null, last_failure_at: null, open_until: null }
            ]
          }],
          scope: ["quote", "daily_bars"],
          observed_from: "in_process_circuit_breaker"
        });
      }

      if (viewport.width === 320 && pathname.includes("/api/v1/quotes/")) {
        quoteRequestCount += 1;
        const requestedSymbol = decodeURIComponent(pathname.split("/api/v1/quotes/")[1] || "");
        if (quoteScenario === "network") return route.abort("failed");
        if (quoteScenario === "delayed-old" && requestedSymbol === "600519.SH") {
          await page.evaluate(() => { window.__browserTestDelayedQuoteStarted = true; });
          const delayedBody = JSON.stringify(quoteForSymbol(requestedSymbol));
          setTimeout(() => {
            void route.fulfill({
              status: 200,
              contentType: "application/json",
              body: delayedBody
            }).catch(() => {});
          }, 1600);
          return;
        }
        if (quoteScenario === "success" || quoteScenario === "ma-seed") {
          return json(200, quoteForSymbol(requestedSymbol));
        }
        const item = quoteErrors[quoteScenario];
        return json(item.status, {
          error: {
            code: item.code,
            message: item.message,
            request_id: item.requestId
          }
        });
      }

      if (viewport.width === 320 && pathname.includes("/api/v1/bars/") && pathname.endsWith("/daily")) {
        const requestedSymbol = decodeURIComponent((pathname.split("/api/v1/bars/")[1] || "").split("/")[0]);
        if (quoteScenario === "ma-seed" && requestedSymbol === "600519.SH") {
          return json(200, { ...barsFixture, symbol: requestedSymbol, bars: maSeedBars });
        }
        if (requestedSymbol === "000001.SH") {
          return json(200, {
            ...barsFixture,
            symbol: requestedSymbol,
            bars: [{
              ...barsFixture.bars[0],
              trade_date: "2026-10-28",
              open: 10.1,
              high: 10.5,
              low: 9.8,
              close: 10.25,
              volume: 985,
              amount: 10125,
              change_percent: -2.38
            }]
          });
        }
        return json(200, { ...barsFixture, symbol: requestedSymbol });
      }

      if (viewport.width === 320 && pathname.endsWith("/api/v1/market/quotes")) {
        batchQuotesRequestCount += 1;
        const symbols = new URL(route.request().url()).searchParams.get("symbols")?.split(",") || [];
        return json(200, {
          items: symbols.map((symbol) => ({
            symbol,
            quote: quoteForSymbol(symbol),
            quality_status: "valid"
          })),
          total: symbols.length,
          succeeded: symbols.length,
          failed: 0,
          fetched_at: "2026-10-30T07:00:00Z",
          quality_status: "valid"
        });
      }

      if (pathname.includes("/api/")) {
        return json(503, { error: "Browser layout test: backend intentionally unavailable" });
      }

      return route.continue();
    });

    await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
    assert.deepEqual(pageErrors, [], "browser page errors after initial load at " + viewport.width + "px: " + pageErrors.join(" | ") + " details=" + JSON.stringify(await page.evaluate(() => window.__pageErrorDetails || [])));
    await page.waitForFunction(
      () => document.querySelectorAll(".nav-link[data-view]").length === 11 &&
        document.querySelectorAll(".workspace-tab[data-view]").length === 0,
      null,
      { timeout: 10000 }
    );
    assert.equal(await page.locator("#market-view").isVisible(), true, "market overview must be the initial view");

    // Keyboard users must be able to reveal and activate the skip link before traversing the dashboard.
    await page.keyboard.press("Tab");
    const skipLinkState = await page.locator(".skip-link").evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return {
        focused: document.activeElement === element,
        top: rect.top,
        outlineStyle: style.outlineStyle,
        outlineWidth: style.outlineWidth
      };
    });
    await page.waitForFunction(() => {
      const element = document.querySelector(".skip-link");
      if (!element || document.activeElement !== element) return false;
      const top = element.getBoundingClientRect().top;
      return top >= 0 && top < 60;
    }, null, { timeout: 2000 });
    const visibleSkipLinkState = await page.locator(".skip-link").evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return { top: rect.top, outlineStyle: style.outlineStyle, outlineWidth: style.outlineWidth };
    });
    assert.equal(skipLinkState.focused, true, "the first keyboard tab stop should be the skip-to-content link");
    assert.ok(visibleSkipLinkState.top >= 0 && visibleSkipLinkState.top < 60, "the skip link must become visible when focused");
    assert.notEqual(visibleSkipLinkState.outlineStyle, "none", "the focused skip link must have a visible outline");
    assert.notEqual(visibleSkipLinkState.outlineWidth, "0px", "the focused skip link outline must be non-zero");
    assert.notEqual(skipLinkState.outlineWidth, "0px", "the focused skip link outline must be non-zero");

    // Each browser context starts with isolated storage; verify both theme directions and their accessible state.
    const themeToggle = page.locator("#theme-toggle");
    const initialTheme = await page.evaluate(() => ({
      theme: document.documentElement.dataset.theme,
      colorScheme: getComputedStyle(document.documentElement).colorScheme
    }));
    assert.equal(initialTheme.theme, "dark", "a fresh browser context should start in the default dark theme");
    assert.equal(initialTheme.colorScheme, "dark", "dark theme should set the browser color scheme");
    assert.equal(await themeToggle.getAttribute("aria-pressed"), "false", "dark theme toggle should report not pressed");

    await themeToggle.click();
    const lightTheme = await page.evaluate(() => ({
      theme: document.documentElement.dataset.theme,
      colorScheme: getComputedStyle(document.documentElement).colorScheme
    }));
    assert.equal(lightTheme.theme, "light", "theme toggle should apply light theme");
    assert.equal(lightTheme.colorScheme, "light", "light theme should set the browser color scheme");
    assert.equal(await themeToggle.getAttribute("aria-pressed"), "true", "light theme toggle should report pressed");
    assert.equal((await page.locator("#theme-toggle-label").textContent()).trim(), "深色", "light theme toggle should offer the dark theme");

    await themeToggle.click();
    const restoredTheme = await page.evaluate(() => ({
      theme: document.documentElement.dataset.theme,
      colorScheme: getComputedStyle(document.documentElement).colorScheme
    }));
    assert.equal(restoredTheme.theme, "dark", "theme toggle should restore dark theme");
    assert.equal(restoredTheme.colorScheme, "dark", "restored dark theme should set the browser color scheme");
    assert.equal(await themeToggle.getAttribute("aria-pressed"), "false", "restored dark theme should report not pressed");
    assert.equal((await page.locator("#theme-toggle-label").textContent()).trim(), "浅色", "dark theme toggle should offer the light theme");

    // Check local theme persistence once; each viewport uses a fresh isolated browser context.
    if (viewport.width === 320) {
      await themeToggle.click();
      await page.reload({ waitUntil: "load", timeout: 30000 });
      await page.locator("#market-view").waitFor({ state: "visible" });
      const persistedTheme = await page.evaluate(() => ({
        theme: document.documentElement.dataset.theme,
        colorScheme: getComputedStyle(document.documentElement).colorScheme
      }));
      assert.equal(persistedTheme.theme, "light", "light theme preference should persist after page reload");
      assert.equal(persistedTheme.colorScheme, "light", "reloaded light theme should restore browser color scheme");
      assert.equal(await page.locator("#theme-toggle").getAttribute("aria-pressed"), "true", "reloaded light theme should restore pressed state");
      assert.equal((await page.locator("#theme-toggle-label").textContent()).trim(), "深色", "reloaded light theme should offer dark theme");

      await page.locator("#theme-toggle").click();
      assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), "dark", "theme should return to dark after persistence check");
    }

    if (viewport.width === 320) {
      await page.locator("#mobile-menu-toggle").click();
      await clickNav(page, '.nav-link[data-view="stocks"]');
      await page.locator("#stocks-view").waitFor({ state: "visible" });
      const savedSettings = await page.evaluate(() => {
        try { return JSON.parse(localStorage.getItem("stock-provider-webui.settings.v1") || "{}"); }
        catch { return {}; }
      });
      assert.equal(savedSettings.apiKey, "browser-test-key", "the isolated API-error scenario must load its test API key");

      // Date range controls are intentionally hidden at narrow widths; set their values while submitting via the visible form button.
      await page.evaluate(() => {
        const start = document.querySelector("#bar-start-date");
        const end = document.querySelector("#bar-end-date");
        start.value = "2026-10-01";
        end.value = "2026-10-28";
        start.dispatchEvent(new Event("change", { bubbles: true }));
        end.dispatchEvent(new Event("change", { bubbles: true }));
      });
      const submitQuote = page.locator("#quote-form button[type='submit']");

      // Explicitly seed a prior valid quote after configuring the range, instead of depending on startup auto-refresh timing.
      quoteScenario = "success";
      await page.waitForFunction(() => !document.querySelector("#quote-form button[type='submit']")?.disabled, null, { timeout: 5000 });
      await submitQuote.click();
      // Successful refreshes may retain a prior same-symbol quote while loading; only wait for the success payload here.
      await page.waitForFunction(() => document.querySelector("#metric-price")?.textContent.trim() === "123.45", null, { timeout: 10000 });
      await page.locator("#refresh-button").click();
      await page.waitForFunction(() => !document.querySelector("#refresh-button")?.disabled, null, { timeout: 10000 });
      const connectionLabel = await page.locator("#connection-label").textContent();
      assert.ok(
        ["连接异常", "服务状态未知", "连接异常或状态未知", "部分连接"].includes(connectionLabel?.trim()),
        "service refresh must not report a successful connection when health endpoints return failures: " + connectionLabel
      );

      for (const scenario of ["401", "403", "404", "503", "network"]) {
        const expected = quoteErrors[scenario];
        const requestsBeforeFailure = quoteRequestCount;
        quoteScenario = scenario;
        await submitQuote.click();

        await page.waitForFunction(
          (expectedMessage) => document.querySelector("#market-notice")?.textContent.includes(expectedMessage),
          expected.expected,
          { timeout: 8000 }
        );
        assert.equal(quoteRequestCount, requestsBeforeFailure + 1, scenario + " should make one quote request");
        await page.waitForTimeout(250);
        assert.equal(quoteRequestCount, requestsBeforeFailure + 1, scenario + " should not silently retry in the background");

        const failedQuote = await page.evaluate(() => ({
          price: document.querySelector("#metric-price")?.textContent.trim(),
          change: document.querySelector("#metric-change")?.textContent.trim(),
          amount: document.querySelector("#metric-amount")?.textContent.trim(),
          volume: document.querySelector("#metric-volume")?.textContent.trim(),
          turnover: document.querySelector("#metric-turnover")?.textContent.trim(),
          qualityText: document.querySelector("#quote-quality")?.textContent.trim(),
          qualityClass: document.querySelector("#quote-quality")?.className || "",
          quoteTime: document.querySelector("#metric-quote-time")?.textContent.trim(),
          barsMeta: document.querySelector("#bars-meta")?.textContent.trim(),
          barsCaption: document.querySelector("#bars-caption")?.textContent || "",
          barsRows: document.querySelector("#bars-body")?.textContent || "",
          overallConnection: document.querySelector("#connection-dot")?.className || ""
        }));
        assert.equal(failedQuote.price, "—", scenario + " must clear the previous price");
        assert.equal(failedQuote.change, "本次报价不可用", scenario + " must not retain old change data");
        assert.equal(failedQuote.amount, "—", scenario + " must not retain old turnover amount");
        assert.equal(failedQuote.volume, "—", scenario + " must not retain old volume");
        assert.equal(failedQuote.turnover, "—", scenario + " must not retain old turnover rate");
        assert.equal(failedQuote.qualityText, "未取得报价", scenario + " must not present a failed quote as healthy");
        assert.match(failedQuote.qualityClass, /quality-unknown/);
        assert.equal(failedQuote.quoteTime, "行情时间未报告", scenario + " must clear the previous quote timestamp");
        // Quote and daily bars are independent requests: a quote failure must not erase a successful bars response.
        assert.equal(failedQuote.barsMeta, "1 根", scenario + " must retain the successful daily-bars response");
        assert.match(failedQuote.barsCaption, /600519\.SH/, scenario + " daily-bars caption must match the requested symbol");
        assert.match(failedQuote.barsRows, /123\.45/, scenario + " must keep the valid daily close even when Quote fails");
        assert.doesNotMatch(failedQuote.overallConnection, /is-online/, scenario + " must not report healthy overall connectivity");

        // A user-initiated retry must issue exactly one new request and restore data only after success.
        quoteScenario = "success";
        await submitQuote.click();
        await page.waitForFunction(() => document.querySelector("#metric-price")?.textContent.trim() === "—", null, { timeout: 3000 });
        await page.waitForFunction(() => document.querySelector("#metric-price")?.textContent.trim() === "123.45", null, { timeout: 8000 });
        assert.equal(quoteRequestCount, requestsBeforeFailure + 2, scenario + " manual retry should make one new request");
        assert.equal(await page.locator("#quote-quality").textContent().then((value) => value.trim()), "质量正常");
      }

      // Seed a distinguishable MA5 for A, then let A's quote response arrive after switching to B.
      quoteScenario = "ma-seed";
      const maSeedRequestCount = quoteRequestCount;
      await page.waitForFunction(() => !document.querySelector("#quote-form button[type='submit']")?.disabled, null, { timeout: 5000 });
      await submitQuote.click();
      await page.waitForFunction(
        () => document.querySelector("#bars-meta")?.textContent.trim() === "5 根" &&
          /^\d+\.\d{2}$/.test(document.querySelector("#technical-ma-5")?.textContent.trim() || "") &&
          document.querySelector("#quote-quality")?.textContent.trim() === "质量正常",
        null,
        { timeout: 10000 }
      );
      assert.equal(quoteRequestCount, maSeedRequestCount + 1, "MA seed query should issue one quote request");
      const oldSymbolMA5 = (await page.locator("#technical-ma-5").textContent()).trim();
      assert.notEqual(oldSymbolMA5, "样本不足", "the old symbol must have a real MA5 before the race test");

      quoteScenario = "delayed-old";
      await page.evaluate(() => { delete window.__browserTestDelayedQuoteStarted; });
      const staleRequestCount = quoteRequestCount;
      await submitQuote.click();
      await page.waitForFunction(
        () => window.__browserTestDelayedQuoteStarted === true &&
          document.querySelector("#quote-form")?.classList.contains("is-loading") &&
          document.querySelector("#query-hint")?.textContent.includes("正在查询 600519.SH"),
        null,
        { timeout: 5000 }
      );
      assert.equal(quoteRequestCount, staleRequestCount + 1, "the old-symbol request must be in flight before switching symbols");

      await page.locator("#symbol-input").fill("000001.SH");
      await page.waitForFunction(
        () => document.querySelector("#symbol-input")?.value === "000001.SH" &&
          document.querySelector("#metric-price")?.textContent.trim() === "—" &&
          !document.querySelector("#quote-form button[type='submit']")?.disabled,
        null,
        { timeout: 5000 }
      );

      // First fail B's quote while B's daily bars succeed; then manually retry B before A's late response attempts to return.
      quoteScenario = "503";
      await submitQuote.click();
      await page.waitForFunction(
        () => document.querySelector("#market-notice")?.textContent.includes("上游 Provider 暂不可用") &&
          document.querySelector("#metric-price")?.textContent.trim() === "—" &&
          document.querySelector("#quote-quality")?.textContent.trim() === "未取得报价" &&
          document.querySelector("#bars-caption")?.textContent.includes("000001.SH") &&
          document.querySelector("#bars-meta")?.textContent.trim() === "1 根" &&
          document.querySelector("#technical-ma-5")?.textContent.trim() === "样本不足",
        null,
        { timeout: 10000 }
      );
      assert.equal(quoteRequestCount, staleRequestCount + 2, "B's first request should fail Quote while its daily bars request succeeds");
      const partialBState = await page.evaluate(() => ({
        quoteTitle: document.querySelector("#quote-title")?.textContent || "",
        price: document.querySelector("#metric-price")?.textContent.trim() || "",
        quality: document.querySelector("#quote-quality")?.textContent.trim() || "",
        qualityClass: document.querySelector("#quote-quality")?.className || "",
        barsCaption: document.querySelector("#bars-caption")?.textContent || "",
        barsMeta: document.querySelector("#bars-meta")?.textContent.trim() || "",
        barsRows: document.querySelector("#bars-body")?.textContent || "",
        technicalMA5: document.querySelector("#technical-ma-5")?.textContent.trim() || "",
        technicalMA20: document.querySelector("#technical-ma-20")?.textContent.trim() || "",
        overviewSymbol: document.querySelector("#research-overview-symbol")?.textContent || "",
        overviewMA5: document.querySelector("#research-overview-ma5")?.textContent.trim() || ""
      }));
      assert.match(partialBState.quoteTitle, /000001\.SH/);
      assert.equal(partialBState.price, "—", "B's failed Quote must not show A's old price");
      assert.equal(partialBState.quality, "未取得报价");
      assert.match(partialBState.qualityClass, /quality-unknown/);
      assert.match(partialBState.barsCaption, /000001\.SH/);
      assert.equal(partialBState.barsMeta, "1 根");
      assert.match(partialBState.barsRows, /10\.25/, "B's valid daily close should remain visible after the Quote 503");
      assert.equal(partialBState.technicalMA5, "样本不足");
      assert.equal(partialBState.technicalMA20, "样本不足");
      assert.match(partialBState.overviewSymbol, /000001\.SH/);
      assert.equal(partialBState.overviewMA5, "数据不足");
      await page.waitForTimeout(250);
      assert.equal(quoteRequestCount, staleRequestCount + 2, "B's quote failure must not trigger a silent retry");

      quoteScenario = "success";
      await page.waitForFunction(() => !document.querySelector("#quote-form button[type='submit']")?.disabled, null, { timeout: 5000 });
      await submitQuote.click();
      await page.waitForFunction(
        () => document.querySelector("#quote-title")?.textContent.includes("000001.SH") &&
          document.querySelector("#metric-price")?.textContent.trim() === "10.25" &&
          document.querySelector("#quote-quality")?.textContent.trim() === "质量正常" &&
          document.querySelector("#bars-caption")?.textContent.includes("000001.SH") &&
          document.querySelector("#bars-meta")?.textContent.trim() === "1 根" &&
          document.querySelector("#technical-ma-5")?.textContent.trim() === "样本不足" &&
          document.querySelector("#technical-ma-20")?.textContent.trim() === "样本不足",
        null,
        { timeout: 10000 }
      );
      assert.equal(quoteRequestCount, staleRequestCount + 3, "manual retry should issue exactly one B quote request");
      const newSymbolState = await page.evaluate(() => ({
        quoteTitle: document.querySelector("#quote-title")?.textContent || "",
        price: document.querySelector("#metric-price")?.textContent.trim() || "",
        quality: document.querySelector("#quote-quality")?.textContent.trim() || "",
        barsCaption: document.querySelector("#bars-caption")?.textContent || "",
        barsMeta: document.querySelector("#bars-meta")?.textContent.trim() || "",
        technicalMA5: document.querySelector("#technical-ma-5")?.textContent.trim() || "",
        technicalMA20: document.querySelector("#technical-ma-20")?.textContent.trim() || "",
        overviewSymbol: document.querySelector("#research-overview-symbol")?.textContent || "",
        overviewMA5: document.querySelector("#research-overview-ma5")?.textContent.trim() || ""
      }));
      assert.match(newSymbolState.quoteTitle, /测试新标的.*000001\.SH/);
      assert.equal(newSymbolState.price, "10.25");
      assert.equal(newSymbolState.quality, "质量正常");
      assert.match(newSymbolState.barsCaption, /000001\.SH/);
      assert.match(newSymbolState.overviewSymbol, /000001\.SH/);
      assert.notEqual(newSymbolState.technicalMA5, oldSymbolMA5, "the old symbol's MA5 must not survive the new-symbol response");
      assert.equal(newSymbolState.technicalMA5, "样本不足");
      assert.equal(newSymbolState.technicalMA20, "样本不足");
      assert.equal(newSymbolState.overviewMA5, "数据不足");

      // Let the old request's delayed server response attempt to arrive after B has failed and then recovered.
      await page.waitForTimeout(1800);
      const finalSymbolState = await page.evaluate(() => ({
        quoteTitle: document.querySelector("#quote-title")?.textContent || "",
        price: document.querySelector("#metric-price")?.textContent.trim() || "",
        quality: document.querySelector("#quote-quality")?.textContent.trim() || "",
        barsCaption: document.querySelector("#bars-caption")?.textContent || "",
        technicalMA5: document.querySelector("#technical-ma-5")?.textContent.trim() || "",
        technicalMA20: document.querySelector("#technical-ma-20")?.textContent.trim() || ""
      }));
      assert.match(finalSymbolState.quoteTitle, /测试新标的.*000001\.SH/);
      assert.equal(finalSymbolState.price, "10.25", "a late A quote must not overwrite B's price");
      assert.equal(finalSymbolState.quality, "质量正常", "a late A quote must not overwrite B's quality status");
      assert.match(finalSymbolState.barsCaption, /000001\.SH/, "a late A response must not overwrite B's daily bars");
      assert.equal(finalSymbolState.technicalMA5, "样本不足", "a late A response must not restore A's MA5");
      assert.equal(finalSymbolState.technicalMA20, "样本不足", "a late A response must not restore A's MA20");
      assert.equal(quoteRequestCount, staleRequestCount + 3, "late response must not cause a hidden additional request");

      // Local watchlist data must never be silently repaired or reported as saved after browser storage errors.
      const watchlistStorageKey = "stock-provider-webui.watchlist.v1";
      await page.evaluate((key) => localStorage.setItem(key, "{malformed"), watchlistStorageKey);
      await page.locator("#add-to-watchlist-button").click();
      await page.waitForFunction(
        () => document.querySelector("#watchlist-feedback")?.textContent.includes("本地自选数据格式异常，未添加或覆盖任何数据"),
        null,
        { timeout: 5000 }
      );
      const malformedWatchlistState = await page.evaluate((key) => ({
        stored: localStorage.getItem(key),
        count: document.querySelector("#watchlist-count")?.textContent.trim() || "",
        list: document.querySelector("#watchlist-list")?.textContent || "",
        feedback: document.querySelector("#watchlist-feedback")?.textContent || "",
        feedbackClass: document.querySelector("#watchlist-feedback")?.className || ""
      }), watchlistStorageKey);
      assert.equal(malformedWatchlistState.stored, "{malformed", "a malformed stored watchlist must remain byte-for-byte unchanged");
      assert.equal(malformedWatchlistState.count, "不可用 / 50");
      assert.match(malformedWatchlistState.list, /本地自选数据格式异常/);
      assert.match(malformedWatchlistState.feedbackClass, /watchlist-feedback-error/);
      assert.doesNotMatch(malformedWatchlistState.feedback, /已将 .* 加入本地自选/);

      await page.evaluate((key) => {
        localStorage.removeItem(key);
        window.__browserTestOriginalSetItem = Storage.prototype.setItem;
        Storage.prototype.setItem = function(storageKey, value) {
          if (storageKey === key) throw new DOMException("Quota exceeded", "QuotaExceededError");
          return window.__browserTestOriginalSetItem.call(this, storageKey, value);
        };
      }, watchlistStorageKey);
      await page.locator("#add-to-watchlist-button").click();
      await page.waitForFunction(
        () => document.querySelector("#watchlist-feedback")?.textContent.includes("已在当前页面临时加入自选，但浏览器存储不可用，刷新后可能丢失"),
        null,
        { timeout: 5000 }
      );
      const failedWatchlistWrite = await page.evaluate((key) => ({
        stored: localStorage.getItem(key),
        count: document.querySelector("#watchlist-count")?.textContent.trim() || "",
        list: document.querySelector("#watchlist-list")?.textContent || "",
        feedback: document.querySelector("#watchlist-feedback")?.textContent || "",
        feedbackClass: document.querySelector("#watchlist-feedback")?.className || ""
      }), watchlistStorageKey);
      assert.equal(failedWatchlistWrite.stored, null, "a failed storage write must not leave a misleading saved value");
      assert.equal(failedWatchlistWrite.count, "1 / 50");
      assert.match(failedWatchlistWrite.list, /000001\.SH/);
      assert.match(failedWatchlistWrite.feedbackClass, /watchlist-feedback-warning/);
      assert.match(failedWatchlistWrite.feedback, /刷新后可能丢失/);
      assert.doesNotMatch(failedWatchlistWrite.feedback, /已将 .* 加入本地自选。/);
      await page.evaluate(() => {
        if (window.__browserTestOriginalSetItem) {
          Storage.prototype.setItem = window.__browserTestOriginalSetItem;
          delete window.__browserTestOriginalSetItem;
        }
      });

      // When storage reads are denied, do not treat the list as empty or rewrite the unreadable original bytes.
      const seedWatchlistRaw = JSON.stringify([{
        symbol: "600519.SH",
        name: "贵州茅台",
        updated_at: "2026-10-29T05:00:00Z"
      }]);
      await page.evaluate(({ key, raw }) => {
        window.__browserTestOriginalGetItem = Storage.prototype.getItem;
        localStorage.setItem(key, raw);
        Storage.prototype.getItem = function(storageKey) {
          if (storageKey === key) throw new DOMException("Blocked by browser settings", "SecurityError");
          return window.__browserTestOriginalGetItem.call(this, storageKey);
        };
      }, { key: watchlistStorageKey, raw: seedWatchlistRaw });
      await page.locator("#add-to-watchlist-button").click();
      await page.waitForFunction(
        () => document.querySelector("#watchlist-feedback")?.textContent.includes("无法读取浏览器自选股存储，未添加或覆盖任何数据"),
        null,
        { timeout: 5000 }
      );
      const unreadableWatchlistState = await page.evaluate((key) => ({
        stored: window.__browserTestOriginalGetItem.call(localStorage, key),
        count: document.querySelector("#watchlist-count")?.textContent.trim() || "",
        list: document.querySelector("#watchlist-list")?.textContent || "",
        feedback: document.querySelector("#watchlist-feedback")?.textContent || "",
        feedbackClass: document.querySelector("#watchlist-feedback")?.className || ""
      }), watchlistStorageKey);
      assert.equal(unreadableWatchlistState.stored, seedWatchlistRaw, "denied reads must not overwrite the original watchlist bytes");
      assert.equal(unreadableWatchlistState.count, "不可用 / 50");
      assert.match(unreadableWatchlistState.list, /无法读取浏览器自选股存储/);
      assert.match(unreadableWatchlistState.feedbackClass, /watchlist-feedback-error/);
      assert.match(unreadableWatchlistState.feedback, /未添加或覆盖任何数据/);
      assert.doesNotMatch(unreadableWatchlistState.feedback, /已将 .* 加入本地自选/);
      await page.evaluate((key) => {
        if (window.__browserTestOriginalGetItem) {
          Storage.prototype.getItem = window.__browserTestOriginalGetItem;
          delete window.__browserTestOriginalGetItem;
        }
        localStorage.removeItem(key);
      }, watchlistStorageKey);

      await clickNav(page, '.nav-link[data-view="market"]');
      await page.locator("#market-view").waitFor({ state: "visible" });

      // Seed a visible prior success so every failure also proves the old market result is cleared.
      marketScenario = "success";
      const seedRequestCount = marketIndexRequestCount;
      await page.locator("#refresh-market-overview").click();
      await page.waitForFunction(
        () => document.querySelector("#market-index-grid .market-index-card")?.textContent.includes("测试指数"),
        null,
        { timeout: 8000 }
      );
      assert.equal(marketIndexRequestCount, seedRequestCount + 1, "market overview success seed should request indices once");
      assert.match(await page.locator("#market-index-grid").textContent(), /测试指数/);

      // Home-market errors must preserve HTTP meaning instead of asserting every 401/403 is an API-key fault.
      for (const scenario of ["401", "403", "404", "503", "network"]) {
        const expected = marketOverviewErrors[scenario];
        const requestsBeforeFailure = marketIndexRequestCount;
        marketScenario = scenario;
        await page.locator("#refresh-market-overview").click();
        await page.waitForFunction(
          (expectedMessage) => document.querySelector("#market-index-grid")?.textContent.includes(expectedMessage),
          expected.expected,
          { timeout: 8000 }
        );
        assert.equal(marketIndexRequestCount, requestsBeforeFailure + 1, "market overview " + scenario + " should request indices once");
        await page.waitForTimeout(250);
        assert.equal(marketIndexRequestCount, requestsBeforeFailure + 1, "market overview " + scenario + " should not retry silently");

        const indexState = await page.evaluate(() => ({
          message: document.querySelector("#market-index-grid")?.textContent || "",
          status: document.querySelector("#market-indices-status")?.textContent.trim() || "",
          className: document.querySelector("#market-indices-status")?.className || "",
          refreshSummary: document.querySelector("#market-overview-updated")?.textContent || ""
        }));
        assert.match(indexState.message, new RegExp(expected.expected));
        assert.doesNotMatch(indexState.message, /测试指数/, scenario + " must clear the previous index cards");
        assert.doesNotMatch(indexState.message, /行情服务 API Key 验证失败/);
        assert.equal(indexState.status, "读取失败");
        assert.match(indexState.className, /is-unavailable/);
        assert.ok(indexState.refreshSummary.includes(expected.summary), "market overview " + scenario + " should have a distinct summary");
      }

      // A manual refresh after any market error must restore a successful index response without a hidden duplicate request.
      marketScenario = "success";
      const marketRecoveryCount = marketIndexRequestCount;
      await page.waitForFunction(
        () => !document.querySelector("#refresh-market-overview")?.disabled,
        null,
        { timeout: 10000 }
      );
      await page.locator("#refresh-market-overview").click();
      await page.waitForFunction(
        () => document.querySelector("#market-index-grid")?.textContent.includes("测试指数"),
        null,
        { timeout: 10000 }
      );
      assert.equal(marketIndexRequestCount, marketRecoveryCount + 1, "manual market overview retry should make exactly one new index request");
      const recoveredIndexState = await page.evaluate(() => ({
        status: document.querySelector("#market-indices-status")?.textContent.trim() || "",
        className: document.querySelector("#market-indices-status")?.className || "",
        message: document.querySelector("#market-index-grid")?.textContent || ""
      }));
      assert.match(recoveredIndexState.message, /测试指数/);
      assert.doesNotMatch(recoveredIndexState.status, /读取失败/);
      assert.doesNotMatch(recoveredIndexState.className, /is-unavailable/);
      await page.waitForTimeout(250);
      assert.equal(marketIndexRequestCount, marketRecoveryCount + 1, "manual market overview retry should not silently duplicate the request");

      // 自选股只在用户主动刷新时调用批量行情接口，不逐只请求。
      const watchlistKey = "stock-provider-webui.watchlist.v1";
      const previousWatchlist = await page.evaluate((key) => localStorage.getItem(key), watchlistKey);
      await page.evaluate((key) => localStorage.setItem(key, JSON.stringify([
        { symbol: "600519.SH", name: "测试股票", updated_at: "2026-10-30T07:00:00Z" }
      ])), watchlistKey);
      await clickNav(page, '.nav-link[data-view="watchlist"]');
      await page.locator("#watchlist-view").waitFor({ state: "visible" });
      const batchQuotesBefore = batchQuotesRequestCount;
      await page.locator("#refresh-watchlist-quotes").click();
      await page.waitForFunction(
        () => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("1 / 1 只") &&
          document.querySelector("#watchlist-list")?.textContent.includes("123.45"),
        null,
        { timeout: 10000 }
      );
      assert.equal(batchQuotesRequestCount, batchQuotesBefore + 1, "watchlist refresh must use one batch request, not one request per symbol");
      assert.match(await page.locator("#watchlist-quote-refresh-status").textContent(), /整体质量/);
      await page.evaluate(({ key, value }) => {
        if (value === null) localStorage.removeItem(key);
        else localStorage.setItem(key, value);
      }, { key: watchlistKey, value: previousWatchlist });
      await clickNav(page, '.nav-link[data-view="market"]');
      await page.locator("#market-view").waitFor({ state: "visible" });

      // Data Center must classify Provider-list errors independently from service probes and clear old capability rows.
      const menu = page.locator("#mobile-menu-toggle");
      if (await menu.isVisible() && (await menu.getAttribute("aria-expanded")) !== "true") {
        await menu.click();
      }
      await clickNav(page, '.nav-link[data-view="data"]');
      await page.locator("#data-view").waitFor({ state: "visible" });

      dataSourceScenario = "success";
      const dataSeedCount = dataSourcesRequestCount;
      const diagnosticsSeedCount = diagnosticsRequestCount;
      await page.locator("#data-center-check").click();
      await page.waitForFunction(
        () => document.querySelector("#data-center-provider-list")?.textContent.includes("测试 Provider") &&
          document.querySelector("#data-center-diagnostics-list")?.textContent.includes("尚无业务观测"),
        null,
        { timeout: 10000 }
      );
      assert.equal(dataSourcesRequestCount, dataSeedCount + 1, "data center success seed should request Provider list once");
      assert.equal(diagnosticsRequestCount, diagnosticsSeedCount + 1, "data center should read runtime diagnostics once");
      const diagnosticState = await page.evaluate(() => ({
        status: document.querySelector("#data-center-diagnostics-status")?.textContent.trim() || "",
        summary: document.querySelector("#data-center-diagnostics-summary")?.textContent || "",
        rows: document.querySelector("#data-center-diagnostics-list")?.textContent || ""
      }));
      assert.match(diagnosticState.status, /部分能力尚无观测/);
      assert.match(diagnosticState.summary, /正常 1/);
      assert.match(diagnosticState.summary, /未观测 1/);
      assert.match(diagnosticState.rows, /累计成功 3/);

      // A pending refresh must not display a prior Provider record as the current connection's result.
      dataSourceScenario = "delayed-success";
      const pendingRequestCount = dataSourcesRequestCount;
      await page.locator("#data-center-check").click();
      await page.waitForFunction(() => {
        const summary = document.querySelector("#data-center-provider-summary")?.textContent || "";
        const list = document.querySelector("#data-center-provider-list")?.textContent || "";
        return summary.includes("旧清单暂不展示") &&
          list.includes("正在读取 Provider 能力清单") &&
          !list.includes("测试 Provider");
      }, null, { timeout: 5000 });
      assert.equal(dataSourcesRequestCount, pendingRequestCount + 1, "pending Provider refresh should make one request");
      await page.waitForFunction(
        () => document.querySelector("#data-center-provider-list")?.textContent.includes("测试 Provider"),
        null,
        { timeout: 10000 }
      );
      dataSourceScenario = "success";

      // HTTP 200 without a recognized status is unknown, not healthy/green.
      healthScenario = "missing-status";
      await page.locator("#data-center-check").click();
      await page.waitForFunction(
        () => document.querySelector("#data-service-market-ready")?.textContent.includes("状态未报告") &&
          document.querySelector("#data-service-analysis-ready")?.textContent.includes("状态未报告"),
        null,
        { timeout: 10000 }
      );
      const missingHealthStatus = await page.evaluate(() => ({
        marketText: document.querySelector("#data-service-market-ready")?.textContent.trim() || "",
        marketClass: document.querySelector("#data-service-market-ready")?.className || "",
        analysisText: document.querySelector("#data-service-analysis-ready")?.textContent.trim() || "",
        analysisClass: document.querySelector("#data-service-analysis-ready")?.className || "",
        mariadb: document.querySelector("#data-service-mariadb-status")?.textContent.trim() || "",
        redis: document.querySelector("#data-service-redis-status")?.textContent.trim() || ""
      }));
      assert.match(missingHealthStatus.marketText, /状态未报告/);
      assert.match(missingHealthStatus.marketClass, /is-warning/);
      assert.doesNotMatch(missingHealthStatus.marketClass, /is-local|is-success/);
      assert.match(missingHealthStatus.analysisText, /状态未报告/);
      assert.match(missingHealthStatus.analysisClass, /is-warning/);
      assert.doesNotMatch(missingHealthStatus.analysisClass, /is-local|is-success/);
      assert.equal(missingHealthStatus.mariadb, "未知");
      assert.equal(missingHealthStatus.redis, "未知");

      healthScenario = "unavailable";
      await page.waitForFunction(() => !document.querySelector("#data-center-check")?.disabled, null, { timeout: 10000 });

      for (const scenario of ["401", "403", "404", "503", "network"]) {
        const expected = dataSourceErrors[scenario];
        const requestsBeforeFailure = dataSourcesRequestCount;
        dataSourceScenario = scenario;
        const checkButton = page.locator("#data-center-check");
        await page.waitForFunction(() => !document.querySelector("#data-center-check")?.disabled, null, { timeout: 10000 });
        await checkButton.click();

        await page.waitForFunction(
          (expectedMessage) => document.querySelector("#data-center-provider-summary")?.textContent.includes(expectedMessage),
          expected.expected,
          { timeout: 10000 }
        );
        assert.equal(dataSourcesRequestCount, requestsBeforeFailure + 1, "data center " + scenario + " should request Provider list once");
        await page.waitForTimeout(250);
        assert.equal(dataSourcesRequestCount, requestsBeforeFailure + 1, "data center " + scenario + " should not retry silently");

        const providerState = await page.evaluate(() => ({
          summary: document.querySelector("#data-center-provider-summary")?.textContent || "",
          overallSummary: document.querySelector("#data-center-summary-message")?.textContent || "",
          status: document.querySelector("#data-center-provider-status")?.textContent.trim() || "",
          className: document.querySelector("#data-center-provider-status")?.className || "",
          list: document.querySelector("#data-center-provider-list")?.textContent || "",
          mariadb: document.querySelector("#data-service-mariadb-status")?.textContent.trim() || "",
          redis: document.querySelector("#data-service-redis-status")?.textContent.trim() || ""
        }));
        assert.match(providerState.summary, new RegExp(expected.expected));
        assert.match(providerState.overallSummary, /Provider 能力清单读取失败/, scenario + " should be visible in the overall data-center summary");
        assert.equal(providerState.status, "读取失败");
        assert.match(providerState.className, /is-unavailable/);
        assert.doesNotMatch(providerState.list, /测试 Provider/, scenario + " must clear the prior Provider row");
        assert.match(providerState.list, /仅 Provider 清单请求失败/);
        assert.equal(providerState.mariadb, "未知", "unavailable analysis readiness must not guess MariaDB failure");
        assert.equal(providerState.redis, "未知", "unavailable analysis readiness must not guess Redis failure");
      }

      // A failed Provider-list read is recoverable: the next user-triggered check must repopulate the current list.
      dataSourceScenario = "success";
      const recoveryRequestCount = dataSourcesRequestCount;
      await page.waitForFunction(() => !document.querySelector("#data-center-check")?.disabled, null, { timeout: 10000 });
      await page.locator("#data-center-check").click();
      await page.waitForFunction(
        () => document.querySelector("#data-center-provider-list")?.textContent.includes("测试 Provider") &&
          document.querySelector("#data-center-provider-status")?.textContent.trim() === "已读取能力声明",
        null,
        { timeout: 10000 }
      );
      assert.equal(dataSourcesRequestCount, recoveryRequestCount + 1, "a manual Provider retry should make exactly one new request");
      assert.doesNotMatch(
        await page.locator("#data-center-provider-list").textContent(),
        /仅 Provider 清单请求失败/,
        "successful retry should clear stale error content"
      );
      await page.waitForTimeout(250);
      assert.equal(dataSourcesRequestCount, recoveryRequestCount + 1, "successful retry should not trigger a hidden duplicate request");

      // Changing the market connection while the Provider read is in flight must invalidate its late response.
      dataSourceScenario = "delayed-success";
      const inFlightRequestCount = dataSourcesRequestCount;
      await page.locator("#data-center-check").click();
      await page.waitForFunction(() => {
        const summary = document.querySelector("#data-center-provider-summary")?.textContent || "";
        const list = document.querySelector("#data-center-provider-list")?.textContent || "";
        return summary.includes("旧清单暂不展示") &&
          list.includes("正在读取 Provider 能力清单") &&
          !list.includes("测试 Provider");
      }, null, { timeout: 5000 });
      assert.equal(dataSourcesRequestCount, inFlightRequestCount + 1, "in-flight Provider check should issue one request");

      await page.locator("#open-settings").click();
      await page.locator("#settings-view").waitFor({ state: "visible" });
      await page.locator("#market-base-input").fill("/market-api-v2");
      await page.waitForFunction(() => !document.querySelector("#save-settings")?.disabled, null, { timeout: 3000 });
      await page.locator("#save-settings").click();
      await page.waitForFunction(() => {
        const summary = document.querySelector("#data-center-summary-message")?.textContent || "";
        const list = document.querySelector("#data-center-provider-list")?.textContent || "";
        return !document.querySelector("#settings-view")?.hidden &&
          summary.includes("连接配置已变更") &&
          !list.includes("测试 Provider");
      }, null, { timeout: 5000 });

      // Allow the old request to resolve; its older generation still must not repopulate Provider state.
      await page.waitForTimeout(1800);
      const staleProviderState = await page.evaluate(() => ({
        summary: document.querySelector("#data-center-provider-summary")?.textContent || "",
        list: document.querySelector("#data-center-provider-list")?.textContent || "",
        status: document.querySelector("#data-center-provider-status")?.textContent.trim() || "",
        buttonText: document.querySelector("#data-center-check")?.textContent.trim() || "",
        buttonDisabled: document.querySelector("#data-center-check")?.disabled
      }));
      assert.match(staleProviderState.summary, /连接配置已变更/);
      assert.doesNotMatch(staleProviderState.list, /测试 Provider/, "late old-connection response must not repopulate Provider list");
      assert.equal(staleProviderState.status, "需重新检查");
      assert.equal(staleProviderState.buttonText, "立即检查");
      assert.equal(staleProviderState.buttonDisabled, false);
      assert.equal(dataSourcesRequestCount, inFlightRequestCount + 1, "invalidated old request must not cause a hidden follow-up request");

      dataSourceScenario = "success";

      // 设置中心现在是一级页面，不再使用旧的连接设置弹窗。
      await page.locator("#open-settings").click();
      await page.locator("#settings-view").waitFor({ state: "visible" });
      assert.equal(await page.locator("#settings-view").isVisible(), true);
      await clickNav(page, '.nav-link[data-view="market"]');
      await page.locator("#market-view").waitFor({ state: "visible" });

      // Saved connection and unsaved draft checks must preserve error distinctions and remain isolated.
      const settingsMenu = page.locator("#mobile-menu-toggle");
      if (await settingsMenu.isVisible() && (await settingsMenu.getAttribute("aria-expanded")) !== "true") {
        await settingsMenu.click();
      }
      await clickNav(page, '.nav-link[data-view="settings"]');
      await page.locator("#settings-view").waitFor({ state: "visible" });

      const savedSettingsBeforeChecks = await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1"));
      assert.ok(savedSettingsBeforeChecks, "connection checks require persisted local settings");
      const parsedSavedSettings = JSON.parse(savedSettingsBeforeChecks);
      assert.match(parsedSavedSettings.marketBase, /^\/[a-z0-9-]+$/);
      assert.match(parsedSavedSettings.analysisBase, /^\/[a-z0-9-]+$/);
      assert.equal(parsedSavedSettings.apiKey, "browser-test-key");

      for (const scenario of ["401", "403", "404", "503", "network"]) {
        const expected = settingsConnectionErrors[scenario];
        const requestsBefore = settingsConnectionRequestCount;
        settingsConnectionScenario = scenario;
        await page.locator("#settings-check-connection").click();
        await page.waitForFunction(
          (expectedMessage) => document.querySelector("#settings-connection-check")?.textContent.includes(expectedMessage),
          expected.expected,
          { timeout: 10000 }
        );
        assert.equal(settingsConnectionRequestCount, requestsBefore + 2,
          "saved connection " + scenario + " should check market and analysis endpoints once each");
        await page.waitForTimeout(250);
        assert.equal(settingsConnectionRequestCount, requestsBefore + 2,
          "saved connection " + scenario + " must not retry silently");
        assert.match(await page.locator("#settings-connection-check").textContent(), /已保存连接配置检查结果：/);
        assert.match(await page.locator("#settings-connection-check").getAttribute("class"), /is-warning/);
        assert.equal(await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1")), savedSettingsBeforeChecks,
          "saved connection check " + scenario + " must not mutate local settings");
        await page.waitForFunction(
          () => !document.querySelector("#settings-check-connection")?.disabled,
          null,
          { timeout: 3000 }
        );
      }
      const savedCheckTextBeforeDraft = await page.locator("#settings-connection-check").textContent();
      const savedSettingsBeforeDraft = await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1"));

      await page.locator("#open-settings").click();
      await page.locator("#settings-view").waitFor({ state: "visible" });
      await page.locator("#market-base-input").fill("/draft-market-api");
      await page.locator("#analysis-base-input").fill("/draft-analysis-api");
      assert.equal(await page.locator("#save-settings").isDisabled(), false,
        "changing API paths should mark settings as an unsaved draft");

      for (const scenario of ["401", "403", "404", "503", "network"]) {
        const expected = settingsConnectionErrors[scenario];
        const requestsBefore = settingsConnectionRequestCount;
        settingsConnectionScenario = scenario;
        await page.locator("#settings-check-draft-connection").click();
        await page.waitForFunction(
          (expectedMessage) => document.querySelector("#settings-draft-connection-check")?.textContent.includes(expectedMessage),
          expected.expected,
          { timeout: 10000 }
        );
        assert.equal(settingsConnectionRequestCount, requestsBefore + 2,
          "draft connection " + scenario + " should check its two unsaved paths once each");
        await page.waitForTimeout(250);
        assert.equal(settingsConnectionRequestCount, requestsBefore + 2,
          "draft connection " + scenario + " must not retry silently");
        const draftText = await page.locator("#settings-draft-connection-check").textContent();
        assert.match(draftText, /表单地址检查结果（尚未保存）：/);
        assert.match(draftText, new RegExp(expected.expected));
        assert.match(await page.locator("#settings-draft-connection-check").getAttribute("class"), /is-warning/);
        assert.equal(await page.locator("#settings-connection-check").textContent(), savedCheckTextBeforeDraft,
          "draft connection " + scenario + " must not overwrite saved-check feedback");
        assert.equal(await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1")), savedSettingsBeforeDraft,
          "draft connection " + scenario + " must not persist unsaved API paths");
        await page.waitForFunction(
          () => !document.querySelector("#settings-check-draft-connection")?.disabled,
          null,
          { timeout: 3000 }
        );
      }

      settingsConnectionScenario = "default";
      await clickNav(page, '.nav-link[data-view="market"]');
      await page.locator("#market-view").waitFor({ state: "visible" });
      await clickNav(page, '.nav-link[data-view="settings"]');
      await page.locator("#settings-view").waitFor({ state: "visible" });
      assert.equal(await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1")), savedSettingsBeforeChecks,
        "discarding an unsaved draft must preserve the saved local settings");

      // A health check started on the previous connection must not overwrite the invalidation message after saving new paths.
      const requestsBeforeStaleHealth = settingsConnectionRequestCount;
      settingsConnectionScenario = "delayed";
      await page.locator("#settings-check-connection").click();
      await page.waitForFunction(() => document.querySelector("#settings-check-connection")?.disabled === true, null, { timeout: 3000 });
      await page.waitForTimeout(120);
      assert.equal(settingsConnectionRequestCount, requestsBeforeStaleHealth + 2,
        "the stale-health race must have both old endpoint requests in flight");

      settingsConnectionScenario = "default";
      await page.locator("#open-settings").click();
      await page.locator("#settings-view").waitFor({ state: "visible" });
      await page.locator("#market-base-input").fill("/race-market-api");
      await page.locator("#analysis-base-input").fill("/race-analysis-api");
      await page.locator("#save-settings").click();
      await page.locator("#settings-view").waitFor({ state: "visible" });
      await page.waitForFunction(() => {
        try {
          const settings = JSON.parse(localStorage.getItem("stock-provider-webui.settings.v1") || "{}");
          return settings.marketBase === "/race-market-api" && settings.analysisBase === "/race-analysis-api";
        } catch {
          return false;
        }
      }, null, { timeout: 5000 });
      await page.waitForFunction(
        () => document.querySelector("#settings-connection-check")?.textContent === "连接配置已变化，请重新检查当前已保存的配置。",
        null,
        { timeout: 5000 }
      );

      await page.waitForTimeout(1700);
      assert.equal(settingsConnectionRequestCount, requestsBeforeStaleHealth + 2,
        "late old health responses must not initiate hidden requests");
      assert.equal(
        await page.locator("#settings-connection-check").textContent(),
        "连接配置已变化，请重新检查当前已保存的配置。",
        "late responses from the old API paths must not overwrite the new-configuration warning"
      );
      assert.doesNotMatch(await page.locator("#settings-connection-check").textContent(), /健康探针服务暂不可用|HTTP 503/);
      assert.equal(await page.locator("#settings-check-connection").isDisabled(), false,
        "saving new connection paths must release the saved-check button even while old requests are pending");

      // A draft check whose endpoints change while in flight must report the old result as stale.
      const savedSettingsBeforeDraftRace = await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1"));
      const savedFeedbackBeforeDraftRace = await page.locator("#settings-connection-check").textContent();
      await page.locator("#open-settings").click();
      await page.locator("#settings-view").waitFor({ state: "visible" });
      await page.locator("#market-base-input").fill("/draft-race-market");
      await page.locator("#analysis-base-input").fill("/draft-race-analysis");

      const requestsBeforeDraftRace = settingsConnectionRequestCount;
      settingsConnectionScenario = "delayed";
      await page.locator("#settings-check-draft-connection").click();
      await page.waitForFunction(
        () => document.querySelector("#settings-check-draft-connection")?.disabled === true,
        null,
        { timeout: 3000 }
      );
      await page.waitForTimeout(120);
      assert.equal(settingsConnectionRequestCount, requestsBeforeDraftRace + 2,
        "draft race should have both old draft health probes in flight");

      settingsConnectionScenario = "default";
      await page.locator("#market-base-input").fill("/edited-market-draft");
      await page.locator("#analysis-base-input").fill("/edited-analysis-draft");
      await page.waitForFunction(
        () => document.querySelector("#settings-draft-connection-check")?.textContent.includes("检查期间连接草稿或已保存配置发生变化，已忽略旧结果"),
        null,
        { timeout: 5000 }
      );

      const staleDraftFeedback = await page.locator("#settings-draft-connection-check").textContent();
      assert.doesNotMatch(staleDraftFeedback, /健康探针服务暂不可用|HTTP 503/,
        "old delayed draft responses must not overwrite the stale-result warning");
      assert.equal(settingsConnectionRequestCount, requestsBeforeDraftRace + 2,
        "stale draft results must not trigger any extra requests");
      assert.equal(await page.locator("#settings-connection-check").textContent(), savedFeedbackBeforeDraftRace,
        "draft edits/results must not change saved-check feedback");
      assert.equal(await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1")), savedSettingsBeforeDraftRace,
        "editing a draft while its check is in flight must not persist the new draft paths");

      settingsConnectionScenario = "default";
      await clickNav(page, '.nav-link[data-view="market"]');
      await page.locator("#market-view").waitFor({ state: "visible" });
      await clickNav(page, '.nav-link[data-view="settings"]');
      await page.locator("#settings-view").waitFor({ state: "visible" });
      assert.equal(await page.evaluate(() => localStorage.getItem("stock-provider-webui.settings.v1")), savedSettingsBeforeDraftRace,
        "discarding the raced draft must preserve the existing saved endpoints");

      await clickNav(page, '.nav-link[data-view="market"]');
      await page.locator("#market-view").waitFor({ state: "visible" });

      // The topbar health summary must use the same evidence threshold as Data Center.
      healthScenario = "missing-status";
      const refreshButton = page.locator("#refresh-button");
      await page.waitForFunction(() => !document.querySelector("#refresh-button")?.disabled, null, { timeout: 10000 });
      await refreshButton.click();
      await page.waitForFunction(
        () => document.querySelector("#connection-label")?.textContent === "服务状态未知" &&
          document.querySelector("#connection-dot")?.classList.contains("is-unknown"),
        null,
        { timeout: 10000 }
      );
      const topbarHealth = await page.evaluate(() => ({
        connectionLabel: document.querySelector("#connection-label")?.textContent.trim() || "",
        connectionDot: document.querySelector("#connection-dot")?.className || ""
      }));
      assert.equal(topbarHealth.connectionLabel, "服务状态未知");
      assert.doesNotMatch(topbarHealth.connectionDot, /is-online/);
      assert.match(topbarHealth.connectionDot, /is-unknown/);
      healthScenario = "unavailable";
    }

    const assertNoHorizontalOverflow = async (stage) => {
      const dimensions = await page.evaluate(() => ({
        viewport: window.innerWidth,
        document: document.documentElement.scrollWidth
      }));
      assert.ok(
        dimensions.document <= dimensions.viewport + 1,
        stage + " has horizontal page overflow at " + viewport.width + "px: " +
          dimensions.document + "px document for " + dimensions.viewport + "px viewport"
      );
    };

    await assertNoHorizontalOverflow("homepage");
    await page.screenshot({
      path: path.join(screenshotDir, viewport.width + "x" + viewport.height + "-home.png"),
      fullPage: true,
      animations: "disabled"
    });

    const menu = page.locator("#mobile-menu-toggle");
    for (const entry of viewNavigation) {
      const viewName = entry.name;
      if (entry.selector.startsWith(".nav-link") &&
          await menu.isVisible() &&
          (await menu.getAttribute("aria-expanded")) !== "true") {
        await menu.click();
      }
      const navigationButton = page.locator(entry.selector);
      await navigationButton.click();
      assert.deepEqual(pageErrors, [], "browser page errors after navigating to " + viewName + " at " + viewport.width + "px: " + pageErrors.join(" | "));
      await page.locator("#" + viewName + "-view").waitFor({ state: "visible" });
      assert.equal(
        await page.locator(".view:not([hidden])").count(),
        1,
        "exactly one workspace should be visible after navigating to " + viewName
      );
      assert.equal(
        await navigationButton.getAttribute("aria-current"),
        "page",
        "navigation should mark " + viewName + " as the current page"
      );
      await assertNoHorizontalOverflow("view " + viewName);
      if (viewName === "market" && viewport.width === 320) {
        const range = await page.evaluate(() => ({
          start: document.querySelector("#market-capital-flow-start")?.value || "",
          end: document.querySelector("#market-capital-flow-end")?.value || ""
        }));
        const days = (Date.parse(range.end + "T00:00:00Z") - Date.parse(range.start + "T00:00:00Z")) / 86400000 + 1;
        assert.equal(days, 90, "default market capital-flow range must include exactly 90 calendar dates");
        await page.locator("#market-capital-flow-start").fill("2026-10-01");
        await page.locator("#market-capital-flow-end").fill("2026-10-30");
        await page.locator("#load-market-capital-flow").click();
        await page.waitForFunction(() => document.querySelector("#market-capital-flow-body")?.textContent.includes("2026-10-30"));
        assert.equal(new URL(capitalFlowRequest.url).searchParams.get("start"), "2026-10-01");
        assert.equal(new URL(capitalFlowRequest.url).searchParams.get("end"), "2026-10-30");
        assert.equal(capitalFlowRequest.headers["x-api-key"], "browser-test-key");
        assert.equal(await page.locator("#market-capital-flow-body tr").count(), 2);
        assert.match(await page.locator("#market-capital-flow-body tr").nth(0).textContent(), /\+1500 元/);
        assert.match(await page.locator("#market-capital-flow-body tr").nth(1).textContent(), /—/);
        assert.match(await page.locator("#market-capital-flow-status").getAttribute("class"), /quality-degraded/);
        assert.match(await page.locator("#market-capital-flow-meta").textContent(), /版本 moneyflow_mkt_dc/);
        assert.match(await page.locator("#market-capital-flow-reasons").textContent(), /源端时间精度不足/);
        const requestsBeforeMissing = capitalFlowRequestCount;
        await page.locator("#market-capital-flow-start").fill("");
        await page.locator("#load-market-capital-flow").click();
        await page.waitForFunction(() => document.querySelector("#market-capital-flow-body")?.textContent.includes("请选择开始和结束日期"));
        assert.equal(capitalFlowRequestCount, requestsBeforeMissing, "missing dates must not issue requests");
        assert.doesNotMatch(await page.locator("#market-capital-flow-body").textContent(), /2026-10-30/, "old rows must be cleared for missing dates");
        await page.locator("#market-capital-flow-start").fill("2026-10-01");
        const requestsBeforeInvalid = capitalFlowRequestCount;
        await page.locator("#market-capital-flow-start").fill("2026-10-31");
        await page.locator("#market-capital-flow-end").fill("2026-10-01");
        await page.locator("#load-market-capital-flow").click();
        await page.waitForFunction(() => document.querySelector("#market-capital-flow-body")?.textContent.includes("开始日期不能晚于结束日期"));
        assert.equal(capitalFlowRequestCount, requestsBeforeInvalid, "invalid date ranges must not issue requests");
        capitalFlowScenario = "503";
        await page.locator("#market-capital-flow-start").fill("2026-10-01");
        await page.locator("#market-capital-flow-end").fill("2026-10-30");
        await page.locator("#load-market-capital-flow").click();
        await page.waitForFunction(() => document.querySelector("#market-capital-flow-body")?.textContent.includes("Tushare Provider unavailable"));
        assert.doesNotMatch(await page.locator("#market-capital-flow-body").textContent(), /2026-10-30/);
        assert.doesNotMatch(await page.locator("#market-capital-flow-meta").textContent(), /moneyflow_mkt_dc/);
      }
      if (viewName === "watchlist" && viewport.width === 320) {
        const watchlistKey = "stock-provider-webui.watchlist.v1";
        const groupsKey = "stock-provider-webui.watchlist-groups.v1";
        await page.evaluate(({ watchlistKey, groupsKey }) => {
          localStorage.setItem(watchlistKey, JSON.stringify([
            { symbol: "600519.SH", name: "贵州茅台", updated_at: "2026-10-29T00:00:00Z" },
            { symbol: "000001.SZ", name: "平安银行", updated_at: "2026-10-28T00:00:00Z" },
            { symbol: "300750.SZ", name: "宁德时代", updated_at: "2026-10-27T00:00:00Z" }
          ]));
          localStorage.setItem(groupsKey, JSON.stringify({ groups: ["未分组"], assignments: {} }));
        }, { watchlistKey, groupsKey });
        await page.reload({ waitUntil: "load" });
        await clickNav(page, '.nav-link[data-view="watchlist"]');
        const groupName = page.locator("#watchlist-group-name");
        await groupName.fill("测试分组");
        await page.locator("#watchlist-group-create").click();
        await page.locator("#watchlist-bulk-group").selectOption("测试分组");
        const selections = page.locator("#watchlist-list [data-watchlist-select-symbol]");
        const selectAll = page.locator("#watchlist-select-visible");
        await selections.nth(0).check();
        assert.equal(await selectAll.isChecked(), false);
        assert.equal(await selectAll.evaluate(element => element.indeterminate), true, "partial row selection must set select-all indeterminate");
        await selections.nth(1).check();
        assert.equal(await selectAll.evaluate(element => element.indeterminate), true);
        await selections.nth(2).check();
        assert.equal(await selectAll.isChecked(), true, "all visible rows selected individually must synchronize select-all");
        assert.equal(await selectAll.evaluate(element => element.indeterminate), false);
        await selections.nth(0).uncheck();
        assert.equal(await selectAll.isChecked(), false);
        assert.equal(await selectAll.evaluate(element => element.indeterminate), true, "unchecking one row must restore partial state");
        for (const input of await selections.all()) await input.uncheck();
        await selections.nth(0).check();
        await selections.nth(1).check();
        await page.locator("#watchlist-bulk-assign").click();
        await page.waitForFunction(() => document.querySelector("#watchlist-bulk-count")?.textContent.includes("已选择 0 只"));
        assert.deepEqual(await page.locator("#watchlist-list .watchlist-item-group").evaluateAll(nodes => nodes.map(node => node.value)), ["测试分组", "测试分组", "未分组"], "bulk group assignment must only affect selected rows");
        assert.equal(await page.locator("#watchlist-bulk-remove").isDisabled(), true, "rendering after bulk assign must clear stale selection");
        assert.equal(await selectAll.isChecked(), false);
        assert.equal(await selectAll.evaluate(element => element.indeterminate), false);
        await page.locator("#watchlist-group-filter").selectOption("测试分组");
        assert.equal(await selections.count(), 2, "group filter must restrict the visible selection scope");
        await selectAll.check();
        assert.deepEqual(await selections.evaluateAll(nodes => nodes.map(node => node.checked)), [true, true], "select-all must select only currently filtered rows");
        await page.locator("#watchlist-group-filter").selectOption("all");
        assert.equal(await page.locator("#watchlist-bulk-count").textContent(), "已选择 0 只", "changing the filter must clear stale row selections");
        await selections.nth(0).check();
        await selections.nth(1).check();
        await page.locator("#watchlist-bulk-remove").click();
        await page.waitForFunction(key => JSON.parse(localStorage.getItem(key)).length === 1, watchlistKey);
        assert.equal(await page.locator("#watchlist-bulk-remove").isDisabled(), true, "rendering after bulk remove must clear stale selection");
        await page.locator(".watchlist-import > summary").click();
        await page.locator("#watchlist-import-text").fill("600519,贵州茅台\n000858,五粮液\ninvalid,无效");
        await page.locator("#watchlist-import-preview-button").click();
        await page.waitForFunction(() => document.querySelector("#watchlist-import-summary")?.textContent.includes("可导入 2 只"));
        assert.equal(await page.locator("#watchlist-import-confirm").isDisabled(), false);
        await page.locator("#watchlist-import-confirm").click();
        await page.waitForFunction(key => JSON.parse(localStorage.getItem(key)).length === 3, watchlistKey);
      }
      if (viewName === "settings") {
        const settingsLinks = page.locator("#settings-view .settings-nav-link");
        assert.equal(await settingsLinks.count(), 10, "settings center must expose all ten groups");
        for (const link of await settingsLinks.all()) {
          const target = await link.getAttribute("data-settings-target");
          assert.ok(target, "each settings group must declare a section target");
          assert.equal(await page.locator("#" + target).count(), 1, "settings group target must exist: " + target);
        }

        if (viewport.width === 320) {
          const settingsKey = "stock-provider-webui.settings.v1";
          await clickNav(page, '.nav-link[data-view="settings"]');
          const marketKeyInput = page.locator("#market-key-input");
          assert.equal(await marketKeyInput.getAttribute("type"), "password", "saved market API Key must remain masked");
          assert.equal(await marketKeyInput.inputValue(), "", "saved API Key must never be echoed into the settings form");
          const keyVisibility = page.locator("#toggle-market-key-visibility");
          await keyVisibility.click();
          assert.equal(await marketKeyInput.getAttribute("type"), "text", "explicit visibility toggle reveals only the current input value");
          assert.equal(await keyVisibility.getAttribute("aria-pressed"), "true");
          await keyVisibility.click();
          assert.equal(await marketKeyInput.getAttribute("type"), "password");
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), settingsKey), {
            marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "browser-test-key"
          }, "visibility toggle must not alter persisted credentials");

          const settingsSearch = page.locator("#settings-section-search");
          await settingsSearch.fill("安全与备份");
          assert.equal(await settingsLinks.evaluateAll(nodes => nodes.filter(node => !node.hidden).length), 1, "settings search must filter groups by their content");
          assert.match(await page.locator("#settings-nav-result").textContent(), /找到 1 个设置分组/);
          await settingsSearch.fill("");
          assert.equal(await settingsLinks.evaluateAll(nodes => nodes.filter(node => !node.hidden).length), 10, "clearing settings search must restore all groups");
          await page.locator("[data-settings-collapse-all]").click();
          assert.equal(await page.locator(".settings-section-toggle").evaluateAll(nodes => nodes.every(node => node.getAttribute("aria-expanded") === "false")), true, "collapse all must update every section state");
          await page.locator("[data-settings-expand-all]").click();
          assert.equal(await page.locator(".settings-section-toggle").evaluateAll(nodes => nodes.every(node => node.getAttribute("aria-expanded") === "true")), true, "expand all must restore every section");
          for (const link of await settingsLinks.all()) {
            const target = await link.getAttribute("data-settings-target");
            await link.focus();
            await page.keyboard.press("Enter");
            assert.equal(await link.getAttribute("aria-current"), "location", "keyboard section navigation must update the active link");
            assert.equal(await page.evaluate(() => document.activeElement?.closest(".settings-section")?.id), target, "keyboard section navigation must move focus to the target section heading");
          }
          const keyInput = page.locator("#market-key-input");
          assert.equal(await keyInput.getAttribute("type"), "password", "saved API Key input must default to masked");
          assert.equal(await keyInput.inputValue(), "", "saved API Key must never be echoed into the input");
          assert.match(await keyInput.getAttribute("placeholder"), /已配置 Key/);
          await page.locator("#toggle-market-key-visibility").click();
          assert.equal(await keyInput.getAttribute("type"), "text");
          assert.equal(await keyInput.inputValue(), "", "showing the key must not reveal the saved value");
          await page.locator("#toggle-market-key-visibility").click();
          assert.equal(await keyInput.getAttribute("type"), "password");

          await page.locator("#settings-default-bar-limit").selectOption("250");
          await page.locator("#settings-default-adjustment").selectOption("backward_adjusted");
          const persistedDefaults = await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), "stock-provider-webui.preferences.v1");
          assert.equal(persistedDefaults.dailyBarLimit, 250);
          assert.equal(persistedDefaults.dailyAdjustment, "backward_adjusted");

          const orderList = page.locator("#watchlist-field-order-list");
          assert.equal(await orderList.locator("[data-watchlist-field-order]").count(), 5);
          assert.equal(await page.locator("[data-watchlist-display-field]").count(), 5);
          const readOrder = () => page.locator("#watchlist-field-order-list [data-watchlist-field-order]").evaluateAll((nodes) =>
            nodes.map((node) => node.dataset.watchlistFieldOrder)
          );
          const initialOrder = await readOrder();
          await orderList.locator("[data-watchlist-field-order-move='down']").first().click();
          const reordered = await readOrder();
          assert.notDeepEqual(reordered, initialOrder, "moving a watchlist field must change the visible order");
          const visibilityInputs = page.locator("[data-watchlist-display-field]");
          const initialVisibility = await visibilityInputs.evaluateAll(nodes => nodes.map(node => node.checked));
          for (let index = 0; index < initialVisibility.length; index += 1) {
            await visibilityInputs.nth(index).setChecked(!initialVisibility[index]);
          }
          const expectedVisibility = initialVisibility.map(value => !value);
          await page.reload({ waitUntil: "load" });
          await page.locator("#open-settings").click();
          await page.locator("#settings-view").waitFor({ state: "visible" });
          assert.deepEqual(await readOrder(), reordered, "watchlist field order must survive a full page reload");
          assert.deepEqual(
            await page.locator("[data-watchlist-display-field]").evaluateAll(nodes => nodes.map(node => node.checked)),
            expectedVisibility,
            "all five watchlist field visibility settings must survive a full page reload"
          );
          assert.equal(await page.locator("#settings-default-bar-limit").inputValue(), "250", "default daily bar count must survive reload");
          assert.equal(await page.locator("#settings-default-adjustment").inputValue(), "backward_adjusted", "default adjustment must survive reload");
          await page.setViewportSize({ width: 1280, height: 900 });
          assert.deepEqual(await readOrder(), reordered, "desktop and mobile settings must share the same persisted field order");
          await page.setViewportSize({ width: 320, height: 800 });

          const keys = {
            settings: "stock-provider-webui.settings.v1",
            theme: "stock-provider-webui.theme.v1",
            preferences: "stock-provider-webui.preferences.v1",
            watchlist: "stock-provider-webui.watchlist.v1",
            tasks: "stock-provider-webui.recent-analysis.v1",
            researchNotes: "stock-provider-webui.research-notes.v1"
          };
          const currentTask = { task_id: "task-1", run_id: "run-1", symbols: ["600519.SH"], task_type: "post_market_scan", status: "running", analysis_base: "/analysis-api", updated_at: "2026-10-30T00:00:00Z" };
          await page.evaluate(({ keys, currentTask }) => {
            localStorage.setItem(keys.watchlist, JSON.stringify([{ symbol: "600519.SH", name: "原名称", updated_at: "2026-10-29T00:00:00Z" }]));
            localStorage.setItem(keys.tasks, JSON.stringify([currentTask]));
            localStorage.setItem(keys.researchNotes, JSON.stringify([{
              id: "note-import-preserve", title: "导入前研究记录", content: "研究笔记不属于工作区备份，导入不得覆盖。",
              symbol: "600519.SH", taskId: "task-1", runId: "run-1", tags: ["验收"],
              createdAt: "2026-10-29T00:00:00Z", updatedAt: "2026-10-29T00:00:00Z"
            }]));
          }, { keys, currentTask });
          const beforePreferenceReset = await page.evaluate((keys) => ({
            theme: localStorage.getItem(keys.theme),
            settings: localStorage.getItem(keys.settings),
            watchlist: localStorage.getItem(keys.watchlist),
            tasks: localStorage.getItem(keys.tasks)
          }), keys);
          await page.locator("#reset-local-preferences").click();
          const afterPreferenceReset = await page.evaluate((keys) => ({
            theme: localStorage.getItem(keys.theme),
            settings: localStorage.getItem(keys.settings),
            watchlist: localStorage.getItem(keys.watchlist),
            tasks: localStorage.getItem(keys.tasks),
            preferences: JSON.parse(localStorage.getItem(keys.preferences))
          }), keys);
          assert.equal(afterPreferenceReset.theme, beforePreferenceReset.theme, "resetting display preferences must preserve the theme");
          assert.equal(afterPreferenceReset.settings, beforePreferenceReset.settings, "resetting display preferences must preserve connection credentials");
          assert.equal(afterPreferenceReset.watchlist, beforePreferenceReset.watchlist, "resetting display preferences must preserve watchlist data");
          assert.equal(afterPreferenceReset.tasks, beforePreferenceReset.tasks, "resetting display preferences must preserve recent task references");
          assert.deepEqual(afterPreferenceReset.preferences.watchlistDisplayOrder, ["updated", "quote", "today", "task", "result"]);

          const makeImportFile = (watchlist, recentTasks, preferences = {}, version = 3, watchlistGroups) => ({
            name: "workspace-test.json",
            mimeType: "application/json",
            buffer: Buffer.from(JSON.stringify({
              format: "stock-provider-webui-browser-data",
              version,
              data: {
                theme: "dark",
                preferences,
                connection: { marketBase: "/market-api", analysisBase: "/analysis-api" },
                watchlist,
                recent_tasks: recentTasks,
                ...(watchlistGroups ? { watchlist_groups: watchlistGroups } : {})
              }
            }))
          });
          const uploadWorkspace = async (file) => {
            await page.locator("#browser-data-file").setInputFiles(file);
            await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === true);
          };
          const currentImportedTasks = [
            { ...currentTask, status: "succeeded" },
            { task_id: "task-2", run_id: "run-2", symbols: ["000001.SZ"], task_type: "post_market_scan", status: "queued", analysis_base: "/analysis-api" }
          ];
          const mergeFile = makeImportFile([
            { symbol: "600519", name: "导入名称", updated_at: "2026-10-30T00:00:00Z" },
            { symbol: "000001.SZ", name: "平安银行", updated_at: "2026-10-30T00:00:00Z" }
          ], currentImportedTasks, { watchlistSort: "name" }, 3, {
            groups: ["未分组", "核心观察"],
            assignments: { "600519.SH": "核心观察" }
          });

          await uploadWorkspace(mergeFile);
          const preview = await page.locator("#browser-data-import-summary").textContent();
          assert.match(preview, /冲突 1 条/, "preview must report the duplicate stock and task conflicts");
          assert.match(await page.locator("#browser-data-import-current").textContent(), /1 条自选股，1 条最近任务/);
          assert.match(await page.locator("#browser-data-import-settings").textContent(), /当前行情 API Key 保留/);
          await page.locator("#browser-data-import-dialog button[value='cancel']").last().click();
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), keys.watchlist), [
            { symbol: "600519.SH", name: "原名称", updated_at: "2026-10-29T00:00:00Z" }
          ], "footer cancel must not modify the current watchlist");

          await uploadWorkspace(mergeFile);
          await page.locator("#browser-data-import-dialog button[aria-label='关闭导入预览']").click();
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), keys.watchlist), [
            { symbol: "600519.SH", name: "原名称", updated_at: "2026-10-29T00:00:00Z" }
          ], "close button must not modify the current watchlist");

          await uploadWorkspace(mergeFile);
          await page.keyboard.press("Escape");
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), keys.watchlist), [
            { symbol: "600519.SH", name: "原名称", updated_at: "2026-10-29T00:00:00Z" }
          ], "Escape must not modify the current watchlist");

          await uploadWorkspace(mergeFile);
          await page.locator("#browser-data-import-merge").click();
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          const mergeFeedback = await page.locator("#browser-data-feedback").textContent();
          assert.match(mergeFeedback, /导入完成/, "merge import failed: " + mergeFeedback);
          const merged = await page.evaluate((keys) => ({
            settings: JSON.parse(localStorage.getItem(keys.settings)),
            watchlist: JSON.parse(localStorage.getItem(keys.watchlist)),
            tasks: JSON.parse(localStorage.getItem(keys.tasks)),
            preferences: JSON.parse(localStorage.getItem(keys.preferences)),
            researchNotes: JSON.parse(localStorage.getItem(keys.researchNotes))
          }), keys);
          assert.equal(merged.settings.apiKey, "browser-test-key", "merge must preserve the current API Key");
          assert.equal(merged.watchlist.length, 2);
          assert.equal(merged.watchlist.find((item) => item.symbol.replace(/\\.(SH|SZ|BJ)$/, "") === "600519")?.name, "导入名称");
          assert.equal(merged.tasks.length, 2);
          assert.equal(merged.tasks.find((item) => item.task_id === "task-1")?.status, "succeeded");
          assert.equal(merged.researchNotes.length, 1, "workspace merge must preserve local research notes outside the backup format");
          assert.equal(merged.researchNotes[0].id, "note-import-preserve");
          assert.equal(merged.preferences.watchlistSort, "name");
          const mergedGroups = await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), "stock-provider-webui.watchlist-groups.v1");
          assert.ok(mergedGroups.groups.includes("测试分组") && mergedGroups.groups.includes("核心观察"), "merge must preserve current and imported watchlist groups");
          assert.equal(mergedGroups.assignments["600519.SH"], "核心观察", "imported group assignment must override the same stock identity");
          assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), "dark", "imported theme must update the running page");
          assert.equal(await page.locator("#settings-watchlist-sort").inputValue(), "name", "imported preferences must update settings controls");
          await clickNav(page, '.nav-link[data-view="watchlist"]');
          await page.waitForFunction(() => [...document.querySelectorAll("#watchlist-list .watchlist-name-input")].some(input => input.value === "导入名称"));
          await clickNav(page, '.nav-link[data-view="tasks"]');
          await page.waitForTimeout(100);
          const importedTaskListText = await page.locator("#recent-analysis-list").textContent();
          const importedTaskStorage = await page.evaluate((key) => localStorage.getItem(key), keys.tasks);
          assert.match(importedTaskListText || "", /task-2/, "recent task list did not refresh after import; rendered=" + importedTaskListText + "; storage=" + importedTaskStorage);
          await clickNav(page, '.nav-link[data-view="settings"]');
          await page.locator("#settings-view").waitFor({ state: "visible" });
          const [workspaceDownload] = await Promise.all([
            page.waitForEvent("download"),
            page.locator("#export-browser-data").click()
          ]);
          const exportedWorkspace = JSON.parse(await readFile(await workspaceDownload.path(), "utf8"));
          assert.deepEqual(exportedWorkspace.data.watchlist_groups, mergedGroups, "workspace export must preserve custom groups and symbol assignments");

          const replaceFile = makeImportFile(
            [{ symbol: "300750.SZ", name: "宁德时代", updated_at: "2026-10-30T00:00:00Z" }],
            [{ task_id: "task-3", symbols: ["300750.SZ"], task_type: "post_market_scan", status: "queued", analysis_base: "/analysis-api" }],
            {}, 3, { groups: ["未分组", "趋势跟踪"], assignments: { "300750.SZ": "趋势跟踪" } }
          );
          await uploadWorkspace(replaceFile);
          await page.locator("#browser-data-import-replace").click();
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          const replaceFeedback = await page.locator("#browser-data-feedback").textContent();
          assert.match(replaceFeedback, /导入完成/, "replace import failed: " + replaceFeedback);
          const replaced = await page.evaluate((keys) => ({
            settings: JSON.parse(localStorage.getItem(keys.settings)),
            watchlist: JSON.parse(localStorage.getItem(keys.watchlist)),
            tasks: JSON.parse(localStorage.getItem(keys.tasks)),
            researchNotes: JSON.parse(localStorage.getItem(keys.researchNotes))
          }), keys);
          assert.equal(replaced.settings.apiKey, "browser-test-key", "replace must preserve the current API Key");
          assert.deepEqual(replaced.watchlist.map((item) => item.symbol), ["300750.SZ"]);
          assert.deepEqual(replaced.tasks.map((item) => item.task_id), ["task-3"]);
          assert.equal(replaced.researchNotes.length, 1, "workspace replacement must preserve research notes outside the backup format");
          assert.equal(replaced.researchNotes[0].id, "note-import-preserve");
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), "stock-provider-webui.watchlist-groups.v1"), {
            groups: ["未分组", "趋势跟踪"], assignments: { "300750.SZ": "趋势跟踪" }
          }, "replace must restore imported watchlist groups and assignments");

          for (const legacyVersion of [1, 2]) {
            const legacyFile = makeImportFile(
              [{ symbol: legacyVersion === 1 ? "600519.SH" : "000001.SZ", name: "旧版导入" }],
              [],
              { watchlistSort: "symbol" },
              legacyVersion
            );
            await uploadWorkspace(legacyFile);
            await page.locator("#browser-data-import-replace").click();
            await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
            const legacyPreferences = await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), keys.preferences);
            assert.deepEqual(legacyPreferences.watchlistDisplayOrder, ["updated", "quote", "today", "task", "result"], "v" + legacyVersion + " import must restore default field order");
            assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), "stock-provider-webui.watchlist-groups.v1"), {
              groups: ["未分组"], assignments: {}
            }, "legacy v" + legacyVersion + " imports must default to the ungrouped watchlist");
          }

          const beforeInvalid = JSON.stringify(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), keys.watchlist));
          await page.locator("#browser-data-file").setInputFiles(makeImportFile(
            [{ symbol: "300750.SZ", name: "非法顺序测试" }],
            [],
            { watchlistDisplayOrder: ["updated", "updated", "today", "task", "result"] }
          ));
          await page.waitForFunction(() => document.querySelector("#browser-data-feedback")?.textContent.includes("导入失败"));
          assert.equal(await page.evaluate((key) => JSON.stringify(JSON.parse(localStorage.getItem(key))), keys.watchlist), beforeInvalid, "invalid field order must not modify current watchlist");
          await page.locator("#browser-data-file").setInputFiles(makeImportFile(
            [],
            [{ task_id: "task-invalid-symbol", symbols: [" "], task_type: "post_market_scan", status: "queued" }]
          ));
          await page.waitForFunction(() => document.querySelector("#browser-data-feedback")?.textContent.includes("任务标的数量或格式无效"));
          assert.equal(await page.evaluate((key) => JSON.stringify(JSON.parse(localStorage.getItem(key))), keys.watchlist), beforeInvalid, "blank task symbols must be rejected before import writes");

          await page.locator("#browser-data-file").setInputFiles({
            name: "broken.json", mimeType: "application/json", buffer: Buffer.from("{")
          });
          await page.waitForFunction(() => document.querySelector("#browser-data-feedback")?.textContent.includes("导入失败"));
          assert.equal(await page.evaluate((key) => localStorage.getItem(key), keys.watchlist), beforeInvalid);
          await page.locator("#browser-data-file").setInputFiles({
            name: "too-large.json", mimeType: "application/json", buffer: Buffer.alloc(1024 * 1024 + 1, 32)
          });
          await page.waitForFunction(() => document.querySelector("#browser-data-feedback")?.textContent.includes("超过 1 MiB 上限"));
          assert.equal(await page.evaluate((key) => localStorage.getItem(key), keys.watchlist), beforeInvalid);

          const fullWatchlist = Array.from({ length: 50 }, (_, index) => ({
            symbol: String(600000 + index), name: "测试" + index, updated_at: ""
          }));
          await page.evaluate(({ key, records }) => localStorage.setItem(key, JSON.stringify(records)), { key: keys.watchlist, records: fullWatchlist });
          await uploadWorkspace(makeImportFile([{ symbol: "688001.SH", name: "新增标的" }], []));
          await page.locator("#browser-data-import-merge").click();
          await page.waitForFunction(() => document.querySelector("#browser-data-feedback")?.textContent.includes("超过本地上限"));
          assert.equal((await page.evaluate((key) => JSON.parse(localStorage.getItem(key)).length, keys.watchlist)), 50);

          const tooManyTasks = Array.from({ length: 11 }, (_, index) => ({
            task_id: "task-limit-" + index, symbols: [], task_type: "post_market_scan", status: "queued", analysis_base: "/analysis-api"
          }));
          await page.locator("#browser-data-file").setInputFiles(makeImportFile([], tooManyTasks));
          await page.waitForFunction(() => document.querySelector("#browser-data-feedback")?.textContent.includes("最近任务引用必须是数组，且不能超过 10 条"));
          assert.equal((await page.evaluate((key) => JSON.parse(localStorage.getItem(key)).length, keys.watchlist)), 50, "over-limit task history must be rejected before writing");

          await page.evaluate((key) => localStorage.setItem(key, "{"), keys.watchlist);
          await uploadWorkspace(makeImportFile([{ symbol: "688001.SH", name: "新增标的" }], []));
          assert.equal(await page.locator("#browser-data-import-merge").isDisabled(), true, "damaged local storage must disable merge");
          await page.keyboard.press("Escape");
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          assert.equal(await page.evaluate((key) => localStorage.getItem(key), keys.watchlist), "{");

          const rollbackWatchlist = [{ symbol: "300750.SZ", name: "原始数据", updated_at: "" }];
          await page.evaluate(({ keys, records }) => {
            localStorage.setItem(keys.watchlist, JSON.stringify(records));
            localStorage.setItem(keys.tasks, JSON.stringify([{ task_id: "task-3", analysis_base: "/analysis-api" }]));
            const original = Storage.prototype.setItem;
            let failOnce = true;
            Storage.prototype.setItem = function (key, value) {
              if (failOnce && key === keys.watchlist) {
                failOnce = false;
                throw new DOMException("quota exceeded", "QuotaExceededError");
              }
              return original.call(this, key, value);
            };
            window.__restoreStorageSetItem = () => { Storage.prototype.setItem = original; };
          }, { keys, records: rollbackWatchlist });
          await uploadWorkspace(makeImportFile([{ symbol: "688001.SH", name: "导入数据" }], []));
          await page.locator("#browser-data-import-replace").click();
          await page.waitForFunction(() => document.querySelector("#browser-data-import-dialog")?.open === false);
          const rollbackFeedback = await page.locator("#browser-data-feedback").textContent();
          await page.evaluate(() => window.__restoreStorageSetItem?.());
          assert.match(rollbackFeedback, /已回读确认导入前的本地存储已恢复/, "rollback result: " + rollbackFeedback);
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)), keys.watchlist), rollbackWatchlist);
          assert.deepEqual(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)).map((item) => item.task_id), keys.tasks), ["task-3"]);
        }
      }
    }

    if (await menu.isVisible() && (await menu.getAttribute("aria-expanded")) !== "true") {
      await menu.click();
    }
    await clickNav(page, '[data-view="tasks"]');
    await page.locator("#tasks-view").waitFor({ state: "visible" });

    const flow = await page.locator("#run-flow-dialog").evaluate((dialog) => {
      const emptyState = document.querySelector("#task-empty");
      const detail = document.querySelector("#task-detail");
      const workspace = document.querySelector(".run-flow-workspace");
      const eventDetail = document.querySelector("#task-event-detail");
      if (emptyState) emptyState.hidden = true;
      if (detail) detail.hidden = false;
      if (eventDetail) eventDetail.hidden = false;
      if (!dialog.open) dialog.showModal();

      const rect = dialog.getBoundingClientRect();
      const detailRect = detail.getBoundingClientRect();
      const workspaceRect = workspace.getBoundingClientRect();
      const eventDetailRect = eventDetail.getBoundingClientRect();
      return {
        left: rect.left,
        right: rect.right,
        width: rect.width,
        height: rect.height,
        detailWidth: detailRect.width,
        workspaceColumns: getComputedStyle(workspace).gridTemplateColumns.split(" ").filter(Boolean).length,
        workspaceWidth: workspaceRect.width,
        eventDetailRight: eventDetailRect.right,
        documentWidth: document.documentElement.scrollWidth,
        viewportWidth: window.innerWidth
      };
    });

    assert.ok(flow.left >= -1, "Run Flow dialog must not extend beyond the left viewport edge at " + viewport.width + "px");
    assert.ok(flow.right <= viewport.width + 1, "Run Flow dialog must not extend beyond the right viewport edge at " + viewport.width + "px");
    assert.ok(flow.width >= viewport.width * 0.85, "Run Flow should use a near-full-screen layout at " + viewport.width + "px");
    assert.ok(flow.detailWidth > 0, "Run Flow details must be visible in the browser layout test");
    assert.ok(flow.eventDetailRight <= flow.right + 1, "event detail must remain within the Run Flow dialog at " + viewport.width + "px");
    assert.ok(flow.documentWidth <= flow.viewportWidth + 1, "Run Flow must not introduce document horizontal overflow at " + viewport.width + "px");

    if (viewport.width > 900) {
      assert.ok(flow.workspaceColumns >= 2, "Run Flow should show timeline and results side by side above 900px; got " + flow.workspaceColumns + " at " + viewport.width + "px");
    } else {
      assert.equal(flow.workspaceColumns, 1, "Run Flow should stack into one column at " + viewport.width + "px");
    }

    await page.screenshot({
      path: path.join(screenshotDir, viewport.width + "x" + viewport.height + "-run-flow.png"),
      fullPage: false,
      animations: "disabled"
    });

    await page.keyboard.press("Escape");
    assert.equal(await page.locator("#run-flow-dialog").evaluate((dialog) => dialog.open), false, "Run Flow should close with Escape");

    assert.deepEqual(pageErrors, [], "browser page errors at " + viewport.width + "px: " + pageErrors.join(" | "));
    assert.deepEqual(consoleErrors, [], "unexpected browser console errors at " + viewport.width + "px: " + consoleErrors.join(" | "));
    assert.deepEqual(unexpectedNetworkFailures, [], "unexpected non-API network failures at " + viewport.width + "px: " + unexpectedNetworkFailures.join(" | "));
    console.log("PASS viewport " + viewport.width + "x" + viewport.height + ": navigation, overflow, focus, console, network, Run Flow layout");

    await context.close();
  }

  // Storage access denied by the browser: the app must remain interactive and must not report a saved watchlist.
  const blockedStorageContext = await browser.newContext({ viewport: { width: 320, height: 812 }, reducedMotion: "reduce" });
  await blockedStorageContext.addInitScript(() => {
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      get() {
        throw new DOMException("Storage access is blocked by browser smoke", "SecurityError");
      }
    });
  });
  const blockedStoragePage = await blockedStorageContext.newPage();
  const blockedStorageErrors = [];
  blockedStoragePage.on("pageerror", (error) => blockedStorageErrors.push(error.message));
  await blockedStoragePage.route("**/*", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.includes("/api/") || pathname.endsWith("/health/ready") || pathname.endsWith("/readyz")) {
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Storage access browser test: backend intentionally unavailable" })
      });
    }
    return route.continue();
  });
  await blockedStoragePage.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await blockedStoragePage.locator("#market-view").waitFor({ state: "visible" });
  assert.deepEqual(blockedStorageErrors, [], "blocked localStorage must not crash initial app rendering");

  await clickNav(blockedStoragePage, '.nav-link[data-view="stocks"]');
  await blockedStoragePage.locator("#stocks-view").waitFor({ state: "visible" });
  await blockedStoragePage.locator("#add-to-watchlist-button").click();
  await blockedStoragePage.waitForFunction(
    () => document.querySelector("#watchlist-feedback")?.textContent.includes("浏览器本地存储不可用，无法保存自选股。"),
    null,
    { timeout: 5000 }
  );
  const blockedAddFeedback = await blockedStoragePage.locator("#watchlist-feedback").textContent();
  assert.doesNotMatch(blockedAddFeedback, /已将 .* 加入本地自选|已在自选列表中/);
  assert.match(await blockedStoragePage.locator("#watchlist-feedback").getAttribute("class"), /watchlist-feedback-error/);

  await clickNav(blockedStoragePage, '.nav-link[data-view="watchlist"]');
  await blockedStoragePage.locator("#watchlist-view").waitFor({ state: "visible" });
  await blockedStoragePage.locator(".watchlist-storage-unavailable").waitFor({ state: "visible" });
  const blockedWatchlistState = await blockedStoragePage.evaluate(() => ({
    warning: document.querySelector(".watchlist-storage-unavailable")?.textContent || "",
    count: document.querySelector("#watchlist-count")?.textContent.trim() || "",
    items: document.querySelectorAll("#watchlist-list .watchlist-item").length,
    actions: document.querySelectorAll("#watchlist-list [data-watchlist-action]").length,
    scope: document.querySelector("#research-list-scope")?.textContent.trim() || ""
  }));
  assert.match(blockedWatchlistState.warning, /无法读取浏览器自选股存储/);
  assert.match(blockedWatchlistState.warning, /添加、改名和移除已暂停/);
  assert.equal(blockedWatchlistState.count, "不可用 / 50");
  assert.equal(blockedWatchlistState.items, 0);
  assert.equal(blockedWatchlistState.actions, 0, "unreadable storage must not render rename/remove controls");
  assert.equal(blockedWatchlistState.scope, "本地自选不可读取");

  // Recent task history must show its own unavailable state and must not claim that a clear operation succeeded.
  await clickNav(blockedStoragePage, '.nav-link[data-view="tasks"]');
  await blockedStoragePage.locator("#tasks-view").waitFor({ state: "visible" });
  await blockedStoragePage.waitForFunction(
    () => document.querySelector("#recent-analysis-count")?.textContent.trim() === "不可用 / 10" &&
      document.querySelector("#recent-analysis-feedback")?.textContent.includes("无法读取浏览器最近任务存储"),
    null,
    { timeout: 5000 }
  );
  const blockedRecentState = await blockedStoragePage.evaluate(() => ({
    count: document.querySelector("#recent-analysis-count")?.textContent.trim() || "",
    feedback: document.querySelector("#recent-analysis-feedback")?.textContent || "",
    feedbackClass: document.querySelector("#recent-analysis-feedback")?.className || "",
    rows: document.querySelectorAll("#recent-analysis-list .recent-analysis-item").length,
    removeActions: document.querySelectorAll("#recent-analysis-list [data-remove-task-id]").length
  }));
  assert.equal(blockedRecentState.count, "不可用 / 10");
  assert.match(blockedRecentState.feedback, /无法读取浏览器最近任务存储/);
  assert.match(blockedRecentState.feedbackClass, /is-error/);
  assert.equal(blockedRecentState.rows, 0);
  assert.equal(blockedRecentState.removeActions, 0);

  blockedStoragePage.once("dialog", (dialog) => dialog.accept());
  await blockedStoragePage.locator("#clear-recent-analysis").click();
  await blockedStoragePage.waitForFunction(
    () => document.querySelector("#recent-analysis-feedback")?.textContent.includes("未能确认最近任务历史已清空"),
    null,
    { timeout: 5000 }
  );
  const blockedClearFeedback = await blockedStoragePage.locator("#recent-analysis-feedback").textContent();
  assert.match(blockedClearFeedback, /未能确认最近任务历史已清空/);
  assert.doesNotMatch(blockedClearFeedback, /本地最近任务列表已清空/);
  assert.deepEqual(blockedStorageErrors, [], "blocked localStorage must not create unhandled browser errors");
  await blockedStorageContext.close();

  // A browser quota/write failure keeps the current page usable, but add/rename/remove must not claim persistence.
  const quotaContext = await browser.newContext({ viewport: { width: 320, height: 812 }, reducedMotion: "reduce" });
  const watchlistKey = "stock-provider-webui.watchlist.v1";
  const recentTasksKey = "stock-provider-webui.recent-analysis.v1";
  const originalWatchlist = JSON.stringify([{
    symbol: "600519.SH",
    name: "贵州茅台",
    updated_at: "2026-10-28T07:00:00Z"
  }]);
  const originalRecentTasks = JSON.stringify([{
    task_id: "task-browser-storage-001",
    run_id: "",
    created_at: "2026-10-28T07:00:00Z",
    symbols: ["600519.SH"],
    task_type: "post_market_scan",
    status: "succeeded",
    analysis_base: "/analysis-api",
    updated_at: "2026-10-28T07:00:00Z"
  }]);
  await quotaContext.addInitScript((seed) => {
    const watchlistKey = "stock-provider-webui.watchlist.v1";
    const recentTasksKey = "stock-provider-webui.recent-analysis.v1";
    const blockedKeys = new Set([watchlistKey, recentTasksKey]);
    window.localStorage.setItem(watchlistKey, seed.watchlist);
    window.localStorage.setItem(recentTasksKey, seed.recentTasks);
    const originalSetItem = Storage.prototype.setItem;
    const originalRemoveItem = Storage.prototype.removeItem;
    Storage.prototype.setItem = function (name, value) {
      if (blockedKeys.has(name)) throw new DOMException("Quota exceeded by browser smoke", "QuotaExceededError");
      return originalSetItem.call(this, name, value);
    };
    Storage.prototype.removeItem = function (name) {
      if (blockedKeys.has(name)) throw new DOMException("Storage write disabled by browser smoke", "SecurityError");
      return originalRemoveItem.call(this, name);
    };
  }, { watchlist: originalWatchlist, recentTasks: originalRecentTasks });
  const quotaPage = await quotaContext.newPage();
  const quotaErrors = [];
  quotaPage.on("pageerror", (error) => quotaErrors.push(error.message));
  await quotaPage.route("**/*", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.includes("/api/") || pathname.endsWith("/health/ready") || pathname.endsWith("/readyz")) {
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Storage quota browser test: backend intentionally unavailable" })
      });
    }
    return route.continue();
  });
  await quotaPage.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await clickNav(quotaPage, '.nav-link[data-view="watchlist"]');
  await quotaPage.locator("#watchlist-view").waitFor({ state: "visible" });
  await quotaPage.locator("#watchlist-list .watchlist-item").waitFor({ state: "visible" });

  const renameInput = quotaPage.locator("#watchlist-list .watchlist-name-input").first();
  await renameInput.fill("临时重命名");
  await renameInput.press("Tab");
  await quotaPage.waitForFunction(
    () => document.querySelector("#watchlist-feedback")?.textContent.includes("名称已在当前页面临时更新，但浏览器存储不可用"),
    null,
    { timeout: 5000 }
  );
  assert.match(await quotaPage.locator("#watchlist-feedback").getAttribute("class"), /watchlist-feedback-warning/);
  assert.doesNotMatch(await quotaPage.locator("#watchlist-feedback").textContent(), /已保存 .* 的自定义名称/);
  assert.equal(await quotaPage.evaluate((key) => localStorage.getItem(key), watchlistKey), originalWatchlist,
    "a failed rename write must preserve the stored watchlist bytes");

  await clickNav(quotaPage, '.nav-link[data-view="stocks"]');
  await quotaPage.locator("#stocks-view").waitFor({ state: "visible" });
  await quotaPage.locator("#symbol-input").fill("000001.SZ");
  await quotaPage.locator("#add-to-watchlist-button").click();
  await quotaPage.waitForFunction(
    () => document.querySelector("#watchlist-feedback")?.textContent.includes("已在当前页面临时加入自选，但浏览器存储不可用，刷新后可能丢失"),
    null,
    { timeout: 5000 }
  );
  assert.match(await quotaPage.locator("#watchlist-feedback").getAttribute("class"), /watchlist-feedback-warning/);
  assert.doesNotMatch(await quotaPage.locator("#watchlist-feedback").textContent(), /已将 .* 加入本地自选/);
  assert.equal(await quotaPage.evaluate((key) => localStorage.getItem(key), watchlistKey), originalWatchlist,
    "a failed add write must not change the stored watchlist bytes");

  await clickNav(quotaPage, '.nav-link[data-view="watchlist"]');
  await quotaPage.locator("#watchlist-view").waitFor({ state: "visible" });
  await quotaPage.locator('#watchlist-list [data-watchlist-action="remove"][data-symbol="600519.SH"]').click();
  await quotaPage.waitForFunction(
    () => document.querySelector("#watchlist-feedback")?.textContent.includes("已在当前页面临时移除，但浏览器存储不可用，刷新后可能恢复"),
    null,
    { timeout: 5000 }
  );
  assert.match(await quotaPage.locator("#watchlist-feedback").getAttribute("class"), /watchlist-feedback-warning/);
  assert.doesNotMatch(await quotaPage.locator("#watchlist-feedback").textContent(), /已从本地自选中移除/);
  assert.equal(await quotaPage.evaluate((key) => localStorage.getItem(key), watchlistKey), originalWatchlist,
    "a failed remove write must preserve the original stored watchlist bytes");
  assert.deepEqual(quotaErrors, [], "quota failures must be handled without unhandled browser errors");

  // Recent-task removal and clearing must also verify the write, because the storage module returns a best-effort array.
  await clickNav(quotaPage, '.nav-link[data-view="tasks"]');
  await quotaPage.locator("#tasks-view").waitFor({ state: "visible" });
  await quotaPage.locator('#recent-analysis-list [data-remove-task-id="task-browser-storage-001"]').waitFor({ state: "visible" });
  await quotaPage.locator('#recent-analysis-list [data-remove-task-id="task-browser-storage-001"]').click();
  await quotaPage.waitForFunction(
    () => document.querySelector("#recent-analysis-feedback")?.textContent.includes("无法确认最近任务引用已移除"),
    null,
    { timeout: 5000 }
  );
  const failedRecentRemove = await quotaPage.locator("#recent-analysis-feedback").textContent();
  assert.match(failedRecentRemove, /无法确认最近任务引用已移除/);
  assert.doesNotMatch(failedRecentRemove, /已从当前浏览器移除该任务引用/);
  assert.equal(await quotaPage.evaluate((key) => localStorage.getItem(key), recentTasksKey), originalRecentTasks,
    "a failed recent-task removal must preserve the original stored history bytes");

  quotaPage.once("dialog", (dialog) => dialog.accept());
  await quotaPage.locator("#clear-recent-analysis").click();
  await quotaPage.waitForFunction(
    () => document.querySelector("#recent-analysis-feedback")?.textContent.includes("未能确认最近任务历史已清空"),
    null,
    { timeout: 5000 }
  );
  const failedRecentClear = await quotaPage.locator("#recent-analysis-feedback").textContent();
  assert.match(failedRecentClear, /未能确认最近任务历史已清空/);
  assert.doesNotMatch(failedRecentClear, /本地最近任务列表已清空/);
  assert.equal(await quotaPage.evaluate((key) => localStorage.getItem(key), recentTasksKey), originalRecentTasks,
    "a failed recent-task clear must preserve the original stored history bytes");
  assert.deepEqual(quotaErrors, [], "recent-task storage failures must not create unhandled browser errors");
  await quotaContext.close();

  console.log("PASS all " + viewports.length + " responsive browser viewports");
  console.log("Screenshots: " + screenshotDir);
} finally {
  if (browser) await browser.close();
  await new Promise((resolve) => server.close(resolve));
}

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
  { name: "market", selector: '.workspace-tab[data-view="market"]' },
  { name: "stocks", selector: '.workspace-tab[data-view="stocks"]' },
  { name: "watchlist", selector: '.workspace-tab[data-view="watchlist"]' },
  { name: "research", selector: '.nav-link[data-view="research"]' },
  { name: "tasks", selector: '.nav-link[data-view="tasks"]' },
  { name: "data", selector: '.nav-link[data-view="data"]' },
  { name: "notifications", selector: '.nav-link[data-view="notifications"]' },
  { name: "chat", selector: '.nav-link[data-view="chat"]' },
  { name: "screening", selector: '.nav-link[data-view="screening"]' },
  { name: "alerts", selector: '.nav-link[data-view="alerts"]' },
  { name: "settings", selector: '.nav-link[data-view="settings"]' }
];
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
    const pageErrors = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));

    // API responses are deterministic fixtures only: one isolated viewport exercises failures, others stay unavailable.
    let quoteScenario = "success";
    let quoteRequestCount = 0;
    let marketScenario = "503";
    let marketIndexRequestCount = 0;
    let dataSourceScenario = "success";
    let dataSourcesRequestCount = 0;
    let healthScenario = "unavailable";
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

      // Settings tests intentionally change the market base path; health fixtures must follow any same-origin prefix.
      if (pathname.endsWith("/health/ready") || pathname.endsWith("/analysis-api/readyz")) {
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

      if (viewport.width === 320 && pathname.includes("/api/v1/quotes/")) {
        quoteRequestCount += 1;
        if (quoteScenario === "network") return route.abort("failed");
        if (quoteScenario === "success") return json(200, quoteFixture);
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
        return json(200, barsFixture);
      }

      if (pathname.includes("/api/")) {
        return json(503, { error: "Browser layout test: backend intentionally unavailable" });
      }

      return route.continue();
    });

    await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
    assert.deepEqual(pageErrors, [], "browser page errors after initial load at " + viewport.width + "px: " + pageErrors.join(" | "));
    await page.waitForFunction(
      () => document.querySelectorAll(".nav-link[data-view]").length === 8 &&
        document.querySelectorAll(".workspace-tab[data-view]").length === 3,
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
      await page.locator('.workspace-tab[data-view="stocks"]').click();
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
      await page.waitForFunction(() =>
        document.querySelector("#market-health-text")?.textContent === "不可用" &&
        document.querySelector("#analysis-health-text")?.textContent === "不可用",
      null, { timeout: 10000 });

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
          marketHealth: document.querySelector("#market-health-dot")?.className || "",
          analysisHealth: document.querySelector("#analysis-health-dot")?.className || "",
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
        assert.doesNotMatch(failedQuote.marketHealth, /is-online/, scenario + " must not report a healthy market API");
        assert.doesNotMatch(failedQuote.analysisHealth, /is-online/, scenario + " must not report a healthy analysis API");
        assert.doesNotMatch(failedQuote.overallConnection, /is-online/, scenario + " must not report healthy overall connectivity");

        // A user-initiated retry must issue exactly one new request and restore data only after success.
        quoteScenario = "success";
        await submitQuote.click();
        await page.waitForFunction(() => document.querySelector("#metric-price")?.textContent.trim() === "—", null, { timeout: 3000 });
        await page.waitForFunction(() => document.querySelector("#metric-price")?.textContent.trim() === "123.45", null, { timeout: 8000 });
        assert.equal(quoteRequestCount, requestsBeforeFailure + 2, scenario + " manual retry should make one new request");
        assert.equal(await page.locator("#quote-quality").textContent().then((value) => value.trim()), "质量正常");
      }

      await page.locator('.workspace-tab[data-view="market"]').click();
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

      // Data Center must classify Provider-list errors independently from service probes and clear old capability rows.
      const menu = page.locator("#mobile-menu-toggle");
      if (await menu.isVisible() && (await menu.getAttribute("aria-expanded")) !== "true") {
        await menu.click();
      }
      await page.locator('.nav-link[data-view="data"]').click();
      await page.locator("#data-view").waitFor({ state: "visible" });

      dataSourceScenario = "success";
      const dataSeedCount = dataSourcesRequestCount;
      await page.locator("#data-center-check").click();
      await page.waitForFunction(
        () => document.querySelector("#data-center-provider-list")?.textContent.includes("测试 Provider"),
        null,
        { timeout: 10000 }
      );
      assert.equal(dataSourcesRequestCount, dataSeedCount + 1, "data center success seed should request Provider list once");

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
      await page.locator("#settings-dialog").waitFor({ state: "visible" });
      await page.locator("#market-base-input").fill("/market-api-v2");
      await page.waitForFunction(() => !document.querySelector("#save-settings")?.disabled, null, { timeout: 3000 });
      await page.locator("#save-settings").click();
      await page.waitForFunction(() => {
        const summary = document.querySelector("#data-center-summary-message")?.textContent || "";
        const list = document.querySelector("#data-center-provider-list")?.textContent || "";
        return !document.querySelector("#settings-dialog")?.open &&
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

      // Keyboard dismissal of the settings dialog must restore focus to its opener.
      const settingsOpener = page.locator("#open-settings");
      await settingsOpener.click();
      await page.locator("#settings-dialog").waitFor({ state: "visible" });
      assert.equal(await page.locator("#settings-dialog").evaluate((dialog) => dialog.open), true);
      assert.equal(
        await page.evaluate(() => Boolean(document.activeElement?.closest("#settings-dialog"))),
        true,
        "settings modal should contain focus while open"
      );
      for (let tabIndex = 0; tabIndex < 12; tabIndex += 1) {
        await page.keyboard.press("Tab");
        assert.equal(
          await page.evaluate(() => {
            const dialog = document.querySelector("#settings-dialog");
            return Boolean(dialog?.open && dialog.contains(document.activeElement));
          }),
          true,
          "Tab " + (tabIndex + 1) + " must keep focus inside the settings modal"
        );
      }
      for (let tabIndex = 0; tabIndex < 12; tabIndex += 1) {
        await page.keyboard.press("Shift+Tab");
        assert.equal(
          await page.evaluate(() => {
            const dialog = document.querySelector("#settings-dialog");
            return Boolean(dialog?.open && dialog.contains(document.activeElement));
          }),
          true,
          "Shift+Tab " + (tabIndex + 1) + " must keep focus inside the settings modal"
        );
      }
      await page.keyboard.press("Escape");
      await page.waitForFunction(() => !document.querySelector("#settings-dialog")?.open, null, { timeout: 3000 });
      assert.equal(
        await settingsOpener.evaluate((element) => document.activeElement === element),
        true,
        "closing settings with Escape should restore focus to its opener"
      );

      await page.locator('.workspace-tab[data-view="market"]').click();
      await page.locator("#market-view").waitFor({ state: "visible" });

      // The topbar health summary must use the same evidence threshold as Data Center.
      healthScenario = "missing-status";
      const refreshButton = page.locator("#refresh-button");
      await page.waitForFunction(() => !document.querySelector("#refresh-button")?.disabled, null, { timeout: 10000 });
      await refreshButton.click();
      await page.waitForFunction(
        () => document.querySelector("#market-health-text")?.textContent === "状态未知" &&
          document.querySelector("#analysis-health-text")?.textContent === "状态未知" &&
          document.querySelector("#market-health-dot")?.classList.contains("is-unknown") &&
          document.querySelector("#analysis-health-dot")?.classList.contains("is-unknown"),
        null,
        { timeout: 10000 }
      );
      const topbarHealth = await page.evaluate(() => ({
        marketLabel: document.querySelector("#market-health-text")?.textContent.trim() || "",
        marketDot: document.querySelector("#market-health-dot")?.className || "",
        analysisLabel: document.querySelector("#analysis-health-text")?.textContent.trim() || "",
        analysisDot: document.querySelector("#analysis-health-dot")?.className || "",
        connectionLabel: document.querySelector("#connection-text")?.textContent.trim() || "",
        connectionDot: document.querySelector("#connection-dot")?.className || ""
      }));
      assert.equal(topbarHealth.marketLabel, "状态未知");
      assert.match(topbarHealth.marketDot, /is-unknown/);
      assert.doesNotMatch(topbarHealth.marketDot, /is-online/);
      assert.equal(topbarHealth.analysisLabel, "状态未知");
      assert.match(topbarHealth.analysisDot, /is-unknown/);
      assert.doesNotMatch(topbarHealth.analysisDot, /is-online/);
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
    }

    if (await menu.isVisible() && (await menu.getAttribute("aria-expanded")) !== "true") {
      await menu.click();
    }
    await page.locator('[data-view="tasks"]').click();
    await page.locator("#tasks-view").waitFor({ state: "visible" });

    const flow = await page.locator("#run-flow-dialog").evaluate((dialog) => {
      const emptyState = document.querySelector("#task-empty");
      const detail = document.querySelector("#task-detail");
      const eventDetail = document.querySelector("#task-event-detail");
      if (emptyState) emptyState.hidden = true;
      if (detail) detail.hidden = false;
      if (eventDetail) eventDetail.hidden = false;
      if (!dialog.open) dialog.showModal();

      const rect = dialog.getBoundingClientRect();
      const detailRect = detail.getBoundingClientRect();
      const eventDetailRect = eventDetail.getBoundingClientRect();
      return {
        left: rect.left,
        right: rect.right,
        width: rect.width,
        height: rect.height,
        detailWidth: detailRect.width,
        detailColumns: getComputedStyle(detail).gridTemplateColumns.split(" ").filter(Boolean).length,
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

    if (viewport.width > 760) {
      assert.ok(flow.detailColumns >= 2, "Run Flow should use split columns above 760px; got " + flow.detailColumns + " at " + viewport.width + "px");
    } else {
      assert.equal(flow.detailColumns, 1, "Run Flow should stack into one column at " + viewport.width + "px");
    }

    await page.screenshot({
      path: path.join(screenshotDir, viewport.width + "x" + viewport.height + "-run-flow.png"),
      fullPage: false,
      animations: "disabled"
    });

    await page.keyboard.press("Escape");
    assert.equal(await page.locator("#run-flow-dialog").evaluate((dialog) => dialog.open), false, "Run Flow should close with Escape");

    assert.deepEqual(pageErrors, [], "browser page errors at " + viewport.width + "px: " + pageErrors.join(" | "));
    console.log("PASS viewport " + viewport.width + "x" + viewport.height + ": navigation, overflow, Run Flow layout");

    await context.close();
  }

  console.log("PASS all " + viewports.length + " responsive browser viewports");
  console.log("Screenshots: " + screenshotDir);
} finally {
  if (browser) await browser.close();
  await new Promise((resolve) => server.close(resolve));
}

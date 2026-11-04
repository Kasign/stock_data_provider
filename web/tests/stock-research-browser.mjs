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
let releasePeriodRequest;
let markPeriodRequestStarted;
let releaseCrossTabResearchRequest;
let markCrossTabResearchRequestStarted;
let markCrossTabResearchRequestSettled;
let crossTabResearchRouteFinished = false;
try {
  browser = await chromium.launch({ headless: true });

  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "research-browser-key"
    }));
    localStorage.setItem("stock-provider-webui.watchlist.v1", JSON.stringify([
      { symbol: "000001.SZ", name: "平安银行", updated_at: "2026-11-03T00:00:00Z" }
    ]));
  });
  const page = await context.newPage();
  const periodRequestStarted = new Promise(resolve => { markPeriodRequestStarted = resolve; });
  const crossTabResearchRequestStarted = new Promise(resolve => { markCrossTabResearchRequestStarted = resolve; });
  const crossTabResearchRequestSettled = new Promise(resolve => { markCrossTabResearchRequestSettled = resolve; });
  const pageErrors = [];
  let crossTabProfileRequestFailure = "";
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  page.on("requestfailed", request => {
    if (request.url().includes("/api/v1/stocks/000001.SZ/profile")) {
      crossTabProfileRequestFailure = request.failure()?.errorText || "requestfailed";
    }
  });
  oldRequestStarted = new Promise(resolve => {
    page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.pathname.includes("/api/v1/market/hot-stocks")) {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items: [{ symbol: "600519.SH", name: "贵州茅台", rank: 1, latest_price: 1500, change: 10, change_percent: 1.2, rank_change: 0 }],
          quality_status: "valid", source: "browser-fixture", fetched_at: "2026-11-03T03:00:00Z"
        }) });
        return;
      }
      if (url.pathname.includes("/api/v1/market/limit-pools")) {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items: [{ symbol: "000001.SZ", name: "平安银行", change_percent: 10, consecutive_boards: 1 }],
          type: "up", total: 1, trade_date: "2026-11-03", quality_status: "valid", source: "browser-fixture", fetched_at: "2026-11-03T03:00:00Z"
        }) });
        return;
      }
      if (url.pathname.includes("/api/v1/market/dragon-tiger")) {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items: [{ symbol: "300750.SZ", name: "宁德时代", change_percent: 2.1, net_buy_amount_cny: 1000000, deal_amount_cny: 5000000, quality_status: "valid" }],
          trade_date: "2026-11-03", page: 1, returned: 1, source_returned: 1, excluded_non_a_share_count: 0,
          has_more: false, quality_status: "valid", source: "browser-fixture", fetched_at: "2026-11-03T03:00:00Z"
        }) });
        return;
      }
      if (url.pathname === "/market-api/api/v1/market/quotes") {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
          items: [{
            symbol: "000001.SZ",
            quote: { symbol: "000001.SZ", name: "平安银行", current_price: 10.5, change_percent: 1.2, amount: 123456, quality_status: "valid", quote_time: "2026-11-04T01:00:00Z" },
            quality_status: "valid"
          }],
          succeeded: 1, total: 1, failed: 0, quality_status: "valid", fetched_at: "2026-11-04T01:01:00Z"
        }) });
        return;
      }
      if (url.pathname.includes("/api/v1/market/stocks")) {
        await route.fulfill({
          status: 200, contentType: "application/json",
          body: JSON.stringify({
            items: [{
              symbol: "600519.SH", name: "贵州茅台", quality_status: "valid",
              quote: { symbol: "600519.SH", name: "贵州茅台", current_price: 1500, change_percent: 1.2, quality_status: "valid" },
              ma5: 1490, ma20: 1480
            }],
            page: 1, page_size: 50, total: 1, total_is_lower_bound: false,
            coverage_ratio: 1, quote_succeeded: 1, quote_failed: 0, quality_status: "valid",
            universe_source: "browser-fixture", universe_source_version: "test", universe_fetched_at: "2026-11-03T03:00:00Z",
            fetched_at: "2026-11-03T03:00:00Z", ma_adjustment_basis: "forward_adjusted"
          })
        });
        return;
      }
      if (url.pathname.endsWith("/api/v1/stocks/000001.SZ/profile")) {
        markCrossTabResearchRequestStarted?.();
        await new Promise(done => { releaseCrossTabResearchRequest = done; });
        try {
          await route.fulfill({
            status: 200, contentType: "application/json",
            body: JSON.stringify({
              symbol: "000001.SZ", name: "延迟旧连接档案", quality_status: "valid",
              source: "browser-fixture", normalization_version: "stale-connection",
            })
          });
        } catch {
          // 浏览器可能已取消请求；路由 fulfill 的异常行为不作为取消判据。
        } finally {
          crossTabResearchRouteFinished = true;
          markCrossTabResearchRequestSettled?.();
        }
        return;
      }
      if (url.pathname.includes("/api/v1/stocks/000001.SZ/fundamentals")) {
        markPeriodRequestStarted?.();
        await new Promise(done => { releasePeriodRequest = done; });
        try {
          await route.fulfill({
            status: 200, contentType: "application/json",
            body: JSON.stringify({
              symbol: "000001.SZ", year: 2026, quarter: 3, quality_status: "valid",
              source: "browser-fixture", sections: []
            })
          });
        } catch {
          // 财务报告期切换后旧请求应已被取消。
        }
        return;
      }
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
      const json = (status, body) => route.fulfill({
        status, contentType: "application/json", body: JSON.stringify(body)
      });
      if (url.pathname === "/analysis-api/api/v1/analysis/tasks" && route.request().method() === "POST") {
        const payload = route.request().postDataJSON();
        assert.deepEqual(payload.symbols, ["000001"], "研究页创建任务必须使用规范化后的当前标的");
        assert.equal(payload.task_type, "post_market_scan");
        return json(202, { task: {
          task_id: "task-research-archive", run_id: "run-research-archive",
          status: "queued", task_type: "post_market_scan", symbols: ["000001"],
          created_at: "2026-11-03T03:00:00Z"
        }, status_url: "/api/v1/analysis/tasks/task-research-archive",
        events_url: "/api/v1/analysis/tasks/task-research-archive/events" });
      }
      if (url.pathname === "/analysis-api/api/v1/analysis/tasks/task-research-archive") {
        return json(200, { task: {
          task_id: "task-research-archive", run_id: "run-research-archive",
          status: "succeeded", task_type: "post_market_scan", symbols: ["000001"],
          created_at: "2026-11-03T03:00:00Z", completed_stages: 1
        } });
      }
      if (url.pathname === "/analysis-api/api/v1/analysis/tasks/task-research-archive/events") {
        return json(200, { task_id: "task-research-archive", next_after_sequence: 1, events: [{
          sequence: 1, event: {
            run_id: "run-research-archive", stage: "研究任务完成", status: "completed",
            message: "浏览器归档闭环专项", at: "2026-11-03T03:00:01Z"
          }
        }] });
      }
      if (url.pathname === "/analysis-api/api/v1/analysis/runs/run-research-archive") {
        return json(200, {
          run_id: "run-research-archive", status: "completed",
          request: { symbols: ["000001"], task_type: "post_market_scan", as_of: "2026-11-03T03:00:00Z" },
          quality: { core_pass: true, checked_at: "2026-11-03T03:00:01Z", issues: [] },
          factors: [], signals: [], plans: [], warnings: [],
          market_state: { regime: "unknown", evidence: [] }
        });
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

  // 市场人气榜、涨跌停池和龙虎榜均应能直达研究工作区。
  await page.locator('.nav-link[data-view="market"]').click();
  await page.locator('#market-hot-stocks-body button[data-hot-action="research"]').waitFor({ state: "visible" });
  await page.locator('#market-hot-stocks-body button[data-hot-action="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.waitForFunction(() => document.querySelector("#symbol-input")?.value === "600519.SH");
  await page.locator('.nav-link[data-view="market"]').click();
  await page.locator('#market-pool-body button[data-pool-action="research"]').waitFor({ state: "visible" });
  await page.locator('#market-pool-body button[data-pool-action="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.waitForFunction(() => document.querySelector("#symbol-input")?.value === "000001.SZ");
  await page.locator('.nav-link[data-view="market"]').click();
  await page.locator('#dragon-tiger-body button[data-dragon-action="research"]').waitFor({ state: "visible" });
  await page.locator('#dragon-tiger-body button[data-dragon-action="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.waitForFunction(() => document.querySelector("#symbol-input")?.value === "300750.SZ");

  // 全市场分页结果应能直接进入研究工作区，并同步当前证券代码。
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#load-market-stocks").click();
  await page.locator('#market-stocks-body button[data-market-stock-action="research"]').waitFor({ state: "visible" });
  const marketRowText = await page.locator("#market-stocks-body tr").first().textContent();
  assert.match(marketRowText, /1,500/);
  assert.match(marketRowText, /1,490/);
  assert.match(marketRowText, /1,480/);
  assert.doesNotMatch(marketRowText, /function formatPrice/);
  await page.locator('#market-stocks-body button[data-market-stock-action="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.waitForFunction(() =>
    document.querySelector("#symbol-input")?.value === "600519.SH" &&
    document.querySelector("#research-overview-symbol")?.textContent.includes("600519.SH")
  );
  assert.equal(await page.locator("#research-overview-symbol").textContent().then(value => value.includes("600519.SH")), true);

  // 本地自选股应提供独立研究入口，并保留自选名称与代码上下文。
  await page.locator('.nav-link[data-view="watchlist"]').click();
  await page.locator('#watchlist-list button[data-watchlist-action="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.waitForFunction(() =>
    document.querySelector("#symbol-input")?.value === "000001.SZ" &&
    document.querySelector("#research-overview-symbol")?.textContent.includes("000001.SZ")
  );
  assert.equal(await page.locator("#research-overview-symbol").textContent().then(value => value.includes("000001.SZ")), true);

  // 研究列表复用自选股批量行情；连接配置从其它标签页变化后，当前研究页必须立即清除旧连接快照。
  await page.locator('.nav-link[data-view="watchlist"]').click();
  await page.locator("#refresh-watchlist-quotes").click();
  await page.waitForFunction(() => document.querySelector("#watchlist-quote-refresh-status")?.textContent.includes("批量行情已返回 1 / 1"));
  await page.locator('.nav-link[data-view="research"]').click();
  await page.waitForFunction(() => document.querySelector("#research-stock-list-body")?.textContent.includes("10.50"));
  const settingsSyncPage = await context.newPage();
  await settingsSyncPage.goto(baseUrl, { waitUntil: "domcontentloaded" });
  await settingsSyncPage.evaluate(() => localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
    marketBase: "/market-api-v2", analysisBase: "/analysis-api", apiKey: "research-browser-key-v2"
  })));
  await page.waitForFunction(() => {
    const text = document.querySelector("#research-stock-list-body")?.textContent || "";
    return !text.includes("10.50") && text.includes("未加载");
  });
  assert.match(await page.locator("#research-list-scope").textContent(), /当前浏览器列表/);
  await settingsSyncPage.close();

  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#symbol-input").fill("600519.SH");
  await page.locator('.nav-link[data-view="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });

  // 当前研究标的切换后，批量行情只能继续用于仍属于当前自选清单的成员。
  const membershipSyncPage = await context.newPage();
  await membershipSyncPage.goto(baseUrl, { waitUntil: "domcontentloaded" });
  await membershipSyncPage.evaluate(() => localStorage.setItem("stock-provider-webui.watchlist.v1", "[]"));
  await page.waitForFunction(() => {
    const text = document.querySelector("#research-stock-list-body")?.textContent || "";
    return !text.includes("10.50") && text.includes("当前没有自选股");
  });
  await membershipSyncPage.close();

  // 个股研究资料在途时，另一标签页切换行情连接；必须取消旧请求并恢复整组加载按钮。
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#symbol-input").fill("000001.SZ");
  await page.locator('.nav-link[data-view="research"]').click();
  const crossTabProfileRequestFailed = page.waitForEvent("requestfailed", request =>
    request.url().includes("/api/v1/stocks/000001.SZ/profile")
  );
  await page.locator("#load-stock-research").click();
  await crossTabResearchRequestStarted;
  const researchSettingsSyncPage = await context.newPage();
  await researchSettingsSyncPage.goto(baseUrl, { waitUntil: "domcontentloaded" });
  await researchSettingsSyncPage.evaluate(() => localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
    marketBase: "/market-api-v3", analysisBase: "/analysis-api", apiKey: "research-browser-key-v3"
  })));
  await page.waitForFunction(() => {
    const button = document.querySelector("#load-stock-research");
    return button && !button.disabled && button.textContent.includes("重新加载公司资料");
  });
  await page.waitForFunction(() => document.querySelector("#stock-research-feedback")?.textContent.includes("行情连接配置已变更"));
  assert.equal(await page.locator("#load-stock-research").isEnabled(), true);
  assert.equal(await page.locator("#load-fundamentals").isEnabled(), true);
  assert.equal(await page.locator("#load-valuations").isEnabled(), true);
  assert.equal(await page.locator("#stock-profile-symbol").textContent(), "000001.SZ");
  assert.doesNotMatch(await page.locator("#stock-profile-fields").textContent(), /延迟旧连接档案|stale-connection/);
  const failedProfileRequest = await crossTabProfileRequestFailed;
  crossTabProfileRequestFailure = failedProfileRequest.failure()?.errorText || crossTabProfileRequestFailure;
  assert.match(crossTabProfileRequestFailure, /ERR_ABORTED|aborted/i, "跨标签页连接变更后旧档案请求必须由浏览器取消");
  releaseCrossTabResearchRequest?.();
  await crossTabResearchRequestSettled;
  assert.equal(crossTabResearchRouteFinished, true);
  assert.doesNotMatch(await page.locator("#stock-profile-fields").textContent(), /延迟旧连接档案|stale-connection/);
  await researchSettingsSyncPage.close();

  await page.locator("#load-fundamentals").click();
  await oldRequestStarted;

  // 切换标的后必须中止旧请求，并恢复所有请求按钮，包括财务指标按钮。
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#symbol-input").fill("000001.SZ");
  await page.locator('.nav-link[data-view="research"]').click();
  await page.waitForFunction(() => {
    const button = document.querySelector("#load-fundamentals");
    return button && !button.disabled && button.textContent.includes("读取财务指标");
  });
  releaseOldRequest();
  await page.waitForTimeout(100);
  assert.equal(await page.locator("#fundamentals-symbol").textContent(), "000001.SZ");
  assert.doesNotMatch(await page.locator("#fundamentals-content").textContent(), /延迟旧标的指标|999/);
  assert.equal(await page.locator("#load-fundamentals").isEnabled(), true);

  // 财务报告期切换会取消当前请求，但必须立即恢复读取按钮，允许按新报告期重试。
  await page.locator("#load-fundamentals").click();
  await periodRequestStarted;
  await page.locator("#fundamentals-quarter").selectOption("2");
  await page.waitForFunction(() => {
    const button = document.querySelector("#load-fundamentals");
    return button && !button.disabled && button.textContent.includes("读取财务指标");
  });
  releasePeriodRequest();
  await page.waitForTimeout(100);
  assert.equal(await page.locator("#load-fundamentals").isEnabled(), true);
  assert.doesNotMatch(await page.locator("#fundamentals-content").textContent(), /2026 Q3/);

  // 其它补充模块的 503 状态应清楚展示，且不锁死主资料按钮。
  await page.locator("#load-stock-research").click();
  await page.waitForFunction(() => document.querySelector("#stock-profile-error")?.textContent.includes("未启用或未配置"));
  assert.equal(await page.locator("#stock-profile-symbol").textContent(), "000001.SZ");
  assert.equal(await page.locator("#load-stock-research").isEnabled(), true);

  // 从研究页创建分析任务，读取真实 Run 回包，再归档并从报告创建关联笔记。
  await page.locator("#research-start-analysis").click();
  await page.locator("#task-detail").waitFor({ state: "visible" });
  await page.waitForFunction(() => !document.querySelector("#task-result-section")?.hidden, null, { timeout: 15000 });
  await page.locator("#archive-run-report").click();
  await page.locator('.nav-link[data-view="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });
  await page.locator('#research-report-list [data-report-action="create-note"]').waitFor({ state: "visible" });
  await page.locator('#research-report-list [data-report-action="create-note"]').click();
  await page.waitForFunction(() =>
    document.querySelector("#research-note-task")?.value === "task-research-archive" &&
    document.querySelector("#research-note-run")?.value === "run-research-archive"
  );
  assert.equal(await page.locator("#research-note-symbol").inputValue(), "000001");
  await page.locator("#research-note-content").fill("浏览器专项：报告与任务关联一致。");
  await page.locator("#research-note-save").click();
  await page.locator("#research-note-list .research-note-card").filter({ hasText: "task-research-archive" }).waitFor({ state: "visible" });
  const archived = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.research-reports.v1") || "[]"));
  assert.equal(archived.length, 1);
  assert.equal(archived[0].taskId, "task-research-archive");
  assert.equal(archived[0].runId, "run-research-archive");
  assert.equal(archived[0].analysisBase, "/analysis-api");
  assert.deepEqual(archived[0].symbols, ["000001"]);
  const notes = await page.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.research-notes.v1") || "[]"));
  assert.equal(notes.length, 1);
  assert.equal(notes[0].taskId, "task-research-archive");
  assert.equal(notes[0].runId, "run-research-archive");
  assert.equal(notes[0].symbol, "000001");
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
  await noKeyPage.locator('.nav-link[data-view="stocks"]').click();
  await noKeyPage.locator("#symbol-input").fill("600519.SH");
  await noKeyPage.locator('.nav-link[data-view="research"]').click();
  await noKeyPage.locator("#load-valuations").click();
  await noKeyPage.waitForFunction(() => document.querySelector("#valuations-quality")?.textContent === "需配置 Key");
  for (const selector of [
    "#load-stock-research", "#load-fundamentals", "#load-valuations",
    "#load-chip-distribution", "#load-capital-flow"
  ]) {
    assert.equal(await noKeyPage.locator(selector).isEnabled(), true, selector + " must remain retryable without API key");
    assert.equal(await noKeyPage.locator(selector).getAttribute("aria-busy"), null, selector + " must not remain busy without an active request");
  }
  await noKeyContext.close();
  console.log("PASS stock research request cancellation and button recovery");
} finally {
  if (releaseOldRequest) releaseOldRequest();
  if (releaseCrossTabResearchRequest) releaseCrossTabResearchRequest();
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(webRoot, "..");
const screenshotDir = path.join(repositoryRoot, "browser-screenshots", "data-center-probe");
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
      : extension === ".css" ? "text/css; charset=utf-8"
        : [".js", ".mjs"].includes(extension) ? "text/javascript; charset=utf-8"
          : "application/octet-stream";
    response.writeHead(200, { "content-type": contentType, "cache-control": "no-store" });
    response.end(body);
  } catch {
    response.writeHead(404); response.end("Not found");
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
const checkedAt = "2026-11-03T02:00:00Z";
const makeItem = (provider, capability, status, extra = {}) => ({
  provider, capability, status, elapsed_ms: 40, quality_status: "valid", checked_at: checkedAt, ...extra
});
const currentRun = {
  run_id: "probe-current", scope: "core", status: "degraded", persisted: true,
  started_at: checkedAt, finished_at: "2026-11-03T02:00:05Z", timeout_seconds: 9,
  requested_checks: ["connectivity", "contract", "quality", "freshness", "cross_provider", "storage"],
  total: 7, returned: 1, degraded: 1, unknown: 1, failed: 1, skipped: 1,
  not_configured: 1, not_supported: 1,
  check_results: [
    { check: "connectivity", status: "passed", checked_at: checkedAt },
    { check: "contract", status: "passed", checked_at: checkedAt },
    { check: "quality", status: "degraded", checked_at: checkedAt, error_code: "quality_degraded", error_message: "部分行情字段缺失" },
    { check: "freshness", status: "degraded", checked_at: checkedAt, freshness_status: "stale", age_seconds: 900, freshness_max_age_seconds: 300, freshness_reason: "source_snapshot_stale", market_phase: "after_close" },
    { check: "cross_provider", status: "failed", checked_at: checkedAt, mismatch_count: 1, field_count: 2, matched_field_count: 1, not_comparable_count: 0, compared_dates: ["2026-11-02"], field_comparisons: [
      { field: "2026-11-02.close", status: "mismatch", left: 10.2, right: 10.4, difference: 0.2, tolerance: 0.01, reason: "两来源收盘价不一致" }
    ] },
    { check: "storage", status: "passed", checked_at: checkedAt }
  ],
  items: [
    makeItem("Tushare", "daily_bars", "returned", {
      sample_symbol: "600519.SH", rows_returned: 300, rows_validated: 298,
      coverage_total: 1000, coverage_returned: 1000, coverage_validated: 993,
      coverage_missing: 5, coverage_excluded: 2, coverage_ratio: 0.993,
      coverage_population: "provider_universe", coverage_source: "provider_total",
      source_version: "daily-v2", provider_time: checkedAt, provider_time_type: "source_snapshot"
    }),
    makeItem("Eastmoney", "quote", "returned_degraded", {
      quality_status: "degraded", quality_reasons: ["部分行情字段未返回"]
    }),
    makeItem("Public", "market_breadth", "returned_unknown", {
      quality_status: "unknown", quality_reasons: ["报告日期无法确认"],
      coverage_dimensions: [
        { dimension: "quote_response", population: "provider_universe", source: "quote_returned", total: 1000, returned: 990, validated: 990, missing: 10, excluded: 0, ratio: 0.99 },
        { dimension: "price_classification", population: "provider_universe", source: "classified_price_count", total: 1000, returned: 990, validated: 970, missing: 10, excluded: 20, ratio: 0.97 },
        { dimension: "quote_timestamp", population: "provider_universe", source: "timestamp_count", total: 1000, returned: 990, validated: 960, missing: 10, excluded: 30, ratio: 0.96 },
        { dimension: "amount", population: "provider_universe", source: "amount_count", total: 1000, returned: 990, validated: 900, missing: 10, excluded: 90, ratio: 0.9 }
      ]
    }),
    makeItem("Tencent", "quote", "returned_degraded", {
      quality_status: "degraded",
      validation_total_count: 3, validation_error_count: 1,
      validation_warning_count: 1, validation_info_count: 1,
      validation_truncated: true,
      validation_issues: [
        { stage: "parse", field: "current_price", provider_field: "f43", code: "invalid_number", expected: "有限数值", actual_type: "string", actual_preview: "NaN", severity: "error", message: "原始行情价格不是有限数值", row_index: 0, symbol: "600519.SH" },
        { stage: "normalize", field: "amount", provider_field: "amount", code: "unit_mismatch", expected: "CNY", actual_type: "number", actual_preview: "unknown", severity: "warning", message: "成交额单位未确认" }
      ]
    }),
    makeItem("Provider-A", "stock_search", "failed", {
      error_code: "provider_timeout", error_stage: "timeout", http_status: 504,
      error_message: "上游请求超时"
    }),
    makeItem("Provider-B", "index_quotes", "skipped"),
    makeItem("Hithink", "capital_flow", "not_configured", {
      error_code: "provider_not_configured", error_stage: "configuration",
      error_message: "未配置第三方凭据"
    }),
    makeItem("Provider-C", "index_catalog", "not_supported"),
    makeItem("Provider-D", "daily_bars", "returned", {
      elapsed_ms: null, coverage_total: 100, coverage_ratio: "not-a-number"
    })
  ]
};
const oldRun = {
  ...currentRun, run_id: "probe-history-old", status: "failed", persisted: true,
  started_at: "2026-11-02T02:00:00Z", finished_at: "2026-11-02T02:00:03Z",
  total: 1, returned: 0, degraded: 0, unknown: 0, failed: 1, skipped: 0,
  not_configured: 0, not_supported: 0, check_results: [],
  items: [makeItem("Provider-A", "quote", "failed", {
    error_code: "connection_refused", error_stage: "connect", error_message: "连接失败"
  })]
};
let probeRequests = 0;
let historyRequests = 0;
let historyUnavailable = false;
let probeRequestBody = null;
let browser;
try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "data-center-probe-key"
    }));
  });
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  page.route("**/api/**", async route => {
    const request = route.request();
    const url = new URL(request.url());
    const pathname = url.pathname;
    const json = (status, body) => route.fulfill({
      status, contentType: "application/json", body: JSON.stringify(body)
    });
    if (pathname.endsWith("/health/live") || pathname.endsWith("/healthz")) return json(200, { status: "ok" });
    if (pathname.endsWith("/health/ready")) return json(200, { status: "ready" });
    if (pathname.endsWith("/readyz")) return json(200, { status: "ready", dependencies: { mariadb: "ok", redis: "ok" } });
    if (pathname.endsWith("/api/v1/data-sources/probe/history")) {
      historyRequests += 1;
      if (historyUnavailable) return json(503, { error: { code: "probe_history_unavailable", message: "探测历史存储不可用" } });
      return json(200, { total: probeRequests ? 2 : 1, items: probeRequests ? [currentRun, oldRun] : [oldRun] });
    }
    if (pathname.endsWith("/api/v1/data-sources/probe") && request.method() === "POST") {
      probeRequests += 1;
      probeRequestBody = request.postDataJSON();
      if (probeRequests > 1) return json(500, { error: { code: "probe_persist_failed", message: "探测结果持久化失败" } });
      return json(200, currentRun);
    }
    if (pathname.endsWith("/api/v1/data-sources/diagnostics")) {
      return json(200, { items: [], scope: ["quote", "daily_bars"], observed_from: "in_process_circuit_breaker" });
    }
    if (pathname.endsWith("/api/v1/data-sources")) {
      return json(200, { sources: [{ name: "eastmoney", enabled: true, quote_capable: true }] });
    }
    return json(200, {});
  });

  await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await page.locator('.nav-link[data-view="data"]').click();
  await page.locator("#data-view").waitFor({ state: "visible" });
  await page.locator("#data-center-check").click();
  await page.waitForFunction(() => document.querySelector("#data-center-probe-history-status")?.textContent.includes("已读取"));
  assert.match(await page.locator("#data-center-probe-history-list").textContent(), /probe-history-old/);

  await page.locator("#data-center-probe-scope").selectOption("core");
  await page.locator("#data-center-probe-timeout").fill("9");
  await page.locator("#data-center-probe-market").selectOption("SH_MAIN");
  await page.locator("#data-center-probe-symbol").fill("600519.sh");
  await page.locator("#data-center-probe-run").click();
  await page.waitForFunction(() => document.querySelector("#data-center-probe-summary")?.textContent.includes("probe-current"));
  assert.deepEqual(probeRequestBody, {
    scope: "core", timeout_seconds: 9,
    checks: ["connectivity", "contract", "quality", "freshness", "cross_provider", "storage"],
    markets: ["SH_MAIN"], symbols: ["600519.SH"]
  });
  const probeText = await page.locator("#data-center-probe-list").textContent();
  for (const label of ["已返回", "返回但质量降级", "返回但质量未知", "失败", "跳过", "未配置", "不支持"]) {
   assert.ok(probeText.includes(label), "missing probe status: " + label);
  }
  assert.match(probeText, /99\.3%/);
  assert.match(probeText, /provider_timeout/);
   assert.match(probeText, /Provider-D/);
   assert.doesNotMatch(probeText, /Provider-D[\s\S]*?耗时 0 ms/);
   assert.doesNotMatch(probeText, /Provider-D[\s\S]*?NaN%/);
   assert.match(probeText, /Provider-D[\s\S]*?覆盖率不可确认/);
  assert.match(probeText, /上游请求超时/);
  assert.match(probeText, /字段校验 3 项/);
  assert.match(probeText, /错误 1/);
  assert.match(probeText, /原始字段 f43/);
  assert.match(probeText, /实际值 NaN/);
  assert.match(probeText, /服务端明细已截断/);
  for (const label of ["行情响应覆盖率", "价格分类覆盖率", "行情时间覆盖率", "成交额覆盖率"]) {
   assert.ok(probeText.includes(label), "missing coverage dimension: " + label);
  }
  assert.match(probeText, /99\.0%/);
  assert.match(probeText, /90\.0%/);
  const checkText = await page.locator("#data-center-probe-check-results").textContent();
  for (const label of ["接口连通性", "数据契约", "数据质量", "数据新鲜度", "跨 Provider 对账", "持久化存储", "已过期", "不一致"]) {
   assert.ok(checkText.includes(label), "missing check evidence: " + label);
  }
  await page.waitForFunction(() => document.querySelector("#data-center-probe-history-list")?.textContent.includes("probe-current"));
  assert.match(await page.locator("#data-center-probe-history-list").textContent(), /probe-history-old/);

  historyUnavailable = true;
  await page.locator("#data-center-probe-history-refresh").click();
  await page.waitForFunction(() => document.querySelector("#data-center-probe-history-summary")?.textContent.includes("HTTP 503"));
  assert.match(await page.locator("#data-center-probe-list").textContent(), /provider_timeout/);

  await page.locator("#data-center-probe-run").click();
  await page.waitForFunction(() => document.querySelector("#data-center-probe-summary")?.textContent.includes("主动探测请求失败"));
  assert.equal(await page.locator("#data-center-probe-run").isEnabled(), true);
  assert.equal(await page.locator("#data-center-probe-run").getAttribute("aria-busy"), null);
  assert.doesNotMatch(await page.locator("#data-center-probe-list").textContent(), /provider_timeout/);
  assert.ok(historyRequests >= 3);
  assert.deepEqual(pageErrors, [], pageErrors.join("\n"));
  await page.screenshot({
    path: path.join(screenshotDir, "1280x900-data-center-probe.png"),
    fullPage: false, animations: "disabled"
  });
  await context.close();

  const noKeyContext = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await noKeyContext.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: ""
    }));
  });
  const noKeyPage = await noKeyContext.newPage();
  let noKeyRequests = 0;
  await noKeyPage.route("**/api/v1/data-sources/probe**", async route => {
    noKeyRequests += 1;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [], total: 0 }) });
  });
  await noKeyPage.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await noKeyPage.locator('.nav-link[data-view="data"]').click();
  await noKeyPage.locator("#data-center-probe-run").click();
  await noKeyPage.locator("#data-center-probe-history-refresh").click();
  assert.equal(noKeyRequests, 0, "missing API Key must not issue probe or history requests");
  assert.match(await noKeyPage.locator("#data-center-probe-summary").textContent(), /未发起主动探测/);
  assert.equal(await noKeyPage.locator("#data-center-probe-run").isEnabled(), true);
  await noKeyContext.close();
  console.log("PASS data center active Provider probe, six diagnostic dimensions and history");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

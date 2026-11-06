import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(webRoot, "..");
const screenshotDir = path.join(repositoryRoot, "browser-screenshots", "stock-research-data");
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
let capitalFlowRequests = 0;
let announcementPage2Requests = 0;
let releaseAnnouncementPage2;
let markAnnouncementPage2Started;
const announcementPage2Started = new Promise(resolve => { markAnnouncementPage2Started = resolve; });
try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "research-data-browser-key"
    }));
  });
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", error => pageErrors.push(error.stack || error.message));
  page.route("**/api/**", async route => {
    const url = new URL(route.request().url());
    const pathname = url.pathname;
    const json = (status, body) => route.fulfill({
      status, contentType: "application/json", body: JSON.stringify(body)
    });
    if (pathname.endsWith("/profile")) {
      return json(200, {
        symbol: "000001.SZ", name: "平安银行", security_type_label: "普通股",
        listing_status_label: "上市", ipo_date: "1991-04-03", source: "browser-fixture",
        normalization_version: "fixture-v1", fetched_at: "2026-11-03T01:00:00Z",
        quality_status: "stale", quality_reasons: ["公司档案源端更新时间较旧"]
      });
    }
    if (pathname.endsWith("/boards")) {
      return json(200, {
        symbol: "000001.SZ", items: [{
          code: "BK001", name: "银行板块", change_percent: 0.8, source: "browser-fixture",
          source_version: "boards-v2", fetched_at: "2026-11-03T01:00:00Z",
          quality_status: "degraded", quality_reasons: ["板块记录来源字段不完整"]
        }], total: 1, has_data: true,
        source: "browser-fixture", fetched_at: "2026-11-03T01:00:00Z",
        quality_status: "degraded", quality_reasons: ["板块快照存在降级记录"]
      });
    }
    if (pathname.endsWith("/announcements")) {
      const pageNumber = Number(url.searchParams.get("page") || "1");
      if (pageNumber === 2 && announcementPage2Requests++ === 0) {
        markAnnouncementPage2Started();
        await new Promise(resolve => { releaseAnnouncementPage2 = resolve; });
        try {
          return json(200, {
            symbol: "000001.SZ",
            items: [{ title_chinese: "已失效的延迟公告", quality_status: "valid", announcement_date: "2026-10-15" }],
            has_more: false, source: "browser-fixture"
          });
        } catch {
          return;
        }
      }
      const pageOneCount = (announcementPage2Requests >= 1 && pageNumber === 1)
        ? "平安银行刷新后的公告列表"
        : "平安银行 2026 年第三季度报告";
      const items = pageNumber === 1
        ? [{ title_chinese: pageOneCount, source_url: "https://www.eastmoney.com/report/1",
            quality_status: "degraded", quality_reasons: ["公告标题使用回退语言字段"], announcement_date: "2026-10-28", published_at: "2026-10-28T18:00:00Z", category: "定期报告" }]
        : [{ title_chinese: "平安银行股东大会决议公告", source_url: "https://notice.eastmoney.com/report/3",
            quality_status: "valid", announcement_date: "2026-10-15", category: "公司公告" }];
      return json(200, {
        symbol: "000001.SZ", items, start_date: "2026-10-01", end_date: "2026-11-03",
        returned: items.length, total: 2, has_more: pageNumber === 1,
        source: "browser-fixture", fetched_at: "2026-11-03T01:00:00Z",
        quality_status: "degraded", quality_reasons: ["公告数据为浏览器合成样例"]
      });
    }
    if (pathname.endsWith("/fundamentals")) {
      return json(200, {
        symbol: "000001.SZ", year: Number(url.searchParams.get("year")), quarter: Number(url.searchParams.get("quarter")),
        quality_status: "degraded", source: "browser-fixture", fetched_at: "2026-11-03T01:00:00Z",
        system_observed_at: "2026-11-03T02:00:00Z",
        quality_reasons: ["部分财务指标缺失"],
        sections: [{
          category: "profitability", quality_status: "degraded", report_date: "2026-09-30",
          publication_date: "2026-10-25", available_at: "2026-10-30T01:30:00Z", has_data: true, quality_reasons: ["每股收益未返回"],
          metrics: [
            { name: "营业收入", key: "revenue", value: 123.45, unit: "亿元" },
            { name: "每股收益", key: "eps", value: null, unit: "元/股" }
          ]
        }]
      });
    }
    if (pathname.endsWith("/valuations")) {
      return json(200, {
        requested_symbols: ["000001.SZ"], missing_symbols: [], quality_status: "stale",
        source: "browser-fixture", provider_timestamp: "2026-11-02T15:00:00Z",
        fetched_at: "2026-11-03T01:00:00Z",
        items: [{
          symbol: "000001.SZ", pe_ttm: null, pe_mrq: -1.2, pb_mrq: 0.8,
          ps_ttm: 1.1, pcf_ttm: null, source: "browser-fixture",
          quality_status: "stale", quality_reasons: ["估值快照不是实时行情"]
        }]
      });
    }
    if (pathname.endsWith("/chip-distribution")) {
      return json(200, {
        symbol: "000001.SZ", has_data: true, quality_status: "degraded",
        average_cost: 12.34, cost_90_low: 10, cost_90_high: 14,
        cost_70_low: 11, cost_70_high: 13, profit_ratio: 0.56,
        concentration_90: 0.2, concentration_70: 0.1,
        source: "browser-fixture", date: "2026-11-02", provider_date: "2026-11-02",
        fetched_at: "2026-11-03T01:00:00Z", quality_reasons: ["筹码数据仅为当日快照"]
      });
    }
    if (pathname.endsWith("/capital-flow")) {
      capitalFlowRequests += 1;
      if (capitalFlowRequests > 1) {
        return json(503, { error: { message: "capital flow provider unavailable" } });
      }
      return json(200, {
        symbol: "000001.SZ", has_data: true, quality_status: "degraded",
        main_net_inflow_yuan: -123456.78, inflow_5d_yuan: null, inflow_10d_yuan: 456789,
        observations: 18, source: "browser-fixture", fetched_at: "2026-11-03T01:00:00Z",
        quality_reasons: ["5 日汇总字段未返回"]
      });
    }
    return json(503, { error: { message: "unmapped research fixture endpoint" } });
  });

  await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
  await page.locator('.nav-link[data-view="stocks"]').click();
  await page.locator("#symbol-input").fill("000001.SZ");
  await page.locator('.nav-link[data-view="research"]').click();
  await page.locator("#research-view").waitFor({ state: "visible" });

  await page.locator("#load-stock-research").click();
  await page.waitForFunction(() =>
    document.querySelector("#stock-profile-fields")?.textContent.includes("平安银行") &&
    document.querySelector("#stock-board-list")?.textContent.includes("银行板块") &&
    document.querySelector("#announcement-list")?.textContent.includes("第三季度报告")
  );
  assert.equal(await page.locator("#announcement-list .stock-announcement-item").count(), 1);
   assert.equal(await page.locator("#announcement-list .stock-announcement-item .supplement-quality").textContent(), "质量降级");
   assert.match(await page.locator("#announcement-list .stock-announcement-item").textContent(), /公告标题使用回退语言字段/);

  // 公告第 2 页在途时触发整组刷新：旧分页响应必须失效，不能追加到新第 1 页。
  await page.locator("#load-more-announcements").click();
  await announcementPage2Started;
  await page.locator("#load-stock-research").click();
  await page.waitForFunction(() =>
    document.querySelector("#announcement-list")?.textContent.includes("刷新后的公告列表")
  );
  releaseAnnouncementPage2();
  await page.waitForTimeout(100);
  assert.equal(await page.locator("#announcement-list .stock-announcement-item").count(), 1);
  assert.doesNotMatch(await page.locator("#announcement-list").textContent(), /已失效的延迟公告/);
  assert.equal(await page.locator("#profile-quality").textContent(), "数据过期");
  assert.match(await page.locator("#stock-profile-reasons").textContent(), /源端更新时间较旧/);
  assert.equal(await page.locator("#boards-quality").textContent(), "质量降级");
  assert.equal(await page.locator("#stock-board-list .supplement-quality").textContent(), "质量降级");
  assert.match(await page.locator("#stock-board-list").textContent(), /板块记录来源字段不完整/);
  assert.match(await page.locator("#stock-board-list").textContent(), /boards-v2/);
  assert.match(await page.locator("#stock-board-list").textContent(), /记录采集时间/);
  assert.match(await page.locator("#stock-board-list").textContent(), /银行板块/);
  assert.equal(await page.locator("#announcement-list a").count(), 1, "only allowlisted HTTPS Eastmoney links are clickable");
  assert.equal(await page.locator("#load-more-announcements").isEnabled(), true);
  await page.locator("#load-more-announcements").click();
  await page.waitForFunction(() => document.querySelector("#announcement-list")?.textContent.includes("股东大会决议公告"));
  assert.equal(await page.locator("#announcement-list .stock-announcement-item").count(), 2);
  assert.equal(await page.locator("#load-more-announcements").isEnabled(), false);
  assert.equal(await page.locator("#load-more-announcements").getAttribute("aria-busy"), null);

  await page.locator("#load-fundamentals").click();
  await page.waitForFunction(() => document.querySelector("#fundamentals-content")?.textContent.includes("营业收入"));
  assert.equal(await page.locator("#fundamentals-quality").textContent(), "质量降级");
  assert.match(await page.locator("#fundamentals-content").textContent(), /123\.45 亿元/);
  assert.match(await page.locator("#fundamentals-content").textContent(), /— 元\/股/);
  assert.match(await page.locator("#fundamentals-meta").textContent(), /Q/);
  assert.match(await page.locator("#fundamentals-content").textContent(), /可用时间 10\/30 09:30/);
  assert.match(await page.locator("#fundamentals-meta").textContent(), /系统观测时间 11\/03 10:00/);

  await page.locator("#load-valuations").click();
  await page.waitForFunction(() => document.querySelector("#valuation-content")?.textContent.includes("市盈率 TTM"));
  assert.equal(await page.locator("#valuations-quality").textContent(), "数据过期");
  assert.match(await page.locator("#valuation-content").textContent(), /未提供/);
  assert.match(await page.locator("#valuation-content").textContent(), /-1\.2 倍/);
  assert.match(await page.locator("#valuations-meta").textContent(), /供应商时间/);

  await page.locator("#load-chip-distribution").click();
  await page.waitForFunction(() => document.querySelector("#chip-distribution-content")?.textContent.includes("平均持仓成本"));
  assert.equal(await page.locator("#chip-quality").textContent(), "质量降级");
  assert.match(await page.locator("#chip-distribution-content").textContent(), /56%/);
  assert.match(await page.locator("#chip-distribution-content").textContent(), /20%/);
  assert.match(await page.locator("#chip-meta").textContent(), /数据日期/);

  await page.locator("#load-capital-flow").click();
  await page.waitForFunction(() => document.querySelector("#capital-flow-content")?.textContent.includes("主力净流入"));
  assert.equal(await page.locator("#capital-flow-quality").textContent(), "质量降级");
  assert.match(await page.locator("#capital-flow-content").textContent(), /-123,456\.78 元/);
  assert.match(await page.locator("#capital-flow-content").textContent(), /未提供/);

  // 重新读取失败时清空旧快照，不能把上一轮成功值留在错误状态下。
  await page.locator("#load-capital-flow").click();
  await page.waitForFunction(() => document.querySelector("#capital-flow-error")?.textContent.includes("未启用或未配置"));
  assert.doesNotMatch(await page.locator("#capital-flow-content").textContent(), /123,456/);
  assert.equal(await page.locator("#load-capital-flow").isEnabled(), true);
  assert.equal(await page.locator("#load-capital-flow").getAttribute("aria-busy"), null);
  assert.deepEqual(pageErrors, [], pageErrors.join("\n"));
  await page.screenshot({
    path: path.join(screenshotDir, "1280x900-stock-research-data.png"),
    fullPage: false,
    animations: "disabled"
  });
  await context.close();
  console.log("PASS stock research data quality, empty state, announcement pagination and error recovery");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

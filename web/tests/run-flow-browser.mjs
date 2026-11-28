import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import {
  makeRunAggregate,
  makeResultsResponse
} from "./fixtures/researchflow.mjs";
const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = path.resolve(webRoot, "..");
const screenshotDir = path.resolve(process.env.BROWSER_SCREENSHOT_DIR || path.join(repoRoot, "browser-screenshots", "run-flow-interaction"));
const viewports = [{ width: 320, height: 800 }, { width: 768, height: 1024 }, { width: 900, height: 900 }, { width: 1280, height: 900 }, { width: 1600, height: 1000 }];

// 历史记录中的 task_id 就是 Researchflow run_ref；标的按契约要求带交易所后缀。
const tasks = [
  { task_id: "run-flow-b", run_id: "run-flow-b", created_at: "2026-10-30T06:30:00Z", symbols: ["000001.SZ"], task_type: "post_market_scan", status: "queued", baseRef: "/", updated_at: "2026-10-30T06:30:00Z" },
  { task_id: "run-flow-a", run_id: "run-flow-a", created_at: "2026-10-30T06:20:00Z", symbols: ["600519.SH"], task_type: "post_market_scan", status: "succeeded", baseRef: "/", updated_at: "2026-10-30T06:20:00Z" }
];

const server = createServer(async (req, res) => {
  const pathname = decodeURIComponent(new URL(req.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) {
    res.writeHead(403);
    res.end("Forbidden");
    return;
  }
  try {
    const body = await readFile(target);
    const ext = path.extname(target);
    const type = ext === ".html" ? "text/html; charset=utf-8" : ext === ".css" ? "text/css; charset=utf-8" : [".js", ".mjs"].includes(ext) ? "text/javascript; charset=utf-8" : "application/octet-stream";
    res.writeHead(200, { "content-type": type, "cache-control": "no-store" });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end("Not found");
  }
});
await mkdir(screenshotDir, { recursive: true });
await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;
try {
  browser = await chromium.launch({ headless: true });
  for (const viewport of viewports) {
    const context = await browser.newContext({ viewport, reducedMotion: "reduce" });
    await context.addInitScript(seed => {
      // Root Server 单入口：连接配置由生产默认值决定，历史记录以 run_ref 保存。
      localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({ marketBase: "/", analysisBase: "/" }));
      localStorage.setItem("stock-provider-webui.recent-runs.v1", JSON.stringify(seed));
    }, tasks);
    const page = await context.newPage();
    const pageErrors = [];
    page.on("pageerror", e => pageErrors.push(e.message));
    const runPollCounts = new Map();
    const resultReadCounts = new Map();
    let runBReady = false;
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      const pathname = url.pathname;
      const json = (status, body) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
      // Run/Stage 聚合：无事件分页、无 next_after_sequence。
      const aggregateMatch = pathname.match(/^\/api\/v1\/researchflow\/runs\/(run-flow-[ab])$/);
      if (aggregateMatch) {
        const runRef = aggregateMatch[1];
        const count = (runPollCounts.get(runRef) || 0) + 1;
        runPollCounts.set(runRef, count);
        if (runRef === "run-flow-a") {
          // 第一次 running、第二次终态，保留"状态由运行中收敛到 succeeded"的轮询验收。
          const runStatus = count === 1 ? "running" : "succeeded";
          const stageStatuses = count === 1 ? { "quant.sma20": "succeeded", "quant.factor": "running" } : undefined;
          return json(200, makeRunAggregate({ runRef, runStatus, stageStatuses }));
        }
        // run-flow-b 在手动刷新前保持 queued，刷新后才进入终态。
        return json(200, makeRunAggregate({ runRef, runStatus: runBReady ? "succeeded" : "queued" }));
      }
      // 授权领域结果（非完整报告），仅在 Run 终态后读取。
      const resultsMatch = pathname.match(/^\/api\/v1\/researchflow\/runs\/(run-flow-[ab])\/results$/);
      if (resultsMatch) {
        const runRef = resultsMatch[1];
        resultReadCounts.set(runRef, (resultReadCounts.get(runRef) || 0) + 1);
        return json(200, makeResultsResponse({ runRef }));
      }
      if (pathname.includes("/api/") || /\/(?:health\/ready|readyz)$/.test(pathname)) return json(503, { error: "Run Flow interaction browser fixture: endpoint not included in this test" });
      return route.continue();
    });
    await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
    if (viewport.width <= 600) {
      await page.locator("#mobile-menu-toggle").click();
      await page.locator("#primary-sidebar.is-open").waitFor();
    }
    await page.locator('[data-view="tasks"]').click();
    await page.locator('[data-open-task-id="run-flow-a"]').waitFor({ state: "visible" });
    await page.locator('[data-open-task-id="run-flow-a"]').click();
    await page.locator("#run-flow-dialog[open]").waitFor();
    const meta = page.locator(".run-flow-meta");
    assert.equal(await meta.evaluate(n => n.open), false, "task facts stay collapsed by default");
    const summaryColumns = await page.locator("#task-flow-summary").evaluate(n => getComputedStyle(n).gridTemplateColumns.split(" ").length);
    assert.equal(summaryColumns, viewport.width <= 800 ? 2 : 4, "summary cards adapt to viewport width");
    await meta.locator("summary").click();
    assert.equal(await page.locator("#task-id-value").isVisible(), true, "task identity is available when metadata expands");
    await meta.locator("summary").click();
    await page.locator("#task-events .run-flow-node").nth(2).waitFor();
    await page.waitForFunction(() => !document.querySelector("#task-result-section")?.hidden, null, { timeout: 10000 });
    await page.waitForFunction(() => document.querySelector("#task-result-structured")?.children.length > 0, null, { timeout: 5000 });
    // 聚合状态轮询收敛；四个 quant Stage 各渲染一个节点，没有旧事件分页游标。
    assert.ok((runPollCounts.get("run-flow-a") || 0) >= 2, "Run aggregate must be polled until terminal status");
    assert.equal(resultReadCounts.get("run-flow-a"), 1, "results are read once after terminal Run status");
    assert.equal(await page.locator("#task-events .run-flow-node").count(), 4, "four quant stages are projected as flow nodes");
    assert.equal(await page.locator('#task-events .run-flow-node[data-event-sequence="1"]').count(), 1, "first stage keeps a stable sequence");
    assert.deepEqual(await page.locator("#task-events .event-state-label").allTextContents(), ["已完成", "已完成", "已完成", "已完成"], "terminal stages expose succeeded status labels");
    // 降级结果视图：明确非完整研究报告，四类结果卡且 risk 无 payload。
    assert.match(await page.locator("#task-result-note").textContent(), /非完整研究报告/, "result area states it is not a full report");
    assert.deepEqual(await page.locator(".analysis-result-card-heading strong").allTextContents(), ["指标结果", "因子结果", "趋势结果", "风险结果"], "authorized domain results render four result cards");
    assert.match(await page.locator(".analysis-result-card").last().textContent(), /仅状态（无明细）/, "risk result has no payload");
    const layout = await page.evaluate(() => {
      const d = document.querySelector("#run-flow-dialog"), w = document.querySelector(".run-flow-workspace"), r = document.querySelector(".run-flow-result-column"), rect = d.getBoundingClientRect();
      return { left: rect.left, right: rect.right, columns: getComputedStyle(w).gridTemplateColumns.split(" ").filter(Boolean).length, resultClientHeight: r.clientHeight, resultScrollHeight: r.scrollHeight, documentWidth: document.documentElement.scrollWidth };
    });
    assert.ok(layout.left >= -1 && layout.right <= viewport.width + 1, "dialog stays inside " + viewport.width + "px viewport");
    assert.ok(layout.documentWidth <= viewport.width + 1, "no page horizontal overflow at " + viewport.width + "px");
    assert.equal(layout.columns, viewport.width > 900 ? 2 : 1, "workspace breakpoint at " + viewport.width + "px");
    assert.ok(layout.resultScrollHeight > layout.resultClientHeight, "right result column independently scrolls at " + viewport.width + "px");
    await page.locator(".run-flow-result-column").evaluate(n => { n.scrollTop = n.scrollHeight; });
    assert.ok(await page.locator(".run-flow-result-column").evaluate(n => n.scrollTop > 0), "result column scrolls independently");
    const first = page.locator("#task-events .run-flow-node").first();
    await first.locator("summary").click();
    assert.equal(await first.evaluate(n => n.open), true, "node expands");
    await first.locator("summary").click();
    assert.equal(await first.evaluate(n => n.open), false, "node collapses");
    const inspector = page.locator(".run-flow-event-inspector");
    await page.locator('#task-events [data-task-event-sequence="2"]').click();
    assert.equal(await page.locator('#task-events .run-flow-node[data-event-sequence="2"]').evaluate(n => n.open), true);
    assert.equal(await inspector.evaluate(n => n.open), true, "selecting a stage opens its inspector");
    assert.equal(await page.locator("#task-event-detail").isVisible(), true, "selected stage details sync");
    assert.equal(await inspector.evaluate(n => {
      const parent = n.closest(".run-flow-result-column"), a = n.getBoundingClientRect(), b = parent.getBoundingClientRect();
      return a.top < b.bottom && a.bottom > b.top;
    }), true, "selected stage inspector scrolls into the visible result area");
    await page.locator(".run-flow-event-inspector > summary").click();
    assert.equal(await inspector.evaluate(n => n.open), false, "inspector can be collapsed manually");
    await page.locator('[data-task-event-filter="all"]').click();
    assert.equal(await inspector.evaluate(n => n.open), false, "poll/list rerender preserves manual collapse for the same stage");
    // Stage 投影不伪造事件详情：标题只有阶段占位文案与序号，JSON 中保留真实 stage_code/status。
    assert.match(await page.locator("#task-event-detail-title").textContent(), /阶段事件 · #2/, "selected stage title uses projected stage sequence");
    assert.match(await page.locator("#task-event-json").textContent(), /"stage_code": "quant.factor"/, "inspector exposes the selected stage_code");
    assert.match(await page.locator("#task-event-json").textContent(), /"status": "succeeded"/, "inspector exposes the projected stage status");
    await page.locator("#close-run-flow-dialog").click();
    await page.locator("#run-flow-dialog:not([open])").waitFor({ state: "hidden" });
    await page.locator('[data-open-task-id="run-flow-b"]').click();
    await page.locator("#run-flow-dialog[open]").waitFor();
    await page.waitForFunction(() => document.querySelector("#task-id-value")?.textContent === "run-flow-b");
    assert.equal(await page.locator("#task-result-section").isVisible(), false, "old result cleared");
    assert.equal(await page.locator("#run-flow-result-empty").isVisible(), true, "placeholder restored");
    await page.locator("#task-events .run-flow-node").nth(3).waitFor();
    assert.equal(await page.locator("#task-events .run-flow-node").count(), 4, "queued Run still projects its four stages");
    assert.match(await page.locator("#task-status-pill").textContent(), /排队中/, "queued Run status is mapped to the page task status");
    assert.deepEqual(await page.locator("#task-events .event-state-label").allTextContents(), ["等待中", "等待中", "等待中", "等待中"], "stale succeeded stages are replaced by fresh pending stages");
    // 切换任务后详情必须来自新 Run，不能残留上一个 Run 的 Stage 数据。
    await page.locator('#task-events [data-task-event-sequence="4"]').click();
    const switchedDetail = await page.locator("#task-event-json").textContent();
    assert.match(switchedDetail, /"stage_ref": "stage-run-flow-b-quant.risk"/, "stage detail belongs to the newly opened Run");
    assert.doesNotMatch(switchedDetail, /run-flow-a/, "no stale stage detail from the previous Run");
    assert.equal(resultReadCounts.get("run-flow-b"), undefined, "results are not read before the Run reaches a terminal status");
    runBReady = true;
    await page.locator("#refresh-task").click();
    await page.waitForFunction(() => document.querySelector("#task-status-pill")?.textContent.includes("执行成功"), null, { timeout: 10000 });
    await page.waitForFunction(() => !document.querySelector("#task-result-section")?.hidden, null, { timeout: 10000 });
    assert.equal(await page.locator("#run-flow-result-empty").isVisible(), false, "placeholder replaced after result response");
    assert.equal(resultReadCounts.get("run-flow-b"), 1, "results are read after manual refresh reaches terminal status");
    assert.equal(await page.locator(".analysis-result-card").count(), 4, "refreshed Run renders the degraded result cards");
    assert.deepEqual(pageErrors, [], "browser errors at " + viewport.width + "px: " + pageErrors.join(" | "));
    await page.screenshot({ path: path.join(screenshotDir, viewport.width + "x" + viewport.height + "-run-flow-interaction.png"), fullPage: false, animations: "disabled" });
    console.log("PASS Run Flow interactions at " + viewport.width + "px");
    await context.close();
  }
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

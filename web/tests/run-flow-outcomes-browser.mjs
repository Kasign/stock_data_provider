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

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const screenshotDir = path.resolve(root, "../browser-screenshots/run-flow-outcomes");
await mkdir(screenshotDir, { recursive: true });

// run_ref 直接作为历史记录 task_id；所有标的带交易所后缀，baseRef 为 Root Server 单入口。
const tasks = [
  { task_id: "run-partial", run_id: "run-partial", symbols: ["600519.SH"], task_type: "post_market_scan", status: "partial", baseRef: "/" },
  { task_id: "run-failed", run_id: "run-failed", symbols: ["000001.SZ"], task_type: "post_market_scan", status: "failed", failure_code: "market_snapshot_failed", error_message: "读取行情快照失败", baseRef: "/" },
  { task_id: "run-unavailable", run_id: "run-unavailable", symbols: ["600519.SH"], task_type: "post_market_scan", status: "succeeded", baseRef: "/" },
  { task_id: "run-race-a", run_id: "run-race-a", symbols: ["600519.SH"], task_type: "post_market_scan", status: "succeeded", baseRef: "/" },
  { task_id: "run-race-b", run_id: "run-race-b", symbols: ["000001.SZ"], task_type: "post_market_scan", status: "running", baseRef: "/" }
];
const server = createServer(async (req, res) => {
  const p = decodeURIComponent(new URL(req.url || "/", "http://127.0.0.1").pathname);
  const file = path.resolve(root, "." + (p === "/" ? "/index.html" : p));
  if (file !== root && !file.startsWith(root + path.sep)) {
    res.writeHead(403);
    res.end();
    return;
  }
  try {
    const body = await readFile(file), ext = path.extname(file);
    res.writeHead(200, { "content-type": ext === ".html" ? "text/html; charset=utf-8" : ext === ".css" ? "text/css; charset=utf-8" : "text/javascript; charset=utf-8", "cache-control": "no-store" });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end();
  }
});
await new Promise((ok, fail) => { server.once("error", fail); server.listen(0, "127.0.0.1", ok); });
const addr = server.address();
assert.ok(addr && typeof addr === "object");

let browser, releaseA, startA;
const startedA = new Promise(resolve => startA = resolve);
let aggregateAttempts = 0;
const resultReads = [];
try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
  await context.addInitScript(seed => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({ marketBase: "/", analysisBase: "/" }));
    localStorage.setItem("stock-provider-webui.recent-runs.v1", JSON.stringify(seed));
  }, tasks);
  const page = await context.newPage(), pageErrors = [];
  page.on("pageerror", e => pageErrors.push(e.message));
  await page.route("**/*", async route => {
    const p = new URL(route.request().url()).pathname;
    const json = (status, body) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
    const aggregateMatch = p.match(/^\/api\/v1\/researchflow\/runs\/(run-[a-z-]+)$/);
    if (aggregateMatch) {
      const runRef = aggregateMatch[1];
      if (runRef === "run-race-a") {
        // 保留旧用例的慢响应竞态：A 的聚合请求在切换到 B 前不返回。
        startA();
        await new Promise(resolve => releaseA = resolve);
      }
      if (runRef === "run-unavailable" && aggregateAttempts++ === 0) return json(503, { error: "Run 聚合暂不可用" });
      if (runRef === "run-partial") {
        return json(200, makeRunAggregate({
          runRef,
          runStatus: "partial",
          stageStatuses: { "quant.factor": "degraded" },
          reasonCodes: { "quant.factor": ["factor_quality_degraded"] }
        }));
      }
      if (runRef === "run-failed") {
        return json(200, makeRunAggregate({
          runRef,
          runStatus: "failed",
          stageStatuses: { "quant.sma20": "failed" },
          reasonCodes: { "quant.sma20": ["market_snapshot_failed"] }
        }));
      }
      if (runRef === "run-race-b") return json(200, makeRunAggregate({ runRef, runStatus: "running" }));
      return json(200, makeRunAggregate({ runRef }));
    }
    const resultsMatch = p.match(/^\/api\/v1\/researchflow\/runs\/(run-[a-z-]+)\/results$/);
    if (resultsMatch) {
      const runRef = resultsMatch[1];
      resultReads.push(runRef);
      if (runRef === "run-failed") return json(200, makeResultsResponse({ runRef, resultTypes: [] }));
      if (runRef === "run-partial") return json(200, makeResultsResponse({ runRef, qualityStatus: "partial", resultStatus: "partial" }));
      return json(200, makeResultsResponse({ runRef }));
    }
    if (p.includes("/api/") || p.endsWith("/readyz")) return json(503, { error: "fixture endpoint unavailable" });
    return route.continue();
  });
  await page.goto("http://127.0.0.1:" + addr.port, { waitUntil: "load" });
  await page.locator('[data-view="tasks"]').click();
  // 1. partial：Stage 降级 + 授权领域结果降级渲染。
  await page.locator('[data-open-task-id="run-partial"]').click();
  await page.locator("#run-flow-dialog[open]").waitFor();
  await page.waitForFunction(() => document.querySelector("#task-run-status-pill")?.dataset.runStatus === "partial");
  assert.match(await page.locator("#task-status-pill").textContent(), /部分完成/);
  assert.match(await page.locator("#task-run-status-pill").textContent(), /部分结果/);
  assert.equal(await page.locator("#task-events .run-flow-node").count(), 4);
  assert.equal(await page.locator("#task-events .event-row-degraded").count(), 1, "degraded stage keeps its status in the aggregate projection");
  await page.locator('#task-events .event-row-degraded button[data-task-event-sequence]').click();
  assert.match(await page.locator("#task-event-json").textContent(), /"stage_code": "quant.factor"/);
  assert.match(await page.locator("#task-event-json").textContent(), /factor_quality_degraded/);
  assert.match(await page.locator("#task-result-note").textContent(), /非完整研究报告/);
  assert.ok(resultReads.includes("run-partial"));
  await page.locator("#close-run-flow-dialog").click();
  await page.locator("#run-flow-dialog:not([open])").waitFor({ state: "hidden" });
  // 2. failed：失败状态由 Stage reason_codes 驱动，不伪造事件详情。
  await page.locator('[data-open-task-id="run-failed"]').click();
  await page.locator("#run-flow-dialog[open]").waitFor();
  await page.waitForFunction(() => document.querySelector("#task-run-status-pill")?.dataset.runStatus === "failed");
  assert.match(await page.locator("#task-status-pill").textContent(), /执行失败/);
  assert.equal(await page.locator("#task-error").getAttribute("role"), "alert");
  assert.equal(await page.locator("#task-error-title").textContent(), "任务执行失败");
  assert.equal(await page.locator("#task-error-message").isVisible(), false, "failure detail is not fabricated from Stage reason codes");
  assert.match(await page.locator("#task-error-code").textContent(), /market_snapshot_failed/);
  assert.match(await page.locator("#task-run-status-pill").textContent(), /分析失败/);
  assert.ok(resultReads.includes("run-failed"));
  await page.locator("#close-run-flow-dialog").click();
  await page.locator("#run-flow-dialog:not([open])").waitFor({ state: "hidden" });
  // 3. Run 聚合暂不可用：无结果读取，刷新后恢复并读取降级结果。
  await page.locator('[data-open-task-id="run-unavailable"]').click();
  await page.locator("#run-flow-dialog[open]").waitFor();
  await page.waitForFunction(() => document.querySelector("#task-poll-label")?.textContent.includes("Run 状态刷新失败"));
  assert.equal(resultReads.includes("run-unavailable"), false, "no results read while Run aggregate is unavailable");
  assert.match(await page.locator("#task-status-pill").textContent(), /状态刷新失败/);
  await page.locator("#refresh-task").click();
  await page.waitForFunction(() => document.querySelector("#task-run-status-pill")?.dataset.runStatus === "succeeded", null, { timeout: 10000 });
  assert.equal(aggregateAttempts, 2, "aggregate endpoint retried exactly once on refresh");
  assert.ok(resultReads.includes("run-unavailable"));
  assert.equal(await page.locator(".analysis-result-card").count(), 4);
  await page.locator("#close-run-flow-dialog").click();
  await page.locator("#run-flow-dialog:not([open])").waitFor({ state: "hidden" });
  // 4. 慢响应竞态：切到 B 后，A 的迟到聚合响应不得覆盖当前 Run 或触发结果读取。
  await page.locator('[data-open-task-id="run-race-a"]').click();
  await page.locator("#run-flow-dialog[open]").waitFor();
  await startedA;
  await page.locator("#close-run-flow-dialog").click();
  await page.locator("#run-flow-dialog:not([open])").waitFor({ state: "hidden" });
  await page.locator('[data-open-task-id="run-race-b"]').click();
  await page.locator("#run-flow-dialog[open]").waitFor();
  await page.waitForFunction(() => document.querySelector("#task-id-value")?.textContent === "run-race-b" && document.querySelector("#task-status-pill")?.textContent.includes("执行中"));
  assert.equal(await page.locator("#task-run-status-pill").getAttribute("data-run-status"), "unknown");
  releaseA();
  await page.waitForTimeout(150);
  assert.equal(await page.locator("#task-id-value").textContent(), "run-race-b");
  assert.match(await page.locator("#task-status-pill").textContent(), /执行中/);
  assert.equal(resultReads.includes("run-race-a"), false, "stale Run response must not read results");
  assert.deepEqual(pageErrors, []);
  await page.screenshot({ path: path.join(screenshotDir, "1280x900.png"), fullPage: false, animations: "disabled" });
  console.log("PASS Run Flow partial, failed, aggregate recovery and stale-response isolation");
  await context.close();
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

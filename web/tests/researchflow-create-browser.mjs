// 浏览器验收：通过表单创建任务的完整 Researchflow 主流程。
// 覆盖"能用"的核心链路：填表提交 → POST /researchflow/runs → Run/Stage 聚合轮询 →
// 降级领域结果渲染（非完整报告）。验证浏览器全程不请求旧 /analysis 端点。
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const screenshotDir = path.resolve(webRoot, "..", "browser-screenshots");
const viewports = [{ width: 1280, height: 900 }];

const stages = [
  { stage_code: "quant.sma20", status: "succeeded" },
  { stage_code: "quant.factor", status: "succeeded" },
  { stage_code: "quant.trend", status: "succeeded" },
  { stage_code: "quant.risk", status: "degraded" }
].map((stage, index) => ({
  schema_version: "1.1.0",
  stage_ref: "stage-" + (index + 1),
  run_ref: "run-created-1",
  stage_code: stage.stage_code,
  status: stage.status,
  version: 1,
  required: true,
  dependency_stage_refs: [],
  retry_policy_ref: "",
  reason_codes: stage.status === "degraded" ? ["RISK_FACTS_STALE"] : [],
  output_ref: "out-" + (index + 1),
  created_at: "2026-11-28T02:00:00Z"
}));

const resultsResponse = {
  schema_version: "1.1.0",
  run_ref: "run-created-1",
  resource_kind: "authorized_domain_results",
  results: [
    {
      result_id: "res-ind", result_type: "indicator", scope: { symbol: "600519.SH" },
      system_as_of: "2026-11-27", quality_status: "valid", result_status: "succeeded",
      authorization_ref: "auth-ind", payload_schema_version: "1.0.0",
      payload: { indicator_code: "sma20", value: 1680.5 }
    },
    {
      result_id: "res-factor", result_type: "factor", scope: { symbol: "600519.SH" },
      system_as_of: "2026-11-27", quality_status: "valid", result_status: "succeeded",
      authorization_ref: "auth-factor", payload_schema_version: "1.0.0",
      payload: { factor_code: "momentum_5d", value: 0.031 }
    },
    {
      result_id: "res-trend", result_type: "trend", scope: { symbol: "600519.SH" },
      system_as_of: "2026-11-27", quality_status: "valid", result_status: "succeeded",
      authorization_ref: "auth-trend", payload_schema_version: "1.0.0",
      payload: { direction: "up", strength: 0.62 }
    },
    {
      result_id: "res-risk", result_type: "risk", scope: { symbol: "600519.SH" },
      system_as_of: "2026-11-27", quality_status: "restricted", result_status: "degraded",
      authorization_ref: "auth-risk", payload_schema_version: "1.0.0"
    }
  ]
};

const server = createServer(async (req, res) => {
  const pathname = decodeURIComponent(new URL(req.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) {
    res.writeHead(403); res.end("Forbidden"); return;
  }
  try {
    const body = await readFile(target);
    const ext = path.extname(target);
    const type = ext === ".html" ? "text/html; charset=utf-8" : ext === ".css" ? "text/css; charset=utf-8" : [".js", ".mjs"].includes(ext) ? "text/javascript; charset=utf-8" : "application/octet-stream";
    res.writeHead(200, { "content-type": type, "cache-control": "no-store" });
    res.end(body);
  } catch {
    res.writeHead(404); res.end("Not found");
  }
});

await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;

try {
  browser = await chromium.launch({ headless: true });
  for (const viewport of viewports) {
    const context = await browser.newContext({ viewport, reducedMotion: "reduce" });
    await context.addInitScript(() => {
      localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
        marketBase: "/", analysisBase: "/", apiKey: ""
      }));
    });
    const page = await context.newPage();
    const pageErrors = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));

    let createRequests = 0;
    let runPolls = 0;
    let resultReads = 0;
    let csrfHeaderSeen = false;
    let legacyCalls = 0;
    let lastCreateBody = null;

    await page.route("**/*", async (route) => {
      const url = new URL(route.request().url());
      const pathname = url.pathname;
      const method = route.request().method();
      const json = (status, body) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });

      if (pathname.includes("/api/v1/analysis/")) {
        legacyCalls += 1;
        return json(410, { error: "legacy analysis endpoint is retired" });
      }
      if (pathname === "/api/v1/owner/setup") return json(200, { mode: "login" });
      if (pathname === "/api/v1/owner/session" && method === "GET") return json(200, { authenticated: true, expires_at: "2026-11-28T12:00:00Z" });
      if (pathname === "/api/v1/owner/session/csrf" && method === "POST") return json(200, { authenticated: true, csrf_token: "browser-create-csrf" });

      if (pathname === "/api/v1/researchflow/runs" && method === "POST") {
        createRequests += 1;
        csrfHeaderSeen = route.request().headers()["x-csrf-token"] === "browser-create-csrf";
        try { lastCreateBody = JSON.parse(route.request().postData() || "{}"); } catch {}
        return json(200, {
          schema_version: "1.1.0",
          run: {
            run_ref: "run-created-1", status: "created", version: 1,
            system_as_of: "2026-11-28T02:00:00Z", deadline_at: "2026-11-28T03:00:00Z",
            template_ref: "quant.research", template_version: "1.0.0",
            template_fingerprint: "fp", input_snapshot_refs: [], correlation_id: "corr",
            created_at: "2026-11-28T02:00:00Z"
          },
          stages
        });
      }
      const runMatch = pathname.match(/\/api\/v1\/researchflow\/runs\/(run-created-1)(?:\/(results))?$/);
      if (runMatch) {
        if (runMatch[2] === "results") {
          resultReads += 1;
          return json(200, resultsResponse);
        }
        runPolls += 1;
        return json(200, {
          schema_version: "1.1.0",
          run: {
            run_ref: "run-created-1", status: "succeeded", version: 3,
            system_as_of: "2026-11-28T02:00:00Z", deadline_at: "2026-11-28T03:00:00Z",
            template_ref: "quant.research", template_version: "1.0.0",
            template_fingerprint: "fp", input_snapshot_refs: [], correlation_id: "corr",
            created_at: "2026-11-28T02:00:00Z", finished_at: "2026-11-28T02:01:00Z"
          },
          stages
        });
      }
      if (pathname.includes("/api/") || /\/(?:health\/ready|readyz)$/.test(pathname)) {
        return json(503, { error: "researchflow-create fixture: endpoint not included" });
      }
      return route.continue();
    });

    await page.goto(baseUrl, { waitUntil: "load", timeout: 30000 });
    await page.locator('[data-view="tasks"]').click();
    await page.locator("#task-symbols").fill("600519");
    await page.locator("#idempotency-key").fill("browser-create-key");
    await page.locator("#create-task-button").click();

    // 创建走新 API；终态后读取降级结果。
    await page.waitForFunction(() => document.querySelector("#task-result-section") && !document.querySelector("#task-result-section").hidden, null, { timeout: 15000 });
    await page.waitForFunction(() => (document.querySelector("#task-result-structured")?.children.length || 0) > 0, null, { timeout: 10000 });

    assert.equal(createRequests, 1, "表单创建必须恰好调用一次 Researchflow 创建端点");
    assert.equal(csrfHeaderSeen, true, "创建请求必须携带 Owner Session CSRF Token");
    assert.ok(runPolls >= 1, "必须轮询 Run/Stage 聚合状态");
    assert.equal(resultReads, 1, "终态后读取一次授权领域结果");
    assert.equal(legacyCalls, 0, "浏览器全程不得请求旧 Analysis 端点");

    // 创建命令字段符合 create-run-request 1.1.0。
    assert.equal(lastCreateBody?.schema_version, "1.1.0");
    assert.equal(lastCreateBody?.template_ref, "quant.research");
    assert.equal(lastCreateBody?.template_version, "1.0.0");
    assert.equal(lastCreateBody?.idempotency_key, "browser-create-key");
    assert.deepEqual(lastCreateBody?.input_snapshot_refs, []);
    assert.equal(lastCreateBody?.parameters?.stages?.["quant.risk"]?.symbol, "600519.SH", "六位代码须补交易所后缀");
    assert.equal(lastCreateBody?.parameters?.stages?.["quant.sma20"]?.symbol, "600519.SH");

    // 结果区明确非完整报告，四类结果卡，risk 无 payload。
    const note = await page.locator("#task-result-note").textContent();
    assert.match(note, /非完整研究报告/);
    const cards = await page.locator(".analysis-result-card-heading strong").allTextContents();
    assert.deepEqual(cards, ["指标结果", "因子结果", "趋势结果", "风险结果"]);
    const riskText = await page.locator(".analysis-result-card").last().textContent();
    assert.match(riskText, /仅状态/);
    assert.doesNotMatch(riskText, /indicator_code|factor_code/, "risk 卡不得渲染 payload");

    // Stage 聚合投影为 4 个有序事件项，含 degraded 警示。
    assert.equal(await page.locator("#task-events .run-flow-node").count(), 4);
    assert.equal(await page.locator("#task-events .event-row-degraded").count(), 1);

    assert.deepEqual(pageErrors, [], "页面不应有未捕获异常: " + pageErrors.join(" | "));
    await page.screenshot({ path: path.join(screenshotDir, viewport.width + "x" + viewport.height + "-researchflow-create.png"), fullPage: false, animations: "disabled" });
    console.log("PASS Researchflow create flow at " + viewport.width + "px");
    await context.close();
  }
} finally {
  if (browser) await browser.close();
  await new Promise((resolve) => server.close(resolve));
}

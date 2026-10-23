import test from "node:test";
import assert from "node:assert/strict";
import {
  analysisHealth,
  createAnalysisTask,
  getAnalysisRun,
  getAnalysisTaskEvents,
  getDailyBars,
  getDataSources,
  getQuote,
  marketHealth
} from "../src/api.js";

function jsonResponse(body, status = 200, headers = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers }
  });
}

test("creates a Go analysis task with the current payload and optional idempotency key", async () => {
  const originalFetch = globalThis.fetch;
  let request;
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), ...init };
    return jsonResponse({ task: { task_id: "task-1", status: "queued" } }, 202);
  };
  try {
    const payload = { task_type: "post_market_scan", symbols: ["600519"] };
    const result = await createAnalysisTask({ analysisBase: "/analysis-api" }, payload, "webui-test-1");
    assert.equal(result.task.task_id, "task-1");
    assert.equal(request.url, "/analysis-api/api/v1/analysis/tasks");
    assert.equal(request.method, "POST");
    assert.equal(request.headers["Content-Type"], "application/json");
    assert.equal(request.headers["Idempotency-Key"], "webui-test-1");
    assert.deepEqual(JSON.parse(request.body), payload);
    assert.equal(request.credentials, "same-origin");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("reads task events incrementally and only sends the market API key to market endpoints", async () => {
  const originalFetch = globalThis.fetch;
  const requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), ...init });
    return jsonResponse({ events: [], run_id: "run-1" });
  };
  try {
    const settings = { marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "market-secret" };
    await getAnalysisTaskEvents(settings, "task/with space", 17);
    await getQuote(settings, "600519.SH");
    await getAnalysisRun(settings, "run-1");

    assert.equal(requests[0].url, "/analysis-api/api/v1/analysis/tasks/task%2Fwith%20space/events?after_sequence=17&limit=100");
    assert.equal(requests[1].url, "/market-api/api/v1/quotes/600519.SH");
    assert.equal(requests[1].headers["X-API-Key"], "market-secret");
    assert.equal(requests[2].url, "/analysis-api/api/v1/analysis/runs/run-1");
    assert.equal(Object.hasOwn(requests[0].headers, "X-API-Key"), false);
    assert.equal(Object.hasOwn(requests[2].headers, "X-API-Key"), false);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("preserves structured server error code, HTTP status and request ID", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => jsonResponse({
    error: { code: "run_not_finished", message: "run result is not ready" }
  }, 409, { "X-Request-ID": "req-webui-123" });
  try {
    await assert.rejects(
      getAnalysisRun({ analysisBase: "/analysis-api" }, "run-pending"),
      (error) => {
        assert.equal(error.message, "run result is not ready");
        assert.equal(error.status, 409);
        assert.equal(error.code, "run_not_finished");
        assert.equal(error.requestId, "req-webui-123");
        return true;
      }
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});


test("uses the current health, provider and daily-bars routes with market-only API-key attachment", async () => {
  const originalFetch = globalThis.fetch;
  const requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), ...init });
    return jsonResponse({ status: "ok", items: [] });
  };
  try {
    const settings = {
      marketBase: "/market-api",
      analysisBase: "/analysis-api",
      apiKey: "market-secret"
    };
    await marketHealth(settings);
    await analysisHealth(settings);
    await getDataSources(settings);
    await getDailyBars(settings, "600519.SH", { limit: 120, adjustment: "forward_adjusted" });

    assert.equal(requests[0].url, "/market-api/health/ready");
    assert.equal(requests[1].url, "/analysis-api/readyz");
    assert.equal(requests[2].url, "/market-api/api/v1/data-sources");
    assert.equal(requests[3].url, "/market-api/api/v1/bars/600519.SH/daily?limit=120&adjustment=forward_adjusted");
    assert.equal(Object.hasOwn(requests[0].headers, "X-API-Key"), false);
    assert.equal(Object.hasOwn(requests[1].headers, "X-API-Key"), false);
    assert.equal(requests[2].headers["X-API-Key"], "market-secret");
    assert.equal(requests[3].headers["X-API-Key"], "market-secret");

    await getDailyBars(settings, "600519.SH", {
      limit: 60, adjustment: "none", start: "2026-10-01", end: "2026-10-23"
    });
    assert.equal(
      requests[4].url,
      "/market-api/api/v1/bars/600519.SH/daily?limit=60&adjustment=none&start=2026-10-01&end=2026-10-23"
    );
    assert.equal(requests[4].headers["X-API-Key"], "market-secret");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("preserves bounded diagnostics for non-JSON reverse-proxy errors", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response("upstream unavailable ".repeat(50), {
    status: 502,
    statusText: "Bad Gateway",
    headers: { "Content-Type": "text/plain", "X-Request-ID": "req-proxy-502" }
  });
  try {
    await assert.rejects(
      getQuote({ marketBase: "/market-api", apiKey: "market-secret" }, "600519.SH"),
      (error) => {
        assert.equal(error.status, 502);
        assert.equal(error.code, "http_error");
        assert.equal(error.requestId, "req-proxy-502");
        assert.ok(error.message.length <= 500, "proxy response text should be bounded");
        assert.match(error.message, /upstream unavailable/);
        return true;
      }
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("prefers request IDs from the structured error envelope when both forms exist", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => jsonResponse({
    error: {
      code: "task_not_found",
      message: "task does not exist",
      request_id: "req-from-body"
    }
  }, 404, { "X-Request-ID": "req-from-header" });
  try {
    await assert.rejects(
      getAnalysisRun({ analysisBase: "/analysis-api" }, "missing-run"),
      (error) => {
        assert.equal(error.status, 404);
        assert.equal(error.code, "task_not_found");
        assert.equal(error.requestId, "req-from-body");
        return true;
      }
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

import test from "node:test";
import assert from "node:assert/strict";
import {
  analysisHealth,
  createAnalysisTask,
  getAnalysisRun,
  getAnalysisTaskEvents,
  getDailyBars,
  getChipDistribution,
  getCapitalFlow,
  getIndexCatalog,
  getIndexConstituents,
  getIndexDailyBars,
  getMarketSession,
  getStockPriceLimitRule,
  getStockTradingStatus,
  getDataSources,
  getQuote,
  searchStocks,
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

test("bounds oversized JSON error messages and keeps plain JSON string diagnostics", async () => {
  const originalFetch = globalThis.fetch;
  const cases = [
    { body: { error: { code: "upstream_error", message: "x".repeat(800) } }, expectedLength: 500 },
    { body: "upstream says unavailable", expectedMessage: "upstream says unavailable" }
  ];
  try {
    for (const item of cases) {
      globalThis.fetch = async () => new Response(JSON.stringify(item.body), {
        status: 502,
        statusText: "Bad Gateway",
        headers: { "Content-Type": "application/json" }
      });
      await assert.rejects(
        getAnalysisRun({ analysisBase: "/analysis-api" }, "run-error"),
        (error) => {
          assert.equal(error.status, 502);
          if (item.expectedLength) assert.equal(error.message.length, item.expectedLength);
          if (item.expectedMessage) assert.equal(error.message, item.expectedMessage);
          return true;
        }
      );
    }
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


test("reads existing chip-distribution and capital-flow endpoints with the market API key", async () => {
  const originalFetch = globalThis.fetch;
  const requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), ...init });
    if (String(url).includes("chip-distribution")) {
      return jsonResponse({ symbol: "600519.SH", average_cost: 1234.5, profit_ratio: 0.62, source: "fixture", quality_status: "degraded" });
    }
    return jsonResponse({ symbol: "600519.SH", main_net_inflow_yuan: 1500000, inflow_5d_yuan: null, observations: 1, source: "fixture", quality_status: "valid" });
  };
  try {
    const settings = { marketBase: "/market-api", apiKey: "market-secret" };
    const [chip, flow] = await Promise.all([
      getChipDistribution(settings, "600519.SH"),
      getCapitalFlow(settings, "600519.SH")
    ]);
    assert.equal(chip.average_cost, 1234.5);
    assert.equal(flow.main_net_inflow_yuan, 1500000);
    assert.deepEqual(requests.map((item) => item.url).sort(), [
      "/market-api/api/v1/stocks/600519.SH/capital-flow",
      "/market-api/api/v1/stocks/600519.SH/chip-distribution"
    ]);
    assert.ok(requests.every((item) => item.headers["X-API-Key"] === "market-secret"));
    assert.ok(requests.every((item) => item.credentials === "same-origin"));
  } finally {
    globalThis.fetch = originalFetch;
  }
});


test("searches stocks by Chinese name through the authenticated market API and forwards cancellation", async () => {
  const originalFetch = globalThis.fetch;
  let request;
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), ...init };
    return jsonResponse({
      query: "贵州茅台",
      total: 1,
      items: [{ symbol: "600519.SH", code: "600519", name: "贵州茅台", exchange: "SH" }]
    });
  };
  const controller = new AbortController();
  try {
    const result = await searchStocks(
      { marketBase: "/market-api", apiKey: "market-secret" },
      " 贵州茅台 ",
      { limit: 10, signal: controller.signal }
    );
    assert.equal(request.url, "/market-api/api/v1/stocks/search?q=%E8%B4%B5%E5%B7%9E%E8%8C%85%E5%8F%B0&limit=10");
    assert.equal(request.headers["X-API-Key"], "market-secret");
    assert.equal(request.signal, controller.signal);
    assert.equal(result.items[0].symbol, "600519.SH");
  } finally {
    globalThis.fetch = originalFetch;
  }
});


test("preserves the full 401/403/404/503 HTTP error matrix without converting failures to data", async () => {
  const originalFetch = globalThis.fetch;
  const cases = [
    { status: 401, code: "auth_required", message: "credentials required" },
    { status: 403, code: "access_denied", message: "permission denied" },
    { status: 404, code: "quote_not_found", message: "quote does not exist" },
    { status: 503, code: "provider_unavailable", message: "upstream provider unavailable" }
  ];
  try {
    for (const item of cases) {
      globalThis.fetch = async () => jsonResponse({
        error: { code: item.code, message: item.message, request_id: "req-" + item.status }
      }, item.status);

      await assert.rejects(
        getQuote({ marketBase: "/market-api", apiKey: "market-secret" }, "600519.SH"),
        (error) => {
          assert.equal(error.status, item.status);
          assert.equal(error.code, item.code);
          assert.equal(error.message, item.message);
          assert.equal(error.requestId, "req-" + item.status);
          return true;
        },
        "HTTP " + item.status + " must reject rather than return an empty/success payload"
      );
    }
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("propagates ordinary fetch network failures so the UI can classify them for retry", async () => {
  const originalFetch = globalThis.fetch;
  const failure = new TypeError("Failed to fetch");
  globalThis.fetch = async () => { throw failure; };
  try {
    await assert.rejects(
      getQuote({ marketBase: "/market-api", apiKey: "market-secret" }, "600519.SH"),
      (error) => error === failure
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});


test("newly integrated market endpoints encode symbols, preserve explicit source and date filters", async () => {
  const originalFetch = globalThis.fetch;
  const requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), ...init });
    return jsonResponse({ quality_status: "valid", items: [] });
  };
  try {
    const settings = { marketBase: "/market-api", apiKey: "market-secret" };
    await getMarketSession(settings);
    await getIndexCatalog(settings, { tag: "industry" });
    await getIndexConstituents(settings, "IDX/000300", { source: "tushare", asOf: "2026-10-28" });
    await getIndexDailyBars(settings, "IDX/000300", { source: "hithink", start: "2026-10-01", end: "2026-10-28" });
    await getStockPriceLimitRule(settings, "600519.SH", "2026-10-28");
    await getStockTradingStatus(settings, "600519.SH", "2026-10-28", { asOf: "2026-10-29T09:00:00+08:00" });

    assert.equal(requests[0].url, "/market-api/api/v1/market/session");
    assert.equal(requests[1].url, "/market-api/api/v1/market/index-catalog?tag=industry");
    assert.equal(requests[2].url, "/market-api/api/v1/market/indices/IDX%2F000300/constituents?source=tushare&as_of=2026-10-28");
    assert.equal(requests[3].url, "/market-api/api/v1/market/indices/IDX%2F000300/daily?start=2026-10-01&end=2026-10-28&source=hithink");
    assert.equal(requests[4].url, "/market-api/api/v1/stocks/600519.SH/price-limit-rule?date=2026-10-28");
    assert.equal(requests[5].url, "/market-api/api/v1/stocks/600519.SH/trading-status?date=2026-10-28&as_of=2026-10-29T09%3A00%3A00%2B08%3A00");
    assert.ok(requests.every(request => request.headers["X-API-Key"] === "market-secret"));
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("rejects malformed or reversed dates before issuing market requests", async () => {
  const originalFetch = globalThis.fetch;
  let count = 0;
  globalThis.fetch = async () => { count += 1; return jsonResponse({}); };
  try {
    const settings = { marketBase: "/market-api", apiKey: "market-secret" };
    await assert.rejects(getIndexDailyBars(settings, "000300.SH", { start: "2026-02-30", end: "2026-10-01" }), /有效的开始和结束日期/);
    await assert.rejects(getIndexDailyBars(settings, "000300.SH", { start: "2026-10-02", end: "2026-10-01" }), /开始日期不能晚于结束日期/);
    await assert.rejects(getStockPriceLimitRule(settings, "600519.SH", "2026-1-01"), /有效的交易日期/);
    assert.equal(count, 0);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

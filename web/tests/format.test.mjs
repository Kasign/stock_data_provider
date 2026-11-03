import test from "node:test";
import assert from "node:assert/strict";
import {
  comparableMarketSymbol,
  describeApiError,
  finiteNumberOrNull,
  formatCompact,
  formatPercent,
  formatPrice,
  isRetryableApiError,
  normalizeAnalysisSymbol,
  normalizeBasePath,
  normalizeSymbol,
  parseAnalysisSymbols,
  parseSymbols,
  qualityLabel,
  runStatusLabel,
  taskStatusLabel
} from "../src/format.js";

test("normalizes same-origin base paths and rejects remote origins", () => {
  assert.equal(normalizeBasePath(" /market-api/// ", "/fallback"), "/market-api");
  assert.equal(normalizeBasePath("", "/fallback"), "/fallback");
  assert.throws(() => normalizeBasePath("https://example.com", "/fallback"), /同源/);
  assert.throws(() => normalizeBasePath("//evil.example", "/fallback"), /同源/);
});

test("accepts plain six digit symbols and exchange-qualified symbols", () => {
  assert.equal(normalizeSymbol(" 600519 "), "600519");
  assert.equal(normalizeSymbol("000001.sz"), "000001.SZ");
  assert.throws(() => normalizeSymbol("60051"), /格式不正确/);
});

test("shares A-share identity mapping across plain and exchange-suffixed symbols", () => {
  assert.equal(comparableMarketSymbol("600519"), "600519.SH");
  assert.equal(comparableMarketSymbol("600519.SH"), "600519.SH");
  assert.equal(comparableMarketSymbol("000001"), "000001.SZ");
  assert.equal(comparableMarketSymbol("430047"), "430047.BJ");
  assert.equal(comparableMarketSymbol("920001"), "920001.BJ");
  assert.equal(comparableMarketSymbol("123456"), "123456", "unknown exchange must not be guessed");
  assert.throws(() => comparableMarketSymbol("not-a-stock"), /格式不正确/);
});

test("parses lists, removes duplicates and caps the payload size", () => {
  assert.deepEqual(parseSymbols("600519, 000001，600519"), ["600519", "000001"]);
  assert.throws(() => parseSymbols(Array.from({ length: 101 }, (_, index) => String(index + 100000).slice(-6)).join(",")), /最多分析 100/);
});

test("formats optional numeric values without converting missing data into zero", () => {
  assert.equal(formatPrice(null), "—");
  assert.equal(formatPercent(undefined), "—");
  assert.equal(formatPercent(1.25), "+1.25%");
  assert.equal(formatPercent(-0.5), "-0.50%");
});

test("maps quality and task statuses to Chinese labels", () => {
  assert.equal(qualityLabel("degraded"), "质量降级");
  assert.equal(taskStatusLabel("queued"), "排队中");
});


test("normalizes symbols for the current Go analysis contract without changing quote symbols", () => {
  assert.equal(normalizeAnalysisSymbol("600519.SH"), "600519");
  assert.equal(normalizeAnalysisSymbol("000001.sz"), "000001");
  assert.deepEqual(parseAnalysisSymbols("600519.SH, 600519, 000001.SZ"), ["600519", "000001"]);
  assert.equal(normalizeSymbol("600519.SH"), "600519.SH");
  assert.throws(() => normalizeAnalysisSymbol("ABCDEF"), /格式不正确/);
});

test("maps Go RunResult statuses separately from asynchronous task statuses", () => {
  assert.equal(runStatusLabel("completed"), "分析完成");
  assert.equal(runStatusLabel("blocked_data_quality"), "数据质量阻断");
  assert.equal(runStatusLabel("rejected_request"), "请求被拒绝");
  assert.equal(taskStatusLabel("succeeded"), "执行成功");
});


test("formats API errors without assuming every 401 is a market API-key problem", () => {
  const authError = describeApiError({
    status: 401,
    code: "auth_required",
    requestId: "req-401",
    message: "unauthorized"
  });
  assert.match(authError, /接口认证失败/);
  assert.match(authError, /HTTP 401/);
  assert.match(authError, /code=auth_required/);
  assert.match(authError, /request_id=req-401/);
  assert.doesNotMatch(authError, /行情 API Key/);

  const runError = describeApiError({
    status: 409,
    code: "run_not_finished",
    requestId: "req-run-pending",
    message: "run result is not ready"
  });
  assert.match(runError, /run result is not ready/);
  assert.match(runError, /HTTP 409/);
  assert.match(runError, /request_id=req-run-pending/);
});


test("describes HTTP error matrix distinctly and turns browser fetch failures into actionable Chinese feedback", () => {
  const cases = [
    { status: 401, code: "auth_required", expected: /接口认证失败/ },
    { status: 403, code: "access_denied", expected: /当前请求被服务拒绝/ },
    { status: 404, code: "route_not_found", expected: /接口或资源不存在/ },
    { status: 503, code: "provider_unavailable", message: "上游 Provider 暂不可用", expected: /上游 Provider 暂不可用/ }
  ];

  for (const item of cases) {
    const message = describeApiError({
      status: item.status,
      code: item.code,
      requestId: "req-" + item.status,
      message: item.message || "server diagnostic"
    });
    assert.match(message, item.expected, "HTTP " + item.status + " should have a specific diagnostic");
    assert.match(message, new RegExp("HTTP " + item.status));
    assert.match(message, new RegExp("code=" + item.code));
    assert.match(message, new RegExp("request_id=req-" + item.status));
  }

  const networkMessage = describeApiError(new TypeError("Failed to fetch"));
  assert.match(networkMessage, /网络请求失败/);
  assert.match(networkMessage, /检查网络连接、反向代理及服务状态/);
  assert.doesNotMatch(networkMessage, /Failed to fetch/);
  assert.equal(isRetryableApiError(new TypeError("Failed to fetch")), true);
});

test("classifies transient request failures separately from permanent client errors", () => {
  for (const error of [
    new Error("network offline"),
    { status: 408 },
    { status: 409, code: "run_not_finished" },
    { status: 425 },
    { status: 429 },
    { status: 500 },
    { status: 502 },
    { status: 503 },
    { status: 504 }
  ]) {
    assert.equal(isRetryableApiError(error), true, "should retry " + JSON.stringify(error));
  }

  for (const status of [400, 401, 403, 404, 405, 413, 415]) {
    assert.equal(isRetryableApiError({ status }), false, "should stop retrying HTTP " + status);
  }
});

 
test("numeric formatters reject booleans and non-scalar API values", () => {
  for (const value of [true, false, {}, [], " "]) {
    assert.equal(finiteNumberOrNull(value), null);
    assert.equal(formatPrice(value), "—");
    assert.equal(formatCompact(value), "—");
    assert.equal(formatPercent(value), "—");
  }
  assert.equal(finiteNumberOrNull("12.5"), 12.5);
  assert.equal(formatPrice("12.5"), "12.50");
});

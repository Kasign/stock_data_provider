import test from "node:test";
import assert from "node:assert/strict";
import { defaultMarketCapitalFlowDateRange, describeMarketOverviewError, isCurrentSentimentHistoryRequest, validateMarketDateRange } from "../src/market-overview.js";

test("market overview keeps authentication, permission, missing-route and provider errors distinct", () => {
  const cases = [
    { status: 401, code: "auth_required", message: "credentials required", expected: /接口认证失败/ },
    { status: 403, code: "access_denied", message: "permission denied", expected: /当前请求被服务拒绝/ },
    { status: 404, code: "route_not_found", message: "route not found", expected: /接口或资源不存在/ },
    { status: 503, code: "provider_unavailable", message: "upstream unavailable", expected: /upstream unavailable/ }
  ];

  for (const item of cases) {
    const result = describeMarketOverviewError({
      status: item.status,
      code: item.code,
      message: item.message,
      requestId: "req-" + item.status
    });
    assert.match(result, item.expected, "market overview HTTP " + item.status + " should be accurately described");
    assert.match(result, new RegExp("HTTP " + item.status));
    assert.match(result, new RegExp("code=" + item.code));
    assert.match(result, new RegExp("request_id=req-" + item.status));
    if ([401, 403].includes(item.status)) assert.doesNotMatch(result, /行情服务 API Key 验证失败/);
  }

  const networkMessage = describeMarketOverviewError(new TypeError("Failed to fetch"));
  assert.match(networkMessage, /网络请求失败/);
});

test("market capital-flow defaults to exactly 90 China calendar dates", () => {
  assert.deepEqual(defaultMarketCapitalFlowDateRange(new Date("2026-11-02T15:30:00Z")), {
    start: "2026-08-05", end: "2026-11-02"
  });
  assert.deepEqual(defaultMarketCapitalFlowDateRange(new Date("2026-11-02T16:30:00Z")), {
    start: "2026-08-06", end: "2026-11-03"
  });
});


test("sentiment history requests are rejected after cancellation or supersession", () => {
  const controller = new AbortController();
  assert.equal(isCurrentSentimentHistoryRequest(3, 3, controller.signal), true);
  assert.equal(isCurrentSentimentHistoryRequest(2, 3, controller.signal), false);
  controller.abort();
  assert.equal(isCurrentSentimentHistoryRequest(3, 3, controller.signal), false);
});

test("market observation date ranges validate dates, order, and inclusive natural-day limits", () => {
  const start = "2024-01-01";
  const afterDays = days => new Date(Date.parse(start + "T00:00:00Z") + days * 86400000).toISOString().slice(0, 10);

  assert.equal(validateMarketDateRange("", start, 2000), "请选择开始和结束日期。");
  assert.equal(validateMarketDateRange(start, afterDays(1999), 2000), "");
  assert.equal(validateMarketDateRange(start, afterDays(2000), 2000), "日期范围不能超过 2000 个自然日。");
  assert.equal(validateMarketDateRange("2024-02-30", "2024-03-01"), "请选择有效的开始和结束日期。");
  assert.equal(validateMarketDateRange("2024-03-02", "2024-03-01"), "开始日期不能晚于结束日期。");

  // 情绪历史接口没有 2000 自然日上限，不能把资金流接口限制错误复用到该查询。
  assert.equal(validateMarketDateRange(start, afterDays(2000)), "");
});

import test from "node:test";
import assert from "node:assert/strict";
import { describeMarketOverviewError } from "../src/market-overview.js";

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

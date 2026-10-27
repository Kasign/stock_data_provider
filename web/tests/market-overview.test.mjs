import test from "node:test";
import assert from "node:assert/strict";
import { summarizeMarketOverviewRefresh } from "../src/market-overview.js";

test("reports missing API key without implying that requests were attempted", () => {
  assert.equal(
    summarizeMarketOverviewRefresh(["needs_key", "needs_key"], false),
    "请先配置行情 API Key 后刷新市场概况"
  );
});

test("reports received responses without claiming data quality is valid", () => {
  assert.equal(
    summarizeMarketOverviewRefresh(["received", "received", "received", "received", "received"]),
    "本次收到 5/5 个模块响应；请查看各模块来源时间与质量状态"
  );
});

test("summarizes unsupported, unavailable, authentication and general failures separately", () => {
  assert.equal(
    summarizeMarketOverviewRefresh(["received", "unsupported", "provider_unavailable", "auth_failed", "failed"]),
    "本次收到 1/5 个模块响应；接口尚未提供 1 个；Provider 不可用 1 个；鉴权失败 1 个；读取失败 1 个"
  );
});

test("reports missing-key and cancelled modules in partial refreshes", () => {
  assert.equal(
    summarizeMarketOverviewRefresh(["received", "needs_key", "cancelled"]),
    "本次收到 1/3 个模块响应；缺少 API Key、未请求 1 个；已取消 1 个"
  );
});

test("normalizes unknown outcomes as failures and handles an empty input", () => {
  assert.equal(summarizeMarketOverviewRefresh(["unexpected"]), "本次收到 0/1 个模块响应；读取失败 1 个");
  assert.equal(summarizeMarketOverviewRefresh([]), "本次未执行市场模块请求");
});

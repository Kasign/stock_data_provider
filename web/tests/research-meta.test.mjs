import test from "node:test";
import assert from "node:assert/strict";
import { buildResearchMetaText } from "../src/research-meta.js";

test("统一元信息保留来源、版本、供应商时间和获取时间的语义", () => {
  const text = buildResearchMetaText({
    source: "eastmoney",
    source_version: "v2",
    provider_timestamp: "2026-11-03T09:30:00+08:00",
    fetched_at: "2026-11-03T09:30:05+08:00"
  });
  assert.match(text, /来源 eastmoney/);
  assert.match(text, /源端版本 v2/);
  assert.match(text, /供应商时间 /);
  assert.match(text, /获取时间 /);
  assert.ok(text.indexOf("供应商时间") < text.indexOf("获取时间"));
});

test("缺失来源与获取时间显示未报告，不补零", () => {
  const text = buildResearchMetaText({ source: null, fetched_at: null });
  assert.match(text, /来源 未报告/);
  assert.match(text, /获取时间 未报告/);
  assert.doesNotMatch(text, /来源 0|获取时间 0/);
});

test("源端报告日期、供应商日期、数据日期和交易日期保持独立", () => {
  const text = buildResearchMetaText({
    source: "provider",
    provider_report_date: "2026-10-31",
    provider_date: "2026-11-01",
    date: "2026-11-02",
    trade_date: "2026-11-03",
    fetched_at: "2026-11-03T09:30:00+08:00"
  });
  assert.match(text, /源端报告日期 2026-10-31/);
  assert.match(text, /供应商日期 2026-11-01/);
  assert.match(text, /数据日期 2026-11-02/);
  assert.match(text, /交易日期 2026-11-03/);
});

test("模块配置要求的供应商时间与数据日期缺失时明确显示未报告", () => {
  const text = buildResearchMetaText({ source: "provider", fetched_at: null }, {
    includeProviderTimestamp: true,
    includeProviderDate: true,
    includeDataDate: true,
    details: [["报告期", "2026 Q3"]]
  });
  assert.match(text, /报告期 2026 Q3/);
  assert.match(text, /供应商时间 未报告/);
  assert.match(text, /供应商日期 未报告/);
  assert.match(text, /数据日期 未报告/);
  assert.match(text, /获取时间 未报告/);
});

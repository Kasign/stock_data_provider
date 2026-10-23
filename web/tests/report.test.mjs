import test from "node:test";
import assert from "node:assert/strict";
import { buildRunResultFilename, buildRunResultMarkdown } from "../src/report.js";

test("exports a readable report from actual result fields and marks missing data", () => {
  const markdown = buildRunResultMarkdown({
    run_id: "run-abc",
    status: "blocked_data_quality",
    request: { symbols: ["600519"], task_type: "post_market_scan", as_of: "2026-10-23T09:30:00+08:00" },
    quality: { core_pass: false, issues: [{ code: "quote_missing", message: "No quote" }] },
    signals: [],
    warnings: ["daily bars unavailable"]
  });
  assert.match(markdown, /run-abc/);
  assert.match(markdown, /数据质量与策略门禁/);
  assert.match(markdown, /quote_missing/);
  assert.match(markdown, /本次 Run 未返回相关数据/);
  assert.match(markdown, /不构成交易建议/);
});

test("creates safe, deterministic filenames from symbols and timestamps", () => {
  const name = buildRunResultFilename({
    request: { symbols: ["600519", "000001"], as_of: "2026-10-23T09:30:00+08:00" }
  }, ".md", new Date("2020-01-01T00:00:00Z"));
  assert.equal(name, "600519-000001-2026-10-23T09-30-00.md");
  assert.equal(buildRunResultFilename({}, "???", new Date("2026-01-02T03:04:05Z")), "analysis-2026-01-02T03-04-05.txt");
});

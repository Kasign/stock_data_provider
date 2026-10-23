import test from "node:test";
import assert from "node:assert/strict";
import { buildRunResultFilename, buildRunResultMarkdown } from "../src/report.js";
import { runResultFixtures } from "./fixtures/run-results.mjs";

test("exports every modeled Go RunResult state from deterministic fixtures", () => {
  const missingCollections = { ...runResultFixtures.emptyCompleted, run_id: "run-fixture-missing-collections" };
  for (const key of ["factors", "signals", "tradability", "risk", "plans", "planning_rejections"]) {
    delete missingCollections[key];
  }
  const malformedCollections = {
    ...runResultFixtures.emptyCompleted,
    run_id: "run-fixture-malformed-collections",
    factors: { invalid: true },
    signals: null
  };
  const scenarios = [
    ["completed", runResultFixtures.completed],
    ["empty completed", runResultFixtures.emptyCompleted],
    ["missing collections", missingCollections],
    ["malformed collections", malformedCollections],
    ["partial", runResultFixtures.partial],
    ["blocked quality", runResultFixtures.blockedDataQuality],
    ["rejected request", runResultFixtures.rejectedRequest],
    ["failed", runResultFixtures.failed]
  ];

  for (const [label, result] of scenarios) {
    const markdown = buildRunResultMarkdown(result);
    assert.ok(markdown.includes("- Run ID：" + result.run_id), label + " should retain its Run ID");
    assert.ok(markdown.includes("- 运行状态：" + result.status.replace(/_/g, "\\_")), label + " should retain the server status");
    assert.match(markdown, /数据质量与策略门禁/, label + " should include the quality section");
    assert.match(markdown, /不构成交易建议/, label + " should retain the disclaimer");
  }
});

test("completed results preserve actual structured fields including gates, rules and execution quantities", () => {
  const markdown = buildRunResultMarkdown(runResultFixtures.completed);

  assert.match(markdown, /核心门禁：是/);
  assert.ok(markdown.includes('- 策略资格：`{"trend_following":true}`'));
  assert.ok(markdown.includes("- 策略阻断原因：`{}`"));
  assert.match(markdown, /因子 ID：ma_distance/);
  assert.match(markdown, /入场规则：合成测试规则：条件 A 成立/);
  assert.match(markdown, /可交易性检查/);
  assert.match(markdown, /风险检查/);
  assert.match(markdown, /参考价格：1326/);
  assert.match(markdown, /已成交数量：0/);
  assert.match(markdown, /没有连接或执行真实交易/);
});

test("empty and partial results distinguish empty collections from missing fields", () => {
  const emptyMarkdown = buildRunResultMarkdown(runResultFixtures.emptyCompleted);
  assert.match(emptyMarkdown, /本次 Run 明确返回空集合/);
  assert.doesNotMatch(emptyMarkdown, /策略资格：.*false/);
  assert.doesNotMatch(emptyMarkdown, /信号数：0/);

  const partialMarkdown = buildRunResultMarkdown(runResultFixtures.partial);
  assert.match(partialMarkdown, /运行状态：partial/);
  assert.match(partialMarkdown, /是否部分数据：是/);
  assert.match(partialMarkdown, /质量问题：.*optional_dataset_missing/);
  assert.match(partialMarkdown, /因子值：未报告/);
  assert.match(partialMarkdown, /缺失原因：上游未提供成交量字段/);
  assert.match(partialMarkdown, /策略阻断原因：.*可选数据不完整/);
  assert.match(partialMarkdown, /未生成计划/);
});

test("quality-blocked results expose the gate failure instead of suggesting a plan", () => {
  const markdown = buildRunResultMarkdown(runResultFixtures.blockedDataQuality);

  assert.match(markdown, /运行状态：blocked_data_quality/);
  assert.match(markdown, /核心门禁：否/);
  assert.match(markdown, /required_quote_missing/);
  assert.match(markdown, /缺少必需的实时行情/);
  assert.match(markdown, /本次 Run 明确返回空集合/);
  assert.doesNotMatch(markdown, /### 600519 · buy/);
});

test("rejected and failed runs export the server failure code, stage and message", () => {
  for (const result of [runResultFixtures.rejectedRequest, runResultFixtures.failed]) {
    const markdown = buildRunResultMarkdown(result);
    assert.match(markdown, /运行失败信息/);
    assert.ok(markdown.includes("错误代码：" + result.failure.code.replace(/_/g, "\\_")));
    assert.ok(markdown.includes("失败阶段：" + result.failure.stage.replace(/_/g, "\\_")));
    assert.ok(markdown.includes("消息：" + result.failure.message));
  }
});

test("distinguishes missing, empty and malformed result collections", () => {
  const missing = { ...runResultFixtures.emptyCompleted };
  delete missing.factors;
  missing.signals = null;
  const missingMarkdown = buildRunResultMarkdown(missing);
  assert.match(missingMarkdown, /未报告相关数据（字段缺失或 null）/);

  const emptyMarkdown = buildRunResultMarkdown({ ...runResultFixtures.emptyCompleted, factors: [], signals: [] });
  assert.match(emptyMarkdown, /明确返回空集合/);

  const malformedMarkdown = buildRunResultMarkdown({ ...runResultFixtures.emptyCompleted, factors: { items: [] } });
  assert.match(malformedMarkdown, /集合格式无效，未展示/);
});

test("escapes server-provided Markdown text and renders objects as code", () => {
  const result = {
    ...runResultFixtures.completed,
    request: { ...runResultFixtures.completed.request, task_type: "__inject__ **bold**" },
    warnings: ["## Fake heading\\n<script>alert(1)</script>", "`ticks` | pipe"]
  };
  const markdown = buildRunResultMarkdown(result);
  assert.doesNotMatch(markdown, /^## Fake heading/m);
  assert.doesNotMatch(markdown, /<script>/);
  assert.match(markdown, /\\_\\_inject\\_\\_/);
  assert.match(markdown, /\\\\\| pipe/);
  assert.ok(markdown.includes('- 策略资格：`{"trend_following":true}`'));
});

test("creates safe, deterministic filenames from symbols and timestamps", () => {
  const name = buildRunResultFilename({
    request: { symbols: ["600519", "000001"], as_of: "2026-10-23T09:30:00+08:00" }
  }, ".md", new Date("2020-01-01T00:00:00Z"));
  assert.equal(name, "600519-000001-2026-10-23T09-30-00.md");
  assert.equal(buildRunResultFilename({}, "???", new Date("2026-01-02T03:04:05Z")), "analysis-2026-01-02T03-04-05.txt");
});

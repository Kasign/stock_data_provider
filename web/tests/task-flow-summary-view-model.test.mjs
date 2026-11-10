import test from "node:test";
import assert from "node:assert/strict";
import { buildTaskFlowSummaryViewModel } from "../src/task-flow-summary-view-model.js";

const formatDateTime = value => "时间:" + value;

test("unloaded task distinguishes no task from waiting for event read", () => {
  assert.deepEqual(buildTaskFlowSummaryViewModel(), {
    eventsCount: "未选择任务", latestEvent: "未报告", quality: "等待 Run 结果",
    qualityClass: "is-unknown", warningCount: "等待 Run 结果"
  });
  assert.equal(buildTaskFlowSummaryViewModel({ taskId: "task-1" }).eventsCount, "等待事件读取");
});

test("loaded event summary sorts events and preserves nested event timestamp", () => {
  const result = buildTaskFlowSummaryViewModel({
    taskLoaded: true,
    taskEvents: [{ sequence: 2, event: { at: "later" } }, { sequence: 1, at: "earlier" }],
    formatDateTime
  });
  assert.equal(result.eventsCount, "2");
  assert.equal(result.latestEvent, "时间:later");
});

test("event read failure distinguishes retained events from an empty failed read", () => {
  assert.equal(buildTaskFlowSummaryViewModel({ taskLoaded: true, taskEventsError: "timeout", taskEvents: [{ sequence: 1 }] }).eventsCount, "1 条（读取异常）");
  assert.equal(buildTaskFlowSummaryViewModel({ taskLoaded: true, taskEventsError: "timeout" }).eventsCount, "读取失败");
  assert.equal(buildTaskFlowSummaryViewModel({ taskLoaded: true }).eventsCount, "暂无持久化事件");
});

test("run quality and warning summaries retain strict true false and unknown semantics", () => {
  const passed = buildTaskFlowSummaryViewModel({ lastRunResult: { quality: { core_pass: true }, warnings: [1, 2] } });
  assert.equal(passed.quality, "通过");
  assert.equal(passed.qualityClass, "is-valid");
  assert.equal(passed.warningCount, "2");
  const failed = buildTaskFlowSummaryViewModel({ lastRunResult: { quality: { core_pass: false }, warnings: [] } });
  assert.equal(failed.quality, "未通过");
  assert.equal(failed.qualityClass, "is-invalid");
  assert.equal(failed.warningCount, "0");
  const unknown = buildTaskFlowSummaryViewModel({ lastRunResult: {} });
  assert.equal(unknown.quality, "未报告");
  assert.equal(unknown.warningCount, "未报告");
});

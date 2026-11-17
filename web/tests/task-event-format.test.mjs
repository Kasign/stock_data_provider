import test from "node:test";
import assert from "node:assert/strict";
import { taskEventDurationLabel, taskEventStatusKey, taskEventStatusLabel, taskEventStatusTone } from "../src/task-event-format.js";

test("任务事件状态映射保持已知状态语义", () => {
  assert.equal(taskEventStatusKey(" SUCCEEDED "), "succeeded");
  assert.equal(taskEventStatusLabel("succeeded"), "已完成");
  assert.equal(taskEventStatusTone("succeeded"), "is-local");
  assert.equal(taskEventStatusTone("failed"), "is-error");
  assert.equal(taskEventStatusTone("partial"), "is-warning");
});

test("任务事件未知状态保留原始文案并安全归类", () => {
  assert.equal(taskEventStatusKey("provider-paused"), "other");
  assert.equal(taskEventStatusLabel("provider-paused"), "provider-paused");
  assert.equal(taskEventStatusKey(""), "unknown");
  assert.equal(taskEventStatusLabel(""), "状态未报告");
  assert.equal(taskEventStatusTone("unknown"), "is-readonly");
});


test("任务事件耗时格式在节点与详情展示中保持一致", () => {
  assert.equal(taskEventDurationLabel(0), "0 毫秒");
  assert.equal(taskEventDurationLabel(999), "999 毫秒");
  assert.equal(taskEventDurationLabel(1000), "1.00 秒");
  assert.equal(taskEventDurationLabel(1234.56), "1.23 秒");
  assert.equal(taskEventDurationLabel("2500"), "2.50 秒");
});

test("任务事件无效耗时不生成展示文案", () => {
  for (const value of [undefined, null, "", "invalid", NaN, -1, -0.1]) {
    assert.equal(taskEventDurationLabel(value), "");
  }
});

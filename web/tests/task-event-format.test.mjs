import test from "node:test";
import assert from "node:assert/strict";
import { taskEventStatusKey, taskEventStatusLabel, taskEventStatusTone } from "../src/task-event-format.js";

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

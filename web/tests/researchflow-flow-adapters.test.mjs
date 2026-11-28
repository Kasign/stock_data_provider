import test from "node:test";
import assert from "node:assert/strict";
import {
  researchSymbol,
  mapRunStatusToTaskStatus,
  runStageToTaskEvent
} from "../src/researchflow-flow-adapters.js";

test("researchSymbol 把六位 A 股代码映射为带交易所后缀的契约格式", () => {
  assert.equal(researchSymbol("600519"), "600519.SH");
  assert.equal(researchSymbol("601398"), "601398.SH");
  assert.equal(researchSymbol("000001"), "000001.SZ");
  assert.equal(researchSymbol("300750"), "300750.SZ");
  assert.equal(researchSymbol("830001"), "830001.BJ");
  assert.equal(researchSymbol("600519.SH"), "600519.SH", "幂等：已带后缀原样返回");
});

test("researchSymbol 对非法代码 fail-closed", () => {
  for (const input of ["", "abc", "60051", "600519 XS", "1234567"]) {
    assert.throws(() => researchSymbol(input), /标的|代码/);
  }
});

test("mapRunStatusToTaskStatus 把 Researchflow Run 状态映射到页面任务状态", () => {
  assert.equal(mapRunStatusToTaskStatus("created"), "queued");
  assert.equal(mapRunStatusToTaskStatus("queued"), "queued");
  assert.equal(mapRunStatusToTaskStatus("running"), "running");
  assert.equal(mapRunStatusToTaskStatus("cancelling"), "running");
  assert.equal(mapRunStatusToTaskStatus("succeeded"), "succeeded");
  assert.equal(mapRunStatusToTaskStatus("partial"), "partial");
  assert.equal(mapRunStatusToTaskStatus("failed"), "failed");
  assert.equal(mapRunStatusToTaskStatus("cancelled"), "failed");
});

test("mapRunStatusToTaskStatus 对未知状态 fail-closed 返回 queued 并标记未知", () => {
  assert.equal(mapRunStatusToTaskStatus("unexpected"), "queued");
});

test("runStageToTaskEvent 把 Stage 投影为页面事件项，不伪造序号以外的执行明细", () => {
  const stage = {
    stageRef: "stage-1",
    stageCode: "quant.sma20",
    status: "succeeded",
    outputRef: "out-1",
    reasonCodes: ["DATA_DEGRADED"],
    required: true
  };
  const event = runStageToTaskEvent(stage, 1);
  assert.equal(event.sequence, 1);
  assert.equal(event.event, "stage_status");
  assert.equal(event.stage_ref, "stage-1");
  assert.equal(event.stage_code, "quant.sma20");
  assert.equal(event.status, "succeeded");
  assert.equal(event.level, "info");
  assert.deepEqual(event.reason_codes, ["DATA_DEGRADED"]);
});

test("runStageToTaskEvent 对 degraded/failed  Stage 标记警示级别", () => {
  assert.equal(runStageToTaskEvent({ stageCode: "quant.risk", status: "degraded", reasonCodes: [] }, 2).level, "warning");
  assert.equal(runStageToTaskEvent({ stageCode: "quant.risk", status: "failed", reasonCodes: [] }, 3).level, "error");
  assert.equal(runStageToTaskEvent({ stageCode: "quant.risk", status: "skipped", reasonCodes: [] }, 4).level, "info");
});

test("runStageToTaskEvent 拒绝无 stage_code 的非法 Stage", () => {
  assert.throws(() => runStageToTaskEvent({ status: "succeeded" }, 1), /stage/);
});

import test from "node:test";
import assert from "node:assert/strict";
import { TASK_EVENT_PAGE_SIZE, validateTaskEventPage } from "../src/task-workflow.js";

function event(sequence, runId = "run-1", extra = {}) {
  return { sequence, event: { run_id: runId, stage: "stage-" + sequence, ...extra } };
}

function page(events, nextAfterSequence = events.at(-1)?.sequence ?? 0, taskId = "task-1") {
  return { task_id: taskId, events, next_after_sequence: nextAfterSequence };
}

test("accepts a valid event page and advances its cursor independently of event sequence", () => {
  const result = validateTaskEventPage(page([event(2), event(3)], 5), {
    taskId: "task-1", runId: "run-1", afterSequence: 1
  });
  assert.equal(result.error, "");
  assert.equal(result.nextAfterSequence, 5);
  assert.deepEqual(result.events.map((item) => item.sequence), [2, 3]);
  assert.equal(result.hasMore, false);
});

test("rejects malformed envelopes, task identity mismatches, and non-advancing cursors", () => {
  assert.match(validateTaskEventPage(null, { taskId: "task-1" }).error, /结构无效/);
  const wrongTask = validateTaskEventPage(page([], 0, "task-2"), { taskId: "task-1" });
  assert.equal(wrongTask.identityMismatch, true);
  assert.match(wrongTask.error, /其他任务/);
  assert.match(validateTaskEventPage(page([event(2)], 1), {
    taskId: "task-1", runId: "run-1", afterSequence: 1
  }).error, /next_after_sequence 无效或未前进/);
});

test("accepts task-scoped events when the optional nested Run ID is absent", () => {
  const result = validateTaskEventPage(page([
    { sequence: 1, event: { stage: "queued", status: "queued" } },
    event(2)
  ], 2), { taskId: "task-1", runId: "run-1", afterSequence: 0 });
  assert.deepEqual(result.events.map((item) => item.sequence), [1, 2]);
  assert.equal(result.rejected.mismatchedRun, 0);
});

test("rejects event rows with invalid structure or a mismatched Run ID without blocking cursor progress", () => {
  const result = validateTaskEventPage(page([
    null,
    { sequence: 2, event: [] },
    event(3, "other-run"),
    event(4)
  ], 8), { taskId: "task-1", runId: "run-1", afterSequence: 1 });
  assert.equal(result.nextAfterSequence, 8);
  assert.deepEqual(result.events.map((item) => item.sequence), [4]);
  assert.deepEqual(result.rejected, { invalidStructure: 2, mismatchedRun: 1, ignoredSequence: 0 });
});

test("ignores old and duplicate sequence numbers across and within pages", () => {
  const known = new Map([[5, event(5)]]);
  const result = validateTaskEventPage(page([event(4), event(5), event(6), event(6)], 9), {
    taskId: "task-1", runId: "run-1", afterSequence: 3, highestSequence: 5, knownSequences: known
  });
  assert.deepEqual(result.events.map((item) => item.sequence), [6]);
  assert.equal(result.rejected.ignoredSequence, 3);
});

test("marks a full page as requiring another read", () => {
  const events = Array.from({ length: TASK_EVENT_PAGE_SIZE }, (_, index) => event(index + 1));
  const result = validateTaskEventPage(page(events, TASK_EVENT_PAGE_SIZE), {
    taskId: "task-1", runId: "run-1", afterSequence: 0
  });
  assert.equal(result.hasMore, true);
  assert.equal(result.events.length, TASK_EVENT_PAGE_SIZE);
});

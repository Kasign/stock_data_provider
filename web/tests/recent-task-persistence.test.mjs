import test from "node:test";
import assert from "node:assert/strict";
import { RECENT_TASKS_KEY, readRecentTasks, upsertRecentTask } from "../src/recent-tasks.js";

test("does not report a task history write as persisted when stored fields differ", () => {
  const values = new Map([[RECENT_TASKS_KEY, JSON.stringify([{
    task_id: "existing-task",
    run_id: "existing-run",
    created_at: "2026-10-28T08:00:00Z",
    symbols: ["600519.SH"],
    task_type: "post_market_scan",
    status: "running",
    analysis_base: "/analysis-api",
    updated_at: "2026-10-28T08:01:00Z"
  }])]]);
  const storage = {
    getItem(key) {
      return values.has(key) ? values.get(key) : null;
    },
    setItem(key, value) {
      const records = JSON.parse(String(value));
      // Simulate a storage adapter that returns success but silently changes a business field.
      records[0].status = "queued";
      values.set(key, JSON.stringify(records));
    }
  };

  const returned = upsertRecentTask({
    task_id: "new-task",
    run_id: "new-run",
    created_at: "2026-10-29T08:00:00Z",
    symbols: ["000001.SZ"],
    task_type: "post_market_scan",
    status: "succeeded",
    analysis_base: "/analysis-api",
    updated_at: "2026-10-29T08:01:00Z"
  }, storage);

  assert.deepEqual(returned.map((item) => item.task_id), ["existing-task"]);
  assert.deepEqual(readRecentTasks(storage).map((item) => item.task_id), ["new-task", "existing-task"]);
  assert.equal(readRecentTasks(storage)[0].status, "queued");
});

import test from "node:test";
import assert from "node:assert/strict";
import { MAX_RECENT_TASKS, RECENT_TASKS_KEY, readRecentTasks, upsertRecentTask } from "../src/recent-tasks.js";

function makeStorage(seed = {}) {
  const values = new Map(Object.entries(seed));
  return {
    getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { values.set(key, String(value)); }
  };
}

test("upserts task references without duplicate IDs and keeps newest first", () => {
  const storage = makeStorage();
  upsertRecentTask({ task_id: "task-a", symbols: ["600519"], status: "running" }, storage);
  upsertRecentTask({ task_id: "task-b", symbols: ["000001"], status: "succeeded" }, storage);
  upsertRecentTask({ task_id: "task-a", run_id: "run-a", symbols: ["600519"], status: "succeeded" }, storage);
  const items = readRecentTasks(storage);
  assert.deepEqual(items.map((item) => item.task_id), ["task-a", "task-b"]);
  assert.equal(items[0].run_id, "run-a");
  assert.equal(items[0].status, "succeeded");
});

test("caps browser history and rejects malformed stored records", () => {
  const storage = makeStorage();
  for (let i = 0; i < MAX_RECENT_TASKS + 3; i++) upsertRecentTask({ task_id: "task-" + i }, storage);
  assert.equal(readRecentTasks(storage).length, MAX_RECENT_TASKS);
  assert.equal(readRecentTasks(storage)[0].task_id, "task-" + (MAX_RECENT_TASKS + 2));
  storage.setItem(RECENT_TASKS_KEY, "{invalid");
  assert.deepEqual(readRecentTasks(storage), []);
});

test("storage failures do not throw or create bogus history entries", () => {
  const storage = { getItem() { throw new Error("storage disabled"); }, setItem() { throw new Error("storage disabled"); } };
  assert.deepEqual(readRecentTasks(storage), []);
  assert.deepEqual(upsertRecentTask({ task_id: "task-a" }, storage), [{
    task_id: "task-a", run_id: "", symbols: [], task_type: "post_market_scan", status: "", updated_at: ""
  }]);
});

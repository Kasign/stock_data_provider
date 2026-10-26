import test from "node:test";
import assert from "node:assert/strict";
import { clearRecentTasks, MAX_RECENT_TASKS, RECENT_TASKS_KEY, readRecentTasks, removeRecentTask, upsertRecentTask } from "../src/recent-tasks.js";

function makeStorage(seed = {}) {
  const values = new Map(Object.entries(seed));
  return {
    getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { values.set(key, String(value)); },
    removeItem(key) { values.delete(key); }
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
    task_id: "task-a", run_id: "", symbols: [], task_type: "post_market_scan", status: "", updated_at: "", analysis_base: ""
  }]);
});


test("stores only task references and excludes report payloads and secrets", () => {
  const storage = makeStorage();
  const input = {
    task_id: "task-safe",
    run_id: "run-safe",
    symbols: ["600519"],
    status: "succeeded",
    report: "private report body",
    result: { quality: { core_pass: true } },
    api_key: "private-api-key",
    settings: { apiKey: "private-api-key" }
  };
  const records = upsertRecentTask(input, storage);
  const stored = JSON.parse(storage.getItem(RECENT_TASKS_KEY));

  assert.deepEqual(Object.keys(stored[0]).sort(), [
    "run_id", "status", "symbols", "task_id", "task_type", "updated_at"
  ].sort());
  assert.equal(records[0].task_id, "task-safe");
  assert.doesNotMatch(storage.getItem(RECENT_TASKS_KEY), /private report body|private-api-key|quality/);
});


test("removes only the selected task reference and preserves remaining order", () => {
  const storage = makeStorage();
  upsertRecentTask({ task_id: "task-a" }, storage);
  upsertRecentTask({ task_id: "task-b" }, storage);
  upsertRecentTask({ task_id: "task-c" }, storage);

  const items = removeRecentTask("task-b", storage);
  assert.deepEqual(items.map((item) => item.task_id), ["task-c", "task-a"]);
  assert.deepEqual(readRecentTasks(storage).map((item) => item.task_id), ["task-c", "task-a"]);
  assert.deepEqual(removeRecentTask("", storage).map((item) => item.task_id), ["task-c", "task-a"]);
});

test("clears local task references without touching unrelated storage keys", () => {
  const storage = makeStorage({ unrelated: "keep-me" });
  upsertRecentTask({ task_id: "task-a" }, storage);
  upsertRecentTask({ task_id: "task-b" }, storage);

  assert.deepEqual(clearRecentTasks(storage), []);
  assert.deepEqual(readRecentTasks(storage), []);
  assert.equal(storage.getItem("unrelated"), "keep-me");
  assert.equal(storage.getItem(RECENT_TASKS_KEY), null);
});

test("remove and clear operations tolerate disabled browser storage", () => {
  const storage = {
    getItem() { throw new Error("storage disabled"); },
    setItem() { throw new Error("storage disabled"); },
    removeItem() { throw new Error("storage disabled"); }
  };
  assert.deepEqual(removeRecentTask("task-a", storage), []);
  assert.deepEqual(clearRecentTasks(storage), []);
});

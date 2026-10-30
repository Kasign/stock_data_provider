import test from "node:test";
import assert from "node:assert/strict";
import { clearRecentTasks, MAX_RECENT_TASKS, RECENT_TASKS_KEY, readRecentTasks, readRecentTasksState, removeRecentTask, upsertRecentTask } from "../src/recent-tasks.js";

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
  assert.equal(items[0].created_at, "");
});

test("caps browser history and rejects malformed stored records", () => {
  const storage = makeStorage();
  for (let i = 0; i < MAX_RECENT_TASKS + 3; i++) upsertRecentTask({ task_id: "task-" + i }, storage);
  assert.equal(readRecentTasks(storage).length, MAX_RECENT_TASKS);
  assert.equal(readRecentTasks(storage)[0].task_id, "task-" + (MAX_RECENT_TASKS + 2));
  storage.setItem(RECENT_TASKS_KEY, "{invalid");
  assert.deepEqual(readRecentTasks(storage), []);
  assert.deepEqual(readRecentTasksState(storage), {
    items: [], storageReadable: true, storageValid: false, reason: "invalid"
  });
});

test("rejects duplicate task references for the same analysis service without rewriting stored data", () => {
  const records = [
    { task_id: "task-a", analysis_base: "/api", run_id: "run-1" },
    { task_id: "task-a", analysis_base: "/api", run_id: "run-2" },
    { task_id: "task-a", analysis_base: "/other-api", run_id: "run-3" }
  ];
  const original = JSON.stringify(records);
  const storage = makeStorage({ [RECENT_TASKS_KEY]: original });

  const state = readRecentTasksState(storage);
  assert.equal(state.storageReadable, true);
  assert.equal(state.storageValid, false);
  assert.equal(state.reason, "invalid");
  assert.deepEqual(state.items.map((item) => [item.task_id, item.analysis_base]), [
    ["task-a", "/api"],
    ["task-a", "/other-api"]
  ]);
  assert.deepEqual(upsertRecentTask({ task_id: "task-b" }, storage), state.items);
  assert.equal(storage.getItem(RECENT_TASKS_KEY), original, "invalid history must not be rewritten as a partial list");
});

test("storage failures do not throw or write task history after an unsuccessful read", () => {
  let writes = 0;
  const storage = {
    getItem() { throw new Error("storage disabled"); },
    setItem() { writes += 1; throw new Error("storage disabled"); }
  };
  assert.deepEqual(readRecentTasks(storage), []);
  assert.deepEqual(readRecentTasksState(storage), {
    items: [], storageReadable: false, storageValid: false, reason: "unavailable"
  });
  assert.deepEqual(upsertRecentTask({ task_id: "task-a" }, storage), []);
  assert.deepEqual(removeRecentTask("task-a", storage), []);
  assert.equal(writes, 0);
});


test("stores only task references and excludes report payloads and secrets", () => {
  const storage = makeStorage();
  const input = {
    task_id: "task-safe",
    run_id: "run-safe",
    created_at: "2026-10-27T01:20:00Z",
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
    "analysis_base", "created_at", "run_id", "status", "symbols", "task_id", "task_type", "updated_at"
  ].sort());
  assert.equal(records[0].task_id, "task-safe");
  assert.equal(records[0].created_at, "2026-10-27T01:20:00Z");
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

test("keeps task history when storage silently ignores removeItem during clear", () => {
  const seedStorage = makeStorage();
  upsertRecentTask({ task_id: "task-a" }, seedStorage);
  upsertRecentTask({ task_id: "task-b" }, seedStorage);
  const raw = seedStorage.getItem(RECENT_TASKS_KEY);
  const storage = {
    getItem() { return raw; },
    // Some adapters can fail silently instead of throwing.
    removeItem() {},
    setItem() {}
  };

  assert.deepEqual(clearRecentTasks(storage).map((item) => item.task_id), ["task-b", "task-a"]);
  assert.deepEqual(readRecentTasks(storage).map((item) => item.task_id), ["task-b", "task-a"]);
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


test("does not overwrite blank, malformed, non-array, or partially malformed history", () => {
  const invalidValues = [
    "",
    "{invalid",
    JSON.stringify({ items: [] }),
    JSON.stringify([{ task_id: "valid-task" }, { status: "missing-task-id" }])
  ];

  for (const raw of invalidValues) {
    let writes = 0;
    const storage = {
      getItem() { return raw; },
      setItem() { writes += 1; }
    };

    const state = readRecentTasksState(storage);
    assert.equal(state.storageReadable, true);
    assert.equal(state.storageValid, false);
    assert.equal(state.reason, "invalid");
    assert.deepEqual(upsertRecentTask({ task_id: "new-task" }, storage), state.items);
    assert.deepEqual(removeRecentTask("valid-task", storage), state.items);
    assert.equal(writes, 0, "invalid history must not be replaced with a partial list");
    assert.equal(storage.getItem(RECENT_TASKS_KEY), raw, "original stored bytes must remain unchanged");
  }
});


test("does not return a new history item when browser storage rejects the write", () => {
  const stored = new Map();
  const storage = {
    getItem(key) { return stored.has(key) ? stored.get(key) : null; },
    setItem() { throw new DOMException("Quota exceeded", "QuotaExceededError"); },
    removeItem(key) { stored.delete(key); }
  };

  const returned = upsertRecentTask({
    task_id: "task-not-persisted",
    analysis_base: "/analysis-api",
    status: "queued"
  }, storage);

  assert.deepEqual(returned, []);
  assert.deepEqual(readRecentTasksState(storage), {
    items: [], storageReadable: true, storageValid: true, reason: ""
  });
});


test("keeps the existing task list when removing a task cannot be persisted", () => {
  const stored = new Map();
  const seedStorage = makeStorage();
  upsertRecentTask({ task_id: "task-a", status: "running" }, seedStorage);
  upsertRecentTask({ task_id: "task-b", status: "succeeded" }, seedStorage);
  stored.set(RECENT_TASKS_KEY, seedStorage.getItem(RECENT_TASKS_KEY));
  const storage = {
    getItem(key) { return stored.has(key) ? stored.get(key) : null; },
    setItem() { throw new DOMException("Quota exceeded", "QuotaExceededError"); },
    removeItem(key) { stored.delete(key); }
  };

  const returned = removeRecentTask("task-b", storage);

  assert.deepEqual(returned.map((item) => item.task_id), ["task-b", "task-a"]);
  assert.deepEqual(readRecentTasks(storage).map((item) => item.task_id), ["task-b", "task-a"]);
});


test("keeps the existing task list when clearing history cannot be persisted", () => {
  const seedStorage = makeStorage();
  upsertRecentTask({ task_id: "task-a" }, seedStorage);
  upsertRecentTask({ task_id: "task-b" }, seedStorage);
  const raw = seedStorage.getItem(RECENT_TASKS_KEY);
  const storage = {
    getItem() { return raw; },
    removeItem() { throw new DOMException("Storage unavailable", "SecurityError"); },
    setItem() { throw new DOMException("Storage unavailable", "SecurityError"); }
  };

  assert.deepEqual(clearRecentTasks(storage).map((item) => item.task_id), ["task-b", "task-a"]);
  assert.deepEqual(readRecentTasks(storage).map((item) => item.task_id), ["task-b", "task-a"]);
});

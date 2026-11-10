import test from "node:test";
import assert from "node:assert/strict";
import { buildTaskEventListViewModel } from "../src/task-event-list-view-model.js";

const event = (sequence, status, nested = false) => ({
  sequence,
  ...(nested ? { event: { status } } : { status })
});

test("sorts events and computes independent counts for every filter", () => {
  const result = buildTaskEventListViewModel({
    items: [event(3, "succeeded"), event(1, "failed"), event(2, "running"), event(4, "skipped", true)],
    filter: "all"
  });
  assert.deepEqual(result.allEvents.map(item => item.sequence), [1, 2, 3, 4]);
  assert.deepEqual(result.filterCounts, { all: 4, issues: 1, active: 1, completed: 2 });
  assert.deepEqual(result.events.map(item => item.sequence), [1, 2, 3, 4]);
});

test("filters nested and flat event payloads and preserves selected visible sequence", () => {
  const result = buildTaskEventListViewModel({
    items: [event(1, "failed"), event(2, "running", true), event(3, "succeeded")],
    filter: "active",
    selectedSequence: 2
  });
  assert.deepEqual(result.events.map(item => item.sequence), [2]);
  assert.equal(result.selectedSequence, 2);
});

test("selects the latest priority event, otherwise the latest visible event", () => {
  const priority = buildTaskEventListViewModel({
    items: [event(1, "failed"), event(2, "succeeded"), event(3, "running")],
    filter: "all",
    selectedSequence: 99
  });
  assert.equal(priority.selectedSequence, 3);
  const fallback = buildTaskEventListViewModel({
    items: [event(1, "succeeded"), event(2, "skipped")],
    filter: "completed",
    selectedSequence: 99
  });
  assert.equal(fallback.selectedSequence, 2);
});

test("invalid filters fall back to all and empty results retain the previous selection", () => {
  const result = buildTaskEventListViewModel({
    items: [event(1, "failed")],
    filter: "unexpected",
    selectedSequence: 8
  });
  assert.equal(result.filter, "all");
  assert.deepEqual(result.events.map(item => item.sequence), [1]);
  assert.equal(result.selectedSequence, 1);
  const empty = buildTaskEventListViewModel({ items: [], filter: "issues", selectedSequence: 8 });
  assert.equal(empty.filter, "issues");
  assert.deepEqual(empty.events, []);
  assert.equal(empty.selectedSequence, 8);
});

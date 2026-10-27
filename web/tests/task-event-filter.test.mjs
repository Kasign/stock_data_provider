import test from "node:test";
import assert from "node:assert/strict";
import { taskEventMatchesFilter } from "../src/task-event-filter.js";

test("the all filter preserves every known and unknown event status", () => {
  for (const status of ["failed", "degraded", "running", "queued", "succeeded", "skipped", "unknown", "other"]) {
    assert.equal(taskEventMatchesFilter(status, "all"), true);
  }
});

test("the issue filter includes failures and degraded or partial outcomes only", () => {
  for (const status of ["failed", "error", "blocked", "rejected", "degraded", "partial"]) {
    assert.equal(taskEventMatchesFilter(status, "issues"), true, status);
  }
  for (const status of ["running", "queued", "succeeded", "skipped", "unknown", "other"]) {
    assert.equal(taskEventMatchesFilter(status, "issues"), false, status);
  }
});

test("the active filter includes running and queued lifecycle states", () => {
  for (const status of ["running", "started", "in-progress", "in_progress", "processing", "queued", "pending"]) {
    assert.equal(taskEventMatchesFilter(status, "active"), true, status);
  }
  for (const status of ["failed", "degraded", "succeeded", "skipped", "unknown"]) {
    assert.equal(taskEventMatchesFilter(status, "active"), false, status);
  }
});

test("the completed filter includes successful and skipped stages", () => {
  for (const status of ["succeeded", "success", "completed", "skipped"]) {
    assert.equal(taskEventMatchesFilter(status, "completed"), true, status);
  }
  for (const status of ["failed", "running", "queued", "unknown", "other"]) {
    assert.equal(taskEventMatchesFilter(status, "completed"), false, status);
  }
});

test("unknown filter values fail open to all events", () => {
  assert.equal(taskEventMatchesFilter("failed", "unexpected-filter"), true);
  assert.equal(taskEventMatchesFilter("unknown", "unexpected-filter"), true);
});

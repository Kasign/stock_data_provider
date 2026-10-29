import test from "node:test";
import assert from "node:assert/strict";
import { calendarResponseRangeError } from "../src/market-activity.js";

test("accepts calendar responses whose explicit date range matches the request", () => {
  assert.equal(
    calendarResponseRangeError(
      { start_date: "2026-10-01", end_date: "2026-10-31", items: [] },
      "2026-10-01",
      "2026-10-31"
    ),
    null
  );
});

test("keeps compatibility with older calendar responses that omit range metadata", () => {
  assert.equal(
    calendarResponseRangeError({ items: [] }, "2026-10-01", "2026-10-31"),
    null
  );
});

test("rejects an explicit response start date that differs from the current request", () => {
  assert.match(
    calendarResponseRangeError(
      { start_date: "2026-09-01", end_date: "2026-10-31" },
      "2026-10-01",
      "2026-10-31"
    ),
    /开始日期.*2026-09-01.*2026-10-01.*拒绝展示/
  );
});

test("rejects an explicit response end date that differs from the current request", () => {
  assert.match(
    calendarResponseRangeError(
      { start_date: "2026-10-01", end_date: "2026-09-30" },
      "2026-10-01",
      "2026-10-31"
    ),
    /结束日期.*2026-09-30.*2026-10-31.*拒绝展示/
  );
});

test("rejects explicitly empty or null range metadata instead of treating it as omitted", () => {
  assert.match(
    calendarResponseRangeError({ start_date: "", end_date: "2026-10-31" }, "2026-10-01", "2026-10-31"),
    /开始日期/
  );
  assert.match(
    calendarResponseRangeError({ start_date: null, end_date: "2026-10-31" }, "2026-10-01", "2026-10-31"),
    /开始日期.*null/
  );
});

test("normalizes newline and tab characters in malformed response metadata", () => {
  const message = calendarResponseRangeError(
    { start_date: "wrong\nrange\tvalue", end_date: "2026-10-31" },
    "2026-10-01",
    "2026-10-31"
  );
  assert.match(message, /wrong range value/);
  assert.doesNotMatch(message, /[\r\n\t]/);
});

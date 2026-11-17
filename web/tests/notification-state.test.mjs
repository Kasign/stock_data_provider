import test from "node:test";
import assert from "node:assert/strict";
import { resolveNotificationViewState } from "../src/notification-state.js";

test("notification state distinguishes unavailable, loading, error, empty and loaded", () => {
  assert.equal(resolveNotificationViewState({ supported: false, items: [] }), "unavailable");
  assert.equal(resolveNotificationViewState({ supported: true, loading: true, items: [] }), "loading");
  assert.equal(resolveNotificationViewState({ supported: true, error: new Error("offline"), items: [] }), "error");
  assert.equal(resolveNotificationViewState({ supported: true, items: [] }), "empty");
  assert.equal(resolveNotificationViewState({ supported: true, items: [{ id: "test-only" }] }), "loaded");
});

test("an unsupported API takes precedence over an empty array and never implies zero notifications", () => {
  assert.equal(resolveNotificationViewState({ supported: false, items: [] }), "unavailable");
  assert.equal(resolveNotificationViewState({ supported: true, items: null }), "error");
});

import test from "node:test";
import assert from "node:assert/strict";
import { resolveChatViewState } from "../src/chat-state.js";

test("chat state distinguishes unavailable, loading, error, empty and loaded", () => {
  assert.equal(resolveChatViewState({ supported: false, messages: [] }), "unavailable");
  assert.equal(resolveChatViewState({ supported: true, loading: true }), "loading");
  assert.equal(resolveChatViewState({ supported: true, error: new Error("offline") }), "error");
  assert.equal(resolveChatViewState({ supported: true, session: { id: "test-session" }, messages: [] }), "empty");
  assert.equal(resolveChatViewState({ supported: true, session: { id: "test-session" }, messages: [{ id: "test-message" }] }), "loaded");
});

test("an unavailable Chat API takes precedence over stale session data", () => {
  assert.equal(resolveChatViewState({ supported: false, session: { id: "old-session" }, messages: [{ id: "old-message" }] }), "unavailable");
  assert.equal(resolveChatViewState({ supported: true, session: { id: "test-session" }, messages: null }), "error");
});

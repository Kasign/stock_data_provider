import test from "node:test";
import assert from "node:assert/strict";
import {
  resolveScreeningCapability,
  resolveScreeningTaskState,
  resolveScreeningCollectionState
} from "../src/screening-state.js";

test("screening capability distinguishes unavailable, enabled, disabled, unknown, loading and error", () => {
  assert.equal(resolveScreeningCapability({ supported: false, enabled: false }), "unavailable");
  assert.equal(resolveScreeningCapability({ supported: true, enabled: true }), "enabled");
  assert.equal(resolveScreeningCapability({ supported: true, enabled: false }), "disabled");
  assert.equal(resolveScreeningCapability({ supported: true }), "unknown");
  assert.equal(resolveScreeningCapability({ supported: true, loading: true }), "loading");
  assert.equal(resolveScreeningCapability({ supported: true, error: new Error("network") }), "error");
});

test("screening task status maps only explicit server task states", () => {
  assert.equal(resolveScreeningTaskState({ supported: false, task: { status: "succeeded" } }), "unavailable");
  assert.equal(resolveScreeningTaskState({ supported: true }), "idle");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "queued" } }), "queued");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "running" } }), "running");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "succeeded" } }), "completed");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "partial" } }), "partial");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "failed" } }), "failed");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "invented" } }), "unknown");
});

test("candidate and history collections distinguish unavailable, error, empty and loaded", () => {
  assert.equal(resolveScreeningCollectionState({ supported: false, candidates: [] }), "unavailable");
  assert.equal(resolveScreeningCollectionState({ supported: true, loading: true, candidates: [] }), "loading");
  assert.equal(resolveScreeningCollectionState({ supported: true, error: new Error("offline") }), "error");
  assert.equal(resolveScreeningCollectionState({ supported: true, candidates: [] }), "empty");
  assert.equal(resolveScreeningCollectionState({ supported: true, candidates: [{ symbol: "test-only" }] }), "loaded");
  assert.equal(resolveScreeningCollectionState({ supported: true, items: [] }, "items"), "empty");
});

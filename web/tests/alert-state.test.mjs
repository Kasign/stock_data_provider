import test from "node:test";
import assert from "node:assert/strict";
import {
  resolveAlertCapability,
  resolveAlertEvaluationState,
  resolveAlertCollectionState
} from "../src/alert-state.js";

test("alert capability distinguishes unavailable, enabled, disabled, unknown, loading and error", () => {
  assert.equal(resolveAlertCapability({ supported: false, enabled: false }), "unavailable");
  assert.equal(resolveAlertCapability({ supported: true, enabled: true }), "enabled");
  assert.equal(resolveAlertCapability({ supported: true, enabled: false }), "disabled");
  assert.equal(resolveAlertCapability({ supported: true }), "unknown");
  assert.equal(resolveAlertCapability({ supported: true, loading: true }), "loading");
  assert.equal(resolveAlertCapability({ supported: true, error: new Error("network") }), "error");
});

test("alert evaluation separates triggered, not-triggered, evaluation-error, degraded and skipped", () => {
  assert.equal(resolveAlertEvaluationState({ supported: false, result: { status: "triggered" } }), "unavailable");
  assert.equal(resolveAlertEvaluationState({ supported: true }), "idle");
  for (const status of ["triggered", "not-triggered", "evaluation-error", "degraded", "skipped"]) {
    assert.equal(resolveAlertEvaluationState({ supported: true, result: { status } }), status);
  }
  assert.equal(resolveAlertEvaluationState({ supported: true, result: { status: "made-up" } }), "unknown");
  assert.equal(resolveAlertEvaluationState({ supported: true, error: new Error("offline") }), "error");
});

test("alert rules, trigger history and delivery attempts distinguish unavailable from empty and error", () => {
  assert.equal(resolveAlertCollectionState({ supported: false, rules: [] }, "rules"), "unavailable");
  assert.equal(resolveAlertCollectionState({ supported: true, rules: [] }, "rules"), "empty");
  assert.equal(resolveAlertCollectionState({ supported: true, rules: [{ id: "test-rule" }] }, "rules"), "loaded");
  assert.equal(resolveAlertCollectionState({ supported: true, error: new Error("offline") }, "items"), "error");
  assert.equal(resolveAlertCollectionState({ supported: true, attempts: [] }, "attempts"), "empty");
});

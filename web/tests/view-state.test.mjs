import test from "node:test";
import assert from "node:assert/strict";
import { resolveNotificationViewState } from "../src/notification-state.js";
import { resolveChatViewState } from "../src/chat-state.js";
import {
  resolveScreeningCapability,
  resolveScreeningTaskState,
  resolveScreeningCollectionState
} from "../src/screening-state.js";
import {
  resolveAlertCapability,
  resolveAlertEvaluationState,
  resolveAlertCollectionState
} from "../src/alert-state.js";

test("通知中心只把受支持且结构有效的回包识别为数据态", () => {
  assert.equal(resolveNotificationViewState({ supported: false, loading: true, items: [{}] }), "unavailable");
  assert.equal(resolveNotificationViewState({ supported: true, loading: true, error: "旧错误", items: [] }), "loading");
  assert.equal(resolveNotificationViewState({ supported: true, error: "读取失败", items: [] }), "error");
  assert.equal(resolveNotificationViewState({ supported: true, items: null }), "error");
  assert.equal(resolveNotificationViewState({ supported: true, items: [] }), "empty");
  assert.equal(resolveNotificationViewState({ supported: true, items: [{ id: "server-item" }] }), "loaded");
});

test("问股状态区分接口未接入、读取错误、空会话和真实消息", () => {
  assert.equal(resolveChatViewState({ supported: false, session: {}, messages: [{}] }), "unavailable");
  assert.equal(resolveChatViewState({ supported: true, loading: true, error: "旧错误" }), "loading");
  assert.equal(resolveChatViewState({ supported: true, error: "读取失败", session: {} }), "error");
  assert.equal(resolveChatViewState({ supported: true, session: null, messages: [] }), "empty");
  assert.equal(resolveChatViewState({ supported: true, session: { id: "session-1" }, messages: null }), "error");
  assert.equal(resolveChatViewState({ supported: true, session: { id: "session-1" }, messages: [] }), "empty");
  assert.equal(resolveChatViewState({ supported: true, session: { id: "session-1" }, messages: [{ id: "message-1" }] }), "loaded");
});

test("条件选股能力、任务和集合状态不会混淆未知、空结果与接口缺失", () => {
  assert.equal(resolveScreeningCapability({ supported: false, enabled: true }), "unavailable");
  assert.equal(resolveScreeningCapability({ supported: true, loading: true }), "loading");
  assert.equal(resolveScreeningCapability({ supported: true, error: "读取失败" }), "error");
  assert.equal(resolveScreeningCapability({ supported: true, enabled: true }), "enabled");
  assert.equal(resolveScreeningCapability({ supported: true, enabled: false }), "disabled");
  assert.equal(resolveScreeningCapability({ supported: true }), "unknown");

  assert.equal(resolveScreeningTaskState({ supported: false, task: { status: "running" } }), "unavailable");
  assert.equal(resolveScreeningTaskState({ supported: true }), "idle");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "queued" } }), "queued");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "running" } }), "running");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "succeeded" } }), "completed");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "partial" } }), "partial");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "failed" } }), "failed");
  assert.equal(resolveScreeningTaskState({ supported: true, task: { status: "invented" } }), "unknown");

  assert.equal(resolveScreeningCollectionState({ supported: true }), "error");
  assert.equal(resolveScreeningCollectionState({ supported: true, candidates: [] }), "empty");
  assert.equal(resolveScreeningCollectionState({ supported: true, candidates: [{ symbol: "600519.SH" }] }), "loaded");
  assert.equal(resolveScreeningCollectionState({ supported: false, candidates: [] }), "unavailable");
});

test("告警能力、评估与记录集合分别保留自己的状态语义", () => {
  assert.equal(resolveAlertCapability({ supported: false, enabled: true }), "unavailable");
  assert.equal(resolveAlertCapability({ supported: true, enabled: true }), "enabled");
  assert.equal(resolveAlertCapability({ supported: true, enabled: false }), "disabled");
  assert.equal(resolveAlertCapability({ supported: true }), "unknown");

  assert.equal(resolveAlertEvaluationState({ supported: false, result: { status: "triggered" } }), "unavailable");
  assert.equal(resolveAlertEvaluationState({ supported: true }), "idle");
  for (const status of ["triggered", "not-triggered", "evaluation-error", "degraded", "skipped"]) {
    assert.equal(resolveAlertEvaluationState({ supported: true, result: { status } }), status);
  }
  assert.equal(resolveAlertEvaluationState({ supported: true, result: { status: "invented" } }), "unknown");
  assert.equal(resolveAlertCollectionState({ supported: true }), "error");
  assert.equal(resolveAlertCollectionState({ supported: true, rules: [] }), "empty");
  assert.equal(resolveAlertCollectionState({ supported: true, rules: [{ id: "server-rule" }] }), "loaded");
  assert.equal(resolveAlertCollectionState({ supported: false, rules: [] }), "unavailable");
});

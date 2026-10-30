import test from "node:test";
import assert from "node:assert/strict";
import { buildScreeningDraftSummary, createScreeningCondition, deleteScreeningPlan, formatScreeningCondition, normalizeScreeningConditions, readScreeningPlans, saveScreeningPlan, SCREENING_PLANS_KEY } from "../src/screening-draft.js";
function makeStorage(seed = null) { let value = seed; return { getItem() { return value; }, setItem(_key, next) { value = String(next); }, removeItem() { value = null; } }; }
test("accepts bounded numeric conditions for supported market fields", () => {
  assert.deepEqual(createScreeningCondition("change_pct", "gte", "2.5"), { valid: true, condition: { field: "change_pct", operator: "gte", value: 2.5 } });
  assert.equal(createScreeningCondition("turnover_rate", "lte", "101").reason, "range");
  assert.equal(createScreeningCondition("unknown", "gte", "1").valid, false);
  assert.equal(createScreeningCondition("amount", "gte", "").valid, false);
});
test("normalizes conditions and describes AND/OR local drafts", () => {
  const list = normalizeScreeningConditions([{ field: "ma5", operator: "gte", value: 10 }, { field: "bad", operator: "gte", value: 2 }, { field: "ma20", operator: "gte", value: Infinity }]);
  assert.equal(list.length, 1);
  const summary = buildScreeningDraftSummary(list, "or");
  assert.equal(summary.count, 1); assert.equal(summary.logic, "or");
  assert.match(summary.conditions[0], /MA5 ≥ 10.00 元/); assert.match(summary.description, /OR/);
  assert.match(buildScreeningDraftSummary([]).description, /尚未添加/);
});
test("formats amount conditions with Chinese units", () => {
  assert.equal(formatScreeningCondition({ field: "amount", operator: "gte", value: 10000000 }), "成交额 ≥ 10,000,000 元");
});
test("saves, reloads, updates and deletes local screening plans", () => {
  const storage = makeStorage(), conditions = [{ field: "change_pct", operator: "gte", value: 2.5 }];
  const created = saveScreeningPlan({ name: "放量观察", conditions, logic: "and" }, storage, "2026-10-30T10:00:00Z");
  assert.equal(created.saved, true); assert.equal(readScreeningPlans(storage).plans[0].name, "放量观察");
  const updated = saveScreeningPlan({ id: created.plan.id, name: "趋势观察", conditions: [...conditions, { field: "ma5", operator: "gte", value: 10 }], logic: "or" }, storage, "2026-10-30T11:00:00Z");
  assert.equal(updated.saved, true); assert.equal(updated.plan.created_at, "2026-10-30T10:00:00Z"); assert.equal(updated.plan.logic, "or");
  assert.equal(deleteScreeningPlan(created.plan.id, storage).deleted, true); assert.deepEqual(readScreeningPlans(storage).plans, []);
});
test("rejects duplicate names, empty plans and malformed storage without overwriting", () => {
  const storage = makeStorage(), condition = [{ field: "ma20", operator: "lte", value: 30 }];
  assert.equal(saveScreeningPlan({ name: "趋势", conditions: condition }, storage).saved, true);
  assert.equal(saveScreeningPlan({ name: "趋势", conditions: condition }, storage).reason, "duplicate-name");
  assert.equal(saveScreeningPlan({ name: "空条件", conditions: [] }, storage).reason, "empty-conditions");
  const broken = makeStorage("{broken");
  assert.equal(readScreeningPlans(broken).valid, false);
  assert.equal(saveScreeningPlan({ name: "不能覆盖", conditions: condition }, broken).reason, "storage-invalid");
  assert.equal(broken.getItem(SCREENING_PLANS_KEY), "{broken");
});
test("reports unavailable storage and failed writes", () => {
  const blocked = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); } };
  assert.equal(readScreeningPlans(blocked).readable, false);
  assert.equal(saveScreeningPlan({ name: "x", conditions: [{ field: "ma5", operator: "gte", value: 1 }] }, blocked).saved, false);
  let raw = null; const silent = { getItem() { return raw; }, setItem() {}, removeItem() { raw = null; } };
  assert.equal(saveScreeningPlan({ name: "x", conditions: [{ field: "ma5", operator: "gte", value: 1 }] }, silent).saved, false);
});

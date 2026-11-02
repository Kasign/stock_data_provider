import test from "node:test";
import assert from "node:assert/strict";
import { buildScreeningDraftSummary, createScreeningCondition, deleteScreeningPlan, evaluateScreeningConditions, formatScreeningCondition, normalizeScreeningConditions, readScreeningPlans, saveScreeningPlan, SCREENING_PLANS_KEY } from "../src/screening-draft.js";
function makeStorage(seed = null) { let value = seed; return { getItem() { return value; }, setItem(_key, next) { value = String(next); }, removeItem() { value = null; } }; }
test("accepts bounded numeric conditions for supported market fields", () => {
  assert.deepEqual(createScreeningCondition("change_pct", "gte", "2.5"), { valid: true, condition: { field: "change_pct", operator: "gte", value: 2.5 } });
  assert.equal(createScreeningCondition("turnover_rate", "lte", "101").reason, "range");
  assert.equal(createScreeningCondition("unknown", "gte", "1").valid, false);
  assert.equal(createScreeningCondition("amount", "gte", "").valid, false);
  assert.equal(createScreeningCondition("amount", "gte", "   ").valid, false);
  assert.equal(createScreeningCondition("amount", "gte", null).valid, false);
});
test("validates the configured bounds for every supported screening field", () => {
  for (const [field, min, max] of [
    ["change_pct", -30, 30], ["turnover_rate", 0, 100], ["amount", 0, 100000000000],
    ["ma5", 0, 1000000], ["ma20", 0, 1000000]
  ]) {
    assert.equal(createScreeningCondition(field, "gte", String(min)).valid, true, field + " minimum");
    assert.equal(createScreeningCondition(field, "lte", String(max)).valid, true, field + " maximum");
    assert.equal(createScreeningCondition(field, "gte", String(min - 1)).reason, "range", field + " below minimum");
    assert.equal(createScreeningCondition(field, "lte", String(max + 1)).reason, "range", field + " above maximum");
  }
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
  const savedAs = saveScreeningPlan({ name: "另存方案", conditions, logic: "and" }, storage, "2026-10-30T12:00:00Z");
  assert.equal(savedAs.saved, true); assert.notEqual(savedAs.plan.id, created.plan.id);
  assert.equal(readScreeningPlans(storage).plans.length, 2);
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
test("rejects mixed invalid conditions instead of silently saving a partial plan", () => {
  const storage = makeStorage();
  const result = saveScreeningPlan({ name: "严格校验", conditions: [
    { field: "ma5", operator: "gte", value: 10 },
    { field: "unknown", operator: "gte", value: 1 }
  ] }, storage);
  assert.equal(result.reason, "invalid-conditions");
  assert.equal(storage.getItem(SCREENING_PLANS_KEY), null);
});

test("supports save-as, enforces the 30-plan limit and rejects duplicate stored names", () => {
  const storage = makeStorage();
  for (let index = 0; index < 30; index += 1) {
    assert.equal(saveScreeningPlan({ name: "方案 " + index, conditions: [{ field: "ma5", operator: "gte", value: index + 1 }] }, storage).saved, true);
  }
  assert.equal(saveScreeningPlan({ name: "第 31 个", conditions: [{ field: "ma5", operator: "gte", value: 1 }] }, storage).reason, "limit");
  const duplicateNames = [
    { id: "a", name: "同名", logic: "and", conditions: [{ field: "ma5", operator: "gte", value: 1 }], created_at: "2026-01-01", updated_at: "2026-01-01" },
    { id: "b", name: "同名", logic: "or", conditions: [{ field: "ma20", operator: "lte", value: 10 }], created_at: "2026-01-01", updated_at: "2026-01-01" }
  ];
  const broken = makeStorage(JSON.stringify(duplicateNames));
  assert.equal(readScreeningPlans(broken).valid, false);
});

test("reports unavailable storage and failed writes", () => {
  const blocked = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); } };
  assert.equal(readScreeningPlans(blocked).readable, false);
  assert.equal(saveScreeningPlan({ name: "x", conditions: [{ field: "ma5", operator: "gte", value: 1 }] }, blocked).saved, false);
  let raw = null; const silent = { getItem() { return raw; }, setItem() {}, removeItem() { raw = null; } };
  assert.equal(saveScreeningPlan({ name: "x", conditions: [{ field: "ma5", operator: "gte", value: 1 }] }, silent).saved, false);
});


test("filters only supplied page rows and preserves unknown values under AND/OR logic", () => {
  const rows = [
    { symbol: "A", quote: { change_percent: 3, amount: 20000000 }, ma5: 12 },
    { symbol: "B", quote: { change_percent: -1, amount: 5000000 }, ma5: 8 },
    { symbol: "C", quote: { change_percent: null, amount: 30000000 }, ma5: null }
  ];
  const conditions = [{ field: "change_pct", operator: "gte", value: 1 }, { field: "ma5", operator: "gte", value: 10 }];
  const andResult = evaluateScreeningConditions(rows, conditions, "and");
  assert.deepEqual(andResult.matchedItems.map(item => item.symbol), ["A"]);
  assert.deepEqual(andResult.unmatchedItems.map(item => item.symbol), ["B"]);
  assert.deepEqual(andResult.unknownItems.map(item => item.symbol), ["C"]);
  const orResult = evaluateScreeningConditions(rows, conditions, "or");
  assert.deepEqual(orResult.matchedItems.map(item => item.symbol), ["A"]);
  assert.deepEqual(orResult.unmatchedItems.map(item => item.symbol), ["B"]);
  assert.deepEqual(orResult.unknownItems.map(item => item.symbol), ["C"]);
  assert.equal(andResult.examinedCount, 3);
  assert.equal(andResult.conditionCount, 2);
  assert.deepEqual(rows.map(item => item.symbol), ["A", "B", "C"], "filtering must not mutate the loaded page");
});

test("OR logic can match on a known condition while another field is missing", () => {
  const result = evaluateScreeningConditions([{ symbol: "A", quote: { change_percent: 4 }, ma20: null }], [
    { field: "change_pct", operator: "gte", value: 2 }, { field: "ma20", operator: "gte", value: 10 }
  ], "or");
  assert.deepEqual(result.matchedItems.map(item => item.symbol), ["A"]);
  assert.equal(result.unknownItems.length, 0);
});

test("rejects an empty condition set instead of returning every page row", () => {
  const result = evaluateScreeningConditions([{ symbol: "A" }], [], "and");
  assert.equal(result.valid, false);
  assert.equal(result.reason, "empty-conditions");
  assert.equal(result.unknownItems.length, 1);
});

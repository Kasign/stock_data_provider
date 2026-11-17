import test from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_PREFERENCES, readPreferences, readPreferencesState, resetPreferences, savePreferences, PREFERENCES_KEY } from "../src/preferences.js";

test("blocked localStorage falls back to default preferences without crashing startup", () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get() { throw new Error("Storage access is blocked"); }
  });
  try {
    assert.deepEqual(readPreferences(), { ...DEFAULT_PREFERENCES });
    assert.throws(() => savePreferences(DEFAULT_PREFERENCES), /浏览器本地存储不可用/);
  } finally {
    if (previous) Object.defineProperty(globalThis, "localStorage", previous);
    else delete globalThis.localStorage;
  }
});


test("preference writes that throw or are silently ignored are reported", () => {
  const throwingStorage = {
    getItem() { return null; },
    setItem() { throw new DOMException("quota exceeded", "QuotaExceededError"); }
  };
  assert.throws(() => savePreferences(DEFAULT_PREFERENCES, throwingStorage), /quota exceeded/);

  const ignoredStorage = {
    getItem() { return null; },
    setItem() {}
  };
  assert.throws(() => savePreferences(DEFAULT_PREFERENCES, ignoredStorage), /偏好设置未能保存/);
});

test("malformed preference data is preserved until the user explicitly resets it", () => {
  let raw = "{invalid-json";
  const storage = {
    getItem(key) { return key === PREFERENCES_KEY ? raw : null; },
    setItem(key, value) { if (key === PREFERENCES_KEY) raw = String(value); }
  };
  assert.deepEqual(readPreferencesState(storage), { readable: true, valid: false, value: null, reason: "invalid" });
  assert.deepEqual(readPreferences(storage), { ...DEFAULT_PREFERENCES });
  assert.throws(() => savePreferences({ ...DEFAULT_PREFERENCES, priceDigits: 3 }, storage), /原始数据未覆盖/);
  assert.equal(raw, "{invalid-json");
  assert.deepEqual(resetPreferences(storage), { ...DEFAULT_PREFERENCES });
  assert.equal(readPreferencesState(storage).valid, true);
  assert.deepEqual(JSON.parse(raw), { ...DEFAULT_PREFERENCES });
});


test("导入偏好规范化保留完整字段并拒绝未知字段", async () => {
  const { normalizeImportedPreferences } = await import("../src/preferences.js");
  const value = normalizeImportedPreferences({
    watchlistSort: "name",
    watchlistSortDirection: "asc",
    watchlistDisplayFields: ["quote", "today"],
    watchlistDisplayOrder: ["result", "task", "today", "quote", "updated"],
    marketColorScheme: "reverse",
    priceDigits: 3,
    percentDigits: 2,
    amountDisplayUnit: "raw",
    dailyBarLimit: "250",
    dailyAdjustment: "none"
  });
  assert.deepEqual(value, {
    watchlistSort: "name",
    watchlistSortDirection: "asc",
    watchlistDisplayFields: ["quote", "today"],
    watchlistDisplayOrder: ["result", "task", "today", "quote", "updated"],
    marketColorScheme: "reverse",
    priceDigits: 3,
    percentDigits: 2,
    amountDisplayUnit: "raw",
    dailyBarLimit: 250,
    dailyAdjustment: "none"
  });
  assert.throws(() => normalizeImportedPreferences({ unexpected: true }), /不支持的字段/);
});

test("导入偏好规范化对空对象补齐默认值且不共享数组引用", async () => {
  const { normalizeImportedPreferences } = await import("../src/preferences.js");
  const first = normalizeImportedPreferences({});
  const second = normalizeImportedPreferences({});
  assert.deepEqual(first, { ...DEFAULT_PREFERENCES });
  assert.notEqual(first.watchlistDisplayFields, second.watchlistDisplayFields);
  assert.notEqual(first.watchlistDisplayOrder, second.watchlistDisplayOrder);
});

test("导入偏好规范化拒绝越界值、非法字段和重复顺序", async () => {
  const { normalizeImportedPreferences } = await import("../src/preferences.js");
  const invalid = [
    [{ watchlistSort: "amount" }, /默认排序字段无效/],
    [{ watchlistSortDirection: "sideways" }, /排序方向无效/],
    [{ watchlistDisplayFields: "quote" }, /展示字段配置无效/],
    [{ watchlistDisplayFields: ["fake"] }, /展示字段配置无效/],
    [{ watchlistDisplayOrder: ["updated", "quote", "today", "task", "task"] }, /字段顺序配置无效/],
    [{ marketColorScheme: "blue" }, /涨跌颜色方案无效/],
    [{ priceDigits: 4 }, /价格精度无效/],
    [{ percentDigits: 4 }, /涨跌幅精度无效/],
    [{ amountDisplayUnit: "million" }, /成交额单位显示无效/],
    [{ dailyBarLimit: 90 }, /默认日 K 条数无效/],
    [{ dailyAdjustment: "split" }, /默认复权方式无效/]
  ];
  for (const [value, message] of invalid) assert.throws(() => normalizeImportedPreferences(value), message);
  assert.throws(() => normalizeImportedPreferences(null), /必须是对象/);
  assert.throws(() => normalizeImportedPreferences([]), /必须是对象/);
});

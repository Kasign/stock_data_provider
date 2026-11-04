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

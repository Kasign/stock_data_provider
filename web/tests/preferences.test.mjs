import test from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_PREFERENCES, readPreferences, savePreferences } from "../src/preferences.js";

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

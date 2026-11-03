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

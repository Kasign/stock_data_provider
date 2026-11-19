import test from "node:test";
import assert from "node:assert/strict";
import { clearSettings, loadSettings, saveSettings, SETTINGS_KEY } from "../src/api.js";

function withLocalStorage(storage, callback) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true, enumerable: true, writable: true, value: storage
  });
  try { return callback(); }
  finally {
    if (previous) Object.defineProperty(globalThis, "localStorage", previous);
    else delete globalThis.localStorage;
  }
}

function memoryStorage(initial = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { values.set(key, String(value)); },
    removeItem(key) { values.delete(key); }
  };
}

test("loadSettings removes legacy Data API Key and split upstream paths", () => {
  withLocalStorage(memoryStorage({
    [SETTINGS_KEY]: JSON.stringify({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "legacy-secret"
    })
  }), () => {
    assert.deepEqual(loadSettings(), { marketBase: "/", analysisBase: "/" });
    assert.equal(localStorage.getItem(SETTINGS_KEY), JSON.stringify({ marketBase: "/", analysisBase: "/" }));
    assert.doesNotMatch(localStorage.getItem(SETTINGS_KEY), /legacy-secret|apiKey|market-api|analysis-api/);
  });
});

test("saveSettings always persists the single Root Server API base", () => {
  withLocalStorage(memoryStorage(), () => {
    const settings = saveSettings({
      marketBase: "/old-market", analysisBase: "/old-analysis", apiKey: "must-not-persist"
    });
    assert.deepEqual(settings, { marketBase: "/", analysisBase: "/" });
    assert.equal(localStorage.getItem(SETTINGS_KEY), JSON.stringify(settings));
    assert.deepEqual(loadSettings(), settings);
  });
});

test("saveSettings rejects storage adapters that silently ignore writes", () => {
  withLocalStorage({ getItem() { return null; }, setItem() {}, removeItem() {} }, () => {
    assert.throws(() => saveSettings({}), /Root Server API 配置未能保存/);
  });
});

test("clearSettings rejects storage adapters that silently ignore removals", () => {
  withLocalStorage({
    getItem() { return JSON.stringify({ marketBase: "/custom-market" }); },
    setItem() {}, removeItem() {}
  }, () => {
    assert.throws(() => clearSettings(), /未能确认本地连接设置已清除/);
  });
});

test("saveSettings and clearSettings propagate browser storage exceptions", () => {
  withLocalStorage({
    getItem() { throw new DOMException("denied", "SecurityError"); },
    setItem() { throw new DOMException("denied", "SecurityError"); },
    removeItem() { throw new DOMException("denied", "SecurityError"); }
  }, () => {
    assert.throws(() => saveSettings({}), /denied/);
    assert.throws(() => clearSettings(), /denied/);
  });
});

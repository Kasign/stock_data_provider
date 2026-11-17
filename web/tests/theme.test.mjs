import test from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULT_THEME,
  THEME_STORAGE_KEY,
  applyTheme,
  normalizeTheme,
  persistTheme,
  readTheme
} from "../src/theme.js";

function memoryStorage(initial = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { values.set(key, String(value)); },
    removeItem(key) { values.delete(key); }
  };
}

test("theme accepts only the supported light and dark values", () => {
  assert.equal(normalizeTheme("light"), "light");
  assert.equal(normalizeTheme("dark"), "dark");
  assert.equal(normalizeTheme("sepia"), DEFAULT_THEME);
  assert.equal(normalizeTheme(null), DEFAULT_THEME);
});

test("theme preference reads and persists without involving API settings", () => {
  const storage = memoryStorage();
  assert.equal(readTheme(storage), "dark");
  assert.deepEqual(persistTheme("light", storage), { theme: "light", persisted: true });
  assert.equal(readTheme(storage), "light");
  assert.equal(storage.getItem(THEME_STORAGE_KEY), "light");
  storage.setItem(THEME_STORAGE_KEY, "not-a-theme");
  assert.equal(readTheme(storage), "dark");
});

test("theme remains usable if browser storage throws and reports the failed persistence", () => {
  const storage = {
    getItem() { throw new Error("storage denied"); },
    setItem() { throw new Error("storage full"); }
  };
  assert.equal(readTheme(storage), "dark");
  assert.deepEqual(persistTheme("light", storage), { theme: "light", persisted: false });
});

test("theme detects storage adapters that silently ignore writes", () => {
  const storage = {
    getItem() { return null; },
    setItem() {},
    removeItem() {}
  };
  assert.deepEqual(persistTheme("light", storage), { theme: "light", persisted: false });
});

test("applyTheme changes document theme metadata without a browser dependency", () => {
  const root = {
    dataset: {},
    style: { setProperty(name, value) { this[name] = value; } }
  };
  assert.equal(applyTheme("light", root), "light");
  assert.equal(root.dataset.theme, "light");
  assert.equal(root.style["color-scheme"], "light");
  assert.equal(applyTheme("invalid", root), "dark");
  assert.equal(root.dataset.theme, "dark");
});

import test from "node:test";
import assert from "node:assert/strict";
import {
  formatPercent,
  formatPrice,
  normalizeBasePath,
  normalizeSymbol,
  parseSymbols,
  qualityLabel,
  taskStatusLabel
} from "../src/format.js";

test("normalizes same-origin base paths and rejects remote origins", () => {
  assert.equal(normalizeBasePath(" /market-api/// ", "/fallback"), "/market-api");
  assert.equal(normalizeBasePath("", "/fallback"), "/fallback");
  assert.throws(() => normalizeBasePath("https://example.com", "/fallback"), /同源/);
  assert.throws(() => normalizeBasePath("//evil.example", "/fallback"), /同源/);
});

test("accepts plain six digit symbols and exchange-qualified symbols", () => {
  assert.equal(normalizeSymbol(" 600519 "), "600519");
  assert.equal(normalizeSymbol("000001.sz"), "000001.SZ");
  assert.throws(() => normalizeSymbol("60051"), /格式不正确/);
});

test("parses lists, removes duplicates and caps the payload size", () => {
  assert.deepEqual(parseSymbols("600519, 000001，600519"), ["600519", "000001"]);
  assert.throws(() => parseSymbols(Array.from({ length: 101 }, (_, index) => String(index + 100000).slice(-6)).join(",")), /最多分析 100/);
});

test("formats optional numeric values without converting missing data into zero", () => {
  assert.equal(formatPrice(null), "—");
  assert.equal(formatPercent(undefined), "—");
  assert.equal(formatPercent(1.25), "+1.25%");
  assert.equal(formatPercent(-0.5), "-0.50%");
});

test("maps quality and task statuses to Chinese labels", () => {
  assert.equal(qualityLabel("degraded"), "质量降级");
  assert.equal(taskStatusLabel("queued"), "排队中");
});

import test from "node:test";
import assert from "node:assert/strict";
import {
  formatPercent,
  formatPrice,
  normalizeAnalysisSymbol,
  normalizeBasePath,
  normalizeSymbol,
  parseAnalysisSymbols,
  parseSymbols,
  qualityLabel,
  runStatusLabel,
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


test("normalizes symbols for the current Go analysis contract without changing quote symbols", () => {
  assert.equal(normalizeAnalysisSymbol("600519.SH"), "600519");
  assert.equal(normalizeAnalysisSymbol("000001.sz"), "000001");
  assert.deepEqual(parseAnalysisSymbols("600519.SH, 600519, 000001.SZ"), ["600519", "000001"]);
  assert.equal(normalizeSymbol("600519.SH"), "600519.SH");
  assert.throws(() => normalizeAnalysisSymbol("ABCDEF"), /格式不正确/);
});

test("maps Go RunResult statuses separately from asynchronous task statuses", () => {
  assert.equal(runStatusLabel("completed"), "分析完成");
  assert.equal(runStatusLabel("blocked_data_quality"), "数据质量阻断");
  assert.equal(runStatusLabel("rejected_request"), "请求被拒绝");
  assert.equal(taskStatusLabel("succeeded"), "执行成功");
});

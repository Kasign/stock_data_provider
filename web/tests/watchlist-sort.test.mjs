import test from "node:test";
import assert from "node:assert/strict";
import { sortWatchlistItems } from "../src/watchlist-sort.js";

const records = [
  { symbol: "000002.SZ", name: "万科A", updated_at: "2026-10-29T09:00:00Z" },
  { symbol: "600519.SH", name: "贵州茅台", updated_at: "2026-10-30T09:00:00Z" },
  { symbol: "000858.SZ", name: "五粮液", updated_at: "2026-10-28T09:00:00Z" }
];
const quotes = new Map([
  ["000002.SZ", { current_price: 8, change_percent: -1.2, amount: 900, turnover_rate: 2.1 }],
  ["600519.SH", { current_price: 1500, change_percent: 0.5, amount: 1200, turnover_rate: 0.3 }],
  ["000858.SZ", { current_price: 120, change_percent: 2.4, amount: 1500, turnover_rate: 1.1 }]
]);
const getQuote = (item) => quotes.get(item.symbol) || null;

test("行情字段支持升降序，且不修改输入数组", () => {
  const ascending = sortWatchlistItems(records, { mode: "price", direction: "asc", getQuote });
  const descending = sortWatchlistItems(records, { mode: "change", direction: "desc", getQuote });
  assert.deepEqual(ascending.map((item) => item.symbol), ["000002.SZ", "000858.SZ", "600519.SH"]);
  assert.deepEqual(descending.map((item) => item.symbol), ["000858.SZ", "600519.SH", "000002.SZ"]);
  assert.deepEqual(records.map((item) => item.symbol), ["000002.SZ", "600519.SH", "000858.SZ"]);
});

test("成交额和换手率使用真实数值排序", () => {
  assert.deepEqual(sortWatchlistItems(records, { mode: "amount", direction: "desc", getQuote }).map((item) => item.symbol), ["000858.SZ", "600519.SH", "000002.SZ"]);
  assert.deepEqual(sortWatchlistItems(records, { mode: "turnover", direction: "asc", getQuote }).map((item) => item.symbol), ["600519.SH", "000858.SZ", "000002.SZ"]);
});

test("缺失行情和无效字段始终排在末尾，未知不当作零", () => {
  const getPartialQuote = (item) => item.symbol === "600519.SH" ? { current_price: 10 } : item.symbol === "000002.SZ" ? { current_price: null } : null;
  assert.deepEqual(sortWatchlistItems(records, { mode: "price", direction: "desc", getQuote: getPartialQuote }).map((item) => item.symbol), ["600519.SH", "000002.SZ", "000858.SZ"]);
  assert.deepEqual(sortWatchlistItems(records, { mode: "price", direction: "asc", getQuote: getPartialQuote }).map((item) => item.symbol), ["600519.SH", "000002.SZ", "000858.SZ"]);
});

test("基础排序保持最近更新、代码和名称语义", () => {
  assert.deepEqual(sortWatchlistItems(records, { mode: "updated", direction: "desc" }).map((item) => item.symbol), ["600519.SH", "000002.SZ", "000858.SZ"]);
  assert.deepEqual(sortWatchlistItems(records, { mode: "name", direction: "asc" }).map((item) => item.symbol), ["600519.SH", "000002.SZ", "000858.SZ"]);
});

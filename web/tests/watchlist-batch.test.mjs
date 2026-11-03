import test from "node:test";
import assert from "node:assert/strict";
import { addScreeningResultsToWatchlist } from "../src/watchlist-batch.js";
import { addWatchlistSymbol, MAX_WATCHLIST_ITEMS, readWatchlistState, WATCHLIST_KEY } from "../src/watchlist.js";
import { createWatchlistGroup, readWatchlistGroups, WATCHLIST_GROUPS_KEY } from "../src/watchlist-groups.js";

function storage(seed = {}) {
  const values = new Map(Object.entries(seed));
  return { getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { values.set(key, String(value)); }, removeItem(key) { values.delete(key); } };
}

test("batch add preserves existing groups and skips duplicates by default", () => {
  const db = storage(); addWatchlistSymbol("600519.SH", "我的别名", db); createWatchlistGroup("长期观察", db);
  const groups = readWatchlistGroups(db); groups.assignments["600519.SH"] = "长期观察";
  db.setItem(WATCHLIST_GROUPS_KEY, JSON.stringify(groups));
  const result = addScreeningResultsToWatchlist([{ symbol: "600519", name: "贵州茅台" }, { symbol: "000001.SZ", name: "平安银行" }], db);
  assert.equal(result.persisted, true); assert.equal(result.added.length, 1); assert.deepEqual(result.duplicates, ["600519"]);
  assert.equal(readWatchlistGroups(db).assignments["600519.SH"], "长期观察"); assert.equal(readWatchlistState(db).items.length, 2);
});

test("explicit target group moves both new and existing selected stocks", () => {
  const db = storage(); addWatchlistSymbol("600519.SH", "贵州茅台", db); createWatchlistGroup("重点观察", db);
  const result = addScreeningResultsToWatchlist([{ symbol: "600519.SH" }, { symbol: "000001.SZ" }], db, "重点观察");
  assert.equal(result.persisted, true); assert.equal(result.assignedCount, 2);
  assert.equal(readWatchlistGroups(db).assignments["600519.SH"], "重点观察");
  assert.equal(readWatchlistGroups(db).assignments["000001.SZ"], "重点观察");
});

test("invalid group rejects before modifying the watchlist", () => {
  const db = storage(), before = db.getItem(WATCHLIST_KEY);
  const result = addScreeningResultsToWatchlist([{ symbol: "600519.SH" }], db, "不存在");
  assert.equal(result.persisted, false); assert.equal(db.getItem(WATCHLIST_KEY), before);
});

test("group write failure rolls back the watchlist write", () => {
  const values = new Map(); let failOnce = false;
  const db = { getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { if (failOnce && key === WATCHLIST_GROUPS_KEY && String(value).includes("重点观察")) { failOnce = false; throw new Error("quota"); } values.set(key, String(value)); },
    removeItem(key) { values.delete(key); } };
  createWatchlistGroup("重点观察", db); failOnce = true;
  const before = db.getItem(WATCHLIST_KEY);
  const result = addScreeningResultsToWatchlist([{ symbol: "600519.SH" }], db, "重点观察");
  assert.equal(result.persisted, false); assert.equal(result.rollbackSucceeded, true); assert.equal(db.getItem(WATCHLIST_KEY), before);
});

test("容量已满时只给仍在自选股中的已选股票分组", () => {
  const db = storage();
  createWatchlistGroup("重点观察", db);
  const full = Array.from({ length: MAX_WATCHLIST_ITEMS }, (_, index) => ({
    symbol: String(index + 1).padStart(6, "0"), name: "股票" + index, updated_at: ""
  }));
  db.setItem(WATCHLIST_KEY, JSON.stringify(full));
  const result = addScreeningResultsToWatchlist([{ symbol: "000001" }, { symbol: "999999.SZ" }], db, "重点观察");
  assert.equal(result.persisted, true);
  assert.equal(result.duplicates.length, 1);
  assert.equal(result.capacitySkipped, 1);
  assert.equal(result.assignedCount, 1);
  assert.equal(readWatchlistGroups(db).assignments["000001.SZ"], "重点观察");
  assert.equal(readWatchlistGroups(db).assignments["999999.SZ"], undefined);
  assert.equal(readWatchlistState(db).items.length, MAX_WATCHLIST_ITEMS);
});

test("全部新股票均因容量不足跳过时不创建孤立分组归属", () => {
  const db = storage();
  createWatchlistGroup("重点观察", db);
  const full = Array.from({ length: MAX_WATCHLIST_ITEMS }, (_, index) => ({
    symbol: String(index + 1).padStart(6, "0"), name: "股票" + index, updated_at: ""
  }));
  db.setItem(WATCHLIST_KEY, JSON.stringify(full));
  const beforeGroups = db.getItem(WATCHLIST_GROUPS_KEY);
  const result = addScreeningResultsToWatchlist([{ symbol: "999999.SZ" }], db, "重点观察");
  assert.equal(result.persisted, true);
  assert.equal(result.capacitySkipped, 1);
  assert.equal(result.assignedCount, 0);
  assert.equal(db.getItem(WATCHLIST_GROUPS_KEY), beforeGroups);
  assert.equal(readWatchlistGroups(db).assignments["999999.SZ"], undefined);
});

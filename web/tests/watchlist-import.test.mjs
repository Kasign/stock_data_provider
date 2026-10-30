import test from "node:test";
import assert from "node:assert/strict";
import { importWatchlistSymbols, MAX_WATCHLIST_ITEMS, readWatchlist, WATCHLIST_KEY } from "../src/watchlist.js";

function makeStorage(seed = "[]") {
  let value = seed;
  return {
    getItem() { return value; },
    setItem(_key, next) { value = String(next); },
    raw() { return value; }
  };
}

test("imports unique normalized records in one verified write and respects the cap", () => {
  const storage = makeStorage(JSON.stringify([{ symbol: "600519.SH", name: "贵州茅台", updated_at: "old" }]));
  const records = [
    { symbol: "600519", name: "duplicate" },
    { symbol: "000858.SZ", name: "五粮液" },
    { symbol: "300750", name: "宁德时代" }
  ];
  const result = importWatchlistSymbols(records, storage, "2026-10-30T00:00:00Z");
  assert.equal(result.persisted, true);
  assert.equal(result.imported.length, 2);
  assert.deepEqual(readWatchlist(storage).map((item) => item.symbol), ["600519.SH", "000858.SZ", "300750"]);
  assert.equal(readWatchlist(storage)[1].updated_at, "2026-10-30T00:00:00Z");
});

test("does not partially rewrite invalid local storage", () => {
  const original = "{broken";
  const storage = makeStorage(original);
  const result = importWatchlistSymbols([{ symbol: "600519.SH" }], storage);
  assert.equal(result.persisted, false);
  assert.equal(result.storageValid, false);
  assert.equal(storage.raw(), original);
});

test("stops at the maximum and reports unimported rows", () => {
  const existing = Array.from({ length: MAX_WATCHLIST_ITEMS - 1 }, (_, index) => ({
    symbol: String(index).padStart(6, "0"), name: "stock-" + index, updated_at: ""
  }));
  const storage = makeStorage(JSON.stringify(existing));
  const result = importWatchlistSymbols([{ symbol: "900001.SH" }, { symbol: "900002.SH" }], storage);
  assert.equal(result.imported.length, 1);
  assert.equal(result.skipped, 1);
  assert.equal(readWatchlist(storage).length, MAX_WATCHLIST_ITEMS);
});

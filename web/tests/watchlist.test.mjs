import test from "node:test";
import assert from "node:assert/strict";
import {
  addWatchlistSymbol,
  MAX_WATCHLIST_ITEMS,
  readWatchlist,
  removeWatchlistSymbol,
  renameWatchlistSymbol,
  WATCHLIST_KEY
} from "../src/watchlist.js";

function makeStorage(seed = {}) {
  const values = new Map(Object.entries(seed));
  return {
    getItem(key) { return values.has(key) ? values.get(key) : null; },
    setItem(key, value) { values.set(key, String(value)); }
  };
}

test("adds normalized stock symbols, deduplicates and keeps the most recently used first", () => {
  const storage = makeStorage();
  const first = addWatchlistSymbol(" 600519.sh ", "贵州茅台", storage, "2026-10-23T01:00:00Z");
  const second = addWatchlistSymbol("000001.SZ", "平安银行", storage, "2026-10-23T02:00:00Z");
  const duplicate = addWatchlistSymbol("600519.SH", "new alias", storage, "2026-10-23T03:00:00Z");

  assert.equal(first.added, true);
  assert.equal(second.added, true);
  assert.equal(duplicate.added, false);
  assert.deepEqual(duplicate.items.map((item) => item.symbol), ["600519.SH", "000001.SZ"]);
  assert.equal(duplicate.items[0].name, "贵州茅台", "adding a duplicate must preserve the user's alias");
  assert.equal(duplicate.items[0].updated_at, "2026-10-23T03:00:00Z");
});

test("rejects malformed symbols and sanitizes optional names", () => {
  const storage = makeStorage();
  assert.equal(addWatchlistSymbol("60051", "bad", storage).valid, false);
  assert.equal(addWatchlistSymbol("ABCDEF", "bad", storage).valid, false);
  const added = addWatchlistSymbol("300750.SZ", "  宁德\u0000时代  ", storage, "2026-10-23T00:00:00Z");
  assert.equal(added.valid, true);
  assert.equal(added.items[0].name, "宁德时代");
  assert.equal(addWatchlistSymbol("600001.SH", "x".repeat(50), storage).items[1].name.length, 32);
});

test("renames and removes entries without retaining unrelated properties", () => {
  const storage = makeStorage();
  storage.setItem(WATCHLIST_KEY, JSON.stringify([{
    symbol: "600519.SH",
    name: "贵州茅台",
    updated_at: "2026-10-20T00:00:00Z",
    result: { quality: { core_pass: true } },
    api_key: "must-not-persist"
  }]));

  const renamed = renameWatchlistSymbol("600519.SH", "  核心观察  ", storage, "2026-10-23T04:00:00Z");
  assert.equal(renamed[0].name, "核心观察");
  assert.deepEqual(Object.keys(renamed[0]).sort(), ["name", "symbol", "updated_at"]);
  assert.doesNotMatch(storage.getItem(WATCHLIST_KEY), /must-not-persist|core_pass|result|api_key/);

  const removed = removeWatchlistSymbol("600519.SH", storage);
  assert.deepEqual(removed, []);
  assert.deepEqual(readWatchlist(storage), []);
});

test("caps list size, skips malformed persisted entries and tolerates unavailable storage", () => {
  const storage = makeStorage();
  for (let index = 0; index < MAX_WATCHLIST_ITEMS + 7; index++) {
    addWatchlistSymbol(String(index).padStart(6, "0"), "stock-" + index, storage, "2026-10-23T00:00:00Z");
  }
  assert.equal(readWatchlist(storage).length, MAX_WATCHLIST_ITEMS);
  assert.deepEqual(readWatchlist(storage, 0), []);

  storage.setItem(WATCHLIST_KEY, JSON.stringify([{ symbol: "bad-symbol" }, { symbol: "600519.SH", name: "valid" }]));
  assert.deepEqual(readWatchlist(storage), [{ symbol: "600519.SH", name: "valid", updated_at: "" }]);

  const brokenStorage = { getItem() { throw new Error("storage disabled"); }, setItem() { throw new Error("storage disabled"); } };
  assert.deepEqual(readWatchlist(brokenStorage), []);
  assert.doesNotThrow(() => addWatchlistSymbol("600519.SH", "贵州茅台", brokenStorage));
  assert.doesNotThrow(() => removeWatchlistSymbol("600519.SH", brokenStorage));
});

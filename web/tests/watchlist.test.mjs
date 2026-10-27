import test from "node:test";
import assert from "node:assert/strict";
import {
  addWatchlistSymbol,
  MAX_WATCHLIST_ITEMS,
  readWatchlist,
  readWatchlistState,
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
  const longAlias = addWatchlistSymbol("600001.SH", "x".repeat(50), storage);
  assert.equal(longAlias.items.find((item) => item.symbol === "600001.SH").name.length, 32);
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
  assert.equal(renamed.valid, true);
  assert.equal(renamed.persisted, true);
  assert.equal(renamed.items[0].name, "核心观察");
  assert.deepEqual(Object.keys(renamed.items[0]).sort(), ["name", "symbol", "updated_at"]);
  assert.doesNotMatch(storage.getItem(WATCHLIST_KEY), /must-not-persist|core_pass|result|api_key/);

  const removed = removeWatchlistSymbol("600519.SH", storage);
  assert.equal(removed.valid, true);
  assert.equal(removed.removed, true);
  assert.equal(removed.persisted, true);
  assert.deepEqual(removed.items, []);
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

  let writes = 0;
  const brokenStorage = {
    getItem() { throw new Error("storage disabled"); },
    setItem() { writes += 1; throw new Error("storage disabled"); }
  };
  assert.deepEqual(readWatchlist(brokenStorage), []);
  assert.deepEqual(readWatchlistState(brokenStorage), {
    items: [], storageReadable: false, storageValid: false, reason: "unavailable"
  });
  const blockedAdd = addWatchlistSymbol("600519.SH", "贵州茅台", brokenStorage);
  assert.equal(blockedAdd.valid, true);
  assert.equal(blockedAdd.persisted, false);
  assert.equal(blockedAdd.storageReadable, false);
  const blockedRename = renameWatchlistSymbol("600519.SH", "贵州茅台", brokenStorage);
  assert.equal(blockedRename.persisted, false);
  assert.equal(blockedRename.storageReadable, false);
  const blockedRemove = removeWatchlistSymbol("600519.SH", brokenStorage);
  assert.equal(blockedRemove.persisted, false);
  assert.equal(blockedRemove.storageReadable, false);
  assert.equal(writes, 0, "mutations must never write after an unsuccessful read");
});

test("reports persistence failures without claiming browser data was saved", () => {
  const seed = JSON.stringify([{ symbol: "600519.SH", name: "贵州茅台", updated_at: "2026-10-20T00:00:00Z" }]);
  const storage = {
    getItem() { return seed; },
    setItem() { throw new Error("quota exceeded"); }
  };

  const added = addWatchlistSymbol("000001.SZ", "平安银行", storage, "2026-10-23T05:00:00Z");
  assert.equal(added.valid, true);
  assert.equal(added.persisted, false);
  assert.deepEqual(added.items.map((item) => item.symbol), ["000001.SZ", "600519.SH"]);

  const renamed = renameWatchlistSymbol("600519.SH", "核心观察", storage, "2026-10-23T06:00:00Z");
  assert.equal(renamed.valid, true);
  assert.equal(renamed.persisted, false);
  assert.equal(renamed.items[0].name, "核心观察");

  const removed = removeWatchlistSymbol("600519.SH", storage);
  assert.equal(removed.valid, true);
  assert.equal(removed.removed, true);
  assert.equal(removed.persisted, false);
  assert.deepEqual(removed.items.map((item) => item.symbol), []);
});

test("does not overwrite malformed stored data when reading or parsing fails", () => {
  let writes = 0;
  const storage = {
    getItem() { return "{malformed"; },
    setItem() { writes += 1; }
  };

  const state = readWatchlistState(storage);
  assert.equal(state.storageReadable, true);
  assert.equal(state.storageValid, false);
  assert.equal(state.reason, "invalid");

  const added = addWatchlistSymbol("600519.SH", "贵州茅台", storage);
  assert.equal(added.valid, true);
  assert.equal(added.persisted, false);
  assert.equal(added.storageValid, false);

  const renamed = renameWatchlistSymbol("600519.SH", "核心观察", storage);
  assert.equal(renamed.persisted, false);
  assert.equal(renamed.storageValid, false);

  const removed = removeWatchlistSymbol("600519.SH", storage);
  assert.equal(removed.persisted, false);
  assert.equal(removed.storageValid, false);
  assert.equal(writes, 0, "malformed data must not be replaced with an empty or partial list");
});

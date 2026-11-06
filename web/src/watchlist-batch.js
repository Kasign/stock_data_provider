import { addWatchlistSymbols, readWatchlistState, WATCHLIST_KEY } from "./watchlist.js";
import { comparableMarketSymbol } from "./format.js";
import { assignWatchlistGroups, readWatchlistGroups, WATCHLIST_GROUPS_KEY } from "./watchlist-groups.js";

function restoreRaw(storage, key, value) {
  try {
    if (value === null || value === undefined) storage.removeItem(key);
    else storage.setItem(key, value);
    return storage.getItem(key) === value;
  } catch { return false; }
}

export function addScreeningResultsToWatchlist(records, storage, targetGroup = null, updatedAt = new Date().toISOString()) {
  let previousWatchlist, previousGroups;
  try {
    previousWatchlist = storage.getItem(WATCHLIST_KEY);
    if (targetGroup !== null) previousGroups = storage.getItem(WATCHLIST_GROUPS_KEY);
  } catch {
    return { persisted: false, storageReadable: false, storageValid: false, added: [], duplicates: [], invalid: [], capacitySkipped: 0 };
  }
  const current = readWatchlistState(storage);
  if (!current.storageReadable || !current.storageValid) return {
    persisted: false, storageReadable: current.storageReadable, storageValid: current.storageValid,
    added: [], duplicates: [], invalid: [], capacitySkipped: 0
  };
  if (targetGroup !== null) {
    const groups = readWatchlistGroups(storage);
    if (!groups.readable || !groups.valid || !groups.groups.includes(targetGroup)) return {
      persisted: false, storageReadable: true, storageValid: false, reason: "invalid-group",
      added: [], duplicates: [], invalid: [], capacitySkipped: 0
    };
  }
  const result = addWatchlistSymbols(records, storage, updatedAt);
  if (!result.persisted) return { ...result, rollbackSucceeded: restoreRaw(storage, WATCHLIST_KEY, previousWatchlist) };
  if (targetGroup !== null) {
    const persistedIdentities = new Set(result.items.map((item) => comparableMarketSymbol(item.symbol)));
    const assignmentSymbols = (Array.isArray(records) ? records : [])
      .map((item) => String(item?.symbol ?? "").trim().toUpperCase())
      .filter((symbol) => persistedIdentities.has(comparableMarketSymbol(symbol)));
    result.assignedCount = 0;
    if (assignmentSymbols.length) {
      const assignment = assignWatchlistGroups(assignmentSymbols, targetGroup, storage);
      if (!assignment.changed || !assignment.persisted) {
        const watchlistRestored = restoreRaw(storage, WATCHLIST_KEY, previousWatchlist);
        const groupsRestored = restoreRaw(storage, WATCHLIST_GROUPS_KEY, previousGroups);
        return { ...result, items: readWatchlistState(storage).items, persisted: false, reason: "group-write-failed",
          rollbackSucceeded: watchlistRestored && groupsRestored };
      }
      result.assignedCount = assignment.assignedCount;
    }
  }
  return { ...result, persisted: true, storageReadable: true, storageValid: true };
}

import { comparableMarketSymbol } from "./format.js";
import { DEFAULT_WATCHLIST_GROUP } from "./watchlist-groups.js";
import { sortWatchlistItems } from "./watchlist-sort.js";

export function buildWatchlistListViewModel({
  items = [],
  keyword = "",
  assignments = {},
  selectedGroup = "all",
  sortMode = "updated",
  sortDirection = "desc",
  getQuote = () => null
} = {}) {
  const records = Array.isArray(items) ? items : [];
  const normalizedKeyword = String(keyword || "").trim().toLocaleLowerCase();
  const groupAssignments = assignments && typeof assignments === "object" ? assignments : {};
  const visibleItems = records.filter((record) => {
    const matchesKeyword = !normalizedKeyword
      || String(record.symbol || "").toLocaleLowerCase().includes(normalizedKeyword)
      || String(record.name || "").toLocaleLowerCase().includes(normalizedKeyword);
    const assignedGroup = groupAssignments[comparableMarketSymbol(record.symbol)]
      || groupAssignments[record.symbol]
      || DEFAULT_WATCHLIST_GROUP;
    return matchesKeyword && (selectedGroup === "all" || assignedGroup === selectedGroup);
  });
  const sortedItems = sortWatchlistItems(visibleItems, {
    mode: sortMode,
    direction: sortDirection,
    getQuote
  });

  return {
    visibleItems,
    sortedItems,
    filterCountText: normalizedKeyword
      ? "匹配 " + visibleItems.length + " / " + records.length + " 只"
      : "显示全部 " + records.length + " 只",
    emptyMessage: normalizedKeyword
      ? "没有匹配的自选股。可尝试股票代码或自定义名称。"
      : "暂无自选股。"
  };
}

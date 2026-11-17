import test from "node:test";
import assert from "node:assert/strict";
import { buildWatchlistListViewModel } from "../src/watchlist-list-view-model.js";

const items = [
  { symbol: "000001.SZ", name: "平安银行", updated_at: "2026-11-01T09:00:00Z" },
  { symbol: "600519.SH", name: "贵州茅台", updated_at: "2026-11-03T09:00:00Z" },
  { symbol: "300750.SZ", name: "宁德时代", updated_at: "2026-11-02T09:00:00Z" }
];

test("关键字仅匹配证券代码和名称，忽略大小写与首尾空白", () => {
  const byName = buildWatchlistListViewModel({ items, keyword: "  贵州  " });
  const bySymbol = buildWatchlistListViewModel({ items, keyword: "300750" });
  assert.deepEqual(byName.visibleItems.map((item) => item.symbol), ["600519.SH"]);
  assert.deepEqual(bySymbol.visibleItems.map((item) => item.symbol), ["300750.SZ"]);
  assert.equal(byName.filterCountText, "匹配 1 / 3 只");
});

test("全部分组包含未分组证券，指定分组兼容带后缀证券代码", () => {
  const assignments = { "600519.SH": "核心观察" };
  const all = buildWatchlistListViewModel({ items, assignments });
  const selected = buildWatchlistListViewModel({ items, assignments, selectedGroup: "核心观察" });
  const unassigned = buildWatchlistListViewModel({ items, assignments, selectedGroup: "未分组" });
  assert.equal(all.visibleItems.length, 3);
  assert.deepEqual(selected.visibleItems.map((item) => item.symbol), ["600519.SH"]);
  assert.deepEqual(unassigned.visibleItems.map((item) => item.symbol), ["000001.SZ", "300750.SZ"]);
});

test("分组过滤不改变既有过滤计数语义", () => {
  const view = buildWatchlistListViewModel({
    items,
    assignments: { "600519.SH": "核心观察" },
    selectedGroup: "核心观察"
  });
  assert.equal(view.visibleItems.length, 1);
  assert.equal(view.filterCountText, "显示全部 3 只");
});

test("行情排序复用既有规则，缺失行情仍始终置底", () => {
  const view = buildWatchlistListViewModel({
    items,
    sortMode: "price",
    sortDirection: "desc",
    getQuote: (record) => record.symbol === "600519.SH" ? { current_price: 10 } : null
  });
  assert.deepEqual(view.sortedItems.map((item) => item.symbol), ["600519.SH", "000001.SZ", "300750.SZ"]);
});

test("无匹配结果保留原有空态文案，且不修改输入数组", () => {
  const before = items.map((item) => item.symbol);
  const view = buildWatchlistListViewModel({ items, keyword: "不存在" });
  assert.deepEqual(view.visibleItems, []);
  assert.deepEqual(view.sortedItems, []);
  assert.equal(view.emptyMessage, "没有匹配的自选股。可尝试股票代码或自定义名称。");
  assert.equal(view.filterCountText, "匹配 0 / 3 只");
  assert.deepEqual(items.map((item) => item.symbol), before);
});

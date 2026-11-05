import test from "node:test";
import assert from "node:assert/strict";
import { buildWatchlistImportPreview } from "../src/watchlist-import-preview.js";
import { MAX_WATCHLIST_ITEMS } from "../src/watchlist.js";

const validState = (items = []) => ({ items, storageReadable: true, storageValid: true });

test("parses CSV, tab and semicolon rows while preserving quoted delimiters", () => {
  const preview = buildWatchlistImportPreview('代码,名称\n600519,"贵州茅台,股份"\n000858\t五粮液\n300750;宁德时代', validState());
  assert.deepEqual(preview.accepted, [
    { symbol: "600519.SH", name: "贵州茅台,股份" },
    { symbol: "000858.SZ", name: "五粮液" },
    { symbol: "300750.SZ", name: "宁德时代" }
  ]);
  assert.equal(preview.counts.ready, 3);
});

test("reports invalid, duplicate and over-capacity rows without accepting them", () => {
  const existing = [{ symbol: "600519.SH" }];
  const preview = buildWatchlistImportPreview("600519\nnot-a-code\n000858\n300750", validState(existing));
  assert.deepEqual(preview.rows.map((row) => row.status), ["duplicate", "invalid", "ready", "ready"]);
  const full = validState(Array.from({ length: MAX_WATCHLIST_ITEMS }, (_, index) => ({ symbol: String(index).padStart(6, "0") })));
  const capped = buildWatchlistImportPreview("900001\n900002", full);
  assert.equal(capped.counts.limit, 2);
  assert.equal(capped.accepted.length, 0);
});

test("does not build an import preview from unreadable or invalid storage", () => {
  for (const state of [
    { items: [], storageReadable: false, storageValid: false },
    { items: [], storageReadable: true, storageValid: false }
  ]) {
    const preview = buildWatchlistImportPreview("600519", state);
    assert.equal(preview.error, "当前本地自选数据不可读取或格式异常，已暂停导入。");
    assert.deepEqual(preview.accepted, []);
  }
});

test("removes BOM, ignores blank lines and retains source line numbering semantics", () => {
  const preview = buildWatchlistImportPreview("\uFEFF代码\n\n600519\ninvalid", validState());
  assert.deepEqual(preview.rows.map((row) => row.line), [2, 3]);
  assert.deepEqual(preview.rows.map((row) => row.status), ["ready", "invalid"]);
});

test("rejects rows with unclosed CSV quotes instead of accepting truncated names", () => {
  const preview = buildWatchlistImportPreview('600519.SH,"贵州茅台\n000858.SZ,五粮液', validState());
  assert.deepEqual(preview.rows.map((row) => row.status), ["invalid", "ready"]);
  assert.equal(preview.rows[0].message, "CSV 引号未闭合");
  assert.deepEqual(preview.accepted, [{ symbol: "000858.SZ", name: "五粮液" }]);
});

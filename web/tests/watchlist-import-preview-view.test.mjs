import test from "node:test";
import assert from "node:assert/strict";
import { buildWatchlistImportPreviewView } from "../src/watchlist-import-preview-view.js";

class TestNode {
  constructor(tagName, text = "") {
    this.tagName = tagName;
    this.children = [];
    this.textContent = text;
  }

  append(...nodes) {
    this.children.push(...nodes);
  }
}

const document = {
  createElement: (tagName) => new TestNode(tagName),
  createTextNode: (text) => new TestNode("#text", text)
};

test("builds the import preview table with stable columns and literal cell text", () => {
  const view = buildWatchlistImportPreviewView({
    document,
    preview: {
      rows: [
        { line: 2, symbol: "600519.SH", name: "贵州茅台", message: "可导入" },
        { line: 3, symbol: "000858.SZ", name: "<script>alert(1)</script>", message: "已存在或文件内重复" }
      ],
      counts: { ready: 1, duplicate: 1, invalid: 0, limit: 0 }
    }
  });

  assert.equal(view.nodes.length, 1);
  const [table] = view.nodes;
  assert.equal(table.tagName, "table");
  assert.deepEqual(table.children.map((node) => node.tagName), ["thead", "tbody"]);
  assert.deepEqual(table.children[0].children[0].children.map((cell) => cell.textContent), ["行", "代码", "名称", "结果"]);
  assert.deepEqual(table.children[1].children.map((row) => row.children.map((cell) => cell.textContent)), [
    ["2", "600519.SH", "贵州茅台", "可导入"],
    ["3", "000858.SZ", "<script>alert(1)</script>", "已存在或文件内重复"]
  ]);
  assert.equal(view.summary, "可导入 1 只 · 重复 1 只 · 无效 0 条 · 超出上限 0 只");
  assert.equal(view.confirmDisabled, false);
});

test("returns a text node and disables confirmation when storage validation failed", () => {
  const view = buildWatchlistImportPreviewView({
    document,
    preview: { error: "当前本地自选数据不可读取或格式异常，已暂停导入。" }
  });

  assert.equal(view.nodes.length, 1);
  assert.equal(view.nodes[0].tagName, "#text");
  assert.equal(view.nodes[0].textContent, "当前本地自选数据不可读取或格式异常，已暂停导入。");
  assert.equal(view.summary, view.nodes[0].textContent);
  assert.equal(view.confirmDisabled, true);
});

test("keeps confirmation disabled when no rows are ready", () => {
  const view = buildWatchlistImportPreviewView({
    document,
    preview: {
      rows: [{ line: 1, symbol: "invalid", name: "", message: "代码格式无效" }],
      counts: { ready: 0, duplicate: 0, invalid: 1, limit: 0 }
    }
  });
  assert.equal(view.confirmDisabled, true);
});

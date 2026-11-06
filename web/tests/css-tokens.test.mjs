import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const css = (await readFile(new URL("../styles.css", import.meta.url), "utf8"))
  .replace(/\/\*[\s\S]*?\*\//g, "");
const declared = new Set([...css.matchAll(/(--[\w-]+)\s*:/g)].map(match => match[1]));
const references = [...css.matchAll(/var\(\s*(--[\w-]+)/g)];
const unresolved = [...new Set(references.filter(match => !declared.has(match[1])).map(match => {
  return match[1] + " (line " + css.slice(0, match.index).split("\n").length + ")";
}))];

test("所有 CSS 自定义属性引用均有定义", () => {
  assert.deepEqual(unresolved, [], "发现未定义的 CSS token");
});

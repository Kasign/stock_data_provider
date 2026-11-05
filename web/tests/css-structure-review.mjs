import { readFile } from "node:fs/promises";
import postcss from "postcss";
import selectorParser from "postcss-selector-parser";

const css = await readFile(new URL("../styles.css", import.meta.url), "utf8");
const root = postcss.parse(css, { from: "web/styles.css" });
const rules = [];
const declarations = [];
let atRuleCount = 0;

function visit(container, conditions = []) {
  for (const node of container.nodes || []) {
    if (node.type === "atrule") {
      atRuleCount += 1;
      const condition = `@${node.name} ${node.params}`.trim();
      visit(node, [...conditions, condition]);
      continue;
    }
    if (node.type === "rule") {
      const rule = {
        selectors: selectorParser().astSync(node.selector).nodes.map((item) => item.toString().trim()),
        conditions,
        source: node.source?.start?.line ?? null,
        declarations: []
      };
      for (const child of node.nodes || []) {
        if (child.type !== "decl") continue;
        const declaration = { property: child.prop, value: child.value, important: child.important === true };
        rule.declarations.push(declaration);
        declarations.push(declaration);
      }
      rules.push(rule);
      visit(node, conditions);
      continue;
    }
    if (node.nodes) visit(node, conditions);
  }
}
visit(root);

const selectorRules = new Map();
for (const rule of rules) {
  for (const selector of rule.selectors) {
    const entries = selectorRules.get(selector) || [];
    entries.push(rule);
    selectorRules.set(selector, entries);
  }
}

const exactDuplicates = [];
const variantSelectors = [];
for (const [selector, entries] of selectorRules) {
  if (entries.length < 2) continue;
  const exact = new Map();
  const propertyValues = new Map();
  for (const entry of entries) {
    const context = entry.conditions.join(" > ");
    const body = entry.declarations.map((item) =>
      `${item.property}:${item.value}${item.important ? " !important" : ""}`
    ).join(";");
    const key = `${context}\n${body}`;
    const positions = exact.get(key) || [];
    positions.push(entry.source);
    exact.set(key, positions);
    for (const item of entry.declarations) {
      const values = propertyValues.get(item.property) || new Set();
      values.add(`${item.value}${item.important ? " !important" : ""}`);
      propertyValues.set(item.property, values);
    }
  }
  for (const [key, lines] of exact) {
    if (lines.length > 1) exactDuplicates.push({ selector, count: lines.length, lines, context: key.split("\n")[0] });
  }
  const variants = [...propertyValues]
    .filter(([, values]) => values.size > 1)
    .map(([property, values]) => ({ property, values: [...values] }));
  if (variants.length) variantSelectors.push({
    selector,
    count: entries.length,
    lines: entries.map((entry) => entry.source),
    variants
  });
}

const sharedSelectors = [
  ".main-area", ".view", ".sidebar", ".topbar", ".nav-link",
  ".button", ".field", ".panel", ".panel-heading", ".page-heading"
];
const missing = sharedSelectors.filter((selector) => !selectorRules.has(selector));
if (missing.length) throw new Error(`Missing shared shell selectors: ${missing.join(", ")}`);
if (rules.length < 1000 || declarations.length < 2000) {
  throw new Error(`Unexpectedly incomplete CSS parse: ${rules.length} rules, ${declarations.length} declarations`);
}

console.log(JSON.stringify({
  file: "web/styles.css",
  bytes: Buffer.byteLength(css),
  lines: css.split("\n").length,
  rules: rules.length,
  declarations: declarations.length,
  atRules: atRuleCount,
  uniqueSelectors: selectorRules.size,
  repeatedSelectors: [...selectorRules.values()].filter((entries) => entries.length > 1).length,
  exactDuplicateGroups: exactDuplicates.length,
  exactDuplicates: exactDuplicates.slice(0, 30),
  selectorsWithDeclarationVariants: variantSelectors.length,
  declarationVariants: variantSelectors.slice(0, 40),
  sharedSelectorOwnershipCandidates: sharedSelectors.map((selector) => ({
    selector,
    occurrences: selectorRules.get(selector).length,
    lines: selectorRules.get(selector).map((entry) => entry.source)
  }))
}, null, 2));

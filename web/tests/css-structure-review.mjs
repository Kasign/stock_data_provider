import { readFile } from "node:fs/promises";
import postcss from "postcss";
import selectorParser from "postcss-selector-parser";

const css = await readFile(new URL("../styles.css", import.meta.url), "utf8");
const root = postcss.parse(css, { from: "web/styles.css" });
const rules = [];
const declarations = [];
let atRuleCount = 0;

function compareSpecificity(left, right) {
  for (let index = 0; index < 3; index += 1) {
    if (left[index] !== right[index]) return left[index] - right[index];
  }
  return 0;
}

function specificityForSelector(selector) {
  const ast = selectorParser().astSync(selector);
  function score(nodes) {
    const total = [0, 0, 0];
    for (const node of nodes) {
      let part = [0, 0, 0];
      if (node.type === "id") part = [1, 0, 0];
      else if (node.type === "class" || node.type === "attribute") part = [0, 1, 0];
      else if (node.type === "tag") part = [0, 0, 1];
      else if (node.type === "pseudo") {
        const name = node.value.toLowerCase();
        const nested = (node.nodes || []).map((nestedSelector) => score(nestedSelector.nodes || []));
        const maximum = nested.reduce((best, item) => compareSpecificity(item, best) > 0 ? item : best, [0, 0, 0]);
        if (name === ":where") part = [0, 0, 0];
        else if ([":is", ":not", ":has", ":matches", ":any"].includes(name)) part = maximum;
        else if (name === ":nth-child" || name === ":nth-last-child") {
          part = [maximum[0], 1 + maximum[1], maximum[2]];
        } else if (name.startsWith("::") || [":before", ":after", ":first-line", ":first-letter"].includes(name)) {
          part = [0, 0, 1];
        } else {
          part = [0, 1, 0];
          if (name === ":host" || name === ":host-context" || name === "::slotted") {
            part = [maximum[0], 1 + maximum[1], maximum[2]];
          }
        }
      }
      total[0] += part[0];
      total[1] += part[1];
      total[2] += part[2];
    }
    return total;
  }
  return ast.nodes.map((item) => score(item.nodes || [])).reduce(
    (best, item) => compareSpecificity(item, best) > 0 ? item : best, [0, 0, 0]
  );
}


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
        selectors: selectorParser().astSync(node.selector).nodes.map((item) => {
          const selector = item.toString().trim();
          return { selector, specificity: specificityForSelector(selector) };
        }),
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
  for (const item of rule.selectors) {
    const entries = selectorRules.get(item.selector) || [];
    entries.push({ ...rule, selector: item.selector, specificity: item.specificity });
    selectorRules.set(item.selector, entries);
  }
}


const declarationOverrideCandidates = [];
for (const [selector, entries] of selectorRules) {
  const byContext = new Map();
  for (const entry of entries) {
    const context = entry.conditions.join(" > ");
    const scoped = byContext.get(context) || [];
    scoped.push(entry);
    byContext.set(context, scoped);
  }
  for (const [context, scoped] of byContext) {
    for (let earlierIndex = 0; earlierIndex < scoped.length; earlierIndex += 1) {
      const earlier = scoped[earlierIndex];
      const earlierByProperty = new Map(earlier.declarations.map((item) => [item.property, item]));
      for (let laterIndex = earlierIndex + 1; laterIndex < scoped.length; laterIndex += 1) {
        const later = scoped[laterIndex];
        for (const [property, before] of earlierByProperty) {
          const after = later.declarations.filter((item) => item.property === property).at(-1);
          if (!after || (before.value === after.value && before.important === after.important)) continue;
          declarationOverrideCandidates.push({
            selector,
            specificity: earlier.specificity,
            property,
            earlier: { line: earlier.source, value: before.value, important: before.important },
            later: { line: later.source, value: after.value, important: after.important },
            expectedWinner: before.important && !after.important ? "earlier-important" : "later-source-order",
            context
          });
        }
      }
    }
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
  declarationOverrideAnalysisScope: "Same normalized selector and identical at-rule context only; candidates are not a full cross-selector cascade proof.",
  declarationOverrideCandidateCount: declarationOverrideCandidates.length,
  declarationOverrideCandidates: declarationOverrideCandidates.slice(0, 100),
  exactDuplicates: exactDuplicates.slice(0, 30),
  selectorsWithDeclarationVariants: variantSelectors.length,
  declarationVariants: variantSelectors.slice(0, 40),
  sharedSelectorOwnershipCandidates: sharedSelectors.map((selector) => ({
    selector,
    specificity: specificityForSelector(selector),
    occurrences: selectorRules.get(selector).length,
    lines: selectorRules.get(selector).map((entry) => entry.source)
  }))
}, null, 2));

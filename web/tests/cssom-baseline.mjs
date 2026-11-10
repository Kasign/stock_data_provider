import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { gunzipSync } from "node:zlib";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(webRoot, "..");
const outputPath = path.resolve(process.env.BROWSER_CSSOM_BASELINE_PATH ||
  path.join(repositoryRoot, "browser-cssom-baseline", "screening-local-batch.json"));
const viewports = [
  { name: "mobile-320", width: 320, height: 800 },
  { name: "tablet-768", width: 768, height: 1024 },
  { name: "desktop-1280", width: 1280, height: 900 },
  { name: "wide-1600", width: 1600, height: 1000 }
];
const themes = ["dark", "light"];
const targets = [
  { name: "batch-toolbar", selector: "#screening-view .screening-local-batch-toolbar" },
  { name: "select-all", selector: "#screening-view .screening-select-all" },
  { name: "selected-count", selector: "#screening-view #screening-local-select-count" },
  { name: "group-field", selector: "#screening-view .screening-local-batch-toolbar .field" },
  { name: "group-select", selector: "#screening-view #screening-local-group" },
  { name: "add-watchlist-button", selector: "#screening-view #screening-local-add-watchlist" }
];
const properties = [
  "display", "box-sizing", "width", "min-width", "max-width", "height", "min-height",
  "flex-direction", "flex-wrap", "align-items", "justify-content", "gap", "row-gap", "column-gap",
  "margin-top", "margin-right", "margin-bottom", "margin-left",
  "padding-top", "padding-right", "padding-bottom", "padding-left",
  "border-top-width", "border-right-width", "border-bottom-width", "border-left-width",
  "border-top-style", "border-top-color", "border-right-color", "border-bottom-color",
  "border-left-color", "border-radius", "background-color", "color",
  "text-align", "white-space", "overflow-wrap"
];
const contentTypes = {
  ".html": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8", ".svg": "image/svg+xml"
};
const server = createServer(async (request, response) => {
  const pathname = decodeURIComponent(new URL(request.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) {
    response.writeHead(403); response.end("Forbidden"); return;
  }
  try {
    const body = await readFile(target);
    response.writeHead(200, {
      "content-type": contentTypes[path.extname(target)] || "application/octet-stream",
      "cache-control": "no-store"
    });
    response.end(body);
  } catch {
    response.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
    response.end("Not found");
  }
});
await new Promise((resolve, reject) => {
  server.once("error", reject); server.listen(0, "127.0.0.1", resolve);
});
const address = server.address();
assert.ok(address && typeof address === "object", "CSSOM fixture server must listen");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;
try {
  browser = await chromium.launch({ headless: true });
  const snapshot = {
    schemaVersion: 1, capturedAt: new Date().toISOString(), browserVersion: browser.version(),
    scope: "screening-local-batch",
    notes: [
      "Computed-style capture only; this is an observation artifact, not a golden pass/fail comparison.",
      "Viewport and theme are separate scenarios because responsive and theme-dependent values are expected to differ.",
      "Font family and pixel screenshots are intentionally excluded; CI does not provide a complete Chinese font environment.",
      "The fixture intercepts API and health requests with 503 responses; no provider data is represented."
    ],
    properties, scenarios: []
  };
  for (const theme of themes) {
    for (const viewport of viewports) {
      const context = await browser.newContext({
        viewport: { width: viewport.width, height: viewport.height }, reducedMotion: "reduce"
      });
      const page = await context.newPage();
      await page.addInitScript((value) => {
        localStorage.setItem("stock-provider-webui.theme.v1", value);
      }, theme);
      await page.route("**/*", async (route) => {
        const url = new URL(route.request().url());
        if (url.origin === baseUrl && (url.pathname.includes("/api/") ||
          /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(url.pathname))) {
          return route.fulfill({
            status: 503, contentType: "application/json",
            body: JSON.stringify({ status: "unavailable", error: "isolated CSSOM fixture" })
          });
        }
        return route.continue();
      });
      await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
      await page.locator("#screening-view").waitFor({ state: "attached" });
      const mobileMenu = page.locator("#mobile-menu-toggle");
      if (await mobileMenu.isVisible() && (await mobileMenu.getAttribute("aria-expanded")) !== "true") {
        await mobileMenu.click();
      }
      await page.locator('.nav-link[data-view="screening"]').click();
      await page.locator("#screening-view").waitFor({ state: "visible" });
      await page.waitForFunction((expected) =>
        document.documentElement.dataset.theme === expected, theme);
      const elements = await page.evaluate(({ requestedTargets, requestedProperties }) =>
        requestedTargets.map((target) => {
          const element = document.querySelector(target.selector);
          if (!element) throw new Error("CSSOM target not found: " + target.selector);
          const style = getComputedStyle(element);
          const computed = {};
          for (const property of requestedProperties) computed[property] = style.getPropertyValue(property);
          const rect = element.getBoundingClientRect();
          return {
            name: target.name, selector: target.selector, computed,
            rect: {
              x: Math.round(rect.x * 100) / 100, y: Math.round(rect.y * 100) / 100,
              width: Math.round(rect.width * 100) / 100, height: Math.round(rect.height * 100) / 100
            }
          };
        }), { requestedTargets: targets, requestedProperties: properties });
      snapshot.scenarios.push({ theme, viewport, elements });
      await context.close();
    }
  }
  const marketSemanticColorChecks = [];
  for (const theme of themes) {
    for (const scheme of ["cn", "reverse"]) {
      const context = await browser.newContext({
        viewport: { width: 1280, height: 900 }, reducedMotion: "reduce"
      });
      const page = await context.newPage();
      await page.addInitScript(({ themeValue, schemeValue }) => {
        localStorage.setItem("stock-provider-webui.theme.v1", themeValue);
        localStorage.setItem("stock-provider-webui.preferences.v1", JSON.stringify({
          marketColorScheme: schemeValue
        }));
      }, { themeValue: theme, schemeValue: scheme });
      await page.route("**/*", async (route) => {
        const url = new URL(route.request().url());
        if (url.origin === baseUrl && (url.pathname.includes("/api/") ||
          /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(url.pathname))) {
          return route.fulfill({
            status: 503, contentType: "application/json",
            body: JSON.stringify({ status: "unavailable", error: "isolated semantic color fixture" })
          });
        }
        return route.continue();
      });
      await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
      await page.waitForFunction(({ themeValue, schemeValue }) =>
        document.documentElement.dataset.theme === themeValue &&
        document.documentElement.dataset.marketColorScheme === schemeValue,
      { themeValue: theme, schemeValue: scheme });
      const observation = await page.evaluate(() => {
        const root = document.documentElement;
        const required = (selector) => {
          const element = document.querySelector(selector);
          if (!element) throw new Error("Semantic color target not found: " + selector);
          return element;
        };
        const tokenColor = (token) => {
          const probe = document.createElement("span");
          probe.style.color = "var(" + token + ")";
          document.body.append(probe);
          const color = getComputedStyle(probe).color;
          probe.remove();
          return color;
        };
        const expected = {
          positive: tokenColor("--positive"),
          negative: tokenColor("--negative"),
          flat: tokenColor("--text-soft"),
          unknown: tokenColor("--text-dim"),
          warning: tokenColor("--warning")
        };
        const checks = {};
        const record = (name, element, token) => {
          checks[name] = {
            actual: getComputedStyle(element).color,
            expected: expected[token],
            className: element.className
          };
        };
        const toggleAndRecord = (name, element, state, token) => {
          element.classList.remove("positive", "negative");
          element.classList.add(state);
          record(name, element, token);
        };
        toggleAndRecord("quote-positive", required("#metric-change"), "positive", "positive");
        toggleAndRecord("quote-negative", required("#metric-change"), "negative", "negative");
        toggleAndRecord("research-positive", required("#research-overview-change"), "positive", "positive");
        toggleAndRecord("research-negative", required("#research-overview-change"), "negative", "negative");
        toggleAndRecord("technical-positive", required("#technical-ma-5-change"), "positive", "positive");
        toggleAndRecord("technical-negative", required("#technical-ma-5-change"), "negative", "negative");

        const indexChanges = document.createElement("div");
        indexChanges.className = "market-index-changes";
        document.body.append(indexChanges);
        const breadth = document.createElement("div");
        breadth.className = "market-breadth-metric";
        document.body.append(breadth);
        const marketLists = required("#market-lists-panel");
        const marketActivity = required("#market-activity-panel");
        for (const state of ["positive", "negative", "flat", "unknown"]) {
          const token = state;
          const indexNode = document.createElement(state === "negative" || state === "unknown" ? "strong" : "span");
          indexNode.className = "market-data-" + state;
          indexChanges.append(indexNode);
          record("index-" + state, indexNode, token);
          const breadthNode = document.createElement("strong");
          breadthNode.className = "market-data-" + state;
          breadth.append(breadthNode);
          record("breadth-" + state, breadthNode, token);
          const listNode = document.createElement("span");
          listNode.className = "market-data-" + state;
          marketLists.append(listNode);
          record("lists-" + state, listNode, token);
          const activityNode = document.createElement("span");
          activityNode.className = "market-data-" + state;
          marketActivity.append(activityNode);
          record("activity-" + state, activityNode, token);
        }
        const footer = document.createElement("div");
        footer.className = "market-index-card-footer";
        const reason = document.createElement("small");
        reason.className = "market-index-reasons";
        footer.append(reason);
        document.body.append(footer);
        record("index-reason", reason, "warning");

        const table = document.createElement("table");
        const row = document.createElement("tr");
        const positiveCell = document.createElement("td");
        positiveCell.classList.add("positive");
        row.append(positiveCell);
        const negativeRow = document.createElement("tr");
        const negativeCell = document.createElement("td");
        negativeCell.classList.add("negative");
        negativeRow.append(negativeCell);
        table.append(row, negativeRow);
        document.body.append(table);
        record("table-positive", positiveCell, "positive");
        record("table-negative", negativeCell, "negative");
        return { theme: root.dataset.theme, scheme: root.dataset.marketColorScheme, checks };
      });
      assert.equal(observation.theme, theme);
      assert.equal(observation.scheme, scheme);
      for (const [name, value] of Object.entries(observation.checks)) {
        assert.equal(value.actual, value.expected, `Semantic color mismatch: ${name} class=${value.className} in ${theme}/${scheme}`);
      }
      marketSemanticColorChecks.push(observation);
      await context.close();
    }
  }
  snapshot.marketSemanticColorChecks = marketSemanticColorChecks;
  console.log("Market semantic colors passed:", marketSemanticColorChecks.length, "theme/scheme scenarios");

  const fixturePath = path.join(webRoot, "tests", "fixtures", "screening-local-batch.cssom.json.gz");
  const fixture = JSON.parse(gunzipSync(await readFile(fixturePath)).toString("utf8"));
  assert.equal(fixture.schemaVersion, 1, "CSSOM fixture schema must be supported");
  assert.equal(fixture.scope, snapshot.scope, "CSSOM fixture scope must match the capture");
  const comparison = {
    schemaVersion: fixture.schemaVersion,
    scope: fixture.scope,
    properties: fixture.properties,
    scenarios: snapshot.scenarios.map((scenario) => ({
      theme: scenario.theme,
      viewport: {
        name: scenario.viewport.name,
        width: scenario.viewport.width,
        height: scenario.viewport.height
      },
      elements: scenario.elements.map((element) => ({
        name: element.name,
        computed: Object.fromEntries(fixture.properties.map((property) =>
          [property, element.computed[property]])),
        rect: {
          width: element.rect.width,
          height: element.rect.height
        }
      }))
    }))
  };
  // Always preserve the observation artifact, including on an intentional baseline mismatch.
  await mkdir(path.dirname(outputPath), { recursive: true });
  await writeFile(outputPath, JSON.stringify(snapshot, null, 2) + "\n", "utf8");
  console.log("CSSOM observation captured:", outputPath);
  assert.deepEqual(comparison, {
    schemaVersion: fixture.schemaVersion,
    scope: fixture.scope,
    properties: fixture.properties,
    scenarios: fixture.scenarios
  }, "Screening CSSOM regression baseline changed; review and update the fixture intentionally");
  console.log("CSSOM regression baseline passed:", fixturePath);
  console.log("Scenarios:", snapshot.scenarios.length, "| targets per scenario:", targets.length);
} finally {
  if (browser) await browser.close();
  await new Promise((resolve) => server.close(resolve));
}

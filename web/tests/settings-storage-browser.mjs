import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const contentTypes = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml"
};
const server = createServer(async (request, response) => {
  const pathname = decodeURIComponent(new URL(request.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) {
    response.writeHead(403);
    response.end("Forbidden");
    return;
  }
  try {
    const body = await readFile(target);
    response.writeHead(200, { "content-type": contentTypes[path.extname(target)] || "application/octet-stream", "cache-control": "no-store" });
    response.end(body);
  } catch {
    response.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
    response.end("Not found");
  }
});

await new Promise((resolve, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolve);
});
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;

try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const first = await context.newPage();
  const second = await context.newPage();
  const pageErrors = [];
  const observedRequests = [];
  first.on("pageerror", error => pageErrors.push(error.stack || error.message));
  first.on("request", request => observedRequests.push(request.url()));

  for (const page of [first, second]) {
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.origin === baseUrl && (
        url.pathname.includes("/api/") ||
        /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(url.pathname)
      )) {
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ status: "unavailable", error: "isolated browser fixture" })
        });
      }
      return route.continue();
    });
    await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
    await page.locator("#settings-form").waitFor({ state: "attached" });
  }

  await first.locator('.nav-link[data-view="settings"]').click();
  await first.locator("#settings-form").waitFor({ state: "visible" });

  await second.evaluate(() => localStorage.setItem("stock-provider-webui.theme.v1", "light"));
  await first.waitForFunction(() => document.documentElement.dataset.theme === "light");
  assert.equal(await first.locator("#theme-toggle").getAttribute("aria-pressed"), "true");
  assert.equal(await first.locator('[data-theme-value="light"]').getAttribute("aria-pressed"), "true");

  await second.evaluate(() => localStorage.setItem("stock-provider-webui.preferences.v1", JSON.stringify({
    marketColorScheme: "reverse",
    priceDigits: 3,
    percentDigits: 3,
    amountDisplayUnit: "raw"
  })));
  await first.waitForFunction(() =>
    document.documentElement.dataset.marketColorScheme === "reverse" &&
    document.querySelector("#settings-market-color-scheme").value === "reverse" &&
    document.querySelector("#settings-price-digits").value === "3" &&
    document.querySelector("#settings-percent-digits").value === "3" &&
    document.querySelector("#settings-amount-unit").value === "raw"
  );
  assert.equal(await first.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--positive").trim()), "#07875c");

  await first.locator("#market-base-input").fill("/draft-market");
  await first.locator("#analysis-base-input").fill("/draft-analysis");
  await first.locator("#market-key-input").fill("unsaved-local-secret");
  assert.equal(await first.locator("#save-settings").isEnabled(), true);

  await second.evaluate(() => localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
    marketBase: "/external-market",
    analysisBase: "/external-analysis",
    apiKey: "external-tab-secret"
  })));
  await first.waitForFunction(() =>
    document.querySelector("#market-base-input").value === "/draft-market" &&
    document.querySelector("#analysis-base-input").value === "/draft-analysis" &&
    document.querySelector("#market-key-input").value === "unsaved-local-secret" &&
    document.querySelector("#settings-feedback").textContent.includes("未保存草稿已保留")
  );
  assert.equal(await first.locator("#save-settings").isEnabled(), true);
  assert.equal(await first.evaluate(() => document.documentElement.innerHTML.includes("external-tab-secret")), false);
  assert.ok(observedRequests.some(url => new URL(url).pathname.startsWith("/external-market/")),
    "new saved connection must be used for subsequent health/data requests");

  await second.evaluate(() => localStorage.setItem("stock-provider-webui.settings.v1", "{invalid-json"));
  await first.waitForFunction(() => document.querySelector("#settings-feedback").textContent.includes("连接配置格式无效"));
  assert.equal(await first.locator("#market-base-input").inputValue(), "/draft-market");

  await second.evaluate(() => localStorage.removeItem("stock-provider-webui.settings.v1"));
  await first.waitForFunction(() => document.querySelector("#settings-feedback").textContent.includes("未保存草稿已保留"));
  assert.equal(await first.locator("#market-base-input").inputValue(), "/draft-market");
  assert.ok(observedRequests.some(url => new URL(url).pathname.startsWith("/market-api/")),
    "removing saved settings in another tab must restore default API paths");

  await first.locator("#cancel-settings").click();
  assert.equal(await first.locator("#market-base-input").inputValue(), "/market-api");
  assert.equal(await first.locator("#analysis-base-input").inputValue(), "/analysis-api");
  assert.equal(await first.locator("#market-key-input").inputValue(), "");
  assert.equal(await first.locator("#save-settings").isEnabled(), false);
  assert.equal(await first.locator("#cancel-settings").isEnabled(), false);

  await first.locator("#market-base-input").fill("/saved-market");
  await first.locator("#analysis-base-input").fill("/saved-analysis");
  await first.locator("#market-key-input").fill("saved-local-secret");
  await first.locator("#save-settings").click();
  await first.waitForFunction(() => document.querySelector("#settings-feedback").textContent.includes("连接配置已保存"));
  const savedSettings = await first.evaluate(() => JSON.parse(localStorage.getItem("stock-provider-webui.settings.v1")));
  assert.deepEqual(savedSettings, {
    marketBase: "/saved-market",
    analysisBase: "/saved-analysis",
    apiKey: "saved-local-secret"
  });
  assert.equal(await first.locator("#market-key-input").inputValue(), "");
  assert.equal(await first.locator("#market-base-input").inputValue(), "/saved-market");
  assert.equal(await first.locator("#save-settings").isEnabled(), false);
  assert.equal(await first.evaluate(() => document.documentElement.innerHTML.includes("saved-local-secret")), false);

  await first.evaluate(() => {
    window.__originalStorageSetItem = Storage.prototype.setItem;
    Storage.prototype.setItem = function (key, value) {
      if (key === "stock-provider-webui.preferences.v1") throw new DOMException("quota exceeded", "QuotaExceededError");
      return window.__originalStorageSetItem.call(this, key, value);
    };
  });
  await first.locator("#settings-price-digits").selectOption("2");
  await first.waitForFunction(() => document.querySelector("#local-preferences-feedback").textContent.includes("偏好保存失败"));
  assert.equal(await first.locator("#settings-price-digits").inputValue(), "3",
    "failed preference persistence must restore the last readable saved value");
  await first.evaluate(() => {
    if (window.__originalStorageSetItem) Storage.prototype.setItem = window.__originalStorageSetItem;
  });

  assert.deepEqual(pageErrors, [], "cross-tab settings synchronization must not raise page errors");
  await context.close();
  console.log("Settings cross-tab synchronization passed: theme, display preferences, reverse market colors, connection changes, invalid data, key removal, and unsaved draft preservation.");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

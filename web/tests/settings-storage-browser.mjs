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
  await context.addInitScript(() => {
    localStorage.setItem("stock-provider-webui.settings.v1", JSON.stringify({
      marketBase: "/legacy-market",
      analysisBase: "/legacy-analysis",
      apiKey: "legacy-browser-secret"
    }));
    localStorage.setItem("stock-provider-webui.theme.v1", "dark");
  });
  const first = await context.newPage();
  const second = await context.newPage();
  const pageErrors = [];
  const observedRequests = [];
  first.on("pageerror", error => pageErrors.push(error.stack || error.message));
  first.on("request", request => observedRequests.push({
    url: request.url(),
    headers: request.headers()
  }));

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
  assert.equal(await first.locator('[data-theme-value="dark"]').getAttribute("aria-pressed"), "true",
    "settings theme buttons must reflect the persisted theme during initialization");

  const fixedConnection = await first.evaluate(() => ({
    marketBase: document.querySelector("#market-base-input")?.value || "",
    analysisBase: document.querySelector("#analysis-base-input")?.value || "",
    marketBaseType: document.querySelector("#market-base-input")?.type || "",
    analysisBaseType: document.querySelector("#analysis-base-input")?.type || "",
    saveHidden: document.querySelector("#save-settings")?.hidden === true,
    draftCheckHidden: document.querySelector("#settings-check-draft-connection")?.hidden === true,
    apiKeyControlCount: document.querySelectorAll("#market-key-input").length,
    stored: localStorage.getItem("stock-provider-webui.settings.v1")
  }));
  assert.deepEqual(fixedConnection, {
    marketBase: "/",
    analysisBase: "/",
    marketBaseType: "hidden",
    analysisBaseType: "hidden",
    saveHidden: true,
    draftCheckHidden: true,
    apiKeyControlCount: 0,
    stored: "{}"
  }, "legacy browser API paths and API keys must be cleared; Root Server owns the same-origin API prefix");

  await second.evaluate(() => localStorage.setItem("stock-provider-webui.theme.v1", "light"));
  await first.waitForFunction(() => document.documentElement.dataset.theme === "light");
  assert.equal(await first.locator("#theme-toggle").getAttribute("aria-pressed"), "true");
  assert.equal(await first.locator('[data-theme-value="light"]').getAttribute("aria-pressed"), "true");

  await first.locator("#theme-toggle").click();
  await first.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  assert.equal(await first.locator('[data-theme-value="dark"]').getAttribute("aria-pressed"), "true",
    "top-bar theme changes must synchronize settings theme buttons");

  await first.evaluate(() => {
    window.__originalStorageSetItem = Storage.prototype.setItem;
    Storage.prototype.setItem = function (key, value) {
      if (key === "stock-provider-webui.theme.v1") throw new DOMException("storage denied", "SecurityError");
      return window.__originalStorageSetItem.call(this, key, value);
    };
  });
  await first.locator("#theme-toggle").click();
  await first.waitForFunction(() => document.documentElement.dataset.theme === "light");
  assert.equal(await first.locator('[data-theme-value="light"]').getAttribute("aria-pressed"), "true",
    "theme buttons must follow the session theme when persistence fails");
  assert.match(await first.locator("#theme-toggle").getAttribute("title"), /未能确认主题偏好已保存/,
    "theme toggle must expose a persistence warning");
  await first.evaluate(() => {
    if (window.__originalStorageSetItem) {
      Storage.prototype.setItem = window.__originalStorageSetItem;
      window.__originalStorageSetItem.call(localStorage, "stock-provider-webui.theme.v1", "light");
    }
  });

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
  assert.equal(await first.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue("--positive").trim()), "#07875c");

  const savedPreferencesBeforeCorruption = await first.evaluate(() =>
    localStorage.getItem("stock-provider-webui.preferences.v1"));
  await second.evaluate(() =>
    localStorage.setItem("stock-provider-webui.preferences.v1", "{invalid-json"));
  await first.waitForFunction(() =>
    document.querySelector("#local-preferences-feedback").textContent.includes("格式异常"));
  assert.equal(await first.locator("#settings-price-digits").inputValue(), "3",
    "malformed cross-tab preference data must not replace the current rendered settings");
  assert.equal(await first.evaluate(() =>
    localStorage.getItem("stock-provider-webui.preferences.v1")), "{invalid-json",
    "malformed preference data must not be silently overwritten");
  await second.evaluate((value) =>
    localStorage.setItem("stock-provider-webui.preferences.v1", value), savedPreferencesBeforeCorruption);
  await first.waitForFunction(() =>
    document.querySelector("#local-preferences-feedback").textContent.includes("已从其他标签页同步"));

  await first.evaluate(() => {
    window.__originalStorageSetItem = Storage.prototype.setItem;
    Storage.prototype.setItem = function (key, value) {
      if (key === "stock-provider-webui.preferences.v1") {
        throw new DOMException("quota exceeded", "QuotaExceededError");
      }
      return window.__originalStorageSetItem.call(this, key, value);
    };
  });
  await first.locator("#settings-price-digits").selectOption("2");
  await first.waitForFunction(() =>
    document.querySelector("#local-preferences-feedback").textContent.includes("偏好保存失败"));
  assert.equal(await first.locator("#settings-price-digits").inputValue(), "3",
    "failed preference persistence must restore the last readable saved value");
  await first.evaluate(() => {
    if (window.__originalStorageSetItem) {
      Storage.prototype.setItem = window.__originalStorageSetItem;
    }
  });

  await first.waitForFunction(() => localStorage.getItem("stock-provider-webui.settings.v1") === "{}");
  assert.equal(await second.evaluate(() =>
    localStorage.getItem("stock-provider-webui.settings.v1")), "{}");
  assert.equal(observedRequests.some(item =>
    new URL(item.url).pathname.startsWith("/legacy-market") ||
    new URL(item.url).pathname.startsWith("/legacy-analysis")), false,
    "legacy browser API paths must never be used for requests");
  assert.equal(observedRequests.some(item => Object.keys(item.headers).some(key =>
    key.toLowerCase() === "x-api-key")), false,
    "browser requests must not carry an API key");
  assert.deepEqual(pageErrors, [], "cross-tab theme and preference synchronization must not raise page errors");

  await context.close();
  console.log("Settings browser storage passed: legacy API settings are cleared, Root Server API paths remain fixed, no browser API key is sent, and theme/preferences synchronize across tabs with persistence-failure recovery.");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

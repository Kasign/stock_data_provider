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
  ".json": "application/json; charset=utf-8"
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
    response.writeHead(404);
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
const alertKey = "stock-provider-webui.alert-drafts.v1";
const preferenceKey = "stock-provider-webui.notification-preferences.v1";
const alertDraft = (overrides = {}) => ({
  id: "alert-1", name: "初始规则", type: "price_above", threshold: 100,
  symbol: "600519.SH", scope: "symbol", severity: "warning", enabled: true,
  createdAt: "2026-11-04T01:00:00.000Z", updatedAt: "2026-11-04T01:00:00.000Z",
  ...overrides
});
const preferences = (overrides = {}) => ({
  events: ["task_completed", "task_failed"], quietStart: "22:00", quietEnd: "08:00", ...overrides
});
let browser;

try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const first = await context.newPage();
  const second = await context.newPage();
  const pageErrors = [];
  first.on("pageerror", error => pageErrors.push(error.stack || error.message));

  for (const page of [first, second]) {
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.origin === baseUrl && (url.pathname.includes("/api/") || /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(url.pathname))) {
        return route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ status: "unavailable", error: "isolated browser fixture" }) });
      }
      return route.continue();
    });
    await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
    await page.locator("#alert-rule-form").waitFor({ state: "attached" });
  }

  await first.locator('.nav-link[data-view="alerts"]').click();
  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), { key: alertKey, item: alertDraft() });
  const firstCard = first.locator(".local-alert-draft-card").filter({ hasText: "初始规则" });
  await firstCard.waitFor();
  await firstCard.locator('[data-action="edit"]').click();
  await first.locator("#alert-rule-name").fill("本地未保存规则");
  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), {
    key: alertKey, item: alertDraft({ name: "远端更新规则", updatedAt: "2026-11-04T02:00:00.000Z" })
  });
  await first.locator("#local-alert-draft-feedback").filter({ hasText: "其他标签页修改" }).waitFor();
  assert.equal(await first.locator("#alert-rule-name").inputValue(), "本地未保存规则", "remote update must not reset the local editing form");
  await first.locator("#alert-rule-submit").click();
  assert.match(await first.locator("#local-alert-draft-feedback").textContent(), /当前编辑未保存/);
  assert.equal(await first.evaluate(key => JSON.parse(localStorage.getItem(key))[0].name, alertKey), "远端更新规则",
    "saving a stale alert draft must not overwrite the remote version");

  await first.locator("#alert-rule-cancel").click();
  await first.locator(".local-alert-draft-card").filter({ hasText: "远端更新规则" }).locator('[data-action="edit"]').click();
  assert.equal(await first.locator("#alert-rule-name").inputValue(), "远端更新规则");
  await first.locator("#alert-rule-name").fill("本地基于新版本编辑");
  await first.locator("#alert-rule-submit").click();
  await first.waitForFunction(key => JSON.parse(localStorage.getItem(key))[0].name === "本地基于新版本编辑", alertKey);

  await first.locator(".local-alert-draft-card").filter({ hasText: "本地基于新版本编辑" }).locator('[data-action="edit"]').click();
  await first.locator("#alert-rule-name").fill("被删除规则的旧草稿");
  await second.evaluate(key => localStorage.setItem(key, "[]"), alertKey);
  await first.locator("#local-alert-draft-feedback").filter({ hasText: "删除了正在编辑的规则" }).waitFor();
  await first.locator("#alert-rule-submit").click();
  assert.equal(await first.evaluate(key => JSON.parse(localStorage.getItem(key)).length, alertKey), 0,
    "a deleted alert draft must not be recreated by a stale edit");
  await first.locator("#alert-rule-cancel").click();

  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), {
    key: alertKey, item: alertDraft({ id: "alert-2", name: "外部新增规则" })
  });
  await first.locator(".local-alert-draft-card").filter({ hasText: "外部新增规则" }).waitFor();
  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), {
    key: alertKey, item: alertDraft({ id: "alert-2", name: "外部启停规则", enabled: false })
  });
  await first.locator(".local-alert-draft-card").filter({ hasText: "外部启停规则" }).waitFor();
  await second.evaluate(key => localStorage.removeItem(key), alertKey);
  await first.waitForFunction(() => document.querySelector("#local-alert-draft-list")?.textContent.includes("尚无本地规则草稿。"));

  await first.locator('.nav-link[data-view="settings"]').click();
  await first.locator('[data-settings-target="settings-notification-section"]').click();
  await first.locator("#notification-preferences-form").waitFor({ state: "visible" });
  await second.evaluate(({ key, value }) => localStorage.setItem(key, JSON.stringify(value)), {
    key: preferenceKey, value: preferences({ events: ["risk_blocked"], quietStart: "21:30", quietEnd: "07:30" })
  });
  await first.waitForFunction(() => document.querySelector("#notification-quiet-start").value === "21:30");
  assert.equal(await first.locator('[name="notification-event"][value="risk_blocked"]').isChecked(), true);
  assert.equal(await first.locator('[name="notification-event"][value="task_completed"]').isChecked(), false);

  await first.locator("#notification-quiet-start").fill("20:15");
  await second.evaluate(({ key, value }) => localStorage.setItem(key, JSON.stringify(value)), {
    key: preferenceKey, value: preferences({ events: ["system_error"], quietStart: "23:00", quietEnd: "06:00" })
  });
  await first.locator("#notification-preferences-feedback").filter({ hasText: "其他标签页修改" }).waitFor();
  assert.equal(await first.locator("#notification-quiet-start").inputValue(), "20:15",
    "remote preference update must preserve a dirty local form");
  await first.locator("#notification-preferences-save").click();
  assert.match(await first.locator("#notification-preferences-feedback").textContent(), /当前表单已保留且未保存/);
  assert.deepEqual(await first.evaluate(key => JSON.parse(localStorage.getItem(key)), preferenceKey),
    preferences({ events: ["system_error"], quietStart: "23:00", quietEnd: "06:00" }),
    "stale notification preference form must not overwrite remote values");

  await first.locator("#notification-quiet-start").fill("22:30");
  await second.evaluate(({ key, value }) => localStorage.setItem(key, JSON.stringify(value)), {
    key: preferenceKey, value: preferences({ events: ["task_failed"], quietStart: "22:00", quietEnd: "08:00" })
  });
  await first.locator("#notification-preferences-feedback").filter({ hasText: "其他标签页修改" }).waitFor();
  await first.locator("#notification-preferences-save").click();
  assert.deepEqual(await first.evaluate(key => JSON.parse(localStorage.getItem(key)), preferenceKey),
    preferences({ events: ["task_failed"], quietStart: "22:00", quietEnd: "08:00" }),
    "save-time snapshot comparison must catch changes even if the storage event was not observed");

  await first.reload({ waitUntil: "domcontentloaded" });
  await first.locator("#notification-preferences-form").waitFor({ state: "attached" });
  await first.locator('.nav-link[data-view="settings"]').click();
  await first.locator('[data-settings-target="settings-notification-section"]').click();
  await first.locator("#notification-preferences-form").waitFor({ state: "visible" });
  await second.evaluate(() => localStorage.clear());
  await first.waitForFunction(() => document.querySelector("#notification-quiet-start").value === "22:00"
    && document.querySelector("#notification-quiet-end").value === "08:00");
  await first.waitForFunction(() => document.querySelector("#local-alert-draft-list")?.textContent.includes("尚无本地规则草稿。"));
  assert.equal(await first.locator("#notification-preferences-save").isDisabled(), false,
    "localStorage.clear must restore editable defaults instead of leaving stale disabled controls");

  await second.evaluate(key => localStorage.setItem(key, "{"), preferenceKey);
  await first.waitForFunction(() => document.querySelector("#notification-preferences-save")?.disabled === true);
  assert.match(await first.locator("#notification-preferences-feedback").textContent(), /格式异常/);
  await second.evaluate(() => localStorage.clear());
  await first.waitForFunction(() => !document.querySelector("#notification-preferences-save").disabled);

  assert.deepEqual(pageErrors, [], "alerts and notification cross-tab synchronization must not raise page errors");
  console.log("Alerts and notification preferences cross-tab browser acceptance passed.");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

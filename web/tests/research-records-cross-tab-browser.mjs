import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const contentTypes = { ".html": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8", ".json": "application/json; charset=utf-8" };
const server = createServer(async (request, response) => {
  const pathname = decodeURIComponent(new URL(request.url || "/", "http://127.0.0.1").pathname);
  const target = path.resolve(webRoot, "." + (pathname === "/" ? "/index.html" : pathname));
  if (target !== webRoot && !target.startsWith(webRoot + path.sep)) { response.writeHead(403); response.end("Forbidden"); return; }
  try { const body = await readFile(target); response.writeHead(200, { "content-type": contentTypes[path.extname(target)] || "application/octet-stream", "cache-control": "no-store" }); response.end(body); }
  catch { response.writeHead(404); response.end("Not found"); }
});
await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
const address = server.address();
assert.ok(address && typeof address === "object");
const baseUrl = "http://127.0.0.1:" + address.port;
let browser;
const noteKey = "stock-provider-webui.research-notes.v1";
const reportKey = "stock-provider-webui.research-reports.v1";
const note = (id, title, content, updatedAt = "2026-11-04T01:00:00.000Z") => ({ id, title, content, symbol: "600519.SH", taskId: "task-1", runId: "run-1", tags: ["基本面"], createdAt: "2026-11-03T01:00:00.000Z", updatedAt });
const report = (id = "report-1", runId = "run-1") => ({ id, runId, taskId: "task-1", analysisBase: "/analysis-api", symbols: ["600519.SH"], status: "completed", corePass: true, asOf: "2026-11-04T09:30:00+08:00", createdAt: "2026-11-04T01:00:00.000Z", result: { run_id: runId, status: "completed", request: { symbols: ["600519.SH"], as_of: "2026-11-04T09:30:00+08:00" }, quality: { core_pass: true }, signals: [], warnings: ["<img src=x onerror=alert(1)>", ...Array.from({ length: 24 }, (_, index) => "长报告滚动验收内容 " + (index + 1))] } });

try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const first = await context.newPage();
  const second = await context.newPage();
  const errors = [];
  first.on("pageerror", error => errors.push(error.stack || error.message));
  for (const page of [first, second]) {
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.origin === baseUrl && (url.pathname.includes("/api/") || /\/(?:health\/(?:ready|live)|readyz|healthz)$/.test(url.pathname))) {
        return route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ status: "unavailable", error: "isolated browser fixture" }) });
      }
      return route.continue();
    });
    await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
    await page.locator("#research-note-form").waitFor({ state: "attached" });
    await page.locator('.nav-link[data-view="research"]').click();
  }

  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), { key: noteKey, item: note("note-1", "初始笔记", "初始内容") });
  await first.locator(".research-note-card").filter({ hasText: "初始笔记" }).waitFor();
  assert.equal(await first.locator("#research-note-editor").isVisible(), false, "note editor is not permanently occupying the list workspace");
  assert.equal(await first.locator("#research-note-count").textContent(), "显示 1 / 1 条记录");
  await first.locator("#research-note-tag-filter").selectOption("基本面");
  assert.equal(await first.locator(".research-note-card").count(), 1, "tag filter selects matching notes");
  await first.locator("#research-note-clear").click();
  await first.locator("#research-note-create").click();
  await first.locator("#research-note-editor:not([hidden])").waitFor();
  for (const width of [320, 768, 1280, 1600]) {
    await first.setViewportSize({ width, height: 900 });
    await first.waitForTimeout(50);
    assert.ok(await first.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), "note editor must not overflow at " + width + "px");
    const bounds = await first.locator("#research-note-editor").evaluate(node => {
      const rect = node.getBoundingClientRect();
      return { width: rect.width, right: rect.right };
    });
    assert.ok(bounds.width > 0 && bounds.right <= width + 1, "note editor stays within viewport at " + width + "px");
  }
  await first.setViewportSize({ width: 1280, height: 900 });
  await first.locator("#research-note-editor-back").click();
  await first.locator("#research-note-editor[hidden]").waitFor({ state: "hidden" });
  await first.locator('[data-note-action="edit"][data-note-id="note-1"]').click();
  await first.locator("#research-note-editor:not([hidden])").waitFor();
  assert.equal(await first.locator(".research-note-card").filter({ hasText: "初始笔记" }).isVisible(), true, "note list remains visible while editing");
  await first.locator("#research-note-title").fill("本地旧草稿");
  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), { key: noteKey, item: note("note-1", "远端新版本", "远端内容", "2026-11-04T02:00:00.000Z") });
  await first.locator(".research-note-card").filter({ hasText: "远端新版本" }).waitFor();
  assert.equal(await first.locator(".research-note-card").filter({ hasText: "远端新版本" }).isVisible(), true, "list refresh remains visible while the editor preserves its draft");
  assert.equal(await first.locator("#research-note-title").inputValue(), "本地旧草稿", "cross-tab list refresh must not reset the editing draft");
  await first.locator("#research-note-save").click();
  await first.locator("#research-note-feedback").filter({ hasText: "已被其他标签页修改" }).waitFor();
  assert.equal(await first.evaluate(key => JSON.parse(localStorage.getItem(key))[0].title, noteKey), "远端新版本", "stale draft must not overwrite remote update");
  await first.getByRole("button", { name: "重新载入最新版本" }).click();
  assert.equal(await first.locator("#research-note-title").inputValue(), "远端新版本");

  await first.locator('[data-note-action="edit"][data-note-id="note-1"]').click();
  await first.locator("#research-note-title").fill("删除后的旧草稿");
  await second.evaluate(key => localStorage.setItem(key, "[]"), noteKey);
  await first.locator(".research-note-list").getByText("尚无研究记录。").waitFor();
  await first.locator("#research-note-save").click();
  await first.locator("#research-note-feedback").filter({ hasText: "已被其他标签页删除" }).waitFor();
  await first.getByRole("button", { name: "另存为新笔记" }).click();
  await first.locator("#research-note-save").click();
  const recreated = await first.evaluate(key => JSON.parse(localStorage.getItem(key)), noteKey);
  assert.equal(recreated.length, 1);
  assert.notEqual(recreated[0].id, "note-1", "deleted record must only be preserved as a new note with a new ID");

  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), { key: reportKey, item: report() });
  await first.locator(".research-report-card").filter({ hasText: "run-1" }).waitFor();
  await first.locator("#research-report-search").fill("600519");
  await first.locator('[data-report-action="open-report"]').click();
  await first.locator("#research-report-detail:not([hidden])").waitFor();
  assert.equal(await first.locator("#research-report-list").isVisible(), false, "report list is replaced by a dedicated detail workspace");
  assert.equal(await first.locator("#research-report-detail-title").textContent(), "600519.SH");
  assert.ok(await first.locator("#research-report-document h2").count() >= 4, "Run result sections render as semantic headings");
  assert.ok(await first.locator("#research-report-toc button[data-report-section]").count() >= 4, "table of contents exposes section navigation controls");
  assert.equal(await first.locator("#research-report-document img").count(), 0, "report content is rendered as text and never interpreted as HTML");
  assert.ok(await first.locator("#research-report-document").evaluate(node => node.scrollHeight > window.innerHeight), "long report content remains readable in the page scroll area");
  for (const width of [320, 768, 1280, 1600]) {
    await first.setViewportSize({ width, height: 900 });
    await first.waitForTimeout(50);
    assert.ok(await first.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), "report detail must not overflow at " + width + "px");
    const bounds = await first.locator("#research-report-document").evaluate(node => {
      const rect = node.getBoundingClientRect();
      return { width: rect.width, right: rect.right };
    });
    assert.ok(bounds.width > 0 && bounds.right <= width + 1, "report document stays within viewport at " + width + "px");
    assert.equal(await first.locator("#research-report-toc").isVisible(), true, "report table of contents remains available at " + width + "px");
  }
  await first.setViewportSize({ width: 1280, height: 900 });
  const firstSection = first.locator("#research-report-toc button[data-report-section]").first();
  const targetId = await firstSection.getAttribute("data-report-section");
  await firstSection.click();
  assert.equal(await first.locator("#" + targetId).count(), 1, "table of contents points to a rendered report section");
  await first.locator("#research-report-back").click();
  await first.locator("#research-report-list:not([hidden])").waitFor();
  assert.equal(await first.locator("#research-report-detail").isVisible(), false, "back action restores the report list");
  assert.equal(await first.locator("#research-report-search").inputValue(), "600519", "back action preserves the report search filter");
  await first.locator('[data-note-action="edit"][data-note-id="' + recreated[0].id + '"]').click();
  await first.locator("#research-note-title").fill("冲突中的旧草稿");
  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), {
    key: noteKey, item: { ...recreated[0], title: "远端更新后的笔记", updatedAt: "2026-11-05T03:00:00.000Z" }
  });
  await first.locator(".research-note-card").filter({ hasText: "远端更新后的笔记" }).waitFor();
  await first.locator("#research-note-save").click();
  await first.locator("#research-note-feedback").filter({ hasText: "已被其他标签页修改" }).waitFor();
  assert.equal(await first.locator(".research-note-conflict-actions").isVisible(), true);

  await second.evaluate(({ key, item }) => localStorage.setItem(key, JSON.stringify([item])), { key: reportKey, item: report() });
  await first.locator(".research-report-card").filter({ hasText: "run-1" }).waitFor();
  assert.equal(await first.locator("#research-report-count").textContent(), "显示 1 / 1 份报告");
  await first.locator("#research-report-search").fill("no-such-report");
  assert.equal(await first.locator("#research-report-count").textContent(), "显示 0 / 1 份报告");
  await first.locator("#research-report-list").getByText("没有匹配的研究报告。").waitFor();
  await first.locator("#research-report-clear").click();
  assert.equal(await first.locator("#research-report-count").textContent(), "显示 1 / 1 份报告");
  assert.equal(await first.locator("#research-report-clear").isDisabled(), true);
  first.once("dialog", dialog => dialog.accept());
  await first.locator('#research-report-list [data-report-action="create-note"]').click();
  assert.equal(await first.locator(".research-note-conflict-actions").isVisible(), false,
    "creating a note from a report must clear the previous note conflict actions");
  assert.equal(await first.locator("#research-note-title").inputValue(), "Run run-1 研究记录");
  await first.locator("#research-note-content").fill("基于报告形成的独立研究判断");
  await first.locator("#research-note-save").click();
  const reportLinkedNotes = await first.evaluate(key => JSON.parse(localStorage.getItem(key)), noteKey);
  assert.equal(reportLinkedNotes[0].runId, "run-1");
  assert.equal(reportLinkedNotes[0].taskId, "task-1");
  assert.equal(reportLinkedNotes[0].symbol, "600519.SH");

  await first.locator('[data-report-action="open-report"]').click();
  await first.locator("#research-report-detail:not([hidden])").waitFor();
  await second.evaluate(key => localStorage.removeItem(key), reportKey);
  await first.locator("#research-report-list").getByText("尚无归档报告。").waitFor();
  assert.equal(await first.locator("#research-report-detail").isVisible(), false, "cross-tab report deletion closes the stale detail workspace");
  await first.locator("#research-report-feedback").filter({ hasText: "其他标签页删除" }).waitFor();
  assert.equal(await first.evaluate(key => JSON.parse(localStorage.getItem(key)).length, noteKey), 2,
    "deleting a report must not cascade-delete independent research notes");

  const clearNote = note("clear-note", "待清理笔记", "内容");
  const clearReport = report();
  await second.evaluate(({ noteKey, reportKey, clearNote, clearReport }) => {
    localStorage.setItem(noteKey, JSON.stringify([clearNote]));
    localStorage.setItem(reportKey, JSON.stringify([clearReport]));
  }, { noteKey, reportKey, clearNote, clearReport });
  await first.locator(".research-note-card").filter({ hasText: "待清理笔记" }).waitFor();
  await first.locator(".research-report-card").filter({ hasText: "run-1" }).waitFor();
  await second.evaluate(() => localStorage.clear());
  await first.locator("#research-note-list").getByText("尚无研究记录。").waitFor();
  await first.locator("#research-report-list").getByText("尚无归档报告。").waitFor();

  for (const width of [320, 768, 1280, 1600]) {
    await first.setViewportSize({ width, height: 900 });
    await first.waitForTimeout(50);
    assert.ok(await first.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), "research records page must not overflow at viewport " + width);
  }
  assert.deepEqual(errors, [], "cross-tab research records flow must not raise page errors");
  console.log("Research records cross-tab browser acceptance passed.");
} finally {
  if (browser) await browser.close();
  await new Promise(resolve => server.close(resolve));
}

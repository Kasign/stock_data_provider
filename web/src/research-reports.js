import { buildRunResultMarkdown } from "./report.js";
export const RESEARCH_REPORTS_KEY = "stock-provider-webui.research-reports.v1";
export const MAX_RESEARCH_REPORTS = 30;
const MAX_REPORT_CHARS = 700000, MAX_TOTAL_CHARS = 4000000;
function safeStorage(storage) { if (storage) return storage; try { return globalThis.localStorage; } catch { return null; } }
function normalizeReport(item) {
  if (!item || typeof item !== "object" || Array.isArray(item) ||
      !item.result || typeof item.result !== "object" || Array.isArray(item.result)) return null;
  if (typeof item.id !== "string" || !item.id.trim() ||
      typeof item.runId !== "string" || !item.runId.trim() || item.result.run_id !== item.runId.trim()) return null;
  const analysisBase = item.analysisBase === undefined ? "" : item.analysisBase;
  const taskId = item.taskId === undefined ? "" : item.taskId;
  const status = item.status === undefined ? (item.result.status || "") : item.status;
  const asOf = item.asOf === undefined ? "" : item.asOf;
  const createdAt = item.createdAt === undefined ? "" : item.createdAt;
  const symbols = item.symbols === undefined ? [] : item.symbols;
  if (typeof analysisBase !== "string" || typeof taskId !== "string" || taskId.length > 160 ||
      typeof status !== "string" || status.length > 80 || typeof asOf !== "string" || asOf.length > 80 ||
      typeof createdAt !== "string" || !Array.isArray(symbols) || symbols.length > 50 ||
      symbols.some(value => typeof value !== "string" || !value.trim() || value.trim().length > 32) ||
      (item.corePass !== undefined && item.corePass !== null && typeof item.corePass !== "boolean")) return null;
  let serialized; try { serialized = JSON.stringify(item.result); } catch { return null; }
  if (!serialized || serialized.length > MAX_REPORT_CHARS) return null;
  return { id: item.id.trim(), runId: item.runId.trim(), taskId, analysisBase: analysisBase.trim(),
    symbols: symbols.map(value => value.trim()), status, corePass: item.corePass ?? null,
    asOf, createdAt, result: item.result };
}
export function readResearchReports(storage) {
  try {
    const target = safeStorage(storage); if (!target) return { items: [], readable: false, valid: false };
    const raw = target.getItem(RESEARCH_REPORTS_KEY); if (raw === null) return { items: [], readable: true, valid: true };
    if (typeof raw === "string" && raw.length > MAX_TOTAL_CHARS) return { items: [], readable: true, valid: false };
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length > MAX_RESEARCH_REPORTS) return { items: [], readable: true, valid: false };
    const items = parsed.map(normalizeReport);
    if (items.some(x => !x) || new Set(items.map(x => x.id)).size !== items.length) return { items: [], readable: true, valid: false };
    return { items, readable: true, valid: true };
  } catch { return { items: [], readable: false, valid: false }; }
}
export function writeResearchReports(items, storage) {
  if (!Array.isArray(items) || items.length > MAX_RESEARCH_REPORTS) return { ok: false, reason: "limit" };
  const normalized = items.map(normalizeReport); if (normalized.some(x => !x) || new Set(normalized.map(x => x.id)).size !== normalized.length) return { ok: false, reason: "invalid" };
  let raw; try { raw = JSON.stringify(normalized); } catch { return { ok: false, reason: "invalid" }; }
  if (raw.length > MAX_TOTAL_CHARS) return { ok: false, reason: "storage-limit" };
  try {
    const target = safeStorage(storage); if (!target) return { ok: false, reason: "unavailable" };
    const current = readResearchReports(target); if (!current.readable || !current.valid) return { ok: false, reason: "invalid-storage" };
    target.setItem(RESEARCH_REPORTS_KEY, raw);
    const check = readResearchReports(target);
    return check.readable && check.valid && JSON.stringify(check.items) === JSON.stringify(normalized) ? { ok: true } : { ok: false, reason: "verify" };
  } catch { return { ok: false, reason: "unavailable" }; }
}
export function saveResearchReport(result, context = {}, storage) {
  if (!result || typeof result !== "object" || typeof result.run_id !== "string" || !result.run_id.trim()) return { ok: false, reason: "invalid-result" };
  let cloned; try { cloned = JSON.parse(JSON.stringify(result)); } catch { return { ok: false, reason: "invalid-result" }; }
  const current = readResearchReports(storage); if (!current.readable || !current.valid) return { ok: false, reason: "invalid-storage" };
  const analysisBase = String(context.analysisBase || "").trim();
  const existing = current.items.find(x => x.runId === cloned.run_id && x.analysisBase === analysisBase);
  if (existing) return { ok: true, duplicate: true, item: existing };
  const item = normalizeReport({ id: "report-" + Date.now() + "-" + Math.random().toString(36).slice(2, 9),
    runId: cloned.run_id, taskId: context.taskId || "", analysisBase, symbols: cloned.request?.symbols || [],
    status: cloned.status, corePass: typeof cloned.quality?.core_pass === "boolean" ? cloned.quality.core_pass : null,
    asOf: cloned.request?.as_of || cloned.market_state?.as_of || "", createdAt: new Date().toISOString(), result: cloned });
  if (!item) return { ok: false, reason: "invalid-result" };
  if (current.items.length >= MAX_RESEARCH_REPORTS) return { ok: false, reason: "limit" };
  const saved = writeResearchReports([item, ...current.items], storage);
  return saved.ok ? { ok: true, duplicate: false, item } : { ok: false, reason: saved.reason };
}
export function filterResearchReports(items, filters = {}) {
  const q = String(filters.search || "").trim().toLowerCase(), from = String(filters.from || ""), to = String(filters.to || "");
  if (from && to && from > to) return [];
  const localDate = value => {
    const time = Date.parse(value || ""); if (!Number.isFinite(time)) return "";
    const d = new Date(time); return [d.getFullYear(), String(d.getMonth() + 1).padStart(2, "0"), String(d.getDate()).padStart(2, "0")].join("-");
  };
  return (Array.isArray(items) ? items : []).filter(x => {
    if (from || to) { const date = localDate(x.createdAt); if (!date || (from && date < from) || (to && date > to)) return false; }
    return !q || [x.runId, x.taskId, x.status, x.asOf, ...x.symbols].join(" ").toLowerCase().includes(q);
  }).sort((a, b) => String(b.createdAt).localeCompare(String(a.createdAt)));
}
export function bindResearchReports({ root = document, storage, onOpenTask = () => {}, onCreateNote = () => {} } = {}) {
  const $ = selector => root.querySelector(selector.startsWith("#") ? selector : "#" + selector), host = $("#research-report-list");
  if (!host) return { save: () => ({ ok: false, reason: "unavailable" }) };
  const feedback = (message, error = false) => { const node = $("#research-report-feedback"); node.textContent = message; node.dataset.state = error ? "error" : "ok"; };
  const render = () => {
    host.replaceChildren(); const state = readResearchReports(storage);
    if (!state.readable || !state.valid) { const p = document.createElement("p"); p.className = "table-empty"; p.textContent = "报告归档存储不可读取或格式异常；为保护原始报告，当前未修改数据。"; host.append(p); return; }
    const items = filterResearchReports(state.items, { search: $("#research-report-search").value, from: $("#research-report-from").value, to: $("#research-report-to").value });
    if (!items.length) { const p = document.createElement("p"); p.className = "table-empty"; p.textContent = state.items.length ? "没有匹配的研究报告。" : "尚无归档报告。请在分析任务的真实 Run 结果中选择“归档到研究记录”。"; host.append(p); return; }
    items.forEach(item => {
      const card = document.createElement("article"); card.className = "research-report-card";
      const title = document.createElement("h3"); title.textContent = item.symbols.join("、") || "未报告标的";
      const meta = document.createElement("p"); meta.className = "research-report-meta";
      meta.textContent = ["Run " + item.runId, item.taskId ? "Task " + item.taskId : "未关联任务", item.status || "状态未报告", item.asOf || "分析时点未报告", item.corePass === true ? "核心门禁通过" : item.corePass === false ? "核心门禁未通过" : "质量门禁未报告"].join(" · ");
      const details = document.createElement("details"), summary = document.createElement("summary"); summary.textContent = "查看报告详情";
      const pre = document.createElement("pre"); pre.className = "research-report-markdown"; pre.textContent = buildRunResultMarkdown(item.result); details.append(summary, pre);
      const actions = document.createElement("div"); actions.className = "research-report-actions";
      [["下载 Markdown", "download"], ["基于报告写笔记", "create-note"], ["打开关联任务", "open-task"], ["删除归档", "delete"]].forEach(([label, action]) => {
        const button = document.createElement("button"); button.type = "button"; button.className = "button button-secondary";
        button.textContent = label; button.dataset.reportAction = action; button.dataset.reportId = item.id;
        if (action === "open-task") button.disabled = !item.taskId;
        actions.append(button);
      });
      card.append(title, meta, details, actions); host.append(card);
    });
  };
  const save = (result, context = {}) => {
    const out = saveResearchReport(result, context, storage);
    if (!out.ok) {
      const messages = { "invalid-result": "当前没有可归档的有效 Run 结果。", "invalid-storage": "报告归档存储异常，未覆盖原数据。", "limit": "已达到 30 份报告上限，请先整理旧报告。", "storage-limit": "报告体积超过本地归档容量限制，请下载保存后再整理。", "unavailable": "浏览器本地存储不可用。" };
      feedback(messages[out.reason] || "报告归档失败，未确认写入。", true); return out;
    }
    feedback(out.duplicate ? "该 Run 已归档，无需重复保存。" : "Run 报告已归档到当前浏览器。"); render(); return out;
  };
  $("#research-report-search").addEventListener("input", render); $("#research-report-from").addEventListener("change", render); $("#research-report-to").addEventListener("change", render);
  host.addEventListener("click", event => {
    const button = event.target.closest("[data-report-action]"); if (!button) return;
    const current = readResearchReports(storage); if (!current.readable || !current.valid) { feedback("归档存储异常，未执行操作。", true); return; }
    const item = current.items.find(x => x.id === button.dataset.reportId); if (!item) return;
    if (button.dataset.reportAction === "open-task") { onOpenTask(item); return; }
    if (button.dataset.reportAction === "create-note") { onCreateNote(item); return; }
    if (button.dataset.reportAction === "download") {
      const blob = new Blob([buildRunResultMarkdown(item.result)], { type: "text/markdown;charset=utf-8" });
      const url = URL.createObjectURL(blob), anchor = document.createElement("a"); anchor.href = url;
      anchor.download = "run-" + item.runId.replace(/[^a-zA-Z0-9._-]/g, "_") + ".md"; anchor.click(); window.setTimeout(() => URL.revokeObjectURL(url), 0); return;
    }
    if (button.dataset.reportAction === "delete") {
      if (!globalThis.confirm("确定删除这份本地归档报告？")) return;
      const saved = writeResearchReports(current.items.filter(x => x.id !== item.id), storage);
      feedback(saved.ok ? "报告归档已删除。" : "删除失败，未能确认存储结果。", !saved.ok); render();
    }
  });
  render(); return { save, render };
}

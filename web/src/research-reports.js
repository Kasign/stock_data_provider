import { buildRunResultMarkdown } from "./report.js";
export const RESEARCH_REPORTS_KEY = "stock-provider-webui.research-reports.v1";
export const MAX_RESEARCH_REPORTS = 30;
const MAX_REPORT_CHARS = 700000, MAX_TOTAL_CHARS = 4000000;
function safeStorage(storage) { if (storage) return storage; try { return globalThis.localStorage; } catch { return null; } }
function isJsonValue(value, ancestors = new WeakSet()) {
  if (value === null || typeof value === "string" || typeof value === "boolean") return true;
  if (typeof value === "number") return Number.isFinite(value) && !Object.is(value, -0);
  if (typeof value !== "object" || ancestors.has(value)) return false;
  ancestors.add(value);
  try {
    const array = Array.isArray(value);
    const prototype = Object.getPrototypeOf(value);
    if (array ? prototype !== Array.prototype : prototype !== Object.prototype && prototype !== null) return false;
    const keys = Reflect.ownKeys(value);
    if (keys.some(key => typeof key !== "string")) return false;
    if (array) {
      if (keys.length !== value.length + 1 || !keys.includes("length")) return false;
      for (let index = 0; index < value.length; index++) {
        if (!Object.prototype.hasOwnProperty.call(value, index)) return false;
      }
    }
    for (const key of keys) {
      if (array && key === "length") continue;
      if (array && (!/^(0|[1-9]\d*)$/.test(key) || Number(key) >= value.length)) return false;
      const descriptor = Object.getOwnPropertyDescriptor(value, key);
      if (!descriptor || !descriptor.enumerable || !Object.prototype.hasOwnProperty.call(descriptor, "value") ||
          !isJsonValue(descriptor.value, ancestors)) return false;
    }
    return true;
  } catch {
    return false;
  } finally {
    ancestors.delete(value);
  }
}
function normalizeReport(item) {
  if (!item || typeof item !== "object" || Array.isArray(item) ||
      !item.result || typeof item.result !== "object" || Array.isArray(item.result)) return null;
  if (typeof item.id !== "string" || !item.id.trim() || item.id.trim().length > 160 ||
      typeof item.runId !== "string" || !item.runId.trim() || item.runId.trim().length > 160 ||
      item.result.run_id !== item.runId.trim()) return null;
  const analysisBase = item.analysisBase === undefined ? "" : item.analysisBase;
  const taskId = item.taskId === undefined ? "" : item.taskId;
  const status = item.status === undefined ? (item.result.status || "") : item.status;
  const asOf = item.asOf === undefined ? "" : item.asOf;
  const createdAt = item.createdAt === undefined ? "" : item.createdAt;
  const symbols = item.symbols === undefined ? [] : item.symbols;
  if (typeof analysisBase !== "string" || analysisBase.length > 2048 || typeof taskId !== "string" || taskId.length > 160 ||
      typeof status !== "string" || status.length > 80 || typeof asOf !== "string" || asOf.length > 80 ||
      typeof createdAt !== "string" || createdAt.length > 64 || !Array.isArray(symbols) || symbols.length > 100 ||
      symbols.some(value => typeof value !== "string" || !value.trim() || value.trim().length > 32) ||
      (item.corePass !== undefined && item.corePass !== null && typeof item.corePass !== "boolean")) return null;
  let serialized; try {
    if (!isJsonValue(item.result)) return null;
    serialized = JSON.stringify(item.result);
  } catch { return null; }
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
  if (!result || typeof result !== "object" || Array.isArray(result) || !isJsonValue(result) ||
      typeof result.run_id !== "string" || !result.run_id.trim()) return { ok: false, reason: "invalid-result" };
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
function appendInlineMarkdown(parent, value) {
  const text = String(value || "");
  const pattern = new RegExp("(\\x60+)(.+?)\\1", "g");
  const appendPlainText = value => parent.append(document.createTextNode(
    value.replace(/\\\\([\\\\x60*_{}\\[\\]()#+.!|>~-])/g, "$1")
  ));
  let offset = 0, match;
  while ((match = pattern.exec(text))) {
    if (match.index > offset) appendPlainText(text.slice(offset, match.index));
    const code = document.createElement("code");
    code.textContent = match[2];
    parent.append(code);
    offset = pattern.lastIndex;
  }
  if (offset < text.length) appendPlainText(text.slice(offset));
}

export function renderResearchReportMarkdown(markdown, contentHost, tocHost) {
  contentHost.replaceChildren();
  tocHost.replaceChildren();
  const lines = String(markdown || "").replace(/\r\n?/g, "\n").split("\n");
  const fence = String.fromCharCode(96).repeat(3);
  let index = 0, sectionIndex = 0, activeContainer = contentHost;
  const appendParagraph = value => {
    const paragraph = document.createElement("p");
    appendInlineMarkdown(paragraph, value);
    activeContainer.append(paragraph);
  };
  while (index < lines.length) {
    const line = lines[index];
    if (!line.trim()) { index++; continue; }
    const heading = line.match(/^(#{1,3})\s+(.+)$/);
    if (heading) {
      const level = heading[1].length;
      const element = document.createElement("h" + level);
      element.textContent = heading[2].replace(/\\([\\x60*_{}\[\]()#+.!|>~-])/g, "$1");
      if (level === 1) {
        element.className = "research-report-document-title";
        activeContainer = contentHost;
        contentHost.append(element);
      } else if (level === 2) {
        const section = document.createElement("section");
        section.className = "research-report-section";
        section.id = "research-report-section-" + (++sectionIndex);
        element.id = section.id + "-heading";
        section.setAttribute("aria-labelledby", element.id);
        section.append(element);
        contentHost.append(section);
        activeContainer = section;
        const item = document.createElement("li");
        const link = document.createElement("button");
        link.type = "button";
        link.className = "research-report-toc-link";
        link.dataset.reportSection = section.id;
        link.textContent = heading[2].replace(/\\\\([\\\\x60*_{}\\[\\]()#+.!|>~-])/g, "$1");
        item.append(link);
        tocHost.append(item);
      } else {
        activeContainer.append(element);
      }
      index++;
      continue;
    }
    if (line.startsWith(fence)) {
      const language = line.slice(fence.length).trim();
      const codeLines = [];
      index++;
      while (index < lines.length && !lines[index].startsWith(fence)) codeLines.push(lines[index++]);
      if (index < lines.length) index++;
      const pre = document.createElement("pre");
      const code = document.createElement("code");
      if (language) code.dataset.language = language;
      code.textContent = codeLines.join("\n");
      pre.append(code);
      activeContainer.append(pre);
      continue;
    }
    if (line.startsWith(">")) {
      const quote = document.createElement("blockquote");
      const paragraph = document.createElement("p");
      appendInlineMarkdown(paragraph, line.replace(/^>\s?/, ""));
      quote.append(paragraph);
      activeContainer.append(quote);
      index++;
      continue;
    }
    if (/^\s*-\s+/.test(line)) {
      const list = document.createElement("ul");
      while (index < lines.length && /^\s*-\s+/.test(lines[index])) {
        const item = document.createElement("li");
        appendInlineMarkdown(item, lines[index++].replace(/^\s*-\s+/, ""));
        list.append(item);
      }
      activeContainer.append(list);
      continue;
    }
    const paragraphLines = [line];
    index++;
    while (index < lines.length && lines[index].trim() &&
      !/^(#{1,3})\s+/.test(lines[index]) && !lines[index].startsWith(">") &&
      !lines[index].startsWith(fence) && !/^\s*-\s+/.test(lines[index])) {
      paragraphLines.push(lines[index++]);
    }
    appendParagraph(paragraphLines.join(" "));
  }
  if (!tocHost.children.length) {
    const item = document.createElement("li");
    item.textContent = "本报告没有可定位的章节。";
    item.className = "research-report-toc-empty";
    tocHost.append(item);
  }
}

export function bindResearchReports({ root = document, storage, onOpenTask = () => {}, onCreateNote = () => {} } = {}) {
  const $ = selector => root.querySelector(selector.startsWith("#") ? selector : "#" + selector);
  const host = $("#research-report-list"), detail = $("#research-report-detail");
  if (!host || !detail) return { save: () => ({ ok: false, reason: "unavailable" }) };
  let activeReportId = "";
  const feedback = (message, error = false) => {
    const node = $("#research-report-feedback");
    node.textContent = message;
    node.dataset.state = error ? "error" : "ok";
  };
  const makeAction = (label, action, item) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "button button-secondary";
    button.textContent = label;
    button.dataset.reportAction = action;
    button.dataset.reportId = item.id;
    if (action === "open-task") button.disabled = !item.taskId;
    return button;
  };
  const renderActions = (item, target, includeOpen = true) => {
    target.replaceChildren();
    if (includeOpen) target.append(makeAction("查看完整报告", "open-report", item));
    target.append(makeAction("下载 Markdown", "download", item),
      makeAction("基于报告写笔记", "create-note", item),
      makeAction("打开关联任务", "open-task", item),
      makeAction("删除归档", "delete", item));
  };
  const renderDetail = item => {
    $("#research-report-detail-title").textContent = item.symbols.join("、") || "未报告标的";
    $("#research-report-detail-meta").textContent = [
      "Run " + item.runId, item.taskId ? "Task " + item.taskId : "未关联任务",
      item.status || "状态未报告", item.asOf || "分析时点未报告",
      item.corePass === true ? "核心门禁通过" : item.corePass === false ? "核心门禁未通过" : "质量门禁未报告"
    ].join(" · ");
    renderActions(item, $("#research-report-detail-actions"), false);
    renderResearchReportMarkdown(buildRunResultMarkdown(item.result),
      $("#research-report-document"), $("#research-report-toc"));
    host.replaceChildren();
    host.hidden = true;
    detail.hidden = false;
  };
  const render = () => {
    host.replaceChildren();
    const state = readResearchReports(storage);
    const count = $("#research-report-count"), clear = $("#research-report-clear");
    if (!state.readable || !state.valid) {
      activeReportId = "";
      detail.hidden = true;
      host.hidden = false;
      count.textContent = "报告归档不可用";
      clear.disabled = true;
      const p = document.createElement("p");
      p.className = "table-empty";
      p.textContent = "报告归档存储不可读取或格式异常；为保护原始报告，当前未修改数据。";
      host.append(p);
      return;
    }
    const filters = { search: $("#research-report-search").value, from: $("#research-report-from").value, to: $("#research-report-to").value };
    const items = filterResearchReports(state.items, filters);
    count.textContent = "显示 " + items.length + " / " + state.items.length + " 份报告";
    clear.disabled = !filters.search.trim() && !filters.from && !filters.to;
    if (!items.length) {
      const p = document.createElement("p");
      p.className = "table-empty";
      p.textContent = state.items.length ? "没有匹配的研究报告。" : "尚无归档报告。请在分析任务的真实 Run 结果中选择“归档到研究记录”。";
      host.append(p);
    } else {
      items.forEach(item => {
        const card = document.createElement("article");
        card.className = "research-report-card";
        const title = document.createElement("h3");
        title.textContent = item.symbols.join("、") || "未报告标的";
        const meta = document.createElement("p");
        meta.className = "research-report-meta";
        meta.textContent = ["Run " + item.runId, item.taskId ? "Task " + item.taskId : "未关联任务",
          item.status || "状态未报告", item.asOf || "分析时点未报告",
          item.corePass === true ? "核心门禁通过" : item.corePass === false ? "核心门禁未通过" : "质量门禁未报告"].join(" · ");
        const actions = document.createElement("div");
        actions.className = "research-report-actions";
        renderActions(item, actions);
        card.append(title, meta, actions);
        host.append(card);
      });
    }
    const selected = activeReportId ? state.items.find(item => item.id === activeReportId) : null;
    if (activeReportId && !selected) {
      activeReportId = "";
      feedback("当前查看的报告已在其他标签页删除。", true);
    }
    if (selected) renderDetail(selected);
    else { detail.hidden = true; host.hidden = false; }
  };
  const save = (result, context = {}) => {
    const out = saveResearchReport(result, context, storage);
    if (!out.ok) {
      const messages = { "invalid-result": "当前没有可归档的有效 Run 结果。", "invalid-storage": "报告归档存储异常，未覆盖原数据。",
        "limit": "已达到 30 份报告上限，请先整理旧报告。", "storage-limit": "报告体积超过本地归档容量限制，请下载保存后再整理。",
        "unavailable": "浏览器本地存储不可用。" };
      feedback(messages[out.reason] || "报告归档失败，未确认写入。", true);
      return out;
    }
    feedback(out.duplicate ? "该 Run 已归档，无需重复保存。" : "Run 报告已归档到当前浏览器。");
    render();
    return out;
  };
  const handleAction = button => {
    const current = readResearchReports(storage);
    if (!current.readable || !current.valid) { feedback("归档存储异常，未执行操作。", true); return; }
    const item = current.items.find(x => x.id === button.dataset.reportId);
    if (!item) { render(); return; }
    const action = button.dataset.reportAction;
    if (action === "open-report") {
      activeReportId = item.id;
      render();
      $("#research-report-detail").scrollIntoView({ behavior: "smooth", block: "start" });
      return;
    }
    if (action === "open-task") { onOpenTask(item); return; }
    if (action === "create-note") { onCreateNote(item); return; }
    if (action === "download") {
      const blob = new Blob([buildRunResultMarkdown(item.result)], { type: "text/markdown;charset=utf-8" });
      const url = URL.createObjectURL(blob), anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = "run-" + item.runId.replace(/[^a-zA-Z0-9._-]/g, "_") + ".md";
      anchor.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
      return;
    }
    if (action === "delete") {
      if (!globalThis.confirm("确定删除这份本地归档报告？")) return;
      const saved = writeResearchReports(current.items.filter(x => x.id !== item.id), storage);
      if (activeReportId === item.id) activeReportId = "";
      feedback(saved.ok ? "报告归档已删除。" : "删除失败，未能确认存储结果。", !saved.ok);
      render();
    }
  };
  $("#research-report-search").addEventListener("input", render);
  $("#research-report-from").addEventListener("change", render);
  $("#research-report-to").addEventListener("change", render);
  $("#research-report-clear").addEventListener("click", () => {
    $("#research-report-search").value = "";
    $("#research-report-from").value = "";
    $("#research-report-to").value = "";
    render();
  });
  $("#research-report-toc").addEventListener("click", event => {
    const button = event.target.closest("[data-report-section]");
    if (!button) return;
    const section = $("#" + button.dataset.reportSection);
    section?.scrollIntoView({ behavior: "smooth", block: "start" });
  });
  $("#research-report-back").addEventListener("click", () => {
    activeReportId = "";
    render();
    $("#research-report-list").scrollIntoView({ behavior: "smooth", block: "start" });
  });
  host.addEventListener("click", event => {
    const button = event.target.closest("[data-report-action]");
    if (button) handleAction(button);
  });
  $("#research-report-detail-actions").addEventListener("click", event => {
    const button = event.target.closest("[data-report-action]");
    if (button) handleAction(button);
  });
  render();
  return { save, render, closeDetail() { activeReportId = ""; render(); } };
}

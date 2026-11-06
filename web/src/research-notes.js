export const RESEARCH_NOTES_KEY = "stock-provider-webui.research-notes.v1";
export const MAX_RESEARCH_NOTES = 200;
const MAX_TITLE = 100, MAX_CONTENT = 10000, MAX_TAGS = 12, MAX_SYMBOLS = 100;
function safeStorage(storage) { if (storage) return storage; try { return globalThis.localStorage; } catch { return null; } }
function normalizeNote(note) {
  if (!note || typeof note !== "object" || Array.isArray(note)) return null;
  const id = typeof note.id === "string" ? note.id.trim() : "";
  const title = typeof note.title === "string" ? note.title.trim() : "";
  const content = typeof note.content === "string" ? note.content.trim() : "";
  const symbol = note.symbol === undefined ? "" : note.symbol;
  const symbols = note.symbols === undefined ? (typeof symbol === "string" && symbol.trim() ? [symbol.trim()] : []) : note.symbols;
  const taskId = note.taskId === undefined ? "" : note.taskId;
  const runId = note.runId === undefined ? "" : note.runId;
  const tags = note.tags === undefined ? [] : note.tags;
  const createdAt = note.createdAt === undefined ? "" : note.createdAt;
  const updatedAt = note.updatedAt === undefined ? "" : note.updatedAt;
  if (!id || id.length > 160 || !title || !content || title.length > MAX_TITLE || content.length > MAX_CONTENT ||
      typeof symbol !== "string" || symbol.trim().length > 32 ||
      !Array.isArray(symbols) || symbols.length > MAX_SYMBOLS ||
      symbols.some(value => typeof value !== "string" || !value.trim() || value.trim().length > 32) ||
      typeof taskId !== "string" || taskId.trim().length > 160 ||
      typeof runId !== "string" || runId.trim().length > 160 ||
      !Array.isArray(tags) || tags.length > MAX_TAGS ||
      tags.some(tag => typeof tag !== "string" || !tag.trim() || tag.trim().length > 40) ||
      typeof createdAt !== "string" || createdAt.length > 64 ||
      typeof updatedAt !== "string" || updatedAt.length > 64) return null;
  const normalizedSymbols = [...new Set(symbols.map(value => value.trim()))];
  const primarySymbol = symbol.trim() || normalizedSymbols[0] || "";
  if (primarySymbol && !normalizedSymbols.includes(primarySymbol)) normalizedSymbols.unshift(primarySymbol);
  if (normalizedSymbols.length > MAX_SYMBOLS) return null;
  return { id, title, content, symbol: primarySymbol, symbols: normalizedSymbols, taskId: taskId.trim(), runId: runId.trim(),
    tags: tags.map(tag => tag.trim()), createdAt, updatedAt };
}
export function readResearchNotes(storage) {
  try {
    const target=safeStorage(storage); if(!target) return {items:[],readable:false,valid:false};
    const raw=target.getItem(RESEARCH_NOTES_KEY);
    if(raw===null) return {items:[],readable:true,valid:true};
    const parsed=JSON.parse(raw);
    if(!Array.isArray(parsed)||parsed.length>MAX_RESEARCH_NOTES) return {items:[],readable:true,valid:false};
    const items=parsed.map(normalizeNote);
    if(items.some(x=>!x)||new Set(items.map(x=>x.id)).size!==items.length) return {items:[],readable:true,valid:false};
    return {items,readable:true,valid:true};
  } catch { return {items:[],readable:false,valid:false}; }
}
export function writeResearchNotes(items, storage) {
  if(!Array.isArray(items)||items.length>MAX_RESEARCH_NOTES) return {ok:false,reason:"limit"};
  const normalized=items.map(normalizeNote);
  if(normalized.some(x=>!x)||new Set(normalized.map(x=>x.id)).size!==normalized.length) return {ok:false,reason:"invalid"};
  try {
    const target=safeStorage(storage); if(!target) return {ok:false,reason:"unavailable"};
    const existing=readResearchNotes(target);
    if(!existing.readable||!existing.valid) return {ok:false,reason:"invalid-storage"};
    target.setItem(RESEARCH_NOTES_KEY,JSON.stringify(normalized));
    const check=readResearchNotes(target);
    return check.readable&&check.valid&&JSON.stringify(check.items)===JSON.stringify(normalized)?{ok:true}:{ok:false,reason:"verify"};
  } catch { return {ok:false,reason:"unavailable"}; }
}
export function buildResearchNoteContextFromReport(report) {
  if (!report || typeof report !== "object") return null;
  const runId = String(report.runId || report.result?.run_id || "").trim();
  if (!runId) return null;
  const rawSymbols = Array.isArray(report.symbols) ? report.symbols : report.result?.request?.symbols;
  const symbols = Array.isArray(rawSymbols) ? [...new Set(rawSymbols.filter(value => typeof value === "string").map(value => value.trim()).filter(Boolean))] : [];
  const symbol = symbols[0] || "";
  return { symbol, symbols, taskId: String(report.taskId || "").trim(), runId };
}

export function filterResearchNotes(items, query = "", symbol = "", tag = "") {
  const q = String(query || "").trim().toLowerCase();
  const s = String(symbol || "").trim().toLowerCase();
  const t = String(tag || "").trim().toLowerCase();
  return (Array.isArray(items) ? items : []).filter(note => {
    const symbols = Array.isArray(note.symbols) ? note.symbols : [note.symbol].filter(Boolean);
    return (!s || symbols.some(value => String(value).toLowerCase().includes(s))) &&
      (!t || note.tags.some(value => value.toLowerCase() === t)) &&
      (!q || [note.title, note.content, note.symbol, ...symbols, note.taskId, note.runId, ...note.tags]
        .join(" ").toLowerCase().includes(q));
  }).sort((a, b) => String(b.updatedAt).localeCompare(String(a.updatedAt)));
}

export function serializeResearchNotesExport(items, exportedAt = new Date().toISOString()) {
  if (!Array.isArray(items) || items.length > MAX_RESEARCH_NOTES) {
    throw new TypeError("研究记录导出数据无效或超过数量上限。");
  }
  const normalized = items.map(normalizeNote);
  if (normalized.some(item => !item) || new Set(normalized.map(item => item.id)).size !== normalized.length) {
    throw new TypeError("研究记录包含无效或重复数据，已取消导出。");
  }
  if (typeof exportedAt !== "string" || exportedAt.length > 64) {
    throw new TypeError("导出时间无效。");
  }
  return JSON.stringify({
    format: "stock-provider-webui.research-notes",
    version: 1,
    exportedAt,
    items: normalized
  }, null, 2);
}


export const MAX_RESEARCH_NOTES_IMPORT_BYTES = 2 * 1024 * 1024;

function parseResearchNotesImport(text) {
  if (typeof text !== "string") throw new TypeError("导入文件内容无效。");
  if (new TextEncoder().encode(text).byteLength > MAX_RESEARCH_NOTES_IMPORT_BYTES) {
    throw new RangeError("导入文件超过 2 MB，已拒绝读取。");
  }
  let payload;
  try { payload = JSON.parse(text); } catch { throw new TypeError("JSON 文件格式损坏，无法解析。"); }
  if (!payload || typeof payload !== "object" || Array.isArray(payload) ||
      payload.format !== "stock-provider-webui.research-notes") {
    throw new TypeError("文件格式不匹配：请选择研究记录 JSON 导出文件。");
  }
  if (payload.version !== 1) throw new TypeError("不支持的研究记录 JSON 版本。");
  if (!Array.isArray(payload.items)) throw new TypeError("导入文件缺少研究记录数组。");
  if (payload.items.length > MAX_RESEARCH_NOTES) throw new RangeError("导入记录超过 200 条上限。");
  return payload.items;
}

export function previewResearchNotesImport(text, currentState) {
  const sourceItems = parseResearchNotesImport(text);
  if (!currentState?.readable || !currentState?.valid) {
    throw new TypeError("当前研究记录存储不可读或数据异常，不能执行导入预览。");
  }
  const sourceIds = sourceItems.map(item => item && typeof item.id === "string" ? item.id.trim() : "");
  const idCounts = new Map();
  sourceIds.forEach(id => { if (id) idCounts.set(id, (idCounts.get(id) || 0) + 1); });
  const invalid = [], additions = [], identical = [], conflicts = [];
  const existingById = new Map(currentState.items.map(item => [item.id, item]));
  sourceItems.forEach((raw, index) => {
    const item = normalizeNote(raw);
    if (!item || !item.id || idCounts.get(item.id) > 1) {
      invalid.push({ index, reason: idCounts.get(sourceIds[index]) > 1 ? "导入文件内存在重复 ID" : "记录字段无效" });
      return;
    }
    const existing = existingById.get(item.id);
    if (!existing) additions.push(item);
    else if (JSON.stringify(existing) === JSON.stringify(item)) identical.push(item);
    else conflicts.push({ item, existing });
  });
  return {
    additions, identical, conflicts, invalid,
    currentCount: currentState.items.length,
    sourceCount: sourceItems.length
  };
}

export function buildResearchNotesImportItems(preview, conflictMode = "skip", createId = () =>
  "note-import-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 10)) {
  if (!preview || !Array.isArray(preview.additions) || !Array.isArray(preview.conflicts)) {
    throw new TypeError("导入预览状态无效。");
  }
  const items = [...preview.additions];
  if (conflictMode === "copy") {
    preview.conflicts.forEach(({ item }) => {
      let id = String(createId() || "").trim();
      if (!id || id.length > 160 || items.some(existing => existing.id === id) ||
          preview.additions.some(existing => existing.id === id) ||
          preview.conflicts.some(({ item: conflicting }) => conflicting.id === id)) {
        throw new TypeError("冲突记录的新 ID 无效或重复，请重新生成。");
      }
      items.push({ ...item, id });
    });
  } else if (conflictMode !== "skip") {
    throw new TypeError("未知的冲突处理方式。");
  }
  return items;
}

export function commitResearchNotesImport(preview, storage, expectedRaw, conflictMode = "skip", createId) {
  const target = safeStorage(storage);
  if (!target) return { ok: false, reason: "unavailable" };
  let before;
  try { before = target.getItem(RESEARCH_NOTES_KEY); }
  catch { return { ok: false, reason: "unavailable" }; }
  if (before !== expectedRaw) return { ok: false, reason: "stale" };
  const current = readResearchNotes(target);
  if (!current.readable || !current.valid) return { ok: false, reason: "invalid-storage" };
  let imported;
  try { imported = buildResearchNotesImportItems(preview, conflictMode, createId); }
  catch { return { ok: false, reason: "invalid" }; }
  const ids = new Set(current.items.map(item => item.id));
  if (imported.some(item => ids.has(item.id))) return { ok: false, reason: "conflict" };
  const merged = [...imported, ...current.items];
  if (merged.length > MAX_RESEARCH_NOTES) return { ok: false, reason: "limit" };
  const normalized = merged.map(normalizeNote);
  if (normalized.some(item => !item) || new Set(normalized.map(item => item.id)).size !== normalized.length) {
    return { ok: false, reason: "invalid" };
  }
  const serialized = JSON.stringify(normalized);
  try {
    if (target.getItem(RESEARCH_NOTES_KEY) !== expectedRaw) return { ok: false, reason: "stale" };
    target.setItem(RESEARCH_NOTES_KEY, serialized);
    const after = readResearchNotes(target);
    if (after.readable && after.valid && JSON.stringify(after.items) === serialized) {
      return { ok: true, importedCount: imported.length, totalCount: after.items.length };
    }
  } catch {}
  let rollback = false;
  try {
    const currentRaw = target.getItem(RESEARCH_NOTES_KEY);
    if (currentRaw === before) rollback = true;
    else if (currentRaw === serialized) {
      if (before === null) target.removeItem(RESEARCH_NOTES_KEY);
      else target.setItem(RESEARCH_NOTES_KEY, before);
      rollback = target.getItem(RESEARCH_NOTES_KEY) === before;
    }
  } catch {}
  return { ok: false, reason: "verify", rollback };
}

export function researchNoteEditState(items, editingId, originalSnapshot) {
  const current = (Array.isArray(items) ? items : []).find(item => item?.id === editingId);
  if (!current) return "missing";
  return JSON.stringify(current) === originalSnapshot ? "unchanged" : "changed";
}

export function bindResearchNotes({ root = document, getContext = () => ({}), storage } = {}) {
  const $ = selector => root.querySelector(selector.startsWith("#") ? selector : "#" + selector);
  const form = $("#research-note-form"), editor = $("#research-note-editor"), host = $("#research-note-list");
  const exportButton = $("#research-note-export");
  if (!form || !editor || !host) return;
  let editingId = "", editingSnapshot = "", editorInitialFormSnapshot = "";
  const state = () => readResearchNotes(storage);
  const feedback = (message, error = false) => {
    const node = $("#research-note-feedback");
    node.textContent = message;
    node.dataset.state = error ? "error" : "ok";
  };
  const readForm = () => ({
    title: $("#research-note-title").value,
    symbol: $("#research-note-symbol").value,
    tags: $("#research-note-tags").value,
    task: $("#research-note-task").value,
    run: $("#research-note-run").value,
    content: $("#research-note-content").value
  });
  const snapshotForm = () => { editorInitialFormSnapshot = JSON.stringify(readForm()); };
  const hasUnsavedChanges = () => JSON.stringify(readForm()) !== editorInitialFormSnapshot;
  const hideConflictActions = () => { conflictActions.hidden = true; };
  const showConflictActions = () => { conflictActions.hidden = false; };
  const conflictActions = document.createElement("div");
  conflictActions.className = "research-note-conflict-actions";
  conflictActions.hidden = true;
  const reloadButton = document.createElement("button");
  reloadButton.type = "button";
  reloadButton.className = "button button-secondary";
  reloadButton.textContent = "重新载入最新版本";
  const saveAsNewButton = document.createElement("button");
  saveAsNewButton.type = "button";
  saveAsNewButton.className = "button button-secondary";
  saveAsNewButton.textContent = "另存为新笔记";
  conflictActions.append(reloadButton, saveAsNewButton);
  $("#research-note-cancel").insertAdjacentElement("afterend", conflictActions);

  const fillCurrentContext = () => {
    const context = getContext() || {};
    $("#research-note-symbol").value = String(context.symbol || "");
    $("#research-note-task").value = String(context.taskId || "");
    $("#research-note-run").value = String(context.runId || "");
  };
  const loadNoteIntoForm = note => {
    $("#research-note-title").value = note.title;
    $("#research-note-content").value = note.content;
    $("#research-note-symbol").value = (note.symbols?.length ? note.symbols : [note.symbol].filter(Boolean)).join(", ");
    $("#research-note-tags").value = note.tags.join(", ");
    $("#research-note-task").value = note.taskId;
    $("#research-note-run").value = note.runId;
  };
  const canLeaveEditor = () => editor.hidden || !hasUnsavedChanges() ||
    globalThis.confirm("当前研究记录有未保存修改，确定离开并丢弃草稿？");
  const showEditor = title => {
    $("#research-note-editor-title").textContent = title;
    editor.hidden = false;
    editor.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  const closeEditor = () => {
    if (!canLeaveEditor()) return false;
    editingId = "";
    editingSnapshot = "";
    form.reset();
    fillCurrentContext();
    $("#research-note-cancel").hidden = true;
    hideConflictActions();
    editor.hidden = true;
    snapshotForm();
    feedback("已返回研究记录列表。");
    return true;
  };
  const openNewNote = () => {
    if (!canLeaveEditor()) return;
    editingId = "";
    editingSnapshot = "";
    form.reset();
    fillCurrentContext();
    $("#research-note-cancel").hidden = true;
    hideConflictActions();
    showEditor("新建研究记录");
    snapshotForm();
    $("#research-note-title").focus();
  };
  const openExistingNote = note => {
    if (!canLeaveEditor()) return;
    editingId = note.id;
    editingSnapshot = JSON.stringify(note);
    loadNoteIntoForm(note);
    $("#research-note-cancel").hidden = false;
    hideConflictActions();
    showEditor("编辑研究记录");
    snapshotForm();
    feedback("正在编辑记录。");
    $("#research-note-title").focus();
  };

  reloadButton.addEventListener("click", () => {
    const current = state();
    if (!current.readable || !current.valid) { feedback("当前存储不可用，无法重新载入。", true); return; }
    const latest = current.items.find(item => item.id === editingId);
    if (!latest) {
      editingId = "";
      editingSnapshot = "";
      $("#research-note-cancel").hidden = true;
      $("#research-note-editor-title").textContent = "新建研究记录";
      editorInitialFormSnapshot = "";
      hideConflictActions();
      feedback("该笔记已被其他标签页删除。当前草稿已保留为新笔记草稿，保存后会创建独立记录。", true);
      return;
    }
    loadNoteIntoForm(latest);
    editingSnapshot = JSON.stringify(latest);
    snapshotForm();
    hideConflictActions();
    feedback("已载入其他标签页保存的最新版本。");
  });
  saveAsNewButton.addEventListener("click", () => {
    editingId = "";
    editingSnapshot = "";
    $("#research-note-cancel").hidden = true;
    $("#research-note-editor-title").textContent = "新建研究记录";
    editorInitialFormSnapshot = "";
    hideConflictActions();
    feedback("已切换为新笔记；当前草稿内容保留，提交后将创建独立记录。");
  });

  const render = () => {
    const result = state();
    host.replaceChildren();
    const search = $("#research-note-search"), symbolFilter = $("#research-note-symbol-filter");
    const tagFilter = $("#research-note-tag-filter"), clear = $("#research-note-clear");
    const create = $("#research-note-create"), count = $("#research-note-count");
    if (!result.readable || !result.valid) {
      count.textContent = "研究记录存储不可用";
      create.disabled = true;
      clear.disabled = true;
      if (exportButton) exportButton.disabled = true;
      const p = document.createElement("p");
      p.className = "table-empty";
      p.textContent = "浏览器存储不可读取或数据格式异常；为保护原始记录，当前未修改数据。";
      host.append(p);
      return;
    }
    create.disabled = false;
    if (exportButton) exportButton.disabled = result.items.length === 0;
    const selectedTag = tagFilter.value;
    const tags = [...new Set(result.items.flatMap(note => note.tags))].sort((a, b) => a.localeCompare(b, "zh-CN"));
    tagFilter.replaceChildren();
    const allTags = document.createElement("option");
    allTags.value = "";
    allTags.textContent = "全部标签";
    tagFilter.append(allTags);
    tags.forEach(tag => {
      const option = document.createElement("option");
      option.value = tag;
      option.textContent = tag;
      tagFilter.append(option);
    });
    if (selectedTag && !tags.includes(selectedTag)) {
      const missing = document.createElement("option");
      missing.value = selectedTag;
      missing.textContent = selectedTag + "（当前无记录）";
      tagFilter.append(missing);
    }
    tagFilter.value = selectedTag;
    const items = filterResearchNotes(result.items, search.value, symbolFilter.value, tagFilter.value);
    count.textContent = "显示 " + items.length + " / " + result.items.length + " 条记录";
    clear.disabled = !search.value.trim() && !symbolFilter.value.trim() && !tagFilter.value;
    if (!items.length) {
      const p = document.createElement("p");
      p.className = "table-empty";
      p.textContent = result.items.length ? "没有匹配的研究记录。" : "尚无研究记录。";
      host.append(p);
      return;
    }
    items.forEach(note => {
      const card = document.createElement("article");
      card.className = "research-note-card";
      const header = document.createElement("header");
      const heading = document.createElement("div");
      const title = document.createElement("h3");
      title.textContent = note.title;
      const meta = document.createElement("p");
      meta.className = "research-note-meta";
      meta.textContent = [
        note.taskId ? "Task " + note.taskId : "未关联任务",
        note.runId ? "Run " + note.runId : "",
        note.updatedAt ? "更新于 " + note.updatedAt : ""
      ].filter(Boolean).join(" · ");
      heading.append(title, meta);
      header.append(heading);
      const symbols = note.symbols?.length ? note.symbols : [note.symbol].filter(Boolean);
      if (symbols.length) {
        const symbolHost = document.createElement("div");
        symbolHost.className = "research-note-symbols";
        symbols.slice(0, 4).forEach(symbol => {
          const chip = document.createElement("span");
          chip.className = "research-note-chip";
          chip.textContent = symbol;
          symbolHost.append(chip);
        });
        if (symbols.length > 4) {
          const more = document.createElement("span");
          more.className = "research-note-chip is-muted";
          more.textContent = "+" + (symbols.length - 4);
          symbolHost.append(more);
        }
        card.append(symbolHost);
      }
      const body = document.createElement("p");
      body.className = "research-note-preview";
      body.textContent = note.content;
      card.append(header, body);
      if (note.tags.length) {
        const tagHost = document.createElement("div");
        tagHost.className = "research-note-tags";
        note.tags.forEach(tag => {
          const chip = document.createElement("span");
          chip.className = "research-note-chip is-tag";
          chip.textContent = tag;
          tagHost.append(chip);
        });
        card.append(tagHost);
      }
      const actions = document.createElement("div");
      actions.className = "research-note-card-actions";
      [["编辑", "edit"], ["删除", "delete"]].forEach(([label, action]) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "button button-secondary";
        button.textContent = label;
        button.dataset.noteAction = action;
        button.dataset.noteId = note.id;
        actions.append(button);
      });
      card.append(actions);
      host.append(card);
    });
  };

  form.addEventListener("submit", event => {
    event.preventDefault();
    const values = readForm();
    const title = values.title.trim(), content = values.content.trim();
    if (!title || !content) { feedback("标题和研究内容不能为空。", true); return; }
    const current = state();
    if (!current.readable || !current.valid) { feedback("存储不可用或数据异常，未写入。", true); return; }
    const now = new Date().toISOString();
    const existing = editingId ? current.items.find(item => item.id === editingId) : null;
    if (editingId) {
      const editState = researchNoteEditState(current.items, editingId, editingSnapshot);
      if (editState === "missing") {
        feedback("该笔记已被其他标签页删除，拒绝使用旧草稿重新创建。可重新载入状态或另存为新笔记。", true);
        showConflictActions();
        return;
      }
      if (editState === "changed") {
        feedback("该笔记已被其他标签页修改，拒绝用旧草稿覆盖。请重新载入最新版本或另存为新笔记。", true);
        showConflictActions();
        return;
      }
    }
    const context = getContext() || {};
    const tags = values.tags.split(/[,，、]/).map(value => value.trim()).filter(Boolean);
    if (tags.length > MAX_TAGS || tags.some(tag => tag.length > 40)) {
      feedback("标签最多 12 个，每个标签不超过 40 个字符。", true);
      return;
    }
    const rawSymbols = values.symbol.split(/[,，、\s]+/).map(value => value.trim()).filter(Boolean);
    const fallbackSymbols = Array.isArray(context.symbols) ? context.symbols : [context.symbol];
    const symbols = [...new Set((rawSymbols.length ? rawSymbols : fallbackSymbols).filter(Boolean).map(value => String(value).trim()))];
    if (symbols.length > MAX_SYMBOLS || symbols.some(value => value.length > 32)) {
      feedback("关联股票最多 100 只，每个证券代码不超过 32 个字符。", true);
      return;
    }
    const note = {
      id: editingId || ("note-" + now + "-" + Math.random().toString(36).slice(2, 8)),
      title, content, symbol: symbols[0] || "", symbols,
      taskId: values.task.trim() || String(context.taskId || ""),
      runId: values.run.trim() || String(context.runId || ""),
      tags, createdAt: existing?.createdAt || now, updatedAt: now
    };
    const next = existing ? current.items.map(item => item.id === editingId ? note : item) : [note, ...current.items];
    if (!existing && next.length > MAX_RESEARCH_NOTES) {
      feedback("最多保存 200 条，请先整理或删除旧记录。", true);
      return;
    }
    const saved = writeResearchNotes(next, storage);
    if (!saved.ok) { feedback("保存失败，原有记录未确认写入：" + saved.reason, true); return; }
    editingId = "";
    editingSnapshot = "";
    form.reset();
    fillCurrentContext();
    $("#research-note-cancel").hidden = true;
    hideConflictActions();
    editor.hidden = true;
    snapshotForm();
    feedback("研究记录已保存在当前浏览器，不会同步到服务端。");
    render();
  });

  exportButton?.addEventListener("click", () => {
    const current = state();
    if (!current.readable || !current.valid) {
      feedback("研究记录存储不可读或数据异常，已取消导出。", true);
      render();
      return;
    }
    if (!current.items.length) {
      feedback("当前没有可导出的研究记录。", true);
      render();
      return;
    }
    let content;
    try {
      content = serializeResearchNotesExport(current.items);
    } catch (error) {
      feedback(error instanceof Error ? error.message : "研究记录导出失败。", true);
      return;
    }
    const blob = new Blob([content], { type: "application/json;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "research-notes-" + new Date().toISOString().slice(0, 10) + ".json";
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
    feedback("已导出全部 " + current.items.length + " 条研究记录 JSON；导出范围不受当前筛选条件影响。");
  });


  const importFile = $("#research-note-import-file");
  const importPanel = $("#research-note-import-preview");
  const importSummary = $("#research-note-import-summary");
  const importFeedback = $("#research-note-import-feedback");
  const importCommit = $("#research-note-import-commit");
  let pendingImport = null, pendingImportRaw = null;

  const clearImportPreview = () => {
    pendingImport = null;
    pendingImportRaw = null;
    if (importPanel) importPanel.hidden = true;
    if (importCommit) importCommit.disabled = false;
    if (importFile) importFile.value = "";
  };
  $("#research-note-import-cancel")?.addEventListener("click", clearImportPreview);
  importFile?.addEventListener("change", async () => {
    const file = importFile.files?.[0];
    if (!file) return;
    try {
      if (file.size > MAX_RESEARCH_NOTES_IMPORT_BYTES) throw new RangeError("导入文件超过 2 MB，已拒绝读取。");
      const current = state();
      if (!current.readable || !current.valid) throw new TypeError("当前研究记录存储不可用或数据异常，不能导入。");
      const target = safeStorage(storage);
      if (!target) throw new TypeError("浏览器本地存储不可用。");
      const raw = target.getItem(RESEARCH_NOTES_KEY);
      const preview = previewResearchNotesImport(await file.text(), current);
      pendingImport = preview;
      pendingImportRaw = raw;
      const copy = preview.conflicts.length ? "；冲突另存为新记录后预计新增 " + preview.conflicts.length + " 条" : "";
      importSummary.textContent = "文件记录 " + preview.sourceCount + " 条：新增 " + preview.additions.length +
        " 条、完全相同 " + preview.identical.length + " 条、冲突 " + preview.conflicts.length +
        " 条、无效 " + preview.invalid.length + " 条" + copy +
        "。本地当前 " + preview.currentCount + " 条；预览不会修改本地数据。";
      importFeedback.textContent = preview.invalid.length ? "无效记录将跳过，不会写入。" : "";
      importPanel.hidden = false;
      importCommit.disabled = preview.additions.length === 0 && preview.conflicts.length === 0;
    } catch (error) {
      clearImportPreview();
      feedback(error instanceof Error ? error.message : "研究记录导入预览失败。", true);
    }
  });
  importCommit?.addEventListener("click", () => {
    if (!pendingImport) return;
    const result = commitResearchNotesImport(
      pendingImport, storage, pendingImportRaw, $("#research-note-import-conflict").value
    );
    if (!result.ok) {
      const messages = {
        stale: "预览后本地研究记录已发生变化，未执行导入。请重新选择文件并预览。",
        limit: "导入后将超过 200 条记录上限，未写入。",
        verify: result.rollback ? "写入校验失败，已恢复导入前数据。" : "写入校验失败，且自动恢复未能确认；请立即导出/检查当前记录。",
        invalid: "导入数据无效，未写入。",
        conflict: "导入 ID 与当前记录冲突，请重新预览。",
        unavailable: "浏览器存储不可用，未写入。",
        "invalid-storage": "当前本地数据异常，未写入。"
      };
      importFeedback.textContent = messages[result.reason] || "导入失败，未确认写入。";
      importFeedback.dataset.state = "error";
      if (result.reason === "stale") importCommit.disabled = true;
      return;
    }
    const count = result.importedCount;
    pendingImport = null;
    pendingImportRaw = null;
    importFile.value = "";
    importCommit.disabled = true;
    importSummary.textContent = "导入完成：新增 " + count + " 条；当前共 " + result.totalCount + " 条。";
    importFeedback.textContent = "记录仅保存在当前浏览器；可关闭此预览继续使用。";
    importFeedback.dataset.state = "ok";
    render();
  });

  $("#research-note-create").addEventListener("click", openNewNote);
  $("#research-note-editor-back").addEventListener("click", closeEditor);
  $("#research-note-cancel").addEventListener("click", closeEditor);
  $("#research-note-search").addEventListener("input", render);
  $("#research-note-symbol-filter").addEventListener("input", render);
  $("#research-note-tag-filter").addEventListener("change", render);
  $("#research-note-clear").addEventListener("click", () => {
    $("#research-note-search").value = "";
    $("#research-note-symbol-filter").value = "";
    $("#research-note-tag-filter").value = "";
    render();
  });
  host.addEventListener("click", event => {
    const button = event.target.closest("[data-note-action]");
    if (!button) return;
    const current = state();
    if (!current.readable || !current.valid) return;
    const note = current.items.find(item => item.id === button.dataset.noteId);
    if (!note) return;
    if (button.dataset.noteAction === "edit") { openExistingNote(note); return; }
    if (!globalThis.confirm("确定删除这条研究记录？")) return;
    const saved = writeResearchNotes(current.items.filter(item => item.id !== note.id), storage);
    feedback(saved.ok ? "记录已删除。" : "删除失败，未能确认存储结果。", !saved.ok);
    render();
  });

  fillCurrentContext();
  snapshotForm();
  render();
  return {
    render,
    createFromReport(report) {
      if (!canLeaveEditor()) return false;
      const linked = buildResearchNoteContextFromReport(report);
      if (!linked) return false;
      editingId = "";
      editingSnapshot = "";
      form.reset();
      $("#research-note-title").value = "Run " + linked.runId + " 研究记录";
      $("#research-note-symbol").value = linked.symbols.join(", ");
      $("#research-note-task").value = linked.taskId;
      $("#research-note-run").value = linked.runId;
      $("#research-note-cancel").hidden = true;
      hideConflictActions();
      showEditor("基于报告新建研究记录");
      editorInitialFormSnapshot = "";
      feedback("已填入报告关联信息，请补充研究内容后保存。");
      $("#research-note-content").focus();
      return true;
    }
  };
}

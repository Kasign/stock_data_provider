import test from "node:test";
import assert from "node:assert/strict";
import {buildResearchNoteContextFromReport,filterResearchNotes,readResearchNotes,writeResearchNotes,researchNoteEditState,serializeResearchNotesExport,RESEARCH_NOTES_KEY} from "../src/research-notes.js";
function memory(){const data=new Map();return {getItem:k=>data.has(k)?data.get(k):null,setItem:(k,v)=>data.set(k,v),removeItem:k=>data.delete(k)};}
const note=(id,title,content,symbol="600519.SH",tags=["基本面"])=>({id,title,content,symbol,tags,createdAt:"2026-01-01",updatedAt:"2026-01-02"});
test("研究记录可写入并回读校验",()=>{const s=memory();assert.equal(writeResearchNotes([note("1","标题","内容")],s).ok,true);assert.equal(readResearchNotes(s).items.length,1);});
test("笔记保留任务和 Run 关联并支持关联 ID 检索",()=>{const s=memory();assert.equal(writeResearchNotes([{...note("1","研究判断","观察内容"),taskId:"task-42",runId:"run-42"}],s).ok,true);const item=readResearchNotes(s).items[0];assert.equal(item.taskId,"task-42");assert.equal(item.runId,"run-42");assert.equal(filterResearchNotes([item],"task-42").length,1);assert.equal(filterResearchNotes([item],"run-42").length,1);});
test("检索覆盖标题正文代码与标签并支持代码筛选",()=>{const items=[note("1","贵州茅台","现金流良好"),note("2","观察记录","等待验证","000858.SZ",["技术面"])];assert.equal(filterResearchNotes(items,"现金流").length,1);assert.equal(filterResearchNotes(items,"技术面").length,1);assert.equal(filterResearchNotes(items,"","000858").length,1);});

test("研究记录筛选支持多标的与标签精确匹配", () => {
  const items = [
    { ...note("multi", "组合研究", "观察组合", "600519.SH", ["基本面", "观察中"]), symbols: ["600519.SH", "000858.SZ"] },
    note("single", "技术观察", "等待确认", "000001.SZ", ["技术面"])
  ];
  assert.equal(filterResearchNotes(items, "", "", "基本面").length, 1);
  assert.equal(filterResearchNotes(items, "", "000858", "基本面").length, 1);
  assert.equal(filterResearchNotes(items, "", "000001", "基本面").length, 0);
  assert.equal(filterResearchNotes(items, "", "", "基本").length, 0, "tag filter is exact rather than substring matching");
});

test("损坏数据不会被误判为空且不覆盖原值",()=>{const s=memory();s.setItem(RESEARCH_NOTES_KEY,"{");const before=s.getItem(RESEARCH_NOTES_KEY);assert.equal(readResearchNotes(s).valid,false);assert.equal(writeResearchNotes([],s).ok,false);assert.equal(s.getItem(RESEARCH_NOTES_KEY),before);});
test("超过条数限制拒绝写入",()=>{assert.equal(writeResearchNotes(Array.from({length:201},(_,i)=>note(String(i),"标题","内容")),memory()).ok,false);});
test("关键词检索不执行 HTML",()=>{const x=note("1","<img src=x>","<script>alert(1)</script>");assert.equal(filterResearchNotes([x],"<img").length,1);});

test("从归档报告创建笔记时准确继承股票、Task 和 Run 关联",()=>{const context=buildResearchNoteContextFromReport({runId:"run-9",taskId:"task-9",symbols:["600519.SH","000858.SZ"]});assert.deepEqual(context,{symbol:"600519.SH",symbols:["600519.SH","000858.SZ"],taskId:"task-9",runId:"run-9"});});
test("报告缺少有效 Run ID 时不创建关联上下文",()=>{assert.equal(buildResearchNoteContextFromReport({taskId:"task-1"}),null);});
test("多标的报告创建笔记时保留全部标的并支持任意标的检索",()=>{const context=buildResearchNoteContextFromReport({runId:"run-many",taskId:"task-many",symbols:["600519.SH","000858.SZ","300750.SZ"]});assert.deepEqual(context.symbols,["600519.SH","000858.SZ","300750.SZ"]);const s=memory();assert.equal(writeResearchNotes([{id:"many",title:"组合研究",content:"观察组合",...context,tags:[],createdAt:"2026-01-01",updatedAt:"2026-01-02"}],s).ok,true);const item=readResearchNotes(s).items[0];assert.deepEqual(item.symbols,["600519.SH","000858.SZ","300750.SZ"]);assert.equal(filterResearchNotes([item],"300750").length,1);assert.equal(filterResearchNotes([item],"","000858").length,1);});
test("旧版单标的笔记读取后兼容为单元素 symbols，编辑模型仍保留主标的",()=>{const s=memory();s.setItem(RESEARCH_NOTES_KEY,JSON.stringify([note("legacy","旧笔记","内容")]));const item=readResearchNotes(s).items[0];assert.equal(item.symbol,"600519.SH");assert.deepEqual(item.symbols,["600519.SH"]);assert.equal(writeResearchNotes([item],s).ok,true);assert.deepEqual(readResearchNotes(s).items[0].symbols,["600519.SH"]);});
test("笔记支持 51 和 100 只标的，101 只及非法代码拒绝写入",()=>{for(const count of [51,100]){const symbols=Array.from({length:count},(_,i)=>"SH"+String(i).padStart(4,"0"));assert.equal(writeResearchNotes([{...note("max-"+count,"多标的研究","内容",symbols[0]),symbols}],memory()).ok,true);}const symbols=Array.from({length:100},(_,i)=>"SH"+String(i).padStart(4,"0"));assert.equal(writeResearchNotes([{...note("over","超限标的","内容",symbols[0]),symbols:[...symbols,"SZ9999"]}],memory()).reason,"invalid");assert.equal(writeResearchNotes([{...note("bad","非法标的","内容"),symbols:["600519.SH",""]}],memory()).reason,"invalid");});

test("拒绝异常标签和超长关联字段，不静默截断研究记录", () => {
  const s = memory();
  for (const tags of ["基本面", ["", "技术面"], Array.from({ length: 13 }, () => "标签"), ["x".repeat(41)]]) {
    assert.equal(writeResearchNotes([{ ...note("bad", "标题", "内容"), tags }], s).reason, "invalid");
    assert.equal(s.getItem(RESEARCH_NOTES_KEY), null);
  }
  assert.equal(writeResearchNotes([{ ...note("bad", "标题", "内容"), taskId: "x".repeat(161) }], s).reason, "invalid");
  assert.equal(writeResearchNotes([{ ...note("bad", "标题", "内容"), symbol: "x".repeat(33) }], s).reason, "invalid");
});

test("拒绝重复笔记 ID，且不会覆盖重复或损坏的旧数据",()=>{const s=memory();const duplicate=[note("same","第一条","内容一"),note("same","第二条","内容二")];assert.equal(writeResearchNotes(duplicate,s).reason,"invalid");s.setItem(RESEARCH_NOTES_KEY,JSON.stringify(duplicate));const before=s.getItem(RESEARCH_NOTES_KEY);assert.equal(readResearchNotes(s).valid,false);assert.equal(writeResearchNotes([note("new","新记录","内容")],s).reason,"invalid-storage");assert.equal(s.getItem(RESEARCH_NOTES_KEY),before);});

test("编辑快照可区分未变化、远端修改和远端删除", () => {
  const original = note("edit-1", "原始标题", "原始内容");
  const snapshot = JSON.stringify(original);
  assert.equal(researchNoteEditState([original], "edit-1", snapshot), "unchanged");
  assert.equal(researchNoteEditState([{ ...original, title: "远端新标题", updatedAt: "2026-02-01" }], "edit-1", snapshot), "changed");
  assert.equal(researchNoteEditState([], "edit-1", snapshot), "missing");
});

test("浏览器存储 API 异常时拒绝读取与写入，不报告成功", () => {
  const denied = {
    getItem() { throw new Error("SecurityError"); },
    setItem() {}
  };
  const read = readResearchNotes(denied);
  assert.equal(read.readable, false);
  assert.equal(read.valid, false);

  const quota = {
    getItem() { return null; },
    setItem() { throw new Error("QuotaExceededError"); }
  };
  assert.deepEqual(writeResearchNotes([note("quota", "标题", "内容")], quota), {
    ok: false,
    reason: "unavailable"
  });
});


test("笔记 ID 与时间字段有长度上限，但不强制单一日期格式", () => {
  const s = memory();
  const base = note("note-a", "标题", "内容");
  for (const item of [
    { ...base, id: "i".repeat(161) },
    { ...base, createdAt: "t".repeat(65) },
    { ...base, updatedAt: "t".repeat(65) }
  ]) {
    assert.equal(writeResearchNotes([item], s).reason, "invalid");
    assert.equal(s.getItem(RESEARCH_NOTES_KEY), null);
  }
  assert.equal(writeResearchNotes([{ ...base, createdAt: "legacy-date", updatedAt: "" }], s).ok, true);
});

test("从报告创建笔记的关联字段不再静默截断", () => {
  const context = buildResearchNoteContextFromReport({
    runId: "run-1",
    taskId: "t".repeat(161),
    symbols: ["s".repeat(33)]
  });
  assert.equal(context.symbol.length, 33);
  assert.equal(context.taskId.length, 161);
  assert.equal(writeResearchNotes([{
    ...note("note-long-context", "标题", "内容"),
    ...context
  }], memory()).reason, "invalid");
});

test("研究记录 JSON 导出保留完整字段并兼容旧版单标的", () => {
  const legacy = note("legacy-export", "旧版记录", "正文");
  const content = serializeResearchNotesExport([legacy], "2026-11-06T00:00:00.000Z");
  const payload = JSON.parse(content);
  assert.equal(payload.format, "stock-provider-webui.research-notes");
  assert.equal(payload.version, 1);
  assert.equal(payload.exportedAt, "2026-11-06T00:00:00.000Z");
  assert.deepEqual(payload.items[0].symbols, ["600519.SH"]);
  assert.equal(payload.items[0].symbol, "600519.SH");
  assert.equal(payload.items[0].taskId, "");
  assert.equal(payload.items[0].runId, "");
});

test("研究记录 JSON 导出拒绝无效记录和重复 ID", () => {
  assert.throws(() => serializeResearchNotesExport([{ id: "bad" }]), /无效或重复数据/);
  assert.throws(() => serializeResearchNotesExport([note("same", "一", "正文"), note("same", "二", "正文")]), /无效或重复数据/);
  assert.throws(() => serializeResearchNotesExport(Array.from({ length: 201 }, (_, i) => note(String(i), "标题", "正文"))), /超过数量上限/);
});


test("研究记录导入预览区分新增、相同、冲突和无效记录，且不修改存储", async () => {
  const { previewResearchNotesImport } = await import("../src/research-notes.js");
  const currentStorage = memory();
  writeResearchNotes([note("same", "本地", "正文"), note("conflict", "本地版本", "旧正文")], currentStorage);
  const current = readResearchNotes(currentStorage);
  const payload = { format: "stock-provider-webui.research-notes", version: 1, items: [
    note("new", "新增", "内容"), note("same", "本地", "正文"),
    note("conflict", "导入版本", "新正文"), { id: "bad" }
  ] };
  const storage = memory();
  const before = storage.getItem(RESEARCH_NOTES_KEY);
  const preview = previewResearchNotesImport(JSON.stringify(payload), current);
  assert.equal(preview.additions.length, 1);
  assert.equal(preview.identical.length, 1);
  assert.equal(preview.conflicts.length, 1);
  assert.equal(preview.invalid.length, 1);
  assert.equal(storage.getItem(RESEARCH_NOTES_KEY), before);
});

test("研究记录导入拒绝错误格式、版本、重复 ID、超量和超大文件", async () => {
  const { previewResearchNotesImport, MAX_RESEARCH_NOTES_IMPORT_BYTES } = await import("../src/research-notes.js");
  const current = { readable: true, valid: true, items: [] };
  assert.throws(() => previewResearchNotesImport("{", current), /JSON/);
  assert.throws(() => previewResearchNotesImport(JSON.stringify({ format: "other", version: 1, items: [] }), current), /格式/);
  assert.throws(() => previewResearchNotesImport(JSON.stringify({ format: "stock-provider-webui.research-notes", version: 2, items: [] }), current), /版本/);
  assert.throws(() => previewResearchNotesImport(JSON.stringify({ format: "stock-provider-webui.research-notes", version: 1, items: Array(201).fill(note("x", "标题", "内容")) }), current), /200/);
  assert.throws(() => previewResearchNotesImport(" ".repeat(MAX_RESEARCH_NOTES_IMPORT_BYTES + 1), current), /2 MB/);
  const duplicate = previewResearchNotesImport(JSON.stringify({ format: "stock-provider-webui.research-notes", version: 1, items: [note("dup", "一", "正文"), note("dup", "二", "正文")] }), current);
  assert.equal(duplicate.invalid.length, 2);
});

test("研究记录导入冲突默认跳过，也可生成新 ID；禁止覆盖和重复 ID", async () => {
  const { previewResearchNotesImport, buildResearchNotesImportItems } = await import("../src/research-notes.js");
  const currentStorage = memory();
  writeResearchNotes([note("conflict", "本地版本", "旧正文")], currentStorage);
  const preview = previewResearchNotesImport(JSON.stringify({ format: "stock-provider-webui.research-notes", version: 1, items: [note("conflict", "导入版本", "新正文")] }), readResearchNotes(currentStorage));
  assert.equal(buildResearchNotesImportItems(preview, "skip").length, 0);
  const copied = buildResearchNotesImportItems(preview, "copy", () => "new-copy");
  assert.equal(copied.length, 1);
  assert.equal(copied[0].id, "new-copy");
  assert.equal(copied[0].title, "导入版本");
  assert.throws(() => buildResearchNotesImportItems(preview, "copy", () => "conflict"), /无效或重复/);
});

test("研究记录导入提交检查快照、容量并在回读失败时恢复原始数据", async () => {
  const { previewResearchNotesImport, commitResearchNotesImport } = await import("../src/research-notes.js");
  const s = memory();
  const original = [note("local", "本地", "内容")];
  writeResearchNotes(original, s);
  const raw = s.getItem(RESEARCH_NOTES_KEY);
  const preview = previewResearchNotesImport(JSON.stringify({ format: "stock-provider-webui.research-notes", version: 1, items: [note("remote", "远端", "内容")] }), readResearchNotes(s));
  s.setItem(RESEARCH_NOTES_KEY, JSON.stringify([note("changed", "跨标签页", "修改")]));
  assert.equal(commitResearchNotesImport(preview, s, raw).reason, "stale");
  s.setItem(RESEARCH_NOTES_KEY, raw);
  const result = commitResearchNotesImport(preview, s, raw);
  assert.equal(result.ok, true);
  assert.equal(result.importedCount, 1);
  assert.deepEqual(readResearchNotes(s).items.map(item => item.id).sort(), ["local", "remote"]);
  const failed = {
    raw,
    writes: 0,
    getItem() { return this.writes === 1 ? "corrupted-after-write" : this.raw; },
    setItem(key, value) { this.writes++; this.raw = value; },
    removeItem() { this.raw = null; }
  };
  const brokenPreview = previewResearchNotesImport(JSON.stringify({ format: "stock-provider-webui.research-notes", version: 1, items: [note("remote-2", "远端", "内容")] }), readResearchNotes({ getItem: () => raw }));
  const failedResult = commitResearchNotesImport(brokenPreview, failed, raw);
  assert.equal(failedResult.ok, false);
  assert.equal(failedResult.rollback, true);
  assert.equal(failed.raw, raw, "failed import must restore the exact pre-import snapshot");
});

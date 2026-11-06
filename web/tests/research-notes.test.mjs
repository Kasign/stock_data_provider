import test from "node:test";
import assert from "node:assert/strict";
import {buildResearchNoteContextFromReport,filterResearchNotes,readResearchNotes,writeResearchNotes,researchNoteEditState,RESEARCH_NOTES_KEY} from "../src/research-notes.js";
function memory(){const data=new Map();return {getItem:k=>data.has(k)?data.get(k):null,setItem:(k,v)=>data.set(k,v),removeItem:k=>data.delete(k)};}
const note=(id,title,content,symbol="600519.SH",tags=["基本面"])=>({id,title,content,symbol,tags,createdAt:"2026-01-01",updatedAt:"2026-01-02"});
test("研究记录可写入并回读校验",()=>{const s=memory();assert.equal(writeResearchNotes([note("1","标题","内容")],s).ok,true);assert.equal(readResearchNotes(s).items.length,1);});
test("笔记保留任务和 Run 关联并支持关联 ID 检索",()=>{const s=memory();assert.equal(writeResearchNotes([{...note("1","研究判断","观察内容"),taskId:"task-42",runId:"run-42"}],s).ok,true);const item=readResearchNotes(s).items[0];assert.equal(item.taskId,"task-42");assert.equal(item.runId,"run-42");assert.equal(filterResearchNotes([item],"task-42").length,1);assert.equal(filterResearchNotes([item],"run-42").length,1);});
test("检索覆盖标题正文代码与标签并支持代码筛选",()=>{const items=[note("1","贵州茅台","现金流良好"),note("2","观察记录","等待验证","000858.SZ",["技术面"])];assert.equal(filterResearchNotes(items,"现金流").length,1);assert.equal(filterResearchNotes(items,"技术面").length,1);assert.equal(filterResearchNotes(items,"","000858").length,1);});
test("损坏数据不会被误判为空且不覆盖原值",()=>{const s=memory();s.setItem(RESEARCH_NOTES_KEY,"{");const before=s.getItem(RESEARCH_NOTES_KEY);assert.equal(readResearchNotes(s).valid,false);assert.equal(writeResearchNotes([],s).ok,false);assert.equal(s.getItem(RESEARCH_NOTES_KEY),before);});
test("超过条数限制拒绝写入",()=>{assert.equal(writeResearchNotes(Array.from({length:201},(_,i)=>note(String(i),"标题","内容")),memory()).ok,false);});
test("关键词检索不执行 HTML",()=>{const x=note("1","<img src=x>","<script>alert(1)</script>");assert.equal(filterResearchNotes([x],"<img").length,1);});

test("从归档报告创建笔记时准确继承股票、Task 和 Run 关联",()=>{const context=buildResearchNoteContextFromReport({runId:"run-9",taskId:"task-9",symbols:["600519.SH","000858.SZ"]});assert.deepEqual(context,{symbol:"600519.SH",symbols:["600519.SH","000858.SZ"],taskId:"task-9",runId:"run-9"});});
test("报告缺少有效 Run ID 时不创建关联上下文",()=>{assert.equal(buildResearchNoteContextFromReport({taskId:"task-1"}),null);});
test("多标的报告创建笔记时保留全部标的并支持任意标的检索",()=>{const context=buildResearchNoteContextFromReport({runId:"run-many",taskId:"task-many",symbols:["600519.SH","000858.SZ","300750.SZ"]});assert.deepEqual(context.symbols,["600519.SH","000858.SZ","300750.SZ"]);const s=memory();assert.equal(writeResearchNotes([{id:"many",title:"组合研究",content:"观察组合",...context,tags:[],createdAt:"2026-01-01",updatedAt:"2026-01-02"}],s).ok,true);const item=readResearchNotes(s).items[0];assert.deepEqual(item.symbols,["600519.SH","000858.SZ","300750.SZ"]);assert.equal(filterResearchNotes([item],"300750").length,1);assert.equal(filterResearchNotes([item],"","000858").length,1);});
test("旧版单标的笔记读取后兼容为单元素 symbols，编辑模型仍保留主标的",()=>{const s=memory();s.setItem(RESEARCH_NOTES_KEY,JSON.stringify([note("legacy","旧笔记","内容")]));const item=readResearchNotes(s).items[0];assert.equal(item.symbol,"600519.SH");assert.deepEqual(item.symbols,["600519.SH"]);assert.equal(writeResearchNotes([item],s).ok,true);assert.deepEqual(readResearchNotes(s).items[0].symbols,["600519.SH"]);});
test("笔记最多关联 100 只股票，101 只及非法代码拒绝写入",()=>{const symbols=Array.from({length:100},(_,i)=>"SH"+String(i).padStart(4,"0"));assert.equal(writeResearchNotes([{...note("max","百只标的","内容"),symbols}],memory()).ok,true);assert.equal(writeResearchNotes([{...note("over","超限标的","内容"),symbols:[...symbols,"SZ9999"]}],memory()).reason,"invalid");assert.equal(writeResearchNotes([{...note("bad","非法标的","内容"),symbols:["600519.SH",""]}],memory()).reason,"invalid");});

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

import test from "node:test";
import assert from "node:assert/strict";
import {buildResearchNoteContextFromReport,filterResearchNotes,readResearchNotes,writeResearchNotes,RESEARCH_NOTES_KEY} from "../src/research-notes.js";
function memory(){const data=new Map();return {getItem:k=>data.has(k)?data.get(k):null,setItem:(k,v)=>data.set(k,v),removeItem:k=>data.delete(k)};}
const note=(id,title,content,symbol="600519.SH",tags=["基本面"])=>({id,title,content,symbol,tags,createdAt:"2026-01-01",updatedAt:"2026-01-02"});
test("研究记录可写入并回读校验",()=>{const s=memory();assert.equal(writeResearchNotes([note("1","标题","内容")],s).ok,true);assert.equal(readResearchNotes(s).items.length,1);});
test("笔记保留任务和 Run 关联并支持关联 ID 检索",()=>{const s=memory();assert.equal(writeResearchNotes([{...note("1","研究判断","观察内容"),taskId:"task-42",runId:"run-42"}],s).ok,true);const item=readResearchNotes(s).items[0];assert.equal(item.taskId,"task-42");assert.equal(item.runId,"run-42");assert.equal(filterResearchNotes([item],"task-42").length,1);assert.equal(filterResearchNotes([item],"run-42").length,1);});
test("检索覆盖标题正文代码与标签并支持代码筛选",()=>{const items=[note("1","贵州茅台","现金流良好"),note("2","观察记录","等待验证","000858.SZ",["技术面"])];assert.equal(filterResearchNotes(items,"现金流").length,1);assert.equal(filterResearchNotes(items,"技术面").length,1);assert.equal(filterResearchNotes(items,"","000858").length,1);});
test("损坏数据不会被误判为空且不覆盖原值",()=>{const s=memory();s.setItem(RESEARCH_NOTES_KEY,"{");const before=s.getItem(RESEARCH_NOTES_KEY);assert.equal(readResearchNotes(s).valid,false);assert.equal(writeResearchNotes([],s).ok,false);assert.equal(s.getItem(RESEARCH_NOTES_KEY),before);});
test("超过条数限制拒绝写入",()=>{assert.equal(writeResearchNotes(Array.from({length:201},(_,i)=>note(String(i),"标题","内容")),memory()).ok,false);});
test("关键词检索不执行 HTML",()=>{const x=note("1","<img src=x>","<script>alert(1)</script>");assert.equal(filterResearchNotes([x],"<img").length,1);});

test("从归档报告创建笔记时准确继承股票、Task 和 Run 关联",()=>{const context=buildResearchNoteContextFromReport({runId:"run-9",taskId:"task-9",symbols:["600519.SH","000858.SZ"]});assert.deepEqual(context,{symbol:"600519.SH",taskId:"task-9",runId:"run-9"});});
test("报告缺少有效 Run ID 时不创建关联上下文",()=>{assert.equal(buildResearchNoteContextFromReport({taskId:"task-1"}),null);});

test("拒绝重复笔记 ID，且不会覆盖重复或损坏的旧数据",()=>{const s=memory();const duplicate=[note("same","第一条","内容一"),note("same","第二条","内容二")];assert.equal(writeResearchNotes(duplicate,s).reason,"invalid");s.setItem(RESEARCH_NOTES_KEY,JSON.stringify(duplicate));const before=s.getItem(RESEARCH_NOTES_KEY);assert.equal(readResearchNotes(s).valid,false);assert.equal(writeResearchNotes([note("new","新记录","内容")],s).reason,"invalid-storage");assert.equal(s.getItem(RESEARCH_NOTES_KEY),before);});

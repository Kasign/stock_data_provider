import test from "node:test";
import assert from "node:assert/strict";
import {filterResearchNotes,readResearchNotes,writeResearchNotes,RESEARCH_NOTES_KEY} from "../src/research-notes.js";
function memory(){const data=new Map();return {getItem:k=>data.has(k)?data.get(k):null,setItem:(k,v)=>data.set(k,v),removeItem:k=>data.delete(k)};}
const note=(id,title,content,symbol="600519.SH",tags=["基本面"])=>({id,title,content,symbol,tags,createdAt:"2026-01-01",updatedAt:"2026-01-02"});
test("研究记录可写入并回读校验",()=>{const s=memory();assert.equal(writeResearchNotes([note("1","标题","内容")],s).ok,true);assert.equal(readResearchNotes(s).items.length,1);});
test("检索覆盖标题正文代码与标签并支持代码筛选",()=>{const items=[note("1","贵州茅台","现金流良好"),note("2","观察记录","等待验证","000858.SZ",["技术面"])];assert.equal(filterResearchNotes(items,"现金流").length,1);assert.equal(filterResearchNotes(items,"技术面").length,1);assert.equal(filterResearchNotes(items,"","000858").length,1);});
test("损坏数据不会被误判为空且不覆盖原值",()=>{const s=memory();s.setItem(RESEARCH_NOTES_KEY,"{");const before=s.getItem(RESEARCH_NOTES_KEY);assert.equal(readResearchNotes(s).valid,false);assert.equal(writeResearchNotes([],s).ok,false);assert.equal(s.getItem(RESEARCH_NOTES_KEY),before);});
test("超过条数限制拒绝写入",()=>{assert.equal(writeResearchNotes(Array.from({length:201},(_,i)=>note(String(i),"标题","内容")),memory()).ok,false);});
test("关键词检索不执行 HTML",()=>{const x=note("1","<img src=x>","<script>alert(1)</script>");assert.equal(filterResearchNotes([x],"<img").length,1);});

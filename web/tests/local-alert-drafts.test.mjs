import test from "node:test";import assert from "node:assert/strict";import {readLocalAlertDrafts,writeLocalAlertDrafts,LOCAL_ALERT_DRAFTS_KEY,MAX_LOCAL_ALERT_DRAFTS} from "../src/local-alert-drafts.js";
function storage(){const m=new Map();return {getItem:k=>m.get(k)??null,setItem:(k,v)=>m.set(k,v)};}
const draft=(id)=>({id,name:"价格观察",type:"price_above",threshold:100,symbol:"600519.SH",scope:"symbol",severity:"warning",enabled:true,createdAt:"2026-01-01",updatedAt:"2026-01-02"});
test("草稿写入回读成功",()=>{const s=storage();assert.equal(writeLocalAlertDrafts([draft("a")],s).ok,true);assert.equal(readLocalAlertDrafts(s).items[0].threshold,100);});
test("未知类型、负数阈值和损坏数据拒绝",()=>{const s=storage();assert.equal(writeLocalAlertDrafts([{...draft("a"),type:"invented"}],s).ok,false);assert.equal(writeLocalAlertDrafts([{...draft("a"),threshold:-1}],s).ok,false);s.setItem(LOCAL_ALERT_DRAFTS_KEY,"{");assert.equal(readLocalAlertDrafts(s).valid,false);});
test("草稿数量上限",()=>assert.equal(writeLocalAlertDrafts(Array.from({length:MAX_LOCAL_ALERT_DRAFTS+1},(_,i)=>draft(String(i))),storage()).ok,false));

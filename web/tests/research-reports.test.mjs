import test from "node:test";
import assert from "node:assert/strict";
import {filterResearchReports,readResearchReports,saveResearchReport,writeResearchReports,RESEARCH_REPORTS_KEY} from "../src/research-reports.js";
function memory(){const data=new Map();return {getItem:k=>data.has(k)?data.get(k):null,setItem:(k,v)=>data.set(k,v),removeItem:k=>data.delete(k)};}
const result=(id="run-1")=>({run_id:id,status:"completed",request:{symbols:["600519.SH"],as_of:"2026-10-30T09:30:00+08:00"},quality:{core_pass:true},signals:[],warnings:[]});
test("归档保存完整 Run 结果并保留任务和 API 地址",()=>{const s=memory();assert.equal(saveResearchReport(result(),{taskId:"task-1",analysisBase:"https://analysis.example"},s).ok,true);const item=readResearchReports(s).items[0];assert.equal(item.taskId,"task-1");assert.equal(item.analysisBase,"https://analysis.example");assert.equal(item.result.run_id,"run-1");});
test("同一 API 地址的 Run 重复归档幂等，不同地址可独立保存",()=>{const s=memory();saveResearchReport(result(),{analysisBase:"https://a"},s);assert.equal(saveResearchReport(result(),{analysisBase:"https://a"},s).duplicate,true);assert.equal(saveResearchReport(result(),{analysisBase:"https://b"},s).ok,true);assert.equal(readResearchReports(s).items.length,2);});
test("搜索与归档日期过滤组合有效",()=>{const s=memory();saveResearchReport(result(),{taskId:"task-a"},s);const items=readResearchReports(s).items;assert.equal(filterResearchReports(items,{search:"600519"}).length,1);assert.equal(filterResearchReports(items,{search:"task-a"}).length,1);assert.equal(filterResearchReports(items,{from:"2999-01-01"}).length,0);assert.equal(filterResearchReports(items,{from:"2026-10-31",to:"2026-10-01"}).length,0);});
test("损坏归档数据不会被误判为空或覆盖",()=>{const s=memory();s.setItem(RESEARCH_REPORTS_KEY,"{");const before=s.getItem(RESEARCH_REPORTS_KEY);assert.equal(readResearchReports(s).valid,false);assert.equal(saveResearchReport(result(),{},s).ok,false);assert.equal(s.getItem(RESEARCH_REPORTS_KEY),before);});
test("拒绝缺少 Run ID、单份过大和超过数量上限的归档",()=>{assert.equal(saveResearchReport({status:"completed"},{},memory()).ok,false);const large= result("large");large.warnings=["x".repeat(700001)];assert.equal(saveResearchReport(large,{},memory()).ok,false);assert.equal(saveResearchReport({status:"completed"},{},memory()).ok,false);const s=memory();for(let i=0;i<30;i++)assert.equal(saveResearchReport(result("run-"+i),{},s).ok,true);assert.equal(saveResearchReport(result("run-31"),{},s).reason,"limit");});

test("拒绝异常或超长标的列表，不能静默截断归档内容", () => {
  const s = memory();
  const base = { id: "report-a", runId: "run-a", result: result("run-a") };
  for (const symbols of ["600519.SH", [600519], ["", "600519.SH"], Array.from({ length: 51 }, () => "600519.SH"), ["x".repeat(33)]]) {
    assert.equal(writeResearchReports([{ ...base, symbols }], s).reason, "invalid");
    assert.equal(s.getItem(RESEARCH_REPORTS_KEY), null);
  }
  assert.equal(writeResearchReports([{ ...base, taskId: "x".repeat(161) }], s).reason, "invalid");
});

test("拒绝重复归档 ID，避免写入后才发现存储无效",()=>{const s=memory();const item={id:"same",runId:"run-1",result:result("run-1")};assert.equal(writeResearchReports([item,{...item,runId:"run-2",result:result("run-2")}],s).reason,"invalid");assert.equal(s.getItem(RESEARCH_REPORTS_KEY),null);});

test("读取时同样执行归档总容量限制，超限原始内容不被覆盖", () => {
  const s = memory();
  const raw = " ".repeat(4000001);
  s.setItem(RESEARCH_REPORTS_KEY, raw);
  const state = readResearchReports(s);
  assert.equal(state.readable, true);
  assert.equal(state.valid, false);
  assert.equal(writeResearchReports([], s).reason, "invalid-storage");
  assert.equal(s.getItem(RESEARCH_REPORTS_KEY), raw);
});


test("归档 ID、Run ID 与分析服务地址长度有上限",()=>{const s=memory();const base={id:"report-a",runId:"run-a",result:result("run-a")};for(const item of [{...base,id:"i".repeat(161)},{...base,runId:"r".repeat(161),result:result("r".repeat(161))},{...base,analysisBase:"https://a/".padEnd(2049,"x")}]){assert.equal(writeResearchReports([item],s).reason,"invalid");assert.equal(s.getItem(RESEARCH_REPORTS_KEY),null);}});

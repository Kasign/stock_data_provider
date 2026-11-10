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
  for (const symbols of ["600519.SH", [600519], ["", "600519.SH"], Array.from({ length: 101 }, () => "600519.SH"), ["x".repeat(33)]]) {
    assert.equal(writeResearchReports([{ ...base, symbols }], s).reason, "invalid");
    assert.equal(s.getItem(RESEARCH_REPORTS_KEY), null);
  }
  assert.equal(writeResearchReports([{ ...base, taskId: "x".repeat(161) }], s).reason, "invalid");
});
test("报告归档支持服务端上限 100 只标的，101 只拒绝",()=>{const s=memory();const symbols=Array.from({length:100},(_,i)=>"SH"+String(i).padStart(4,"0"));const item={id:"report-100",runId:"run-100",symbols,result:{...result("run-100"),request:{symbols}}};assert.equal(writeResearchReports([item],s).ok,true);assert.equal(writeResearchReports([{...item,id:"report-101",runId:"run-101",symbols:[...symbols,"SZ9999"],result:{...result("run-101"),request:{symbols:[...symbols,"SZ9999"]}}}],memory()).reason,"invalid");});

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

test("浏览器存储 API 异常时拒绝读取与写入，不报告成功", () => {
  const denied = {
    getItem() { throw new Error("SecurityError"); },
    setItem() {}
  };
  const read = readResearchReports(denied);
  assert.equal(read.readable, false);
  assert.equal(read.valid, false);

  const quota = {
    getItem() { return null; },
    setItem() { throw new Error("QuotaExceededError"); }
  };
  assert.deepEqual(writeResearchReports([{
    id: "report-quota",
    runId: "run-quota",
    result: result("run-quota")
  }], quota), {
    ok: false,
    reason: "unavailable"
  });
});


test("归档创建时间有长度上限，不强制单一日期格式", () => {
  const s = memory();
  const base = { id: "report-time", runId: "run-1", result: result("run-1") };
  assert.equal(writeResearchReports([{ ...base, createdAt: "t".repeat(65) }], s).reason, "invalid");
  assert.equal(s.getItem(RESEARCH_REPORTS_KEY), null);
  assert.equal(writeResearchReports([{ ...base, createdAt: "legacy-date" }], s).ok, true);
});


test("拒绝会被 JSON 序列化静默丢弃或改写的报告字段", () => {
  const make = extra => ({ ...result("run-json"), extra });
  const circular = result("run-cycle");
  circular.self = circular;
  const invalidResults = [
    make({ missing: undefined }),
    make({ date: new Date("2026-01-01T00:00:00Z") }),
    make({ notFinite: Number.NaN }),
    make({ infinity: Number.POSITIVE_INFINITY }),
    make({ negativeZero: -0 }),
    Object.assign(result("run-symbol-key"), { [Symbol("hidden")]: true }),
    make({ callback() {} }),
    make({ token: Symbol("hidden") }),
    make({ amount: 1n }),
    circular,
    Object.assign(result("run-tojson"), { toJSON() { return { run_id: "rewritten" }; } })
  ];
  for (const invalid of invalidResults) {
    const s = memory();
    assert.equal(saveResearchReport(invalid, {}, s).reason, "invalid-result");
    assert.equal(s.getItem(RESEARCH_REPORTS_KEY), null);
    assert.equal(writeResearchReports([{
      id: "report-json",
      runId: invalid.run_id,
      result: invalid
    }], s).reason, "invalid");
    assert.equal(s.getItem(RESEARCH_REPORTS_KEY), null);
  }
});

test("JSON 报告允许共享引用，但不允许稀疏数组或访问器字段", () => {
  const shared = { value: 1 };
  const valid = result("run-shared");
  valid.extra = { first: shared, second: shared };
  assert.equal(saveResearchReport(valid, {}, memory()).ok, true);

  const sparse = result("run-sparse");
  sparse.extra = [];
  sparse.extra.length = 2;
  sparse.extra[1] = "present";
  assert.equal(saveResearchReport(sparse, {}, memory()).reason, "invalid-result");

  const accessor = result("run-accessor");
  Object.defineProperty(accessor, "secret", { enumerable: true, get() { return "hidden"; } });
  assert.equal(saveResearchReport(accessor, {}, memory()).reason, "invalid-result");
});

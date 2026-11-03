import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import { runResultFixtures } from "./fixtures/run-results.mjs";

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"..");
const screenshotDir=path.resolve(root,"../browser-screenshots/run-flow-outcomes");
await mkdir(screenshotDir,{recursive:true});
const tasks=[
 {task_id:"task-partial",run_id:"run-partial",symbols:["600519"],task_type:"post_market_scan",status:"partial",analysis_base:"/analysis-api"},
 {task_id:"task-failed",run_id:"run-failed",symbols:["000001"],task_type:"post_market_scan",status:"failed",failure_code:"market_snapshot_failed",error_message:"读取行情快照失败",analysis_base:"/analysis-api"},
 {task_id:"task-event-unavailable",run_id:"run-event-unavailable",symbols:["600519"],task_type:"post_market_scan",status:"succeeded",analysis_base:"/analysis-api"},
 {task_id:"task-race-a",run_id:"run-race-a",symbols:["600519"],task_type:"post_market_scan",status:"succeeded",analysis_base:"/analysis-api"},
 {task_id:"task-race-b",run_id:"run-race-b",symbols:["000001"],task_type:"post_market_scan",status:"running",analysis_base:"/analysis-api"}
];
const runIds=Object.fromEntries(tasks.map(t=>[t.task_id,t.run_id]));
const symbols=Object.fromEntries(tasks.map(t=>[t.task_id,t.symbols[0]]));
const statuses=Object.fromEntries(tasks.map(t=>[t.task_id,t.status]));
const eventPage=id=>({task_id:id,next_after_sequence:1,events:[{sequence:1,event:{run_id:runIds[id],stage:id==="task-partial"?"数据质量检查":id==="task-failed"?"读取行情快照":"任务结束",status:id==="task-partial"?"degraded":id==="task-failed"?"failed":"completed",message:"浏览器专项合成事件",at:"2026-11-03T01:00:00Z"}}]});
const server=createServer(async(req,res)=>{const p=decodeURIComponent(new URL(req.url||"/","http://127.0.0.1").pathname);const file=path.resolve(root,"."+(p==="/"?"/index.html":p));if(file!==root&&!file.startsWith(root+path.sep)){res.writeHead(403);res.end();return;}try{const body=await readFile(file),ext=path.extname(file);res.writeHead(200,{"content-type":ext===".html"?"text/html; charset=utf-8":ext===".css"?"text/css; charset=utf-8":"text/javascript; charset=utf-8","cache-control":"no-store"});res.end(body);}catch{res.writeHead(404);res.end();}});
await new Promise((ok,fail)=>{server.once("error",fail);server.listen(0,"127.0.0.1",ok);});
const addr=server.address();assert.ok(addr&&typeof addr==="object");
let browser,releaseA,startA;const startedA=new Promise(resolve=>startA=resolve);let eventAttempts=0;const runRequests=[];
try{
 browser=await chromium.launch({headless:true});
 const context=await browser.newContext({viewport:{width:1280,height:900},reducedMotion:"reduce"});
 await context.addInitScript(seed=>{localStorage.setItem("stock-provider-webui.settings.v1",JSON.stringify({marketBase:"/market-api",analysisBase:"/analysis-api",apiKey:"browser-test-key"}));localStorage.setItem("stock-provider-webui.recent-analysis.v1",JSON.stringify(seed));},tasks);
 const page=await context.newPage(),pageErrors=[];page.on("pageerror",e=>pageErrors.push(e.message));
 await page.route("**/*",async route=>{
  const p=new URL(route.request().url()).pathname,parts=p.split("/"),ti=parts.indexOf("tasks"),ri=parts.indexOf("runs");
  const json=(status,body)=>route.fulfill({status,contentType:"application/json",body:JSON.stringify(body)});
  if(ti>=0&&parts[ti+1]?.startsWith("task-")){
   const id=parts[ti+1];
   if(parts[ti+2]==="events"){if(id==="task-event-unavailable"&&eventAttempts++===0)return json(404,{error:"事件接口暂不可用"});return json(200,eventPage(id));}
   if(id==="task-race-a"){startA();await new Promise(resolve=>releaseA=resolve);}
   const body={task_id:id,run_id:runIds[id],status:statuses[id],symbols:[symbols[id]],task_type:"post_market_scan",current_stage:statuses[id]==="running"?"正在执行":"任务已结束"};
   if(id==="task-failed"){body.failure_code="market_snapshot_failed";body.error_message="读取行情快照失败";}
   return json(200,body);
  }
  if(ri>=0&&parts[ri+1]?.startsWith("run-")){
   const id=parts[ri+1];runRequests.push(id);
   const taskId=Object.keys(runIds).find(k=>runIds[k]===id);
   const fixture=id==="run-partial"?runResultFixtures.partial:id==="run-failed"?runResultFixtures.failed:runResultFixtures.completed;
   return json(200,{...fixture,run_id:id,request:{...fixture.request,symbols:[symbols[taskId]]}});
  }
  if(p.includes("/api/")||p.endsWith("/readyz"))return json(503,{error:"fixture endpoint unavailable"});
  return route.continue();
 });
 await page.goto("http://127.0.0.1:"+addr.port,{waitUntil:"load"});
 await page.locator('[data-view="tasks"]').click();
 await page.locator('[data-open-task-id="task-partial"]').click();
 await page.locator("#run-flow-dialog[open]").waitFor();
 await page.waitForFunction(()=>document.querySelector("#task-run-status-pill")?.dataset.runStatus==="partial");
 assert.match(await page.locator("#task-status-pill").textContent(),/部分完成/);
 assert.match(await page.locator("#task-run-status-pill").textContent(),/部分结果/);
 assert.match(await page.locator("#task-events").textContent(),/数据质量检查/);
 assert.ok(runRequests.includes("run-partial"));
 await page.locator("#close-run-flow-dialog").click();await page.locator("#run-flow-dialog:not([open])").waitFor({state:"hidden"});
 await page.locator('[data-open-task-id="task-failed"]').click();
 await page.locator("#run-flow-dialog[open]").waitFor();
 await page.waitForFunction(()=>document.querySelector("#task-run-status-pill")?.dataset.runStatus==="failed");
 assert.match(await page.locator("#task-status-pill").textContent(),/执行失败/);
 assert.match(await page.locator("#task-error").textContent(),/market_snapshot_failed/);
 assert.match(await page.locator("#task-error").textContent(),/读取行情快照失败/);
 assert.match(await page.locator("#task-run-status-pill").textContent(),/分析失败/);
 assert.ok(runRequests.includes("run-failed"));
 await page.locator("#close-run-flow-dialog").click();await page.locator("#run-flow-dialog:not([open])").waitFor({state:"hidden"});
 await page.locator('[data-open-task-id="task-event-unavailable"]').click();
 await page.locator("#run-flow-dialog[open]").waitFor();
 await page.waitForFunction(()=>document.querySelector("#task-poll-label")?.textContent.includes("Run 报告暂不加载"));
 assert.equal(runRequests.includes("run-event-unavailable"),false);
 assert.match(await page.locator("#task-events-read-status").textContent(),/阶段事件读取提示/);
 await page.locator("#refresh-task").click();
 await page.waitForFunction(()=>document.querySelector("#task-run-status-pill")?.dataset.runStatus==="completed",null,{timeout:10000});
 await page.waitForFunction(()=>document.querySelector("#task-run-status-pill")?.dataset.runStatus==="completed",null,{timeout:10000});
 assert.equal(eventAttempts,2);
 await page.locator("#close-run-flow-dialog").click();await page.locator("#run-flow-dialog:not([open])").waitFor({state:"hidden"});
 await page.locator('[data-open-task-id="task-race-a"]').click();await page.locator("#run-flow-dialog[open]").waitFor();await startedA;
 await page.locator("#close-run-flow-dialog").click();await page.locator("#run-flow-dialog:not([open])").waitFor({state:"hidden"});
 await page.locator('[data-open-task-id="task-race-b"]').click();await page.locator("#run-flow-dialog[open]").waitFor();
 await page.waitForFunction(()=>document.querySelector("#task-id-value")?.textContent==="task-race-b"&&document.querySelector("#task-status-pill")?.textContent.includes("执行中"));
 assert.equal(await page.locator("#task-run-status-pill").getAttribute("data-run-status"),"unknown");
 releaseA();await page.waitForTimeout(150);
 assert.equal(await page.locator("#task-id-value").textContent(),"task-race-b");
 assert.match(await page.locator("#task-status-pill").textContent(),/执行中/);
 assert.equal(runRequests.includes("run-race-a"),false);
 assert.deepEqual(pageErrors,[]);
 await page.screenshot({path:path.join(screenshotDir,"1280x900.png"),fullPage:false,animations:"disabled"});
 console.log("PASS Run Flow partial, failed, event recovery and stale-response isolation");
 await context.close();
}finally{if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}

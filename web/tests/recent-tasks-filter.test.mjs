import test from "node:test";
import assert from "node:assert/strict";
import { filterRecentTasks } from "../src/recent-tasks.js";
const items=[
 {task_id:"a1",symbols:["600519.SH"],task_type:"post_market_scan",status:"succeeded"},
 {task_id:"b2",symbols:["000001.SZ"],task_type:"watchlist_scan",status:"failed"},
 {task_id:"c3",symbols:["300750.SZ"],task_type:"post_market_scan",status:"running"}
];
test("filters recent task references by status, type and keyword without mutating source",()=>{
 assert.deepEqual(filterRecentTasks(items,{status:"failed"}).map(x=>x.task_id),["b2"]);
 assert.deepEqual(filterRecentTasks(items,{taskType:"post_market_scan"}).map(x=>x.task_id),["a1","c3"]);
 assert.deepEqual(filterRecentTasks(items,{search:"600519"}).map(x=>x.task_id),["a1"]);
 assert.deepEqual(filterRecentTasks(items,{status:"running",taskType:"watchlist_scan"}),[]);
 assert.equal(items.length,3);
});

test("filters recent tasks by creation date and searches failure diagnostics", () => {
 const items = [
  { task_id: "a1", created_at: "2026-10-29T02:00:00Z", status: "failed", failure_code: "PROVIDER_TIMEOUT", error_message: "行情源超时" },
  { task_id: "b2", created_at: "2026-10-30T02:00:00Z", status: "succeeded" },
  { task_id: "c3", created_at: "", status: "failed" }
 ];
 assert.deepEqual(filterRecentTasks(items, { from: "2026-10-29", to: "2026-10-29" }).map((item) => item.task_id), ["a1"]);
 assert.deepEqual(filterRecentTasks(items, { from: "2026-10-31" }), []);
 assert.deepEqual(filterRecentTasks(items, { from: "2026-10-30", to: "2026-10-29" }), []);
 assert.deepEqual(filterRecentTasks(items, { search: "PROVIDER_TIMEOUT" }).map((item) => item.task_id), ["a1"]);
 assert.deepEqual(filterRecentTasks(items, { search: "行情源超时" }).map((item) => item.task_id), ["a1"]);
});

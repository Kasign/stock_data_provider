import test from "node:test";
import assert from "node:assert/strict";
import { assignWatchlistGroup, createWatchlistGroup, DEFAULT_WATCHLIST_GROUP, readWatchlistGroups, removeWatchlistGroup, WATCHLIST_GROUPS_KEY } from "../src/watchlist-groups.js";
function storage(seed = null) { let value = seed; return { getItem() { return value; }, setItem(_key, next) { value = String(next); } }; }
test("创建、分配和删除分组时保留股票清单本身", () => {
 const db=storage(); assert.equal(createWatchlistGroup("长期持有",db).persisted,true);
 assert.equal(assignWatchlistGroup("600519.SH","长期持有",db).persisted,true);
 assert.equal(readWatchlistGroups(db).assignments["600519.SH"],"长期持有");
 assert.equal(removeWatchlistGroup("长期持有",db).persisted,true);
 assert.equal(readWatchlistGroups(db).assignments["600519.SH"],undefined);
 assert.deepEqual(readWatchlistGroups(db).groups,[DEFAULT_WATCHLIST_GROUP]);
});
test("拒绝重复、空名称和超出数量上限的分组",()=>{
 const db=storage(); assert.equal(createWatchlistGroup(" ",db).reason,"invalid-name");
 assert.equal(createWatchlistGroup("A",db).changed,true); assert.equal(createWatchlistGroup("A",db).reason,"duplicate");
 for(let i=0;i<10;i++) createWatchlistGroup("G"+i,db);
 assert.equal(createWatchlistGroup("overflow",db).reason,"limit");
});
test("分组存储损坏时不覆盖原始内容",()=>{
 const db=storage("{bad"),result=createWatchlistGroup("测试",db);
 assert.equal(result.valid,false); assert.equal(result.reason,"invalid-storage");
 assert.equal(db.getItem(WATCHLIST_GROUPS_KEY),"{bad");
});
test("浏览器禁止访问 localStorage 时安全降级",()=>{
 const blocked={getItem(){throw new Error("blocked")}};
 const result=readWatchlistGroups(blocked);
 assert.equal(result.valid,false); assert.equal(result.readable,false);
 assert.equal(createWatchlistGroup("测试",blocked).reason,"invalid-storage");
});

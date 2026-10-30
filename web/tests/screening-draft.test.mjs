import test from "node:test";
import assert from "node:assert/strict";
import { buildScreeningDraftSummary, createScreeningCondition, formatScreeningCondition, normalizeScreeningConditions } from "../src/screening-draft.js";
test("accepts bounded numeric conditions for supported market fields",()=>{
 assert.deepEqual(createScreeningCondition("change_pct","gte","2.5"),{valid:true,condition:{field:"change_pct",operator:"gte",value:2.5}});
 assert.equal(createScreeningCondition("turnover_rate","lte","101").reason,"range");
 assert.equal(createScreeningCondition("unknown","gte","1").valid,false);
 assert.equal(createScreeningCondition("amount","gte","").valid,false);
});
test("normalizes conditions and creates an explicit local-only summary",()=>{
 const list=normalizeScreeningConditions([{field:"ma5",operator:"gte",value:10},{field:"bad",operator:"gte",value:2},{field:"ma20",operator:"gte",value:Infinity}]);
 assert.equal(list.length,1);
 const summary=buildScreeningDraftSummary(list);
 assert.equal(summary.count,1);
 assert.match(summary.conditions[0],/MA5 ≥ 10.00 元/);
 assert.match(summary.description,/仅当前页面内存有效/);
 assert.match(buildScreeningDraftSummary([]).description,/尚未添加/);
});
test("formats amount conditions without pretending to have actual market data",()=>{
 assert.equal(formatScreeningCondition({field:"amount",operator:"gte",value:10000000}),"成交额 ≥ 10,000,000 元");
});
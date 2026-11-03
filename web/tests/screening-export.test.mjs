import test from "node:test";
import assert from "node:assert/strict";
import { buildScreeningCSV } from "../src/screening-export.js";

test("exports judgment, quality and page metadata while leaving missing values blank", () => {
  const csv = buildScreeningCSV({
    rows: [{ status: "待判断", item: { symbol: "600001.SH", name: "示例股票",
      quote: { change_percent: null, turnover_rate: 2.5, amount: 1200000, amount_unit: "CNY",
        quality_status: "degraded", quality_reasons: ["涨跌幅未报告"] }, ma5: null, ma20: 10 } }],
    page: 2, pageSize: 50, adjustment: "forward_adjusted", logic: "and",
    conditions: [{ field: "ma5", operator: "gte", value: 10 }]
  });
  assert.match(csv, /"证券代码","证券名称"/);
  assert.match(csv, /"600001.SH","示例股票",,"2.5",1200000,"CNY",,"10","degraded","涨跌幅未报告","待判断","2","50","forward_adjusted"/);
  assert.match(csv, /"当前页筛选结果"/);
  assert.match(csv, /缺失数值保持为空/);
});

test("preserves numeric zero and neutralizes spreadsheet formulas in text fields", () => {
  const csv = buildScreeningCSV({ rows: [{ status: "命中", item: {
    symbol: "000001.SZ", name: '=HYPERLINK("https://bad.example","x"),A',
    quote: { change_percent: 0, turnover_rate: 0, amount: 0 }, ma5: 0, ma20: 0
  } }] });
  assert.ok(csv.includes("\"'=HYPERLINK(\"\"https://bad.example\"\",\"\"x\"\"),A\""));
  assert.match(csv, /,"0","0","0",/);
});

test("does not fabricate data rows for an empty result set", () => {
  const csv = buildScreeningCSV({ rows: [] });
  assert.equal(csv.split("\\r\\n").filter(Boolean).length, 5);
});

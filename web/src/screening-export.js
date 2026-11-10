const CSV_COLUMNS = Object.freeze([
  ["证券代码", item => item.symbol],
  ["证券名称", item => item.name],
  ["涨跌幅(%)", item => item.quote.change_percent],
  ["换手率(%)", item => item.quote.turnover_rate],
  ["成交额", item => item.quote.amount],
  ["成交额单位", item => item.quote.amount_unit || item.amount_unit],
  ["MA5", item => item.ma5],
  ["MA20", item => item.ma20],
  ["数据质量", item => item.quality],
  ["质量原因", item => item.reasons],
  ["筛选判断", item => item.status],
  ["行情页码", item => item.page],
  ["每页条数", item => item.pageSize],
  ["复权口径", item => item.adjustment]
]);

function csvCell(value) {
  if (value === null || value === undefined) return "";
  let text = String(value);
  if (/^[\s]*[=+\-@]/.test(text)) text = "'" + text;
  return '"' + text.replace(/"/g, '""') + '"';
}

function numericCell(value) {
  if (value === null || value === undefined || (typeof value !== "number" && typeof value !== "string") || (typeof value === "string" && !value.trim())) return "";
  const number = Number(value);
  return Number.isFinite(number) ? String(number) : "";
}

export function buildScreeningCSV({ rows = [], page, pageSize, adjustment, logic = "and", conditions = [] } = {}) {
  const header = CSV_COLUMNS.map(([label]) => csvCell(label)).join(",");
  const records = (Array.isArray(rows) ? rows : []).map(({ item = {}, status = "" }) => {
    const quote = item.quote && typeof item.quote === "object" ? item.quote : {};
    const reasons = [...(Array.isArray(item.quality_reasons) ? item.quality_reasons : []),
      ...(Array.isArray(quote.quality_reasons) ? quote.quality_reasons : [])].filter(Boolean);
    const normalized = {
      symbol: item.symbol || item.code || quote.symbol || "",
      name: item.name || quote.name || "",
      quote,
      ma5: item.ma5,
      ma20: item.ma20,
      quality: item.quality_status || quote.quality_status || "unknown",
      reasons: [...new Set(reasons)].join("；"),
      status, page, pageSize, adjustment
    };
    return CSV_COLUMNS.map(([label, getter]) => {
      const value = getter(normalized);
      return ["涨跌幅(%)", "换手率(%)", "成交额", "MA5", "MA20"].includes(label)
        ? numericCell(value) : csvCell(value);
    }).join(",");
  });
  const meta = [
    ["导出范围", "当前页筛选结果"],
    ["筛选逻辑", logic === "or" ? "OR" : "AND"],
    ["筛选条件", (Array.isArray(conditions) ? conditions : []).map(item =>
      String(item.field || "") + " " + String(item.operator || "") + " " + String(item.value ?? "")).join("；")],
    ["说明", "仅为本次已加载分页的本地筛选结果，不代表全市场扫描；缺失数值保持为空。"]
  ].map(row => row.map(csvCell).join(","));
  return [...meta, "", header, ...records].join("\r\n") + "\r\n";
}

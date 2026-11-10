import { formatDateTime } from "./format.js";

function hasValue(value) {
  return value !== null && value !== undefined && value !== "";
}

function displayValue(value) {
  return hasValue(value) ? String(value) : "未报告";
}

export function buildResearchMetaText(data = {}, options = {}) {
  const fields = Array.isArray(options.details)
    ? options.details.filter((entry) => Array.isArray(entry) && entry.length >= 2)
    : [];
  const source = hasValue(options.source) ? options.source : data?.source;
  fields.push(["来源", displayValue(source)]);

  if (hasValue(data?.source_version)) fields.push(["源端版本", data.source_version]);
  if (options.includeProviderTimestamp) {
    fields.push(["供应商时间", hasValue(data?.provider_timestamp) ? formatDateTime(data.provider_timestamp) : "未报告"]);
  } else if (hasValue(data?.provider_timestamp)) {
    fields.push(["供应商时间", formatDateTime(data.provider_timestamp)]);
  }
  if (hasValue(data?.provider_report_date)) fields.push(["源端报告日期", data.provider_report_date]);
  if (options.includeProviderDate && !hasValue(data?.provider_date)) {
    fields.push(["供应商日期", "未报告"]);
  } else if (hasValue(data?.provider_date)) {
    fields.push(["供应商日期", data.provider_date]);
  }
  if (options.includeDataDate && !hasValue(data?.date)) {
    fields.push(["数据日期", "未报告"]);
  } else if (hasValue(data?.date)) {
    fields.push(["数据日期", data.date]);
  }
  if (hasValue(data?.trade_date)) fields.push(["交易日期", data.trade_date]);
  fields.push(["获取时间", hasValue(data?.fetched_at) ? formatDateTime(data.fetched_at) : "未报告"]);
  return fields.map(([label, value]) => label + " " + displayValue(value)).join(" · ");
}

import { finiteNumberOrNull } from "./format.js";

const QUOTE_SORT_FIELDS = Object.freeze({
  price: "current_price",
  change: "change_percent",
  amount: "amount",
  turnover: "turnover_rate"
});

function compareText(left, right) {
  return String(left ?? "").localeCompare(String(right ?? ""), "zh-CN");
}

export function sortWatchlistItems(items, { mode = "updated", direction = "desc", getQuote = () => null } = {}) {
  const source = Array.isArray(items) ? [...items] : [];
  const sign = direction === "asc" ? 1 : -1;
  const quoteField = QUOTE_SORT_FIELDS[mode];

  source.sort((left, right) => {
    if (quoteField) {
      const leftValue = finiteNumberOrNull(getQuote(left)?.[quoteField]);
      const rightValue = finiteNumberOrNull(getQuote(right)?.[quoteField]);
      // 缺失、失败或非有限行情始终排在末尾，不受升降序影响。
      if (leftValue === null && rightValue !== null) return 1;
      if (leftValue !== null && rightValue === null) return -1;
      if (leftValue !== null && rightValue !== null && leftValue !== rightValue) {
        return sign * (leftValue - rightValue);
      }
    } else if (mode === "symbol") {
      const result = compareText(left.symbol, right.symbol);
      if (result) return sign * result;
    } else if (mode === "name") {
      const result = compareText(left.name, right.name);
      if (result) return sign * result;
    } else {
      const leftTime = Date.parse(left.updated_at || "") || 0;
      const rightTime = Date.parse(right.updated_at || "") || 0;
      if (leftTime !== rightTime) return sign * (leftTime - rightTime);
    }
    return compareText(left.symbol, right.symbol);
  });

  return source;
}

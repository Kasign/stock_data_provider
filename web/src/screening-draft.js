export const SCREENING_FIELDS = Object.freeze({
  change_pct: { label: "涨跌幅", unit: "%", min: -30, max: 30, step: 0.01 },
  turnover_rate: { label: "换手率", unit: "%", min: 0, max: 100, step: 0.01 },
  amount: { label: "成交额", unit: "元", min: 0, max: 100000000000, step: 10000 },
  ma5: { label: "MA5", unit: "元", min: 0, max: 1000000, step: 0.01 },
  ma20: { label: "MA20", unit: "元", min: 0, max: 1000000, step: 0.01 }
});
export const SCREENING_OPERATORS = Object.freeze({
  gte: { label: "大于等于", symbol: "≥" },
  lte: { label: "小于等于", symbol: "≤" }
});
export function createScreeningCondition(field, operator, rawValue) {
  const definition = SCREENING_FIELDS[field];
  const value = Number(rawValue);
  if (!definition || !SCREENING_OPERATORS[operator] || rawValue === "" || !Number.isFinite(value)) return { valid: false, reason: "invalid" };
  if (value < definition.min || value > definition.max) return { valid: false, reason: "range" };
  return { valid: true, condition: { field, operator, value } };
}
export function normalizeScreeningConditions(conditions = []) {
  if (!Array.isArray(conditions)) return [];
  return conditions.filter((item) => item && SCREENING_FIELDS[item.field] && SCREENING_OPERATORS[item.operator]
    && Number.isFinite(item.value) && item.value >= SCREENING_FIELDS[item.field].min && item.value <= SCREENING_FIELDS[item.field].max)
    .map(({ field, operator, value }) => ({ field, operator, value }));
}
export function formatScreeningCondition(condition) {
  const field = SCREENING_FIELDS[condition.field], operator = SCREENING_OPERATORS[condition.operator];
  if (!field || !operator) return "";
  const digits = condition.field === "amount" ? 0 : 2;
  return field.label + " " + operator.symbol + " " + Number(condition.value).toLocaleString("zh-CN", { minimumFractionDigits: digits, maximumFractionDigits: digits }) + (field.unit ? " " + field.unit : "");
}
export function buildScreeningDraftSummary(conditions = []) {
  const normalized = normalizeScreeningConditions(conditions);
  return {
    count: normalized.length,
    conditions: normalized.map(formatScreeningCondition),
    description: normalized.length
      ? "本地条件草稿（仅当前页面内存有效），共 " + normalized.length + " 条；尚未提交，也未产生扫描结果。"
      : "尚未添加筛选条件。当前仅编辑页面内存中的草稿，不会保存到服务端或生成扫描结果。"
  };
}
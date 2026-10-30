export const SCREENING_PLANS_KEY = "stock-provider-webui.screening-plans.v1";
export const MAX_SCREENING_PLANS = 30;
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
function resolveStorage(storage) {
  if (storage !== undefined) return storage;
  try { return globalThis.localStorage; } catch { return null; }
}
function cleanPlanName(value) {
  return String(value ?? "").replace(/[\u0000-\u001f\u007f]/g, "").trim().slice(0, 40).trim();
}
export function createScreeningCondition(field, operator, rawValue) {
  const definition = SCREENING_FIELDS[field], value = Number(rawValue);
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
export function buildScreeningDraftSummary(conditions = [], logic = "and") {
  const normalized = normalizeScreeningConditions(conditions), normalizedLogic = logic === "or" ? "or" : "and";
  return {
    count: normalized.length, logic: normalizedLogic, conditions: normalized.map(formatScreeningCondition),
    description: normalized.length
      ? "本地条件草稿 · " + normalized.length + " 条 · 组合关系 " + normalizedLogic.toUpperCase() + "；尚未提交，也未产生扫描结果。"
      : "尚未添加条件。当前仅编辑本地草稿，不会提交服务端或生成扫描结果。"
  };
}
function normalizePlan(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const id = String(value.id || "").trim(), name = cleanPlanName(value.name);
  const logic = value.logic === "or" || value.logic === "and" ? value.logic : "";
  const conditions = normalizeScreeningConditions(value.conditions);
  if (!id || !name || !logic || !conditions.length || conditions.length !== value.conditions.length || !value.created_at || !value.updated_at) return null;
  return { id, name, logic, conditions, created_at: String(value.created_at), updated_at: String(value.updated_at) };
}
export function readScreeningPlans(storage) {
  const target = resolveStorage(storage);
  if (!target) return { plans: [], readable: false, valid: false, reason: "unavailable" };
  let raw;
  try { raw = target.getItem(SCREENING_PLANS_KEY); } catch { return { plans: [], readable: false, valid: false, reason: "unavailable" }; }
  if (raw == null) return { plans: [], readable: true, valid: true, reason: "" };
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length > MAX_SCREENING_PLANS) throw new Error("invalid");
    const plans = parsed.map(normalizePlan);
    if (plans.some((item) => !item) || new Set(plans.map((item) => item.id)).size !== plans.length) throw new Error("invalid");
    return { plans, readable: true, valid: true, reason: "" };
  } catch { return { plans: [], readable: true, valid: false, reason: "invalid" }; }
}
function persistPlans(plans, storage) {
  const target = resolveStorage(storage);
  if (!target) return false;
  let previous;
  try {
    previous = target.getItem(SCREENING_PLANS_KEY);
    const serialized = JSON.stringify(plans);
    target.setItem(SCREENING_PLANS_KEY, serialized);
    if (target.getItem(SCREENING_PLANS_KEY) === serialized) return true;
    if (previous == null) target.removeItem(SCREENING_PLANS_KEY); else target.setItem(SCREENING_PLANS_KEY, previous);
    return false;
  } catch {
    try { if (previous == null) target.removeItem(SCREENING_PLANS_KEY); else target.setItem(SCREENING_PLANS_KEY, previous); } catch {}
    return false;
  }
}
export function saveScreeningPlan({ id = "", name, conditions, logic = "and" } = {}, storage, now = new Date().toISOString()) {
  const state = readScreeningPlans(storage);
  if (!state.readable || !state.valid) return { saved: false, reason: "storage-invalid", plans: state.plans };
  const normalizedName = cleanPlanName(name), normalizedConditions = normalizeScreeningConditions(conditions);
  const normalizedLogic = logic === "or" || logic === "and" ? logic : "";
  if (!normalizedName) return { saved: false, reason: "invalid-name", plans: state.plans };
  if (!normalizedConditions.length) return { saved: false, reason: "empty-conditions", plans: state.plans };
  if (!normalizedLogic) return { saved: false, reason: "invalid-logic", plans: state.plans };
  const existing = id ? state.plans.find((item) => item.id === id) : null;
  if (id && !existing) return { saved: false, reason: "not-found", plans: state.plans };
  if (state.plans.some((item) => item.id !== id && item.name.toLocaleLowerCase() === normalizedName.toLocaleLowerCase())) return { saved: false, reason: "duplicate-name", plans: state.plans };
  if (!existing && state.plans.length >= MAX_SCREENING_PLANS) return { saved: false, reason: "limit", plans: state.plans };
  let nextId = id;
  if (!nextId) { const baseId = "screening-" + Date.now(); nextId = baseId; let suffix = 1; while (state.plans.some((item) => item.id === nextId)) nextId = baseId + "-" + suffix++; }
  const plan = { id: nextId, name: normalizedName, logic: normalizedLogic, conditions: normalizedConditions, created_at: existing?.created_at || now, updated_at: now };
  const plans = existing ? state.plans.map((item) => item.id === id ? plan : item) : [plan, ...state.plans];
  const persisted = persistPlans(plans, storage);
  return { saved: persisted, reason: persisted ? "" : "persistence-failed", plan: persisted ? plan : null, plans: persisted ? plans : state.plans };
}
export function deleteScreeningPlan(id, storage) {
  const state = readScreeningPlans(storage);
  if (!state.readable || !state.valid) return { deleted: false, reason: "storage-invalid", plans: state.plans };
  const plans = state.plans.filter((item) => item.id !== id);
  if (plans.length === state.plans.length) return { deleted: false, reason: "not-found", plans: state.plans };
  const persisted = persistPlans(plans, storage);
  return { deleted: persisted, reason: persisted ? "" : "persistence-failed", plans: persisted ? plans : state.plans };
}

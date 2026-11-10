export const PREFERENCES_KEY = "stock-provider-webui.preferences.v1";

export const WATCHLIST_DISPLAY_FIELDS = Object.freeze(["updated", "quote", "today", "task", "result"]);

export const DEFAULT_PREFERENCES = Object.freeze({
  watchlistDisplayOrder: [...WATCHLIST_DISPLAY_FIELDS],
  watchlistSort: "updated",
  watchlistSortDirection: "desc",
  watchlistDisplayFields: [...WATCHLIST_DISPLAY_FIELDS],
  marketColorScheme: "cn",
  priceDigits: 2,
  percentDigits: 2,
  amountDisplayUnit: "compact",
  dailyBarLimit: 120,
  dailyAdjustment: "forward_adjusted"
});

function normalizeWatchlistDisplayOrder(value) {
  const requested = Array.isArray(value) ? value.filter((field) => WATCHLIST_DISPLAY_FIELDS.includes(field)) : [];
  return [...new Set([...requested, ...WATCHLIST_DISPLAY_FIELDS])];
}

const SORT_MODES = new Set(["updated", "symbol", "name", "price", "change", "amount", "turnover"]);
const SORT_DIRECTIONS = new Set(["asc", "desc"]);

export function normalizePreferences(value = {}) {
  const source = value && typeof value === "object" && !Array.isArray(value) ? value : {};
  return {
    watchlistSort: SORT_MODES.has(source.watchlistSort) ? source.watchlistSort : DEFAULT_PREFERENCES.watchlistSort,
    watchlistSortDirection: SORT_DIRECTIONS.has(source.watchlistSortDirection)
      ? source.watchlistSortDirection
      : DEFAULT_PREFERENCES.watchlistSortDirection,
    watchlistDisplayFields: Array.isArray(source.watchlistDisplayFields)
      ? WATCHLIST_DISPLAY_FIELDS.filter((field) => source.watchlistDisplayFields.includes(field))
      : [...DEFAULT_PREFERENCES.watchlistDisplayFields],
    watchlistDisplayOrder: normalizeWatchlistDisplayOrder(source.watchlistDisplayOrder),
    marketColorScheme: ["cn", "reverse"].includes(source.marketColorScheme) ? source.marketColorScheme : DEFAULT_PREFERENCES.marketColorScheme,
    priceDigits: [2, 3].includes(source.priceDigits) ? source.priceDigits : DEFAULT_PREFERENCES.priceDigits,
    percentDigits: [2, 3].includes(source.percentDigits) ? source.percentDigits : DEFAULT_PREFERENCES.percentDigits,
    amountDisplayUnit: ["compact", "raw"].includes(source.amountDisplayUnit) ? source.amountDisplayUnit : DEFAULT_PREFERENCES.amountDisplayUnit,
    dailyBarLimit: [60, 120, 250].includes(Number(source.dailyBarLimit)) ? Number(source.dailyBarLimit) : DEFAULT_PREFERENCES.dailyBarLimit,
    dailyAdjustment: ["forward_adjusted", "backward_adjusted", "none"].includes(source.dailyAdjustment) ? source.dailyAdjustment : DEFAULT_PREFERENCES.dailyAdjustment
  };
}

function resolveStorage(storage) {
  if (storage !== undefined) return storage;
  try {
    return globalThis.localStorage;
  } catch {
    return null;
  }
}

export function readPreferencesState(storage) {
  const target = resolveStorage(storage);
  if (!target) return { readable: false, valid: false, value: null, reason: "unavailable" };
  let raw;
  try {
    raw = target.getItem(PREFERENCES_KEY);
  } catch {
    return { readable: false, valid: false, value: null, reason: "unavailable" };
  }
  if (raw === null) return { readable: true, valid: true, value: normalizePreferences({}), reason: "" };
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { readable: true, valid: false, value: null, reason: "invalid" };
    }
    return { readable: true, valid: true, value: normalizePreferences(parsed), reason: "" };
  } catch {
    return { readable: true, valid: false, value: null, reason: "invalid" };
  }
}

export function readPreferences(storage) {
  const state = readPreferencesState(storage);
  return state.valid ? state.value : { ...DEFAULT_PREFERENCES };
}

function persistPreferences(value, storage, allowInvalidStorage = false) {
  const target = resolveStorage(storage);
  if (!target) throw new Error("浏览器本地存储不可用。");
  const current = readPreferencesState(target);
  if (!current.readable) throw new Error("浏览器本地存储不可用。");
  if (!current.valid && !allowInvalidStorage) {
    throw new Error("本地展示偏好格式异常，为保护原始数据未覆盖；如需恢复，请明确执行“恢复默认值”。");
  }
  const preferences = normalizePreferences(value);
  const serialized = JSON.stringify(preferences);
  target.setItem(PREFERENCES_KEY, serialized);
  if (target.getItem(PREFERENCES_KEY) !== serialized) {
    throw new Error("偏好设置未能保存，请检查浏览器存储权限或可用空间。");
  }
  return preferences;
}

export function savePreferences(value, storage) {
  return persistPreferences(value, storage);
}

export function resetPreferences(storage) {
  // 用户显式触发恢复默认值时，允许替换无法解析的旧偏好数据。
  return persistPreferences(DEFAULT_PREFERENCES, storage, true);
}

export function normalizeImportedPreferences(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("本地偏好字段必须是对象。");
  }
  const allowed = new Set(["watchlistSort", "watchlistSortDirection", "watchlistDisplayFields", "watchlistDisplayOrder", "marketColorScheme", "priceDigits", "percentDigits", "amountDisplayUnit", "dailyBarLimit", "dailyAdjustment"]);
  if (Object.keys(value).some((key) => !allowed.has(key))) {
    throw new Error("本地偏好包含不支持的字段。");
  }
  if (value.watchlistSort !== undefined && !["updated", "symbol", "name"].includes(value.watchlistSort)) {
    throw new Error("自选股默认排序字段无效。");
  }
  if (value.watchlistSortDirection !== undefined && !["asc", "desc"].includes(value.watchlistSortDirection)) {
    throw new Error("自选股排序方向无效。");
  }
  if (value.watchlistDisplayFields !== undefined && (!Array.isArray(value.watchlistDisplayFields)
    || value.watchlistDisplayFields.some((field) => !WATCHLIST_DISPLAY_FIELDS.includes(field)))) {
    throw new Error("自选股展示字段配置无效。");
  }
  if (value.watchlistDisplayOrder !== undefined && (!Array.isArray(value.watchlistDisplayOrder)
    || value.watchlistDisplayOrder.length !== WATCHLIST_DISPLAY_FIELDS.length
    || new Set(value.watchlistDisplayOrder).size !== WATCHLIST_DISPLAY_FIELDS.length
    || value.watchlistDisplayOrder.some((field) => !WATCHLIST_DISPLAY_FIELDS.includes(field)))) {
    throw new Error("自选股字段顺序配置无效。");
  }
  if (value.marketColorScheme !== undefined && !["cn", "reverse"].includes(value.marketColorScheme)) throw new Error("涨跌颜色方案无效。");
  if (value.priceDigits !== undefined && ![2, 3].includes(value.priceDigits)) throw new Error("价格精度无效。");
  if (value.percentDigits !== undefined && ![2, 3].includes(value.percentDigits)) throw new Error("涨跌幅精度无效。");
  if (value.amountDisplayUnit !== undefined && !["compact", "raw"].includes(value.amountDisplayUnit)) throw new Error("成交额单位显示无效。");
  if (value.dailyBarLimit !== undefined && ![60, 120, 250].includes(Number(value.dailyBarLimit))) throw new Error("默认日 K 条数无效。");
  if (value.dailyAdjustment !== undefined && !["forward_adjusted", "backward_adjusted", "none"].includes(value.dailyAdjustment)) throw new Error("默认复权方式无效。");
  return {
    watchlistSort: value.watchlistSort || DEFAULT_PREFERENCES.watchlistSort,
    watchlistSortDirection: value.watchlistSortDirection || DEFAULT_PREFERENCES.watchlistSortDirection,
    watchlistDisplayFields: value.watchlistDisplayFields === undefined
      ? [...DEFAULT_PREFERENCES.watchlistDisplayFields]
      : WATCHLIST_DISPLAY_FIELDS.filter((field) => value.watchlistDisplayFields.includes(field)),
    watchlistDisplayOrder: Array.isArray(value.watchlistDisplayOrder) ? [...value.watchlistDisplayOrder] : [...DEFAULT_PREFERENCES.watchlistDisplayOrder],
    marketColorScheme: value.marketColorScheme || DEFAULT_PREFERENCES.marketColorScheme,
    priceDigits: value.priceDigits || DEFAULT_PREFERENCES.priceDigits,
    percentDigits: value.percentDigits || DEFAULT_PREFERENCES.percentDigits,
    amountDisplayUnit: value.amountDisplayUnit || DEFAULT_PREFERENCES.amountDisplayUnit,
    dailyBarLimit: [60, 120, 250].includes(Number(value.dailyBarLimit)) ? Number(value.dailyBarLimit) : DEFAULT_PREFERENCES.dailyBarLimit,
    dailyAdjustment: value.dailyAdjustment || DEFAULT_PREFERENCES.dailyAdjustment
  };
}


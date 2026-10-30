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

export function readPreferences(storage) {
  const target = resolveStorage(storage);
  try {
    const raw = target?.getItem(PREFERENCES_KEY);
    return normalizePreferences(raw ? JSON.parse(raw) : {});
  } catch {
    return { ...DEFAULT_PREFERENCES };
  }
}

export function savePreferences(value, storage) {
  const preferences = normalizePreferences(value);
  const target = resolveStorage(storage);
  if (!target) throw new Error("浏览器本地存储不可用。");
  const serialized = JSON.stringify(preferences);
  target.setItem(PREFERENCES_KEY, serialized);
  if (target.getItem(PREFERENCES_KEY) !== serialized) {
    throw new Error("偏好设置未能保存，请检查浏览器存储权限或可用空间。");
  }
  return preferences;
}

export function resetPreferences(storage) {
  return savePreferences(DEFAULT_PREFERENCES, storage);
}

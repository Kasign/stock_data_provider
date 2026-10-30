export const PREFERENCES_KEY = "stock-provider-webui.preferences.v1";

export const WATCHLIST_DISPLAY_FIELDS = Object.freeze(["updated", "quote", "today", "task", "result"]);

export const DEFAULT_PREFERENCES = Object.freeze({
  watchlistSort: "updated",
  watchlistSortDirection: "desc",
  watchlistDisplayFields: [...WATCHLIST_DISPLAY_FIELDS]
});

const SORT_MODES = new Set(["updated", "symbol", "name"]);
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
      : [...DEFAULT_PREFERENCES.watchlistDisplayFields]
  };
}

export function readPreferences(storage = globalThis.localStorage) {
  try {
    const raw = storage?.getItem(PREFERENCES_KEY);
    return normalizePreferences(raw ? JSON.parse(raw) : {});
  } catch {
    return { ...DEFAULT_PREFERENCES };
  }
}

export function savePreferences(value, storage = globalThis.localStorage) {
  const preferences = normalizePreferences(value);
  if (!storage) throw new Error("浏览器本地存储不可用。");
  const serialized = JSON.stringify(preferences);
  storage.setItem(PREFERENCES_KEY, serialized);
  if (storage.getItem(PREFERENCES_KEY) !== serialized) {
    throw new Error("偏好设置未能保存，请检查浏览器存储权限或可用空间。");
  }
  return preferences;
}

export function resetPreferences(storage = globalThis.localStorage) {
  return savePreferences(DEFAULT_PREFERENCES, storage);
}

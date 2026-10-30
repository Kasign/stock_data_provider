export const THEME_STORAGE_KEY = "stock-provider-webui.theme.v1";
export const DEFAULT_THEME = "dark";

function resolveStorage(storage) {
  if (storage !== undefined) return storage;
  try {
    return globalThis.localStorage;
  } catch {
    return null;
  }
}

export function normalizeTheme(value) {
  return value === "light" || value === "dark" ? value : DEFAULT_THEME;
}

export function readTheme(storage) {
  const target = resolveStorage(storage);
  if (!target) return DEFAULT_THEME;
  try {
    return normalizeTheme(target.getItem(THEME_STORAGE_KEY));
  } catch {
    return DEFAULT_THEME;
  }
}

export function persistTheme(value, storage) {
  const theme = normalizeTheme(value);
  const target = resolveStorage(storage);
  if (!target) return { theme, persisted: false };

  try {
    target.setItem(THEME_STORAGE_KEY, theme);
    return { theme, persisted: target.getItem(THEME_STORAGE_KEY) === theme };
  } catch {
    // The theme still applies for this page session when storage is unavailable.
    return { theme, persisted: false };
  }
}

export function applyTheme(value, root) {
  const theme = normalizeTheme(value);
  const target = root ?? (typeof document !== "undefined" ? document.documentElement : null);
  if (!target?.dataset) return theme;
  target.dataset.theme = theme;
  target.style?.setProperty("color-scheme", theme);
  return theme;
}

if (typeof document !== "undefined") {
  applyTheme(readTheme(), document.documentElement);
}

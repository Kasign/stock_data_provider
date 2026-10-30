export const WATCHLIST_GROUPS_KEY = "stock-provider-webui.watchlist-groups.v1";
export const DEFAULT_WATCHLIST_GROUP = "未分组";
export const MAX_WATCHLIST_GROUPS = 12;
function cleanName(value) {
  return [...String(value ?? "")].filter((char) => char.charCodeAt(0) >= 32 && char.charCodeAt(0) !== 127).join("").trim().slice(0, 20).trim();
}
export function readWatchlistGroups(storage) {
  try {
    const target = storage ?? globalThis.localStorage;
    const raw = target.getItem(WATCHLIST_GROUPS_KEY);
    if (raw == null) return { groups: [DEFAULT_WATCHLIST_GROUP], assignments: {}, valid: true, readable: true };
    const data = JSON.parse(raw);
    if (!data || !Array.isArray(data.groups) || !data.assignments || typeof data.assignments !== "object" || Array.isArray(data.assignments)) throw new Error("invalid");
    const groups = data.groups.map(cleanName);
    if (!groups.includes(DEFAULT_WATCHLIST_GROUP) || groups.length > MAX_WATCHLIST_GROUPS || groups.some((name) => !name) || new Set(groups).size !== groups.length) throw new Error("invalid");
    const assignments = {};
    for (const [symbol, group] of Object.entries(data.assignments)) {
      if (!/^[0-9]{6}(\.(SH|SZ|BJ))?$/.test(symbol) || !groups.includes(group)) throw new Error("invalid");
      assignments[symbol] = group;
    }
    return { groups, assignments, valid: true, readable: true };
  } catch (error) { return { groups: [DEFAULT_WATCHLIST_GROUP], assignments: {}, valid: false, readable: error?.message === "invalid" }; }
}
function save(state, storage) {
  try { const target = storage ?? globalThis.localStorage; const value = JSON.stringify({ groups: state.groups, assignments: state.assignments }); target.setItem(WATCHLIST_GROUPS_KEY, value); return target.getItem(WATCHLIST_GROUPS_KEY) === value; }
  catch { return false; }
}
export function createWatchlistGroup(name, storage) {
  const state = readWatchlistGroups(storage), normalized = cleanName(name);
  if (!state.valid) return { ...state, changed: false, reason: "invalid-storage" };
  if (!normalized || normalized === DEFAULT_WATCHLIST_GROUP) return { ...state, changed: false, reason: "invalid-name" };
  if (state.groups.includes(normalized)) return { ...state, changed: false, reason: "duplicate" };
  if (state.groups.length >= MAX_WATCHLIST_GROUPS) return { ...state, changed: false, reason: "limit" };
  const next = { groups: [...state.groups, normalized], assignments: state.assignments };
  return { ...next, valid: true, readable: true, changed: true, persisted: save(next, storage) };
}
export function assignWatchlistGroup(symbol, group, storage) {
  const state = readWatchlistGroups(storage), code = String(symbol ?? "").trim().toUpperCase();
  if (!state.valid || !/^[0-9]{6}(\.(SH|SZ|BJ))?$/.test(code) || !state.groups.includes(group)) return { ...state, changed: false, persisted: false };
  const assignments = { ...state.assignments };
  if (group === DEFAULT_WATCHLIST_GROUP) delete assignments[code]; else assignments[code] = group;
  const next = { groups: state.groups, assignments };
  return { ...next, valid: true, readable: true, changed: true, persisted: save(next, storage) };
}
export function removeWatchlistGroup(name, storage) {
  const state = readWatchlistGroups(storage);
  if (!state.valid || !state.groups.includes(name) || name === DEFAULT_WATCHLIST_GROUP) return { ...state, changed: false, persisted: false };
  const assignments = Object.fromEntries(Object.entries(state.assignments).filter(([, group]) => group !== name));
  const next = { groups: state.groups.filter((group) => group !== name), assignments };
  return { ...next, valid: true, readable: true, changed: true, persisted: save(next, storage) };
}
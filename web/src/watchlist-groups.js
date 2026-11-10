import { comparableMarketSymbol } from "./format.js";

export const WATCHLIST_GROUPS_KEY = "stock-provider-webui.watchlist-groups.v1";
export const DEFAULT_WATCHLIST_GROUP = "未分组";
export const MAX_WATCHLIST_GROUPS = 12;

function cleanName(value) {
  return [...String(value ?? "")]
    .filter((char) => char.charCodeAt(0) >= 32 && char.charCodeAt(0) !== 127)
    .join("")
    .trim()
    .slice(0, 20)
    .trim();
}

export function normalizeWatchlistGroups(value) {
  if (value === undefined || value === null) return { groups: [DEFAULT_WATCHLIST_GROUP], assignments: {} };
  if (!value || typeof value !== "object" || Array.isArray(value) ||
      !Array.isArray(value.groups) || !value.assignments ||
      typeof value.assignments !== "object" || Array.isArray(value.assignments)) throw new Error("invalid");
  const groups = value.groups.map((name) => typeof name === "string" ? cleanName(name) : "");
  if (!groups.includes(DEFAULT_WATCHLIST_GROUP) || groups.length > MAX_WATCHLIST_GROUPS ||
      groups.some((name) => !name) || new Set(groups).size !== groups.length) throw new Error("invalid");
  const assignments = {};
  for (const [symbol, group] of Object.entries(value.assignments)) {
    if (!/^[0-9]{6}(\.(SH|SZ|BJ))?$/.test(symbol) ||
        typeof group !== "string" || !groups.includes(group)) throw new Error("invalid");
    const identity = comparableMarketSymbol(symbol);
    if (Object.hasOwn(assignments, identity) && assignments[identity] !== group) throw new Error("invalid");
    assignments[identity] = group;
  }
  return { groups, assignments };
}

export function readWatchlistGroups(storage) {
  let raw;
  try { raw = (storage ?? globalThis.localStorage).getItem(WATCHLIST_GROUPS_KEY); }
  catch { return { groups: [DEFAULT_WATCHLIST_GROUP], assignments: {}, valid: false, readable: false }; }
  if (raw == null) return { groups: [DEFAULT_WATCHLIST_GROUP], assignments: {}, valid: true, readable: true };
  try { return { ...normalizeWatchlistGroups(JSON.parse(raw)), valid: true, readable: true }; }
  catch { return { groups: [DEFAULT_WATCHLIST_GROUP], assignments: {}, valid: false, readable: true }; }
}

function save(state, storage) {
  try {
    const target = storage ?? globalThis.localStorage;
    const value = JSON.stringify({ groups: state.groups, assignments: state.assignments });
    target.setItem(WATCHLIST_GROUPS_KEY, value);
    return target.getItem(WATCHLIST_GROUPS_KEY) === value;
  } catch { return false; }
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
  const identity = comparableMarketSymbol(code), assignments = { ...state.assignments };
  if (group === DEFAULT_WATCHLIST_GROUP) delete assignments[identity]; else assignments[identity] = group;
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

export function assignWatchlistGroups(symbols, group, storage) {
  const state = readWatchlistGroups(storage);
  if (!state.valid || !state.groups.includes(group)) return { ...state, changed: false, persisted: false, reason: "invalid-state-or-group" };
  const identities = [...new Set((Array.isArray(symbols) ? symbols : [])
    .map((symbol) => String(symbol ?? "").trim().toUpperCase())
    .filter((symbol) => /^[0-9]{6}(\.(SH|SZ|BJ))?$/.test(symbol))
    .map(comparableMarketSymbol))];
  if (!identities.length) return { ...state, changed: false, persisted: false, reason: "empty-selection" };
  const assignments = { ...state.assignments };
  identities.forEach((identity) => { if (group === DEFAULT_WATCHLIST_GROUP) delete assignments[identity]; else assignments[identity] = group; });
  const next = { groups: state.groups, assignments };
  return { ...next, valid: true, readable: true, changed: true, persisted: save(next, storage), assignedCount: identities.length };
}

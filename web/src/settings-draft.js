import { normalizeBasePath } from "./format.js";

const DRAFT_FIELDS = Object.freeze(["marketBase", "analysisBase", "apiKey"]);

export function connectionDraftChanged(initial, current) {
  if (!initial || !current) return false;
  return DRAFT_FIELDS.some((field) => String(initial[field] ?? "") !== String(current[field] ?? ""));
}

export function mergeConnectionDraft(draft, existingSettings = {}) {
  return {
    marketBase: normalizeBasePath(draft?.marketBase, "/market-api"),
    analysisBase: normalizeBasePath(draft?.analysisBase, "/analysis-api"),
    apiKey: String(draft?.apiKey ?? "").trim() || String(existingSettings.apiKey ?? "").trim()
  };
}

const DRAFT_FIELDS = Object.freeze(["marketBase", "analysisBase"]);

export function connectionDraftChanged(initial, current) {
  if (!initial || !current) return false;
  return DRAFT_FIELDS.some((field) => String(initial[field] ?? "") !== String(current[field] ?? ""));
}

export function mergeConnectionDraft() {
  return { marketBase: "/", analysisBase: "/" };
}

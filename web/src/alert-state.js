const EVALUATION_STATUSES = new Set([
  "triggered", "not-triggered", "evaluation-error", "degraded", "skipped"
]);

export function resolveAlertCapability(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (snapshot.enabled === true) return "enabled";
  if (snapshot.enabled === false) return "disabled";
  return "unknown";
}

export function resolveAlertEvaluationState(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (!snapshot.result) return "idle";
  const status = String(snapshot.result.status || "").toLowerCase();
  return EVALUATION_STATUSES.has(status) ? status : "unknown";
}

export function resolveAlertCollectionState(snapshot = {}, field = "rules") {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (!Array.isArray(snapshot[field])) return "error";
  return snapshot[field].length ? "loaded" : "empty";
}

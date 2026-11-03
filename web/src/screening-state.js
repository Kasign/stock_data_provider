const RUN_STATUSES = new Set(["queued", "running", "succeeded", "partial", "failed"]);

export function resolveScreeningCapability(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (snapshot.enabled === true) return "enabled";
  if (snapshot.enabled === false) return "disabled";
  return "unknown";
}

export function resolveScreeningTaskState(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (!snapshot.task) return "idle";
  const status = String(snapshot.task.status || "").toLowerCase();
  if (RUN_STATUSES.has(status)) return status === "succeeded" ? "completed" : status;
  return "unknown";
}

export function resolveScreeningCollectionState(snapshot = {}, field = "candidates") {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (!Array.isArray(snapshot[field])) return "error";
  return snapshot[field].length ? "loaded" : "empty";
}

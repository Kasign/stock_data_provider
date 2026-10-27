const FILTER_STATUSES = Object.freeze({
  issues: new Set(["failed", "error", "blocked", "rejected", "degraded", "partial"]),
  active: new Set(["running", "started", "in-progress", "in_progress", "processing", "queued", "pending"]),
  completed: new Set(["succeeded", "success", "completed", "skipped"])
});

export function taskEventMatchesFilter(statusKey, filter = "all") {
  const accepted = FILTER_STATUSES[filter];
  return !accepted || accepted.has(statusKey);
}

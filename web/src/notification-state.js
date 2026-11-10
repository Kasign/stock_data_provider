export function resolveNotificationViewState(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (!Array.isArray(snapshot.items)) return "error";
  return snapshot.items.length ? "loaded" : "empty";
}

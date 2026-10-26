export function resolveChatViewState(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (!snapshot.session) return "empty";
  if (!Array.isArray(snapshot.messages)) return "error";
  return snapshot.messages.length ? "loaded" : "empty";
}

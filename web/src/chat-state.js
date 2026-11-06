export function resolveChatViewState(snapshot = {}) {
  if (snapshot.supported !== true) return "unavailable";
  if (snapshot.loading === true) return "loading";
  if (snapshot.error) return "error";
  if (snapshot.session === null || snapshot.session === undefined) return "empty";
  if (typeof snapshot.session !== "object" || Array.isArray(snapshot.session)) return "error";
  if (!Array.isArray(snapshot.messages)) return "error";
  return snapshot.messages.length ? "loaded" : "empty";
}

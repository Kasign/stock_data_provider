import test from "node:test";
import assert from "node:assert/strict";
import { connectionDraftChanged, mergeConnectionDraft } from "../src/settings-draft.js";

test("connection draft is fixed to the single Root Server API base", () => {
  const initial = { marketBase: "/", analysisBase: "/" };
  assert.equal(connectionDraftChanged(initial, { ...initial }), false);
  assert.equal(connectionDraftChanged(initial, { ...initial, marketBase: "/other" }), true);
  assert.equal(connectionDraftChanged(initial, { ...initial, analysisBase: "/other" }), true);
  assert.equal(connectionDraftChanged(null, initial), false);
});

test("mergeConnectionDraft ignores legacy split paths and browser API keys", () => {
  assert.deepEqual(
    mergeConnectionDraft({
      marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "legacy-secret"
    }, { marketBase: "/other", analysisBase: "/other", apiKey: "old-secret" }),
    { marketBase: "/", analysisBase: "/" }
  );
});

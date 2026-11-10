import test from "node:test";
import assert from "node:assert/strict";
import { connectionDraftChanged, mergeConnectionDraft } from "../src/settings-draft.js";

test("connection draft detects changed paths and replacement keys", () => {
  const initial = { marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "" };
  assert.equal(connectionDraftChanged(initial, { ...initial }), false);
  assert.equal(connectionDraftChanged(initial, { ...initial, marketBase: "/market-v2" }), true);
  assert.equal(connectionDraftChanged(initial, { ...initial, analysisBase: "/analysis-v2" }), true);
  assert.equal(connectionDraftChanged(initial, { ...initial, apiKey: "replacement-test-value" }), true);
  assert.equal(connectionDraftChanged(null, initial), false);
});

test("blank key draft retains the current browser key; a replacement is trimmed", () => {
  const existing = { marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "existing-test-value" };
  assert.deepEqual(
    mergeConnectionDraft({ marketBase: "/market-api/", analysisBase: "/analysis-api", apiKey: "" }, existing),
    { marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "existing-test-value" }
  );
  assert.equal(
    mergeConnectionDraft({ marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "  replacement-test-value  " }, existing).apiKey,
    "replacement-test-value"
  );
});

test("connection drafts reject non-same-origin paths before persistence", () => {
  const base = { marketBase: "/market-api", analysisBase: "/analysis-api", apiKey: "" };
  assert.throws(() => mergeConnectionDraft({ ...base, marketBase: "https://example.com" }, {}), /同源/);
  assert.throws(() => mergeConnectionDraft({ ...base, analysisBase: "//other-host" }, {}), /同源/);
  assert.throws(() => mergeConnectionDraft({ ...base, marketBase: "/market-api?token=x" }, {}), /查询参数/);
});

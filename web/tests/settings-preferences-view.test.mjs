import test from "node:test";
import assert from "node:assert/strict";
import { syncPreferencesControls } from "../src/settings-preferences-view.js";

test("preference control adapter safely skips unavailable documents", () => {
  assert.doesNotThrow(() => syncPreferencesControls(null));
  assert.doesNotThrow(() => syncPreferencesControls({}));
});

import test from "node:test";
import assert from "node:assert/strict";
import { setupSettingsNavigation } from "../src/settings-navigation.js";

test("missing settings navigation safely skips enhancement", () => {
  let queries = 0;
  setupSettingsNavigation({
    querySelector() {
      queries += 1;
      return null;
    }
  });
  assert.equal(queries, 1);
});

test("settings navigation enhancement is idempotent", () => {
  let queries = 0;
  const nav = { dataset: { enhanced: "true" } };
  setupSettingsNavigation({
    querySelector() {
      queries += 1;
      return nav;
    }
  });
  assert.equal(queries, 1);
  assert.equal(nav.dataset.enhanced, "true");
});

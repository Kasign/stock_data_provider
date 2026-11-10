import test from "node:test";
import assert from "node:assert/strict";
import { syncPreferencesControls } from "../src/settings-preferences-view.js";

test("preference control adapter safely skips unavailable documents", () => {
  assert.doesNotThrow(() => syncPreferencesControls(null));
  assert.doesNotThrow(() => syncPreferencesControls({}));
});

test("market color preference updates the root scheme used by CSS semantic tokens", () => {
  const control = { value: "" };
  const root = { dataset: {} };
  const document = {
    documentElement: root,
    querySelector(selector) {
      return selector === "#settings-market-color-scheme" ? control : null;
    },
    querySelectorAll() {
      return [];
    }
  };

  syncPreferencesControls(document, {
    watchlistSort: "updated",
    watchlistSortDirection: "desc",
    watchlistDisplayFields: [],
    watchlistDisplayOrder: [],
    marketColorScheme: "reverse",
    priceDigits: 2,
    percentDigits: 2,
    amountDisplayUnit: "compact",
    dailyBarLimit: 120,
    dailyAdjustment: "forward_adjusted"
  });

  assert.equal(control.value, "reverse");
  assert.equal(root.dataset.marketColorScheme, "reverse");
});

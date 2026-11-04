import { DEFAULT_PREFERENCES } from "./preferences.js";

export function syncPreferencesControls(document, preferences = DEFAULT_PREFERENCES) {
  if (!document?.querySelector) return;
  const normalized = preferences || DEFAULT_PREFERENCES;
  const normalized = preferences;
  const sort = $("#settings-watchlist-sort");
  const direction = $("#settings-watchlist-direction");
  const listSort = $("#watchlist-sort");
  if (sort) sort.value = normalized.watchlistSort;
  if (direction) direction.value = normalized.watchlistSortDirection;
  if (listSort) listSort.value = normalized.watchlistSort;
  document.querySelectorAll("[data-watchlist-display-field]").forEach((input) => {
    input.checked = normalized.watchlistDisplayFields.includes(input.value);
  });
  const orderRoot = $("#watchlist-field-order-list");
  if (orderRoot) {
    (normalized.watchlistDisplayOrder || DEFAULT_PREFERENCES.watchlistDisplayOrder).forEach((field) => {
      const item = orderRoot.querySelector(`[data-watchlist-field-order="${field}"]`);
      if (item) orderRoot.append(item);
    });
    orderRoot.querySelectorAll("[data-watchlist-field-order-move]").forEach((button) => {
      const items = [...orderRoot.children];
      const index = items.indexOf(button.closest("[data-watchlist-field-order]"));
      button.disabled = button.dataset.watchlistFieldOrderMove === "up" ? index <= 0 : index >= items.length - 1;
    });
  }
  const controls = {
    "#settings-market-color-scheme": normalized.marketColorScheme,
    "#settings-price-digits": String(normalized.priceDigits),
    "#settings-percent-digits": String(normalized.percentDigits),
    "#settings-amount-unit": normalized.amountDisplayUnit
  };
  Object.entries(controls).forEach(([selector, value]) => {
    const control = $(selector);
    if (control) control.value = value;
  });
  document.documentElement.dataset.marketColorScheme = normalized.marketColorScheme;
  const barLimit = $("#settings-default-bar-limit");
  const adjustment = $("#settings-default-adjustment");
  if (barLimit) barLimit.value = String(normalized.dailyBarLimit);
  if (adjustment) adjustment.value = normalized.dailyAdjustment;
  const queryLimit = $("#bar-limit");
  const queryAdjustment = $("#adjustment");
  if (queryLimit) queryLimit.value = String(normalized.dailyBarLimit);
  if (queryAdjustment) queryAdjustment.value = normalized.dailyAdjustment;

}

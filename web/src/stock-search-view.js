export function normalizeStockSearchItems(items) {
  return Array.isArray(items) ? items.filter((item) =>
    item && typeof item.symbol === "string" && typeof item.code === "string"
    && typeof item.name === "string" && typeof item.exchange === "string"
  ) : [];
}

export function renderStockSearchResultsView({ document, root, items, generation }) {
  root.replaceChildren();
  items.forEach((item, index) => {
    const option = document.createElement("button");
    option.type = "button";
    option.className = "stock-search-option";
    option.id = "stock-search-option-" + generation + "-" + index;
    option.dataset.stockSearchIndex = String(index);
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", "false");

    const main = document.createElement("span");
    main.className = "stock-search-option-main";
    const name = document.createElement("span");
    name.className = "stock-search-option-name";
    name.textContent = item.name;
    const symbol = document.createElement("span");
    symbol.className = "stock-search-option-symbol";
    symbol.textContent = item.symbol;
    main.append(name, symbol);

    const exchange = document.createElement("span");
    exchange.className = "stock-search-option-exchange";
    exchange.textContent = item.exchange;
    option.append(main, exchange);
    root.append(option);
  });
}

export function updateStockSearchActiveOptionView({ input, root, activeIndex }) {
  if (!input || !root) return;
  const options = root.querySelectorAll("[data-stock-search-index]");
  options.forEach((option, index) => {
    option.setAttribute("aria-selected", String(index === activeIndex));
  });
  const active = root.querySelector('[aria-selected="true"]');
  if (active) {
    input.setAttribute("aria-activedescendant", active.id);
    active.scrollIntoView?.({ block: "nearest" });
  } else {
    input.removeAttribute("aria-activedescendant");
  }
}

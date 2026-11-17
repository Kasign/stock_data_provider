export function buildWatchlistItemNode({
  document,
  record,
  trackingFields = [],
  groups = [],
  selectedGroup = "未分组"
}) {
  const row = document.createElement("article");
  row.className = "watchlist-item";

  const main = document.createElement("div");
  main.className = "watchlist-item-main";

  const selectionLabel = document.createElement("label");
  selectionLabel.className = "watchlist-select-label";
  const selection = document.createElement("input");
  selection.type = "checkbox";
  selection.value = record.symbol;
  selection.dataset.watchlistSelectSymbol = "true";
  selection.setAttribute("aria-label", "选择 " + record.symbol);
  selectionLabel.append(selection);
  main.append(selectionLabel);

  const symbol = document.createElement("strong");
  symbol.className = "watchlist-symbol";
  symbol.textContent = record.symbol;

  const aliasLabel = document.createElement("label");
  aliasLabel.className = "watchlist-name-field";
  const aliasCaption = document.createElement("span");
  aliasCaption.textContent = "自定义名称";
  const aliasInput = document.createElement("input");
  aliasInput.type = "text";
  aliasInput.className = "watchlist-name-input";
  aliasInput.maxLength = 32;
  aliasInput.value = record.name === record.symbol ? "" : record.name;
  aliasInput.placeholder = "可选，最多 32 字";
  aliasInput.dataset.watchlistNameSymbol = record.symbol;
  aliasInput.setAttribute("aria-label", record.symbol + " 的自定义名称");
  aliasLabel.append(aliasCaption, aliasInput);
  main.append(symbol, aliasLabel);

  if (trackingFields.length) {
    const tracking = document.createElement("div");
    tracking.className = "watchlist-tracking-summary";
    trackingFields.forEach((field) => {
      const line = document.createElement("small");
      line.className = field.className || "watchlist-tracking-line";
      if (field.title) line.title = field.title;
      line.textContent = field.text == null ? "" : String(field.text);
      tracking.append(line);
    });
    main.append(tracking);
  }

  const actions = document.createElement("div");
  actions.className = "watchlist-actions";
  for (const [action, label] of [["query", "查行情"], ["research", "研究"], ["analyze", "分析"], ["remove", "移除"]]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "watchlist-action" + (action === "remove" ? " watchlist-action-muted" : "");
    button.dataset.watchlistAction = action;
    button.dataset.symbol = record.symbol;
    button.dataset.name = record.name || record.symbol;
    button.textContent = label;
    if (action === "remove") button.setAttribute("aria-label", "移除自选 " + record.symbol);
    if (action === "research") button.setAttribute("aria-label", "研究自选股 " + record.symbol);
    actions.append(button);
  }

  const groupSelect = document.createElement("select");
  groupSelect.className = "watchlist-item-group";
  groupSelect.dataset.watchlistGroupSymbol = record.symbol;
  groupSelect.setAttribute("aria-label", record.symbol + " 所属分组");
  groups.forEach((group) => {
    const option = document.createElement("option");
    option.value = group;
    option.textContent = group;
    groupSelect.append(option);
  });
  groupSelect.value = selectedGroup;
  actions.append(groupSelect);

  row.append(main, actions);
  return row;
}

export function buildWatchlistImportPreviewView({ document, preview }) {
  if (preview.error) {
    return {
      nodes: [document.createTextNode(preview.error)],
      summary: preview.error,
      confirmDisabled: true
    };
  }

  const counts = preview.counts;
  const summary = "可导入 " + counts.ready + " 只 · 重复 " + counts.duplicate
    + " 只 · 无效 " + counts.invalid + " 条 · 超出上限 " + counts.limit + " 只";
  const table = document.createElement("table");
  const head = document.createElement("thead");
  const headerRow = document.createElement("tr");
  ["行", "代码", "名称", "结果"].forEach((label) => {
    const cell = document.createElement("th");
    cell.textContent = label;
    headerRow.append(cell);
  });
  head.append(headerRow);
  table.append(head);

  const body = document.createElement("tbody");
  preview.rows.forEach((item) => {
    const row = document.createElement("tr");
    [item.line, item.symbol, item.name || "—", item.message].forEach((value) => {
      const cell = document.createElement("td");
      cell.textContent = String(value);
      row.append(cell);
    });
    body.append(row);
  });
  table.append(body);

  return {
    nodes: [table],
    summary,
    confirmDisabled: counts.ready === 0
  };
}

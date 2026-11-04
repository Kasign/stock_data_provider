export function renderScreeningLocalResultRowsView({
  document, body, rows, filter, snapshot,
  formatPercent, formatCompact, formatPrice, finiteNumberOrNull, qualityLabel
}) {
  body.replaceChildren();
  if (!rows.length) {
    const row = document.createElement("tr"), cell = document.createElement("td");
    cell.className = "table-empty"; cell.colSpan = 9;
    cell.textContent = filter === "matched" ? "当前页没有可确认命中的股票；请检查待判断数量。" :
      filter === "unknown" ? "当前页没有因字段缺失而待判断的股票。" :
      filter === "unmatched" ? "当前页没有明确未命中的股票。" : "当前页没有可展示的筛选记录。";
    row.append(cell); body.append(row);
    return { rowCount: 0 };
  }
  rows.forEach(({ item, status }) => {
    const quote = item?.quote || {}, symbol = String(item?.symbol || item?.code || quote.symbol || "").trim();
    const name = String(item?.name || quote.name || symbol || "未报告");
    const row = document.createElement("tr"), checkCell = document.createElement("td"), stockCell = document.createElement("td");
    const validSymbol = /^\d{6}(?:\.(?:SH|SZ|BJ))?$/.test(symbol.toUpperCase());
    const checkbox = document.createElement("input"); checkbox.type = "checkbox"; checkbox.value = symbol.toUpperCase();
    checkbox.dataset.screeningLocalSelectSymbol = symbol.toUpperCase(); checkbox.disabled = !validSymbol;
    checkbox.setAttribute("aria-label", "选择 " + name + " 加入自选股"); checkCell.append(checkbox); row.append(checkCell);
    const select = document.createElement("button"); select.type = "button"; select.className = "market-hot-stock-link";
    select.dataset.screeningLocalSymbol = symbol; select.dataset.screeningLocalName = name;
    select.disabled = !symbol; select.append(document.createTextNode(name), document.createElement("br"));
    const code = document.createElement("small"); code.textContent = symbol || "代码未报告"; select.append(code); stockCell.append(select); row.append(stockCell);
    [
      formatPercent(quote.change_percent), formatPercent(quote.turnover_rate),
      finiteNumberOrNull(quote.amount) === null || quote.amount_unit !== "CNY" ? "—" : formatCompact(quote.amount) + " 元",
      finiteNumberOrNull(item.ma5) === null ? "—" : formatPrice(item.ma5),
      finiteNumberOrNull(item.ma20) === null ? "—" : formatPrice(item.ma20)
    ].forEach(value => { const cell = document.createElement("td"); cell.textContent = value; row.append(cell); });
    const qualityCell = document.createElement("td"), quality = item.quality_status || quote.quality_status || "unknown";
    qualityCell.textContent = qualityLabel(quality);
    const reasons = [...(Array.isArray(item.quality_reasons) ? item.quality_reasons : []), ...(Array.isArray(quote.quality_reasons) ? quote.quality_reasons : [])].filter(Boolean);
    qualityCell.title = reasons.join("；") || "服务端未提供详细质量原因";
    const reason = document.createElement("small"); reason.className = "screening-quality-reason";
    reason.textContent = reasons.length ? reasons.join("；").slice(0, 100) : "质量原因未报告";
    qualityCell.append(document.createElement("br"), reason); row.append(qualityCell);
    const judgmentCell = document.createElement("td"); judgmentCell.textContent = status; row.append(judgmentCell);
    body.append(row);
  });
  return { rowCount: rows.length, page: snapshot.page };
}

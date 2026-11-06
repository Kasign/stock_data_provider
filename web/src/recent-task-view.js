export function renderRecentTaskList({
  root,
  countElement,
  historyState,
  records,
  totalCount = Array.isArray(records) ? records.length : 0,
  analysisBase = "",
  formatDateTime,
  taskStatusLabel,
  recentTaskTypeLabel
}) {
  root.replaceChildren();
  const document = root.ownerDocument;
  if (!historyState.storageReadable || !historyState.storageValid) {
    countElement.textContent = "不可用 / 10";
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = !historyState.storageReadable
      ? "无法读取浏览器最近任务存储。为避免误报为空，当前历史暂不可用；请检查浏览器存储权限后重新载入。"
      : "本地最近任务数据格式异常。为避免覆盖原始记录，历史修改已暂停；请先检查或备份浏览器数据。";
    root.append(empty);
    return {
      feedbackMessage: empty.textContent,
      feedbackKind: "error",
      clearFeedback: false,
      refreshResearchStockList: true
    };
  }

  const items = Array.isArray(records) ? records : [];
  countElement.textContent = items.length + " / " + totalCount + "（最多 10）";

  if (!totalCount || !items.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = !totalCount
      ? "完成一次分析后，这里会显示最近任务。"
      : "没有匹配项，请调整关键词或清除搜索。";
    root.append(empty);
    return { clearFeedback: true, refreshResearchStockList: true };
  }

  items.forEach((record) => {
    const item = document.createElement("article");
    item.className = "recent-analysis-item";

    const open = document.createElement("button");
    open.type = "button";
    open.className = "recent-analysis-open";
    open.dataset.openTaskId = record.task_id;
    const main = document.createElement("span");
    main.className = "recent-analysis-item-main";
    const title = document.createElement("strong");
    title.textContent = record.symbols?.join(", ") || "标的未记录";
    const recordAnalysisBase = String(record.analysis_base || analysisBase);
    const isOtherEndpoint = recordAnalysisBase !== analysisBase;
    open.dataset.openTaskBase = recordAnalysisBase;
    const detail = document.createElement("small");
    detail.textContent = recentTaskTypeLabel(record.task_type) + " · " + (record.updated_at ? formatDateTime(record.updated_at) : "时间未记录") + " · " + record.task_id + (isOtherEndpoint ? " · 旧分析 API 地址" : "");
    const failure = document.createElement("small");
    failure.className = "recent-analysis-failure";
    failure.textContent = [record.failure_code ? "错误代码：" + record.failure_code : "", record.error_message || ""].filter(Boolean).join(" · ").slice(0, 300);
    failure.hidden = !failure.textContent;
    if (failure.textContent) main.append(failure);
    const status = document.createElement("span");
    status.className = "recent-analysis-item-status";
    status.textContent = taskStatusLabel(record.status);
    main.append(title, detail);
    open.append(main, status);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "recent-analysis-remove";
    remove.dataset.removeTaskId = record.task_id;
    remove.dataset.removeTaskBase = recordAnalysisBase;
    remove.textContent = "移除";
    remove.setAttribute("aria-label", "从本地列表移除任务 " + record.task_id);

    item.append(open, remove);
    root.append(item);
  });

  return { clearFeedback: true, refreshResearchStockList: true };
}

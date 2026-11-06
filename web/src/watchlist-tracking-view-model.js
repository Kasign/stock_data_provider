function isOtherAnalysisBase(task, currentAnalysisBase) {
  return Boolean(task && String(task.analysis_base || currentAnalysisBase) !== String(currentAnalysisBase));
}

export function buildWatchlistTrackingFields(input) {
  const {
    record,
    batchQuote,
    currentQuote,
    quoteStatus,
    recentTasks = [],
    currentTask = {},
    runResult,
    runResultReadStatus = "",
    runResultReadError = "",
    currentAnalysisBase = "",
    displayOrder = [],
    displayFields = [],
    now = new Date(),
    formatters = {}
  } = input || {};
  const {
    normalizeSymbol = (value) => String(value || "").trim().toUpperCase(),
    dateKey = (value) => {
      const date = value instanceof Date ? value : new Date(String(value || ""));
      if (!Number.isFinite(date.getTime())) return "";
      return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, "0"), String(date.getDate()).padStart(2, "0")].join("-");
    },
    formatDateTime = (value) => String(value || ""),
    formatPrice = (value) => String(value ?? "—"),
    formatPercent = (value) => String(value ?? "—"),
    qualityLabel = (value) => String(value || "未知"),
    taskStatusLabel = (value) => String(value || "未知"),
    runStatusLabel = (value) => String(value || "未知")
  } = formatters;

  const itemSymbol = normalizeSymbol(record?.symbol);
  const quoteTitle = [
    batchQuote?.errorCode || "",
    ...(Array.isArray(batchQuote?.qualityReasons) ? batchQuote.qualityReasons : []),
    batchQuote?.fetchedAt ? "批量采集 " + formatDateTime(batchQuote.fetchedAt) : "",
    currentQuote?.quote_time ? "行情时间 " + formatDateTime(currentQuote.quote_time) : ""
  ].filter(Boolean).join("；") || "行情质量原因未报告";

  let quoteText = "";
  if (batchQuote?.errorCode) {
    quoteText = "批量行情不可用：" + batchQuote.errorCode + " · " + qualityLabel(batchQuote.qualityStatus);
  } else if (currentQuote) {
    const oldSnapshot = ["error", "loading"].includes(quoteStatus) && batchQuote;
    quoteText = (oldSnapshot ? "上次成功快照：" : "行情快照：")
      + formatPrice(currentQuote.current_price) + " · " + formatPercent(currentQuote.change_percent)
      + " · " + (currentQuote.quote_time ? formatDateTime(currentQuote.quote_time) : "时间未报告")
      + " · " + qualityLabel(batchQuote?.qualityStatus || currentQuote.quality_status)
      + (batchQuote?.fetchedAt ? " · 批量采集 " + formatDateTime(batchQuote.fetchedAt) : "");
  } else if (quoteStatus === "loading") {
    quoteText = "批量行情读取中…";
  } else if (quoteStatus === "loaded") {
    quoteText = "本次批量响应未包含该证券，质量未知。";
  } else {
    quoteText = "当前行情：未加载（可刷新批量行情或点击“查行情”读取）";
  }

  const matchingTasks = (Array.isArray(recentTasks) ? recentTasks : []).filter((task) =>
    Array.isArray(task.symbols) && task.symbols.some((value) => normalizeSymbol(value) === itemSymbol)
  );
  const recentTask = [...matchingTasks]
    .sort((a, b) => (Date.parse(b.updated_at || "") || 0) - (Date.parse(a.updated_at || "") || 0))[0] || null;
  const todayKey = dateKey(now);
  const todayTask = [...matchingTasks]
    .filter((task) => Boolean(todayKey) && dateKey(task.created_at) === todayKey)
    .sort((a, b) => (Date.parse(b.created_at || "") || 0) - (Date.parse(a.created_at || "") || 0))[0] || null;
  const todayTaskUsesOtherEndpoint = isOtherAnalysisBase(todayTask, currentAnalysisBase);
  const recentTaskUsesOtherEndpoint = isOtherAnalysisBase(recentTask, currentAnalysisBase);
  const activeTaskMatches = Boolean(currentTask.taskId) && Array.isArray(currentTask.symbols)
    && currentTask.symbols.some((value) => normalizeSymbol(value) === itemSymbol)
    && (currentTask.statusLoading || Boolean(currentTask.statusError)
      || ["running", "started", "pending", "queued", "processing", "in-progress", "in_progress"].includes(String(currentTask.status || "").toLowerCase()));

  let todayText = "";
  if (todayTask) {
    todayText = "今日分析：有本地记录" + (todayTaskUsesOtherEndpoint ? "（旧分析 API 地址）" : "") + " · " + taskStatusLabel(todayTask.status) + " · 创建于 " + formatDateTime(todayTask.created_at);
  } else if (!matchingTasks.length) {
    todayText = "今日分析：未知（当前浏览器没有该股票的任务引用）";
  } else if (matchingTasks.some((task) => !dateKey(task.created_at))) {
    todayText = "今日分析：未知（部分旧任务缺少服务端创建时间）";
  } else {
    todayText = "今日分析：本地最近任务未发现今日创建记录（不代表服务端无分析）";
  }

  const taskText = activeTaskMatches
    ? currentTask.statusLoading
      ? "当前任务：正在确认服务端状态（上次状态：" + taskStatusLabel(currentTask.status) + "） · " + currentTask.taskId
      : currentTask.statusError
        ? "当前任务：状态读取异常（上次状态：" + taskStatusLabel(currentTask.status) + (currentTask.statusErrorRetrying ? "，自动重试中" : "，自动刷新已暂停") + "） · " + currentTask.taskId
        : "当前任务：" + taskStatusLabel(currentTask.status) + " · " + currentTask.taskId
    : recentTask
      ? "最近任务：" + taskStatusLabel(recentTask.status) + (recentTaskUsesOtherEndpoint ? " · 旧分析 API 地址" : "") + " · " + (recentTask.updated_at ? formatDateTime(recentTask.updated_at) : "时间未记录")
      : "最近任务：暂无本地任务记录（不代表从未分析）";

  const latestResultMatches = Boolean(recentTask && runResult
    && currentTask.taskId === recentTask.task_id
    && recentTask.run_id
    && runResult.run_id === recentTask.run_id
    && String(recentTask.analysis_base || currentAnalysisBase) === String(currentTask.analysisBase || currentAnalysisBase)
    && Array.isArray(currentTask.symbols)
    && currentTask.symbols.some((value) => normalizeSymbol(value) === itemSymbol));
  const activeRunReadMatches = Boolean(recentTask
    && currentTask.taskId === recentTask.task_id
    && String(recentTask.analysis_base || currentAnalysisBase) === String(currentTask.analysisBase || currentAnalysisBase)
    && Array.isArray(currentTask.symbols)
    && currentTask.symbols.some((value) => normalizeSymbol(value) === itemSymbol)
    && ["loading", "retrying", "error"].includes(runResultReadStatus));

  let resultText = "";
  if (latestResultMatches) {
    const corePass = runResult.quality?.core_pass;
    const qualityState = corePass === true ? "核心质量门禁通过" : corePass === false ? "核心质量门禁未通过" : "核心质量门禁未报告";
    const warnings = Array.isArray(runResult.warnings) ? " · 警告 " + runResult.warnings.length + " 项" : "";
    const readLabel = runResultReadStatus === "loading"
      ? "本页保留旧结果（正在重读，尚未确认）："
      : runResultReadStatus === "retrying"
        ? "本页保留旧结果（重试中，尚未确认）："
        : runResultReadStatus === "error"
          ? "本页保留旧结果（本次读取失败）："
          : "本页已载入结果：";
    const readError = ["retrying", "error"].includes(runResultReadStatus) && runResultReadError
      ? " · 重读提示：" + runResultReadError
      : "";
    resultText = readLabel + runStatusLabel(runResult.status) + " · " + qualityState + warnings + readError;
  } else if (activeRunReadMatches) {
    const readLabel = runResultReadStatus === "loading"
      ? "当前 Run 结果：正在读取服务端结果"
      : runResultReadStatus === "retrying"
        ? "当前 Run 结果：暂时不可读，正在自动重试"
        : "当前 Run 结果：读取失败";
    resultText = readLabel + (runResultReadError ? " · " + runResultReadError : "");
  } else {
    resultText = recentTask ? "结果详情：尚未载入当前页面" : "分析结果：未知";
  }

  const displayFieldNodes = {
    updated: { className: "watchlist-item-meta", text: "添加/更新于 " + (record?.updated_at ? formatDateTime(record.updated_at) : "时间未记录") },
    quote: { className: "watchlist-tracking-line", text: quoteText, title: quoteTitle },
    today: { className: "watchlist-tracking-line" + (todayTask ? " is-today" : " is-unknown"), text: todayText },
    task: { className: "watchlist-tracking-line", text: taskText },
    result: { className: "watchlist-tracking-line", text: resultText }
  };
  return (Array.isArray(displayOrder) ? displayOrder : [])
    .filter((field) => Array.isArray(displayFields) && displayFields.includes(field))
    .map((field) => displayFieldNodes[field] ? { key: field, ...displayFieldNodes[field] } : null)
    .filter(Boolean);
}

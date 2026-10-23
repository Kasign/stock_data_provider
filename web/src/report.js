function escapeMarkdownText(value) {
  return String(value)
    .replace(/\r?\n/g, " / ")
    .replace(/\\/g, "\\\\")
    .replace(/([\x60*_{}()\[\]#+.!|<>~])/g, "\\$1");
}

function inlineJson(value) {
  const serialized = JSON.stringify(value);
  if (serialized === undefined) return "未报告";
  const longestTicks = [...serialized.matchAll(/\x60+/g)]
    .reduce((longest, match) => Math.max(longest, match[0].length), 0);
  const fence = "\x60".repeat(longestTicks + 1);
  return fence + serialized + fence;
}

function valueText(value) {
  if (value === null || value === undefined || value === "") return "未报告";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "object") {
    try {
      return inlineJson(value);
    } catch {
      return escapeMarkdownText(String(value));
    }
  }
  return escapeMarkdownText(value);
}

function collectionCount(value) {
  return Array.isArray(value) ? String(value.length) : "未报告";
}

function pushFields(lines, record, fields) {
  fields.forEach(([label, key]) => lines.push("- " + label + "：" + valueText(record ? record[key] : undefined)));
}

function pushCollection(lines, title, values, fields, getTitle) {
  lines.push("", "## " + title, "");
  if (values === null || values === undefined) {
    lines.push("本次 Run 未报告相关数据（字段缺失或 null）。");
    return;
  }
  if (!Array.isArray(values)) {
    lines.push("本次 Run 返回的集合格式无效，未展示。");
    return;
  }
  if (values.length === 0) {
    lines.push("本次 Run 明确返回空集合。");
    return;
  }
  values.forEach((item, index) => {
    const itemTitle = typeof getTitle === "function" ? getTitle(item, index) : title + " " + (index + 1);
    lines.push("### " + valueText(itemTitle));
    pushFields(lines, item, fields);
    lines.push("");
  });
}

export function buildRunResultMarkdown(result = {}) {
  const request = result.request || {};
  const quality = result.quality || {};
  const market = result.market_state || {};
  const execution = result.execution_assessment || {};
  const lines = [
    "# A 股分析结果", "",
    "- Run ID：" + valueText(result.run_id),
    "- 运行状态：" + valueText(result.status),
    "- 标的：" + valueText(request.symbols),
    "- 任务类型：" + valueText(request.task_type),
    "- 分析市场：" + valueText(request.market_id),
    "- 分析时点：" + valueText(request.as_of),
    "- 开始时间：" + valueText(result.started_at),
    "- 结束时间：" + valueText(result.finished_at), "",
    "> 本文件按服务端返回的结构化结果生成。没有返回的字段会标为“未报告”；本报告不构成交易建议。",
    "", "## 结果摘要", "",
    "- 核心质量门禁：" + valueText(quality.core_pass),
    "- 是否部分数据：" + valueText(quality.partial),
    "- 市场状态：" + valueText(market.regime),
    "- 因子记录数：" + collectionCount(result.factors),
    "- 候选信号数：" + collectionCount(result.signals),
    "- 交易计划数：" + collectionCount(result.plans),
    "- 警告条数：" + collectionCount(result.warnings),
    "", "## 市场状态", ""
  ];
  pushFields(lines, market, [
    ["状态", "regime"], ["有效时点", "as_of"], ["算法版本", "algorithm_version"],
    ["指标", "metrics"], ["证据", "evidence"]
  ]);
  lines.push("", "## 数据质量与策略门禁", "");
  pushFields(lines, quality, [
    ["核心门禁", "core_pass"], ["是否部分数据", "partial"], ["检查时间", "checked_at"],
    ["策略资格", "strategy_eligible"], ["策略阻断原因", "strategy_blockers"],
    ["质量策略版本", "policy_version"], ["质量问题", "issues"]
  ]);
  pushCollection(lines, "分析因子", result.factors, [
    ["标的", "symbol"], ["因子 ID", "factor_id"], ["因子版本", "factor_version"],
    ["因子值", "value"], ["单位", "unit"], ["有效时点", "as_of"],
    ["输入质量", "input_quality"], ["缺失原因", "missing_reason"], ["证据引用", "evidence_refs"]
  ], (item, index) => item.factor_id || ("因子 " + (index + 1)));
  pushCollection(lines, "候选信号", result.signals, [
    ["信号 ID", "id"], ["标的", "symbol"], ["方向", "direction"],
    ["策略", "strategy_id"], ["策略版本", "strategy_version"], ["信号时间", "signal_at"],
    ["参考价格", "reference_price"], ["有效截止", "valid_until"], ["预期持有天数", "expected_hold_days"],
    ["入场规则", "entry_rule"], ["退出规则", "exit_rule"], ["失效规则", "invalidation_rule"],
    ["证据引用", "evidence_refs"], ["扩展属性", "attributes"]
  ], (item, index) => (item.symbol || "标的未报告") + " · " + (item.direction || "方向未报告") + " · " + (item.strategy_id || ("信号 " + (index + 1))));
  pushCollection(lines, "可交易性检查", result.tradability, [
    ["信号 ID", "signal_id"], ["允许", "allowed"], ["原因", "reasons"],
    ["规则 ID", "rule_ids"], ["规则版本", "rule_refs"]
  ]);
  pushCollection(lines, "风险检查", result.risk, [
    ["信号 ID", "signal_id"], ["允许", "allowed"], ["原因", "reasons"],
    ["规则 ID", "rule_ids"], ["规则版本", "rule_refs"]
  ]);
  pushCollection(lines, "交易计划", result.plans, [
    ["信号 ID", "signal_id"], ["标的", "symbol"], ["方向", "side"], ["数量", "quantity"],
    ["参考价格", "reference_price"], ["止损价", "stop_price"], ["目标价", "target_price"],
    ["最早执行时间", "earliest_execute_at"], ["过期时间", "expire_at"],
    ["风险预算", "risk_budget"], ["成本估算", "cost_estimate"], ["规则版本", "rule_version"]
  ], (item, index) => (item.symbol || "标的未报告") + " · " + (item.side || ("计划 " + (index + 1))));
  pushCollection(lines, "计划拒绝原因", result.planning_rejections, [
    ["信号 ID", "signal_id"], ["拒绝原因", "reasons"]
  ]);
  lines.push("", "## 执行评估", "");
  pushFields(lines, execution, [["模型版本", "model_version"], ["执行警告", "warnings"]]);
  pushCollection(lines, "执行订单评估", execution.orders, [
    ["信号 ID", "signal_id"], ["状态", "status"], ["请求数量", "requested_quantity"],
    ["已成交数量", "filled_quantity"], ["剩余数量", "remaining_quantity"], ["原因", "reason"]
  ]);
  if (result.failure) {
    lines.push("", "## 运行失败信息", "");
    pushFields(lines, result.failure, [["错误代码", "code"], ["失败阶段", "stage"], ["消息", "message"]]);
  }
  lines.push("", "## 警告", "");
  if (Array.isArray(result.warnings) && result.warnings.length) result.warnings.forEach((warning) => lines.push("- " + valueText(warning)));
  else lines.push("本次 Run 未返回 warning。");
  return lines.join("\n") + "\n";
}

export function buildRunResultFilename(result = {}, extension = "json", now = new Date()) {
  const symbols = Array.isArray(result.request?.symbols) ? result.request.symbols : [];
  const label = (symbols.join("-") || "analysis").replace(/[^0-9A-Za-z.-]+/g, "_").slice(0, 80);
  const timeValue = result.request?.as_of || result.started_at || now.toISOString();
  const compactTime = String(timeValue).slice(0, 19).replace(/[^0-9A-Za-z]/g, "-");
  const suffix = String(extension).replace(/^\.+/, "").replace(/[^a-zA-Z0-9]/g, "") || "txt";
  return label + "-" + compactTime + "." + suffix;
}

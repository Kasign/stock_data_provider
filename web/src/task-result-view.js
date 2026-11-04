function resultDisplayValue(value) {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) return value.length ? value.map(resultDisplayValue).join("、") : "无";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function createResultSection(document, root, title, description = "") {
  const section = document.createElement("section");
  section.className = "analysis-result-block";
  const heading = document.createElement("h3");
  heading.textContent = title;
  section.append(heading);
  if (description) {
    const detail = document.createElement("p");
    detail.textContent = description;
    section.append(detail);
  }
  const grid = document.createElement("div");
  grid.className = "analysis-result-grid";
  section.append(grid);
  if (title === "数据质量与策略门禁") {
    const disclosure = document.createElement("details");
    disclosure.className = "analysis-result-disclosure";
    const summary = document.createElement("summary");
    summary.textContent = "数据质量与策略门禁 · 展开诊断";
    disclosure.append(summary, section);
    root.append(disclosure);
  } else {
    root.append(section);
  }
  return grid;
}

function appendResultEmpty(document, parent, message) {
  const empty = document.createElement("div");
  empty.className = "analysis-result-empty";
  empty.textContent = message;
  parent.append(empty);
}

function appendResultCard(document, parent, title, fields, badgeText = "", badgeTone = "") {
  const card = document.createElement("article");
  card.className = "analysis-result-card";
  const heading = document.createElement("div");
  heading.className = "analysis-result-card-heading";
  const titleNode = document.createElement("strong");
  titleNode.textContent = title;
  heading.append(titleNode);
  if (badgeText) {
    const badge = document.createElement("span");
    badge.className = "analysis-result-badge" + (badgeTone ? " " + badgeTone : "");
    badge.textContent = badgeText;
    heading.append(badge);
  }
  card.append(heading);
  const details = document.createElement("dl");
  details.className = "analysis-result-fields";
  fields.forEach(([label, value]) => {
    const term = document.createElement("dt");
    term.textContent = label;
    const description = document.createElement("dd");
    description.textContent = resultDisplayValue(value);
    details.append(term, description);
  });
  if (fields.length) card.append(details);
  parent.append(card);
  return card;
}

function resultPairs(record, fields) {
  return fields.map(([label, key, transform]) => [
    label,
    typeof transform === "function" ? transform(record?.[key]) : record?.[key]
  ]);
}


export function renderTaskResultView({ document, result, runStatusLabel, fallbackRunId = "" }) {
  const quality = result.quality || {};
  const factors = Array.isArray(result.factors) ? result.factors : [];
  const signals = Array.isArray(result.signals) ? result.signals : [];
  const plans = Array.isArray(result.plans) ? result.plans : [];
  const warnings = Array.isArray(result.warnings) ? result.warnings : [];
  const metrics = [
    ["运行状态", runStatusLabel(result.status)],
    ["核心质量门禁", quality.core_pass === true ? "通过" : quality.core_pass === false ? "未通过" : "未报告"],
    ["分析时点", result.request?.as_of || result.market_state?.as_of || result.started_at || "未报告"],
    ["因子数量", Array.isArray(result.factors) ? factors.length : "未报告"],
    ["候选信号", Array.isArray(result.signals) ? signals.length : "未报告"],
    ["计划数量", Array.isArray(result.plans) ? plans.length : "未报告"],
    ["警告数量", Array.isArray(result.warnings) ? warnings.length : "未报告"]
  ];
  const metricRoot = document.querySelector("#task-result-metrics");
  metricRoot.replaceChildren();
  metrics.forEach(([label, value]) => {
    const card = document.createElement("div");
    card.className = "task-result-metric";
    const caption = document.createElement("small");
    caption.textContent = label;
    const amount = document.createElement("strong");
    amount.textContent = String(value);
    card.append(caption, amount);
    metricRoot.append(card);
  });

  const root = document.querySelector("#task-result-structured");
  root.replaceChildren();

  const overview = createResultSection(document, root, "本次分析概况");
  appendResultCard(document, overview, "分析请求", [
    ["标的", result.request?.symbols],
    ["任务类型", result.request?.task_type],
    ["市场", result.request?.market_id],
    ["分析时点", result.request?.as_of],
    ["时点模式", result.request?.as_of_mode],
    ["策略", result.request?.strategy_ids],
    ["策略配置版本", result.request?.strategy_config_version],
    ["风险配置版本", result.request?.risk_config_version]
  ]);
  appendResultCard(document, overview, "运行信息", [
    ["Run ID", result.run_id],
    ["状态", runStatusLabel(result.status)],
    ["开始时间", result.started_at],
    ["结束时间", result.finished_at],
    ["Universe Snapshot", result.universe?.id],
    ["数据快照", result.raw_snapshot_id],
    ["市场快照", result.market_snapshot_id],
    ["质量策略版本", quality.policy_version]
  ]);

  const marketGrid = createResultSection(document, root, "市场状态", "展示后端返回的市场状态、指标与证据；不会在前端自行推导大盘结论。");
  if (result.market_state && Object.keys(result.market_state).length) {
    const market = result.market_state;
    appendResultCard(document, marketGrid, market.regime || "市场状态", [
      ["状态", market.regime],
      ["有效时点", market.as_of],
      ["算法版本", market.algorithm_version],
      ["指标", market.metrics],
      ["证据", market.evidence]
    ]);
  } else {
    appendResultEmpty(document, marketGrid, "本次 Run 未返回 market_state。");
  }

  const qualityGrid = createResultSection(document, root, "数据质量与策略门禁", "缺失字段保留为未报告，不把未知值当作 0 或通过。");
  appendResultCard(document, qualityGrid, "质量报告", [
    ["核心门禁", quality.core_pass === true ? "通过" : quality.core_pass === false ? "未通过" : "未报告"],
    ["部分数据", quality.partial],
    ["检查时间", quality.checked_at],
    ["策略资格", quality.strategy_eligible],
    ["策略阻断原因", quality.strategy_blockers],
    ["质量策略版本", quality.policy_version]
  ], quality.core_pass === true ? "核心门禁通过" : quality.core_pass === false ? "核心门禁未通过" : "状态未知",
  quality.core_pass === true ? "is-allowed" : quality.core_pass === false ? "is-blocked" : "");
  const issues = Array.isArray(quality.issues) ? quality.issues : [];
  if (issues.length) {
    issues.forEach((issue, index) => {
      appendResultCard(document, qualityGrid, issue.message || issue.code || ("质量问题 " + (index + 1)), resultPairs(issue, [
        ["严重程度", "severity"], ["问题代码", "code"], ["数据集", "dataset"],
        ["标的", "symbol"], ["字段", "field"], ["策略", "strategy_id"]
      ]), issue.severity || "");
    });
  } else {
    appendResultEmpty(document, qualityGrid, "没有返回 quality.issues；这不等同于所有可选数据都完整。");
  }

  const factorGrid = createResultSection(document, root, "分析因子", "显示实际计算结果、输入质量和证据引用。");
  if (factors.length) {
    factors.forEach((factor, index) => {
      appendResultCard(document, factorGrid, factor.factor_id || ("因子 " + (index + 1)), resultPairs(factor, [
        ["标的", "symbol"], ["因子版本", "factor_version"], ["因子值", "value"],
        ["单位", "unit"], ["有效时点", "as_of"], ["输入质量", "input_quality"],
        ["缺失原因", "missing_reason"], ["证据引用", "evidence_refs"]
      ]), factor.input_quality || "");
    });
  } else {
    appendResultEmpty(document, factorGrid, "本次 Run 未返回分析因子。");
  }

  const signalGrid = createResultSection(document, root, "候选信号", "这些是分析引擎生成的候选信号，不是自动交易指令。");
  if (signals.length) {
    signals.forEach((signal, index) => {
      appendResultCard(document, signalGrid, (signal.symbol || "标的未报告") + " · " + (signal.direction || "方向未报告"), resultPairs(signal, [
        ["信号 ID", "id"], ["策略", "strategy_id"], ["策略版本", "strategy_version"],
        ["信号时间", "signal_at"], ["参考价格", "reference_price"],
        ["有效截止", "valid_until"], ["预期持有天数", "expected_hold_days"],
        ["入场规则", "entry_rule"], ["退出规则", "exit_rule"],
        ["失效规则", "invalidation_rule"], ["证据引用", "evidence_refs"],
        ["扩展属性", "attributes"]
      ]), signal.strategy_id || ("候选 " + (index + 1)));
    });
  } else {
    appendResultEmpty(document, signalGrid, "本次 Run 未生成候选信号；请同时查看质量门禁、拒绝原因及警告。");
  }

  const renderDecisions = (title, values) => {
    const grid = createResultSection(document, root, title);
    if (!Array.isArray(values) || !values.length) {
      appendResultEmpty(document, grid, "本次 Run 未返回 " + title + " 决策。");
      return;
    }
    values.forEach((decision, index) => {
      appendResultCard(document, grid, decision.signal_id || (title + " " + (index + 1)), resultPairs(decision, [
        ["允许", "allowed"], ["阻断/通过原因", "reasons"],
        ["规则 ID", "rule_ids"], ["规则版本", "rule_refs"]
      ]), decision.allowed === true ? "允许" : decision.allowed === false ? "阻断" : "未报告",
      decision.allowed === true ? "is-allowed" : decision.allowed === false ? "is-blocked" : "");
    });
  };
  renderDecisions("可交易性检查", result.tradability);
  renderDecisions("风险检查", result.risk);

  const plansGrid = createResultSection(document, root, "交易计划", "仅呈现后端实际生成的计划；页面不会自行补造价格或数量。");
  if (plans.length) {
    plans.forEach((plan, index) => {
      appendResultCard(document, plansGrid, (plan.symbol || "标的未报告") + " · " + (plan.side || "方向未报告"), resultPairs(plan, [
        ["信号 ID", "signal_id"], ["数量", "quantity"], ["参考价格", "reference_price"],
        ["止损价", "stop_price"], ["目标价", "target_price"],
        ["最早执行时间", "earliest_execute_at"], ["过期时间", "expire_at"],
        ["风险预算", "risk_budget"], ["成本估算", "cost_estimate"], ["规则版本", "rule_version"]
      ]), "计划 " + (index + 1));
    });
  } else {
    appendResultEmpty(document, plansGrid, "没有返回交易计划。请检查下方的计划拒绝、质量和风险决策。");
  }

  const rejectionsGrid = createResultSection(document, root, "计划拒绝原因");
  const rejections = Array.isArray(result.planning_rejections) ? result.planning_rejections : [];
  if (rejections.length) {
    rejections.forEach((rejection, index) => {
      appendResultCard(document, rejectionsGrid, rejection.signal_id || ("拒绝项 " + (index + 1)), resultPairs(rejection, [
        ["信号 ID", "signal_id"], ["拒绝原因", "reasons"]
      ]), "未生成计划", "is-blocked");
    });
  } else {
    appendResultEmpty(document, rejectionsGrid, "本次 Run 未返回 planning_rejections。");
  }

  const executionGrid = createResultSection(document, root, "执行评估");
  const execution = result.execution_assessment || {};
  appendResultCard(document, executionGrid, "评估模型", resultPairs(execution, [["模型版本", "model_version"], ["警告", "warnings"]]));
  const orders = Array.isArray(execution.orders) ? execution.orders : [];
  if (orders.length) {
    orders.forEach((order, index) => {
      appendResultCard(document, executionGrid, order.signal_id || ("订单评估 " + (index + 1)), resultPairs(order, [
        ["状态", "status"], ["请求数量", "requested_quantity"], ["已成交数量", "filled_quantity"],
        ["剩余数量", "remaining_quantity"], ["原因", "reason"]
      ]), order.status || "");
    });
  } else {
    appendResultEmpty(document, executionGrid, "没有返回订单执行评估记录；不应将其理解为已成交。");
  }

  const failure = result.failure;
  if (failure) {
    const failureGrid = createResultSection(document, root, "运行失败信息");
    appendResultCard(document, failureGrid, failure.message || "分析失败", resultPairs(failure, [
      ["错误代码", "code"], ["失败阶段", "stage"], ["消息", "message"]
    ]), failure.code || "Failure", "is-blocked");
  }

  const warningRoot = document.querySelector("#task-result-warnings");
  warningRoot.replaceChildren();
  warningRoot.hidden = warnings.length === 0;
  warnings.forEach((warning) => {
    const line = document.createElement("p");
    line.textContent = String(warning);
    warningRoot.append(line);
  });

  document.querySelector("#task-result-json").textContent = JSON.stringify(result, null, 2);
  document.querySelector("#task-result-note").textContent = "Run ID：" + (result.run_id || fallbackRunId || "—") + " · 状态：" + runStatusLabel(result.status) + "。结果以 Go API 返回为准，仅供研究与工程验证，不构成交易建议。";
  document.querySelector("#task-result-section").hidden = false;
}

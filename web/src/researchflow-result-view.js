// Researchflow 降级结果渲染器（SRV-CTR-001 1.1.0 authorized_domain_results）。
//
// 严格边界：输入必须是 researchflow-flow.js 的 projectResultsView 输出（降级视图）。
// 本渲染器绝不把授权领域结果伪装成完整研究报告：显式标注“非完整研究报告”，
// risk 项只渲染状态、不渲染 payload。任何完整报告视图都 fail-closed 拒绝。

const RESULT_LABELS = Object.freeze({
  indicator: "指标",
  factor: "因子",
  trend: "趋势",
  risk: "风险"
});

function fail(message) {
  const error = new Error(message);
  error.responseContractError = true;
  return error;
}

function displayValue(value) {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) return value.length ? value.map(displayValue).join("、") : "无";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function validateView(view) {
  if (!view || typeof view !== "object" || Array.isArray(view)) throw fail("降级结果视图无效");
  if (view.isFullReport !== false) throw fail("拒绝完整报告视图：本渲染器只接受降级领域结果");
  if (view.resourceKind !== "authorized_domain_results") throw fail("resource_kind 必须是 authorized_domain_results");
  if (typeof view.runRef !== "string" || !view.runRef.trim()) throw fail("降级结果视图缺少 runRef");
  if (!Array.isArray(view.items)) throw fail("降级结果视图缺少 items");
  for (const item of view.items) {
    if (!item || typeof item !== "object") throw fail("结果项结构无效");
    if (!(item.resultType in RESULT_LABELS)) throw fail("结果项类型不受支持");
    if (item.resultType === "risk") {
      if (item.hasPayload === true || item.payload !== undefined) throw fail("risk 项不得携带 payload");
    } else {
      if (item.hasPayload !== true) throw fail("indicator/factor/trend 项缺少 payload");
      if (!item.payload || typeof item.payload !== "object" || Array.isArray(item.payload)) {
        throw fail("payload 必须是对象");
      }
    }
  }
}

function resultSummary(item) {
  if (item.resultType === "risk") return "仅状态（无明细）";
  const payload = item.payload;
  const code = payload.indicator_code ?? payload.factor_code ?? payload.direction ?? payload.result_code;
  const value = payload.value ?? payload.strength ?? payload.score ?? payload.signal;
  if (code !== undefined && value !== undefined) return code + " = " + displayValue(value);
  if (code !== undefined) return displayValue(code);
  if (value !== undefined) return displayValue(value);
  return "已授权";
}

function appendCard(document, parent, item) {
  const card = document.createElement("article");
  card.className = "analysis-result-card";
  const heading = document.createElement("div");
  heading.className = "analysis-result-card-heading";
  const title = document.createElement("strong");
  title.textContent = RESULT_LABELS[item.resultType] + "结果";
  heading.append(title);
  const badge = document.createElement("span");
  badge.className = "analysis-result-badge";
  badge.textContent = item.resultStatus || "未报告";
  heading.append(badge);
  card.append(heading);
  const fields = document.createElement("dl");
  fields.className = "analysis-result-fields";
  const pairs = [
    ["摘要", resultSummary(item)],
    ["质量状态", displayValue(item.qualityStatus)],
    ["分析时点", displayValue(item.systemAsOf)],
    ["授权引用", displayValue(item.authorizationRef)],
    ["结果 ID", displayValue(item.resultId)]
  ];
  for (const [label, value] of pairs) {
    const term = document.createElement("dt");
    term.textContent = label;
    const description = document.createElement("dd");
    description.textContent = displayValue(value);
    fields.append(term, description);
  }
  card.append(fields);
  parent.append(card);
}

export function renderResearchflowResultsView({ document, view, runStatusLabel }) {
  validateView(view);

  const metricRoot = document.querySelector("#task-result-metrics");
  const root = document.querySelector("#task-result-structured");
  const note = document.querySelector("#task-result-note");
  metricRoot.replaceChildren();
  root.replaceChildren();

  if (note) {
    note.textContent = "授权领域结果（非完整研究报告）。完整报告需另行授权与生成。";
  }

  const metrics = [
    ["运行状态", typeof runStatusLabel === "function" ? runStatusLabel(view.runStatus) : displayValue(view.runStatus)],
    ["结果项数", String(view.items.length)],
    ["资源类型", view.resourceKind],
    ["报告性质", "非完整研究报告"]
  ];
  for (const [label, value] of metrics) {
    const card = document.createElement("div");
    card.className = "task-result-metric";
    const caption = document.createElement("small");
    caption.textContent = label;
    const amount = document.createElement("strong");
    amount.textContent = displayValue(value);
    card.append(caption, amount);
    metricRoot.append(card);
  }

  const overview = document.createElement("section");
  overview.className = "analysis-result-block";
  const heading = document.createElement("h3");
  heading.textContent = "授权领域结果 · 非完整研究报告";
  overview.append(heading);
  const meta = document.createElement("p");
  meta.textContent = "Run 引用 " + view.runRef + " · 资源类型 " + view.resourceKind;
  overview.append(meta);
  const grid = document.createElement("div");
  grid.className = "analysis-result-grid";
  overview.append(grid);

  if (!view.items.length) {
    const empty = document.createElement("div");
    empty.className = "analysis-result-empty";
    empty.textContent = "暂无已授权的领域结果。Run 可能尚未完成，或结果未通过当前授权策略。";
    grid.append(empty);
  } else {
    for (const item of view.items) appendCard(document, grid, item);
  }
  root.append(overview);
}

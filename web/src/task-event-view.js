export function renderRunStatusView({ target, status = "", runId = "", runStatusLabel }) {
  const normalized = String(status || "").toLowerCase();
  const label = normalized ? runStatusLabel(normalized) : runId ? "等待 Run 结果" : "尚未关联 Run";
  const tone = normalized === "completed"
    ? "is-local"
    : ["partial", "blocked_data_quality", "rejected_request", "failed"].includes(normalized)
      ? "is-unavailable"
      : "is-readonly";
  target.className = "availability-pill " + tone;
  target.textContent = label;
  target.title = normalized || "Run 尚未返回状态";
  target.dataset.runStatus = normalized || "unknown";
}

export function renderSelectedTaskEventView({
  panel,
  detailLayout,
  title,
  message,
  status,
  fieldsRoot,
  json,
  item,
  formatDateTime,
  taskEventStatusKey,
  taskEventStatusLabel,
  taskEventStatusTone
}) {
  if (!panel) return;
  if (!item) {
    panel.hidden = true;
    detailLayout?.classList.remove("has-selected-event");
    return;
  }

  const event = item.event && typeof item.event === "object" ? item.event : item;
  panel.hidden = false;
  detailLayout?.classList.add("has-selected-event");
  const sequence = Number(item.sequence);
  const label = event.stage || event.name || "阶段事件";
  title.textContent = label + " · #" + (Number.isFinite(sequence) ? sequence : "未报告");
  message.textContent = event.message || "服务端未提供事件说明。";

  const statusKey = taskEventStatusKey(event.status);
  const statusText = taskEventStatusLabel(event.status, statusKey);
  status.textContent = statusText;
  status.className = "availability-pill " + taskEventStatusTone(statusKey);
  status.title = event.status || "服务端未报告状态";
  status.dataset.eventStatus = statusKey;

  const fields = [
    ["服务端序号", Number.isFinite(sequence) ? sequence : null],
    ["阶段键", event.stage || event.name],
    ["事件状态", statusText],
    ["服务端时间", event.at ? formatDateTime(event.at) : null],
    ["错误代码", event.error_code]
  ];
  if (event.duration_ms !== undefined && event.duration_ms !== null && event.duration_ms !== "" &&
      Number.isFinite(Number(event.duration_ms)) && Number(event.duration_ms) >= 0) {
    const durationMs = Number(event.duration_ms);
    fields.push(["服务端耗时", durationMs >= 1000 ? (durationMs / 1000).toFixed(2) + " 秒" : durationMs + " 毫秒"]);
  }

  const document = fieldsRoot.ownerDocument;
  fieldsRoot.replaceChildren();
  fields.forEach(([labelText, value]) => {
    if (value === undefined || value === null || value === "") return;
    const field = document.createElement("div");
    field.className = "event-detail-field";
    const caption = document.createElement("small");
    caption.textContent = labelText;
    const content = document.createElement("strong");
    content.textContent = String(value);
    field.append(caption, content);
    fieldsRoot.append(field);
  });
  json.textContent = JSON.stringify(item, null, 2);
}

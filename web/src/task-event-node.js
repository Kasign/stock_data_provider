import { taskEventDurationLabel } from "./task-event-format.js";

export function buildTaskEventNode({
  document,
  item,
  selected = false,
  expanded = false,
  formatDateTime,
  taskEventStatusKey,
  taskEventStatusLabel
}) {
  const event = item?.event && typeof item.event === "object" ? item.event : item;
  const sequence = Number(item.sequence);
  const statusKey = taskEventStatusKey(event.status);
  const statusLabel = taskEventStatusLabel(event.status, statusKey);
  const node = document.createElement("details");
  node.className = "run-flow-node event-row event-row-" + statusKey;
  node.dataset.eventSequence = String(item.sequence);
  node.setAttribute("aria-pressed", String(selected));
  node.open = selected || expanded;

  const heading = document.createElement("summary");
  heading.className = "run-flow-node-heading";
  const icon = document.createElement("span");
  icon.className = "event-status event-status-" + statusKey;
  icon.textContent = ["succeeded", "success", "completed"].includes(statusKey) ? "✓"
    : ["failed", "error", "blocked", "rejected"].includes(statusKey) ? "!"
      : statusKey === "skipped" ? "–"
        : ["running", "started", "in-progress", "in_progress", "processing"].includes(statusKey) ? "↻" : "·";
  const body = document.createElement("span");
  body.className = "event-body";
  const title = document.createElement("strong");
  title.textContent = event.stage || event.name || "分析阶段";
  const titleLine = document.createElement("span");
  titleLine.className = "event-row-heading";
  const badge = document.createElement("span");
  badge.className = "event-state-label";
  badge.textContent = statusLabel;
  titleLine.append(title, badge);
  const meta = document.createElement("small");
  const parts = ["#" + item.sequence, formatDateTime(event.at)];
  const durationLabel = taskEventDurationLabel(event.duration_ms);
  if (durationLabel) parts.push(durationLabel);
  meta.textContent = parts.join(" · ");
  body.append(titleLine, meta);
  heading.append(icon, body);
  node.append(heading);

  const content = document.createElement("div");
  content.className = "run-flow-node-content";
  const message = document.createElement("p");
  message.textContent = event.message || "服务端事件未提供描述";
  content.append(message);
  if (event.error_code) {
    const diagnostic = document.createElement("span");
    diagnostic.className = "event-diagnostic";
    diagnostic.textContent = "错误代码：" + String(event.error_code);
    content.append(diagnostic);
  }
  const select = document.createElement("button");
  select.type = "button";
  select.className = "text-button run-flow-node-detail";
  select.dataset.taskEventSequence = String(item.sequence);
  select.textContent = selected ? "当前节点详情已显示" : "查看节点详情";
  content.append(select);
  node.append(content);
  return node;
}

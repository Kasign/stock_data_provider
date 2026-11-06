import { taskEventMatchesFilter } from "./task-event-filter.js";
import { taskEventStatusKey } from "./task-event-format.js";

const FILTER_LABELS = Object.freeze({
  all: "全部",
  issues: "异常 / 降级",
  active: "运行中",
  completed: "成功 / 跳过"
});

const PRIORITY_STATUSES = new Set([
  "failed", "error", "blocked", "rejected", "degraded", "partial",
  "running", "started", "in-progress", "in_progress", "processing"
]);

function eventStatusKey(item) {
  const event = item?.event && typeof item.event === "object" ? item.event : item;
  return taskEventStatusKey(event?.status);
}

export function buildTaskEventListViewModel({ items = [], filter = "all", selectedSequence = null } = {}) {
  const allEvents = [...items].sort((a, b) => Number(a.sequence) - Number(b.sequence));
  const activeFilter = Object.prototype.hasOwnProperty.call(FILTER_LABELS, filter) ? filter : "all";
  const filterCounts = { all: allEvents.length, issues: 0, active: 0, completed: 0 };
  allEvents.forEach((item) => {
    const statusKey = eventStatusKey(item);
    for (const key of ["issues", "active", "completed"]) {
      if (taskEventMatchesFilter(statusKey, key)) filterCounts[key]++;
    }
  });
  const events = allEvents.filter((item) => taskEventMatchesFilter(eventStatusKey(item), activeFilter));
  let nextSelectedSequence = selectedSequence;
  if (events.length) {
    const hasSelected = events.some((item) => Number(item.sequence) === Number(selectedSequence));
    if (!hasSelected) {
      const priority = [...events].reverse().find((item) => PRIORITY_STATUSES.has(eventStatusKey(item)));
      nextSelectedSequence = Number((priority || events[events.length - 1]).sequence);
    }
  }
  return {
    allEvents,
    events,
    filter: activeFilter,
    filterLabels: FILTER_LABELS,
    filterCounts,
    selectedSequence: nextSelectedSequence
  };
}

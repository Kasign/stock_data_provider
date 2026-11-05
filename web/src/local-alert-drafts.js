export const LOCAL_ALERT_DRAFTS_KEY = "stock-provider-webui.alert-drafts.v1";
export const MAX_LOCAL_ALERT_DRAFTS = 100;

const TYPES = Object.freeze({
  price_above: "价格高于",
  price_below: "价格低于",
  change_above: "涨跌幅高于",
  change_below: "涨跌幅低于",
  turnover_above: "换手率高于"
});

function getStorage(storage) {
  if (storage) return storage;
  try {
    return globalThis.localStorage;
  } catch {
    return null;
  }
}

function normalize(item) {
  if (!item || typeof item !== "object" || Array.isArray(item)) return null;
  const id = String(item.id || "").trim();
  const name = String(item.name || "").trim();
  const type = String(item.type || "");
  const threshold = Number(item.threshold);
  const symbol = item.symbol == null ? "" : item.symbol;
  const scope = item.scope == null || item.scope === "" ? "symbol" : item.scope;
  const severity = item.severity == null || item.severity === "" ? "info" : item.severity;
  const createdAt = String(item.createdAt || "");
  const updatedAt = String(item.updatedAt || "");
  if (!id || id.length > 160 || !name || name.length > 80 || !TYPES[type] || !Number.isFinite(threshold)
    || threshold < 0 && type !== "change_above" && type !== "change_below"
    || typeof symbol !== "string" || symbol.length > 32
    || !["symbol", "watchlist"].includes(scope)
    || !["info", "warning", "critical"].includes(severity)
    || typeof item.enabled !== "boolean"
    || createdAt.length > 64 || updatedAt.length > 64) return null;
  if ((type === "change_above" || type === "change_below") && (threshold < -100 || threshold > 100)) return null;
  if (validateLocalAlertDraft({ name, type, threshold, symbol, scope })) return null;
  return {
    id, name, type, threshold, symbol, scope, severity,
    enabled: item.enabled === true,
    createdAt,
    updatedAt
  };
}

function itemSnapshot(item) {
  return item ? JSON.stringify(item) : "null";
}

export function validateLocalAlertDraft(item) {
  if (!item || typeof item !== "object") return "invalid";
  const name = String(item.name || "").trim();
  const type = String(item.type || "");
  const threshold = Number(item.threshold);
  const scope = String(item.scope || "symbol");
  const symbol = String(item.symbol || "").trim();
  if (!name || name.length > 80 || !TYPES[type] || !Number.isFinite(threshold)) return "invalid";
  if (!["symbol", "watchlist"].includes(scope)) return "invalid";
  if (scope === "symbol" && !symbol) return "symbol-required";
  if ((type === "price_above" || type === "price_below") && threshold <= 0) return "price-threshold";
  if ((type === "change_above" || type === "change_below") && (threshold < -100 || threshold > 100)) return "change-threshold";
  if (threshold < 0 && type !== "change_above" && type !== "change_below") return "negative-threshold";
  return "";
}

export function readLocalAlertDrafts(storage) {
  try {
    const target = getStorage(storage);
    if (!target) return { items: [], readable: false, valid: false };
    const raw = target.getItem(LOCAL_ALERT_DRAFTS_KEY);
    if (raw === null) return { items: [], readable: true, valid: true };
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length > MAX_LOCAL_ALERT_DRAFTS) {
      return { items: [], readable: true, valid: false };
    }
    const items = parsed.map(normalize);
    if (items.some((item) => !item) || new Set(items.map((item) => item.id)).size !== items.length) {
      return { items: [], readable: true, valid: false };
    }
    return { items, readable: true, valid: true };
  } catch {
    return { items: [], readable: false, valid: false };
  }
}

export function writeLocalAlertDrafts(items, storage) {
  if (!Array.isArray(items) || items.length > MAX_LOCAL_ALERT_DRAFTS) return { ok: false, reason: "limit" };
  const values = items.map(normalize);
  if (values.some((item) => !item) || new Set(values.map((item) => item.id)).size !== values.length) {
    return { ok: false, reason: "invalid" };
  }
  try {
    const target = getStorage(storage);
    if (!target) return { ok: false, reason: "unavailable" };
    const current = readLocalAlertDrafts(target);
    if (!current.readable || !current.valid) return { ok: false, reason: "invalid-storage" };
    target.setItem(LOCAL_ALERT_DRAFTS_KEY, JSON.stringify(values));
    const check = readLocalAlertDrafts(target);
    return check.readable && check.valid && JSON.stringify(check.items) === JSON.stringify(values)
      ? { ok: true } : { ok: false, reason: "verify" };
  } catch {
    return { ok: false, reason: "unavailable" };
  }
}

export function bindLocalAlertDrafts({ root = document, storage } = {}) {
  const $ = (selector) => root.querySelector(selector);
  const form = $("#alert-rule-form");
  if (!form) return null;

  let editing = "";
  let editingSnapshot = "null";
  let editorBaselineSnapshot = "";
  let conflict = false;

  const currentEditorSnapshot = () => JSON.stringify({
    name: $("#alert-rule-name").value,
    type: $("#alert-rule-type").value,
    threshold: $("#alert-rule-threshold").value,
    scope: $("#alert-rule-scope").value,
    symbol: $("#alert-rule-symbol").value,
    severity: $("#alert-rule-severity").value,
    enabled: $("#alert-rule-enabled").value
  });

  const hasUnsavedEditorChanges = () => currentEditorSnapshot() !== editorBaselineSnapshot;

  const feedback = (message, error = false) => {
    const node = $("#local-alert-draft-feedback");
    node.textContent = message;
    node.dataset.state = error ? "error" : "ok";
  };

  const resetEditor = () => {
    editing = "";
    editingSnapshot = "null";
    conflict = false;
    form.reset();
    $("#alert-rule-cancel").hidden = true;
    $("#alert-rule-submit").textContent = "保存本地草稿";
    syncScope();
    syncThreshold();
    editorBaselineSnapshot = currentEditorSnapshot();
  };

  const render = () => {
    const host = $("#local-alert-draft-list");
    const result = readLocalAlertDrafts(storage);
    host.replaceChildren();
    if (!result.readable || !result.valid) {
      const paragraph = document.createElement("p");
      paragraph.className = "table-empty";
      paragraph.textContent = "浏览器存储不可读取或草稿数据异常；为保护原始数据，未进行修改。";
      host.append(paragraph);
      return result;
    }
    if (!result.items.length) {
      const paragraph = document.createElement("p");
      paragraph.className = "table-empty";
      paragraph.textContent = "尚无本地规则草稿。";
      host.append(paragraph);
      return result;
    }
    result.items.forEach((item) => {
      const card = document.createElement("article");
      card.className = "local-alert-draft-card";
      const copy = document.createElement("div");
      const title = document.createElement("h4");
      const description = document.createElement("p");
      const meta = document.createElement("p");
      title.textContent = item.name;
      description.textContent = (item.symbol || "未指定股票") + " · " + TYPES[item.type] + " " + item.threshold;
      meta.textContent = (item.enabled ? "草稿状态：启用" : "草稿状态：停用")
        + " · " + ({ info: "提示", warning: "警告", critical: "重要" }[item.severity])
        + " · " + (item.updatedAt || "");
      copy.append(title, description, meta);
      const actions = document.createElement("div");
      actions.className = "local-alert-draft-actions";
      [["编辑", "edit"], [item.enabled ? "停用" : "启用", "toggle"], ["删除", "delete"]].forEach(([label, action]) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "button button-secondary";
        button.textContent = label;
        button.dataset.action = action;
        button.dataset.id = item.id;
        actions.append(button);
      });
      card.append(copy, actions);
      host.append(card);
    });
    return result;
  };

  const syncScope = () => {
    const symbolScope = $("#alert-rule-scope").value === "symbol";
    $("#alert-rule-symbol-field").hidden = !symbolScope;
    $("#alert-rule-symbol").required = symbolScope;
  };

  const syncThreshold = () => {
    const type = $("#alert-rule-type").value;
    const input = $("#alert-rule-threshold");
    input.min = type === "change_above" || type === "change_below" ? "-100"
      : type === "price_above" || type === "price_below" ? "0.01" : "0";
    input.max = type === "change_above" || type === "change_below" ? "100" : "";
    input.placeholder = type === "change_above" || type === "change_below" ? "百分比，例如 5 或 -3"
      : type === "turnover_above" ? "换手率百分比" : "价格（元）";
  };

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const result = readLocalAlertDrafts(storage);
    if (!result.readable || !result.valid) {
      feedback("存储不可用或数据异常，未写入。", true);
      return;
    }
    const old = editing ? result.items.find((item) => item.id === editing) : null;
    if (editing && (conflict || itemSnapshot(old) !== editingSnapshot)) {
      conflict = true;
      feedback(old ? "这条规则已被其他标签页修改；当前编辑未保存。请取消编辑后重新载入最新版本。" : "这条规则已被其他标签页删除；当前编辑未保存。请取消编辑后再决定是否新建。", true);
      return;
    }

    const name = $("#alert-rule-name").value.trim();
    const threshold = Number($("#alert-rule-threshold").value);
    const scope = $("#alert-rule-scope").value;
    const type = $("#alert-rule-type").value;
    const symbol = scope === "symbol" ? $("#alert-rule-symbol").value.trim() : "";
    const validation = validateLocalAlertDraft({ name, type, threshold, scope, symbol });
    if (validation) {
      const messages = {
        "symbol-required": "单只股票规则必须填写股票代码。",
        "price-threshold": "价格阈值必须大于 0。",
        "change-threshold": "涨跌幅阈值必须在 -100% 至 100% 之间。",
        "negative-threshold": "阈值不能为负数。"
      };
      feedback(messages[validation] || "规则参数无效，请检查输入。", true);
      if (validation === "symbol-required") $("#alert-rule-symbol").focus();
      return;
    }

    const now = new Date().toISOString();
    const item = {
      id: editing || "draft-" + now + "-" + Math.random().toString(36).slice(2, 7),
      name, type, threshold, symbol, scope,
      severity: $("#alert-rule-severity").value,
      enabled: $("#alert-rule-enabled").value === "true",
      createdAt: old?.createdAt || now,
      updatedAt: now
    };
    const next = old
      ? result.items.map((entry) => entry.id === editing ? item : entry)
      : [item, ...result.items];
    if (next.length > MAX_LOCAL_ALERT_DRAFTS) {
      feedback("最多保存 100 条草稿。", true);
      return;
    }
    const saved = writeLocalAlertDrafts(next, storage);
    if (!saved.ok) {
      feedback("保存失败，未能确认本地存储结果。", true);
      return;
    }
    resetEditor();
    feedback("草稿已保存在当前浏览器；不会执行监控或发送通知。");
    render();
  });

  $("#alert-rule-scope").addEventListener("change", syncScope);
  $("#alert-rule-type").addEventListener("change", syncThreshold);
  $("#alert-rule-cancel").addEventListener("click", () => {
    resetEditor();
    feedback("已取消当前编辑。");
  });
  $("#alert-new-rule").addEventListener("click", () => {
    if (editing) {
      if (hasUnsavedEditorChanges() && !globalThis.confirm("当前规则有未保存的修改，确定放弃并新建草稿？")) return;
      resetEditor();
      feedback("已退出原规则编辑；可以新建草稿。");
    }
    $("#alert-rule-name").focus();
    form.scrollIntoView({ behavior: "smooth", block: "center" });
  });
  $("#local-alert-draft-list").addEventListener("click", (event) => {
    const button = event.target.closest("[data-action]");
    if (!button) return;
    const result = readLocalAlertDrafts(storage);
    if (!result.readable || !result.valid) return;
    const item = result.items.find((entry) => entry.id === button.dataset.id);
    if (!item) return;
    if (button.dataset.action === "edit") {
      if (hasUnsavedEditorChanges() && !globalThis.confirm("当前规则有未保存的修改，确定放弃并切换编辑对象？")) return;
      editing = item.id;
      editingSnapshot = itemSnapshot(item);
      conflict = false;
      $("#alert-rule-name").value = item.name;
      $("#alert-rule-type").value = item.type;
      $("#alert-rule-threshold").value = String(item.threshold);
      $("#alert-rule-scope").value = item.scope;
      $("#alert-rule-severity").value = item.severity;
      $("#alert-rule-enabled").value = String(item.enabled);
      $("#alert-rule-symbol").value = item.symbol;
      syncScope();
      syncThreshold();
      $("#alert-rule-cancel").hidden = false;
      $("#alert-rule-submit").textContent = "更新本地草稿";
      editorBaselineSnapshot = currentEditorSnapshot();
      feedback("正在编辑本地规则草稿。");
      form.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    if (editing && editing === item.id) {
      feedback("请先取消当前规则编辑，再启停或删除这条规则。", true);
      return;
    }
    if (button.dataset.action === "delete" && !globalThis.confirm("确定删除这条本地规则草稿？")) return;
    const next = result.items
      .map((entry) => button.dataset.action === "toggle" && entry.id === item.id
        ? { ...entry, enabled: !entry.enabled, updatedAt: new Date().toISOString() }
        : entry)
      .filter((entry) => button.dataset.action !== "delete" || entry.id !== item.id);
    const saved = writeLocalAlertDrafts(next, storage);
    feedback(saved.ok ? "本地草稿已更新。" : "操作失败，未能确认存储结果。", !saved.ok);
    if (saved.ok) render();
  });

  render();
  return {
    render,
    handleStorageChange(event) {
      if (event?.key !== null && event?.key !== LOCAL_ALERT_DRAFTS_KEY) return;
      const result = readLocalAlertDrafts(storage);
      if (editing) {
        const latest = result.readable && result.valid
          ? result.items.find((item) => item.id === editing) || null
          : null;
        conflict = !result.readable || !result.valid || itemSnapshot(latest) !== editingSnapshot;
        if (conflict) {
          feedback(latest
            ? "检测到其他标签页修改了正在编辑的规则。当前表单已保留；保存会被阻止，请取消编辑后重新载入最新版本。"
            : "检测到其他标签页删除了正在编辑的规则。当前表单已保留；保存会被阻止，请取消编辑。", true);
        } else {
          feedback("已同步其他标签页的规则草稿变更；当前编辑内容未改变。");
        }
      } else if (!result.readable || !result.valid) {
        feedback("其他标签页的规则草稿存储不可读取或格式异常；未覆盖原始数据。", true);
      } else {
        feedback("已同步其他标签页的本地规则草稿。");
      }
      render();
    }
  };
}

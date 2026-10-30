export const NOTIFICATION_PREFERENCES_KEY = "stock-provider-webui.notification-preferences.v1";
const EVENTS = new Set(["task_completed", "task_failed", "risk_blocked", "system_error"]);
const DEFAULTS = Object.freeze({ events: ["task_completed", "task_failed", "risk_blocked", "system_error"], quietStart: "22:00", quietEnd: "08:00" });

function storageFor(storage) {
  if (storage) return storage;
  try { return globalThis.localStorage; } catch { return null; }
}

function normalize(value) {
  if (!value || typeof value !== "object" || !Array.isArray(value.events)) return null;
  if (value.events.some((event) => !EVENTS.has(event))) return null;
  const timePattern = /^(?:[01]\d|2[0-3]):[0-5]\d$/;
  if (!timePattern.test(String(value.quietStart || "")) || !timePattern.test(String(value.quietEnd || ""))) return null;
  return { events: [...new Set(value.events)], quietStart: value.quietStart, quietEnd: value.quietEnd };
}

export function readNotificationPreferences(storage) {
  try {
    const target = storageFor(storage);
    if (!target) return { readable: false, valid: false, value: null };
    const raw = target.getItem(NOTIFICATION_PREFERENCES_KEY);
    if (raw === null) return { readable: true, valid: true, value: { ...DEFAULTS, events: [...DEFAULTS.events] } };
    const value = normalize(JSON.parse(raw));
    return value ? { readable: true, valid: true, value } : { readable: true, valid: false, value: null };
  } catch {
    return { readable: false, valid: false, value: null };
  }
}

export function writeNotificationPreferences(value, storage) {
  const normalized = normalize(value);
  if (!normalized) return { ok: false, reason: "invalid" };
  try {
    const target = storageFor(storage);
    if (!target) return { ok: false, reason: "unavailable" };
    const current = readNotificationPreferences(target);
    if (!current.readable || !current.valid) return { ok: false, reason: "invalid-storage" };
    target.setItem(NOTIFICATION_PREFERENCES_KEY, JSON.stringify(normalized));
    const check = readNotificationPreferences(target);
    return check.readable && check.valid && JSON.stringify(check.value) === JSON.stringify(normalized)
      ? { ok: true } : { ok: false, reason: "verify" };
  } catch {
    return { ok: false, reason: "unavailable" };
  }
}

export function bindNotificationPreferences({ root = document, storage } = {}) {
  const $ = (selector) => root.querySelector(selector);
  const form = $("#notification-preferences-form");
  if (!form) return;
  const feedback = (message, error = false) => {
    const node = $("#notification-preferences-feedback");
    node.textContent = message;
    node.dataset.state = error ? "error" : "ok";
  };
  const render = () => {
    const result = readNotificationPreferences(storage);
    if (!result.readable || !result.valid) {
      feedback("本地偏好不可读取或格式异常；为保护原始数据，未覆盖。", true);
      form.querySelectorAll("[name=notification-event]").forEach((input) => { input.disabled = true; });
      $("#notification-quiet-start").disabled = true;
      $("#notification-quiet-end").disabled = true;
      $("#notification-preferences-save").disabled = true;
      return;
    }
    form.querySelectorAll("[name=notification-event]").forEach((input) => {
      input.checked = result.value.events.includes(input.value);
    });
    $("#notification-quiet-start").value = result.value.quietStart;
    $("#notification-quiet-end").value = result.value.quietEnd;
    feedback("偏好草稿仅保存在当前浏览器；不会创建通知渠道或触发真实发送。");
  };
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const value = {
      events: [...form.querySelectorAll("[name=notification-event]:checked")].map((input) => input.value),
      quietStart: $("#notification-quiet-start").value,
      quietEnd: $("#notification-quiet-end").value
    };
    const saved = writeNotificationPreferences(value, storage);
    feedback(saved.ok ? "本地通知偏好草稿已保存；当前没有服务端投递行为。" : "保存失败，原有偏好未确认修改：" + saved.reason, !saved.ok);
  });
  render();
}

export const NOTIFICATION_PREFERENCES_KEY = "stock-provider-webui.notification-preferences.v1";
const EVENTS = new Set(["task_completed", "task_failed", "risk_blocked", "system_error"]);
const DEFAULTS = Object.freeze({
  events: ["task_completed", "task_failed", "risk_blocked", "system_error"],
  quietStart: "22:00",
  quietEnd: "08:00"
});

function storageFor(storage) {
  if (storage) return storage;
  try {
    return globalThis.localStorage;
  } catch {
    return null;
  }
}

function canonical(value) {
  return JSON.stringify({
    events: [...value.events].sort(),
    quietStart: value.quietStart,
    quietEnd: value.quietEnd
  });
}

function formValue(form, root) {
  return {
    events: [...form.querySelectorAll("[name=notification-event]:checked")].map((input) => input.value).sort(),
    quietStart: root.querySelector("#notification-quiet-start").value,
    quietEnd: root.querySelector("#notification-quiet-end").value
  };
}

export function validateNotificationPreferences(value, { allowEqualQuietHours = false } = {}) {
  if (!value || typeof value !== "object" || !Array.isArray(value.events)) return "invalid";
  if (value.events.some((event) => !EVENTS.has(event))) return "invalid-events";
  const timePattern = /^(?:[01]\d|2[0-3]):[0-5]\d$/;
  if (!timePattern.test(String(value.quietStart || "")) || !timePattern.test(String(value.quietEnd || ""))) return "invalid-time";
  if (!allowEqualQuietHours && value.quietStart === value.quietEnd) return "equal-quiet-hours";
  return "";
}

function normalize(value) {
  if (validateNotificationPreferences(value, { allowEqualQuietHours: true })) return null;
  return {
    events: [...new Set(value.events)],
    quietStart: value.quietStart,
    quietEnd: value.quietEnd
  };
}

export function readNotificationPreferences(storage) {
  try {
    const target = storageFor(storage);
    if (!target) return { readable: false, valid: false, value: null };
    const raw = target.getItem(NOTIFICATION_PREFERENCES_KEY);
    if (raw === null) return {
      readable: true, valid: true,
      value: { ...DEFAULTS, events: [...DEFAULTS.events].sort() }
    };
    const value = normalize(JSON.parse(raw));
    return value
      ? { readable: true, valid: true, value }
      : { readable: true, valid: false, value: null };
  } catch {
    return { readable: false, valid: false, value: null };
  }
}

export function writeNotificationPreferences(value, storage) {
  const validation = validateNotificationPreferences(value);
  const normalized = validation ? null : normalize(value);
  if (!normalized) return { ok: false, reason: validation || "invalid" };
  try {
    const target = storageFor(storage);
    if (!target) return { ok: false, reason: "unavailable" };
    const current = readNotificationPreferences(target);
    if (!current.readable || !current.valid) return { ok: false, reason: "invalid-storage" };
    target.setItem(NOTIFICATION_PREFERENCES_KEY, JSON.stringify(normalized));
    const check = readNotificationPreferences(target);
    return check.readable && check.valid && canonical(check.value) === canonical(normalized)
      ? { ok: true } : { ok: false, reason: "verify" };
  } catch {
    return { ok: false, reason: "unavailable" };
  }
}

export function bindNotificationPreferences({ root = document, storage } = {}) {
  const $ = (selector) => root.querySelector(selector);
  const form = $("#notification-preferences-form");
  if (!form) return null;

  let baselineSnapshot = null;
  let conflict = false;

  const feedback = (message, error = false) => {
    const node = $("#notification-preferences-feedback");
    node.textContent = message;
    node.dataset.state = error ? "error" : "ok";
  };

  const setControlsDisabled = (disabled) => {
    form.querySelectorAll("[name=notification-event]").forEach((input) => { input.disabled = disabled; });
    $("#notification-quiet-start").disabled = disabled;
    $("#notification-quiet-end").disabled = disabled;
    $("#notification-preferences-save").disabled = disabled;
  };

  const render = () => {
    const result = readNotificationPreferences(storage);
    if (!result.readable || !result.valid) {
      baselineSnapshot = null;
      setControlsDisabled(true);
      feedback("本地偏好不可读取或格式异常；为保护原始数据，未覆盖。", true);
      return result;
    }
    setControlsDisabled(false);
    form.querySelectorAll("[name=notification-event]").forEach((input) => {
      input.checked = result.value.events.includes(input.value);
    });
    $("#notification-quiet-start").value = result.value.quietStart;
    $("#notification-quiet-end").value = result.value.quietEnd;
    baselineSnapshot = canonical(result.value);
    conflict = false;
    feedback(result.value.quietStart === result.value.quietEnd
      ? "读取到旧版免打扰时间：开始与结束相同，含义不明确；请调整时间后再保存。"
      : "偏好草稿仅保存在当前浏览器；不会创建通知渠道或触发真实发送。",
    result.value.quietStart === result.value.quietEnd);
    return result;
  };

  const isDirty = () => {
    if (baselineSnapshot === null) return false;
    return canonical(formValue(form, root)) !== baselineSnapshot;
  };

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const current = readNotificationPreferences(storage);
    if (!current.readable || !current.valid) {
      feedback("本地偏好存储不可读取或格式异常，未保存。", true);
      return;
    }
    if (baselineSnapshot === null || conflict || canonical(current.value) !== baselineSnapshot) {
      conflict = true;
      feedback("通知偏好已被其他标签页修改；当前表单已保留且未保存。请重新载入最新偏好后再编辑。", true);
      return;
    }
    const value = formValue(form, root);
    const saved = writeNotificationPreferences(value, storage);
    const messages = {
      "equal-quiet-hours": "免打扰开始和结束时间不能相同；请设置一个明确的时间区间。",
      "invalid-time": "免打扰时间格式无效，请检查开始和结束时间。",
      "invalid-events": "关注事件包含不支持的类型，未保存。",
      invalid: "通知偏好格式无效，未保存。"
    };
    if (saved.ok) {
      baselineSnapshot = canonical(value);
      conflict = false;
      feedback("本地通知偏好草稿已保存；当前没有服务端投递行为。");
    } else {
      feedback(messages[saved.reason] || "保存失败，原有偏好未确认修改：" + saved.reason, true);
    }
  });

  render();
  return {
    render,
    handleStorageChange(event) {
      if (event?.key !== null && event?.key !== NOTIFICATION_PREFERENCES_KEY) return;
      const current = readNotificationPreferences(storage);
      if (isDirty()) {
        conflict = !current.readable || !current.valid || canonical(current.value) !== baselineSnapshot;
        feedback(conflict
          ? "检测到其他标签页修改了通知偏好。当前未保存表单已保留；保存会被阻止，请重新载入最新偏好后再编辑。"
          : "已收到其他标签页的变更通知；当前未保存表单已保留。",
        conflict);
        return;
      }
      if (!current.readable || !current.valid) {
        baselineSnapshot = null;
        conflict = true;
        setControlsDisabled(true);
        feedback("其他标签页写入的通知偏好不可读取或格式异常；未覆盖当前表单。", true);
        return;
      }
      render();
      feedback("已同步其他标签页保存的通知偏好。");
    }
  };
}

import test from "node:test";
import assert from "node:assert/strict";
import { readNotificationPreferences, writeNotificationPreferences, validateNotificationPreferences, NOTIFICATION_PREFERENCES_KEY } from "../src/notification-preferences.js";

function memory() {
  const data = new Map();
  return { getItem: (key) => data.has(key) ? data.get(key) : null, setItem: (key, value) => data.set(key, value) };
}

const valid = { events: ["task_completed", "risk_blocked"], quietStart: "22:30", quietEnd: "07:30" };

test("通知偏好草稿可保存并回读校验", () => {
  const storage = memory();
  assert.equal(writeNotificationPreferences(valid, storage).ok, true);
  assert.deepEqual(readNotificationPreferences(storage).value, valid);
});

test("拒绝未知事件和非法免打扰时间", () => {
  const storage = memory();
  assert.equal(writeNotificationPreferences({ ...valid, events: ["fake"] }, storage).ok, false);
  assert.equal(writeNotificationPreferences({ ...valid, quietStart: "25:00" }, storage).ok, false);
});

test("损坏数据不会被误判为空或覆盖", () => {
  const storage = memory();
  storage.setItem(NOTIFICATION_PREFERENCES_KEY, "{");
  assert.equal(readNotificationPreferences(storage).valid, false);
  assert.equal(writeNotificationPreferences(valid, storage).ok, false);
  assert.equal(storage.getItem(NOTIFICATION_PREFERENCES_KEY), "{");
});

test("免打扰开始与结束不能相同，避免全天静默语义不明确", () => {
  assert.equal(validateNotificationPreferences({ ...valid, quietStart: "08:00", quietEnd: "08:00" }), "equal-quiet-hours");
  const storage = memory();
  assert.deepEqual(writeNotificationPreferences({ ...valid, quietStart: "08:00", quietEnd: "08:00" }, storage), {
    ok: false, reason: "equal-quiet-hours"
  });
  assert.equal(storage.getItem(NOTIFICATION_PREFERENCES_KEY), null);
});

test("历史相同起止时间仍可读取，但必须调整后才能重新保存", () => {
  const storage = memory();
  const legacy = { ...valid, quietStart: "08:00", quietEnd: "08:00" };
  storage.setItem(NOTIFICATION_PREFERENCES_KEY, JSON.stringify(legacy));
  assert.equal(readNotificationPreferences(storage).valid, true);
  assert.deepEqual(readNotificationPreferences(storage).value, legacy);
  assert.equal(writeNotificationPreferences(legacy, storage).reason, "equal-quiet-hours");
  assert.equal(storage.getItem(NOTIFICATION_PREFERENCES_KEY), JSON.stringify(legacy));
});

test("跨午夜免打扰区间有效，且允许关闭全部事件关注", () => {
  assert.equal(validateNotificationPreferences({ events: [], quietStart: "22:00", quietEnd: "08:00" }), "");
  assert.equal(writeNotificationPreferences({ events: [], quietStart: "22:00", quietEnd: "08:00" }, memory()).ok, true);
});


test("拒绝重复事件和超过支持集合上限的事件列表，损坏历史偏好不被覆盖", () => {
  const duplicate = { ...valid, events: ["task_completed", "task_completed"] };
  const oversized = { ...valid, events: ["task_completed", "task_failed", "risk_blocked", "system_error", "task_completed"] };
  for (const item of [duplicate, oversized]) {
    const storage = memory();
    assert.equal(writeNotificationPreferences(item, storage).reason, "invalid-events");
    const raw = JSON.stringify(item);
    storage.setItem(NOTIFICATION_PREFERENCES_KEY, raw);
    assert.equal(readNotificationPreferences(storage).valid, false);
    assert.equal(writeNotificationPreferences(valid, storage).reason, "invalid-storage");
    assert.equal(storage.getItem(NOTIFICATION_PREFERENCES_KEY), raw);
  }
});

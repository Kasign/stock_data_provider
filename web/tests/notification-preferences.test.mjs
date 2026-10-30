import test from "node:test";
import assert from "node:assert/strict";
import { readNotificationPreferences, writeNotificationPreferences, NOTIFICATION_PREFERENCES_KEY } from "../src/notification-preferences.js";

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

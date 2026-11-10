import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { bindTaskPollingLifecycle } from "../src/task-poll-lifecycle.js";

function target() {
  const listeners = new Map();
  return {
    addEventListener(name, callback) { listeners.set(name, callback); },
    removeEventListener(name, callback) {
      if (listeners.get(name) === callback) listeners.delete(name);
    },
    dispatch(name, event = {}) { listeners.get(name)?.(event); },
    listeners
  };
}

test("hidden tabs pause once and visible tabs resume once", () => {
  const documentRef = target();
  const windowRef = target();
  documentRef.hidden = false;
  let pauses = 0, resumes = 0;
  bindTaskPollingLifecycle({
    documentRef, windowRef,
    onPause: () => { pauses += 1; },
    onResume: () => { resumes += 1; }
  });

  documentRef.hidden = true;
  documentRef.dispatch("visibilitychange");
  windowRef.dispatch("pagehide", { persisted: true });
  documentRef.dispatch("visibilitychange");
  assert.equal(pauses, 1);
  assert.equal(resumes, 0);

  documentRef.hidden = false;
  documentRef.dispatch("visibilitychange");
  windowRef.dispatch("pageshow", { persisted: true });
  assert.equal(pauses, 1);
  assert.equal(resumes, 1);
});

test("BFCache pagehide/pageshow pause and resume without duplicate lifecycle callbacks", () => {
  const documentRef = target();
  const windowRef = target();
  documentRef.hidden = false;
  let pauses = 0, resumes = 0;
  bindTaskPollingLifecycle({
    documentRef, windowRef,
    onPause: () => { pauses += 1; },
    onResume: () => { resumes += 1; }
  });

  windowRef.dispatch("pagehide", { persisted: false });
  assert.equal(pauses, 0);
  windowRef.dispatch("pagehide", { persisted: true });
  windowRef.dispatch("pagehide", { persisted: true });
  assert.equal(pauses, 1);
  windowRef.dispatch("pageshow", { persisted: true });
  windowRef.dispatch("pageshow", { persisted: true });
  assert.equal(resumes, 1);
});

test("lifecycle listeners can be removed", () => {
  const documentRef = target();
  const windowRef = target();
  documentRef.hidden = false;
  const dispose = bindTaskPollingLifecycle({
    documentRef, windowRef, onPause() {}, onResume() {}
  });
  assert.equal(documentRef.listeners.has("visibilitychange"), true);
  assert.equal(windowRef.listeners.has("pagehide"), true);
  assert.equal(windowRef.listeners.has("pageshow"), true);
  dispose();
  assert.equal(documentRef.listeners.has("visibilitychange"), false);
  assert.equal(windowRef.listeners.has("pagehide"), false);
  assert.equal(windowRef.listeners.has("pageshow"), false);
});

test("app wires visibility and BFCache lifecycle only to task polling", async () => {
  const app = await readFile(new URL("../src/app.js", import.meta.url), "utf8");
  assert.match(app, /pollPausedForVisibility: false/);
  assert.match(app, /function pauseTaskPollingForHiddenPage\(\)[\s\S]*?state\.pollPausedForVisibility = true;[\s\S]*?stopTaskPolling\(\);/);
  assert.match(app, /function resumeTaskPollingAfterVisiblePage\(\)[\s\S]*?state\.pollPausedForVisibility = false;[\s\S]*?pollTaskSafely\(state\.pollGeneration\)/);
  assert.match(app, /function pollTaskSafely\([\s\S]*?if \(document\.hidden\)[\s\S]*?stopTaskPolling\(\);[\s\S]*?return Promise\.resolve\(\);/);
  assert.match(app, /bindTaskPollingLifecycle\(\{[\s\S]*?onPause: pauseTaskPollingForHiddenPage,[\s\S]*?onResume: resumeTaskPollingAfterVisiblePage/);
});

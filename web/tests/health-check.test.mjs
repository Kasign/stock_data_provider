import test from "node:test";
import assert from "node:assert/strict";
import { runHealthCheck } from "../src/health-check.js";

async function withWindowTimers(operation) {
  const previousWindow = globalThis.window;
  globalThis.window = {
    setTimeout: (...args) => globalThis.setTimeout(...args),
    clearTimeout: (timer) => globalThis.clearTimeout(timer)
  };

  try {
    await operation();
  } finally {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
  }
}

test("returns a successful health-check result", async () => {
  await withWindowTimers(async () => {
    const settings = { analysisBase: "/analysis-api" };
    const payload = { status: "ok" };
    const result = await runHealthCheck(async (receivedSettings, signal) => {
      assert.equal(receivedSettings, settings);
      assert.equal(signal.aborted, false);
      return payload;
    }, settings, 100);

    assert.equal(result.ok, true);
    assert.equal(result.data, payload);
    assert.equal(result.timedOut, undefined);
    assert.equal(typeof result.durationMs, "number");
    assert.equal(result.error, undefined);
  });
});

test("returns rejected checks as failures without marking them as timeouts", async () => {
  await withWindowTimers(async () => {
    const failure = new Error("health endpoint unavailable");
    const result = await runHealthCheck(async () => {
      throw failure;
    }, {}, 100);

    assert.equal(result.ok, false);
    assert.equal(result.error, failure);
    assert.equal(result.timedOut, false);
    assert.equal(typeof result.durationMs, "number");
    assert.equal(result.data, undefined);
  });
});

test("aborts a timed-out check and preserves timeout after a late success", async () => {
  await withWindowTimers(async () => {
    let requestSignal;
    const result = await runHealthCheck((_settings, signal) => {
      requestSignal = signal;
      return new Promise((resolve) => {
        globalThis.setTimeout(() => resolve({ status: "ok", late: true }), 25);
      });
    }, {}, 5);

    assert.equal(result.ok, false);
    assert.equal(result.timedOut, true);
    assert.equal(result.error.message, "检查超时");
    assert.equal(requestSignal.aborted, true);
    assert.equal(result.data, undefined);

    await new Promise((resolve) => globalThis.setTimeout(resolve, 35));
    assert.equal(result.ok, false);
    assert.equal(result.timedOut, true);
    assert.equal(result.data, undefined);
  });
});

test("contains synchronous setup errors from health checks", async () => {
  await withWindowTimers(async () => {
    const failure = new TypeError("invalid check callback");
    const result = await runHealthCheck(() => {
      throw failure;
    }, null, 100);

    assert.equal(result.ok, false);
    assert.equal(result.error, failure);
    assert.equal(result.timedOut, false);
  });
});

export async function runHealthCheck(check, settings, timeoutMs = 6000) {
  const controller = new AbortController();
  const started = performance.now();
  let timeoutId = null;
  let timedOut = false;
  const duration = () => Math.round(performance.now() - started);
  const timeout = new Promise((resolve) => {
    timeoutId = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
      resolve({ ok: false, error: new Error("检查超时"), timedOut: true, durationMs: duration() });
    }, timeoutMs);
  });
  const request = Promise.resolve()
    .then(() => check(settings, controller.signal))
    .then((data) => timedOut
      ? { ok: false, error: new Error("检查超时"), timedOut: true, durationMs: duration() }
      : { ok: true, data, durationMs: duration() })
    .catch((error) => ({ ok: false, error, timedOut, durationMs: duration() }));
  try {
    return await Promise.race([request, timeout]);
  } finally {
    if (timeoutId !== null) window.clearTimeout(timeoutId);
  }
}

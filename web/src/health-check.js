export async function runHealthCheck(check, settings, timeoutMs = 6000, externalSignal = null) {
  const controller = new AbortController();
  const started = performance.now();
  let timeoutId = null;
  let timedOut = false;
  let abortListener = null;
  const duration = () => Math.round(performance.now() - started);
  const timeout = new Promise((resolve) => {
    timeoutId = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
      resolve({ ok: false, error: new Error("检查超时"), timedOut: true, durationMs: duration() });
    }, timeoutMs);
  });
  const request = Promise.resolve()
    .then(() => {
      if (controller.signal.aborted) throw Object.assign(new Error("检查已取消"), { name: "AbortError" });
      return check(settings, controller.signal);
    })
    .then((data) => timedOut
      ? { ok: false, error: new Error("检查超时"), timedOut: true, durationMs: duration() }
      : { ok: true, data, durationMs: duration() })
    .catch((error) => ({ ok: false, error, timedOut, durationMs: duration() }));
  const cancelled = new Promise((resolve) => {
    if (!externalSignal) return;
    abortListener = () => {
      controller.abort();
      resolve({ ok: false, error: Object.assign(new Error("检查已取消"), { name: "AbortError" }), cancelled: true, durationMs: duration() });
    };
    if (externalSignal.aborted) abortListener();
    else externalSignal.addEventListener("abort", abortListener, { once: true });
  });
  try {
    return await Promise.race([request, timeout, cancelled]);
  } finally {
    if (timeoutId !== null) window.clearTimeout(timeoutId);
    if (abortListener) externalSignal?.removeEventListener("abort", abortListener);
  }
}

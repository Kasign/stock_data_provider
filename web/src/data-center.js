import { analysisHealth, analysisLiveness, getDataSources, getDataSourceDiagnostics, marketHealth, marketLiveness } from "./api.js";
import { runHealthCheck } from "./health-check.js";
import { describeApiError, formatDateTime } from "./format.js";

const $ = (selector) => document.querySelector(selector);
const CAPABILITIES = [
  ["quote_capable", "实时行情"], ["daily_bar_capable", "日 K"],
  ["stock_basic_capable", "证券档案"], ["stock_search_capable", "股票搜索"],
  ["sector_ranking_capable", "行业/概念排行"], ["index_quote_capable", "核心指数"],
  ["market_breadth_capable", "市场涨跌分布"], ["limit_pool_capable", "涨跌停池"],
  ["dragon_tiger_capable", "龙虎榜"], ["trading_calendar_capable", "交易日历"],
  ["fundamentals_capable", "财务指标"], ["valuation_capable", "估值指标"],
  ["stock_boards_capable", "所属板块"], ["announcements_capable", "公告"],
  ["chip_capable", "筹码分布"], ["capital_flow_capable", "个股资金流"],
  ["hot_ranking_capable", "人气/飙升榜"]
];

function setText(selector, value) {
  const node = $(selector);
  if (node) node.textContent = value === null || value === undefined || value === "" ? "—" : String(value);
}

function setBadge(selector, status, label) {
  const node = $(selector);
  if (!node) return;
  const tone = ["healthy", "ready", "enabled"].includes(status) ? "is-local"
    : ["failed", "unhealthy", "timeout", "degraded"].includes(status) ? "is-unavailable" : "is-readonly";
  node.className = "availability-pill " + tone;
  node.textContent = label;
}

function statusWord(status) {
  return ({
    healthy: "正常", ready: "就绪", failed: "异常", unhealthy: "异常",
    timeout: "超时", unknown: "未知", skipped: "未检查", degraded: "降级",
    enabled: "已注册", disabled: "已禁用"
  })[status] || status || "未知";
}

function safeError(error) {
  const status = Number(error?.status);
  const code = String(error?.code || "").trim();
  const message = String(describeApiError(error, "请求失败。") || "")
    .replace(/[\r\n\t]+/g, " ").slice(0, 180);
  return [status ? "HTTP " + status : "", code && code !== "http_error" ? code : "", message]
    .filter(Boolean).join(" · ") || "未返回错误详情";
}

async function checkWebUI(_settings, signal) {
  const response = await fetch(window.location.pathname || "/", { method: "GET", credentials: "same-origin", cache: "no-store", signal });
  if (!response.ok) {
    const error = new Error("WebUI 页面返回 HTTP " + response.status);
    error.status = response.status;
    throw error;
  }
  return { status: "ok" };
}


function probeState(result, ready = false) {
  if (!result?.ok) return result?.timedOut ? "timeout" : "failed";
  const raw = String(result.data?.status || "").trim().toLowerCase();
  if (!raw) return "unknown";
  if (["ok", "ready", "healthy", "alive", "running"].includes(raw)) return "healthy";
  if (["failed", "unhealthy", "unavailable", "error", "down"].includes(raw)) return "failed";
  if (["degraded", "partial"].includes(raw)) return "degraded";
  return "unknown";
}

function showCheck(selector, result, options = {}) {
  const node = $(selector);
  if (!node) return "unknown";
  const current = probeState(result, Boolean(options.ready));
  node.className = current === "healthy" ? "is-success"
    : current === "degraded" || current === "unknown" ? "is-warning" : "is-error";
  node.textContent = result?.ok
    ? (current === "healthy" ? (options.ready ? "就绪" : "正常")
      : current === "unknown" ? "状态未报告" : "返回状态需关注")
      + " · " + result.durationMs + " ms"
    : statusWord(current) + " · " + result.durationMs + " ms";
  return current;
}

function setDetail(selector, parts) {
  setText(selector, parts.filter((part) => part !== null && part !== undefined && String(part).trim()).join(" · "));
}

function renderProviderCapabilities(payload) {
  const root = $("#data-center-provider-list");
  root.replaceChildren();
  const sources = Array.isArray(payload?.sources)
    ? payload.sources.filter((source) => source && typeof source === "object") : [];
  const hithink = sources.find((source) => String(source.name || "").trim().toLowerCase() === "hithink");
  const hithinkStatus = $("#data-center-hithink-status");
  const hithinkDetail = $("#data-center-hithink-detail");
  if (hithinkStatus) {
    hithinkStatus.className = "availability-pill " + (hithink?.enabled ? "is-local" : "is-readonly");
    hithinkStatus.textContent = hithink ? (hithink.enabled ? "已注册 · 已启用" : "已注册 · 已禁用") : "未在注册清单中";
  }
  if (hithinkDetail) {
    const capabilities = hithink ? CAPABILITIES.filter(([key]) => hithink[key] === true).map(([, label]) => label) : [];
    hithinkDetail.textContent = hithink
      ? (hithink.enabled ? "服务端报告 Provider 已启用。" : "服务端报告 Provider 当前已禁用。")
        + (capabilities.length ? " 注册能力：" + capabilities.join("、") + "。" : " 当前未返回已识别的能力声明。")
        + "此状态不是实时上游健康探测。"
      : "本次真实注册清单中没有名为 hithink 的 Provider；不能据此判断 API Key 是否配置。";
  }
  setBadge("#data-center-provider-status", "unknown", "已读取能力声明");
  setText("#data-center-provider-summary", sources.length
    ? "已读取 " + sources.length + " 个 Provider 注册记录；此清单不执行上游实时探测。"
    : "服务响应成功，但未返回已注册 Provider。空清单不代表所有数据源都发生故障。");

  if (!sources.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "后端未返回 Provider 记录。";
    root.append(empty);
    return;
  }

  sources.forEach((source) => {
    const row = document.createElement("article");
    row.className = "data-center-provider-row";
    const top = document.createElement("div");
    top.className = "data-center-provider-top";
    const title = document.createElement("strong");
    title.textContent = source.name || "未命名 Provider";
    const state = document.createElement("span");
    state.className = "data-center-capability-status";
    state.textContent = source.enabled ? "已启用 · 未实测" : "已禁用";

    top.append(title, state);
    const capabilities = CAPABILITIES.filter(([key]) => source[key] === true);
    const detail = document.createElement("small");
    detail.textContent = capabilities.length
      ? "声明支持 " + capabilities.length + " 项能力；实时健康尚未探测。"
      : "没有声明当前清单支持的能力；不据此推断整个服务故障。";
    const list = document.createElement("div");
    list.className = "data-center-capability-list";

    if (capabilities.length) {
      capabilities.forEach(([, label]) => {
        const tag = document.createElement("span");
        tag.className = "data-center-capability-tag";
        tag.textContent = label;
        list.append(tag);
      });
    } else {
      const tag = document.createElement("span");
      tag.className = "data-center-capability-tag";
      tag.textContent = "未声明能力";
      list.append(tag);
    }
    row.append(top, detail, list);
    root.append(row);
  });
}

function renderDependency(selector, detailSelector, dependencies, name, analysisReady) {
  const hasValue = dependencies && Object.prototype.hasOwnProperty.call(dependencies, name);
  const value = hasValue ? String(dependencies[name]).toLowerCase() : "";
  if (!analysisReady || !value) {
    setBadge(selector, "unknown", "未知");
    setText(detailSelector, analysisReady
      ? "分析服务未返回此依赖的独立状态。"
      : "分析服务就绪状态不可读取，当前无法确认依赖健康。");
    return "unknown";
  }

  if (["ok", "healthy", "ready", "connected"].includes(value)) {
    setBadge(selector, "healthy", "正常");
    setText(detailSelector, "分析服务就绪接口报告：" + value);
    return "healthy";
  }
  if (["failed", "unhealthy", "error", "disconnected", "unavailable"].includes(value)) {
    setBadge(selector, "unhealthy", "异常");
    setText(detailSelector, "分析服务就绪接口报告：" + value);
    return "failed";
  }

  setBadge(selector, "unknown", "未知：" + value);
  setText(detailSelector, "后端返回未归类状态；需结合服务日志进一步诊断。");
  return "unknown";
}


const DIAGNOSTIC_CAPABILITY_LABELS = Object.freeze({
  quote: "实时行情",
  daily_bars: "日 K",
  index_quotes: "指数行情",
  market_breadth: "市场广度",
  sector_rankings: "板块排行"
});

const DIAGNOSTIC_STATUS = Object.freeze({
  not_observed: "尚无业务观测",
  healthy: "正常",
  degraded: "降级",
  open: "熔断开启",
  half_open: "半开探测"
});

function renderProviderDiagnostics(payload) {
  const root = $("#data-center-diagnostics-list");
  if (!root) return;
  const items = Array.isArray(payload?.items)
    ? payload.items.filter((item) => item && typeof item === "object") : [];
  const scope = Array.isArray(payload?.scope)
    ? payload.scope.map((value) => String(value || "").trim()).filter(Boolean) : [];
  root.replaceChildren();

  if (!items.length) {
    setBadge("#data-center-diagnostics-status", "unknown", "暂无观测记录");
    setText("#data-center-diagnostics-summary", "诊断接口已响应，但当前进程没有返回 Provider 观测记录；这不代表 Provider 故障。");
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "当前没有可展示的运行诊断记录。";
    root.append(empty);
    return;
  }

  const counts = { healthy: 0, degraded: 0, open: 0, half_open: 0, not_observed: 0 };
  let capabilityCount = 0;
  items.forEach((provider) => {
    const row = document.createElement("article");
    row.className = "data-center-diagnostic-provider";
    const heading = document.createElement("div");
    heading.className = "data-center-diagnostic-provider-heading";
    const name = document.createElement("strong");
    name.textContent = String(provider.provider || "未命名 Provider");
    const capabilities = Array.isArray(provider.capabilities)
      ? provider.capabilities.filter((item) => item && typeof item === "object") : [];
    const summary = document.createElement("small");
    summary.textContent = capabilities.length + " 项已纳入诊断的能力";
    heading.append(name, summary);
    const list = document.createElement("div");
    list.className = "data-center-diagnostic-capabilities";

    capabilities.forEach((capability) => {
      const status = Object.hasOwn(DIAGNOSTIC_STATUS, capability.status)
        ? capability.status : "not_observed";
      counts[status] += 1;
      capabilityCount += 1;
      const item = document.createElement("div");
      item.className = "data-center-diagnostic-capability";
      const top = document.createElement("div");
      top.className = "data-center-diagnostic-capability-heading";
      const label = document.createElement("strong");
      const capabilityName = String(capability.capability || "");\n      label.textContent = DIAGNOSTIC_CAPABILITY_LABELS[capabilityName] || capabilityName || "未命名能力";
      const state = document.createElement("span");
      state.className = "data-center-diagnostic-state is-" + status.replace(/_/g, "-");
      state.textContent = DIAGNOSTIC_STATUS[status];
      top.append(label, state);

      const stats = document.createElement("small");
      stats.textContent = "累计成功 " + Number(capability.total_success || 0)
        + " · 累计失败 " + Number(capability.total_failure || 0)
        + " · 连续失败 " + Number(capability.consecutive_failures || 0);
      const times = document.createElement("small");
      const timeParts = [];
      if (capability.last_success_at) timeParts.push("最近成功 " + formatDateTime(capability.last_success_at));
      if (capability.last_failure_at) timeParts.push("最近失败 " + formatDateTime(capability.last_failure_at));
      if (capability.open_until) timeParts.push("熔断截止 " + formatDateTime(capability.open_until));
      times.textContent = timeParts.length ? timeParts.join(" · ") : "尚无成功/失败时间记录";
      item.append(top, stats, times);
      list.append(item);
    });

    if (!capabilities.length) {
      const empty = document.createElement("small");
      empty.textContent = "此 Provider 没有返回能力诊断记录。";
      list.append(empty);
    }
    row.append(heading, list);
    root.append(row);
  });

  const unhealthy = counts.open + counts.degraded + counts.half_open;
  const unobserved = counts.not_observed;
  const tone = unhealthy ? "degraded" : unobserved || !capabilityCount ? "unknown" : "healthy";
  setBadge("#data-center-diagnostics-status", tone,
    unhealthy ? "存在降级/熔断" : unobserved ? "部分能力尚无观测" : "已读取诊断");
  setText("#data-center-diagnostics-summary", [
    "Provider " + items.length + " 个",
    "能力 " + capabilityCount + " 项",
    "正常 " + counts.healthy,
    "降级 " + counts.degraded,
    "熔断 " + counts.open,
    "半开 " + counts.half_open,
    "未观测 " + counts.not_observed,
    scope.length ? "范围 " + scope.join("、") : "",
    payload?.observed_from ? "来源 " + String(payload.observed_from) : ""
  ].filter(Boolean).join(" · "));
}

function showProviderDiagnosticsFailure(result) {
  const root = $("#data-center-diagnostics-list");
  setBadge("#data-center-diagnostics-status", result?.skipped ? "unknown" : "failed",
    result?.skipped ? "需要行情服务 API Key" : "读取失败");
  setText("#data-center-diagnostics-summary", result?.skipped
    ? "配置 WebUI 访问行情服务所需的 API Key 后，仅读取服务端已有的进程内统计；不会测试同花顺 API Key 或请求上游。"
    : "只读运行诊断接口读取失败：" + safeError(result?.error));
  if (root) {
    root.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = result?.skipped
      ? "未执行诊断请求，不代表 Provider 不可用。"
      : "诊断记录暂不可读；不据此推断 Provider 当前健康状态。";
    root.append(empty);
  }
}

export function bindDataCenter(getSettings) {
  let checking = false;
  let generation = 0;
  const button = $("#data-center-check");

  async function refresh() {
    if (checking) return;
    checking = true;
    const current = ++generation;
    let settings = null;
    try {
      settings = typeof getSettings === "function" ? getSettings() : null;
      try {
        if (button) {
          button.disabled = true;
          button.textContent = "检查中…";
          button.setAttribute("aria-busy", "true");
        }
      } catch {
        // 按钮 DOM 更新失败不能阻止服务探测。
      }
      try {
        setBadge("#data-center-summary-status", "unknown", "检查中");
        setText("#data-center-summary-message", "正在检查服务存活/就绪状态与 Provider 能力清单；单项失败不会中断其他检查。");
      } catch {
        // 初始提示失败不能阻止检查请求，也不能跳过最终解锁。
      }
      try {
        setBadge("#data-center-provider-status", "unknown", "读取中");
        setText("#data-center-provider-summary", "正在读取当前连接配置的 Provider 能力清单；旧清单暂不展示。");
        const providerList = $("#data-center-provider-list");
        if (providerList) {
          providerList.replaceChildren();
          const empty = document.createElement("div");
          empty.className = "empty-row";
          empty.textContent = "正在读取 Provider 能力清单…";
          providerList.append(empty);
        }
      } catch {
        // Provider 加载占位失败不能阻止健康探针和 Provider 请求。
      }

      const checks = await Promise.all([
        runHealthCheck(checkWebUI, settings),
        runHealthCheck(marketLiveness, settings),
        runHealthCheck(marketHealth, settings),
        runHealthCheck(analysisLiveness, settings),
        runHealthCheck(analysisHealth, settings),
        settings?.apiKey
          ? runHealthCheck(getDataSources, settings)
          : Promise.resolve({ ok: false, skipped: true, error: new Error("需先配置行情 API Key"), durationMs: 0 }),
        settings?.apiKey
          ? runHealthCheck(getDataSourceDiagnostics, settings)
          : Promise.resolve({ ok: false, skipped: true, error: new Error("需先配置行情 API Key"), durationMs: 0 })
      ]);
      if (current !== generation) return;

      const [webUi, marketLive, marketReady, analysisLive, analysisReady, sources, diagnostics] = checks;
      const webUiState = probeState(webUi);
      setBadge("#data-service-webui-status", webUiState, webUiState === "healthy" ? "正常" : statusWord(webUiState));
      setText("#data-service-webui-detail", webUi.ok
        ? "当前页面路径请求成功 · " + webUi.durationMs + " ms · " + formatDateTime(new Date().toISOString())
        : "当前页面路径检查失败 · " + safeError(webUi.error));
      const marketLiveState = showCheck("#data-service-market-live", marketLive);
      const marketReadyState = showCheck("#data-service-market-ready", marketReady, { ready: true });
      const analysisLiveState = showCheck("#data-service-analysis-live", analysisLive);
      const analysisReadyState = showCheck("#data-service-analysis-ready", analysisReady, { ready: true });
      const timestamp = formatDateTime(new Date().toISOString());

      setDetail("#data-service-market-detail", [
        "检查时间 " + timestamp,
        "存活 " + (marketLive.ok ? marketLive.durationMs + " ms" : safeError(marketLive.error)),
        "就绪 " + (marketReady.ok ? marketReady.durationMs + " ms" : safeError(marketReady.error))
      ]);
      setDetail("#data-service-analysis-detail", [
        "检查时间 " + timestamp,
        "存活 " + (analysisLive.ok ? analysisLive.durationMs + " ms" : safeError(analysisLive.error)),
        "就绪 " + (analysisReady.ok ? analysisReady.durationMs + " ms" : safeError(analysisReady.error))
      ]);

      const dependencies = analysisReady.ok && analysisReady.data && typeof analysisReady.data.dependencies === "object"
        ? analysisReady.data.dependencies : null;
      const mariadb = renderDependency("#data-service-mariadb-status", "#data-service-mariadb-detail", dependencies, "mariadb", analysisReady.ok);
      const redis = renderDependency("#data-service-redis-status", "#data-service-redis-detail", dependencies, "redis", analysisReady.ok);

      if (sources.ok) {
        renderProviderCapabilities(sources.data);
      } else if (sources.skipped) {
        setBadge("#data-center-provider-status", "unknown", "需要 API Key");
        setText("#data-center-provider-summary", "基础健康检查已执行；配置行情 API Key 后才能读取 Provider 能力清单。");
        const root = $("#data-center-provider-list");
        root.replaceChildren();
        const empty = document.createElement("div");
        empty.className = "empty-row";
        empty.textContent = "未执行能力清单请求，不代表 Provider 不可用。";
        root.append(empty);
      } else {
        setBadge("#data-center-provider-status", "failed", "读取失败");
        setText("#data-center-provider-summary", "Provider 能力清单读取失败：" + safeError(sources.error));
        const root = $("#data-center-provider-list");
        root.replaceChildren();
        const empty = document.createElement("div");
        empty.className = "empty-row";
        empty.textContent = "仅 Provider 清单请求失败；服务健康状态与其它检查结果仍保留。";
        root.append(empty);
      }

      if (diagnostics.ok) {
        renderProviderDiagnostics(diagnostics.data);
      } else {
        showProviderDiagnosticsFailure(diagnostics);
      }

      const serviceStates = [webUiState, marketLiveState, marketReadyState, analysisLiveState, analysisReadyState];
      const healthyServices = serviceStates.filter((status) => status === "healthy").length;
      const failedServices = serviceStates.filter((status) => status === "failed" || status === "timeout").length;
      const degradedServices = serviceStates.filter((status) => status === "degraded").length;
      const unknownServices = serviceStates.filter((status) => status === "unknown").length;
      const dependencyStates = [mariadb, redis];
      const healthyDependencies = dependencyStates.filter((status) => status === "healthy").length;
      const failedDependencies = dependencyStates.filter((status) => status === "failed").length;
      const unknownDependencies = dependencyStates.filter((status) => status === "unknown").length;
      const providerUnprobed = sources.ok && Array.isArray(sources.data?.sources)
        ? sources.data.sources.filter((source) => source && typeof source === "object" && source.enabled === true).length
        : 0;
      const endpointOrDependencyFailures = failedServices + failedDependencies + degradedServices;
      const providerReadFailed = !sources.ok && !sources.skipped;
      const unknown = unknownServices + unknownDependencies + (sources.skipped ? 1 : 0) + providerUnprobed;
      const failed = endpointOrDependencyFailures + (providerReadFailed ? 1 : 0);
      setText("#data-center-count-healthy", healthyServices + healthyDependencies);
      setText("#data-center-count-failed", failed);
      setText("#data-center-count-unknown", unknown);
      const allNormal = failed === 0 && unknown === 0;
      setBadge("#data-center-summary-status", allNormal ? "healthy" : failed ? "unhealthy" : "unknown",
        allNormal ? "基础检查完成" : failed ? "存在异常" : "部分状态未知");
      setBadge("#data-center-last-check", "unknown", "检查于 " + timestamp);
      const failureReasons = [];
      if (endpointOrDependencyFailures) failureReasons.push("服务端点或已明确报告的依赖异常/降级");
      if (providerReadFailed) failureReasons.push("Provider 能力清单读取失败");
      setText("#data-center-summary-message",
        failed
          ? "检查完成：" + failureReasons.join("；") + "。请查看对应错误摘要与部署日志；其它检查已独立完成。"
          : unknown
            ? "检查已完成，但有服务、依赖或 Provider 状态无法确认。不能仅凭分析服务不可达就断定 MariaDB/Redis 故障。"
            : "基础检查完成：当前页面、服务端点和已报告的依赖状态已检查。Provider 能力清单仍只是配置声明，未实测项目已计入未知数量。");
    } catch (error) {
      if (current !== generation) return;
      try {
        setBadge("#data-center-summary-status", "failed", "检查过程异常");
      } catch {}
      try {
        setText("#data-center-summary-message", safeError(error));
      } catch {
        // 故障提示不可写时，错误仍被捕获且 finally 会继续复位。
      }
    } finally {
      if (current === generation) {
        checking = false;
        try {
          if (button) {
            button.disabled = false;
            button.textContent = "重新检查";
            button.removeAttribute("aria-busy");
          }
        } catch {
          // 数据中心按钮不可写时也不能让 finally 产生新的 rejection。
        }
      }
    }
  }

  function invalidate(message = "连接配置已变更，之前的检查结果已作废。请重新检查。") {
    generation += 1;
    checking = false;
    try {
      if (button) {
        button.disabled = false;
        button.textContent = "立即检查";
        button.removeAttribute("aria-busy");
      }
    } catch {
      // 先递增 generation，旧请求不会覆盖新连接状态。
    }

    const safeInvalidateStep = (operation) => {
      try { operation(); } catch {}
    };
    safeInvalidateStep(() => setBadge("#data-center-summary-status", "unknown", "需重新检查"));
    safeInvalidateStep(() => setText("#data-center-summary-message", message));
    safeInvalidateStep(() => setBadge("#data-center-last-check", "unknown", "尚未检查"));

    [
      "#data-service-webui-status", "#data-service-market-live", "#data-service-market-ready",
      "#data-service-analysis-live", "#data-service-analysis-ready", "#data-service-mariadb-status",
      "#data-service-redis-status", "#data-center-provider-status"
    ].forEach((selector) => safeInvalidateStep(() => setBadge(selector, "unknown", "需重新检查")));
    [
      "#data-service-webui-detail", "#data-service-market-detail", "#data-service-analysis-detail",
      "#data-service-mariadb-detail", "#data-service-redis-detail"
    ].forEach((selector) => safeInvalidateStep(() => setText(selector, "连接配置已变更，尚未重新检查。")));
    safeInvalidateStep(() => setText("#data-center-provider-summary", "连接配置已变更，尚未重新读取 Provider 能力清单。"));
    safeInvalidateStep(() => setBadge("#data-center-diagnostics-status", "unknown", "需重新检查"));
    safeInvalidateStep(() => setText("#data-center-diagnostics-summary", "连接配置已变更，尚未重新读取运行诊断。"));
    safeInvalidateStep(() => {
      const providerList = $("#data-center-provider-list");
      if (providerList) providerList.replaceChildren();
      const diagnosticsList = $("#data-center-diagnostics-list");
      if (diagnosticsList) diagnosticsList.replaceChildren();
    });
    ["#data-center-count-healthy", "#data-center-count-failed", "#data-center-count-unknown"]
      .forEach((selector) => safeInvalidateStep(() => setText(selector, "—")));
  }
  button?.addEventListener("click", () => { void refresh(); });
  return { refresh, invalidate };
}

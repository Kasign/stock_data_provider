import { analysisHealth, analysisLiveness, getDataSources, marketHealth, marketLiveness } from "./api.js";
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

function probe(fn, settings, timeoutMs = 6000) {
  const controller = new AbortController();
  const started = performance.now();
  let timedOut = false;
  const timeout = window.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);

  return Promise.resolve()
    .then(() => fn(settings, controller.signal))
    .then((data) => timedOut
      ? { ok: false, error: new Error("检查超时"), timedOut: true, durationMs: Math.round(performance.now() - started) }
      : { ok: true, data, durationMs: Math.round(performance.now() - started) })
    .catch((error) => ({ ok: false, error, timedOut, durationMs: Math.round(performance.now() - started) }))
    .finally(() => window.clearTimeout(timeout));
}

function probeState(result, ready = false) {
  if (!result?.ok) return result?.timedOut ? "timeout" : "failed";
  const raw = String(result.data?.status || "").trim().toLowerCase();
  if (!raw) return "unknown";
  if (["ok", "ready", "healthy", "alive", "running"].includes(raw)) return "healthy";
  if (ready && ["degraded", "partial"].includes(raw)) return "degraded";
  return "degraded";
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

export function bindDataCenter(getSettings) {
  let checking = false;
  let generation = 0;
  const button = $("#data-center-check");

  async function refresh() {
    if (checking) return;
    checking = true;
    const current = ++generation;
    const settings = typeof getSettings === "function" ? getSettings() : null;
    if (button) {
      button.disabled = true;
      button.textContent = "检查中…";
      button.setAttribute("aria-busy", "true");
    }
    setBadge("#data-center-summary-status", "unknown", "检查中");
    setText("#data-center-summary-message", "正在检查服务存活/就绪状态与 Provider 能力清单；单项失败不会中断其他检查。");

    try {
      const checks = await Promise.all([
        probe(checkWebUI, settings),
        probe(marketLiveness, settings),
        probe(marketHealth, settings),
        probe(analysisLiveness, settings),
        probe(analysisHealth, settings),
        settings?.apiKey
          ? probe(getDataSources, settings)
          : Promise.resolve({ ok: false, skipped: true, error: new Error("需先配置行情 API Key"), durationMs: 0 })
      ]);
      if (current !== generation) return;

      const [webUi, marketLive, marketReady, analysisLive, analysisReady, sources] = checks;
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
      const unknown = unknownServices + unknownDependencies + (sources.skipped ? 1 : 0) + providerUnprobed;
      const failed = failedServices + failedDependencies + degradedServices + (!sources.ok && !sources.skipped ? 1 : 0);
      setText("#data-center-count-healthy", healthyServices + healthyDependencies);
      setText("#data-center-count-failed", failed);
      setText("#data-center-count-unknown", unknown);
      const allNormal = failed === 0 && unknown === 0;
      setBadge("#data-center-summary-status", allNormal ? "healthy" : failed ? "unhealthy" : "unknown",
        allNormal ? "基础检查完成" : failed ? "存在异常" : "部分状态未知");
      setBadge("#data-center-last-check", "unknown", "检查于 " + timestamp);
      setText("#data-center-summary-message",
        failed
          ? "检查完成：发现服务端点或已明确报告的依赖异常/降级。请查看对应错误摘要与部署日志；其它检查已独立完成。"
          : unknown
            ? "检查已完成，但有服务、依赖或 Provider 状态无法确认。不能仅凭分析服务不可达就断定 MariaDB/Redis 故障。"
            : "基础检查完成：当前页面、服务端点和已报告的依赖状态已检查。Provider 能力清单仍只是配置声明，未实测项目已计入未知数量。");
    } catch (error) {
      setBadge("#data-center-summary-status", "failed", "检查过程异常");
      setText("#data-center-summary-message", safeError(error));
    } finally {
      checking = false;
      if (button) {
        button.disabled = false;
        button.textContent = "重新检查";
        button.removeAttribute("aria-busy");
      }
    }
  }

  button?.addEventListener("click", () => { void refresh(); });
  return { refresh };
}

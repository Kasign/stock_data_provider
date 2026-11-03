import { analysisHealth, analysisLiveness, getDataSources, getDataSourceDiagnostics, getDataSourceProbeHistory, marketHealth, marketLiveness, runDataSourceProbe } from "./api.js";
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
      const capabilityName = String(capability.capability || "");
      label.textContent = DIAGNOSTIC_CAPABILITY_LABELS[capabilityName] || capabilityName || "未命名能力";
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

const PROBE_STATUS_LABELS = Object.freeze({
  returned: "已返回", returned_degraded: "返回但质量降级", returned_unknown: "返回但质量未知",
  failed: "失败", skipped: "跳过", not_configured: "未配置", not_supported: "不支持"
});
const PROBE_RUN_STATUS_LABELS = Object.freeze({
  success: "探测完成", degraded: "存在降级/未知项", failed: "存在失败项",
  not_configured: "未配置可探测 Provider", no_capabilities_configured: "未配置探测能力"
});
const PROBE_CHECK_LABELS = Object.freeze({
  connectivity: "接口连通性", contract: "数据契约", quality: "数据质量",
  freshness: "数据新鲜度", cross_provider: "跨 Provider 对账", storage: "持久化存储"
});
const PROBE_CAPABILITY_LABELS = Object.freeze({
  quote: "实时行情", daily_bars: "日 K", stock_basic: "证券档案", stock_search: "股票搜索",
  stock_universe: "全市场证券目录", index_quotes: "指数行情", index_catalog: "指数目录",
  market_breadth: "市场广度", sector_rankings: "板块排行", market_capital_flow: "市场级资金流",
  limit_pool: "涨跌停池", dragon_tiger: "龙虎榜", announcements: "公告",
  trading_calendar: "交易日历", security_trading_status: "证券交易状态",
  price_limit_event: "涨跌幅事件", stock_boards: "所属板块", fundamentals: "财务指标",
  valuations: "估值指标", hot_ranking: "人气/飙升榜", capital_flow: "个股资金流",
  chip_distribution: "筹码分布"
});
const PROBE_ERROR_STAGE_LABELS = Object.freeze({
  timeout: "超时", cancellation: "取消", rate_limit: "限流", provider_auth: "Provider 认证",
  provider_permission: "Provider 权限", contract: "数据契约", connect: "连接",
  http: "HTTP", provider: "Provider", configuration: "配置"
});
const PROBE_CHECK_STATUS_LABELS = Object.freeze({
  passed: "通过", degraded: "降级", skipped: "跳过", failed: "失败",
  not_configured: "未配置", not_supported: "不支持"
});
const PROBE_FRESHNESS_LABELS = Object.freeze({
  fresh: "新鲜", stale: "已过期", unknown: "未知", not_applicable: "不适用"
});

function probeStatusTone(status) {
  if (["returned", "passed", "success"].includes(status)) return "healthy";
  if (["returned_degraded", "degraded"].includes(status)) return "degraded";
  if (status === "returned_unknown" || status === "unknown") return "unknown";
  if (status === "failed") return "failed";
  return "readonly";
}

function appendProbeText(parent, className, value) {
  if (value === null || value === undefined || value === "") return null;
  const node = document.createElement("small");
  if (className) node.className = className;
  node.textContent = String(value);
  parent.append(node);
  return node;
}

function renderProbeCheckResults(results, root) {
  if (!root) return;
  root.replaceChildren();
  const values = Array.isArray(results) ? results.filter((item) => item && typeof item === "object") : [];
  if (!values.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "服务端没有返回独立诊断维度结果；请以 Provider 能力明细为准。";
    root.append(empty);
    return;
  }
  values.forEach((result) => {
    const row = document.createElement("article");
    row.className = "data-center-diagnostic-capability";
    const top = document.createElement("div");
    top.className = "data-center-diagnostic-capability-heading";
    const label = document.createElement("strong");
    label.textContent = PROBE_CHECK_LABELS[result.check] || String(result.check || "未知诊断维度");
    const state = document.createElement("span");
    state.className = "data-center-diagnostic-state is-" + probeStatusTone(result.status);
    state.textContent = PROBE_CHECK_STATUS_LABELS[result.status] || String(result.status || "未知");
    top.append(label, state);
    row.append(top);
    const metadata = [
      result.provider ? "Provider " + result.provider : "",
      result.capability ? "能力 " + (PROBE_CAPABILITY_LABELS[result.capability] || result.capability) : "",
      result.checked_at ? "检查时间 " + formatDateTime(result.checked_at) : "",
      result.freshness_status ? "新鲜度 " + (PROBE_FRESHNESS_LABELS[result.freshness_status] || result.freshness_status) : "",
      result.age_seconds !== undefined ? "数据年龄 " + result.age_seconds + " 秒" : "",
      result.freshness_max_age_seconds !== undefined ? "最大允许年龄 " + result.freshness_max_age_seconds + " 秒" : "",
      result.market_phase ? "市场阶段 " + result.market_phase : "",
      result.freshness_domain ? "策略 " + result.freshness_domain : "",
      Array.isArray(result.compared_dates) && result.compared_dates.length ? "对账日期 " + result.compared_dates.join("、") : "",
      result.field_count !== undefined ? "字段 " + result.field_count : "",
      result.matched_field_count !== undefined ? "匹配 " + result.matched_field_count : "",
      result.mismatch_count !== undefined ? "不一致 " + result.mismatch_count : "",
      result.not_comparable_count !== undefined ? "不可比较 " + result.not_comparable_count : ""
    ].filter(Boolean);
    appendProbeText(row, "", metadata.join(" · "));
    if (result.error_code || result.error_message || result.freshness_reason) {
      appendProbeText(row, "data-center-diagnostic-error", [
        result.error_code || "", result.freshness_reason || "",
        result.error_message ? String(result.error_message).slice(0, 240) : ""
      ].filter(Boolean).join(" · "));
    }
    const comparisons = Array.isArray(result.field_comparisons) ? result.field_comparisons.slice(0, 30) : [];
    if (comparisons.length) {
      const evidence = document.createElement("div");
      evidence.className = "data-center-probe-evidence";
      const title = document.createElement("strong");
      title.textContent = "字段对账明细（最多展示 30 项）";
      evidence.append(title);
      comparisons.forEach((comparison) => {
        const line = document.createElement("div");
        line.className = "data-center-probe-comparison" + (comparison.status === "mismatch" ? " is-mismatch" : "");
        const field = document.createElement("span");
        field.textContent = String(comparison.field || "未命名字段") + " · "
          + ({ matched: "一致", mismatch: "不一致", not_comparable: "不可比较" }[comparison.status] || comparison.status || "未知");
        const values = [comparison.left, comparison.right, comparison.difference]
          .map((value) => value === null || value === undefined ? "—" : String(value));
        const numbers = document.createElement("span");
        numbers.textContent = "来源值 " + values[0] + " / " + values[1] + " · 差值 " + values[2];
        line.append(field, numbers);
        if (comparison.reason) appendProbeText(line, "", comparison.reason);
        evidence.append(line);
      });
      row.append(evidence);
    }
    root.append(row);
  });
}

const PROBE_COVERAGE_DIMENSION_LABELS = Object.freeze({
  quote_response: "行情响应覆盖率",
  price_classification: "价格分类覆盖率",
  quote_timestamp: "行情时间覆盖率",
  amount: "成交额覆盖率"
});

function formatProbeCount(value) {
  return Number.isInteger(value) && value >= 0 ? String(value) : "未知";
}

function renderProbeValidationEvidence(item, card) {
  const countKeys = [
    "validation_total_count", "validation_error_count",
    "validation_warning_count", "validation_info_count"
  ];
  const hasCounts = countKeys.some((key) => item[key] !== undefined && item[key] !== null);
  const issues = Array.isArray(item.validation_issues)
    ? item.validation_issues.filter((issue) => issue && typeof issue === "object") : [];
  if (!hasCounts && !issues.length && item.validation_truncated !== true) return;

  const total = formatProbeCount(item.validation_total_count);
  const errors = formatProbeCount(item.validation_error_count);
  const warnings = formatProbeCount(item.validation_warning_count);
  const infos = formatProbeCount(item.validation_info_count);
  const evidence = document.createElement("details");
  evidence.className = "data-center-probe-evidence data-center-probe-validation";
  const summary = document.createElement("summary");
  summary.textContent = "字段校验 " + total + " 项 · 错误 " + errors + " · 警告 " + warnings + " · 提示 " + infos;
  evidence.append(summary);

  if (issues.length) {
    const list = document.createElement("div");
    list.className = "data-center-probe-field-issues";
    const visibleIssues = issues.slice(0, 30);
    visibleIssues.forEach((issue) => {
      const line = document.createElement("div");
      const severity = String(issue.severity || "info");
      line.className = "data-center-probe-field-issue"
        + (severity === "error" ? " is-error" : severity === "warning" ? " is-warning" : "");
      const identity = [
        issue.field || "未知字段",
        issue.provider_field ? "原始字段 " + issue.provider_field : "",
        issue.stage ? "阶段 " + issue.stage : "",
        issue.code ? "规则 " + issue.code : "",
        issue.severity ? "级别 " + issue.severity : ""
      ].filter(Boolean);
      const context = [
        issue.expected ? "预期 " + issue.expected : "",
        issue.actual_type ? "实际类型 " + issue.actual_type : "",
        issue.actual_preview ? "实际值 " + String(issue.actual_preview).slice(0, 160) : "",
        issue.row_index !== undefined ? "行 " + issue.row_index : "",
        issue.symbol ? "证券 " + issue.symbol : "",
        issue.trade_date ? "交易日 " + issue.trade_date : ""
      ].filter(Boolean);
      line.textContent = identity.join(" · ") + (context.length ? "；" + context.join(" · ") : "")
        + (issue.message ? "；" + String(issue.message).slice(0, 200) : "");
      list.append(line);
    });
    evidence.append(list);
    if (issues.length > visibleIssues.length) {
      appendProbeText(evidence, "", "前端最多展示 30 条，另有 " + (issues.length - visibleIssues.length) + " 条未展开。");
    }
  }

  if (item.validation_truncated === true) {
    appendProbeText(evidence, "data-center-diagnostic-error",
      "服务端明细已截断；总问题数为 " + total + "，当前仅返回 " + issues.length + " 条明细。");
  } else if (hasCounts && Number.isInteger(item.validation_total_count) && item.validation_total_count > issues.length && issues.length) {
    appendProbeText(evidence, "", "当前返回 " + issues.length + " 条明细，问题总数为 " + total + "。");
  }
  card.append(evidence);
}

function renderProbeCoverageDimensions(item, card) {
  const dimensions = Array.isArray(item.coverage_dimensions)
    ? item.coverage_dimensions.filter((dimension) => dimension && typeof dimension === "object") : [];
  if (!dimensions.length) return;

  const evidence = document.createElement("div");
  evidence.className = "data-center-probe-evidence data-center-probe-coverage";
  const title = document.createElement("strong");
  title.textContent = "分项覆盖率证据";
  evidence.append(title);
  dimensions.forEach((dimension) => {
    const line = document.createElement("div");
    line.className = "data-center-probe-coverage-dimension";
    const label = PROBE_COVERAGE_DIMENSION_LABELS[dimension.dimension] || String(dimension.dimension || "未知维度");
    const ratio = typeof dimension.ratio === "number" && Number.isFinite(dimension.ratio)
      && dimension.ratio >= 0 && dimension.ratio <= 1
      ? (dimension.ratio * 100).toFixed(1) + "%" : "未知";
    const counts = [
      "分母 " + formatProbeCount(dimension.total),
      "返回 " + formatProbeCount(dimension.returned),
      "有效 " + formatProbeCount(dimension.validated),
      "缺失 " + formatProbeCount(dimension.missing),
      "排除 " + formatProbeCount(dimension.excluded)
    ];
    line.textContent = label + " · " + ratio + " · " + counts.join(" · ")
      + (dimension.population ? " · 范围 " + dimension.population : "")
      + (dimension.source ? " · 依据 " + dimension.source : "");
    evidence.append(line);
  });
  card.append(evidence);
}

function renderProbeItems(items, root) {
  if (!root) return;
  root.replaceChildren();
  const values = Array.isArray(items) ? items.filter((item) => item && typeof item === "object") : [];
  const groups = new Map();
  values.forEach((item) => {
    const provider = String(item.provider || "未命名 Provider");
    if (!groups.has(provider)) {
      const row = document.createElement("article");
      row.className = "data-center-diagnostic-provider";
      const heading = document.createElement("div");
      heading.className = "data-center-diagnostic-provider-heading";
      const name = document.createElement("strong");
      name.textContent = provider;
      const count = document.createElement("small");
      count.textContent = "探测结果";
      heading.append(name, count);
      const list = document.createElement("div");
      list.className = "data-center-diagnostic-capabilities";
      row.append(heading, list);
      root.append(row);
      groups.set(provider, { count, list, total: 0 });
    }
    const group = groups.get(provider);
    group.total += 1;
    group.count.textContent = group.total + " 项能力";
    const card = document.createElement("div");
    card.className = "data-center-diagnostic-capability";
    const top = document.createElement("div");
    top.className = "data-center-diagnostic-capability-heading";
    const label = document.createElement("strong");
    const capability = String(item.capability || "");
    label.textContent = PROBE_CAPABILITY_LABELS[capability] || capability || "未命名能力";
    const status = String(item.status || "skipped");
    const state = document.createElement("span");
    state.className = "data-center-diagnostic-state is-" + probeStatusTone(status);
    state.textContent = PROBE_STATUS_LABELS[status] || status || "未知";
    top.append(label, state);
    card.append(top);

    const metadata = [
      Number.isFinite(Number(item.elapsed_ms)) ? "耗时 " + Number(item.elapsed_ms) + " ms" : "",
      item.quality_status ? "质量 " + item.quality_status : "",
      item.http_status ? "HTTP " + item.http_status : "",
      item.sample_symbol ? "样本 " + item.sample_symbol : "",
      item.sample_market ? "市场 " + item.sample_market : "",
      item.sample_date ? "日期 " + item.sample_date : "",
      item.rows_returned !== undefined ? "返回行 " + item.rows_returned : "",
      item.rows_validated !== undefined ? "有效行 " + item.rows_validated : "",
      item.coverage_total !== undefined ? "覆盖分母 " + item.coverage_total : "",
      item.coverage_validated !== undefined ? "覆盖有效 " + item.coverage_validated : "",
      item.coverage_missing !== undefined ? "缺失 " + item.coverage_missing : "",
      item.coverage_excluded !== undefined ? "排除 " + item.coverage_excluded : "",
      item.coverage_ratio !== undefined && item.coverage_ratio !== null
        ? "总体覆盖率 " + (Number(item.coverage_ratio) * 100).toFixed(1) + "%" : "",
      item.coverage_ratio === null || (item.coverage_ratio === undefined && item.coverage_total === undefined) ? "覆盖率不可确认" : "",
      item.coverage_population ? "分母范围 " + item.coverage_population : "",
      item.coverage_source ? "分母依据 " + item.coverage_source : "",
      item.provider_time ? "源端时间 " + formatDateTime(item.provider_time) : "",
      item.provider_time_type ? "时间语义 " + item.provider_time_type : "",
      item.fetched_at ? "采集时间 " + formatDateTime(item.fetched_at) : "",
      item.checked_at ? "检查时间 " + formatDateTime(item.checked_at) : "",
      item.source_version ? "版本 " + item.source_version : ""
    ].filter(Boolean);
    appendProbeText(card, "", metadata.join(" · "));
    renderProbeValidationEvidence(item, card);
    renderProbeCoverageDimensions(item, card);
    if (Array.isArray(item.quality_reasons) && item.quality_reasons.length) {
      appendProbeText(card, "data-center-probe-evidence", "质量原因：" + item.quality_reasons.map((value) => String(value)).join("；"));
    }
    if (item.error_code || item.error_stage || item.error_message) {
      appendProbeText(card, "data-center-diagnostic-error", [
        item.error_code ? "错误码 " + item.error_code : "",
        item.error_stage ? "阶段 " + (PROBE_ERROR_STAGE_LABELS[item.error_stage] || item.error_stage) : "",
        item.error_message ? String(item.error_message).slice(0, 240) : ""
      ].filter(Boolean).join(" · "));
    }
    group.list.append(card);
  });
  if (!values.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "本批次没有返回 Provider 能力明细。";
    root.append(empty);
  }
}

function renderProviderProbeRun(payload) {
  const run = payload && typeof payload === "object" ? payload : {};
  const status = String(run.status || "unknown");
  const tone = status === "success" ? "healthy" : status === "degraded" ? "degraded"
    : status === "failed" ? "failed" : "unknown";
  setBadge("#data-center-probe-status", tone, PROBE_RUN_STATUS_LABELS[status] || "状态未知");
  setText("#data-center-probe-summary", [
    run.run_id ? "批次 " + run.run_id : "",
    "总计 " + Number(run.total || 0), "已返回 " + Number(run.returned || 0),
    "降级 " + Number(run.degraded || 0), "未知 " + Number(run.unknown || 0),
    "失败 " + Number(run.failed || 0), "跳过 " + Number(run.skipped || 0),
    "未配置 " + Number(run.not_configured || 0), "不支持 " + Number(run.not_supported || 0),
    run.started_at ? "开始 " + formatDateTime(run.started_at) : "",
    run.finished_at ? "结束 " + formatDateTime(run.finished_at) : "",
    run.persisted ? "已持久化" : "本次结果未持久化，历史记录不可恢复",
    run.timeout_seconds ? "单项超时 " + run.timeout_seconds + " 秒" : "",
    Array.isArray(run.requested_checks) && run.requested_checks.length
      ? "维度 " + run.requested_checks.map((value) => PROBE_CHECK_LABELS[value] || value).join("、") : ""
  ].filter(Boolean).join(" · "));
  renderProbeCheckResults(run.check_results, $("#data-center-probe-check-results"));
  renderProbeItems(run.items, $("#data-center-probe-list"));
}

function renderProviderProbeHistory(payload) {
  const root = $("#data-center-probe-history-list");
  if (!root) return;
  const runs = Array.isArray(payload?.items)
    ? payload.items.filter((item) => item && typeof item === "object") : [];
  root.replaceChildren();
  setBadge("#data-center-probe-history-status", "unknown", "已读取 " + runs.length + " 批");
  setText("#data-center-probe-history-summary",
    "服务端记录 " + Number(payload?.total || 0) + " 批；当前展示最近 " + runs.length + " 批。");
  if (!runs.length) {
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = "没有已持久化的探测批次。首次成功保存后会显示在这里。";
    root.append(empty);
    return;
  }
  runs.forEach((run) => {
    const details = document.createElement("details");
    details.className = "data-center-probe-history-item";
    const summary = document.createElement("summary");
    const title = document.createElement("strong");
    title.textContent = (run.run_id ? "#" + run.run_id + " · " : "")
      + (PROBE_RUN_STATUS_LABELS[run.status] || run.status || "状态未知");
    const counts = document.createElement("span");
    counts.textContent = Number(run.returned || 0) + "/" + Number(run.total || 0)
      + " 已返回 · 降级 " + Number(run.degraded || 0) + " · 失败 " + Number(run.failed || 0);
    summary.append(title, counts);
    const meta = document.createElement("p");
    meta.className = "data-center-probe-history-meta";
    meta.textContent = [
      run.started_at ? "开始 " + formatDateTime(run.started_at) : "",
      run.finished_at ? "结束 " + formatDateTime(run.finished_at) : "",
      run.persisted ? "已持久化" : "未持久化",
      run.scope ? "范围 " + (run.scope === "core" ? "核心能力" : "全部能力") : "",
      Array.isArray(run.requested_symbols) && run.requested_symbols.length ? "标的 " + run.requested_symbols.join("、") : "",
      Array.isArray(run.requested_markets) && run.requested_markets.length ? "市场 " + run.requested_markets.join("、") : ""
    ].filter(Boolean).join(" · ");
    const checkList = document.createElement("div");
    checkList.className = "data-center-probe-check-results";
    renderProbeCheckResults(run.check_results, checkList);
    const itemList = document.createElement("div");
    itemList.className = "data-center-probe-list";
    renderProbeItems(run.items, itemList);
    details.append(summary, meta, checkList, itemList);
    root.append(details);
  });
}

function showProviderProbeFailure(error, skipped = false) {
  setBadge("#data-center-probe-status", skipped ? "unknown" : "failed",
    skipped ? "需要行情服务 API Key" : "主动探测失败");
  setText("#data-center-probe-summary", skipped
    ? "未发起主动探测。请先配置行情服务 API Key；此凭据不是第三方 Provider API Key。"
    : "主动探测请求失败：" + safeError(error));
  renderProbeCheckResults([], $("#data-center-probe-check-results"));
  renderProbeItems([], $("#data-center-probe-list"));
}

function showProviderProbeHistoryFailure(error, skipped = false) {
  setBadge("#data-center-probe-history-status", skipped ? "unknown" : "failed",
    skipped ? "需要 API Key" : "历史读取失败");
  setText("#data-center-probe-history-summary", skipped
    ? "配置行情服务 API Key 后才能读取探测历史。"
    : "探测历史读取失败：" + safeError(error)
      + "；当前探测结果仍可能已返回，历史存储不可用不会覆盖本次结果。");
  const root = $("#data-center-probe-history-list");
  if (root) {
    root.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-row";
    empty.textContent = skipped ? "未执行历史读取。" : "历史暂不可读；不推断 Provider 健康状态。";
    root.append(empty);
  }
}

export function bindDataCenter(getSettings) {
  let checking = false;
  let generation = 0;
  let probing = false;
  let probeGeneration = 0;
  let probeController = null;
  let historyLoading = false;
  let historyGeneration = 0;
  let historyController = null;
  const button = $("#data-center-check");
  const probeButton = $("#data-center-probe-run");
  const historyButton = $("#data-center-probe-history-refresh");

  async function loadProbeHistory(settingsOverride = null, force = false) {
    if (historyLoading && !force) return false;
    let current = null;
    let controller = null;
    try {
      const settings = settingsOverride || (typeof getSettings === "function" ? getSettings() : null);
      if (!settings?.apiKey) {
        showProviderProbeHistoryFailure(new Error("需先配置行情服务 API Key"), true);
        return false;
      }
      historyLoading = true;
      current = ++historyGeneration;
      historyController?.abort();
      controller = new AbortController();
      historyController = controller;
      if (historyButton) {
        historyButton.disabled = true;
        historyButton.textContent = "读取中…";
        historyButton.setAttribute("aria-busy", "true");
      }
      setBadge("#data-center-probe-history-status", "unknown", "读取中");
      setText("#data-center-probe-history-summary", "正在读取最近 20 个已持久化探测批次…");
      const payload = await getDataSourceProbeHistory(settings, { limit: 20, signal: controller.signal });
      if (current !== historyGeneration) return false;
      if (!payload || typeof payload !== "object" || !Array.isArray(payload.items)
        || !Number.isInteger(Number(payload.total)) || Number(payload.total) < 0) {
        throw new Error("探测历史响应不符合 ProviderProbeHistory 契约。");
      }
      renderProviderProbeHistory(payload);
      return true;
    } catch (error) {
      if (controller?.signal.aborted || current !== historyGeneration) return false;
      showProviderProbeHistoryFailure(error);
      return false;
    } finally {
      if (current === historyGeneration) {
        historyLoading = false;
        if (historyButton) {
          historyButton.disabled = false;
          historyButton.textContent = "刷新历史";
          historyButton.removeAttribute("aria-busy");
        }
      }
    }
  }

  async function runProviderProbe() {
    if (probing) return false;
    let current = null;
    let controller = null;
    try {
      const settings = typeof getSettings === "function" ? getSettings() : null;
      if (!settings?.apiKey) {
        showProviderProbeFailure(new Error("需先配置行情服务 API Key"), true);
        return false;
      }
      const checks = Array.from(document.querySelectorAll('input[name="data-center-probe-check"]:checked'))
        .map((input) => input.value);
      if (!checks.length) {
        showProviderProbeFailure(new Error("至少选择一个诊断维度。"));
        return false;
      }
      const scope = $("#data-center-probe-scope")?.value || "all";
      const timeoutSeconds = Number($("#data-center-probe-timeout")?.value || 12);
      const market = $("#data-center-probe-market")?.value || "";
      const symbol = String($("#data-center-probe-symbol")?.value || "").trim().toUpperCase();
      const options = { scope, timeoutSeconds, checks };
      if (market) options.markets = [market];
      if (symbol) options.symbols = [symbol];
      probing = true;
      current = ++probeGeneration;
      probeController?.abort();
      controller = new AbortController();
      probeController = controller;
      if (probeButton) {
        probeButton.disabled = true;
        probeButton.textContent = "探测中…";
        probeButton.setAttribute("aria-busy", "true");
      }
      setBadge("#data-center-probe-status", "unknown", "探测中");
      setText("#data-center-probe-summary", "正在执行 " + (scope === "core" ? "核心能力" : "全部能力")
        + " 主动探测；单项超时 " + timeoutSeconds + " 秒。");
      renderProbeCheckResults([], $("#data-center-probe-check-results"));
      renderProbeItems([], $("#data-center-probe-list"));
      const payload = await runDataSourceProbe(settings, options, controller.signal);
      if (current !== probeGeneration) return false;
      const requiredCounts = ["total", "returned", "degraded", "unknown", "failed", "skipped", "not_configured", "not_supported"];
      if (!payload || typeof payload !== "object" || typeof payload.run_id !== "string"
        || !["success", "degraded", "failed", "not_configured", "no_capabilities_configured"].includes(payload.status)
        || typeof payload.persisted !== "boolean" || !Array.isArray(payload.items)
        || requiredCounts.some((key) => !Number.isInteger(Number(payload[key])))) {
        throw new Error("主动探测响应不符合 ProviderProbeRun 契约。");
      }
      renderProviderProbeRun(payload);
      if (payload.persisted) {
        await loadProbeHistory(settings, true);
      } else {
        setBadge("#data-center-probe-history-status", "unknown", "本次未持久化");
        setText("#data-center-probe-history-summary",
          "本次探测结果未写入历史存储；当前结果仍可查看，但刷新页面后无法从历史恢复。请检查 MariaDB 配置。");
      }
      return true;
    } catch (error) {
      if (controller?.signal.aborted || current !== probeGeneration) return false;
      showProviderProbeFailure(error);
      return false;
    } finally {
      if (current === probeGeneration) {
        probing = false;
        if (probeButton) {
          probeButton.disabled = false;
          probeButton.textContent = "开始主动探测";
          probeButton.removeAttribute("aria-busy");
        }
      }
    }
  }

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
      void loadProbeHistory(settings);

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
    probeGeneration += 1;
    historyGeneration += 1;
    probing = false;
    historyLoading = false;
    probeController?.abort();
    historyController?.abort();
    probeController = null;
    historyController = null;
    [[probeButton, "开始主动探测"], [historyButton, "刷新历史"]].forEach(([control, label]) => {
      if (!control) return;
      control.disabled = false;
      control.textContent = label;
      control.removeAttribute("aria-busy");
    });
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
    safeInvalidateStep(() => setBadge("#data-center-probe-status", "unknown", "需重新探测"));
    safeInvalidateStep(() => setText("#data-center-probe-summary", "连接配置已变更，之前的探测结果已作废。"));
    safeInvalidateStep(() => setBadge("#data-center-probe-history-status", "unknown", "需重新读取"));
    safeInvalidateStep(() => setText("#data-center-probe-history-summary", "连接配置已变更，尚未重新读取探测历史。"));
    safeInvalidateStep(() => {
      const providerList = $("#data-center-provider-list");
      if (providerList) providerList.replaceChildren();
      const diagnosticsList = $("#data-center-diagnostics-list");
      if (diagnosticsList) diagnosticsList.replaceChildren();
      const probeList = $("#data-center-probe-list");
      if (probeList) probeList.replaceChildren();
      const probeCheckList = $("#data-center-probe-check-results");
      if (probeCheckList) probeCheckList.replaceChildren();
      const probeHistoryList = $("#data-center-probe-history-list");
      if (probeHistoryList) probeHistoryList.replaceChildren();
    });
    ["#data-center-count-healthy", "#data-center-count-failed", "#data-center-count-unknown"]
      .forEach((selector) => safeInvalidateStep(() => setText(selector, "—")));
  }
  button?.addEventListener("click", () => { void refresh(); });
  probeButton?.addEventListener("click", () => { void runProviderProbe(); });
  historyButton?.addEventListener("click", () => { void loadProbeHistory(); });
  return { refresh, invalidate };
}

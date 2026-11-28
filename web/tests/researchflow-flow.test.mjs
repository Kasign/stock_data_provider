import test from "node:test";
import assert from "node:assert/strict";
import {
  QUANT_RESEARCH_TEMPLATE,
  DEFAULT_FACTOR_CODE,
  buildQuantResearchCommand,
  projectRunView,
  projectResultsView
} from "../src/researchflow-flow.js";

// ---- buildQuantResearchCommand ----

test("buildQuantResearchCommand 构造 1.1.0 CreateRun 命令：四 Stage 同一标的、幂等键只在 body", () => {
  const command = buildQuantResearchCommand({
    symbol: "600519.SH",
    idempotencyKey: "research-abc-1",
    window: { smaStartDate: "2026-10-01", smaEndDate: "2026-10-30", smaLimit: 20,
              factorStartDate: "2026-10-01", factorEndDate: "2026-10-30", factorLimit: 6,
              trendStartDate: "2026-09-01", trendEndDate: "2026-10-30", trendLimit: 61,
              riskTradeDate: "2026-11-25" }
  });
  assert.equal(command.schema_version, "1.1.0");
  assert.equal(command.idempotency_key, "research-abc-1");
  assert.equal(command.template_ref, QUANT_RESEARCH_TEMPLATE.ref);
  assert.equal(command.template_version, QUANT_RESEARCH_TEMPLATE.version);
  assert.ok(Array.isArray(command.input_snapshot_refs));
  assert.equal(command.input_snapshot_refs.length, 0);

  const params = JSON.parse(JSON.stringify(command.parameters));
  assert.deepEqual(Object.keys(params), ["stages"]);
  const stages = params.stages;
  assert.deepEqual(Object.keys(stages).sort(),
    ["quant.factor", "quant.risk", "quant.sma20", "quant.trend"]);
  const factor = stages["quant.factor"];
  assert.equal(factor.factor_code, DEFAULT_FACTOR_CODE);
  assert.equal(factor.symbol, "600519.SH");
  assert.equal(factor.limit, 6);
  assert.equal(stages["quant.sma20"].symbol, "600519.SH");
  assert.equal(stages["quant.sma20"].limit, 20);
  assert.equal(stages["quant.trend"].symbol, "600519.SH");
  assert.equal(stages["quant.trend"].limit, 61);
  assert.equal(stages["quant.risk"].symbol, "600519.SH");
  assert.equal(stages["quant.risk"].trade_date, "2026-11-25");
  assert.equal(stages["quant.risk"].start_date, undefined);
  // 无 HTTP header 概念：命令对象里不得出现 idempotency-key header 字段。
  assert.equal(command.headers, undefined);
  assert.equal(command["Idempotency-Key"], undefined);
});

test("buildQuantResearchCommand 拒绝非法标的格式（fail-closed，不发请求）", () => {
  for (const bad of ["", "600519", "600519.SH ", "abcdef", "600519.XY", "600519.SZ/../x", 123]) {
    assert.throws(() => buildQuantResearchCommand({ symbol: bad, idempotencyKey: "k" }),
      /标的格式/);
  }
});

test("buildQuantResearchCommand 拒绝空幂等键与超长键（body 幂等键契约）", () => {
  assert.throws(() => buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "" }), /幂等键/);
  assert.throws(() => buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "   " }), /幂等键/);
  assert.throws(() => buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "x".repeat(129) }), /幂等键/);
});

test("buildQuantResearchCommand 拒绝非法 factor_code（封闭分派，三值 enum）", () => {
  assert.throws(() => buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "k", factorCode: "bogus" }), /factor/);
  const ok = buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "k", factorCode: "ma20_distance" });
  assert.equal(JSON.parse(JSON.stringify(ok.parameters)).stages["quant.factor"].factor_code, "ma20_distance");
});

test("buildQuantResearchCommand 日期窗口缺省时提供保守默认，且不早于起始约束", () => {
  const command = buildQuantResearchCommand({ symbol: "000001.SZ", idempotencyKey: "k", asOfDate: "2026-11-27" });
  const stages = JSON.parse(JSON.stringify(command.parameters)).stages;
  assert.match(stages["quant.sma20"].start_date, /^\d{4}-\d{2}-\d{2}$/);
  assert.match(stages["quant.sma20"].end_date, /^\d{4}-\d{2}-\d{2}$/);
  assert.ok(stages["quant.sma20"].limit >= 20);
  assert.ok(stages["quant.trend"].limit >= 61);
  assert.ok(stages["quant.factor"].limit >= 6);
  assert.match(stages["quant.risk"].trade_date, /^\d{4}-\d{2}-\d{2}$/);
});

test("buildQuantResearchCommand 对显式窗口做范围校验（limit 下界 / 日期格式）", () => {
  assert.throws(() => buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "k",
    window: { smaLimit: 3 } }), /sma20/);
  assert.throws(() => buildQuantResearchCommand({ symbol: "600519.SH", idempotencyKey: "k",
    window: { trendStartDate: "2026/10/01" } }), /日期/);
});

// ---- projectRunView ----

const aggregate = (overrides = {}) => ({
  schema_version: "1.0.0",
  run: {
    run_ref: "run-1", status: "running", version: 3,
    system_as_of: "2026-11-27T08:00:00Z", deadline_at: "2026-11-27T08:05:00Z",
    template_ref: "quant.research", template_version: "1.0.0",
    template_fingerprint: "sha256:abc", input_snapshot_refs: [],
    result_summary_ref: null, correlation_id: "c-1", termination_reason: null,
    created_at: "2026-11-27T08:00:00Z", started_at: "2026-11-27T08:00:01Z", finished_at: null
  },
  stages: [
    { stage_ref: "s1", run_ref: "run-1", stage_code: "quant.sma20", status: "succeeded", version: 2,
      required: true, dependency_stage_refs: [], retry_policy_ref: "rp", retry_at: null,
      output_ref: "out-1", reason_codes: [], created_at: "2026-11-27T08:00:00Z",
      started_at: "2026-11-27T08:00:01Z", finished_at: "2026-11-27T08:00:03Z",
      quality_summary_ref: null, diagnostic_refs: [] },
    { stage_ref: "s2", run_ref: "run-1", stage_code: "quant.risk", status: "failed", version: 1,
      required: true, dependency_stage_refs: [], retry_policy_ref: "rp", retry_at: null,
      output_ref: null, reason_codes: ["risk_reject"], created_at: "2026-11-27T08:00:00Z",
      started_at: "2026-11-27T08:00:01Z", finished_at: "2026-11-27T08:00:04Z",
      quality_summary_ref: null, diagnostic_refs: [] }
  ],
  result_summary: null,
  ...overrides
});

test("projectRunView 投影 Run/Stage 聚合，标识终态与失败原因，不伪造事件", () => {
  const view = projectRunView(aggregate());
  assert.equal(view.runRef, "run-1");
  assert.equal(view.status, "running");
  assert.equal(view.terminal, false);
  assert.equal(view.stages.length, 2);
  assert.deepEqual(view.stages[0], {
    stageRef: "s1", stageCode: "quant.sma20", status: "succeeded",
    outputRef: "out-1", reasonCodes: [], required: true
  });
  assert.deepEqual(view.stages[1].reasonCodes, ["risk_reject"]);
  // 视图不含事件/Attempt 字段。
  assert.equal(view.events, undefined);
  assert.equal(view.taskEventCursor, undefined);
});

test("projectRunView 识别 succeeded/partial/failed/cancelled 为终态", () => {
  for (const status of ["succeeded", "partial", "failed", "cancelled"]) {
    const view = projectRunView(aggregate({ run: { ...aggregate().run, status } }));
    assert.equal(view.terminal, true, status);
  }
  for (const status of ["created", "queued", "running", "cancelling"]) {
    const view = projectRunView(aggregate({ run: { ...aggregate().run, status } }));
    assert.equal(view.terminal, false, status);
  }
});

test("projectRunView 拒绝无 run_ref / 未知状态的响应（契约违规 fail-closed）", () => {
  assert.throws(() => projectRunView({ run: { ...aggregate().run, run_ref: "" }, stages: [] }), /run_ref/);
  assert.throws(() => projectRunView({ run: { ...aggregate().run, status: "banana" }, stages: [] }), /状态/);
  assert.throws(() => projectRunView(null), /响应/);
});

test("projectRunView 拒绝跨 Run 的 Stage（身份隔离）", () => {
  const bad = aggregate();
  bad.stages = [{ ...bad.stages[0], run_ref: "another-run" }];
  assert.throws(() => projectRunView(bad), /run_ref/);
});

// ---- projectResultsView ----

const resultsResponse = (results = []) => ({
  schema_version: "1.1.0",
  run_ref: "run-1",
  resource_kind: "authorized_domain_results",
  results
});

const factorItem = {
  schema_version: "1.0.0",
  result_id: "res-1", result_type: "factor", scope: "600519.SH",
  system_as_of: "2026-11-27T08:00:00Z", quality_status: "valid",
  result_status: "available", authorization_ref: "auth-1", payload_schema_version: "1.0.0",
  payload: { indicator_code: "return", metrics: [{ metric_code: "return", value: 0.02, unit: "ratio",
    period: "5d", system_as_of: "2026-11-27T08:00:00Z", quality_status: "valid",
    metric_status: "available", calculation_version: "1.0.0", lower_bound: null, upper_bound: null }] }
};
const riskItem = {
  schema_version: "1.0.0",
  result_id: "res-2", result_type: "risk", scope: "600519.SH",
  system_as_of: "2026-11-27T08:00:00Z", quality_status: "unknown",
  result_status: "rejected", authorization_ref: "auth-2", payload_schema_version: "1.0.0"
};

test("projectResultsView 投影授权领域结果并明确标注非完整报告", () => {
  const view = projectResultsView(resultsResponse([factorItem, riskItem]));
  assert.equal(view.isFullReport, false);
  assert.equal(view.resourceKind, "authorized_domain_results");
  assert.equal(view.runRef, "run-1");
  assert.equal(view.items.length, 2);
  assert.equal(view.items[0].resultType, "factor");
  assert.equal(view.items[0].resultStatus, "available");
  assert.ok(view.items[0].payload);
  assert.equal(view.items[1].resultType, "risk");
  assert.equal(view.items[1].hasPayload, false);
  assert.equal(view.items[1].payload, undefined);
});

test("projectResultsView 拒绝非授权领域结果（resource_kind fail-closed）", () => {
  assert.throws(() => projectResultsView({ ...resultsResponse([]), resource_kind: "full_report" }), /resource_kind/);
  assert.throws(() => projectResultsView({ ...resultsResponse([]), schema_version: "2.0.0" }), /schema_version/);
  assert.throws(() => projectResultsView(null), /响应/);
});

test("projectResultsView 拒绝 risk 项携带 payload 与非四种 result_type", () => {
  assert.throws(() => projectResultsView(resultsResponse([{ ...riskItem, payload: {} }])), /risk/);
  assert.throws(() => projectResultsView(resultsResponse([{ ...factorItem, result_type: "news" }])), /result_type/);
});

test("projectResultsView 拒绝 indicator/factor/trend 缺 payload（fail-closed）", () => {
  const { payload, ...withoutPayload } = factorItem;
  assert.throws(() => projectResultsView(resultsResponse([withoutPayload])), /payload/);
});

test("projectResultsView 拒绝 result 身份字段缺失", () => {
  assert.throws(() => projectResultsView(resultsResponse([{ ...factorItem, result_id: "" }])), /result_id/);
  assert.throws(() => projectResultsView(resultsResponse([{ ...factorItem, authorization_ref: "" }])), /authorization_ref/);
});

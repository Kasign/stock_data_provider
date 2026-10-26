// Deterministic contract fixtures shaped from internal/analysis/domain/types.go.
// Synthetic values are for UI/export tests only; they are not live market data or recommendations.
const request = {
  task_type: "post_market_scan",
  as_of: "2026-10-23T09:30:00+08:00",
  as_of_mode: "realtime_snapshot",
  universe_id: "fixture-a-share",
  symbols: ["600519"],
  portfolio_id: "fixture-portfolio",
  strategy_ids: ["trend_following"],
  market_id: "CN-A",
  strategy_config_version: "strategy-v1",
  risk_config_version: "risk-v1",
  trading_rules_version: "rules-v1",
  data_policy_version: "data-v1"
};

const base = {
  request,
  started_at: "2026-10-23T09:30:01+08:00",
  finished_at: "2026-10-23T09:30:02+08:00",
  quality: {
    core_pass: true,
    partial: false,
    issues: [],
    strategy_eligible: { trend_following: true },
    strategy_blockers: {},
    checked_at: "2026-10-23T09:30:02+08:00",
    policy_version: "quality-v1"
  },
  market_state: {
    as_of: "2026-10-23T09:30:00+08:00",
    regime: "neutral",
    metrics: { breadth_ratio: 0.48 },
    evidence: ["合成测试证据，不代表真实市场判断"],
    algorithm_version: "market-regime-test-v1"
  },
  factors: [],
  signals: [],
  tradability: [],
  risk: [],
  plans: [],
  planning_rejections: [],
  execution_assessment: { model_version: "execution-test-v1", orders: [], warnings: [] },
  warnings: []
};

export const runResultFixtures = {
  completed: {
    ...base,
    run_id: "run-fixture-completed",
    status: "completed",
    factors: [{
      symbol: "600519",
      factor_id: "ma_distance",
      factor_version: "factor-v1",
      value: -0.012,
      unit: "ratio",
      as_of: "2026-10-23T09:30:00+08:00",
      input_quality: "valid",
      evidence_refs: ["snapshot:fixture-1"]
    }],
    signals: [{
      id: "signal-fixture-1",
      symbol: "600519",
      strategy_id: "trend_following",
      strategy_version: "strategy-v1",
      direction: "long",
      signal_at: "2026-10-23T09:30:00+08:00",
      valid_until: "2026-10-23T15:00:00+08:00",
      entry_rule: "合成测试规则：条件 A 成立",
      exit_rule: "合成测试规则：条件 B 成立",
      invalidation_rule: "合成测试规则：条件 C 成立",
      expected_hold_days: 3,
      reference_price: 1326,
      evidence_refs: ["snapshot:fixture-1"],
      attributes: { fixture: "true" }
    }],
    tradability: [{
      signal_id: "signal-fixture-1",
      allowed: true,
      reasons: [],
      rule_ids: ["liquidity-check"],
      rule_refs: [{ id: "liquidity-check", version: "v1" }]
    }],
    risk: [{
      signal_id: "signal-fixture-1",
      allowed: true,
      reasons: [],
      rule_ids: ["position-risk"],
      rule_refs: [{ id: "position-risk", version: "v1" }]
    }],
    plans: [{
      signal_id: "signal-fixture-1",
      symbol: "600519",
      side: "buy",
      quantity: 100,
      reference_price: 1326,
      stop_price: 1280,
      target_price: 1400,
      earliest_execute_at: "2026-10-23T09:31:00+08:00",
      expire_at: "2026-10-23T15:00:00+08:00",
      risk_budget: 4600,
      cost_estimate: 12,
      rule_version: "plan-v1"
    }],
    execution_assessment: {
      model_version: "execution-test-v1",
      orders: [{
        signal_id: "signal-fixture-1",
        status: "pending",
        requested_quantity: 100,
        filled_quantity: 0,
        remaining_quantity: 100,
        reason: "测试记录：没有连接或执行真实交易"
      }],
      warnings: ["这是测试 fixture，计划不代表实际委托"]
    },
    warnings: []
  },
  emptyCompleted: {
    ...base,
    run_id: "run-fixture-empty",
    status: "completed",
    factors: [],
    signals: [],
    tradability: [],
    risk: [],
    plans: [],
    planning_rejections: [],
    warnings: []
  },
  partial: {
    ...base,
    run_id: "run-fixture-partial",
    status: "partial",
    quality: {
      ...base.quality,
      partial: true,
      issues: [{
        code: "optional_dataset_missing",
        dataset: "daily_bars",
        symbol: "600519",
        field: "volume",
        severity: "warning",
        message: "可选日线字段缺失"
      }],
      strategy_eligible: { trend_following: false },
      strategy_blockers: { trend_following: ["可选数据不完整，策略资格未确认"] }
    },
    factors: [{
      symbol: "600519",
      factor_id: "volume_confirmation",
      factor_version: "factor-v1",
      unit: "ratio",
      as_of: "2026-10-23T09:30:00+08:00",
      input_quality: "missing",
      missing_reason: "上游未提供成交量字段",
      evidence_refs: []
    }],
    planning_rejections: [{
      signal_id: "signal-fixture-unconfirmed",
      reasons: ["所需因子缺失，未生成计划"]
    }],
    warnings: ["该场景用于测试部分数据与缺失字段呈现"]
  },
  blockedDataQuality: {
    ...base,
    run_id: "run-fixture-blocked",
    status: "blocked_data_quality",
    quality: {
      ...base.quality,
      core_pass: false,
      partial: false,
      issues: [{
        code: "required_quote_missing",
        dataset: "quote.realtime",
        symbol: "600519",
        field: "current_price",
        severity: "critical",
        message: "缺少必需的实时行情"
      }],
      strategy_eligible: { trend_following: false },
      strategy_blockers: { trend_following: ["缺少必需实时行情"] }
    },
    factors: [],
    signals: [],
    tradability: [],
    risk: [],
    plans: [],
    planning_rejections: [],
    warnings: ["核心数据质量门禁未通过"]
  },
  rejectedRequest: {
    ...base,
    run_id: "run-fixture-rejected",
    status: "rejected_request",
    request: { ...request, symbols: [] },
    failure: {
      code: "invalid_request",
      stage: "validate_request",
      message: "symbols 至少需要包含一个有效 A 股代码"
    },
    factors: [],
    signals: [],
    plans: [],
    warnings: ["请求在验证阶段被拒绝，未进入分析执行"]
  },
  failed: {
    ...base,
    run_id: "run-fixture-failed",
    status: "failed",
    failure: {
      code: "market_snapshot_failed",
      stage: "load_market_snapshot",
      message: "读取行情快照失败（合成错误）"
    },
    factors: [],
    signals: [],
    plans: [],
    warnings: ["本场景用于验证失败信息能被导出"]
  }
};

package risk

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func tradabilityInput(facts TradabilityFacts) TradabilityGateInput {
	return TradabilityGateInput{
		DecisionID:  "risk-decision-tradability-001",
		EvaluatedAt: "2026-11-26T01:00:00Z",
		SystemAsOf:  "2026-11-26T00:59:00Z",
		InputRefs:   []string{"daily_bars_close_series:sha256:tradability"},
		Facts:       facts,
	}
}

func tradabilityReason(t *testing.T, decision Decision) Reason {
	t.Helper()
	if len(decision.Reasons) != 1 {
		t.Fatalf("len(reasons) = %d, want 1", len(decision.Reasons))
	}
	return decision.Reasons[0]
}

// 可交易性事实不可用时必须 fail-closed 拒绝：上游未提供
// ST/停牌/涨跌停/可成交量，不得推导为默认放行。
func TestTradabilityGateRejectsWhenFactsUnavailable(t *testing.T) {
	decision, err := EvaluateTradabilityGate(context.Background(), tradabilityInput(TradabilityFacts{}))
	if err != nil {
		t.Fatalf("EvaluateTradabilityGate() error = %v", err)
	}
	if decision.Status != DecisionReject {
		t.Fatalf("status = %q, want reject", decision.Status)
	}
	if decision.RuleSetVersion != TradabilityRuleSetVersion {
		t.Fatalf("rule_set_version = %q, want %q", decision.RuleSetVersion, TradabilityRuleSetVersion)
	}
	reason := tradabilityReason(t, decision)
	if reason.Code != TradabilityGateCode || reason.Severity != SeverityBlock || reason.Status != ReasonTriggered {
		t.Fatalf("reason = %+v, want block+triggered %q", reason, TradabilityGateCode)
	}
	if !strings.Contains(reason.SafeMessage, "ST") || !strings.Contains(reason.SafeMessage, "无法确认可交易性") {
		t.Fatalf("safe_message = %q, want 说明缺失 ST/停牌等事实", reason.SafeMessage)
	}
	if reason.EvidenceRefs == nil {
		t.Fatal("evidence_refs must be present (may be empty)")
	}
}

func TestTradabilityGateReportsEveryMissingFact(t *testing.T) {
	facts := TradabilityFacts{MissingFacts: []TradabilityFact{
		TradabilityFactRiskWarning,
		TradabilityFactSuspension,
		TradabilityFactPriceLimit,
		TradabilityFactTradableVolume,
	}}
	decision, err := EvaluateTradabilityGate(context.Background(), tradabilityInput(facts))
	if err != nil {
		t.Fatalf("EvaluateTradabilityGate() error = %v", err)
	}
	if decision.Status != DecisionReject {
		t.Fatalf("status = %q, want reject", decision.Status)
	}
	message := tradabilityReason(t, decision).SafeMessage
	for _, want := range []string{"风险警示状态（ST）", "停牌状态", "涨跌停状态", "可成交量"} {
		if !strings.Contains(message, want) {
			t.Fatalf("safe_message = %q, want contains %q", message, want)
		}
	}
}

func TestTradabilityGatePassesOnlyWithVerifiedFactsAndEvidence(t *testing.T) {
	facts := TradabilityFacts{
		Verified:     true,
		EvidenceRefs: []string{"security_trading_status:sha256:001"},
	}
	decision, err := EvaluateTradabilityGate(context.Background(), tradabilityInput(facts))
	if err != nil {
		t.Fatalf("EvaluateTradabilityGate() error = %v", err)
	}
	if decision.Status != DecisionPass {
		t.Fatalf("status = %q, want pass", decision.Status)
	}
	reason := tradabilityReason(t, decision)
	if reason.Status != ReasonNotTriggered || reason.Severity != SeverityBlock {
		t.Fatalf("reason = %+v, want block+not_triggered", reason)
	}
	if !reflect.DeepEqual(reason.EvidenceRefs, facts.EvidenceRefs) {
		t.Fatalf("evidence_refs = %v, want %v", reason.EvidenceRefs, facts.EvidenceRefs)
	}
}

// 声明"已验证"但不给证据时不得 pass：这既是缺证据 fail-closed，也是防止
// 上游用一句布尔断言绕过硬门禁。
func TestTradabilityGateFailsClosedWhenVerifiedWithoutEvidence(t *testing.T) {
	decision, err := EvaluateTradabilityGate(context.Background(), tradabilityInput(TradabilityFacts{Verified: true}))
	if err != nil {
		t.Fatalf("EvaluateTradabilityGate() error = %v, want block+unknown decision", err)
	}
	if decision.Status != DecisionUnknown {
		t.Fatalf("status = %q, want unknown", decision.Status)
	}
	reason := tradabilityReason(t, decision)
	if reason.Severity != SeverityBlock || reason.Status != ReasonUnknown {
		t.Fatalf("reason = %+v, want block+unknown", reason)
	}
}

func TestTradabilityGateFailsClosedOnInvalidEvidenceRef(t *testing.T) {
	for name, facts := range map[string]TradabilityFacts{
		"blank ref":    {Verified: true, EvidenceRefs: []string{" "}},
		"padded ref":   {Verified: true, EvidenceRefs: []string{" ref "}},
		"invalid utf8": {Verified: true, EvidenceRefs: []string{string([]byte{0xff})}},
	} {
		t.Run(name, func(t *testing.T) {
			decision, err := EvaluateTradabilityGate(context.Background(), tradabilityInput(facts))
			if err != nil {
				t.Fatalf("EvaluateTradabilityGate() error = %v, want block+unknown decision", err)
			}
			if decision.Status == DecisionPass {
				t.Fatal("status = pass, want fail-closed")
			}
		})
	}
}

// 表外事实必须阻断而不是静默忽略，避免新增事实未接入时仍判为"已核验"。
func TestTradabilityGateFailsClosedOnUnknownFact(t *testing.T) {
	facts := TradabilityFacts{
		Verified:     true,
		EvidenceRefs: []string{"security_trading_status:sha256:001"},
		MissingFacts: []TradabilityFact{"unsupported_fact"},
	}
	decision, err := EvaluateTradabilityGate(context.Background(), tradabilityInput(facts))
	if err != nil {
		t.Fatalf("EvaluateTradabilityGate() error = %v, want block+unknown decision", err)
	}
	if decision.Status == DecisionPass {
		t.Fatal("status = pass, want fail-closed")
	}
	if decision.Status != DecisionUnknown {
		t.Fatalf("status = %q, want unknown", decision.Status)
	}
}

func TestTradabilityRuleSetIsValidAndBlockIsRequired(t *testing.T) {
	set := TradabilityRuleSet()
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(set.Rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(set.Rules))
	}
	if set.Rules[0].Severity != SeverityBlock || !set.Rules[0].Required {
		t.Fatalf("rule = %+v, want block + required", set.Rules[0])
	}
	// 规则集版本是契约身份，不能被随意改动而不同步 Decision。
	if set.Version != TradabilityRuleSetVersion {
		t.Fatalf("version = %q, want %q", set.Version, TradabilityRuleSetVersion)
	}
}

// 门禁输入刻意不承载状态、评分或模型建议：结构上没有覆盖入口，
// 任何新增字段都必须先经过本白名单审视。
func TestTradabilityGateInputHasNoOverrideChannel(t *testing.T) {
	allowed := map[string]struct{}{
		"DecisionID": {}, "EvaluatedAt": {}, "SystemAsOf": {}, "InputRefs": {}, "Facts": {},
	}
	typ := reflect.TypeOf(TradabilityGateInput{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, ok := allowed[name]; !ok {
			t.Fatalf("TradabilityGateInput 新增字段 %q：门禁输入不得承载状态/评分/模型建议，需先更新白名单与设计依据", name)
		}
	}
}

func TestEvaluateTradabilityGateRejectsInvalidDecisionIdentity(t *testing.T) {
	input := tradabilityInput(TradabilityFacts{})
	input.DecisionID = " "
	if _, err := EvaluateTradabilityGate(context.Background(), input); err == nil {
		t.Fatal("EvaluateTradabilityGate() error = nil, want decision identity rejection")
	}
}

package scenario

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validScenario() Scenario {
	metric, unit := "price.close", "CNY"
	return Scenario{
		ScenarioID: "scenario:600000.SH:breakout", ScenarioVersion: 1,
		SubjectRefs: []string{"600000.SH"},
		SystemAsOf: time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC),
		Horizon: "next_20_sessions",
		Conditions: []ConditionNode{{
			Condition: &ScenarioCondition{
				ConditionID: "close_above", ConditionType: ConditionPrice,
				Description: "收盘价高于前高", Operator: OperatorGT,
				MetricRef: &metric, ExpectedValue: float64(12.5), ExpectedValuePresent: true,
				Unit: &unit, EvaluationStatus: EvaluationUnknown, EvidenceRefs: []string{}, Required: true,
			},
		}},
		ExpectedPath: "价格延续突破", EvidenceRefs: []string{},
		ProbabilityStatus: ProbabilityNotEvaluated,
		Risks: []string{}, InvalidationConditions: []string{"收盘价跌破突破位"}, Limitations: []string{},
	}
}

func TestScenarioValidate(t *testing.T) {
	if err := validScenario().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestScenarioClosedConditionValueTypes(t *testing.T) {
	s := validScenario()
	s.Conditions[0].Condition.ExpectedValue = map[string]any{"x": true}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "scalar") {
		t.Fatalf("object value should fail: %v", err)
	}

	s = validScenario()
	s.Conditions[0].Condition.Operator = OperatorIn
	s.Conditions[0].Condition.ExpectedValue = []any{"a", true, float64(3)}
	if err := s.Validate(); err != nil {
		t.Fatalf("scalar array should pass: %v", err)
	}
	s.Conditions[0].Condition.ExpectedValue = []any{[]any{"nested"}}
	if err := s.Validate(); err == nil {
		t.Fatal("nested collection should fail")
	}
}

func TestScenarioConditionGroupRecursivelyValidates(t *testing.T) {
	s := validScenario()
	s.Conditions = []ConditionNode{{
		Group: &ConditionGroup{
			Operator: "all",
			Conditions: []ConditionNode{
				{Condition: s.Conditions[0].Condition},
				{Group: &ConditionGroup{
					Operator: "any",
					Conditions: []ConditionNode{{
						Condition: &ScenarioCondition{
							ConditionID: "event_exists", ConditionType: ConditionEvent, Description: "事件存在",
							Operator: OperatorExists, EvaluationStatus: EvaluationNotEvaluated,
							EvidenceRefs: []string{}, Required: false,
						},
					}},
				}},
			},
		},
	}}
	if err := s.Validate(); err != nil {
		t.Fatalf("nested groups should pass: %v", err)
	}
	s.Conditions[0].Group.Conditions[1].Group.Operator = "xor"
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "all or any") {
		t.Fatalf("bad group operator should fail: %v", err)
	}
}

func TestScenarioQuantReferenceRequiredForAvailableProbability(t *testing.T) {
	s := validScenario()
	s.ProbabilityStatus = ProbabilityAvailable
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "quant_evaluation_ref") {
		t.Fatalf("missing Quant ref should fail: %v", err)
	}
	s.QuantEvaluationRef = json.RawMessage(`{"result_id":"quant-result:1"}`)
	if err := s.Validate(); err != nil {
		t.Fatalf("valid opaque Quant ref should pass structural boundary: %v", err)
	}
}

func TestConditionNodeJSONRoundTrip(t *testing.T) {
	s := validScenario()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Scenario
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-tripped Scenario invalid: %v", err)
	}
}

func TestScenarioRejectsUnknownFields(t *testing.T) {
	var value Scenario
	if err := json.Unmarshal([]byte(`{"scenario_id":"x","unknown":true}`), &value); err == nil {
		t.Fatal("unknown Scenario field should fail")
	}
	var node ConditionNode
	if err := json.Unmarshal([]byte(`{"condition_id":"x","unknown":true}`), &node); err == nil {
		t.Fatal("unknown ScenarioCondition field should fail")
	}
}

func TestExistsOperatorRejectsExplicitNullExpectedValue(t *testing.T) {
	s := validScenario()
	s.Conditions[0].Condition.Operator = OperatorExists
	s.Conditions[0].Condition.ExpectedValue = nil
	s.Conditions[0].Condition.ExpectedValuePresent = true
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "must be omitted") {
		t.Fatalf("explicit null expected_value should fail: %v", err)
	}
}

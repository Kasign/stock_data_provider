package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type ProbabilityStatus string

const (
	ProbabilityAvailable          ProbabilityStatus = "available"
	ProbabilityInsufficientSample ProbabilityStatus = "insufficient_sample"
	ProbabilityUnknown            ProbabilityStatus = "unknown"
	ProbabilityNotEvaluated       ProbabilityStatus = "not_evaluated"
)

type ConditionType string

const (
	ConditionPrice       ConditionType = "price"
	ConditionVolume      ConditionType = "volume"
	ConditionIndicator   ConditionType = "indicator"
	ConditionEvent       ConditionType = "event"
	ConditionFundamental ConditionType = "fundamental"
	ConditionMarket      ConditionType = "market"
	ConditionIndustry    ConditionType = "industry"
	ConditionOther       ConditionType = "other"
)

type ConditionOperator string

const (
	OperatorGT        ConditionOperator = "gt"
	OperatorGTE       ConditionOperator = "gte"
	OperatorLT        ConditionOperator = "lt"
	OperatorLTE       ConditionOperator = "lte"
	OperatorEQ        ConditionOperator = "eq"
	OperatorNEQ       ConditionOperator = "neq"
	OperatorIn        ConditionOperator = "in"
	OperatorNotIn     ConditionOperator = "not_in"
	OperatorCrosses   ConditionOperator = "crosses"
	OperatorExists    ConditionOperator = "exists"
	OperatorNotExists ConditionOperator = "not_exists"
)

type EvaluationStatus string

const (
	EvaluationSatisfied    EvaluationStatus = "satisfied"
	EvaluationNotSatisfied EvaluationStatus = "not_satisfied"
	EvaluationUnknown      EvaluationStatus = "unknown"
	EvaluationNotEvaluated EvaluationStatus = "not_evaluated"
)

type Scenario struct {
	ScenarioID             string            `json:"scenario_id"`
	ScenarioVersion        int               `json:"scenario_version"`
	SubjectRefs            []string          `json:"subject_refs"`
	SystemAsOf             time.Time         `json:"system_as_of"`
	Horizon                string            `json:"horizon"`
	Conditions             []ConditionNode   `json:"conditions"`
	ExpectedPath           string            `json:"expected_path"`
	EvidenceRefs           []string          `json:"evidence_refs"`
	QuantEvaluationRef     json.RawMessage   `json:"quant_evaluation_ref,omitempty"`
	ProbabilityStatus      ProbabilityStatus `json:"probability_status"`
	Risks                  []string          `json:"risks"`
	InvalidationConditions []string          `json:"invalidation_conditions"`
	Limitations            []string          `json:"limitations"`
}

type ConditionNode struct {
	Condition *ScenarioCondition `json:"-"`
	Group     *ConditionGroup    `json:"-"`
}

type ConditionGroup struct {
	Operator   string          `json:"operator"`
	Conditions []ConditionNode `json:"conditions"`
}

type ScenarioCondition struct {
	ConditionID          string            `json:"condition_id"`
	ConditionType        ConditionType     `json:"condition_type"`
	Description          string            `json:"description"`
	Operator             ConditionOperator `json:"operator"`
	MetricRef            *string           `json:"metric_ref,omitempty"`
	ExpectedValue        any               `json:"expected_value,omitempty"`
	ExpectedValuePresent bool              `json:"-"`
	Unit                 *string           `json:"unit,omitempty"`
	Window               *string           `json:"window,omitempty"`
	EvaluationStatus     EvaluationStatus  `json:"evaluation_status"`
	EvidenceRefs         []string          `json:"evidence_refs"`
	Required             bool              `json:"required"`
}

func (n ConditionNode) MarshalJSON() ([]byte, error) {
	if (n.Condition == nil) == (n.Group == nil) {
		return nil, fmt.Errorf("condition node must contain exactly one condition or group")
	}
	if n.Condition != nil {
		return json.Marshal(n.Condition)
	}
	return json.Marshal(n.Group)
}

func (n *ConditionNode) UnmarshalJSON(data []byte) error {
	if n == nil {
		return fmt.Errorf("condition node is nil")
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		return err
	}
	if _, ok := shape["condition_id"]; ok {
		var value ScenarioCondition
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		n.Condition, n.Group = &value, nil
		return nil
	}
	var value ConditionGroup
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Condition, n.Group = nil, &value
	return nil
}

func (c *ScenarioCondition) UnmarshalJSON(data []byte) error {
	type conditionAlias ScenarioCondition
	var value conditionAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["expected_value"]; ok {
		value.ExpectedValuePresent = true
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			value.ExpectedValue = nil
		}
	}
	*c = ScenarioCondition(value)
	return nil
}

func (s Scenario) Validate() error {
	if err := required("scenario_id", s.ScenarioID); err != nil {
		return err
	}
	if s.ScenarioVersion < 1 {
		return fmt.Errorf("scenario_version must be >= 1")
	}
	if err := validateStrings("subject_refs", s.SubjectRefs); err != nil {
		return err
	}
	if err := utc(s.SystemAsOf); err != nil {
		return err
	}
	if err := required("horizon", s.Horizon); err != nil {
		return err
	}
	if len(s.Conditions) == 0 {
		return fmt.Errorf("conditions must be a non-empty array")
	}
	for i, node := range s.Conditions {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("conditions[%d]: %w", i, err)
		}
	}
	if err := required("expected_path", s.ExpectedPath); err != nil {
		return err
	}
	if err := validateStrings("evidence_refs", s.EvidenceRefs); err != nil {
		return err
	}
	if err := validateProbabilityStatus(s.ProbabilityStatus); err != nil {
		return err
	}
	if s.ProbabilityStatus == ProbabilityAvailable && len(s.QuantEvaluationRef) == 0 {
		return fmt.Errorf("quant_evaluation_ref is required when probability_status is available")
	}
	if len(s.QuantEvaluationRef) > 0 && (!json.Valid(s.QuantEvaluationRef) || bytes.TrimSpace(s.QuantEvaluationRef)[0] != '{') {
		return fmt.Errorf("quant_evaluation_ref must be a JSON object")
	}
	for name, values := range map[string][]string{
		"risks": s.Risks, "invalidation_conditions": s.InvalidationConditions, "limitations": s.Limitations,
	} {
		if values == nil {
			return fmt.Errorf("%s must be a non-null array", name)
		}
		for i, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s[%d] must not be blank", name, i)
			}
		}
	}
	return nil
}

func (n ConditionNode) Validate() error {
	if (n.Condition == nil) == (n.Group == nil) {
		return fmt.Errorf("exactly one condition or group is required")
	}
	if n.Condition != nil {
		return n.Condition.Validate()
	}
	return n.Group.Validate()
}

func (g ConditionGroup) Validate() error {
	if g.Operator != "all" && g.Operator != "any" {
		return fmt.Errorf("group operator must be all or any")
	}
	if len(g.Conditions) == 0 {
		return fmt.Errorf("group conditions must be a non-empty array")
	}
	for i, node := range g.Conditions {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("conditions[%d]: %w", i, err)
		}
	}
	return nil
}

func (c ScenarioCondition) Validate() error {
	if err := required("condition_id", c.ConditionID); err != nil {
		return err
	}
	switch c.ConditionType {
	case ConditionPrice, ConditionVolume, ConditionIndicator, ConditionEvent, ConditionFundamental, ConditionMarket, ConditionIndustry, ConditionOther:
	default:
		return fmt.Errorf("condition_type is invalid")
	}
	if err := required("description", c.Description); err != nil {
		return err
	}
	switch c.Operator {
	case OperatorGT, OperatorGTE, OperatorLT, OperatorLTE, OperatorEQ, OperatorNEQ, OperatorIn, OperatorNotIn, OperatorCrosses, OperatorExists, OperatorNotExists:
	default:
		return fmt.Errorf("operator is invalid")
	}
	needsMetric := c.ConditionType == ConditionPrice || c.ConditionType == ConditionVolume ||
		c.ConditionType == ConditionIndicator || c.ConditionType == ConditionFundamental ||
		c.ConditionType == ConditionMarket || c.ConditionType == ConditionIndustry
	if needsMetric {
		if c.MetricRef == nil || strings.TrimSpace(*c.MetricRef) == "" {
			return fmt.Errorf("metric_ref is required for this condition_type")
		}
		if c.Operator != OperatorExists && c.Operator != OperatorNotExists && (c.Unit == nil || strings.TrimSpace(*c.Unit) == "") {
			return fmt.Errorf("unit is required for metric comparisons")
		}
	}
	switch c.Operator {
	case OperatorExists, OperatorNotExists:
		if c.ExpectedValuePresent || c.ExpectedValue != nil {
			return fmt.Errorf("expected_value must be omitted for exists operators")
		}
	case OperatorIn, OperatorNotIn:
		if !validScalarCollection(c.ExpectedValue) {
			return fmt.Errorf("in/not_in expected_value must be a non-empty scalar array")
		}
	default:
		if !c.ExpectedValuePresent && c.ExpectedValue == nil {
			return fmt.Errorf("expected_value is required for this operator")
		}
		if !validScalar(c.ExpectedValue) {
			return fmt.Errorf("expected_value must be a string, number or boolean scalar")
		}
	}
	if err := validateEvaluationStatus(c.EvaluationStatus); err != nil {
		return err
	}
	return validateStrings("evidence_refs", c.EvidenceRefs)
}

func validScalar(value any) bool {
	switch v := value.(type) {
	case string, bool:
		return true
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0)
	case json.Number:
		parsed, err := strconv.ParseFloat(string(v), 64)
		return err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return false
	}
}

func validScalarCollection(value any) bool {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, item := range values {
		if !validScalar(item) {
			return false
		}
	}
	return true
}

func validateStrings(name string, values []string) error {
	if values == nil {
		return fmt.Errorf("%s must be a non-null array", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := required(name+" item", value); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func required(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s is required without surrounding whitespace", name)
	}
	return nil
}

func utc(value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("system_as_of is required")
	}
	_, offset := value.Zone()
	if offset != 0 || value.Location() != time.UTC {
		return fmt.Errorf("system_as_of must be UTC")
	}
	return nil
}

func validateProbabilityStatus(value ProbabilityStatus) error {
	switch value {
	case ProbabilityAvailable, ProbabilityInsufficientSample, ProbabilityUnknown, ProbabilityNotEvaluated:
		return nil
	default:
		return fmt.Errorf("probability_status is invalid")
	}
}

func validateEvaluationStatus(value EvaluationStatus) error {
	switch value {
	case EvaluationSatisfied, EvaluationNotSatisfied, EvaluationUnknown, EvaluationNotEvaluated:
		return nil
	default:
		return fmt.Errorf("evaluation_status is invalid")
	}
}

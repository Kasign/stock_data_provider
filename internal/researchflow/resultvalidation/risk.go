package resultvalidation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidRiskDecision      = errors.New("invalid Quant RiskDecision")
	ErrRiskPassContradictsBlock = errors.New("RiskDecision pass contradicts a blocking reason")
)

type RiskReason struct {
	Code            string          `json:"code"`
	Severity        string          `json:"severity"`
	Status          string          `json:"status"`
	SafeMessage     string          `json:"safe_message"`
	RuleRef         string          `json:"rule_ref"`
	EvidenceRefs    []string        `json:"evidence_refs"`
	ObservedValue   json.RawMessage `json:"observed_value,omitempty"`
	ExpectedRange   *string         `json:"expected_range,omitempty"`
	RemediationCode *string         `json:"remediation_code,omitempty"`
}

type RiskDecision struct {
	DecisionID     string       `json:"decision_id"`
	Status         string       `json:"status"`
	RuleSetVersion string       `json:"rule_set_version"`
	EvaluatedAt    time.Time    `json:"evaluated_at"`
	SystemAsOf     time.Time    `json:"system_as_of"`
	Reasons        []RiskReason `json:"reasons"`
	InputRefs      []string     `json:"input_refs"`
}

// ValidateRiskDecision prevents a model score or narrative from overriding a
// blocking/unknown hard-risk result. A pass is rejected if any blocking reason
// is triggered or unknown; the caller must retain reject/unknown as-is.
func ValidateRiskDecision(decision RiskDecision, expectedSystemAsOf time.Time) error {
	if strings.TrimSpace(decision.DecisionID) == "" || strings.TrimSpace(decision.RuleSetVersion) == "" ||
		(decision.Status != "pass" && decision.Status != "reject" && decision.Status != "unknown") ||
		!isUTC(decision.EvaluatedAt) || !isUTC(decision.SystemAsOf) ||
		expectedSystemAsOf.IsZero() || !isUTC(expectedSystemAsOf) ||
		!decision.SystemAsOf.Equal(expectedSystemAsOf) ||
		decision.Reasons == nil || decision.InputRefs == nil ||
		hasDuplicateOrInvalidRefs(decision.InputRefs) {
		return ErrInvalidRiskDecision
	}
	blockingUnresolved := false
	for _, reason := range decision.Reasons {
		if strings.TrimSpace(reason.Code) == "" || strings.TrimSpace(reason.RuleRef) == "" ||
			strings.TrimSpace(reason.SafeMessage) == "" || reason.EvidenceRefs == nil ||
			hasDuplicateOrInvalidRefs(reason.EvidenceRefs) || !validObservedValue(reason.ObservedValue) {
			return fmt.Errorf("%w: incomplete risk reason", ErrInvalidRiskDecision)
		}
		if reason.Severity != "block" && reason.Severity != "warn" && reason.Severity != "info" {
			return fmt.Errorf("%w: unsupported severity %q", ErrInvalidRiskDecision, reason.Severity)
		}
		if reason.Status != "triggered" && reason.Status != "not_triggered" && reason.Status != "unknown" {
			return fmt.Errorf("%w: unsupported reason status %q", ErrInvalidRiskDecision, reason.Status)
		}
		if reason.Severity == "block" && (reason.Status == "triggered" || reason.Status == "unknown") {
			blockingUnresolved = true
		}
	}
	if decision.Status == "pass" && blockingUnresolved {
		return ErrRiskPassContradictsBlock
	}
	return nil
}

func validObservedValue(value json.RawMessage) bool {
	if len(value) == 0 {
		return true
	}
	if !json.Valid(value) {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(value)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return false
	}
	switch decoded.(type) {
	case nil, string, json.Number:
		return true
	default:
		return false
	}
}

package resultvalidation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidRiskDecision      = errors.New("invalid Researchflow RiskDecision")
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

// RiskDecision is Researchflow’s composition-side risk input. It is not wire-compatible with QNT-CTR-004.
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
	if !validRiskText(decision.DecisionID) || !validRiskText(decision.RuleSetVersion) ||
		(decision.Status != "pass" && decision.Status != "reject" && decision.Status != "unknown") ||
		!isUTC(decision.EvaluatedAt) || !isUTC(decision.SystemAsOf) ||
		expectedSystemAsOf.IsZero() || !isUTC(expectedSystemAsOf) ||
		!decision.SystemAsOf.Equal(expectedSystemAsOf) ||
		len(decision.Reasons) == 0 || decision.InputRefs == nil ||
		hasDuplicateOrInvalidRefs(decision.InputRefs) || !validRiskTexts(decision.InputRefs) ||
		decision.EvaluatedAt.Before(decision.SystemAsOf) {
		return ErrInvalidRiskDecision
	}
	blockingUnresolved := false
	seenReasonCodes := make(map[string]struct{}, len(decision.Reasons))
	for _, reason := range decision.Reasons {
		if !validRiskText(reason.Code) || !validRiskText(reason.RuleRef) ||
			!validRiskText(reason.SafeMessage) || reason.EvidenceRefs == nil ||
			hasDuplicateOrInvalidRefs(reason.EvidenceRefs) || !validRiskTexts(reason.EvidenceRefs) ||
			!validObservedValue(reason.ObservedValue) {
			return fmt.Errorf("%w: incomplete risk reason", ErrInvalidRiskDecision)
		}
		if _, exists := seenReasonCodes[reason.Code]; exists {
			return fmt.Errorf("%w: duplicate risk reason code %q", ErrInvalidRiskDecision, reason.Code)
		}
		seenReasonCodes[reason.Code] = struct{}{}
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

// validRiskText 与 Quant risk.Decision 的 nonBlank 约束保持一致：
// 必须是有效 UTF-8、非空且不含首尾空白。
func validRiskText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}

func validRiskTexts(values []string) bool {
	for _, value := range values {
		if !validRiskText(value) {
			return false
		}
	}
	return true
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

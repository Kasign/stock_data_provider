package risk

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidDecision = errors.New("invalid risk decision")

type DecisionStatus string

const (
	DecisionPass    DecisionStatus = "pass"
	DecisionReject  DecisionStatus = "reject"
	DecisionUnknown DecisionStatus = "unknown"
)

type Severity string

const (
	SeverityBlock Severity = "block"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
)

type ReasonStatus string

const (
	ReasonTriggered    ReasonStatus = "triggered"
	ReasonNotTriggered ReasonStatus = "not_triggered"
	ReasonUnknown      ReasonStatus = "unknown"
)

// Reason 描述一条可审计的风险规则结论。
type Reason struct {
	Code         string       `json:"code"`
	Severity     Severity     `json:"severity"`
	Status       ReasonStatus `json:"status"`
	SafeMessage  string       `json:"safe_message"`
	EvidenceRefs []string     `json:"evidence_refs"`
}

// Decision 是 QNT-CTR-004 RiskDecision 的强类型表示。
// Validate 只校验 DTO 结构；具体规则与 fail-closed 决策由 QNT-010 Application 负责。
type Decision struct {
	DecisionID     string         `json:"decision_id"`
	Status         DecisionStatus `json:"status"`
	RuleSetVersion string         `json:"rule_set_version"`
	EvaluatedAt    string         `json:"evaluated_at"`
	SystemAsOf     string         `json:"system_as_of"`
	Reasons        []Reason       `json:"reasons"`
	InputRefs      []string       `json:"input_refs"`
}

// Validate 不推断 status 与 reasons 的业务组合，避免 DTO 层暗中实现风控规则。
func (d Decision) Validate() error {
	if !nonBlank(d.DecisionID) || !nonBlank(d.RuleSetVersion) ||
		!validDecisionStatus(d.Status) ||
		!canonicalUTCTimestamp(d.EvaluatedAt) ||
		!canonicalUTCTimestamp(d.SystemAsOf) ||
		d.Reasons == nil || d.InputRefs == nil {
		return ErrInvalidDecision
	}

	for _, ref := range d.InputRefs {
		if !nonBlank(ref) {
			return ErrInvalidDecision
		}
	}
	for _, reason := range d.Reasons {
		if !nonBlank(reason.Code) || !validSeverity(reason.Severity) ||
			!validReasonStatus(reason.Status) || !nonBlank(reason.SafeMessage) ||
			reason.EvidenceRefs == nil {
			return ErrInvalidDecision
		}
		for _, ref := range reason.EvidenceRefs {
			if !nonBlank(ref) {
				return ErrInvalidDecision
			}
		}
	}
	return nil
}

func nonBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

func canonicalUTCTimestamp(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || !strings.HasSuffix(value, "Z") {
		return false
	}
	return parsed.UTC().Format(time.RFC3339Nano) == value
}

func validDecisionStatus(value DecisionStatus) bool {
	switch value {
	case DecisionPass, DecisionReject, DecisionUnknown:
		return true
	default:
		return false
	}
}

func validSeverity(value Severity) bool {
	switch value {
	case SeverityBlock, SeverityWarn, SeverityInfo:
		return true
	default:
		return false
	}
}

func validReasonStatus(value ReasonStatus) bool {
	switch value {
	case ReasonTriggered, ReasonNotTriggered, ReasonUnknown:
		return true
	default:
		return false
	}
}

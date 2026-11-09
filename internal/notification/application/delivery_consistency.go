package application

import "example.com/stock-analysis-flow/internal/notification/domain"

type DeliveryConsistencyStatus string

const (
	DeliveryConsistencyPass           DeliveryConsistencyStatus = "pass"
	DeliveryConsistencyDegrade        DeliveryConsistencyStatus = "degrade"
	DeliveryConsistencyFail           DeliveryConsistencyStatus = "fail"
	DeliveryConsistencyNotComparable  DeliveryConsistencyStatus = "not_comparable"
)

// DeliveryConsistencyInput contains one caller-supplied read set.
// Comparable flags must be false when a history read failed, was skipped, or cannot
// be proven complete. This function does not claim the three inputs are an atomic DB snapshot.
type DeliveryConsistencyInput struct {
	Delivery               domain.DeliveryRecord
	Attempts               []domain.DeliveryAttempt
	AttemptsComparable     bool
	StateEvents            []domain.DeliveryStateEvent
	StateEventsComparable bool
}

// DeliveryConsistencyResult reports independent checks and the aggregate result.
// DeliveryConsistencyEvidence is a deliberately allowlisted, non-sensitive summary.
// It excludes delivery/recipient/channel IDs, evidence references, payloads, provider
// messages, actor references, request IDs, and raw errors.
type DeliveryConsistencyEvidence struct {
	DeliveryStatus       domain.DeliveryStatus       `json:"delivery_status"`
	DeliveryVersion      int64                       `json:"delivery_version"`
	ExpectedAttemptCount int                         `json:"expected_attempt_count"`
	ObservedAttemptCount int                         `json:"observed_attempt_count"`
	StateEventCount      int                         `json:"state_event_count"`
	LatestAttemptOutcome domain.DeliveryAttemptOutcome `json:"latest_attempt_outcome,omitempty"`
	LatestAttemptFinished bool                       `json:"latest_attempt_finished"`
	EvidenceRefCount     int                         `json:"evidence_ref_count"`
}

// DeliveryConsistencyResult is a safe diagnostic DTO; do not add raw domain records.
type DeliveryConsistencyResult struct {
	Status             DeliveryConsistencyStatus `json:"status"`
	AttemptStatus      DeliveryConsistencyStatus `json:"attempt_status"`
	StateHistoryStatus DeliveryConsistencyStatus `json:"state_history_status"`
	ReasonCodes        []string                   `json:"reason_codes,omitempty"`
	Evidence           DeliveryConsistencyEvidence `json:"evidence"`
}

// AssessDeliveryConsistency combines read-only snapshot/history checks.
// Aggregate precedence is fail > not_comparable > degrade > pass: a proven contradiction
// is never hidden by another unavailable source; missing evidence prevents a clean pass.
func AssessDeliveryConsistency(input DeliveryConsistencyInput) DeliveryConsistencyResult {
	result := DeliveryConsistencyResult{
		AttemptStatus:      DeliveryConsistencyNotComparable,
		StateHistoryStatus: DeliveryConsistencyNotComparable,
		Evidence: DeliveryConsistencyEvidence{
			DeliveryStatus: input.Delivery.Status,
			DeliveryVersion: input.Delivery.Version,
			ExpectedAttemptCount: input.Delivery.AttemptCount,
			ObservedAttemptCount: len(input.Attempts),
			StateEventCount: len(input.StateEvents),
		},
	}
	for _, attempt := range input.Attempts {
		if attempt.EvidenceRef != nil && *attempt.EvidenceRef != "" {
			result.Evidence.EvidenceRefCount++
		}
		if attempt.FinishedAt != nil {
			result.Evidence.LatestAttemptFinished = true
		}
		if attempt.Outcome != nil {
			result.Evidence.LatestAttemptOutcome = *attempt.Outcome
		}
	}
	for _, event := range input.StateEvents {
		if event.EvidenceRef != nil && *event.EvidenceRef != "" {
			result.Evidence.EvidenceRefCount++
		}
	}
	if err := input.Delivery.Validate(); err != nil {
		result.Status = DeliveryConsistencyFail
		result.ReasonCodes = append(result.ReasonCodes, "delivery_invalid")
		result.AttemptStatus = DeliveryConsistencyFail
		result.StateHistoryStatus = DeliveryConsistencyFail
		return result
	}

	if input.AttemptsComparable {
		consistency, err := AssessDeliveryAttemptConsistency(input.Delivery, input.Attempts)
		switch {
		case err != nil:
			result.AttemptStatus = DeliveryConsistencyFail
			result.ReasonCodes = append(result.ReasonCodes, "attempt_history_conflict")
		case consistency == DeliveryAttemptConsistencyIntermediate:
			result.AttemptStatus = DeliveryConsistencyDegrade
			result.ReasonCodes = append(result.ReasonCodes, "attempt_delivery_intermediate")
		default:
			result.AttemptStatus = DeliveryConsistencyPass
		}
	}

	if input.StateEventsComparable {
		if _, err := ValidateDeliveryStateHistory(input.Delivery, input.StateEvents); err != nil {
			result.StateHistoryStatus = DeliveryConsistencyFail
			result.ReasonCodes = append(result.ReasonCodes, "state_event_history_conflict")
		} else {
			result.StateHistoryStatus = DeliveryConsistencyPass
		}
	}

	result.Status = aggregateDeliveryConsistency(result.AttemptStatus, result.StateHistoryStatus)
	return result
}

func aggregateDeliveryConsistency(statuses ...DeliveryConsistencyStatus) DeliveryConsistencyStatus {
	hasDegrade := false
	hasNotComparable := false
	for _, status := range statuses {
		switch status {
		case DeliveryConsistencyFail:
			return DeliveryConsistencyFail
		case DeliveryConsistencyNotComparable:
			hasNotComparable = true
		case DeliveryConsistencyDegrade:
			hasDegrade = true
		}
	}
	if hasNotComparable {
		return DeliveryConsistencyNotComparable
	}
	if hasDegrade {
		return DeliveryConsistencyDegrade
	}
	return DeliveryConsistencyPass
}

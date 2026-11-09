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
type DeliveryConsistencyResult struct {
	Status             DeliveryConsistencyStatus
	AttemptStatus      DeliveryConsistencyStatus
	StateHistoryStatus DeliveryConsistencyStatus
	ReasonCodes        []string
}

// AssessDeliveryConsistency combines read-only snapshot/history checks.
// Aggregate precedence is fail > not_comparable > degrade > pass: a proven contradiction
// is never hidden by another unavailable source; missing evidence prevents a clean pass.
func AssessDeliveryConsistency(input DeliveryConsistencyInput) DeliveryConsistencyResult {
	result := DeliveryConsistencyResult{
		AttemptStatus:      DeliveryConsistencyNotComparable,
		StateHistoryStatus: DeliveryConsistencyNotComparable,
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

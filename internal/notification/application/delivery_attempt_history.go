package application

import (
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryAttemptHistory = errors.New("notification: delivery attempt history is inconsistent")

// ValidateDeliveryAttemptHistory 校验完整历史的序号、身份、租约 fencing 与前序结果。
// 允许最后一条 Attempt 仍在运行，以便租约恢复判断；任何前序 Attempt 必须已完成。
func ValidateDeliveryAttemptHistory(deliveryID string, expectedCount int, attempts []domain.DeliveryAttempt) error {
	if deliveryID == "" || expectedCount < 0 || len(attempts) != expectedCount {
		return ErrDeliveryAttemptHistory
	}

	seenIDs := make(map[string]struct{}, len(attempts))
	var previous *domain.DeliveryAttempt
	for i := range attempts {
		current := attempts[i]
		if current.Validate() != nil || current.DeliveryID != deliveryID || current.AttemptNo != i+1 {
			return ErrDeliveryAttemptHistory
		}
		if _, exists := seenIDs[current.AttemptID]; exists {
			return ErrDeliveryAttemptHistory
		}
		seenIDs[current.AttemptID] = struct{}{}

		if previous != nil {
			if current.FencingToken <= previous.FencingToken ||
				previous.FinishedAt == nil || previous.Outcome == nil ||
				current.StartedAt.Before(*previous.FinishedAt) {
				return ErrDeliveryAttemptHistory
			}
			switch *previous.Outcome {
			case domain.DeliveryAttemptRejected:
				if !previous.Retryable {
					return ErrDeliveryAttemptHistory
				}
			case domain.DeliveryAttemptNotSent:
			default:
				return ErrDeliveryAttemptHistory
			}
		}
		previous = &attempts[i]
	}
	return nil
}

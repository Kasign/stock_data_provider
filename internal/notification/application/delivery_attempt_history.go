package application

import (
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryAttemptHistory = errors.New("notification: delivery attempt history is inconsistent")

// ValidateDeliveryAttemptHistoryForDelivery 在完整历史校验之外，验证每次尝试均处于 Delivery 的有效生命周期内。
// 渠道请求可在截止时间前开始并在截止时间后结束，因此只限制 StartedAt 与租约上限，不限制 FinishedAt。 
func ValidateDeliveryAttemptHistoryForDelivery(delivery domain.DeliveryRecord, attempts []domain.DeliveryAttempt) error {
	if err := delivery.Validate(); err != nil || ValidateDeliveryAttemptHistory(delivery.DeliveryID, delivery.AttemptCount, attempts) != nil {
		return ErrDeliveryAttemptHistory
	}
	for i := range attempts {
		attempt := attempts[i]
		if attempt.StartedAt.Before(delivery.CreatedAt) || !attempt.StartedAt.Before(delivery.DeadlineAt) ||
			attempt.LeaseExpiresAt.After(delivery.DeadlineAt) {
			return ErrDeliveryAttemptHistory
		}
	}
	return nil
}

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

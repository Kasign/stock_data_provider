package application

import (
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryAttemptConsistency = errors.New("notification: delivery status conflicts with attempt history")

// DeliveryAttemptConsistency 描述 Delivery 快照与 Attempt 历史的关系。
type DeliveryAttemptConsistency string

const (
	DeliveryAttemptConsistencyConsistent   DeliveryAttemptConsistency = "consistent"
	DeliveryAttemptConsistencyIntermediate DeliveryAttemptConsistency = "intermediate"
)

// AssessDeliveryAttemptConsistency 仅拒绝可由持久化事实明确证明的矛盾。
// sending + 已完成 Attempt 是 Attempt 先落库、Delivery CAS 尚未完成的合法中间态；
// sending + 无 Attempt 也可能是 Worker 在 StartWithLease 前退出。
func AssessDeliveryAttemptConsistency(delivery domain.DeliveryRecord, attempts []domain.DeliveryAttempt) (DeliveryAttemptConsistency, error) {
	if err := delivery.Validate(); err != nil {
		return "", err
	}
	if err := ValidateDeliveryAttemptHistoryForDelivery(delivery, attempts); err != nil {
		return "", ErrDeliveryAttemptConsistency
	}
	if len(attempts) == 0 {
		switch delivery.Status {
		case domain.DeliveryQueued, domain.DeliverySending, domain.DeliverySkipped, domain.DeliveryBlocked, domain.DeliveryFailed:
			return DeliveryAttemptConsistencyConsistent, nil
		default:
			return "", ErrDeliveryAttemptConsistency
		}
	}

	last := attempts[len(attempts)-1]
	running := last.FinishedAt == nil || last.Outcome == nil
	switch delivery.Status {
	case domain.DeliveryQueued:
		// 包含 not_sent 恢复重排及失败后的授权人工重试；历史终态不必与当前状态同义。
		return DeliveryAttemptConsistencyConsistent, nil
	case domain.DeliverySending:
		if running {
			return DeliveryAttemptConsistencyConsistent, nil
		}
		return DeliveryAttemptConsistencyIntermediate, nil
	case domain.DeliveryRetryWait:
		if !running && *last.Outcome == domain.DeliveryAttemptRejected && last.Retryable {
			return DeliveryAttemptConsistencyConsistent, nil
		}
	case domain.DeliverySent:
		if !running && *last.Outcome == domain.DeliveryAttemptAccepted &&
			sameAttemptProjectionText(delivery.ProviderMessageRef, last.ProviderMessageRef) &&
			delivery.ReasonCode == nil && delivery.LastErrorCode == nil {
			return DeliveryAttemptConsistencyConsistent, nil
		}
	case domain.DeliveryUnknown:
		if running || (!running && *last.Outcome == domain.DeliveryAttemptAmbiguous) {
			return DeliveryAttemptConsistencyConsistent, nil
		}
	case domain.DeliverySkipped:
		if !running && *last.Outcome == domain.DeliveryAttemptNotSent {
			return DeliveryAttemptConsistencyConsistent, nil
		}
	case domain.DeliveryBlocked:
		// 已有 Attempt 的 blocked 可能是发送前策略变更导致；不能仅凭快照断言矛盾。
		return DeliveryAttemptConsistencyConsistent, nil
	case domain.DeliveryFailed:
		if !running && *last.Outcome == domain.DeliveryAttemptRejected {
			return DeliveryAttemptConsistencyConsistent, nil
		}
	}
	return "", ErrDeliveryAttemptConsistency
}

func sameAttemptProjectionText(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

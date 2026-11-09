package application

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type LeaseRecoveryAction string

const (
	LeaseRecoverySafeToRetry      LeaseRecoveryAction = "safe_to_retry"
	LeaseRecoveryFinalizeSent     LeaseRecoveryAction = "finalize_sent"
	LeaseRecoveryApplyRetryPolicy LeaseRecoveryAction = "apply_retry_policy"
	LeaseRecoveryFinalizeFailed   LeaseRecoveryAction = "finalize_failed"
	LeaseRecoveryMarkUnknown      LeaseRecoveryAction = "mark_unknown"
)

var (
	ErrLeaseRecoveryNotApplicable = errors.New("notification: lease recovery is not applicable")
	ErrLeaseRecoveryHistory       = errors.New("notification: lease recovery attempt history is inconsistent")
)

// LeaseRecoveryAssessment 是只读恢复判断，不直接修改 Delivery 或触发渠道调用。
// 调用方必须在后续写入时再次使用 Delivery 版本与 fencing token 做 CAS。
type LeaseRecoveryAssessment struct {
	Action             LeaseRecoveryAction
	ReasonCode         string
	AttemptID          string
	EvidenceRef        string
	RetryAfterAt       *time.Time
	ProviderMessageRef string
}

// AssessExpiredDeliveryLease 根据完整 Attempt 历史判断过期租约能否安全收敛。
// StartWithLease 在外部 Send 前原子追加 Attempt，因此当前 token 没有 Attempt
// 可证明该 Worker 尚未进入渠道调用；运行中的 Attempt 则无法证明外部副作用状态。
func AssessExpiredDeliveryLease(
	delivery domain.DeliveryRecord,
	lease DeliveryLease,
	attempts []domain.DeliveryAttempt,
	now time.Time,
) (LeaseRecoveryAssessment, error) {
	if err := delivery.Validate(); err != nil {
		return LeaseRecoveryAssessment{}, err
	}
	if delivery.Status != domain.DeliverySending ||
		strings.TrimSpace(lease.WorkerID) == "" || len(lease.WorkerID) > 256 ||
		lease.WorkerID != strings.TrimSpace(lease.WorkerID) ||
		lease.FencingToken < 1 || !isRecoveryUTC(lease.ExpiresAt) ||
		!isRecoveryUTC(now) || now.Before(lease.ExpiresAt) {
		return LeaseRecoveryAssessment{}, ErrLeaseRecoveryNotApplicable
	}
	if len(attempts) != delivery.AttemptCount {
		return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
	}

	var current *domain.DeliveryAttempt
	seenIDs := make(map[string]struct{}, len(attempts))
	seenNumbers := make(map[int]struct{}, len(attempts))
	seenTokens := make(map[int64]struct{}, len(attempts))
	for i := range attempts {
		attempt := attempts[i]
		if err := attempt.Validate(); err != nil || attempt.DeliveryID != delivery.DeliveryID {
			return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
		}
		if _, exists := seenIDs[attempt.AttemptID]; exists {
			return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
		}
		if _, exists := seenNumbers[attempt.AttemptNo]; exists {
			return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
		}
		if _, exists := seenTokens[attempt.FencingToken]; exists {
			return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
		}
		seenIDs[attempt.AttemptID] = struct{}{}
		seenNumbers[attempt.AttemptNo] = struct{}{}
		seenTokens[attempt.FencingToken] = struct{}{}
		if attempt.FencingToken > lease.FencingToken {
			return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
		}
		if attempt.FencingToken == lease.FencingToken {
			if current != nil || attempt.WorkerID != lease.WorkerID ||
				!attempt.LeaseExpiresAt.Equal(lease.ExpiresAt) {
				return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
			}
			current = &attempts[i]
		}
	}
	for number := 1; number <= len(attempts); number++ {
		if _, exists := seenNumbers[number]; !exists {
			return LeaseRecoveryAssessment{}, ErrLeaseRecoveryHistory
		}
	}

	if current == nil {
		return recoveryBudgetAssessment(delivery, now, "lease_expired_before_attempt", "", ""), nil
	}
	result := LeaseRecoveryAssessment{AttemptID: current.AttemptID}
	if current.FinishedAt == nil || current.Outcome == nil {
		result.Action = LeaseRecoveryMarkUnknown
		result.ReasonCode = "lease_expired_during_attempt"
		return result, nil
	}
	switch *current.Outcome {
	case domain.DeliveryAttemptAccepted:
		result.Action = LeaseRecoveryFinalizeSent
		result.ReasonCode = "attempt_accepted_before_worker_crash"
		if current.ProviderMessageRef != nil {
			result.ProviderMessageRef = *current.ProviderMessageRef
		}
	case domain.DeliveryAttemptRejected:
		if !current.Retryable {
			result.Action = LeaseRecoveryFinalizeFailed
			result.ReasonCode = "channel_rejection_permanent"
		} else if delivery.AttemptCount >= delivery.MaxAttempts || !now.Before(delivery.DeadlineAt) {
			result.Action = LeaseRecoveryFinalizeFailed
			result.ReasonCode = "retry_budget_exhausted"
		} else {
			result.Action = LeaseRecoveryApplyRetryPolicy
			result.ReasonCode = "retryable_rejection_recovery"
			result.RetryAfterAt = cloneDeliveryTime(current.RetryAfterAt)
		}
	case domain.DeliveryAttemptNotSent:
		result.EvidenceRef = dereferenceRecoveryEvidence(current.EvidenceRef)
		return recoveryBudgetAssessment(delivery, now, "attempt_proved_not_sent", current.AttemptID, result.EvidenceRef), nil
	case domain.DeliveryAttemptAmbiguous:
		result.Action = LeaseRecoveryMarkUnknown
		result.ReasonCode = "channel_outcome_ambiguous"
	default:
		return LeaseRecoveryAssessment{}, fmt.Errorf("%w: unsupported attempt outcome", ErrLeaseRecoveryHistory)
	}
	return result, nil
}

func recoveryBudgetAssessment(delivery domain.DeliveryRecord, now time.Time, reason, attemptID, evidenceRef string) LeaseRecoveryAssessment {
	result := LeaseRecoveryAssessment{AttemptID: attemptID, EvidenceRef: evidenceRef, ReasonCode: reason}
	if delivery.AttemptCount >= delivery.MaxAttempts || !now.Before(delivery.DeadlineAt) {
		result.Action = LeaseRecoveryFinalizeFailed
		result.ReasonCode = "retry_budget_exhausted"
		return result
	}
	result.Action = LeaseRecoverySafeToRetry
	return result
}

func dereferenceRecoveryEvidence(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isRecoveryUTC(value time.Time) bool {
	if value.IsZero() || value.Location() != time.UTC {
		return false
	}
	_, offset := value.Zone()
	return offset == 0
}

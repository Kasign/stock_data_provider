package domain

import (
	"errors"
	"fmt"
	"time"
)

type DeliveryAttemptOutcome string

const (
	DeliveryAttemptAccepted  DeliveryAttemptOutcome = "accepted"
	DeliveryAttemptRejected  DeliveryAttemptOutcome = "rejected"
	DeliveryAttemptAmbiguous DeliveryAttemptOutcome = "ambiguous"
	DeliveryAttemptNotSent   DeliveryAttemptOutcome = "not_sent"
)

var ErrInvalidDeliveryAttempt = errors.New("notification: invalid delivery attempt")

// DeliveryAttempt 是一次真实渠道调用的追加式执行记录。
// 完成后的记录不可修改；结果纠正必须通过独立对账事实追加。
type DeliveryAttempt struct {
	AttemptID           string                 `json:"attempt_id"`
	DeliveryID          string                 `json:"delivery_id"`
	AttemptNo           int                    `json:"attempt_no"`
	WorkerID            string                 `json:"worker_id"`
	FencingToken        int64                  `json:"fencing_token"`
	LeaseExpiresAt      time.Time              `json:"lease_expires_at"`
	StartedAt           time.Time              `json:"started_at"`
	FinishedAt          *time.Time             `json:"finished_at"`
	Outcome             *DeliveryAttemptOutcome `json:"outcome"`
	ProviderMessageRef  *string                `json:"provider_message_ref"`
	ErrorCode           *string                `json:"error_code"`
	Retryable           bool                   `json:"retryable"`
	DurationMS          *int64                 `json:"duration_ms"`
	EvidenceRef         *string                `json:"evidence_ref"`
}

// Validate 校验运行中或已完成 Attempt 的字段一致性。
// 运行中记录的完成字段必须为空；已完成记录必须同时具有 outcome、finished_at 和 duration_ms。
func (a DeliveryAttempt) Validate() error {
	if !validOpaqueRef(a.AttemptID) || !validOpaqueRef(a.DeliveryID) ||
		a.AttemptNo < 1 || !validOpaqueRef(a.WorkerID) || a.FencingToken < 1 ||
		!isUTCTime(a.LeaseExpiresAt) || !isUTCTime(a.StartedAt) ||
		!a.LeaseExpiresAt.After(a.StartedAt) {
		return ErrInvalidDeliveryAttempt
	}
	if a.ProviderMessageRef != nil && !validOpaqueRef(*a.ProviderMessageRef) {
		return fmt.Errorf("%w: invalid provider_message_ref", ErrInvalidDeliveryAttempt)
	}
	if a.ErrorCode != nil && !validDeliveryCode(*a.ErrorCode) {
		return fmt.Errorf("%w: invalid error_code", ErrInvalidDeliveryAttempt)
	}
	if a.EvidenceRef != nil && !validOpaqueRef(*a.EvidenceRef) {
		return fmt.Errorf("%w: invalid evidence_ref", ErrInvalidDeliveryAttempt)
	}

	if a.FinishedAt == nil || a.Outcome == nil || a.DurationMS == nil {
		if a.FinishedAt != nil || a.Outcome != nil || a.DurationMS != nil ||
			a.ProviderMessageRef != nil || a.ErrorCode != nil || a.EvidenceRef != nil || a.Retryable {
			return fmt.Errorf("%w: incomplete attempt completion", ErrInvalidDeliveryAttempt)
		}
		return nil
	}
	if !isUTCTime(*a.FinishedAt) || a.FinishedAt.Before(a.StartedAt) ||
		*a.DurationMS < 0 || *a.DurationMS != a.FinishedAt.Sub(a.StartedAt).Milliseconds() {
		return fmt.Errorf("%w: invalid completion time or duration", ErrInvalidDeliveryAttempt)
	}

	switch *a.Outcome {
	case DeliveryAttemptAccepted:
		if a.ErrorCode != nil || a.Retryable {
			return fmt.Errorf("%w: accepted outcome cannot carry an error or retryable hint", ErrInvalidDeliveryAttempt)
		}
	case DeliveryAttemptRejected:
		if a.ErrorCode == nil {
			return fmt.Errorf("%w: rejected outcome requires error_code", ErrInvalidDeliveryAttempt)
		}
	case DeliveryAttemptAmbiguous:
		if a.ErrorCode == nil {
			return fmt.Errorf("%w: ambiguous outcome requires error_code", ErrInvalidDeliveryAttempt)
		}
	case DeliveryAttemptNotSent:
		if a.ErrorCode == nil || a.EvidenceRef == nil || a.ProviderMessageRef != nil || a.Retryable {
			return fmt.Errorf("%w: not_sent requires proof and cannot carry provider acceptance", ErrInvalidDeliveryAttempt)
		}
	default:
		return fmt.Errorf("%w: unknown outcome", ErrInvalidDeliveryAttempt)
	}
	return nil
}

// Complete 返回完成后的新快照，不修改原 Attempt。
// not_sent 必须附带能够证明请求未发出的 evidence_ref。
func (a DeliveryAttempt) Complete(at time.Time, outcome DeliveryAttemptOutcome, providerMessageRef, errorCode string, retryable bool, evidenceRef string) (DeliveryAttempt, error) {
	if err := a.Validate(); err != nil {
		return DeliveryAttempt{}, err
	}
	if a.FinishedAt != nil || !isUTCTime(at) || at.Before(a.StartedAt) {
		return DeliveryAttempt{}, ErrInvalidDeliveryAttempt
	}
	finished := at
	duration := at.Sub(a.StartedAt).Milliseconds()
	next := a
	next.FinishedAt = &finished
	next.Outcome = &outcome
	next.DurationMS = &duration
	next.ProviderMessageRef = stringPointer(providerMessageRef)
	next.ErrorCode = stringPointer(errorCode)
	next.Retryable = retryable
	next.EvidenceRef = stringPointer(evidenceRef)
	if err := next.Validate(); err != nil {
		return DeliveryAttempt{}, err
	}
	return next, nil
}

func cloneDeliveryAttempt(a DeliveryAttempt) DeliveryAttempt {
	if a.FinishedAt != nil {
		value := *a.FinishedAt
		a.FinishedAt = &value
	}
	if a.Outcome != nil {
		value := *a.Outcome
		a.Outcome = &value
	}
	a.ProviderMessageRef = cloneString(a.ProviderMessageRef)
	a.ErrorCode = cloneString(a.ErrorCode)
	a.DurationMS = cloneInt64(a.DurationMS)
	a.EvidenceRef = cloneString(a.EvidenceRef)
	return a
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

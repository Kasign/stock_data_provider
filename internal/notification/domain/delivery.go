package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliverySending   DeliveryStatus = "sending"
	DeliveryRetryWait DeliveryStatus = "retry_wait"
	DeliverySent      DeliveryStatus = "sent"
	DeliverySkipped   DeliveryStatus = "skipped"
	DeliveryBlocked   DeliveryStatus = "blocked"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryUnknown   DeliveryStatus = "unknown"
)

var (
	ErrInvalidDelivery       = errors.New("notification: invalid delivery")
	ErrInvalidDeliveryChange = errors.New("notification: invalid delivery transition")
)

// DeliveryRecord 是一次不可变投递计划对应的当前状态快照。
// 历史状态必须由 Repository 以 DeliveryStateEvent 追加保存，不能覆盖历史事件。
type DeliveryRecord struct {
	DeliveryID             string         `json:"delivery_id"`
	SourceModule           string         `json:"source_module"`
	EventID                string         `json:"event_id"`
	SubscriptionID         string         `json:"subscription_id"`
	SubscriptionVersion    int64          `json:"subscription_version"`
	ChannelRef             string         `json:"channel_ref"`
	ChannelConfigVersion   int64          `json:"channel_config_version"`
	TemplateID             string         `json:"template_id"`
	TemplateVersion        int64          `json:"template_version"`
	RecipientRef           string         `json:"recipient_ref"`
	PayloadFingerprint     string         `json:"payload_fingerprint"`
	PlanFingerprint        string         `json:"plan_fingerprint"`
	DeliveryIdempotencyKey string         `json:"delivery_idempotency_key"`
	Status                 DeliveryStatus `json:"status"`
	Version                int64          `json:"version"`
	AttemptCount           int            `json:"attempt_count"`
	MaxAttempts            int            `json:"max_attempts"`
	NextAttemptAt          *time.Time     `json:"next_attempt_at"`
	DeadlineAt             time.Time      `json:"deadline_at"`
	ReasonCode             *string        `json:"reason_code"`
	LastErrorCode          *string        `json:"last_error_code"`
	ProviderMessageRef     *string        `json:"provider_message_ref"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// DeliveryStateEvent 是不可变状态变更事实，version 与 DeliveryRecord.version 一一对应。
type DeliveryStateEvent struct {
	DeliveryID  string         `json:"delivery_id"`
	Version     int64          `json:"version"`
	From        DeliveryStatus `json:"from"`
	To          DeliveryStatus `json:"to"`
	ChangedAt   time.Time      `json:"changed_at"`
	ReasonCode  *string        `json:"reason_code"`
	EvidenceRef *string        `json:"evidence_ref"`
	ActorRef    *string        `json:"actor_ref"`
	RequestID   *string        `json:"request_id"`
	ActorReason *string        `json:"actor_reason"`
}

// DeliveryTransition 描述一次 CAS 前的领域状态变更请求。
// RequestNotSent 只能由调用方在能够证明外部请求未发出时设置。
// AuthorizedManualRetry 必须由 Application 层完成身份/权限检查后设置。
type DeliveryTransition struct {
	To                    DeliveryStatus
	At                    time.Time
	ReasonCode            string
	ErrorCode             string
	EvidenceRef           string
	ProviderMessageRef    string
	NextAttemptAt          *time.Time
	RequestNotSent        bool
	AuthorizedManualRetry bool
	ActorRef              string
	RequestID             string
	ManualRetryReason     string
}

func (d DeliveryRecord) Validate() error {
	if !validOpaqueRef(d.DeliveryID) ||
		!validNotificationSource(d.SourceModule) ||
		!validOpaqueRef(d.EventID) ||
		!validOpaqueRef(d.SubscriptionID) || d.SubscriptionVersion < 1 ||
		!validOpaqueRef(d.ChannelRef) || d.ChannelConfigVersion < 1 ||
		!validOpaqueRef(d.TemplateID) || d.TemplateVersion < 1 ||
		!validOpaqueRef(d.RecipientRef) ||
		!validSHA256Digest(d.PayloadFingerprint) ||
		!validSHA256Digest(d.PlanFingerprint) ||
		!validOpaqueRef(d.DeliveryIdempotencyKey) ||
		!validDeliveryStatus(d.Status) ||
		d.Version < 1 || d.AttemptCount < 0 || d.MaxAttempts < 1 ||
		d.AttemptCount > d.MaxAttempts ||
		!isUTCTime(d.DeadlineAt) || !isUTCTime(d.CreatedAt) ||
		!isUTCTime(d.UpdatedAt) || d.UpdatedAt.Before(d.CreatedAt) ||
		!d.DeadlineAt.After(d.CreatedAt) {
		return ErrInvalidDelivery
	}
	if d.NextAttemptAt != nil {
		if !isUTCTime(*d.NextAttemptAt) || d.Status != DeliveryRetryWait ||
			!d.NextAttemptAt.After(d.UpdatedAt) || !d.NextAttemptAt.Before(d.DeadlineAt) {
			return fmt.Errorf("%w: invalid next_attempt_at", ErrInvalidDelivery)
		}
	}
	if d.Status == DeliveryRetryWait && d.NextAttemptAt == nil {
		return fmt.Errorf("%w: retry_wait requires next_attempt_at", ErrInvalidDelivery)
	}
	if d.Status != DeliveryRetryWait && d.NextAttemptAt != nil {
		return fmt.Errorf("%w: next_attempt_at is only valid for retry_wait", ErrInvalidDelivery)
	}
	if d.ReasonCode != nil && !validDeliveryCode(*d.ReasonCode) {
		return fmt.Errorf("%w: invalid reason_code", ErrInvalidDelivery)
	}
	if d.LastErrorCode != nil && !validDeliveryCode(*d.LastErrorCode) {
		return fmt.Errorf("%w: invalid last_error_code", ErrInvalidDelivery)
	}
	if d.ProviderMessageRef != nil && !validOpaqueRef(*d.ProviderMessageRef) {
		return fmt.Errorf("%w: invalid provider_message_ref", ErrInvalidDelivery)
	}
	if deliveryRequiresReason(d.Status) && (d.ReasonCode == nil || *d.ReasonCode == "") {
		return fmt.Errorf("%w: status requires reason_code", ErrInvalidDelivery)
	}
	return nil
}

func (d DeliveryRecord) Transition(change DeliveryTransition) (DeliveryRecord, DeliveryStateEvent, error) {
	if err := d.Validate(); err != nil {
		return DeliveryRecord{}, DeliveryStateEvent{}, err
	}
	if !isUTCTime(change.At) || change.At.Before(d.UpdatedAt) || !validDeliveryStatus(change.To) ||
		((change.To == DeliverySending || change.To == DeliveryQueued) && !change.At.Before(d.DeadlineAt)) {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if change.ReasonCode != "" && !validDeliveryCode(change.ReasonCode) ||
		change.ErrorCode != "" && !validDeliveryCode(change.ErrorCode) ||
		change.EvidenceRef != "" && !validOpaqueRef(change.EvidenceRef) ||
		change.ProviderMessageRef != "" && !validOpaqueRef(change.ProviderMessageRef) {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if d.Status == DeliveryRetryWait && change.To == DeliverySending && change.At.Before(*d.NextAttemptAt) {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if change.RequestNotSent && (change.To != DeliverySkipped || change.EvidenceRef == "") {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if d.Status == DeliverySending && change.To == DeliverySkipped && (!change.RequestNotSent || change.EvidenceRef == "") {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if !deliveryTransitionAllowed(d.Status, change.To, change) {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if change.ManualRetryReason != "" && (len(change.ManualRetryReason) > 512 || strings.TrimSpace(change.ManualRetryReason) != change.ManualRetryReason || strings.ContainsAny(change.ManualRetryReason, "\r\n\x00")) {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if deliveryRequiresReason(change.To) && change.ReasonCode == "" {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if change.To == DeliveryRetryWait {
		if change.NextAttemptAt == nil || !isUTCTime(*change.NextAttemptAt) ||
			!change.NextAttemptAt.After(change.At) || !change.NextAttemptAt.Before(d.DeadlineAt) {
			return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
		}
	} else if change.NextAttemptAt != nil {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if d.Status == DeliveryUnknown && change.EvidenceRef == "" {
		return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
	}
	if change.To == DeliveryQueued && d.Status == DeliveryFailed {
		if !change.AuthorizedManualRetry || !validOpaqueRef(change.ActorRef) ||
			!validOpaqueRef(change.RequestID) || strings.TrimSpace(change.ManualRetryReason) == "" ||
			len(change.ManualRetryReason) > 512 {
			return DeliveryRecord{}, DeliveryStateEvent{}, ErrInvalidDeliveryChange
		}
	}

	next := d
	next.Status = change.To
	next.Version++
	next.UpdatedAt = change.At
	next.NextAttemptAt = cloneTime(change.NextAttemptAt)
	next.ReasonCode = stringPointer(change.ReasonCode)
	next.LastErrorCode = stringPointer(change.ErrorCode)
	next.ProviderMessageRef = stringPointer(change.ProviderMessageRef)
	if change.To == DeliverySent {
		next.NextAttemptAt = nil
		next.LastErrorCode = nil
	}
	if err := next.Validate(); err != nil {
		return DeliveryRecord{}, DeliveryStateEvent{}, err
	}

	event := DeliveryStateEvent{
		DeliveryID:  next.DeliveryID,
		Version:     next.Version,
		From:        d.Status,
		To:          next.Status,
		ChangedAt:   change.At,
		ReasonCode:  stringPointer(change.ReasonCode),
		EvidenceRef: stringPointer(change.EvidenceRef),
		ActorRef:    stringPointer(change.ActorRef),
		RequestID:   stringPointer(change.RequestID),
		ActorReason: stringPointer(change.ManualRetryReason),
	}
	return next, event, nil
}

func deliveryTransitionAllowed(from, to DeliveryStatus, change DeliveryTransition) bool {
	switch from {
	case DeliveryQueued:
		return to == DeliverySending || to == DeliverySkipped || to == DeliveryBlocked
	case DeliveryBlocked:
		return to == DeliveryQueued || to == DeliverySkipped || to == DeliveryFailed
	case DeliverySending:
		if to == DeliverySkipped {
			return change.RequestNotSent
		}
		return to == DeliverySent || to == DeliveryRetryWait ||
			to == DeliveryFailed || to == DeliveryUnknown
	case DeliveryRetryWait:
		return to == DeliverySending || to == DeliverySkipped ||
			to == DeliveryBlocked || to == DeliveryFailed
	case DeliveryUnknown:
		return to == DeliverySent || to == DeliveryRetryWait || to == DeliveryFailed
	case DeliveryFailed:
		return to == DeliveryQueued && change.AuthorizedManualRetry
	default:
		return false
	}
}

func validDeliveryStatus(status DeliveryStatus) bool {
	switch status {
	case DeliveryQueued, DeliverySending, DeliveryRetryWait, DeliverySent,
		DeliverySkipped, DeliveryBlocked, DeliveryFailed, DeliveryUnknown:
		return true
	default:
		return false
	}
}

func validNotificationSource(source string) bool {
	return source == "quant" || source == "research" || source == "researchflow"
}

func deliveryRequiresReason(status DeliveryStatus) bool {
	switch status {
	case DeliveryRetryWait, DeliverySkipped, DeliveryBlocked, DeliveryFailed, DeliveryUnknown:
		return true
	default:
		return false
	}
}

func validDeliveryCode(code string) bool {
	if code == "" || len(code) > 128 || code != strings.TrimSpace(code) {
		return false
	}
	for _, character := range code {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= '0' && character <= '9') &&
			character != '_' && character != '.' && character != '-' {
			return false
		}
	}
	return true
}

func isUTCTime(value time.Time) bool {
	if value.IsZero() {
		return false
	}
	_, offset := value.Zone()
	return value.Location() == time.UTC && offset == 0
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

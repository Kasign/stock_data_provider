package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidChannelLookupRequest = errors.New("notification: invalid channel lookup request")
	ErrInvalidChannelLookupResult = errors.New("notification: invalid channel lookup result")
)

type ChannelLookupOutcome string

const (
	ChannelLookupAccepted ChannelLookupOutcome = "accepted"
	ChannelLookupNotAccepted ChannelLookupOutcome = "not_accepted"
	ChannelLookupIndeterminate ChannelLookupOutcome = "indeterminate"
)

// ChannelDeliveryLookupRequest 只携带精确投递身份和不透明查询键。
type ChannelDeliveryLookupRequest struct {
	ChannelRef string
	ConfigVersion int64
	DeliveryID string
	AttemptID string
	IdempotencyKey string
	ProviderMessageRef string
}

func (r ChannelDeliveryLookupRequest) Validate(capabilities ChannelCapabilities) error {
	if capabilities.Validate() != nil ||
		!validOpaqueRef(r.ChannelRef) || r.ConfigVersion < 1 ||
		!validOpaqueRef(r.DeliveryID) || !validOpaqueRef(r.AttemptID) {
		return ErrInvalidChannelLookupRequest
	}
	if r.IdempotencyKey != "" && capabilities.SupportsIdempotency != CapabilitySupported {
		return ErrInvalidChannelLookupRequest
	}
	if r.IdempotencyKey != "" && !validOpaqueRef(r.IdempotencyKey) {
		return ErrInvalidChannelLookupRequest
	}
	if r.ProviderMessageRef != "" && !validOpaqueRef(r.ProviderMessageRef) {
		return ErrInvalidChannelLookupRequest
	}
	return nil
}

// ChannelDeliveryLookupResult 的身份字段必须与请求完全一致。
// evidence_ref 仅用于审计定位，不单独构成权威性证明。
type ChannelDeliveryLookupResult struct {
	ChannelRef string
	ConfigVersion int64
	DeliveryID string
	AttemptID string
	Outcome ChannelLookupOutcome
	EvidenceRef string
	ObservedAt time.Time
	ProviderMessageRef string
	ErrorCode string
}

func (r ChannelDeliveryLookupResult) Validate(request ChannelDeliveryLookupRequest) error {
	if r.ChannelRef != request.ChannelRef || r.ConfigVersion != request.ConfigVersion ||
		r.DeliveryID != request.DeliveryID || r.AttemptID != request.AttemptID ||
		!validOpaqueRef(r.EvidenceRef) || r.ObservedAt.IsZero() ||
		r.ObservedAt.Location() != time.UTC {
		return ErrInvalidChannelLookupResult
	}
	switch r.Outcome {
	case ChannelLookupAccepted:
		if !validOpaqueRef(r.ProviderMessageRef) || r.ErrorCode != "" {
			return ErrInvalidChannelLookupResult
		}
	case ChannelLookupNotAccepted:
		if !validChannelErrorCode(r.ErrorCode) || r.ProviderMessageRef != "" {
			return ErrInvalidChannelLookupResult
		}
	case ChannelLookupIndeterminate:
		if r.ProviderMessageRef != "" || r.ErrorCode != "" {
			return ErrInvalidChannelLookupResult
		}
	default:
		return ErrInvalidChannelLookupResult
	}
	return nil
}

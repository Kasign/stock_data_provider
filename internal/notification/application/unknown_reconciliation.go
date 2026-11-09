package application

import (
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrUnknownReconciliationNotApplicable = errors.New("notification: unknown reconciliation is not applicable")

type UnknownLookupOutcome string

const (
	UnknownLookupAccepted      UnknownLookupOutcome = "accepted"
	UnknownLookupNotAccepted   UnknownLookupOutcome = "not_accepted"
	UnknownLookupIndeterminate UnknownLookupOutcome = "indeterminate"
)

type UnknownDeliveryEvidence struct {
	DeliveryID         string
	AttemptID          string
	Outcome            UnknownLookupOutcome
	EvidenceRef        string
	ObservedAt         time.Time
	ProviderMessageRef string
	ErrorCode          string
}

type UnknownReconciliationAction string

const (
	UnknownReconciliationRemainUnknown    UnknownReconciliationAction = "remain_unknown"
	UnknownReconciliationFinalizeSent     UnknownReconciliationAction = "finalize_sent"
	UnknownReconciliationApplyRetryPolicy UnknownReconciliationAction = "apply_retry_policy"
	UnknownReconciliationFinalizeFailed   UnknownReconciliationAction = "finalize_failed"
)

type UnknownReconciliationAssessment struct {
	DeliveryID         string
	AttemptID          string
	FencingToken       int64
	Action             UnknownReconciliationAction
	ReasonCode         string
	EvidenceRef        string
	ProviderMessageRef string
	ErrorCode          string
}

// AssessUnknownDelivery validates authoritative channel lookup evidence without
// mutating Delivery. The caller must persist a resulting transition with a
// separate CAS operation; an indeterminate result always remains unknown.
func AssessUnknownDelivery(
	delivery domain.DeliveryRecord,
	channel domain.ChannelConfigSnapshot,
	attempts []domain.DeliveryAttempt,
	evidence UnknownDeliveryEvidence,
	now time.Time,
) (UnknownReconciliationAssessment, error) {
	if err := delivery.Validate(); err != nil {
		return UnknownReconciliationAssessment{}, err
	}
	if err := channel.Validate(); err != nil {
		return UnknownReconciliationAssessment{}, err
	}
	if delivery.Status != domain.DeliveryUnknown ||
		channel.ChannelRef != delivery.ChannelRef ||
		channel.ConfigVersion != delivery.ChannelConfigVersion ||
		channel.Capabilities.SupportsDeliveryLookup != domain.CapabilitySupported ||
		!isRecoveryUTC(now) || !isRecoveryUTC(evidence.ObservedAt) ||
		evidence.ObservedAt.After(now) ||
		evidence.DeliveryID != delivery.DeliveryID ||
		!validReconciliationRef(evidence.AttemptID) ||
		!validReconciliationRef(evidence.EvidenceRef) ||
		len(attempts) != delivery.AttemptCount {
		return UnknownReconciliationAssessment{}, ErrUnknownReconciliationNotApplicable
	}

	var matched *domain.DeliveryAttempt
	seenIDs := make(map[string]struct{}, len(attempts))
	for i := range attempts {
		attempt := attempts[i]
		if err := attempt.Validate(); err != nil || attempt.DeliveryID != delivery.DeliveryID {
			return UnknownReconciliationAssessment{}, ErrLeaseRecoveryHistory
		}
		if _, exists := seenIDs[attempt.AttemptID]; exists {
			return UnknownReconciliationAssessment{}, ErrLeaseRecoveryHistory
		}
		seenIDs[attempt.AttemptID] = struct{}{}
		if attempt.AttemptNo == delivery.AttemptCount {
			matched = &attempts[i]
		}
	}
	if matched == nil || matched.AttemptID != evidence.AttemptID ||
		(matched.Outcome != nil && *matched.Outcome != domain.DeliveryAttemptAmbiguous) {
		return UnknownReconciliationAssessment{}, ErrUnknownReconciliationNotApplicable
	}

	result := UnknownReconciliationAssessment{
		DeliveryID:   delivery.DeliveryID,
		AttemptID:    matched.AttemptID,
		FencingToken: matched.FencingToken,
		Action:       UnknownReconciliationRemainUnknown,
		ReasonCode:   "channel_lookup_indeterminate",
		EvidenceRef:  evidence.EvidenceRef,
	}
	switch evidence.Outcome {
	case UnknownLookupIndeterminate:
		return result, nil
	case UnknownLookupAccepted:
		if !validReconciliationRef(evidence.ProviderMessageRef) || evidence.ErrorCode != "" {
			return UnknownReconciliationAssessment{}, ErrUnknownReconciliationNotApplicable
		}
		result.Action = UnknownReconciliationFinalizeSent
		result.ReasonCode = "channel_lookup_confirmed_accepted"
		result.ProviderMessageRef = evidence.ProviderMessageRef
	case UnknownLookupNotAccepted:
		if !validReconciliationCode(evidence.ErrorCode) || evidence.ProviderMessageRef != "" {
			return UnknownReconciliationAssessment{}, ErrUnknownReconciliationNotApplicable
		}
		result.ErrorCode = evidence.ErrorCode
		result.ReasonCode = "channel_lookup_confirmed_not_accepted"
		if channelAllowsRetry(channel, evidence.ErrorCode) {
			result.Action = UnknownReconciliationApplyRetryPolicy
		} else {
			result.Action = UnknownReconciliationFinalizeFailed
			result.ReasonCode = "channel_lookup_confirmed_permanent_rejection"
		}
	default:
		return UnknownReconciliationAssessment{}, ErrUnknownReconciliationNotApplicable
	}
	return result, nil
}

func channelAllowsRetry(channel domain.ChannelConfigSnapshot, code string) bool {
	for _, allowed := range channel.RetryableErrorCodes {
		if allowed == code {
			return true
		}
	}
	return false
}

func validReconciliationRef(value string) bool {
	if value == "" || len(value) > 256 || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

func validReconciliationCode(value string) bool {
	if value == "" || len(value) > 128 || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '.' || r == '-') {
			return false
		}
	}
	return true
}

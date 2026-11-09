package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CapabilityState string

const (
	CapabilityUnknown     CapabilityState = "unknown"
	CapabilitySupported   CapabilityState = "supported"
	CapabilityUnsupported CapabilityState = "unsupported"
)

type ChannelCapabilities struct {
	SupportsIdempotency    CapabilityState `json:"supports_idempotency"`
	SupportsDeliveryLookup CapabilityState `json:"supports_delivery_lookup"`
	SupportsRetryAfter     CapabilityState `json:"supports_retry_after"`
	MaxPayloadBytes        int64           `json:"max_payload_bytes"`
}

func (c ChannelCapabilities) Validate() error {
	if !validCapabilityState(c.SupportsIdempotency) ||
		!validCapabilityState(c.SupportsDeliveryLookup) ||
		!validCapabilityState(c.SupportsRetryAfter) ||
		c.MaxPayloadBytes <= 0 {
		return ErrInvalidChannelConfig
	}
	return nil
}

func validCapabilityState(state CapabilityState) bool {
	switch state {
	case CapabilityUnknown, CapabilitySupported, CapabilityUnsupported:
		return true
	default:
		return false
	}
}

type ChannelConfigSnapshot struct {
	ChannelRef          string              `json:"channel_ref"`
	ConfigVersion       int64               `json:"config_version"`
	ChannelKind         string              `json:"channel_kind"`
	Enabled             bool                `json:"enabled"`
	SecretRef           string              `json:"secret_ref,omitempty"`
	Capabilities        ChannelCapabilities `json:"capabilities"`
	RetryableErrorCodes []string            `json:"retryable_error_codes"`
}

var ErrInvalidChannelConfig = errors.New("notification: invalid channel config")
var ErrChannelDisabled = errors.New("notification: channel is disabled")
var ErrInvalidChannelSendRequest = errors.New("notification: invalid channel send request")
var ErrInvalidChannelSendResult = errors.New("notification: invalid channel send result")
var ErrInvalidRecipientTarget = errors.New("notification: invalid recipient target")

func (c ChannelConfigSnapshot) Validate() error {
	if strings.TrimSpace(c.ChannelRef) == "" || c.ChannelRef != strings.TrimSpace(c.ChannelRef) ||
		len(c.ChannelRef) > 128 || c.ConfigVersion < 1 ||
		strings.TrimSpace(c.ChannelKind) == "" || c.ChannelKind != strings.TrimSpace(c.ChannelKind) ||
		len(c.ChannelKind) > 64 ||
		(c.SecretRef != "" && (c.SecretRef != strings.TrimSpace(c.SecretRef) || len(c.SecretRef) > 256)) {
		return ErrInvalidChannelConfig
	}
	if err := c.Capabilities.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(c.RetryableErrorCodes))
	for _, code := range c.RetryableErrorCodes {
		if !validChannelErrorCode(code) {
			return ErrInvalidChannelConfig
		}
		if _, exists := seen[code]; exists {
			return fmt.Errorf("%w: duplicate retryable error code", ErrInvalidChannelConfig)
		}
		seen[code] = struct{}{}
	}
	return nil
}

type ChannelSendOutcome string

const (
	ChannelOutcomeAccepted  ChannelSendOutcome = "accepted"
	ChannelOutcomeRejected  ChannelSendOutcome = "rejected"
	ChannelOutcomeAmbiguous ChannelSendOutcome = "ambiguous"
	ChannelOutcomeNotSent   ChannelSendOutcome = "not_sent"
)

// RecipientTarget contains only opaque references. It must never contain an
// email address, phone number, URL, or other plaintext destination.
type RecipientTarget struct {
	RecipientRef     string `json:"recipient_ref"`
	ChannelRef       string `json:"channel_ref"`
	ConfigVersion    int64  `json:"config_version"`
	AdapterTargetRef string `json:"adapter_target_ref"`
}

func (r RecipientTarget) Validate(channel ChannelConfigSnapshot) error {
	if !validOpaqueRef(r.RecipientRef) || r.ChannelRef != channel.ChannelRef ||
		r.ConfigVersion != channel.ConfigVersion || !validOpaqueRef(r.AdapterTargetRef) {
		return ErrInvalidRecipientTarget
	}
	return nil
}

type ChannelSendRequest struct {
	DeliveryID     string          `json:"delivery_id"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Target         RecipientTarget `json:"target"`
	Content        RenderedContent `json:"content"`
}

func (r ChannelSendRequest) Validate(channel ChannelConfigSnapshot) error {
	if err := channel.Validate(); err != nil {
		return err
	}
	if !channel.Enabled {
		return ErrChannelDisabled
	}
	if r.DeliveryID == "" || r.Target.Validate(channel) != nil ||
		r.Content.TemplateID == "" || r.Content.TemplateVersion < 1 ||
		!validSHA256Digest(r.Content.TemplateDigest) ||
		!validSHA256Digest(r.Content.EventPayloadFingerprint) ||
		(r.Content.Format != TemplateFormatText && r.Content.Format != TemplateFormatHTML) {
		return ErrInvalidChannelSendRequest
	}
	if int64(len(r.Content.Subject)+len(r.Content.Body)) > channel.Capabilities.MaxPayloadBytes {
		return fmt.Errorf("%w: rendered content exceeds channel payload limit", ErrInvalidChannelSendRequest)
	}
	if channel.Capabilities.SupportsIdempotency == CapabilitySupported {
		if r.IdempotencyKey == "" {
			return fmt.Errorf("%w: supported idempotency requires a stable key", ErrInvalidChannelSendRequest)
		}
	} else if r.IdempotencyKey != "" {
		return fmt.Errorf("%w: idempotency key must not be sent without verified support", ErrInvalidChannelSendRequest)
	}
	return nil
}

type ChannelSendResult struct {
	Outcome            ChannelSendOutcome `json:"outcome"`
	ProviderMessageRef string             `json:"provider_message_ref,omitempty"`
	ErrorCode          string             `json:"error_code,omitempty"`
	RetryAfter         *time.Time         `json:"retry_after,omitempty"`
}

func (r ChannelSendResult) Validate(capabilities ChannelCapabilities) error {
	if err := capabilities.Validate(); err != nil {
		return err
	}
	switch r.Outcome {
	case ChannelOutcomeAccepted:
		if r.ErrorCode != "" || r.RetryAfter != nil {
			return ErrInvalidChannelSendResult
		}
	case ChannelOutcomeRejected:
		if !validChannelErrorCode(r.ErrorCode) {
			return ErrInvalidChannelSendResult
		}
	case ChannelOutcomeAmbiguous, ChannelOutcomeNotSent:
		if !validChannelErrorCode(r.ErrorCode) || r.RetryAfter != nil {
			return ErrInvalidChannelSendResult
		}
	default:
		return ErrInvalidChannelSendResult
	}
	if r.RetryAfter != nil {
		if r.Outcome != ChannelOutcomeRejected ||
			capabilities.SupportsRetryAfter != CapabilitySupported ||
			r.RetryAfter.IsZero() || r.RetryAfter.Location() != time.UTC {
			return ErrInvalidChannelSendResult
		}
	}
	return nil
}

func validOpaqueRef(value string) bool {
	if value == "" || len(value) > 256 || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			character != '-' && character != '_' && character != '.' && character != ':' {
			return false
		}
	}
	return true
}

func validSHA256Digest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validChannelErrorCode(code string) bool {
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

// IsRetryable deliberately accepts only an explicit rejection in the
// channel-specific allowlist. Ambiguous and not_sent outcomes are never
// automatically retried by this rule.
func (r ChannelSendResult) IsRetryable(channel ChannelConfigSnapshot) bool {
	if channel.Validate() != nil || r.Validate(channel.Capabilities) != nil ||
		r.Outcome != ChannelOutcomeRejected || !validChannelErrorCode(r.ErrorCode) {
		return false
	}
	for _, code := range channel.RetryableErrorCodes {
		if code == r.ErrorCode {
			return true
		}
	}
	return false
}

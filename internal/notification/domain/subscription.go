package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type SubscriptionStatus string

const (
	SubscriptionActive   SubscriptionStatus = "active"
	SubscriptionPaused   SubscriptionStatus = "paused"
	SubscriptionDisabled SubscriptionStatus = "disabled"
)

type RecipientScope string

const (
	RecipientScopeOwner               RecipientScope = "subscription_owner"
	RecipientScopePreconfiguredTarget RecipientScope = "preconfigured_target"
)

var (
	ErrInvalidSubscription         = errors.New("notification: invalid subscription")
	ErrSubscriptionVersionConflict = errors.New("notification: subscription version conflict")
)

// EventTypeLookup rejects subscriptions for unregistered event types.
type EventTypeLookup interface {
	HasEventType(eventType string) bool
}

// FilterSchemaValidator validates a constrained filter document for one event type/version.
// A missing validator must never cause a non-empty filter to be accepted.
type FilterSchemaValidator interface {
	ValidateFilter(eventType, schemaVersion string, filters json.RawMessage) error
}

type Subscription struct {
	SubscriptionID      string             `json:"subscription_id"`
	OwnerRef            string             `json:"owner_ref"`
	Version             int64              `json:"version"`
	Status              SubscriptionStatus `json:"status"`
	EventType           string             `json:"event_type"`
	FilterSchemaVersion string             `json:"filter_schema_version"`
	Filters             json.RawMessage    `json:"filters"`
	RecipientScope      RecipientScope     `json:"recipient_scope"`
	RecipientRef        string             `json:"recipient_ref"`
	ChannelRef          string             `json:"channel_ref"`
	TemplateID          string             `json:"template_id"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
	EffectiveAt         time.Time          `json:"effective_at"`
}

func (s Subscription) Validate(eventTypes EventTypeLookup, filterValidator FilterSchemaValidator) error {
	if s.SubscriptionID == "" || s.OwnerRef == "" || s.Version < 1 ||
		s.EventType == "" || s.ChannelRef == "" || s.TemplateID == "" {
		return ErrInvalidSubscription
	}
	if eventTypes == nil || !eventTypes.HasEventType(s.EventType) {
		return fmt.Errorf("%w: event_type is not registered", ErrInvalidSubscription)
	}
	switch s.Status {
	case SubscriptionActive, SubscriptionPaused, SubscriptionDisabled:
	default:
		return ErrInvalidSubscription
	}
	switch s.RecipientScope {
	case RecipientScopeOwner:
		if s.RecipientRef != "" {
			return fmt.Errorf("%w: subscription_owner must not carry recipient_ref", ErrInvalidSubscription)
		}
	case RecipientScopePreconfiguredTarget:
		if s.RecipientRef == "" {
			return fmt.Errorf("%w: preconfigured_target requires recipient_ref", ErrInvalidSubscription)
		}
	default:
		return ErrInvalidSubscription
	}
	if !isUTCInstant(s.CreatedAt) || !isUTCInstant(s.UpdatedAt) || !isUTCInstant(s.EffectiveAt) ||
		s.UpdatedAt.Before(s.CreatedAt) || s.EffectiveAt.Before(s.CreatedAt) {
		return fmt.Errorf("%w: timestamps must be RFC3339Nano UTC and monotonic", ErrInvalidSubscription)
	}

	filters := bytes.TrimSpace(s.Filters)
	if len(filters) == 0 || bytes.Equal(filters, []byte("null")) {
		if s.FilterSchemaVersion != "" {
			return fmt.Errorf("%w: null filters require null filter_schema_version", ErrInvalidSubscription)
		}
		return nil
	}
	if s.FilterSchemaVersion == "" {
		return fmt.Errorf("%w: filters require filter_schema_version", ErrInvalidSubscription)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(filters, &object); err != nil || object == nil {
		return fmt.Errorf("%w: filters must be a JSON object", ErrInvalidSubscription)
	}
	if filterValidator == nil {
		return fmt.Errorf("%w: filter schema validator is unavailable", ErrInvalidSubscription)
	}
	if err := filterValidator.ValidateFilter(s.EventType, s.FilterSchemaVersion, filters); err != nil {
		return fmt.Errorf("%w: filter schema validation: %v", ErrInvalidSubscription, err)
	}
	return nil
}

// ValidateSubscriptionVersion enforces append-only versioning for an existing subscription.
func ValidateSubscriptionVersion(previous, next Subscription) error {
	if previous.SubscriptionID == "" || previous.SubscriptionID != next.SubscriptionID ||
		previous.OwnerRef != next.OwnerRef || next.Version != previous.Version+1 ||
		!previous.CreatedAt.Equal(next.CreatedAt) ||
		!isUTCInstant(next.UpdatedAt) || !isUTCInstant(next.EffectiveAt) ||
		next.UpdatedAt.Before(previous.UpdatedAt) || next.EffectiveAt.Before(previous.EffectiveAt) {
		return ErrSubscriptionVersionConflict
	}
	return nil
}

func isUTCInstant(value time.Time) bool {
	if value.IsZero() || value.Location() != time.UTC {
		return false
	}
	_, offset := value.Zone()
	return offset == 0
}

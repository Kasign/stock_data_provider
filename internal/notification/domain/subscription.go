package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	if err := validateSubscriptionFilterJSON(filters); err != nil {
		return fmt.Errorf("%w: filters must be a strict JSON object: %v", ErrInvalidSubscription, err)
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

func validateSubscriptionFilterJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("top-level value must be an object")
	}
	if err := consumeSubscriptionFilterObject(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func consumeSubscriptionFilterObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("object key must be a string")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate object key %q", key)
		}
		seen[key] = struct{}{}
		if err := consumeSubscriptionFilterValue(decoder); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	if end != json.Delim('}') {
		return errors.New("malformed object")
	}
	return nil
}

func consumeSubscriptionFilterValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return consumeSubscriptionFilterObject(decoder)
	case '[':
		for decoder.More() {
			if err := consumeSubscriptionFilterValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("malformed array")
		}
		return nil
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

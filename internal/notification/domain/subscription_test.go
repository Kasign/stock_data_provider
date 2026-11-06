package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type acceptingEventTypeLookup struct{}

func (acceptingEventTypeLookup) HasEventType(eventType string) bool {
	return eventType == "quant.risk_decision.changed"
}

type acceptingFilterValidator struct{}

func (acceptingFilterValidator) ValidateFilter(eventType, schemaVersion string, filters json.RawMessage) error {
	if eventType != "quant.risk_decision.changed" || schemaVersion != "1.0.0" {
		return errors.New("unknown filter schema")
	}
	return nil
}

func validSubscription() Subscription {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	return Subscription{
		SubscriptionID: "sub-1", OwnerRef: "owner-1", Version: 1,
		Status: SubscriptionActive, EventType: "quant.risk_decision.changed",
		RecipientScope: RecipientScopeOwner, ChannelRef: "channel-1", TemplateID: "template-1",
		CreatedAt: now, UpdatedAt: now, EffectiveAt: now,
	}
}

func TestSubscriptionRecipientScope(t *testing.T) {
	sub := validSubscription()
	if err := sub.Validate(acceptingEventTypeLookup{}, nil); err != nil {
		t.Fatalf("subscription_owner should be valid: %v", err)
	}
	sub.RecipientRef = "recipient-1"
	if err := sub.Validate(acceptingEventTypeLookup{}, nil); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected invalid subscription_owner recipient_ref, got %v", err)
	}
	sub = validSubscription()
	sub.RecipientScope = RecipientScopePreconfiguredTarget
	if err := sub.Validate(acceptingEventTypeLookup{}, nil); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected preconfigured target to require recipient_ref, got %v", err)
	}
	sub.RecipientRef = "target-1"
	if err := sub.Validate(acceptingEventTypeLookup{}, nil); err != nil {
		t.Fatalf("preconfigured target should be valid: %v", err)
	}
}

func TestSubscriptionFiltersFailClosed(t *testing.T) {
	sub := validSubscription()
	sub.Filters = json.RawMessage(`{"current_status":"reject"}`)
	sub.FilterSchemaVersion = "1.0.0"
	if err := sub.Validate(acceptingEventTypeLookup{}, nil); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected unavailable filter validator to fail closed, got %v", err)
	}
	if err := sub.Validate(acceptingEventTypeLookup{}, acceptingFilterValidator{}); err != nil {
		t.Fatalf("valid registered filter should pass: %v", err)
	}
	sub.Filters = json.RawMessage(`[]`)
	if err := sub.Validate(acceptingEventTypeLookup{}, acceptingFilterValidator{}); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected non-object filters to fail, got %v", err)
	}
	sub.Filters = json.RawMessage(`null`)
	if err := sub.Validate(acceptingEventTypeLookup{}, acceptingFilterValidator{}); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("null filters must not carry filter schema version, got %v", err)
	}
}

func TestSubscriptionVersionMustBeAppendOnly(t *testing.T) {
	previous := validSubscription()
	next := previous
	next.Version = 2
	next.UpdatedAt = next.UpdatedAt.Add(time.Second)
	if err := ValidateSubscriptionVersion(previous, next); err != nil {
		t.Fatalf("next version should be valid: %v", err)
	}
	next.Version = 3
	if !errors.Is(ValidateSubscriptionVersion(previous, next), ErrSubscriptionVersionConflict) {
		t.Fatal("expected version gap to be rejected")
	}
}

func TestSubscriptionRequiresRegisteredEventType(t *testing.T) {
	sub := validSubscription()
	if err := sub.Validate(nil, nil); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected missing event registry to fail closed, got %v", err)
	}
	sub.EventType = "unregistered.event"
	if err := sub.Validate(acceptingEventTypeLookup{}, nil); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected unknown event type to fail, got %v", err)
	}
}

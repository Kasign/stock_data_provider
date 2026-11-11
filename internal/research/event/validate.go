package event

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func (e ResearchEvent) Validate() error {
	if err := required("event_id", e.EventID); err != nil { return err }
	if e.EventVersion < 1 { return fmt.Errorf("event_version must be >= 1") }
	if err := required("event_type", e.EventType); err != nil { return err }
	if err := required("title", e.Title); err != nil { return err }
	if err := validateStatus(e.Status); err != nil { return err }
	for _, item := range []struct { name string; values []string }{
		{"subject_refs", e.SubjectRefs},
		{"evidence_refs", e.EvidenceRefs},
		{"impact_hypothesis_refs", e.ImpactHypothesisRefs},
		{"invalidation_conditions", e.InvalidationConditions},
		{"uncertainty", e.Uncertainty},
	} {
		if item.values == nil { return fmt.Errorf("%s must be a non-null array", item.name) }
		if err := uniqueStrings(item.name, item.values); err != nil { return err }
	}
	if err := utc("system_observed_at", e.SystemObservedAt); err != nil { return err }
	if err := utc("system_as_of", e.SystemAsOf); err != nil { return err }
	if e.SystemAsOf.Before(e.SystemObservedAt) {
		return fmt.Errorf("system_as_of must not be earlier than system_observed_at")
	}
	for _, item := range []struct { name string; value *time.Time }{
		{"occurred_at", e.OccurredAt},
		{"published_at", e.PublishedAt},
		{"available_at", e.AvailableAt},
	} {
		if item.value != nil {
			if err := utc(item.name, *item.value); err != nil { return err }
		}
	}
	if e.SupersedesEventVersion != nil && (*e.SupersedesEventVersion < 1 || *e.SupersedesEventVersion >= e.EventVersion) {
		return fmt.Errorf("supersedes_event_version must be positive and less than event_version")
	}
	if e.SourceTimes == nil { return fmt.Errorf("source_times must be a non-null array") }
	evidenceRefs := make(map[string]struct{}, len(e.EvidenceRefs))
	for _, ref := range e.EvidenceRefs { evidenceRefs[ref] = struct{}{} }
	timeTypes := make(map[EventTimeType]struct{}, len(e.SourceTimes))
	for index, sourceTime := range e.SourceTimes {
		if err := sourceTime.Validate(); err != nil { return fmt.Errorf("source_times[%d]: %w", index, err) }
		if _, exists := evidenceRefs[sourceTime.EvidenceRef]; !exists {
			return fmt.Errorf("source_times[%d].evidence_ref must exist in evidence_refs", index)
		}
		timeTypes[sourceTime.TimeType] = struct{}{}
	}
	for _, item := range []struct {
		name string
		value *time.Time
		timeType EventTimeType
	}{
		{"occurred_at", e.OccurredAt, TimeOccurredAt},
		{"published_at", e.PublishedAt, TimePublishedAt},
		{"available_at", e.AvailableAt, TimeAvailableAt},
	} {
		if item.value != nil {
			if _, exists := timeTypes[item.timeType]; !exists {
				return fmt.Errorf("%s requires a matching source_times entry", item.name)
			}
		}
	}
	return nil
}

func (t EventSourceTime) Validate() error {
	if err := required("evidence_ref", t.EvidenceRef); err != nil { return err }
	if err := validateTimeType(t.TimeType); err != nil { return err }
	if err := utc("value", t.Value); err != nil { return err }
	if err := required("asserted_by", t.AssertedBy); err != nil { return err }
	if t.Confidence != nil {
		if math.IsNaN(*t.Confidence) || math.IsInf(*t.Confidence, 0) || *t.Confidence < 0 || *t.Confidence > 1 {
			return fmt.Errorf("confidence must be within [0,1]")
		}
		if t.ConfidenceMethod == nil || strings.TrimSpace(*t.ConfidenceMethod) == "" {
			return fmt.Errorf("confidence_method is required when confidence is provided")
		}
		if t.ConfidenceMethodVersion == nil || strings.TrimSpace(*t.ConfidenceMethodVersion) == "" {
			return fmt.Errorf("confidence_method_version is required when confidence is provided")
		}
	} else if t.ConfidenceMethod != nil || t.ConfidenceMethodVersion != nil {
		return fmt.Errorf("confidence method fields require a non-null confidence")
	}
	return nil
}

func (next ResearchEvent) ValidateNextVersion(previous ResearchEvent) error {
	if err := previous.Validate(); err != nil { return fmt.Errorf("previous event: %w", err) }
	if err := next.Validate(); err != nil { return fmt.Errorf("next event: %w", err) }
	if next.EventID != previous.EventID { return fmt.Errorf("event_id cannot change across versions") }
	if previous.EventVersion == int(^uint(0)>>1) {
		return fmt.Errorf("event_version reached the supported integer limit")
	}
	if next.EventVersion != previous.EventVersion+1 { return fmt.Errorf("event_version must increment by exactly one") }
	if next.SystemAsOf.Before(previous.SystemAsOf) { return fmt.Errorf("system_as_of cannot move backwards across event versions") }
	if next.Status != previous.Status && !CanTransition(previous.Status, next.Status) {
		return fmt.Errorf("invalid event status transition: %s -> %s", previous.Status, next.Status)
	}
	return nil
}

func NewLifecycleTransition(previous, next ResearchEvent, changedAt time.Time) (*LifecycleTransition, error) {
	if err := next.ValidateNextVersion(previous); err != nil {
		return nil, err
	}
	if previous.Status == next.Status {
		return nil, nil
	}
	if err := utc("changed_at", changedAt); err != nil {
		return nil, err
	}
	return &LifecycleTransition{
		EventID: next.EventID,
		EventVersion: next.EventVersion,
		FromStatus: previous.Status,
		ToStatus: next.Status,
		ChangedAt: changedAt,
	}, nil
}

func (t LifecycleTransition) Validate() error {
	if err := required("event_id", t.EventID); err != nil { return err }
	if t.EventVersion < 2 { return fmt.Errorf("lifecycle transition event_version must be >= 2") }
	if err := validateStatus(t.FromStatus); err != nil { return fmt.Errorf("from_status: %w", err) }
	if err := validateStatus(t.ToStatus); err != nil { return fmt.Errorf("to_status: %w", err) }
	if t.FromStatus == t.ToStatus { return fmt.Errorf("lifecycle transition requires different statuses") }
	return utc("changed_at", t.ChangedAt)
}

func CanTransition(from, to EventStatus) bool {
	if from == to { return false }
	if to == StatusInvalidated {
		return from != StatusClosed && from != StatusInvalidated && validStatus(from) == nil
	}
	switch from {
	case StatusDetected: return to == StatusConsolidating
	case StatusConsolidating: return to == StatusActive
	case StatusActive: return to == StatusMaterializing
	case StatusMaterializing: return to == StatusFading
	case StatusFading: return to == StatusClosed
	default: return false
	}
}

func validateStatus(value EventStatus) error {
	if err := validStatus(value); err != nil { return fmt.Errorf("status: %w", err) }
	return nil
}

func validStatus(value EventStatus) error {
	switch value {
	case StatusDetected, StatusConsolidating, StatusActive, StatusMaterializing, StatusFading, StatusInvalidated, StatusClosed:
		return nil
	default: return fmt.Errorf("unknown status %q", value)
	}
}

func validateTimeType(value EventTimeType) error {
	switch value {
	case TimeOccurredAt, TimePublishedAt, TimeAvailableAt, TimeSystemObservedAt: return nil
	default: return fmt.Errorf("time_type is invalid")
	}
}

func required(name, value string) error {
	if value == "" || value != strings.TrimSpace(value) { return fmt.Errorf("%s is required without surrounding whitespace", name) }
	return nil
}

func uniqueStrings(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := required(name+" item", value); err != nil { return err }
		if _, exists := seen[value]; exists { return fmt.Errorf("%s contains duplicate value %q", name, value) }
		seen[value] = struct{}{}
	}
	return nil
}

func utc(name string, value time.Time) error {
	if value.IsZero() { return fmt.Errorf("%s is required", name) }
	_, offset := value.Zone()
	if offset != 0 || value.Location() != time.UTC { return fmt.Errorf("%s must be UTC", name) }
	return nil
}

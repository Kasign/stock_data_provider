package event

import (
	"encoding/json"
	"fmt"
	"time"
)

type EventStatus string

const (
	StatusDetected      EventStatus = "detected"
	StatusConsolidating EventStatus = "consolidating"
	StatusActive        EventStatus = "active"
	StatusMaterializing EventStatus = "materializing"
	StatusFading        EventStatus = "fading"
	StatusInvalidated   EventStatus = "invalidated"
	StatusClosed        EventStatus = "closed"
)

type EventTimeType string

const (
	TimeOccurredAt       EventTimeType = "occurred_at"
	TimePublishedAt      EventTimeType = "published_at"
	TimeAvailableAt      EventTimeType = "available_at"
	TimeSystemObservedAt EventTimeType = "system_observed_at"
)

type ResearchEventRef struct {
	EventID      string `json:"event_id"`
	EventVersion int    `json:"event_version"`
}

func (r *ResearchEventRef) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("ResearchEventRef target is nil")
	}
	if err := validateJSONShape(data,
		[]string{"event_id", "event_version"},
		[]string{"event_id", "event_version"}); err != nil {
		return err
	}
	type eventRefAlias ResearchEventRef
	var value eventRefAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = ResearchEventRef(value)
	return nil
}

type EventSourceTime struct {
	EvidenceRef             string        `json:"evidence_ref"`
	TimeType                EventTimeType `json:"time_type"`
	Value                   time.Time     `json:"value"`
	AssertedBy              string        `json:"asserted_by"`
	Confidence              *float64      `json:"confidence,omitempty"`
	ConfidenceMethod        *string       `json:"confidence_method,omitempty"`
	ConfidenceMethodVersion *string       `json:"confidence_method_version,omitempty"`
	Notes                   *string       `json:"notes,omitempty"`
}

// ResearchEvent is one immutable version of a cross-source real-world event.
// Notification mapping and persistence are intentionally outside this package.
type ResearchEvent struct {
	EventID                      string             `json:"event_id"`
	EventVersion                 int                `json:"event_version"`
	EventType                    string             `json:"event_type"`
	Title                        string             `json:"title"`
	Status                       EventStatus        `json:"status"`
	SubjectRefs                  []string           `json:"subject_refs"`
	EvidenceRefs                 []string           `json:"evidence_refs"`
	OccurredAt                   *time.Time         `json:"occurred_at"`
	PublishedAt                  *time.Time         `json:"published_at"`
	AvailableAt                  *time.Time         `json:"available_at"`
	SystemObservedAt             time.Time          `json:"system_observed_at"`
	SystemAsOf                   time.Time          `json:"system_as_of"`
	ImpactHypothesisRefs         []string           `json:"impact_hypothesis_refs"`
	InvalidationConditions       []string           `json:"invalidation_conditions"`
	Uncertainty                  []string           `json:"uncertainty"`
	SourceTimes                  []EventSourceTime  `json:"source_times"`
	TimeNormalizationRuleVersion string             `json:"time_normalization_rule_version"`
	DerivedFromRefs              []ResearchEventRef `json:"derived_from_refs"`
}

// LifecycleTransition is an immutable lifecycle_changed payload.
// ChangedAt must be fixed by the same transaction that persists the new
// event version and Outbox record; retries must reuse the persisted value.
type LifecycleTransition struct {
	EventID      string      `json:"event_id"`
	EventVersion int         `json:"event_version"`
	FromStatus   EventStatus `json:"from_status"`
	ToStatus     EventStatus `json:"to_status"`
	ChangedAt    time.Time   `json:"changed_at"`
}

// UnmarshalJSON rejects fields outside the frozen RSC-CTR-002 Event contract
// and requires nullable timestamp properties to be explicitly present.
func (e *ResearchEvent) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("ResearchEvent target is nil")
	}
	if err := validateJSONShape(data,
		[]string{"event_id", "event_version", "event_type", "title", "status", "subject_refs", "evidence_refs", "occurred_at", "published_at", "available_at", "system_observed_at", "system_as_of", "impact_hypothesis_refs", "invalidation_conditions", "uncertainty", "source_times", "time_normalization_rule_version", "derived_from_refs"},
		[]string{"event_id", "event_version", "event_type", "title", "status", "subject_refs", "evidence_refs", "occurred_at", "published_at", "available_at", "system_observed_at", "system_as_of", "impact_hypothesis_refs", "invalidation_conditions", "uncertainty", "source_times", "time_normalization_rule_version", "derived_from_refs"}); err != nil {
		return err
	}
	type eventAlias ResearchEvent
	var value eventAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*e = ResearchEvent(value)
	return nil
}

func (t *EventSourceTime) UnmarshalJSON(data []byte) error {
	if t == nil {
		return fmt.Errorf("EventSourceTime target is nil")
	}
	if err := validateJSONShape(data,
		[]string{"evidence_ref", "time_type", "value", "asserted_by"},
		[]string{"evidence_ref", "time_type", "value", "asserted_by", "confidence", "confidence_method", "confidence_method_version", "notes"}); err != nil {
		return err
	}
	type sourceTimeAlias EventSourceTime
	var value sourceTimeAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*t = EventSourceTime(value)
	return nil
}

func validateJSONShape(data []byte, required, allowed []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedSet[field]; !ok {
			return fmt.Errorf("unknown field %q", field)
		}
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("required field %q is missing", field)
		}
	}
	return nil
}

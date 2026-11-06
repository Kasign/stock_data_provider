package event

import "time"

type EventStatus string

const (
	StatusDetected      EventStatus = "detected"
	StatusConsolidating EventStatus = "consolidating"
	StatusActive        EventStatus = "active"
	StatusMaterializing EventStatus = "materializing"
	StatusFading         EventStatus = "fading"
	StatusInvalidated    EventStatus = "invalidated"
	StatusClosed         EventStatus = "closed"
)

type EventTimeType string

const (
	TimeOccurredAt       EventTimeType = "occurred_at"
	TimePublishedAt      EventTimeType = "published_at"
	TimeAvailableAt      EventTimeType = "available_at"
	TimeSystemObservedAt EventTimeType = "system_observed_at"
)

type EventSourceTime struct {
	EvidenceRef           string        `json:"evidence_ref"`
	TimeType              EventTimeType `json:"time_type"`
	Value                 time.Time     `json:"value"`
	AssertedBy            string        `json:"asserted_by"`
	Confidence            *float64      `json:"confidence,omitempty"`
	ConfidenceMethod      *string       `json:"confidence_method,omitempty"`
	ConfidenceMethodVersion *string     `json:"confidence_method_version,omitempty"`
	Notes                 *string       `json:"notes,omitempty"`
}

// ResearchEvent is one immutable version of a cross-source real-world event.
// Notification mapping and persistence are intentionally outside this package.
type ResearchEvent struct {
	EventID               string            `json:"event_id"`
	EventVersion          int               `json:"event_version"`
	EventType             string            `json:"event_type"`
	Title                 string            `json:"title"`
	Status                EventStatus       `json:"status"`
	SubjectRefs           []string          `json:"subject_refs"`
	EvidenceRefs          []string          `json:"evidence_refs"`
	OccurredAt            *time.Time        `json:"occurred_at"`
	PublishedAt           *time.Time        `json:"published_at"`
	AvailableAt           *time.Time        `json:"available_at"`
	SystemObservedAt      time.Time         `json:"system_observed_at"`
	SystemAsOf            time.Time         `json:"system_as_of"`
	ImpactHypothesisRefs  []string          `json:"impact_hypothesis_refs"`
	InvalidationConditions []string         `json:"invalidation_conditions"`
	Uncertainty           []string          `json:"uncertainty"`
	SupersedesEventVersion *int             `json:"supersedes_event_version,omitempty"`
	SourceTimes           []EventSourceTime `json:"source_times"`
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

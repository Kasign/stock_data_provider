package event

import (
	"math"
	"strings"
	"testing"
	"time"
)

func eventTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil { t.Fatal(err) }
	return parsed
}

func validEvent(t *testing.T) ResearchEvent {
	t.Helper()
	occurred := eventTime(t, "2026-11-05T01:00:00Z")
	observed := eventTime(t, "2026-11-05T01:02:00Z")
	asOf := eventTime(t, "2026-11-06T05:00:00Z")
	return ResearchEvent{
		EventID: "event:research:0001", EventVersion: 1, EventType: "earnings_guidance",
		Title: "Company raises annual guidance", Status: StatusDetected,
		SubjectRefs: []string{"600000.SH"}, EvidenceRefs: []string{"evidence:news:0001"},
		OccurredAt: &occurred, PublishedAt: nil, AvailableAt: nil,
		SystemObservedAt: observed, SystemAsOf: asOf,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{"company retracts guidance"},
		Uncertainty: []string{"guidance may be revised"},
		SourceTimes: []EventSourceTime{{
			EvidenceRef: "evidence:news:0001", TimeType: TimeOccurredAt,
			Value: occurred, AssertedBy: "news:source:exchange_notice",
		}},
	}
}

func TestResearchEventValidate(t *testing.T) {
	value := validEvent(t)
	if err := value.Validate(); err != nil { t.Fatalf("Validate() error = %v", err) }
}

func TestResearchEventRejectsInvalidStructureAndTimeEvidence(t *testing.T) {
	tests := []struct { name string; mutate func(*ResearchEvent); want string }{
		{"zero version", func(e *ResearchEvent) { e.EventVersion = 0 }, "event_version"},
		{"unknown status", func(e *ResearchEvent) { e.Status = "archived" }, "unknown status"},
		{"nil evidence refs", func(e *ResearchEvent) { e.EvidenceRefs = nil }, "non-null array"},
		{"duplicate subject refs", func(e *ResearchEvent) { e.SubjectRefs = []string{"600000.SH", "600000.SH"} }, "duplicate"},
		{"system cutoff before observation", func(e *ResearchEvent) { e.SystemAsOf = e.SystemObservedAt.Add(-time.Second) }, "must not be earlier"},
		{"source time evidence missing", func(e *ResearchEvent) { e.SourceTimes[0].EvidenceRef = "evidence:missing" }, "must exist in evidence_refs"},
		{"invalid time type", func(e *ResearchEvent) { e.SourceTimes[0].TimeType = "received_at" }, "time_type"},
		{"source time non UTC", func(e *ResearchEvent) { e.SourceTimes[0].Value = time.Date(2026, 11, 5, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60)) }, "must be UTC"},
		{"confidence without method", func(e *ResearchEvent) { confidence := 0.8; e.SourceTimes[0].Confidence = &confidence }, "confidence_method is required"},
		{"method without confidence", func(e *ResearchEvent) { method := "source_rank"; e.SourceTimes[0].ConfidenceMethod = &method }, "require a non-null confidence"},
		{"version one supersedes", func(e *ResearchEvent) { version := 0; e.SupersedesEventVersion = &version }, "must not supersede"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := validEvent(t)
			tt.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestResearchEventNormalizedTimesNeedSourceTimeTrace(t *testing.T) {
	value := validEvent(t)
	value.SourceTimes = []EventSourceTime{}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "occurred_at requires a matching source_times entry") {
		t.Fatalf("normalized occurred_at without source time should fail, got %v", err)
	}
}

func TestResearchEventVersionMustAppendAndPreserveLifecycle(t *testing.T) {
	previous := validEvent(t)
	next := previous
	next.EventVersion = 2
	supersedes := 1
	next.SupersedesEventVersion = &supersedes
	next.SystemAsOf = previous.SystemAsOf.Add(time.Minute)
	if err := next.ValidateNextVersion(previous); err != nil { t.Fatalf("same-status append should pass: %v", err) }
	next.Status = StatusConsolidating
	if err := next.ValidateNextVersion(previous); err != nil { t.Fatalf("valid lifecycle transition should pass: %v", err) }
	next.Status = StatusActive
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "invalid event status transition") {
		t.Fatalf("skipped lifecycle state should fail, got %v", err)
	}
	next = previous
	next.EventVersion = 3
	supersedes = 2
	next.SupersedesEventVersion = &supersedes
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "increment by exactly one") {
		t.Fatalf("version gap should fail, got %v", err)
	}
}

func TestResearchEventLifecycleTerminalAndInvalidationRules(t *testing.T) {
	allowed := []struct{ from, to EventStatus }{
		{StatusDetected, StatusConsolidating}, {StatusConsolidating, StatusActive},
		{StatusActive, StatusMaterializing}, {StatusMaterializing, StatusFading},
		{StatusFading, StatusClosed}, {StatusDetected, StatusInvalidated},
		{StatusConsolidating, StatusInvalidated}, {StatusActive, StatusInvalidated},
		{StatusMaterializing, StatusInvalidated}, {StatusFading, StatusInvalidated},
	}
	for _, pair := range allowed {
		if !CanTransition(pair.from, pair.to) { t.Errorf("CanTransition(%s, %s) = false, want true", pair.from, pair.to) }
	}
	for _, pair := range []struct{ from, to EventStatus }{
		{StatusDetected, StatusActive}, {StatusActive, StatusConsolidating},
		{StatusClosed, StatusActive}, {StatusInvalidated, StatusActive},
		{StatusClosed, StatusInvalidated}, {StatusInvalidated, StatusClosed},
		{StatusActive, StatusActive},
	} {
		if CanTransition(pair.from, pair.to) { t.Errorf("CanTransition(%s, %s) = true, want false", pair.from, pair.to) }
	}
}

func TestResearchEventVersionCanOmitOptionalSupersedesReference(t *testing.T) {
	previous := validEvent(t)
	next := previous
	next.EventVersion = 2
	next.SystemAsOf = previous.SystemAsOf.Add(time.Minute)
	next.SupersedesEventVersion = nil
	if err := next.ValidateNextVersion(previous); err != nil {
		t.Fatalf("optional supersedes_event_version should not be required: %v", err)
	}
}

func TestLifecycleTransitionOnlyEmitsForStatusChange(t *testing.T) {
	previous := validEvent(t)
	next := previous
	next.EventVersion = 2
	next.SystemAsOf = previous.SystemAsOf.Add(time.Minute)
	next.Status = StatusConsolidating
	changedAt := eventTime(t, "2026-11-06T05:10:00Z")
	transition, err := NewLifecycleTransition(previous, next, changedAt)
	if err != nil {
		t.Fatalf("NewLifecycleTransition() error = %v", err)
	}
	if transition == nil {
		t.Fatal("status change should produce lifecycle transition")
	}
	if err := transition.Validate(); err != nil {
		t.Fatalf("transition validation failed: %v", err)
	}
	if !transition.ChangedAt.Equal(changedAt) {
		t.Fatalf("changed_at = %s, want %s", transition.ChangedAt, changedAt)
	}

	noChange := next
	noChange.EventVersion = 3
	noChange.SystemAsOf = next.SystemAsOf.Add(time.Minute)
	noChange.Title = "修订标题但状态未变"
	noTransition, err := NewLifecycleTransition(next, noChange, changedAt)
	if err != nil {
		t.Fatalf("same-status revision should pass: %v", err)
	}
	if noTransition != nil {
		t.Fatal("same-status revision must not emit lifecycle_changed")
	}
}

func TestLifecycleTransitionRejectsNonUTCTime(t *testing.T) {
	previous := validEvent(t)
	next := previous
	next.EventVersion = 2
	next.SystemAsOf = previous.SystemAsOf.Add(time.Minute)
	next.Status = StatusConsolidating
	changedAt := time.Date(2026, 11, 6, 13, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if _, err := NewLifecycleTransition(previous, next, changedAt); err == nil || !strings.Contains(err.Error(), "must be UTC") {
		t.Fatalf("non-UTC changed_at should fail, got %v", err)
	}
}


func TestResearchEventValidateNextVersionRejectsIntegerOverflow(t *testing.T) {
	previous := validEvent(t)
	previous.EventVersion = int(^uint(0) >> 1)
	next := previous
	next.EventVersion = 1
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "integer limit") {
		t.Fatalf("ValidateNextVersion() error = %v, want integer limit rejection", err)
	}
}


func TestEventSourceTimeConfidenceBoundaries(t *testing.T) {
	method := "source_rank"
	methodVersion := "source_rank.v1"

	for _, boundary := range []struct {
		name  string
		value float64
	}{
		{"zero", 0},
		{"one", 1},
	} {
		t.Run("valid boundary "+boundary.name, func(t *testing.T) {
			value := validEvent(t)
			value.SourceTimes[0].Confidence = &boundary.value
			value.SourceTimes[0].ConfidenceMethod = &method
			value.SourceTimes[0].ConfidenceMethodVersion = &methodVersion
			if err := value.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want success", err)
			}
		})
	}

	for _, confidence := range []struct {
		name  string
		value float64
	}{
		{"below zero", -0.0001},
		{"above one", 1.0001},
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	} {
		t.Run(confidence.name, func(t *testing.T) {
			value := validEvent(t)
			value.SourceTimes[0].Confidence = &confidence.value
			value.SourceTimes[0].ConfidenceMethod = &method
			value.SourceTimes[0].ConfidenceMethodVersion = &methodVersion
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "confidence must be within [0,1]") {
				t.Fatalf("Validate() error = %v, want confidence range rejection", err)
			}
		})
	}
}

func TestEventSourceTimeConfidenceRequiresMethodAndVersion(t *testing.T) {
	confidence := 0.5
	method := "source_rank"
	methodVersion := "source_rank.v1"
	tests := []struct {
		name string
		set func(*EventSourceTime)
		want string
	}{
		{"missing method", func(value *EventSourceTime) { value.ConfidenceMethod = nil }, "confidence_method is required"},
		{"blank method", func(value *EventSourceTime) { blank := " "; value.ConfidenceMethod = &blank }, "confidence_method is required"},
		{"missing method version", func(value *EventSourceTime) { value.ConfidenceMethodVersion = nil }, "confidence_method_version is required"},
		{"blank method version", func(value *EventSourceTime) { blank := " "; value.ConfidenceMethodVersion = &blank }, "confidence_method_version is required"},
		{"method without confidence", func(value *EventSourceTime) { value.Confidence = nil }, "require a non-null confidence"},
		{"method version without confidence", func(value *EventSourceTime) { value.Confidence = nil; value.ConfidenceMethod = nil }, "require a non-null confidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := validEvent(t)
			sourceTime := &value.SourceTimes[0]
			sourceTime.Confidence = &confidence
			sourceTime.ConfidenceMethod = &method
			sourceTime.ConfidenceMethodVersion = &methodVersion
			tt.set(sourceTime)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

func TestBuildResearchLifecycleEnvelopeKeepsStablePublicationIdentity(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 2, 3, 123456789, time.UTC)
	previous := researchevent.ResearchEvent{
		EventID: "evt_abc", EventVersion: 1, EventType: "earnings_guidance",
		Title: "业绩指引", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_1"},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	next := previous
	next.EventVersion = 2
	next.Status = researchevent.StatusConsolidating
	next.SystemAsOf = now.Add(time.Second)
	transition, err := researchevent.NewLifecycleTransition(previous, next, now.Add(2*time.Second))
	if err != nil { t.Fatal(err) }
	first, payload, err := buildResearchLifecycleEnvelope(previous, next, *transition, nil, "req_123", now.Add(2*time.Second))
	if err != nil { t.Fatal(err) }
	second, _, err := buildResearchLifecycleEnvelope(previous, next, *transition, nil, "req_123", now.Add(2*time.Second))
	if err != nil { t.Fatal(err) }
	if first.IdempotencyKey != "research-event-state:research-event:evt_abc:2" {
		t.Fatalf("unexpected idempotency key: %s", first.IdempotencyKey)
	}
	if first.EventID == second.EventID {
		t.Fatal("independent envelope creation must not reuse event IDs; retry must reuse persisted envelope")
	}
	if first.PayloadFingerprint != second.PayloadFingerprint {
		t.Fatal("same lifecycle payload must have the same fingerprint")
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil { t.Fatal(err) }
	if got["source_event_ref"] != "research-event-version:evt_abc:2" ||
		got["research_event_ref"] != "research-event:evt_abc" ||
		got["previous_state"] != string(researchevent.StatusDetected) ||
		got["current_state"] != string(researchevent.StatusConsolidating) {
		t.Fatalf("unexpected lifecycle payload: %#v", got)
	}
	if value, ok := got["reason_code"]; !ok || value != nil {
		t.Fatalf("reason_code must be explicitly null, got %#v", value)
	}
	if first.OccurredAt != first.PublishedAt {
		t.Fatal("this transaction fixes occurred_at and published_at to the same repository time")
	}
	if len(first.PayloadFingerprint) != 64 || strings.ToLower(first.PayloadFingerprint) != first.PayloadFingerprint {
		t.Fatalf("invalid payload fingerprint: %q", first.PayloadFingerprint)
	}
}

func TestBuildResearchLifecycleEnvelopeRejectsMismatchedTransition(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 2, 3, 0, time.UTC)
	previous := researchevent.ResearchEvent{EventID: "evt_a", EventVersion: 1, Status: researchevent.StatusDetected}
	next := researchevent.ResearchEvent{EventID: "evt_a", EventVersion: 2, Status: researchevent.StatusActive}
	transition := researchevent.LifecycleTransition{
		EventID: "evt_other", EventVersion: 2, FromStatus: researchevent.StatusDetected,
		ToStatus: researchevent.StatusActive, ChangedAt: now,
	}
	if _, _, err := buildResearchLifecycleEnvelope(previous, next, transition, nil, "req_123", now); err == nil {
		t.Fatal("mismatched transition identity must be rejected")
	}
}

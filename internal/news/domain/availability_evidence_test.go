package domain

import (
	"testing"
	"time"
)

func validAvailabilityEvidence() AvailabilityEvidence {
	observed := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	available := observed.Add(-time.Second)
	return AvailabilityEvidence{
		ID: "evidence-1", NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1",
		FieldPath: "data.publish_time", RawValue: "2026-11-16T09:59+08:00",
		SourceTimezoneOrOffset: "+08:00", Precision: AvailabilityTimePrecisionMinute,
		ParseStatus: AvailabilityTimeParsed, CandidateAvailableAt: &available,
		ParserVersion: "availability-time-v1", ObservedAt: observed,
		RecordedAt: observed.Add(time.Second), RetentionUntil: observed.Add(24 * time.Hour),
	}
}

func TestAvailabilityEvidenceValidatesParsedMinutePrecisionCandidate(t *testing.T) {
	value := validAvailabilityEvidence()
	if err := value.Validate(); err != nil {
		t.Fatalf("valid availability evidence rejected: %v", err)
	}
}

func TestAvailabilityEvidenceRejectsSubsecondAndFutureCandidates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AvailabilityEvidence)
	}{
		{"subsecond", func(value *AvailabilityEvidence) {
			candidate := value.CandidateAvailableAt.Add(250 * time.Millisecond)
			value.CandidateAvailableAt = &candidate
		}},
		{"after observation", func(value *AvailabilityEvidence) {
			candidate := value.ObservedAt.Add(time.Second)
			value.CandidateAvailableAt = &candidate
		}},
		{"missing timezone", func(value *AvailabilityEvidence) { value.SourceTimezoneOrOffset = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validAvailabilityEvidence()
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid availability evidence unexpectedly passed")
			}
		})
	}
}

func TestAvailabilityEvidenceRequiresReasonForUnparsedValue(t *testing.T) {
	value := validAvailabilityEvidence()
	value.ParseStatus = AvailabilityTimeAmbiguous
	value.ParseReasonCode = "news.available_at.timezone_missing"
	value.CandidateAvailableAt = nil
	value.SourceTimezoneOrOffset = ""
	if err := value.Validate(); err != nil {
		t.Fatalf("valid ambiguous evidence rejected: %v", err)
	}
	value.ParseReasonCode = ""
	if err := value.Validate(); err == nil {
		t.Fatal("ambiguous evidence without reason must fail")
	}
}

func TestAvailabilityEvidencePurgedPayloadKeepsOnlyTombstone(t *testing.T) {
	value := validAvailabilityEvidence()
	purgedAt := value.RecordedAt.Add(time.Hour)
	value.PayloadPurgedAt = &purgedAt
	value.RawValue = ""
	value.EvidenceRef = nil
	value.EvidenceHash = nil
	value.CandidateAvailableAt = nil
	if err := value.Validate(); err != nil {
		t.Fatalf("valid purged tombstone rejected: %v", err)
	}
	value.RawValue = "still present"
	if err := value.Validate(); err == nil {
		t.Fatal("purged evidence must not retain restricted raw value")
	}
}

func TestAvailabilityDecisionValidatesCompleteStateAndStableEvidenceSet(t *testing.T) {
	selected := "evidence-2"
	decision := AvailabilityDecision{
		ID: "decision-1", VersionID: "version-1", Sequence: 1,
		Status: AvailabilityDecisionVerified, SelectedEvidenceID: &selected,
		ConsideredEvidenceIDs: []string{"evidence-1", "evidence-2"},
		ReasonCode:            "availability.source_verified", ReviewRuleVersion: "review-v1",
		RecordedAt: time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC),
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("valid verified decision rejected: %v", err)
	}
}

func TestAvailabilityDecisionRejectsForkAndInvalidStatusShapes(t *testing.T) {
	selected := "evidence-1"
	recorded := time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value AvailabilityDecision
	}{
		{"selected not considered", AvailabilityDecision{ID: "d1", VersionID: "v1", Sequence: 1, Status: AvailabilityDecisionVerified, SelectedEvidenceID: &selected, ConsideredEvidenceIDs: []string{}, ReasonCode: "r", ReviewRuleVersion: "v1", RecordedAt: recorded}},
		{"conflict with one evidence", AvailabilityDecision{ID: "d1", VersionID: "v1", Sequence: 1, Status: AvailabilityDecisionConflict, ConsideredEvidenceIDs: []string{"evidence-1"}, ReasonCode: "r", ReviewRuleVersion: "v1", RecordedAt: recorded}},
		{"unsorted evidence", AvailabilityDecision{ID: "d1", VersionID: "v1", Sequence: 1, Status: AvailabilityDecisionUnresolved, ConsideredEvidenceIDs: []string{"evidence-2", "evidence-1"}, ReasonCode: "r", ReviewRuleVersion: "v1", RecordedAt: recorded}},
		{"nil evidence array", AvailabilityDecision{ID: "d1", VersionID: "v1", Sequence: 1, Status: AvailabilityDecisionUnresolved, ReasonCode: "r", ReviewRuleVersion: "v1", RecordedAt: recorded}},
		{"missing predecessor", AvailabilityDecision{ID: "d2", VersionID: "v1", Sequence: 2, Status: AvailabilityDecisionRevoked, ReasonCode: "r", ReviewRuleVersion: "v1", RecordedAt: recorded}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.value.Validate(); err == nil {
				t.Fatal("invalid decision unexpectedly passed")
			}
		})
	}
}

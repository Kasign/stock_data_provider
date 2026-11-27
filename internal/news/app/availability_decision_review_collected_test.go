package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func newReviewCollectedForTest(t *testing.T, store AvailabilityDecisionStore, rule AvailabilityReviewRule, version string, now time.Time) *ReviewCollectedAvailability {
	t.Helper()
	review, err := NewReviewAvailabilityDecision(store, rule, func() string { return "decision-next" })
	if err != nil {
		t.Fatal(err)
	}
	review.now = func() time.Time { return now }
	usecase, err := NewReviewCollectedAvailability(review, version)
	if err != nil {
		t.Fatal(err)
	}
	return usecase
}

func TestReviewCollectedAvailabilityGroupsEvidenceByVersion(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{
		version: "cninfo-disclosure-review-v1",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-1": {Outcome: AvailabilityEvidenceReviewEligible, ReasonCode: "availability.source_semantics_verified"},
			"evidence-2": {Outcome: AvailabilityEvidenceReviewEligible, ReasonCode: "availability.source_semantics_verified"},
		},
	}
	store := &availabilityDecisionStoreStub{}
	usecase := newReviewCollectedForTest(t, store, rule, rule.version, recordedAt)

	decisions, err := usecase.Execute(context.Background(), ReviewCollectedAvailabilityRequest{
		Evidence: []domain.AvailabilityEvidence{
			parsedReviewEvidence("evidence-2", "version-2"),
			parsedReviewEvidence("evidence-1", "version-1"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(decisions))
	}
	if decisions[0].VersionID != "version-1" || decisions[1].VersionID != "version-2" {
		t.Fatalf("decision versions = %q,%q, want sorted version-1,version-2", decisions[0].VersionID, decisions[1].VersionID)
	}
	for _, decision := range decisions {
		if decision.Status != domain.AvailabilityDecisionVerified || decision.SelectedEvidenceID == nil {
			t.Fatalf("decision %s = %s without selected evidence", decision.VersionID, decision.Status)
		}
		if decision.ReviewRuleVersion != rule.version {
			t.Fatalf("decision rule version = %q, want %q", decision.ReviewRuleVersion, rule.version)
		}
	}
	if len(store.appended) != 2 {
		t.Fatalf("appended decisions = %d, want 2", len(store.appended))
	}
}

func TestReviewCollectedAvailabilityReviewsAllEvidenceOfOneVersionTogether(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{
		version: "cninfo-disclosure-review-v1",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-a": {Outcome: AvailabilityEvidenceReviewEligible, ReasonCode: "availability.source_semantics_verified"},
			"evidence-b": {Outcome: AvailabilityEvidenceReviewEligible, ReasonCode: "availability.source_semantics_verified"},
		},
	}
	store := &availabilityDecisionStoreStub{}
	usecase := newReviewCollectedForTest(t, store, rule, rule.version, recordedAt)

	decisions, err := usecase.Execute(context.Background(), ReviewCollectedAvailabilityRequest{
		Evidence: []domain.AvailabilityEvidence{
			parsedReviewEvidence("evidence-b", "version-1"),
			parsedReviewEvidence("evidence-a", "version-1"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0].Status != domain.AvailabilityDecisionConflict {
		t.Fatalf("status = %s, want conflict for multiple eligible evidence", decisions[0].Status)
	}
	if len(decisions[0].ConsideredEvidenceIDs) != 2 ||
		decisions[0].ConsideredEvidenceIDs[0] != "evidence-a" ||
		decisions[0].ConsideredEvidenceIDs[1] != "evidence-b" {
		t.Fatalf("considered = %v, want both sorted evidence ids", decisions[0].ConsideredEvidenceIDs)
	}
}

func TestReviewCollectedAvailabilityUsesConfiguredRuleVersion(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{version: "cninfo-disclosure-review-v1"}
	store := &availabilityDecisionStoreStub{}
	usecase := newReviewCollectedForTest(t, store, rule, rule.version, recordedAt)

	_, err := usecase.Execute(context.Background(), ReviewCollectedAvailabilityRequest{
		Evidence: []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-1", "version-1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rule.calls) != 1 {
		t.Fatalf("rule calls = %d, want 1", len(rule.calls))
	}
	if len(store.appended) != 1 || store.appended[0].ReviewRuleVersion != rule.version {
		t.Fatalf("decision rule version did not come from the trusted startup configuration")
	}
}

func TestReviewCollectedAvailabilityRejectsInvalidGroupingAndEvidence(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{version: "cninfo-disclosure-review-v1"}

	foreign := parsedReviewEvidence("evidence-1", "version-1")
	foreign.NewsID = "another-news"
	duplicate := parsedReviewEvidence("evidence-1", "version-1")
	cases := []struct {
		name     string
		evidence []domain.AvailabilityEvidence
	}{
		{"blank version", []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-1", "")}},
		{"inconsistent news identity", []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-1", "version-1"), foreign}},
		{"duplicate evidence id", []domain.AvailabilityEvidence{duplicate, duplicate}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			localStore := &availabilityDecisionStoreStub{}
			localRule := &availabilityReviewRuleStub{version: rule.version}
			localUsecase := newReviewCollectedForTest(t, localStore, localRule, rule.version, recordedAt)
			_, err := localUsecase.Execute(context.Background(), ReviewCollectedAvailabilityRequest{Evidence: tc.evidence})
			if !errors.Is(err, ErrAvailabilityReviewCollectedInput) {
				t.Fatalf("err = %v, want ErrAvailabilityReviewCollectedInput", err)
			}
			if len(localStore.appended) != 0 {
				t.Fatalf("unexpected appended decisions: %d", len(localStore.appended))
			}
		})
	}
}

func TestReviewCollectedAvailabilityPropagatesReviewFailure(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{version: "cninfo-disclosure-review-v1"}
	store := &availabilityDecisionStoreStub{appendErr: errors.New("database unavailable")}
	usecase := newReviewCollectedForTest(t, store, rule, rule.version, recordedAt)

	_, err := usecase.Execute(context.Background(), ReviewCollectedAvailabilityRequest{
		Evidence: []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-1", "version-1")},
	})
	if !errors.Is(err, ErrAvailabilityReviewCollectedInput) {
		t.Fatalf("err = %v, want ErrAvailabilityReviewCollectedInput", err)
	}
}

func TestReviewCollectedAvailabilityEmptyInputIsNoop(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{version: "cninfo-disclosure-review-v1"}
	store := &availabilityDecisionStoreStub{}
	usecase := newReviewCollectedForTest(t, store, rule, rule.version, recordedAt)

	decisions, err := usecase.Execute(context.Background(), ReviewCollectedAvailabilityRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 0 || len(store.appended) != 0 || len(rule.calls) != 0 {
		t.Fatalf("empty input produced decisions=%d appended=%d calls=%d", len(decisions), len(store.appended), len(rule.calls))
	}
}

func TestNewReviewCollectedAvailabilityRequiresDependencies(t *testing.T) {
	rule := &availabilityReviewRuleStub{version: "cninfo-disclosure-review-v1"}
	review, err := NewReviewAvailabilityDecision(&availabilityDecisionStoreStub{}, rule, func() string { return "decision" })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReviewCollectedAvailability(nil, rule.version); !errors.Is(err, ErrAvailabilityReviewCollectedInput) {
		t.Fatalf("nil review err = %v", err)
	}
	if _, err := NewReviewCollectedAvailability(review, ""); !errors.Is(err, ErrAvailabilityReviewCollectedInput) {
		t.Fatalf("blank rule version err = %v", err)
	}
	if _, err := NewReviewCollectedAvailability(review, " bad version"); !errors.Is(err, ErrAvailabilityReviewCollectedInput) {
		t.Fatalf("invalid rule version err = %v", err)
	}
}

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type availabilityReviewRuleStub struct {
	version  string
	outcomes map[string]AvailabilityEvidenceReview
	err      error
	calls    []string
}

func (s *availabilityReviewRuleStub) Version() string { return s.version }

func (s *availabilityReviewRuleStub) Evaluate(_ context.Context, evidence domain.AvailabilityEvidence) (AvailabilityEvidenceReview, error) {
	s.calls = append(s.calls, evidence.ID)
	if s.err != nil {
		return AvailabilityEvidenceReview{}, s.err
	}
	if review, ok := s.outcomes[evidence.ID]; ok {
		return review, nil
	}
	return AvailabilityEvidenceReview{
		Outcome:    AvailabilityEvidenceReviewIneligible,
		ReasonCode: "availability.review_no_rule",
	}, nil
}

type availabilityDecisionStoreStub struct {
	state     AvailabilityDecisionState
	found     bool
	appended  []domain.AvailabilityDecision
	appendErr error
}

func (s *availabilityDecisionStoreStub) LatestAvailabilityDecisionForVersion(_ context.Context, versionID string) (AvailabilityDecisionState, error) {
	if !s.found || (s.state.VersionID != "" && s.state.VersionID != versionID) {
		return AvailabilityDecisionState{}, ErrAvailabilityDecisionNotFound
	}
	return s.state, nil
}

func (s *availabilityDecisionStoreStub) AppendAvailabilityDecision(_ context.Context, decision domain.AvailabilityDecision) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appended = append(s.appended, decision)
	return nil
}

func reviewAvailabilityEvidence(id, versionID string, status domain.AvailabilityTimeParseStatus, candidate *time.Time) domain.AvailabilityEvidence {
	observedAt := time.Date(2026, 11, 27, 2, 0, 0, 0, time.UTC)
	recordedAt := observedAt.Add(time.Minute)
	reasonCode := ""
	if status != domain.AvailabilityTimeParsed {
		reasonCode = "news.available_at.invalid_format"
	}
	evidenceRef := "https://source.example/evidence/" + id
	return domain.AvailabilityEvidence{
		ID:                     id,
		NewsID:                 "news-review-1",
		VersionID:              versionID,
		ObservationID:          "observation-review-1",
		FieldPath:              "data.available_time",
		RawValue:               "2026-11-27T10:00:00+08:00",
		SourceTimezoneOrOffset: "+08:00",
		Precision:              domain.AvailabilityTimePrecisionSecond,
		ParseStatus:            status,
		ParseReasonCode:        reasonCode,
		CandidateAvailableAt:   candidate,
		ParserVersion:          "availability-time-v1",
		EvidenceRef:            &evidenceRef,
		ObservedAt:             observedAt,
		RecordedAt:             recordedAt,
		RetentionUntil:         recordedAt.Add(24 * time.Hour),
	}
}

func parsedReviewEvidence(id, versionID string) domain.AvailabilityEvidence {
	candidate := time.Date(2026, 11, 27, 2, 0, 0, 0, time.UTC)
	return reviewAvailabilityEvidence(id, versionID, domain.AvailabilityTimeParsed, &candidate)
}

func newReviewAvailabilityDecisionForTest(t *testing.T, store AvailabilityDecisionStore, rule AvailabilityReviewRule, now time.Time) *ReviewAvailabilityDecision {
	t.Helper()
	usecase, err := NewReviewAvailabilityDecision(store, rule, func() string { return "decision-next" })
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	return usecase
}

func TestReviewAvailabilityDecisionVerifiesOnlyRuleEligibleEvidence(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	eligible := parsedReviewEvidence("evidence-b", "version-1")
	parsedButNotApproved := parsedReviewEvidence("evidence-a", "version-1")
	previousID := "decision-previous"
	store := &availabilityDecisionStoreStub{
		found: true,
		state: AvailabilityDecisionState{
			VersionID:  "version-1",
			DecisionID: previousID,
			Sequence:   4,
			Status:     domain.AvailabilityDecisionVerified,
		},
	}
	rule := &availabilityReviewRuleStub{
		version: "review-v2",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-b": {
				Outcome:    AvailabilityEvidenceReviewEligible,
				ReasonCode: "availability.source_semantics_verified",
			},
		},
	}
	usecase := newReviewAvailabilityDecisionForTest(t, store, rule, recordedAt)

	decision, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID:         "version-1",
		Evidence:          []domain.AvailabilityEvidence{eligible, parsedButNotApproved},
		ReviewRuleVersion: "review-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != domain.AvailabilityDecisionVerified ||
		decision.SelectedEvidenceID == nil || *decision.SelectedEvidenceID != "evidence-b" {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.Sequence != 5 || decision.SupersedesDecisionID == nil || *decision.SupersedesDecisionID != previousID {
		t.Fatalf("decision chain fields=%+v", decision)
	}
	if len(decision.ConsideredEvidenceIDs) != 2 ||
		decision.ConsideredEvidenceIDs[0] != "evidence-a" || decision.ConsideredEvidenceIDs[1] != "evidence-b" {
		t.Fatalf("considered=%v", decision.ConsideredEvidenceIDs)
	}
	if decision.ReasonCode != "availability.source_semantics_verified" ||
		decision.ReviewRuleVersion != "review-v2" || !decision.RecordedAt.Equal(recordedAt) {
		t.Fatalf("review metadata=%+v", decision)
	}
	if len(store.appended) != 1 || store.appended[0].ID != "decision-next" {
		t.Fatalf("appended=%+v", store.appended)
	}
	if len(rule.calls) != 2 || rule.calls[0] != "evidence-a" || rule.calls[1] != "evidence-b" {
		t.Fatalf("rule calls=%v", rule.calls)
	}
}

func TestReviewAvailabilityDecisionTreatsParsedWithoutRuleEligibilityAsUnresolved(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	store := &availabilityDecisionStoreStub{}
	rule := &availabilityReviewRuleStub{
		version: "review-v2",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-parsed": {
				Outcome:    AvailabilityEvidenceReviewIneligible,
				ReasonCode: "availability.source_semantics_not_verified",
			},
		},
	}
	usecase := newReviewAvailabilityDecisionForTest(t, store, rule, recordedAt)

	decision, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID:         "version-1",
		Evidence:          []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-parsed", "version-1")},
		ReviewRuleVersion: "review-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != domain.AvailabilityDecisionUnresolved || decision.SelectedEvidenceID != nil {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.Sequence != 1 || decision.SupersedesDecisionID != nil {
		t.Fatalf("decision chain fields=%+v", decision)
	}
	if decision.ReasonCode != "availability.review_unresolved" {
		t.Fatalf("reason=%q", decision.ReasonCode)
	}
	if len(store.appended) != 1 {
		t.Fatalf("appended=%d, want 1", len(store.appended))
	}
}

func TestReviewAvailabilityDecisionMarksMultipleEligibleEvidenceAsConflict(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	store := &availabilityDecisionStoreStub{}
	rule := &availabilityReviewRuleStub{
		version: "review-v2",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-a": {Outcome: AvailabilityEvidenceReviewEligible},
			"evidence-b": {Outcome: AvailabilityEvidenceReviewEligible},
		},
	}
	usecase := newReviewAvailabilityDecisionForTest(t, store, rule, recordedAt)

	decision, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID: "version-1",
		Evidence: []domain.AvailabilityEvidence{
			parsedReviewEvidence("evidence-b", "version-1"),
			parsedReviewEvidence("evidence-a", "version-1"),
		},
		ReviewRuleVersion: "review-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != domain.AvailabilityDecisionConflict || decision.SelectedEvidenceID != nil {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.ReasonCode != "availability.review_conflict" {
		t.Fatalf("reason=%q", decision.ReasonCode)
	}
	if len(decision.ConsideredEvidenceIDs) != 2 {
		t.Fatalf("considered=%v", decision.ConsideredEvidenceIDs)
	}
}

func TestReviewAvailabilityDecisionSelectsSinglePreferredEvidence(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	store := &availabilityDecisionStoreStub{}
	rule := &availabilityReviewRuleStub{
		version: "review-v2",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-a": {Outcome: AvailabilityEvidenceReviewEligible, Preferred: true},
			"evidence-b": {Outcome: AvailabilityEvidenceReviewEligible},
		},
	}
	usecase := newReviewAvailabilityDecisionForTest(t, store, rule, recordedAt)

	decision, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID: "version-1",
		Evidence: []domain.AvailabilityEvidence{
			parsedReviewEvidence("evidence-a", "version-1"),
			parsedReviewEvidence("evidence-b", "version-1"),
		},
		ReviewRuleVersion: "review-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != domain.AvailabilityDecisionVerified ||
		decision.SelectedEvidenceID == nil || *decision.SelectedEvidenceID != "evidence-a" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestReviewAvailabilityDecisionMarksRevokedEvidenceAsRevoked(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	store := &availabilityDecisionStoreStub{}
	rule := &availabilityReviewRuleStub{
		version: "review-v2",
		outcomes: map[string]AvailabilityEvidenceReview{
			"evidence-revoked": {
				Outcome:    AvailabilityEvidenceReviewRevoked,
				ReasonCode: "availability.source_revoked",
			},
		},
	}
	usecase := newReviewAvailabilityDecisionForTest(t, store, rule, recordedAt)

	decision, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID:         "version-1",
		Evidence:          []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-revoked", "version-1")},
		ReviewRuleVersion: "review-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != domain.AvailabilityDecisionRevoked || decision.SelectedEvidenceID != nil {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.ReasonCode != "availability.source_revoked" {
		t.Fatalf("reason=%q", decision.ReasonCode)
	}
}

func TestReviewAvailabilityDecisionRejectsForeignPurgedAndRuleMismatch(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	rule := &availabilityReviewRuleStub{version: "review-v2"}
	candidate := time.Date(2026, 11, 27, 2, 0, 0, 0, time.UTC)

	foreign := parsedReviewEvidence("evidence-foreign", "version-other")
	foreignStore := &availabilityDecisionStoreStub{}
	foreignUsecase := newReviewAvailabilityDecisionForTest(t, foreignStore, rule, recordedAt)
	_, err := foreignUsecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID: "version-1", Evidence: []domain.AvailabilityEvidence{foreign}, ReviewRuleVersion: "review-v2",
	})
	if !errors.Is(err, ErrAvailabilityReviewInvalidRequest) || len(foreignStore.appended) != 0 {
		t.Fatalf("foreign evidence error=%v appended=%d", err, len(foreignStore.appended))
	}

	purged := parsedReviewEvidence("evidence-purged", "version-1")
	purged.PayloadPurgedAt = &recordedAt
	purged.RawValue = ""
	purged.CandidateAvailableAt = nil
	purged.EvidenceRef = nil
	purgedStore := &availabilityDecisionStoreStub{}
	purgedUsecase := newReviewAvailabilityDecisionForTest(t, purgedStore, rule, recordedAt)
	_, err = purgedUsecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID: "version-1", Evidence: []domain.AvailabilityEvidence{purged}, ReviewRuleVersion: "review-v2",
	})
	if !errors.Is(err, ErrAvailabilityReviewInvalidRequest) || len(purgedStore.appended) != 0 {
		t.Fatalf("purged evidence error=%v appended=%d", err, len(purgedStore.appended))
	}

	mismatchStore := &availabilityDecisionStoreStub{}
	mismatchUsecase := newReviewAvailabilityDecisionForTest(t, mismatchStore, rule, recordedAt)
	_, err = mismatchUsecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID: "version-1",
		Evidence: []domain.AvailabilityEvidence{
			parsedReviewEvidence("evidence-1", "version-1"),
		},
		ReviewRuleVersion: "review-v1",
	})
	if !errors.Is(err, ErrAvailabilityReviewRuleRejected) || len(mismatchStore.appended) != 0 {
		t.Fatalf("rule mismatch error=%v appended=%d", err, len(mismatchStore.appended))
	}

	ruleErrorStore := &availabilityDecisionStoreStub{}
	ruleError := &availabilityReviewRuleStub{version: "review-v2", err: errors.New("rule unavailable")}
	ruleErrorUsecase := newReviewAvailabilityDecisionForTest(t, ruleErrorStore, ruleError, recordedAt)
	_, err = ruleErrorUsecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID: "version-1",
		Evidence: []domain.AvailabilityEvidence{
			reviewAvailabilityEvidence("evidence-unparsed", "version-1", domain.AvailabilityTimeUnparseable, nil),
		},
		ReviewRuleVersion: "review-v2",
	})
	if !errors.Is(err, ErrAvailabilityReviewRuleRejected) || len(ruleErrorStore.appended) != 0 {
		t.Fatalf("rule error=%v appended=%d", err, len(ruleErrorStore.appended))
	}

	if candidate.After(recordedAt) {
		t.Fatal("unexpected fixture ordering")
	}
}

func TestReviewAvailabilityDecisionPropagatesAppendFailure(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	store := &availabilityDecisionStoreStub{appendErr: errors.New("append failed")}
	rule := &availabilityReviewRuleStub{version: "review-v2"}
	usecase := newReviewAvailabilityDecisionForTest(t, store, rule, recordedAt)

	_, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID:         "version-1",
		Evidence:          []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-1", "version-1")},
		ReviewRuleVersion: "review-v2",
	})
	if !errors.Is(err, ErrAvailabilityDecisionAppendFailed) {
		t.Fatalf("error=%v", err)
	}
}

type sourceBoundAvailabilityReviewRuleStub struct {
	version         string
	expectedBinding AvailabilityEvidenceSourceBinding
	directCalls     int
	boundCalls      int
}

func (s *sourceBoundAvailabilityReviewRuleStub) Version() string { return s.version }

func (s *sourceBoundAvailabilityReviewRuleStub) Evaluate(
	context.Context,
	domain.AvailabilityEvidence,
) (AvailabilityEvidenceReview, error) {
	s.directCalls++
	return AvailabilityEvidenceReview{Outcome: AvailabilityEvidenceReviewIneligible}, nil
}

func (s *sourceBoundAvailabilityReviewRuleStub) EvaluateSourceBound(
	_ context.Context,
	_ domain.AvailabilityEvidence,
	binding AvailabilityEvidenceSourceBinding,
) (AvailabilityEvidenceReview, error) {
	s.boundCalls++
	if binding != s.expectedBinding {
		return AvailabilityEvidenceReview{}, errors.New("source binding mismatch")
	}
	return AvailabilityEvidenceReview{
		Outcome:    AvailabilityEvidenceReviewEligible,
		ReasonCode: "availability.source_bound_verified",
	}, nil
}

type availabilityEvidenceBindingResolverStub struct {
	binding AvailabilityEvidenceSourceBinding
	err     error
	calls   int
}

func (s *availabilityEvidenceBindingResolverStub) ResolveAvailabilityEvidenceSourceBinding(
	context.Context,
	domain.AvailabilityEvidence,
) (AvailabilityEvidenceSourceBinding, error) {
	s.calls++
	if s.err != nil {
		return AvailabilityEvidenceSourceBinding{}, s.err
	}
	return s.binding, nil
}

func TestReviewAvailabilityDecisionUsesSourceBoundRuleWithResolvedBinding(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	binding := testAvailabilityEvidenceSourceBinding()
	binding.EvidenceID = "evidence-source-bound"
	resolver := &availabilityEvidenceBindingResolverStub{binding: binding}
	rule := &sourceBoundAvailabilityReviewRuleStub{version: "review-v2", expectedBinding: binding}
	store := &availabilityDecisionStoreStub{}
	usecase, err := NewReviewAvailabilityDecision(store, rule, func() string { return "decision-next" }, resolver)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return recordedAt }

	decision, err := usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID:         "version-1",
		Evidence:          []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-source-bound", "version-1")},
		ReviewRuleVersion: "review-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != domain.AvailabilityDecisionVerified ||
		decision.ReasonCode != "availability.source_bound_verified" {
		t.Fatalf("decision=%+v", decision)
	}
	if resolver.calls != 1 || rule.boundCalls != 1 || rule.directCalls != 0 {
		t.Fatalf("resolver=%d bound=%d direct=%d", resolver.calls, rule.boundCalls, rule.directCalls)
	}
}

func TestReviewAvailabilityDecisionFailsClosedWhenSourceBindingCannotBeResolved(t *testing.T) {
	recordedAt := time.Date(2026, 11, 27, 2, 10, 0, 0, time.UTC)
	resolver := &availabilityEvidenceBindingResolverStub{err: ErrAvailabilityEvidenceBindingNotFound}
	rule := &sourceBoundAvailabilityReviewRuleStub{version: "review-v2"}
	store := &availabilityDecisionStoreStub{}
	usecase, err := NewReviewAvailabilityDecision(store, rule, func() string { return "decision-next" }, resolver)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return recordedAt }

	_, err = usecase.Execute(context.Background(), ReviewAvailabilityDecisionRequest{
		VersionID:         "version-1",
		Evidence:          []domain.AvailabilityEvidence{parsedReviewEvidence("evidence-1", "version-1")},
		ReviewRuleVersion: "review-v2",
	})
	if !errors.Is(err, ErrAvailabilityEvidenceBindingNotFound) || len(store.appended) != 0 {
		t.Fatalf("error=%v appended=%d", err, len(store.appended))
	}
}

package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type availabilityQualificationCatalogStub struct {
	record      AvailabilitySourceQualification
	err         error
	lookupCalls int
}

func (s *availabilityQualificationCatalogStub) LookupAvailabilitySourceQualification(
	_ context.Context,
	_ AvailabilityEvidenceSourceBinding,
	_ string,
	_ time.Time,
) (AvailabilitySourceQualification, error) {
	s.lookupCalls++
	if s.err != nil {
		return AvailabilitySourceQualification{}, s.err
	}
	return s.record, nil
}

func testAvailabilityEvidenceSourceBinding() AvailabilityEvidenceSourceBinding {
	return AvailabilityEvidenceSourceBinding{
		EvidenceID:                    "evidence-qualified",
		SourceID:                      "source-qualified",
		EndpointID:                    "endpoint-qualified",
		ProviderCode:                  "provider-qualified",
		SourcePolicyVersionID:         "source-policy-v1",
		EndpointPolicyVersionID:       "endpoint-policy-v1",
		ProviderOutputPolicyRef:       "provider-policy-ref",
		ProviderOutputPolicyVersionID: "provider-policy-v1",
	}
}

func testAvailabilitySourceQualification() AvailabilitySourceQualification {
	return AvailabilitySourceQualification{
		QualificationRef:              "availability-qualification-v1",
		SourceID:                      "source-qualified",
		EndpointID:                    "endpoint-qualified",
		ProviderCode:                  "provider-qualified",
		SourcePolicyVersionID:         "source-policy-v1",
		EndpointPolicyVersionID:       "endpoint-policy-v1",
		ProviderOutputPolicyRef:       "provider-policy-ref",
		ProviderOutputPolicyVersionID: "provider-policy-v1",
		FieldPath:                     "data.available_time",
		ParserVersion:                 "availability-time-v1",
		ReviewRuleVersion:             "review-qualified-v1",
		DeclaredTimezoneOrOffset:      "+08:00",
		AllowedPrecisions:             []domain.AvailabilityTimePrecision{domain.AvailabilityTimePrecisionSecond},
		OfficialSemanticsRef:          "https://source.example/docs/available-time#first-public",
		EndpointAuthorizationRef:      "https://source.example/docs/endpoint-authorization",
		VersionBindingRef:             "https://source.example/docs/version-binding-v1",
		RawResponseSampleHash:         strings.Repeat("a", 64),
		Status:                        AvailabilitySourceQualificationQualified,
		EffectiveAt:                   time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestQualifyAvailabilityEvidenceRequiresQualifiedOfficialFieldSemantics(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	catalog := &availabilityQualificationCatalogStub{record: testAvailabilitySourceQualification()}
	usecase, err := NewQualifyAvailabilityEvidence(catalog)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }

	result, err := usecase.Execute(context.Background(), QualifyAvailabilityEvidenceRequest{
		Binding:      testAvailabilityEvidenceSourceBinding(),
		FieldPath:    "data.available_time",
		RawValue:     "2026-11-27T10:00:00+08:00",
		EvidenceRef:  "https://source.example/evidence/1",
		EvidenceHash: strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidate.Status != domain.AvailabilityTimeParsed ||
		result.Candidate.CandidateAvailableAt == nil ||
		!result.Candidate.CandidateAvailableAt.Equal(time.Date(2026, 11, 27, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("candidate=%+v", result.Candidate)
	}
	if result.ParserVersion != "availability-time-v1" || result.ReviewRuleVersion != "review-qualified-v1" {
		t.Fatalf("qualified result=%+v", result)
	}
	if result.EvidenceRef == nil || result.EvidenceHash == nil || catalog.lookupCalls != 1 {
		t.Fatalf("provenance=%v hash=%v lookup_calls=%d", result.EvidenceRef, result.EvidenceHash, catalog.lookupCalls)
	}
}

func TestQualifyAvailabilityEvidenceRejectsUnqualifiedRevokedAndMismatchedFields(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	binding := testAvailabilityEvidenceSourceBinding()
	request := QualifyAvailabilityEvidenceRequest{
		Binding:      binding,
		FieldPath:    "data.available_time",
		RawValue:     "2026-11-27T10:00:00+08:00",
		EvidenceRef:  "https://source.example/evidence/1",
		EvidenceHash: strings.Repeat("b", 64),
	}

	notFound := &availabilityQualificationCatalogStub{err: ErrAvailabilitySourceQualificationNotFound}
	notFoundUsecase, err := NewQualifyAvailabilityEvidence(notFound)
	if err != nil {
		t.Fatal(err)
	}
	notFoundUsecase.now = func() time.Time { return now }
	if _, err := notFoundUsecase.Execute(context.Background(), request); !errors.Is(err, ErrAvailabilitySourceQualificationNotFound) {
		t.Fatalf("unqualified error=%v", err)
	}

	revokedQualification := testAvailabilitySourceQualification()
	revokedQualification.Status = AvailabilitySourceQualificationRevoked
	revokedAt := now.Add(-time.Hour)
	revokedQualification.RevokedAt = &revokedAt
	revokedCatalog := &availabilityQualificationCatalogStub{record: revokedQualification}
	revokedUsecase, err := NewQualifyAvailabilityEvidence(revokedCatalog)
	if err != nil {
		t.Fatal(err)
	}
	revokedUsecase.now = func() time.Time { return now }
	if _, err := revokedUsecase.Execute(context.Background(), request); !errors.Is(err, ErrAvailabilitySourceQualificationDenied) {
		t.Fatalf("revoked error=%v", err)
	}

	wrongTimezone := testAvailabilitySourceQualification()
	wrongTimezone.DeclaredTimezoneOrOffset = "Z"
	wrongTimezoneCatalog := &availabilityQualificationCatalogStub{record: wrongTimezone}
	wrongTimezoneUsecase, err := NewQualifyAvailabilityEvidence(wrongTimezoneCatalog)
	if err != nil {
		t.Fatal(err)
	}
	wrongTimezoneUsecase.now = func() time.Time { return now }
	if _, err := wrongTimezoneUsecase.Execute(context.Background(), request); !errors.Is(err, ErrAvailabilitySourceQualificationDenied) {
		t.Fatalf("timezone mismatch error=%v", err)
	}

	wrongPrecision := testAvailabilitySourceQualification()
	wrongPrecision.AllowedPrecisions = []domain.AvailabilityTimePrecision{domain.AvailabilityTimePrecisionMinute}
	wrongPrecisionCatalog := &availabilityQualificationCatalogStub{record: wrongPrecision}
	wrongPrecisionUsecase, err := NewQualifyAvailabilityEvidence(wrongPrecisionCatalog)
	if err != nil {
		t.Fatal(err)
	}
	wrongPrecisionUsecase.now = func() time.Time { return now }
	if _, err := wrongPrecisionUsecase.Execute(context.Background(), request); !errors.Is(err, ErrAvailabilitySourceQualificationDenied) {
		t.Fatalf("precision mismatch error=%v", err)
	}

	invalidHashRequest := request
	invalidHashRequest.EvidenceHash = strings.Repeat("A", 64)
	invalidHashCatalog := &availabilityQualificationCatalogStub{record: testAvailabilitySourceQualification()}
	invalidHashUsecase, err := NewQualifyAvailabilityEvidence(invalidHashCatalog)
	if err != nil {
		t.Fatal(err)
	}
	invalidHashUsecase.now = func() time.Time { return now }
	if _, err := invalidHashUsecase.Execute(context.Background(), invalidHashRequest); !errors.Is(err, ErrAvailabilitySourceQualificationDenied) {
		t.Fatalf("invalid evidence hash error=%v", err)
	}
}

func TestSourceQualifiedAvailabilityReviewRuleRequiresExactSourceBinding(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	qualification := testAvailabilitySourceQualification()
	catalog := &availabilityQualificationCatalogStub{record: qualification}
	rule, err := NewSourceQualifiedAvailabilityReviewRule(catalog, qualification.ReviewRuleVersion)
	if err != nil {
		t.Fatal(err)
	}
	rule.now = func() time.Time { return now }
	evidence := parsedReviewEvidence("evidence-qualified", "version-1")

	// 未绑定来源的通用 Evaluate 不得把 parsed 提升为 verified。
	generic, err := rule.Evaluate(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if generic.Outcome != AvailabilityEvidenceReviewIneligible {
		t.Fatalf("generic review=%+v", generic)
	}

	qualified, err := rule.EvaluateSourceBound(context.Background(), evidence, testAvailabilityEvidenceSourceBinding())
	if err != nil {
		t.Fatal(err)
	}
	if qualified.Outcome != AvailabilityEvidenceReviewEligible ||
		qualified.ReasonCode != "availability.source_semantics_verified" {
		t.Fatalf("qualified review=%+v", qualified)
	}

	wrongBinding := testAvailabilityEvidenceSourceBinding()
	wrongBinding.SourcePolicyVersionID = "source-policy-v2"
	if _, err := rule.EvaluateSourceBound(context.Background(), evidence, wrongBinding); err != nil {
		t.Fatal(err)
	}
	// Stub catalog 不按 binding 过滤；规则必须自行校验精确绑定。
	review, err := rule.EvaluateSourceBound(context.Background(), evidence, wrongBinding)
	if err != nil || review.Outcome != AvailabilityEvidenceReviewIneligible {
		t.Fatalf("wrong binding review=%+v err=%v", review, err)
	}

	revokedQualification := qualification
	revokedQualification.Status = AvailabilitySourceQualificationRevoked
	revokedAt := now.Add(-time.Minute)
	revokedQualification.RevokedAt = &revokedAt
	revokedCatalog := &availabilityQualificationCatalogStub{record: revokedQualification}
	revokedRule, err := NewSourceQualifiedAvailabilityReviewRule(revokedCatalog, qualification.ReviewRuleVersion)
	if err != nil {
		t.Fatal(err)
	}
	revokedRule.now = func() time.Time { return now }
	revoked, err := revokedRule.EvaluateSourceBound(context.Background(), evidence, testAvailabilityEvidenceSourceBinding())
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Outcome != AvailabilityEvidenceReviewRevoked {
		t.Fatalf("revoked review=%+v", revoked)
	}
}

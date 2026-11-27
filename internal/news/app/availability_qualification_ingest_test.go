package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func g3CNInfoRegistration() SourceEndpointRegistration {
	return SourceEndpointRegistration{
		SourceID:                      "cninfo",
		EndpointID:                    "cninfo-his-announcement",
		Enabled:                       true,
		SourcePolicyVersionID:         "cninfo-source-policy-v1",
		EndpointPolicyVersionID:       "cninfo-endpoint-policy-v1",
		ProviderCode:                  "cninfo",
		ProviderOutputPolicyRef:       "cninfo-provider-output-policy",
		ProviderOutputPolicyVersionID: "cninfo-provider-output-policy-v1",
	}
}

func g3CNInfoQualification() AvailabilitySourceQualification {
	return AvailabilitySourceQualification{
		QualificationRef:              "availability-qualification-cninfo-disclosure-v1",
		SourceID:                      "cninfo",
		EndpointID:                    "cninfo-his-announcement",
		ProviderCode:                  "cninfo",
		SourcePolicyVersionID:         "cninfo-source-policy-v1",
		EndpointPolicyVersionID:       "cninfo-endpoint-policy-v1",
		ProviderOutputPolicyRef:       "cninfo-provider-output-policy",
		ProviderOutputPolicyVersionID: "cninfo-provider-output-policy-v1",
		FieldPath:                     "announcements.announcementTime",
		ParserVersion:                 "cninfo-disclosure-date-conservative-v1",
		ReviewRuleVersion:             "cninfo-disclosure-review-v1",
		DeclaredTimezoneOrOffset:      "+08:00",
		AllowedPrecisions:             []domain.AvailabilityTimePrecision{domain.AvailabilityTimePrecisionDisclosureBoundary},
		OfficialSemanticsRef:          "CSRC指定法定披露渠道：announcementTime为披露日",
		DateBoundarySemanticsRef:      "DEC-PLAT-002-073:disclosure-date-end-of-day-v1",
		EndpointAuthorizationRef:      "personal-use:cninfo-official-api",
		VersionBindingRef:             "NEWS-CTR-001 1.19.1 sample",
		RawResponseSampleHash:         strings.Repeat("e", 64),
		Status:                        AvailabilitySourceQualificationQualified,
		EffectiveAt:                   time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
	}
}

func g3CNInfoCollection() CollectSourceResult {
	collection := CollectSourceResult{
		SourceID:                "cninfo",
		EndpointID:              "cninfo-his-announcement",
		SourcePolicyVersionID:   "cninfo-source-policy-v1",
		EndpointPolicyVersionID: "cninfo-endpoint-policy-v1",
		ResponseHash:            strings.Repeat("d", 64),
		EvidenceRef:             "response-snapshot:cninfo/cninfo-his-announcement@" + strings.Repeat("d", 64),
		Entries: []CollectedSourceEntry{{
			Candidate: domain.Candidate{
				SourceItemID:   "1225649440",
				PublishedAtRaw: "1793376000000",
			},
		}},
	}
	if err := sealCollectSourceResult(&collection); err != nil {
		panic(err)
	}
	return collection
}

func TestQualifyCollectedAvailabilityUsesG1BindingAndDisclosureBoundary(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	catalog := &availabilityQualificationCatalogStub{record: g3CNInfoQualification()}
	qualifier, err := NewQualifyCollectedAvailability(catalog)
	if err != nil {
		t.Fatal(err)
	}
	qualifier.qualify.now = func() time.Time { return now }

	results, err := qualifier.Execute(context.Background(), QualifyCollectedAvailabilityRequest{
		Registration: g3CNInfoRegistration(),
		Collection:   g3CNInfoCollection(),
		FieldPaths:   []string{"announcements.announcementTime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("qualified=%d want 1", len(results))
	}
	got := results[0]
	if got.EntryIndex != 0 || got.SourceItemID != "1225649440" {
		t.Fatalf("qualified entry=%+v", got)
	}
	if got.Binding.SourceID != "cninfo" || got.Binding.EndpointID != "cninfo-his-announcement" ||
		got.Binding.ProviderCode != "cninfo" ||
		got.Binding.SourcePolicyVersionID != "cninfo-source-policy-v1" ||
		got.Binding.EndpointPolicyVersionID != "cninfo-endpoint-policy-v1" ||
		got.Binding.ProviderOutputPolicyRef != "cninfo-provider-output-policy" ||
		got.Binding.ProviderOutputPolicyVersionID != "cninfo-provider-output-policy-v1" {
		t.Fatalf("binding=%+v", got.Binding)
	}
	candidate := got.Qualified.Candidate
	if candidate.Precision != domain.AvailabilityTimePrecisionDisclosureBoundary ||
		candidate.CandidateAvailableAt == nil ||
		!candidate.CandidateAvailableAt.Equal(time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC)) {
		t.Fatalf("candidate=%+v", candidate)
	}
	if got.Qualified.ParserVersion != "cninfo-disclosure-date-conservative-v1" ||
		got.Qualified.ReviewRuleVersion != "cninfo-disclosure-review-v1" {
		t.Fatalf("qualified=%+v", got.Qualified)
	}
	if got.Qualified.EvidenceHash == nil || *got.Qualified.EvidenceHash != strings.Repeat("d", 64) ||
		got.Qualified.EvidenceRef == nil || !strings.HasPrefix(*got.Qualified.EvidenceRef, "response-snapshot:cninfo/") {
		t.Fatalf("provenance=%+v", got.Qualified)
	}
}

func TestQualifyCollectedAvailabilityRejectsWrongProviderBinding(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	catalog := &availabilityQualificationCatalogStub{record: g3CNInfoQualification()}
	qualifier, err := NewQualifyCollectedAvailability(catalog)
	if err != nil {
		t.Fatal(err)
	}
	qualifier.qualify.now = func() time.Time { return now }

	registration := g3CNInfoRegistration()
	registration.ProviderOutputPolicyVersionID = "cninfo-provider-output-policy-v2"
	_, err = qualifier.Execute(context.Background(), QualifyCollectedAvailabilityRequest{
		Registration: registration,
		Collection:   g3CNInfoCollection(),
		FieldPaths:   []string{"announcements.announcementTime"},
	})
	if err == nil {
		t.Fatal("wrong provider policy version must fail closed")
	}
	if !errors.Is(err, ErrAvailabilitySourceQualificationDenied) {
		t.Fatalf("err=%v", err)
	}
}

func TestQualifyCollectedAvailabilitySkipsSourcesWithoutQualification(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	catalog := &availabilityQualificationCatalogStub{err: ErrAvailabilitySourceQualificationNotFound}
	qualifier, err := NewQualifyCollectedAvailability(catalog)
	if err != nil {
		t.Fatal(err)
	}
	qualifier.qualify.now = func() time.Time { return now }

	results, err := qualifier.Execute(context.Background(), QualifyCollectedAvailabilityRequest{
		Registration: g3CNInfoRegistration(),
		Collection:   g3CNInfoCollection(),
		FieldPaths:   []string{"announcements.announcementTime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("unqualified source produced %d candidates", len(results))
	}
}

func TestQualifyCollectedAvailabilityRejectsTamperedCollection(t *testing.T) {
	catalog := &availabilityQualificationCatalogStub{record: g3CNInfoQualification()}
	qualifier, err := NewQualifyCollectedAvailability(catalog)
	if err != nil {
		t.Fatal(err)
	}
	collection := g3CNInfoCollection()
	collection.Entries[0].Candidate.PublishedAtRaw = "1793376000001"
	_, err = qualifier.Execute(context.Background(), QualifyCollectedAvailabilityRequest{
		Registration: g3CNInfoRegistration(),
		Collection:   collection,
		FieldPaths:   []string{"announcements.announcementTime"},
	})
	if err == nil {
		t.Fatal("tampered collection must fail closed")
	}
}

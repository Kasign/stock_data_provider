package source

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

func availabilityQualificationFixture() app.AvailabilitySourceQualification {
	return app.AvailabilitySourceQualification{
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
		Status:                        app.AvailabilitySourceQualificationQualified,
		EffectiveAt:                   time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
}

func availabilityQualificationBinding() app.AvailabilityEvidenceSourceBinding {
	return app.AvailabilityEvidenceSourceBinding{
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

func TestStaticAvailabilityQualificationCatalogRequiresExactVersionBinding(t *testing.T) {
	qualification := availabilityQualificationFixture()
	catalog, err := NewStaticAvailabilityQualificationCatalog([]app.AvailabilitySourceQualification{qualification})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)

	loaded, err := catalog.LookupAvailabilitySourceQualification(
		context.Background(), availabilityQualificationBinding(), "data.available_time", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.QualificationRef != qualification.QualificationRef {
		t.Fatalf("loaded=%+v", loaded)
	}
	loaded.FieldPath = "tampered"
	again, err := catalog.LookupAvailabilitySourceQualification(
		context.Background(), availabilityQualificationBinding(), "data.available_time", now,
	)
	if err != nil || again.FieldPath != "data.available_time" {
		t.Fatalf("catalog returned mutable state: %+v err=%v", again, err)
	}

	wrongBinding := availabilityQualificationBinding()
	wrongBinding.SourcePolicyVersionID = "source-policy-v2"
	if _, err := catalog.LookupAvailabilitySourceQualification(
		context.Background(), wrongBinding, "data.available_time", now,
	); !errors.Is(err, app.ErrAvailabilitySourceQualificationNotFound) {
		t.Fatalf("wrong binding error=%v", err)
	}
	if _, err := catalog.LookupAvailabilitySourceQualification(
		context.Background(), availabilityQualificationBinding(), "data.other_time", now,
	); !errors.Is(err, app.ErrAvailabilitySourceQualificationNotFound) {
		t.Fatalf("wrong field error=%v", err)
	}
	if _, err := catalog.LookupAvailabilitySourceQualification(
		context.Background(), availabilityQualificationBinding(), "data.available_time", qualification.EffectiveAt.Add(-time.Second),
	); !errors.Is(err, app.ErrAvailabilitySourceQualificationNotFound) {
		t.Fatalf("before effective_at error=%v", err)
	}
}

func TestStaticAvailabilityQualificationCatalogSelectsLatestEffectiveQualification(t *testing.T) {
	first := availabilityQualificationFixture()
	second := first
	second.QualificationRef = "availability-qualification-v2"
	second.ParserVersion = "availability-time-v2"
	second.EffectiveAt = first.EffectiveAt.Add(24 * time.Hour)
	catalog, err := NewStaticAvailabilityQualificationCatalog([]app.AvailabilitySourceQualification{second, first})
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := catalog.LookupAvailabilitySourceQualification(
		context.Background(), availabilityQualificationBinding(), "data.available_time", second.EffectiveAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.QualificationRef != second.QualificationRef || loaded.ParserVersion != second.ParserVersion {
		t.Fatalf("loaded=%+v", loaded)
	}
}

func TestStaticAvailabilityQualificationCatalogRejectsInvalidSnapshots(t *testing.T) {
	qualification := availabilityQualificationFixture()
	duplicate := qualification
	if _, err := NewStaticAvailabilityQualificationCatalog([]app.AvailabilitySourceQualification{qualification, duplicate}); err == nil {
		t.Fatal("duplicate qualification ref accepted")
	}
	invalid := qualification
	invalid.AllowedPrecisions = nil
	if _, err := NewStaticAvailabilityQualificationCatalog([]app.AvailabilitySourceQualification{invalid}); err == nil {
		t.Fatal("invalid qualification accepted")
	}

	empty, err := NewStaticAvailabilityQualificationCatalog(nil)
	if err != nil {
		t.Fatalf("empty catalog should be valid and fail closed: %v", err)
	}
	if _, err := empty.LookupAvailabilitySourceQualification(
		context.Background(), availabilityQualificationBinding(), "data.available_time", time.Now().UTC(),
	); !errors.Is(err, app.ErrAvailabilitySourceQualificationNotFound) {
		t.Fatalf("empty catalog error=%v", err)
	}
}

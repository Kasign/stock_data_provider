package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func g4Registration() SourceEndpointRegistration {
	registration := sourcePersistenceRegistration()
	registration.SourceID = "cninfo"
	registration.EndpointID = "cninfo-his-announcement"
	registration.AccessMethod = domain.AccessOfficialAPI
	registration.SourcePolicyVersionID = "cninfo-source-policy-v1"
	registration.SourcePolicy.AllowedAccessMethods = []domain.AccessMethod{domain.AccessOfficialAPI}
	registration.SourcePolicy.AllowedMetadataFields = []string{"announcements.announcementTime"}
	registration.EndpointPolicyVersionID = "cninfo-endpoint-policy-v1"
	registration.EndpointPolicy = &domain.EndpointPolicy{EvidenceRef: "endpoint-authorization"}
	registration.ProviderCode = "cninfo"
	registration.ProviderOutputPolicyRef = "cninfo-provider-output-policy"
	registration.ProviderOutputPolicyVersionID = "cninfo-provider-output-policy-v1"
	return registration
}

func g4ProviderPolicy() domain.ProviderOutputPolicy {
	return domain.ProviderOutputPolicy{
		Status:                  domain.AuthorizationApproved,
		AllowedPurposes:         []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:     []domain.ContentKind{domain.ContentMetadata},
		AllowRequestProcessing:  true,
		AllowedStorageModes:     []domain.StorageMode{domain.StoragePersistentMetadata},
		AllowedMetadataFields:   []string{"announcements.announcementTime"},
		MaxPersistenceRetention: 24 * time.Hour,
		EffectiveAt:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EvidenceRef:             "personal-use:cninfo-official-api",
	}
}

func g4RecordFixture(t *testing.T, now time.Time) (
	SourceEndpointRegistration,
	*memoryIdentityRepository,
	*RecordAvailabilityEvidence,
	[]QualifiedCollectedAvailability,
	[]IngestCollectedSourceResult,
) {
	t.Helper()
	registration := g4Registration()
	repository := newSourcePersistenceMemoryRepository(registration)
	repository.providerOutputPolicy = g4ProviderPolicy()
	repository.providerOutputPolicyRef = registration.ProviderOutputPolicyRef
	repository.providerOutputPolicyVersionID = registration.ProviderOutputPolicyVersionID

	qualifier, err := NewQualifyCollectedAvailability(&availabilityQualificationCatalogStub{record: g3CNInfoQualification()})
	if err != nil {
		t.Fatal(err)
	}
	qualifier.qualify.now = func() time.Time { return now.Add(-time.Hour) }
	qualified, err := qualifier.Execute(context.Background(), QualifyCollectedAvailabilityRequest{
		Registration: registration,
		Collection:   g3CNInfoCollection(),
		FieldPaths:   []string{"announcements.announcementTime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(qualified) != 1 {
		t.Fatalf("qualified=%d want 1", len(qualified))
	}

	observedAt := now
	observation := domain.Observation{
		ID: "observation-g4", NewsID: "news-g4", SourceID: registration.SourceID,
		EndpointID: registration.EndpointID, ProviderCode: registration.ProviderCode,
		SourcePolicyVersionID:         registration.SourcePolicyVersionID,
		EndpointPolicyVersionID:       registration.EndpointPolicyVersionID,
		ProviderOutputPolicyRef:       registration.ProviderOutputPolicyRef,
		ProviderOutputPolicyVersionID: registration.ProviderOutputPolicyVersionID,
		Candidate:                     domain.Candidate{SourceItemID: "1225649440"},
		ObservedAt:                    observedAt,
		RetentionUntil:                observedAt.Add(24 * time.Hour),
	}
	version := domain.NewsItemVersion{
		ID: "version-g4", NewsID: observation.NewsID, VersionNo: 1,
		ObservationID: observation.ID, ObservedAt: observedAt,
	}
	recorder, err := NewRecordAvailabilityEvidence(repository,
		providerIngestCatalogStub{registration: registration},
		providerIngestPolicyStub{binding: ProviderOutputPolicyBinding{
			ProviderCode:  registration.ProviderCode,
			PolicyRef:     registration.ProviderOutputPolicyRef,
			PolicyVersion: registration.ProviderOutputPolicyVersionID,
		}},
		func() string { return "unused-random-evidence-id" },
	)
	if err != nil {
		t.Fatal(err)
	}
	recorder.now = func() time.Time { return now.Add(time.Second) }
	return registration, repository, recorder, qualified, []IngestCollectedSourceResult{{
		Observation: observation, Version: version,
	}}
}

func TestRecordCollectedAvailabilityBindsQualifiedCandidateToPersistedVersion(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	_, repository, recorder, qualified, ingested := g4RecordFixture(t, now)
	usecase, err := NewRecordCollectedAvailability(recorder)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := usecase.Execute(context.Background(), RecordCollectedAvailabilityRequest{
		Qualified: qualified, Ingest: ingested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || len(repository.availabilityEvidence) != 1 {
		t.Fatalf("evidence=%d stored=%d", len(evidence), len(repository.availabilityEvidence))
	}
	stored := repository.availabilityEvidence[0]
	if stored.ID == "" || stored.ID == "unused-random-evidence-id" ||
		stored.VersionID != ingested[0].Version.ID || stored.ObservationID != ingested[0].Observation.ID ||
		stored.FieldPath != "announcements.announcementTime" ||
		stored.RawValue != "1793376000000" {
		t.Fatalf("stored evidence=%+v", stored)
	}
	if stored.CandidateAvailableAt == nil ||
		!stored.CandidateAvailableAt.Equal(time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC)) {
		t.Fatalf("candidate=%+v", stored.CandidateAvailableAt)
	}
	if stored.EvidenceHash == nil || *stored.EvidenceHash != strings.Repeat("d", 64) ||
		stored.EvidenceRef == nil || !strings.HasPrefix(*stored.EvidenceRef, "response-snapshot:cninfo/") {
		t.Fatalf("provenance=%+v", stored)
	}
}

func TestRecordCollectedAvailabilityRejectsWrongEntryBinding(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	_, repository, recorder, qualified, ingested := g4RecordFixture(t, now)
	usecase, err := NewRecordCollectedAvailability(recorder)
	if err != nil {
		t.Fatal(err)
	}
	qualified[0].EntryIndex = 1
	_, err = usecase.Execute(context.Background(), RecordCollectedAvailabilityRequest{
		Qualified: qualified, Ingest: ingested,
	})
	if !errors.Is(err, ErrAvailabilityRecordInput) || len(repository.availabilityEvidence) != 0 {
		t.Fatalf("err=%v stored=%d", err, len(repository.availabilityEvidence))
	}
}

func TestRecordCollectedAvailabilityRejectsWrongSourceItemBinding(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	_, repository, recorder, qualified, ingested := g4RecordFixture(t, now)
	usecase, err := NewRecordCollectedAvailability(recorder)
	if err != nil {
		t.Fatal(err)
	}
	qualified[0].SourceItemID = "other-item"
	_, err = usecase.Execute(context.Background(), RecordCollectedAvailabilityRequest{
		Qualified: qualified, Ingest: ingested,
	})
	if !errors.Is(err, ErrAvailabilityRecordInput) || len(repository.availabilityEvidence) != 0 {
		t.Fatalf("err=%v stored=%d", err, len(repository.availabilityEvidence))
	}
}

func TestRecordCollectedAvailabilityRejectsUnpersistedVersionBinding(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	_, repository, recorder, qualified, ingested := g4RecordFixture(t, now)
	usecase, err := NewRecordCollectedAvailability(recorder)
	if err != nil {
		t.Fatal(err)
	}
	ingested[0].Version.ObservationID = "observation-from-another-version"
	_, err = usecase.Execute(context.Background(), RecordCollectedAvailabilityRequest{
		Qualified: qualified, Ingest: ingested,
	})
	if !errors.Is(err, ErrAvailabilityRecordInput) || len(repository.availabilityEvidence) != 0 {
		t.Fatalf("err=%v stored=%d", err, len(repository.availabilityEvidence))
	}
}

func TestRecordCollectedAvailabilityReplayIsIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	_, repository, recorder, qualified, ingested := g4RecordFixture(t, now)
	usecase, err := NewRecordCollectedAvailability(recorder)
	if err != nil {
		t.Fatal(err)
	}
	request := RecordCollectedAvailabilityRequest{Qualified: qualified, Ingest: ingested}
	first, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	recorder.now = func() time.Time { return now.Add(5 * time.Minute) }
	second, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || len(repository.availabilityEvidence) != 1 ||
		first[0].ID != second[0].ID || first[0].ID != repository.availabilityEvidence[0].ID {
		t.Fatalf("first=%+v second=%+v stored=%+v", first, second, repository.availabilityEvidence)
	}
}

func TestRecordAvailabilityEvidenceRejectsArbitraryEvidenceID(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	_, repository, recorder, _, ingested := g4RecordFixture(t, now)
	candidate := domain.ParseAvailabilityTime("2026-10-31T23:59:59+08:00")
	_, err := recorder.Execute(context.Background(), RecordAvailabilityEvidenceRequest{
		Observation: ingested[0].Observation, Version: ingested[0].Version,
		FieldPath: "announcements.announcementTime", Candidate: candidate,
		ParserVersion: "cninfo-disclosure-date-conservative-v1", EvidenceID: "operator-chosen-id",
	})
	if !errors.Is(err, ErrInvalidRequest) || len(repository.availabilityEvidence) != 0 {
		t.Fatalf("err=%v stored=%d", err, len(repository.availabilityEvidence))
	}
}

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func TestRecordAvailabilityEvidenceRequiresFieldAndPolicyPermission(t *testing.T) {
	registration := sourcePersistenceRegistration()
	registration.SourcePolicy.AllowedMetadataFields = []string{"payload.publishTime"}
	repository := newSourcePersistenceMemoryRepository(registration)
	observedAt := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	now := observedAt.Add(time.Minute)
	observation := domain.Observation{
		ID: "observation-available-at", NewsID: "news-available-at",
		SourceID: registration.SourceID, EndpointID: registration.EndpointID,
		SourcePolicyVersionID: registration.SourcePolicyVersionID,
		ObservedAt:            observedAt, RetentionUntil: now.Add(2 * time.Hour),
	}
	version := domain.NewsItemVersion{
		ID: "version-available-at", NewsID: observation.NewsID, VersionNo: 1,
		ObservationID: observation.ID, ObservedAt: observedAt,
	}
	usecase, err := NewRecordAvailabilityEvidence(repository,
		providerIngestCatalogStub{registration: registration}, nil,
		func() string { return "availability-evidence-1" })
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	candidate := domain.ParseAvailabilityTime("2026-11-16T10:00:00+08:00")
	evidence, err := usecase.Execute(context.Background(), RecordAvailabilityEvidenceRequest{
		Observation: observation, Version: version, FieldPath: "payload.publishTime",
		Candidate: candidate, ParserVersion: "news-availability/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ID != "availability-evidence-1" || evidence.CandidateAvailableAt == nil ||
		evidence.RetentionUntil.After(observation.RetentionUntil) ||
		len(repository.availabilityEvidence) != 1 {
		t.Fatalf("evidence=%+v stored=%+v", evidence, repository.availabilityEvidence)
	}

	deniedRegistration := sourcePersistenceRegistration()
	deniedRegistration.SourcePolicy.AllowedMetadataFields = []string{"payload.title"}
	deniedRepository := newSourcePersistenceMemoryRepository(deniedRegistration)
	deniedUsecase, err := NewRecordAvailabilityEvidence(deniedRepository,
		providerIngestCatalogStub{registration: deniedRegistration}, nil,
		func() string { return "must-not-write" })
	if err != nil {
		t.Fatal(err)
	}
	deniedUsecase.now = func() time.Time { return now }
	_, err = deniedUsecase.Execute(context.Background(), RecordAvailabilityEvidenceRequest{
		Observation: observation, Version: version, FieldPath: "payload.publishTime",
		Candidate: candidate, ParserVersion: "news-availability/1",
	})
	if !errors.Is(err, ErrAvailabilityEvidencePolicyDenied) || len(deniedRepository.availabilityEvidence) != 0 {
		t.Fatalf("denied error=%v stored=%+v", err, deniedRepository.availabilityEvidence)
	}
}

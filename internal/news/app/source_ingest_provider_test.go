package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type sourceIngestProviderPolicyStub struct {
	binding ProviderOutputPolicyBinding
	err     error
}

func (s sourceIngestProviderPolicyStub) ResolveProviderOutputPolicy(
	_ context.Context, providerCode string,
) (ProviderOutputPolicyBinding, error) {
	if s.err != nil {
		return ProviderOutputPolicyBinding{}, s.err
	}
	if s.binding.ProviderCode != providerCode {
		return ProviderOutputPolicyBinding{}, ErrProviderOutputPolicyUnavailable
	}
	return s.binding, nil
}

func sourceIngestProviderRegistration() SourceEndpointRegistration {
	registration := sourcePersistenceRegistration()
	registration.EndpointID = "cninfo-his-announcement"
	registration.AccessMethod = domain.AccessOfficialAPI
	registration.SourcePolicy.AllowedAccessMethods = []domain.AccessMethod{domain.AccessOfficialAPI}
	registration.SourcePolicy.AllowedMetadataFields = []string{"announcements.announcementTime"}
	registration.EndpointPolicy = &domain.EndpointPolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessOfficialAPI},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentAbstract, domain.ContentMetadata},
		AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentMetadata, domain.StoragePersistentExcerpt},
		AllowedMetadataFields: []string{
			"announcements.announcementTime",
		},
		EffectiveAt: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		EvidenceRef: "personal-use:cninfo-endpoint",
	}
	registration.EndpointPolicyVersionID = "cninfo-endpoint-policy-v1"
	registration.ProviderCode = "cninfo"
	registration.ProviderOutputPolicyRef = "cninfo-provider-output-policy"
	registration.ProviderOutputPolicyVersionID = "cninfo-provider-output-policy-v1"
	return registration
}

func sourceIngestProviderPolicy() domain.ProviderOutputPolicy {
	return domain.ProviderOutputPolicy{
		Status:                  domain.AuthorizationApproved,
		AllowedPurposes:         []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:     []domain.ContentKind{domain.ContentAbstract, domain.ContentMetadata},
		AllowRequestProcessing:  true,
		MaxRequestRetention:     24 * time.Hour,
		AllowedStorageModes:     []domain.StorageMode{domain.StoragePersistentMetadata, domain.StoragePersistentExcerpt},
		AllowedMetadataFields:   []string{"announcements.announcementTime"},
		MaxPersistenceRetention: 24 * time.Hour,
		EffectiveAt:             time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		EvidenceRef:             "personal-use:cninfo-provider-output",
	}
}

func sourceIngestProviderBinding() ProviderOutputPolicyBinding {
	return ProviderOutputPolicyBinding{
		ProviderCode:  "cninfo",
		PolicyRef:     "cninfo-provider-output-policy",
		PolicyVersion: "cninfo-provider-output-policy-v1",
		Policy:        sourceIngestProviderPolicy(),
	}
}

func TestIngestCollectedSourceWritesRegisteredProviderBinding(t *testing.T) {
	registration := sourceIngestProviderRegistration()
	repository := newSourcePersistenceMemoryRepository(registration)
	repository.providerOutputPolicy = sourceIngestProviderPolicy()
	repository.providerOutputPolicyRef = registration.ProviderOutputPolicyRef
	repository.providerOutputPolicyVersionID = registration.ProviderOutputPolicyVersionID
	ingest := newCandidateIngest(repository, func() string { return "observation-cninfo-1" })
	usecase, err := NewIngestCollectedSourceWithProviderPolicies(
		ingest.repository, ingest.newID,
		providerIngestCatalogStub{registration: registration},
		sourceIngestProviderPolicyStub{binding: sourceIngestProviderBinding()},
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC)
	usecase.now = func() time.Time { return now }
	result := sealedCollectedSourceResult()
	result.EndpointID = registration.EndpointID
	result.EndpointPolicyVersionID = registration.EndpointPolicyVersionID
	result.Entries = []CollectedSourceEntry{{
		Candidate: domain.Candidate{SourceItemID: "announcement-1", Title: "授权摘要"},
	}}
	if err := sealCollectSourceResult(&result); err != nil {
		t.Fatal(err)
	}
	observations, err := usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "cninfo-collection-1",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("observations=%d want 1", len(observations))
	}
	got := observations[0]
	if got.ProviderCode != "cninfo" || got.ProviderOutputPolicyRef != "cninfo-provider-output-policy" ||
		got.ProviderOutputPolicyVersionID != "cninfo-provider-output-policy-v1" {
		t.Fatalf("provider binding not persisted: %+v", got)
	}
	if got.EndpointPolicyVersionID != registration.EndpointPolicyVersionID {
		t.Fatalf("endpoint binding not persisted: %+v", got)
	}
}

func TestIngestCollectedSourceRejectsProviderBindingMismatch(t *testing.T) {
	registration := sourceIngestProviderRegistration()
	repository := newSourcePersistenceMemoryRepository(registration)
	repository.providerOutputPolicy = sourceIngestProviderPolicy()
	repository.providerOutputPolicyRef = registration.ProviderOutputPolicyRef
	repository.providerOutputPolicyVersionID = registration.ProviderOutputPolicyVersionID
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
	binding := sourceIngestProviderBinding()
	binding.PolicyVersion = "cninfo-provider-output-policy-v2"
	usecase, err := NewIngestCollectedSourceWithProviderPolicies(
		ingest.repository, ingest.newID,
		providerIngestCatalogStub{registration: registration},
		sourceIngestProviderPolicyStub{binding: binding},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := sealedCollectedSourceResult()
	result.EndpointID = registration.EndpointID
	result.EndpointPolicyVersionID = registration.EndpointPolicyVersionID
	if err := sealCollectSourceResult(&result); err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "cninfo-mismatch",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, ErrProviderOutputPolicyUnavailable) {
		t.Fatalf("provider binding mismatch error=%v, want ErrProviderOutputPolicyUnavailable", err)
	}
	if len(repository.observations) != 0 {
		t.Fatalf("mismatched provider binding persisted observations: %+v", repository.observations)
	}
}

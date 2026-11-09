package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type providerIngestCatalogStub struct {
	registration SourceEndpointRegistration
	err error
}

func (s providerIngestCatalogStub) LookupEndpoint(context.Context, string, string) (SourceEndpointRegistration, error) {
	return s.registration, s.err
}

type providerIngestPolicyStub struct {
	binding ProviderOutputPolicyBinding
	err error
}

func (s providerIngestPolicyStub) ResolveProviderOutputPolicy(context.Context, string) (ProviderOutputPolicyBinding, error) {
	return s.binding, s.err
}

func validProviderIngestRegistration() SourceEndpointRegistration {
	return SourceEndpointRegistration{
		SourceID: "source-1", EndpointID: "endpoint-1", Enabled: true,
		AccessMethod: domain.AccessRSS, URL: "https://news.example/feed",
		URLPolicy: domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}},
		SourcePolicyVersionID: "source-policy-v2",
		SourcePolicy: domain.SourcePolicy{
			Status: domain.AuthorizationApproved,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes: []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds: []domain.ContentKind{domain.ContentSnippet},
			AllowedStorageModes: []domain.StorageMode{domain.StorageTransient},
			MaxRetention: time.Hour, EvidenceRef: "source-evidence-v2",
		},
	}
}

func validProviderIngestPolicy() ProviderOutputPolicyBinding {
	return ProviderOutputPolicyBinding{
		ProviderCode: "brave", PolicyRef: "brave-output", PolicyVersion: "policy-v4",
		Policy: domain.ProviderOutputPolicy{
			Status: domain.AuthorizationApproved,
			AllowedPurposes: []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds: []domain.ContentKind{domain.ContentSnippet},
			AllowRequestProcessing: true, MaxRequestRetention: time.Hour,
			EvidenceRef: "provider-evidence-v4",
		},
	}
}

func TestIngestProviderCandidateEvaluatesTrustedPolicyBeforePersistence(t *testing.T) {
	repository := &memoryIdentityRepository{}
	ingest := NewIngestCandidate(repository, func() string { return "observation-provider-allowed" })
	usecase, err := NewIngestProviderCandidate(ingest, providerIngestCatalogStub{
		registration: validProviderIngestRegistration(),
	}, providerIngestPolicyStub{binding: validProviderIngestPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	evaluatedAt := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	usecase.now = func() time.Time { return evaluatedAt }

	observation, err := usecase.Execute(context.Background(), IngestProviderCandidateRequest{
		SourceID: "source-1", EndpointID: "endpoint-1", IdempotencyKey: "run-1/item-1",
		Candidate: domain.Candidate{ProviderCode: "brave", SourceItemID: "native-1", Title: "headline"},
		Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
		StorageMode: domain.StorageTransient, Retention: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if observation.ProviderOutputPolicyRef != "brave-output" ||
		observation.ProviderOutputPolicyVersionID != "policy-v4" ||
		observation.SourcePolicyVersionID != "source-policy-v2" ||
		observation.PolicyEvaluatedAt != evaluatedAt {
		t.Fatalf("policy trace not persisted: %+v", observation)
	}
	if len(repository.observations) != 1 || repository.observations[0].ID != observation.ID {
		t.Fatalf("observation was not persisted: %+v", repository)
	}
}

func TestIngestProviderCandidateFailsClosedBeforePersistence(t *testing.T) {
	tests := []struct {
		name string
		registration SourceEndpointRegistration
		policy ProviderOutputPolicyBinding
		policyErr error
		want error
	}{
		{
			name: "provider policy denied",
			registration: validProviderIngestRegistration(),
			policy: func() ProviderOutputPolicyBinding {
				value := validProviderIngestPolicy()
				value.Policy.AllowRequestProcessing = false
				return value
			}(),
			want: ErrProviderOutputPolicyDenied,
		},
		{
			name: "provider policy unavailable",
			registration: validProviderIngestRegistration(),
			policyErr: errors.New("catalog unavailable"),
			want: ErrProviderOutputPolicyUnavailable,
		},
		{
			name: "source policy version missing",
			registration: func() SourceEndpointRegistration {
				value := validProviderIngestRegistration()
				value.SourcePolicyVersionID = ""
				return value
			}(),
			policy: validProviderIngestPolicy(),
			want: ErrProviderOutputPolicyUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryIdentityRepository{}
			ingest := NewIngestCandidate(repository, func() string { return "must-not-persist" })
			usecase, err := NewIngestProviderCandidate(ingest,
				providerIngestCatalogStub{registration: test.registration},
				providerIngestPolicyStub{binding: test.policy, err: test.policyErr},
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = usecase.Execute(context.Background(), IngestProviderCandidateRequest{
				SourceID: "source-1", EndpointID: "endpoint-1", IdempotencyKey: "run-1/item-1",
				Candidate: domain.Candidate{ProviderCode: "brave", SourceItemID: "native-1"},
				Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
				StorageMode: domain.StorageTransient, Retention: time.Minute,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if len(repository.observations) != 0 || len(repository.items) != 0 {
				t.Fatalf("denied candidate reached persistence: %+v", repository)
			}
		})
	}
}

func TestIngestProviderCandidateRejectsProviderIdentityMismatch(t *testing.T) {
	repository := &memoryIdentityRepository{}
	ingest := NewIngestCandidate(repository, func() string { return "must-not-persist" })
	binding := validProviderIngestPolicy()
	binding.ProviderCode = "tavily"
	usecase, err := NewIngestProviderCandidate(ingest,
		providerIngestCatalogStub{registration: validProviderIngestRegistration()},
		providerIngestPolicyStub{binding: binding},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Execute(context.Background(), IngestProviderCandidateRequest{
		SourceID: "source-1", EndpointID: "endpoint-1", IdempotencyKey: "run-1/item-1",
		Candidate: domain.Candidate{ProviderCode: "brave", SourceItemID: "native-1"},
		Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
		StorageMode: domain.StorageTransient, Retention: time.Minute,
	})
	if !errors.Is(err, ErrProviderOutputPolicyUnavailable) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}

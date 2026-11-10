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
	err          error
	afterLookup  func()
}

func (s providerIngestCatalogStub) LookupEndpoint(context.Context, string, string) (SourceEndpointRegistration, error) {
	if s.afterLookup != nil {
		s.afterLookup()
	}
	return s.registration, s.err
}

type providerIngestPolicyStub struct {
	binding      ProviderOutputPolicyBinding
	err          error
	afterResolve func()
}

func (s providerIngestPolicyStub) ResolveProviderOutputPolicy(context.Context, string) (ProviderOutputPolicyBinding, error) {
	if s.afterResolve != nil {
		s.afterResolve()
	}
	return s.binding, s.err
}

func validProviderIngestRegistration() SourceEndpointRegistration {
	return SourceEndpointRegistration{
		SourceID: "source-1", EndpointID: "endpoint-1", Enabled: true,
		AccessMethod: domain.AccessRSS, URL: "https://news.example/feed",
		URLPolicy:             domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}},
		SourcePolicyVersionID: "source-policy-v2",
		SourcePolicy: domain.SourcePolicy{
			Status:               domain.AuthorizationApproved,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:  []domain.ContentKind{domain.ContentSnippet},
			AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient},
			MaxRetention:         time.Hour, EvidenceRef: "source-evidence-v2",
		},
	}
}

func validProviderIngestPolicy() ProviderOutputPolicyBinding {
	return ProviderOutputPolicyBinding{
		ProviderCode: "brave", PolicyRef: "brave-output", PolicyVersion: "policy-v4",
		Policy: domain.ProviderOutputPolicy{
			Status:                 domain.AuthorizationApproved,
			AllowedPurposes:        []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:    []domain.ContentKind{domain.ContentSnippet},
			AllowRequestProcessing: true, MaxRequestRetention: time.Hour,
			EvidenceRef: "provider-evidence-v4",
		},
	}
}

func TestIngestProviderCandidateEvaluatesTrustedPolicyBeforePersistence(t *testing.T) {
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "observation-provider-allowed" })
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
		Purpose:   domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
		StorageMode: domain.StorageTransient, Retention: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if observation.RetentionUntil.IsZero() || observation.RetentionUntil.Sub(observation.ObservedAt) != 5*time.Minute {
		t.Fatalf("authorized provider retention was not persisted: %+v", observation)
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

func TestIngestProviderCandidateRejectsNonPositiveRetentionBeforePolicyLookup(t *testing.T) {
	for _, retention := range []time.Duration{0, -time.Minute} {
		repository := &memoryIdentityRepository{}
		ingest := newCandidateIngest(repository, func() string { return "must-not-persist-nonpositive-retention" })
		policies := providerIngestPolicyStub{binding: validProviderIngestPolicy()}
		usecase, err := NewIngestProviderCandidate(ingest,
			providerIngestCatalogStub{registration: validProviderIngestRegistration()}, policies)
		if err != nil {
			t.Fatal(err)
		}
		_, err = usecase.Execute(context.Background(), IngestProviderCandidateRequest{
			SourceID: "source-1", EndpointID: "endpoint-1", IdempotencyKey: "nonpositive-retention",
			Candidate: domain.Candidate{ProviderCode: "brave", SourceItemID: "native-1"},
			Purpose:   domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
			StorageMode: domain.StorageTransient, Retention: retention,
		})
		if !errors.Is(err, ErrInvalidRequest) || len(repository.observations) != 0 {
			t.Fatalf("retention=%s must be rejected before persistence: err=%v repository=%+v", retention, err, repository)
		}
	}
}

func TestIngestProviderCandidateFailsClosedBeforePersistence(t *testing.T) {
	tests := []struct {
		name         string
		registration SourceEndpointRegistration
		policy       ProviderOutputPolicyBinding
		policyErr    error
		want         error
	}{
		{
			name:         "provider policy denied",
			registration: validProviderIngestRegistration(),
			policy: func() ProviderOutputPolicyBinding {
				value := validProviderIngestPolicy()
				value.Policy.AllowRequestProcessing = false
				return value
			}(),
			want: ErrProviderOutputPolicyDenied,
		},
		{
			name:         "provider policy unavailable",
			registration: validProviderIngestRegistration(),
			policyErr:    errors.New("catalog unavailable"),
			want:         ErrProviderOutputPolicyUnavailable,
		},
		{
			name: "source policy version missing",
			registration: func() SourceEndpointRegistration {
				value := validProviderIngestRegistration()
				value.SourcePolicyVersionID = ""
				return value
			}(),
			policy: validProviderIngestPolicy(),
			want:   ErrProviderOutputPolicyUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryIdentityRepository{}
			ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
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
		Purpose:   domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
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
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
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
				Purpose:   domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
		StorageMode: domain.StorageTransient, Retention: time.Minute,
	})
	if !errors.Is(err, ErrProviderOutputPolicyUnavailable) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}

func TestIngestProviderCandidatePreservesSourceCatalogFailureClassification(t *testing.T) {
	catalogFailure := errors.New("database timeout")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "not found", err: ErrSourceEndpointNotFound, want: ErrSourceEndpointNotFound},
		{name: "wrapped not found", err: errors.Join(errors.New("lookup"), ErrSourceEndpointNotFound), want: ErrSourceEndpointNotFound},
		{name: "database failure", err: catalogFailure, want: ErrSourceCatalogUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryIdentityRepository{}
			ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
			usecase, err := NewIngestProviderCandidate(ingest,
				providerIngestCatalogStub{err: test.err},
				providerIngestPolicyStub{binding: validProviderIngestPolicy()},
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = usecase.Execute(context.Background(), IngestProviderCandidateRequest{
				SourceID: "source-1", EndpointID: "endpoint-1", IdempotencyKey: "catalog-error",
				Candidate: domain.Candidate{ProviderCode: "brave", SourceItemID: "native-1"},
				Purpose:   domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
				StorageMode: domain.StorageTransient, Retention: time.Minute,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want errors.Is(_, %v)", err, test.want)
			}
			if test.name == "database failure" && !errors.Is(err, catalogFailure) {
				t.Fatalf("catalog cause was not preserved: %v", err)
			}
			if len(repository.observations) != 0 || len(repository.items) != 0 {
				t.Fatalf("catalog failure reached persistence: %+v", repository)
			}
		})
	}
}

func TestIngestProviderCandidateStopsAfterContextCancellationAtPolicyBoundaries(t *testing.T) {
	for _, boundary := range []string{"source catalog", "provider policy"} {
		t.Run(boundary, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repository := &memoryIdentityRepository{}
			ingest := newCandidateIngest(repository, func() string { return "must-not-persist-after-cancel" })
			catalog := providerIngestCatalogStub{registration: validProviderIngestRegistration()}
			policies := providerIngestPolicyStub{binding: validProviderIngestPolicy()}
			if boundary == "source catalog" {
				catalog.afterLookup = cancel
			} else {
				policies.afterResolve = cancel
			}
			usecase, err := NewIngestProviderCandidate(ingest, catalog, policies)
			if err != nil {
				t.Fatal(err)
			}
			_, err = usecase.Execute(ctx, IngestProviderCandidateRequest{
				SourceID: "source-1", EndpointID: "endpoint-1", IdempotencyKey: "cancelled",
				Candidate: domain.Candidate{ProviderCode: "brave", SourceItemID: "item"},
				Purpose:   domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentSnippet,
				StorageMode: domain.StorageTransient, Retention: time.Minute,
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Execute() error=%v, want context.Canceled", err)
			}
			if len(repository.observations) != 0 || len(repository.items) != 0 {
				t.Fatalf("cancelled ingest persisted data: %+v", repository)
			}
		})
	}
}

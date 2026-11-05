package domain

import (
	"testing"
	"time"
)

func validSourcePolicy() SourcePolicy {
	return SourcePolicy{
		Status:               AuthorizationApproved,
		AllowedAccessMethods: []AccessMethod{AccessOfficialAPI, AccessRSS},
		AllowedPurposes:      []Purpose{PurposeInternalPersonalResearch},
		AllowedContentKinds:  []ContentKind{ContentMetadata, ContentTitle, ContentFullText},
		AllowedStorageModes:  []StorageMode{StorageNone, StorageTransient, StoragePersistentMetadata, StoragePersistentFullText},
		MaxRetention:         30 * 24 * time.Hour,
		EvidenceRef:          "agreement:source-1:v1",
	}
}

func validCollectionRequest() CollectionPolicyRequest {
	return CollectionPolicyRequest{
		AccessMethod: AccessOfficialAPI,
		Purpose:      PurposeInternalPersonalResearch,
		ContentKind:  ContentFullText,
		StorageMode:  StoragePersistentFullText,
		Retention:    7 * 24 * time.Hour,
	}
}

func TestEvaluateSourceCollectionPolicyAllowsApprovedSource(t *testing.T) {
	decision := EvaluateSourceCollectionPolicy(validSourcePolicy(), nil, validCollectionRequest(), time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC))
	if !decision.Allowed || decision.ReasonCode != "" || decision.MaxRetention != 30*24*time.Hour {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEvaluateSourceCollectionPolicyEndpointCanOnlyTighten(t *testing.T) {
	endpoint := &EndpointPolicy{
		AllowedStorageModes: []StorageMode{StorageTransient},
		MaxRetention:        durationPointer(2 * time.Hour),
		EvidenceRef:         "agreement:endpoint-1:v1",
	}
	decision := EvaluateSourceCollectionPolicy(validSourcePolicy(), endpoint, validCollectionRequest(), time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.endpoint_storage_denied" {
		t.Fatalf("decision=%+v", decision)
	}

	request := validCollectionRequest()
	request.StorageMode = StorageTransient
	request.Retention = time.Hour
	decision = EvaluateSourceCollectionPolicy(validSourcePolicy(), endpoint, request, time.Now().UTC())
	if !decision.Allowed || decision.MaxRetention != 2*time.Hour {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEvaluateSourceCollectionPolicyRejectsEndpointExpansion(t *testing.T) {
	endpoint := &EndpointPolicy{
		AllowedPurposes: []Purpose{PurposeInternalPersonalResearch, PurposeRedistribution},
		EvidenceRef:     "agreement:endpoint-1:v1",
	}
	decision := EvaluateSourceCollectionPolicy(validSourcePolicy(), endpoint, validCollectionRequest(), time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.endpoint_expansion_requires_evidence" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEvaluateSourceCollectionPolicyRejectsPendingAndExpiredPolicies(t *testing.T) {
	source := validSourcePolicy()
	source.Status = AuthorizationPending
	decision := EvaluateSourceCollectionPolicy(source, nil, validCollectionRequest(), time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.source_pending" {
		t.Fatalf("pending decision=%+v", decision)
	}

	source = validSourcePolicy()
	expiredAt := time.Now().UTC().Add(-time.Minute)
	source.ExpiresAt = &expiredAt
	decision = EvaluateSourceCollectionPolicy(source, nil, validCollectionRequest(), time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.source_expired" {
		t.Fatalf("expired decision=%+v", decision)
	}
}

func TestEvaluateSourceCollectionPolicyRequiresEvidenceAndExplicitRetention(t *testing.T) {
	source := validSourcePolicy()
	source.EvidenceRef = ""
	decision := EvaluateSourceCollectionPolicy(source, nil, validCollectionRequest(), time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.source_evidence_missing" {
		t.Fatalf("decision=%+v", decision)
	}

	source = validSourcePolicy()
	request := validCollectionRequest()
	request.Retention = source.MaxRetention + time.Hour
	decision = EvaluateSourceCollectionPolicy(source, nil, request, time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.retention_exceeded" {
		t.Fatalf("decision=%+v", decision)
	}
}

func durationPointer(value time.Duration) *time.Duration {
	return &value
}

func validProviderOutputPolicy() ProviderOutputPolicy {
	return ProviderOutputPolicy{
		Status:                  AuthorizationApproved,
		AllowedPurposes:         []Purpose{PurposeInternalPersonalResearch},
		AllowedContentKinds:     []ContentKind{ContentMetadata},
		AllowRequestProcessing:  true,
		MaxRequestRetention:     time.Hour,
		AllowedStorageModes:     []StorageMode{StoragePersistentMetadata},
		MaxPersistenceRetention: 7 * 24 * time.Hour,
		EvidenceRef:             "provider-terms:tavily:v1",
	}
}

func TestEvaluateCandidateCollectionPolicyIntersectsProviderSourceAndEndpoint(t *testing.T) {
	source := validSourcePolicy()
	source.AllowedContentKinds = []ContentKind{ContentMetadata}
	source.AllowedStorageModes = []StorageMode{StoragePersistentMetadata}
	endpoint := &EndpointPolicy{
		MaxRetention: durationPointer(48 * time.Hour),
		EvidenceRef:  "agreement:endpoint-1:v1",
	}
	provider := validProviderOutputPolicy()
	request := validCollectionRequest()
	request.ContentKind = ContentMetadata
	request.StorageMode = StoragePersistentMetadata
	request.Retention = 24 * time.Hour

	decision := EvaluateCandidateCollectionPolicy(&provider, source, endpoint, request, time.Now().UTC())
	if !decision.Allowed || decision.MaxRetention != 48*time.Hour {
		t.Fatalf("decision=%+v", decision)
	}

	request.Retention = 3 * 24 * time.Hour
	decision = EvaluateCandidateCollectionPolicy(&provider, source, endpoint, request, time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.endpoint_retention_exceeded" {
		t.Fatalf("endpoint cap decision=%+v", decision)
	}
}

func TestEvaluateProviderOutputPolicySeparatesProcessingCacheAndPersistence(t *testing.T) {
	policy := validProviderOutputPolicy()
	request := validCollectionRequest()
	request.ContentKind = ContentMetadata
	request.StorageMode = StorageTransient
	request.Retention = 30 * time.Minute

	decision := EvaluateProviderOutputPolicy(policy, request, time.Now().UTC())
	if !decision.Allowed || decision.MaxRetention != time.Hour {
		t.Fatalf("request processing decision=%+v", decision)
	}

	request.CrossRequestCache = true
	decision = EvaluateProviderOutputPolicy(policy, request, time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.provider_cache_denied" {
		t.Fatalf("cache decision=%+v", decision)
	}

	policy.AllowCrossRequestCache = true
	policy.MaxCacheRetention = 10 * time.Minute
	request.Retention = 5 * time.Minute
	decision = EvaluateProviderOutputPolicy(policy, request, time.Now().UTC())
	if !decision.Allowed || decision.MaxRetention != 10*time.Minute {
		t.Fatalf("approved cache decision=%+v", decision)
	}
}

func TestEvaluateProviderOutputPolicyRejectsUnauthorizedPersistence(t *testing.T) {
	policy := validProviderOutputPolicy()
	request := validCollectionRequest()
	request.ContentKind = ContentMetadata
	request.StorageMode = StoragePersistentFullText
	request.ContentKind = ContentFullText

	decision := EvaluateProviderOutputPolicy(policy, request, time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.provider_content_denied" {
		t.Fatalf("decision=%+v", decision)
	}

	request.StorageMode = StoragePersistentMetadata
	request.ContentKind = ContentMetadata
	request.Retention = 8 * 24 * time.Hour
	decision = EvaluateProviderOutputPolicy(policy, request, time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.provider_persistence_retention_exceeded" {
		t.Fatalf("retention decision=%+v", decision)
	}
}

func TestEvaluateCandidateCollectionPolicyRequiresProviderPolicyForCrossRequestCache(t *testing.T) {
	request := validCollectionRequest()
	request.StorageMode = StorageTransient
	request.CrossRequestCache = true
	request.Retention = time.Minute
	decision := EvaluateCandidateCollectionPolicy(nil, validSourcePolicy(), nil, request, time.Now().UTC())
	if decision.Allowed || decision.ReasonCode != "news.policy.provider_policy_missing" {
		t.Fatalf("decision=%+v", decision)
	}
}

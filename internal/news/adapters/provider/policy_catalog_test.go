package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

func staticOutputPolicyBinding() app.ProviderOutputPolicyBinding {
	expiresAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	return app.ProviderOutputPolicyBinding{
		ProviderCode: "bocha", PolicyRef: "provider-output-bocha",
		PolicyVersion: "bocha-policy-v2",
		Policy: domain.ProviderOutputPolicy{
			Status:                  domain.AuthorizationApproved,
			AllowedPurposes:         []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:     []domain.ContentKind{domain.ContentMetadata},
			AllowRequestProcessing:  true,
			AllowedStorageModes:     []domain.StorageMode{domain.StoragePersistentMetadata},
			MaxRequestRetention:     time.Hour,
			MaxPersistenceRetention: 24 * time.Hour,
			EffectiveAt:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt:               &expiresAt, EvidenceRef: "provider-terms-evidence",
		},
	}
}

func TestStaticOutputPolicyCatalogReturnsDefensiveCopies(t *testing.T) {
	binding := staticOutputPolicyBinding()
	catalog, err := NewStaticOutputPolicyCatalog([]app.ProviderOutputPolicyBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	binding.Policy.AllowedPurposes[0] = domain.PurposePublicDisplay

	first, err := catalog.ResolveProviderOutputPolicy(context.Background(), "bocha")
	if err != nil {
		t.Fatal(err)
	}
	if first.Policy.AllowedPurposes[0] != domain.PurposeInternalPersonalResearch {
		t.Fatalf("catalog changed after input mutation: %+v", first)
	}
	first.Policy.AllowedPurposes[0] = domain.PurposePublicDisplay
	again, err := catalog.ResolveProviderOutputPolicy(context.Background(), "bocha")
	if err != nil {
		t.Fatal(err)
	}
	if again.Policy.AllowedPurposes[0] != domain.PurposeInternalPersonalResearch {
		t.Fatalf("catalog leaked mutable policy data: %+v", again)
	}
}

func TestStaticOutputPolicyCatalogRejectsDuplicateAndInvalidBindings(t *testing.T) {
	binding := staticOutputPolicyBinding()
	if _, err := NewStaticOutputPolicyCatalog([]app.ProviderOutputPolicyBinding{binding, binding}); !errors.Is(err, ErrInvalidOutputPolicyCatalog) {
		t.Fatalf("duplicate binding error = %v", err)
	}
	binding.PolicyVersion = ""
	if _, err := NewStaticOutputPolicyCatalog([]app.ProviderOutputPolicyBinding{binding}); !errors.Is(err, ErrInvalidOutputPolicyCatalog) {
		t.Fatalf("missing policy version error = %v", err)
	}
	binding = staticOutputPolicyBinding()
	binding.Policy.MaxPersistenceRetention = -time.Second
	if _, err := NewStaticOutputPolicyCatalog([]app.ProviderOutputPolicyBinding{binding}); !errors.Is(err, ErrInvalidOutputPolicyCatalog) {
		t.Fatalf("negative retention error = %v", err)
	}
}

func TestStaticOutputPolicyCatalogLookupIsExactAndHonorsContext(t *testing.T) {
	catalog, err := NewStaticOutputPolicyCatalog([]app.ProviderOutputPolicyBinding{staticOutputPolicyBinding()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResolveProviderOutputPolicy(context.Background(), "BOCHA"); !errors.Is(err, app.ErrProviderOutputPolicyUnavailable) {
		t.Fatalf("unknown provider error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := catalog.ResolveProviderOutputPolicy(ctx, "bocha"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lookup error = %v", err)
	}
}

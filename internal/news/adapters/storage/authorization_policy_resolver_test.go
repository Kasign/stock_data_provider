package storage

import (
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func TestAuthorizationPurposeForCode(t *testing.T) {
	tests := []struct {
		code string
		want domain.Purpose
		ok   bool
	}{
		{code: "research_analysis", want: domain.PurposeInternalPersonalResearch, ok: true},
		{code: "risk_review", want: domain.PurposeInternalBusiness, ok: true},
		{code: "user_display", want: domain.PurposePublicDisplay, ok: true},
		{code: "internal_business"},
		{code: "redistribution"},
		{code: "model_training"},
		{code: "unknown"},
	}
	for _, testCase := range tests {
		t.Run(testCase.code, func(t *testing.T) {
			got, ok := authorizationPurposeForCode(testCase.code)
			if got != testCase.want || ok != testCase.ok {
				t.Fatalf("authorizationPurposeForCode(%q) = (%q, %t), want (%q, %t)",
					testCase.code, got, ok, testCase.want, testCase.ok)
			}
		})
	}
}

func TestRiskReviewRequiresExplicitPurposeCodeAndBusinessPolicy(t *testing.T) {
	binding := domain.NewsAuthorizationBinding{
		PurposeCode:           "risk_review",
		RequestedContentLevel: domain.AuthorizationContentMetadata,
	}
	businessPolicy := []domain.Purpose{domain.PurposeInternalBusiness}
	researchPolicy := []domain.Purpose{domain.PurposeInternalPersonalResearch}
	contentKinds := []domain.ContentKind{domain.ContentMetadata}

	if !policyAllowsPurposeAndContent(businessPolicy, contentKinds, binding) {
		t.Fatal("explicit risk_review must be evaluated against internal_business policy")
	}

	binding.PurposeCode = "research_analysis"
	if policyAllowsPurposeAndContent(businessPolicy, contentKinds, binding) {
		t.Fatal("research_analysis must not be inferred from an internal_business-only policy")
	}

	binding.PurposeCode = "risk_review"
	if policyAllowsPurposeAndContent(researchPolicy, contentKinds, binding) {
		t.Fatal("risk_review must not be inferred from an internal_personal_research-only policy")
	}
}

func TestEndpointRiskReviewUsesExplicitPurposeMapping(t *testing.T) {
	binding := domain.NewsAuthorizationBinding{
		PurposeCode:           "risk_review",
		RequestedContentLevel: domain.AuthorizationContentMetadata,
	}
	policy := domain.EndpointPolicy{
		AllowedPurposes:     []domain.Purpose{domain.PurposeInternalBusiness},
		AllowedContentKinds: []domain.ContentKind{domain.ContentMetadata},
	}
	if !endpointAllowsPurposeAndContent(policy, binding) {
		t.Fatal("endpoint internal_business policy must allow an explicitly requested risk_review")
	}

	binding.PurposeCode = "research_analysis"
	if endpointAllowsPurposeAndContent(policy, binding) {
		t.Fatal("research_analysis must not inherit endpoint internal_business permission")
	}
}

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type authorizationFingerprintVerifierStub struct {
	err        error
	calls      int
	afterCheck func()
}

func (s *authorizationFingerprintVerifierStub) VerifyNewsAuthorizationFingerprint(context.Context, domain.NewsAuthorizationBinding) error {
	s.calls++
	if s.afterCheck != nil {
		s.afterCheck()
	}
	return s.err
}

type currentAuthorizationPolicyResolverStub struct {
	set          domain.CurrentAuthorizationPolicySet
	err          error
	calls        int
	afterResolve func()
}

func (s *currentAuthorizationPolicyResolverStub) ResolveCurrentNewsAuthorizationPolicies(context.Context, domain.NewsAuthorizationBinding) (domain.CurrentAuthorizationPolicySet, error) {
	s.calls++
	if s.afterResolve != nil {
		s.afterResolve()
	}
	return s.set, s.err
}

func allowedAuthorizationBinding(t *testing.T) domain.NewsAuthorizationBinding {
	t.Helper()
	binding := loadAppAuthorizationBinding(t)
	binding.AuthorizationRef.Decision = domain.AuthorizationDecisionAllowed
	binding.AuthorizationRef.Conditions = []string{}
	expires := "2026-12-01T00:00:00Z"
	binding.AuthorizationRef.ExpiresAt.Value = &expires
	return binding
}

func currentAuthorizationPolicySet(binding domain.NewsAuthorizationBinding, evaluatedAt time.Time) domain.CurrentAuthorizationPolicySet {
	policies := make([]domain.CurrentAuthorizationPolicy, 0, len(binding.PolicySnapshot.Policies))
	for _, item := range binding.PolicySnapshot.Policies {
		policies = append(policies, domain.CurrentAuthorizationPolicy{
			PolicyType: item.PolicyType, PolicyRef: item.PolicyRef, PolicyVersion: item.PolicyVersion,
			Status: domain.AuthorizationApproved, Allows: true,
		})
	}
	return domain.CurrentAuthorizationPolicySet{EvaluatedAt: evaluatedAt, Policies: policies}
}

func TestEvaluateNewsEvidenceReadAuthorizationAllowsOnlyMatchingCurrentPolicies(t *testing.T) {
	binding := allowedAuthorizationBinding(t)
	observation, version := appAuthorizationEvidence()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	verifier := &authorizationFingerprintVerifierStub{}
	resolver := &currentAuthorizationPolicyResolverStub{set: currentAuthorizationPolicySet(binding, now)}
	usecase, err := NewEvaluateNewsEvidenceReadAuthorization(verifier, resolver)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }

	err = usecase.Execute(context.Background(), binding, observation, version,
		binding.CallerRef, binding.PurposeCode, binding.RequestedContentLevel)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if verifier.calls != 1 || resolver.calls != 1 {
		t.Fatalf("verifier calls=%d resolver calls=%d", verifier.calls, resolver.calls)
	}
}

func TestEvaluateNewsEvidenceReadAuthorizationUsesTransactionBoundPolicies(t *testing.T) {
	binding := allowedAuthorizationBinding(t)
	observation, version := appAuthorizationEvidence()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	verifier := &authorizationFingerprintVerifierStub{}
	resolver := &currentAuthorizationPolicyResolverStub{set: currentAuthorizationPolicySet(binding, now)}
	usecase, err := NewEvaluateNewsEvidenceReadAuthorization(verifier, resolver)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }

	current := currentAuthorizationPolicySet(binding, now)
	err = usecase.ExecuteWithCurrentPolicies(context.Background(), binding, observation, version,
		binding.CallerRef, binding.PurposeCode, binding.RequestedContentLevel, current)
	if err != nil {
		t.Fatalf("ExecuteWithCurrentPolicies() error = %v", err)
	}
	if verifier.calls != 1 || resolver.calls != 0 {
		t.Fatalf("transaction-bound authorization called verifier=%d resolver=%d, want 1 and 0", verifier.calls, resolver.calls)
	}

	denied := current
	denied.Policies = append([]domain.CurrentAuthorizationPolicy(nil), current.Policies...)
	denied.Policies[0].Allows = false
	err = usecase.ExecuteWithCurrentPolicies(context.Background(), binding, observation, version,
		binding.CallerRef, binding.PurposeCode, binding.RequestedContentLevel, denied)
	if !errors.Is(err, ErrNewsCurrentPolicyDenied) || verifier.calls != 2 || resolver.calls != 0 {
		t.Fatalf("denied transaction-bound authorization err=%v verifier=%d resolver=%d", err, verifier.calls, resolver.calls)
	}
}

func TestEvaluateNewsEvidenceReadAuthorizationFailsClosed(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*domain.NewsAuthorizationBinding, *domain.CurrentAuthorizationPolicySet)
		verifierErr error
		resolverErr error
		want        error
	}{
		{
			name:        "fingerprint verification unavailable",
			verifierErr: errors.New("verifier unavailable"),
			want:        ErrNewsAuthorizationFingerprintInvalid,
		},
		{
			name:        "current policy resolver unavailable",
			resolverErr: errors.New("catalog unavailable"),
			want:        ErrNewsCurrentPolicyUnavailable,
		},
		{
			name: "policy version changed",
			mutate: func(_ *domain.NewsAuthorizationBinding, set *domain.CurrentAuthorizationPolicySet) {
				set.Policies[0].PolicyVersion = "2.0.0"
			},
			want: ErrNewsCurrentPolicyDenied,
		},
		{
			name: "policy revoked",
			mutate: func(_ *domain.NewsAuthorizationBinding, set *domain.CurrentAuthorizationPolicySet) {
				set.Policies[0].Status = domain.AuthorizationRevoked
			},
			want: ErrNewsCurrentPolicyDenied,
		},
		{
			name: "policy scope no longer allows request",
			mutate: func(_ *domain.NewsAuthorizationBinding, set *domain.CurrentAuthorizationPolicySet) {
				set.Policies[0].Allows = false
			},
			want: ErrNewsCurrentPolicyDenied,
		},
		{
			name: "current policy missing",
			mutate: func(_ *domain.NewsAuthorizationBinding, set *domain.CurrentAuthorizationPolicySet) {
				set.Policies = set.Policies[:len(set.Policies)-1]
			},
			want: ErrNewsCurrentPolicyDenied,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := allowedAuthorizationBinding(t)
			observation, version := appAuthorizationEvidence()
			now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
			set := currentAuthorizationPolicySet(binding, now)
			if test.mutate != nil {
				test.mutate(&binding, &set)
			}
			verifier := &authorizationFingerprintVerifierStub{err: test.verifierErr}
			resolver := &currentAuthorizationPolicyResolverStub{set: set, err: test.resolverErr}
			usecase, err := NewEvaluateNewsEvidenceReadAuthorization(verifier, resolver)
			if err != nil {
				t.Fatal(err)
			}
			usecase.now = func() time.Time { return now }
			err = usecase.Execute(context.Background(), binding, observation, version,
				binding.CallerRef, binding.PurposeCode, binding.RequestedContentLevel)
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if test.verifierErr != nil && resolver.calls != 0 {
				t.Fatalf("resolver called after fingerprint failure: %d", resolver.calls)
			}
		})
	}
}

func TestEvaluateNewsEvidenceReadAuthorizationRejectsRequestBindingMismatch(t *testing.T) {
	binding := allowedAuthorizationBinding(t)
	observation, version := appAuthorizationEvidence()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	verifier := &authorizationFingerprintVerifierStub{}
	resolver := &currentAuthorizationPolicyResolverStub{set: currentAuthorizationPolicySet(binding, now)}
	usecase, err := NewEvaluateNewsEvidenceReadAuthorization(verifier, resolver)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	err = usecase.Execute(context.Background(), binding, observation, version,
		"service:other", binding.PurposeCode, binding.RequestedContentLevel)
	if !errors.Is(err, ErrInvalidAuthorizationSnapshot) || verifier.calls != 0 || resolver.calls != 0 {
		t.Fatalf("err=%v verifier=%d resolver=%d", err, verifier.calls, resolver.calls)
	}
}

func TestEvaluateNewsEvidenceReadAuthorizationRejectsConditionalBeforeExternalChecks(t *testing.T) {
	binding := loadAppAuthorizationBinding(t)
	observation, version := appAuthorizationEvidence()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	verifier := &authorizationFingerprintVerifierStub{}
	resolver := &currentAuthorizationPolicyResolverStub{}
	usecase, err := NewEvaluateNewsEvidenceReadAuthorization(verifier, resolver)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	err = usecase.Execute(context.Background(), binding, observation, version,
		binding.CallerRef, binding.PurposeCode, binding.RequestedContentLevel)
	if !errors.Is(err, ErrNewsCurrentPolicyDenied) || verifier.calls != 0 || resolver.calls != 0 {
		t.Fatalf("err=%v verifier=%d resolver=%d", err, verifier.calls, resolver.calls)
	}
}

func TestEvaluateNewsEvidenceReadAuthorizationStopsAfterContextCancellation(t *testing.T) {
	cases := []struct {
		name     string
		cancelAt string
	}{
		{name: "fingerprint verifier returned", cancelAt: "verifier"},
		{name: "current policy resolver returned", cancelAt: "resolver"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			binding := allowedAuthorizationBinding(t)
			observation, version := appAuthorizationEvidence()
			now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verifier := &authorizationFingerprintVerifierStub{}
			resolver := &currentAuthorizationPolicyResolverStub{set: currentAuthorizationPolicySet(binding, now)}
			if test.cancelAt == "verifier" {
				verifier.afterCheck = cancel
			} else {
				resolver.afterResolve = cancel
			}
			usecase, err := NewEvaluateNewsEvidenceReadAuthorization(verifier, resolver)
			if err != nil {
				t.Fatal(err)
			}
			usecase.now = func() time.Time { return now }
			err = usecase.Execute(ctx, binding, observation, version, binding.CallerRef, binding.PurposeCode, binding.RequestedContentLevel)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Execute() error = %v, want context.Canceled", err)
			}
			if test.cancelAt == "verifier" && resolver.calls != 0 {
				t.Fatalf("resolver called after verifier cancellation: %d", resolver.calls)
			}
		})
	}
}

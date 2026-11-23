package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/platform/security"
)

type dataAuthorizationPolicyStoreFake struct {
	current authorization.CurrentPolicy
	err     error
}

func (f *dataAuthorizationPolicyStoreFake) PublishDataAuthorizationPolicyVersion(context.Context, string, authorization.PolicyVersion) error {
	return errors.New("not used")
}
func (f *dataAuthorizationPolicyStoreFake) ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error) {
	return f.current, f.err
}

type dataAuthorizationDecisionStoreFake struct {
	decisions map[string]app.DataAuthorizationDecision
	revoked   map[string]bool
}

func (f *dataAuthorizationDecisionStoreFake) AppendDataAuthorizationDecision(_ context.Context, value app.DataAuthorizationDecision) error {
	var ref struct {
		AuthorizationRef string `json:"authorization_ref"`
	}
	if err := json.Unmarshal(value.AuthorizationRef, &ref); err != nil {
		return err
	}
	if f.decisions == nil {
		f.decisions = map[string]app.DataAuthorizationDecision{}
	}
	f.decisions[ref.AuthorizationRef] = value
	return nil
}
func (f *dataAuthorizationDecisionStoreFake) GetDataAuthorizationDecision(_ context.Context, ref string) (app.DataAuthorizationDecision, error) {
	if f.revoked[ref] {
		return app.DataAuthorizationDecision{}, errors.New("revoked")
	}
	value, ok := f.decisions[ref]
	if !ok {
		return app.DataAuthorizationDecision{}, errors.New("not found")
	}
	return value, nil
}

func newDataAuthorizationTestOwner(t *testing.T, now time.Time) (*DataAuthorizationOwner, *dataAuthorizationPolicyStoreFake, *dataAuthorizationDecisionStoreFake) {
	t.Helper()
	policy := authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:quant"},
		AllowedPurposes: []string{"quant_analysis"},
		RequiredScopes:  []string{"data:read"},
		EffectiveAt:     now.Add(-time.Minute).UTC(),
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	version := authorization.PolicyVersion{
		PolicyType:    authorization.DataAccessPolicyType,
		PolicyRef:     authorization.DailyBarsV2PolicyRef,
		PolicyVersion: "policy-v1",
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(sum[:]),
		CreatedAt:     now.Add(-time.Minute).UTC(),
	}
	policyStore := &dataAuthorizationPolicyStoreFake{current: authorization.CurrentPolicy{Version: version, Policy: policy}}
	decisionStore := &dataAuthorizationDecisionStoreFake{decisions: map[string]app.DataAuthorizationDecision{}, revoked: map[string]bool{}}
	owner, err := NewDataAuthorizationOwner(policyStore, decisionStore)
	if err != nil {
		t.Fatal(err)
	}
	owner.now = func() time.Time { return now.UTC() }
	return owner, policyStore, decisionStore
}

func dataAuthorizationPrincipalContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := security.WithPrincipal(context.Background(), security.Principal{
		Kind: security.IdentityService, AuthMethod: security.AuthMethodServiceBearer,
		Subject: "quant", Scopes: []string{security.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestDataAuthorizationOwnerIssuesAndRechecksCurrentPolicy(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	owner, _, _ := newDataAuthorizationTestOwner(t, now)
	ctx := dataAuthorizationPrincipalContext(t)
	snapshotRef := "data_snapshot:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	authorizationRef, err := owner.Issue(ctx, snapshotRef, "quant_analysis")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if authorizationRef == "" {
		t.Fatal("Issue() returned an empty AuthorizationRef")
	}
	if err := owner.AuthorizeRead(ctx, authorizationRef, snapshotRef, "quant_analysis"); err != nil {
		t.Fatalf("AuthorizeRead() error = %v", err)
	}
	if err := owner.AuthorizeRead(ctx, authorizationRef, snapshotRef, "research_analysis"); !errors.Is(err, ErrDataAuthorizationDenied) {
		t.Fatalf("AuthorizeRead() with another purpose error = %v, want denied", err)
	}
}

func TestDataAuthorizationOwnerFailsClosedWithoutPrincipalOrPolicy(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	owner, policyStore, _ := newDataAuthorizationTestOwner(t, now)
	snapshotRef := "data_snapshot:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if _, err := owner.Issue(context.Background(), snapshotRef, "quant_analysis"); !errors.Is(err, ErrDataAuthorizationDenied) {
		t.Fatalf("Issue() without Principal error = %v, want denied", err)
	}
	policyStore.err = errors.New("database unavailable")
	if _, err := owner.Issue(dataAuthorizationPrincipalContext(t), snapshotRef, "quant_analysis"); !errors.Is(err, ErrDataAuthorizationUnavailable) {
		t.Fatalf("Issue() without current policy error = %v, want unavailable", err)
	}
}

func TestDataAuthorizationOwnerRejectsPolicyChangeAndRevocation(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	owner, policyStore, decisionStore := newDataAuthorizationTestOwner(t, now)
	ctx := dataAuthorizationPrincipalContext(t)
	snapshotRef := "data_snapshot:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	authorizationRef, err := owner.Issue(ctx, snapshotRef, "quant_analysis")
	if err != nil {
		t.Fatal(err)
	}
	policyStore.current.Version.PolicyVersion = "policy-v2"
	if err := owner.AuthorizeRead(ctx, authorizationRef, snapshotRef, "quant_analysis"); !errors.Is(err, ErrDataAuthorizationDenied) {
		t.Fatalf("AuthorizeRead() after policy change error = %v, want denied", err)
	}
	policyStore.current.Version.PolicyVersion = "policy-v1"
	decisionStore.revoked[authorizationRef] = true
	if err := owner.AuthorizeRead(ctx, authorizationRef, snapshotRef, "quant_analysis"); !errors.Is(err, ErrDataAuthorizationDenied) {
		t.Fatalf("AuthorizeRead() after revocation error = %v, want denied", err)
	}
}

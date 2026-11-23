package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/platform/security"
)

type dataPolicyOwnerRepositoryFake struct {
	current   authorization.CurrentPolicy
	published []authorization.PolicyVersion
	err       error
	afterPublish func(*dataPolicyOwnerRepositoryFake)
	resolveCalls int
}

func (fake *dataPolicyOwnerRepositoryFake) PublishDataAuthorizationPolicyVersion(
	_ context.Context, expected string, version authorization.PolicyVersion,
) error {
	if fake.err != nil {
		return fake.err
	}
	if fake.current.Version.PolicyVersion != expected {
		return app.ErrDataAuthorizationPolicyVersionConflict
	}
	fake.published = append(fake.published, version)
	policy, err := authorization.DecodePolicyVersion(version)
	if err != nil {
		return err
	}
	fake.current = authorization.CurrentPolicy{Version: version, Policy: policy}
	if fake.afterPublish != nil {
		fake.afterPublish(fake)
	}
	return nil
}

func (fake *dataPolicyOwnerRepositoryFake) ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error) {
	fake.resolveCalls++
	if fake.err != nil {
		return authorization.CurrentPolicy{}, fake.err
	}
	return fake.current, nil
}

func dataPolicyOwnerContext(t *testing.T, method security.AuthMethod) context.Context {
	t.Helper()
	principal := security.Principal{
		Kind: security.IdentityOwner, AuthMethod: method, Subject: "owner",
	}
	if method == security.AuthMethodOwnerBearer {
		principal.Scopes = []string{security.ScopePlatformAdmin}
	}
	ctx, err := security.WithPrincipal(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func validDataAccessPolicy(now time.Time) authorization.DataAccessPolicy {
	return authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:quant"},
		AllowedPurposes: []string{"quant_analysis"},
		RequiredScopes:  []string{"data:read"},
		EffectiveAt:     now.Add(-time.Minute).UTC(),
	}
}

func TestDataAuthorizationPolicyOwnerPublishesWithExpectedVersion(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	repository := &dataPolicyOwnerRepositoryFake{}
	owner, err := NewDataAuthorizationPolicyOwner(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner.now = func() time.Time { return now }
	ctx := dataPolicyOwnerContext(t, security.AuthMethodOwnerSession)

	current, err := owner.Publish(ctx, "", "policy-v1", validDataAccessPolicy(now))
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if current.Version.PolicyVersion != "policy-v1" || current.Policy.Status != authorization.PolicyApproved {
		t.Fatalf("unexpected current policy: %+v", current)
	}
	sum := sha256.Sum256(repository.published[0].Payload)
	if repository.published[0].PayloadHash != hex.EncodeToString(sum[:]) {
		t.Fatal("published policy payload hash mismatch")
	}

	nextPolicy := validDataAccessPolicy(now)
	nextPolicy.Status = authorization.PolicyRestricted
	current, err = owner.Publish(ctx, "policy-v1", "policy-v2", nextPolicy)
	if err != nil {
		t.Fatalf("Publish() second version error = %v", err)
	}
	if current.Version.PolicyVersion != "policy-v2" || current.Policy.Status != authorization.PolicyRestricted {
		t.Fatalf("unexpected second policy: %+v", current)
	}
}


func TestDataAuthorizationPolicyOwnerReturnsPublishedVersionWhenCurrentPointerAdvances(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	repository := &dataPolicyOwnerRepositoryFake{}
	repository.afterPublish = func(fake *dataPolicyOwnerRepositoryFake) {
		// 模拟本次 CAS 提交后，另一个 Owner 紧接着推进 current pointer。
		fake.current.Version.PolicyVersion = "policy-v2"
		fake.current.Policy.Status = authorization.PolicyRestricted
	}
	owner, err := NewDataAuthorizationPolicyOwner(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner.now = func() time.Time { return now }

	published, err := owner.Publish(
		dataPolicyOwnerContext(t, security.AuthMethodOwnerSession),
		"", "policy-v1", validDataAccessPolicy(now),
	)
	if err != nil {
		t.Fatalf("Publish() returned error after a concurrent pointer advance: %v", err)
	}
	if published.Version.PolicyVersion != "policy-v1" || published.Policy.Status != authorization.PolicyApproved {
		t.Fatalf("Publish() returned a later current policy instead of its own version: %+v", published)
	}
	if repository.current.Version.PolicyVersion != "policy-v2" {
		t.Fatalf("fake current pointer = %q, want policy-v2", repository.current.Version.PolicyVersion)
	}
	if repository.resolveCalls != 0 {
		t.Fatalf("Publish() re-read current pointer %d times, want 0", repository.resolveCalls)
	}
}

func TestDataAuthorizationPolicyOwnerRejectsStaleVersionAndNonSessionIdentity(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	repository := &dataPolicyOwnerRepositoryFake{}
	owner, err := NewDataAuthorizationPolicyOwner(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner.now = func() time.Time { return now }
	policy := validDataAccessPolicy(now)
	if _, err := owner.Publish(dataPolicyOwnerContext(t, security.AuthMethodOwnerSession), "", "policy-v1", policy); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Publish(dataPolicyOwnerContext(t, security.AuthMethodOwnerSession), "", "policy-v2", policy); !errors.Is(err, ErrDataPolicyVersionConflict) {
		t.Fatalf("Publish() stale expected version error = %v, want conflict", err)
	}
	if _, err := owner.Publish(dataPolicyOwnerContext(t, security.AuthMethodOwnerBearer), "policy-v1", "policy-v2", policy); !errors.Is(err, ErrDataPolicyOwnerDenied) {
		t.Fatalf("Publish() Owner Bearer error = %v, want Owner Session denial", err)
	}
}

func TestDataAuthorizationPolicyOwnerRejectsInvalidPolicy(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	repository := &dataPolicyOwnerRepositoryFake{}
	owner, err := NewDataAuthorizationPolicyOwner(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner.now = func() time.Time { return now }
	policy := validDataAccessPolicy(now)
	policy.AllowedSubjects = nil
	if _, err := owner.Publish(dataPolicyOwnerContext(t, security.AuthMethodOwnerSession), "", "policy-v1", policy); !errors.Is(err, ErrDataPolicyInvalidChange) {
		t.Fatalf("Publish() invalid policy error = %v, want invalid change", err)
	}
}

func TestDataAuthorizationPolicyOwnerReturnsUnavailableWhenStoreFails(t *testing.T) {
	repository := &dataPolicyOwnerRepositoryFake{err: errors.New("database unavailable")}
	owner, err := NewDataAuthorizationPolicyOwner(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	owner.now = func() time.Time { return now }
	if _, err := owner.Publish(dataPolicyOwnerContext(t, security.AuthMethodOwnerSession), "", "policy-v1", validDataAccessPolicy(now)); !errors.Is(err, ErrDataPolicyOwnerUnavailable) {
		t.Fatalf("Publish() store error = %v, want unavailable", err)
	}
}

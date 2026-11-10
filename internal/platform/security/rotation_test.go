package security

import (
	"testing"

	"example.com/stock-analysis-flow/internal/platform/config"
)

func TestServiceCredentialRotationRequiresAuthenticatorReplacement(t *testing.T) {
	makeSecret := func(value string) config.Secret {
		return config.NewEnvWithLookup(func(string) (string, bool) {
			return value, true
		}).Secret("TOKEN")
	}
	oldToken := "old-service-credential-0123456789-abcdef"
	newToken := "new-service-credential-0123456789-abcdef"

	build := func(token string) *Authenticator {
		auth, err := NewAuthenticator(makeSecret(ownerToken), []ServiceCredential{{
			Name: "rotated-service", Secret: makeSecret(token), Scopes: []string{ScopeDataRead},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return auth
	}

	oldAuthenticator := build(oldToken)
	newAuthenticator := build(newToken)

	if _, err := newAuthenticator.Authenticate(requestWithToken(oldToken)); err == nil {
		t.Fatal("new authenticator must reject the revoked credential")
	}
	if _, err := newAuthenticator.Authenticate(requestWithToken(newToken)); err != nil {
		t.Fatalf("new authenticator rejected rotated credential: %v", err)
	}
	if _, err := oldAuthenticator.Authenticate(requestWithToken(oldToken)); err != nil {
		t.Fatalf("old authenticator snapshot unexpectedly changed: %v", err)
	}
	if _, err := oldAuthenticator.Authenticate(requestWithToken(newToken)); err == nil {
		t.Fatal("old authenticator must not implicitly accept the new credential")
	}
}

func TestServiceCredentialCannotReceiveOwnerAdminScope(t *testing.T) {
	secret := config.NewEnvWithLookup(func(string) (string, bool) {
		return "service-credential-0123456789-abcdef", true
	}).Secret("SERVICE")
	owner := config.NewEnvWithLookup(func(string) (string, bool) {
		return ownerToken, true
	}).Secret("OWNER")
	_, err := NewAuthenticator(owner, []ServiceCredential{{
		Name: "not-owner", Secret: secret, Scopes: []string{ScopePlatformAdmin},
	}})
	if err == nil {
		t.Fatal("service credential must not receive platform admin scope")
	}
}

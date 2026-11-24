package bootstrap

import (
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

func TestQuantServicePrincipalRequiresExplicitDataReadCredential(t *testing.T) {
	secret := platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "0123456789abcdef0123456789abcdef", true
	}).Secret("TOKEN")
	principal, err := quantServicePrincipal([]platformsecurity.ServiceCredential{{
		Name: "quant", Secret: secret,
		Scopes: []string{platformsecurity.ScopeDataRead},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if principal == nil || principal.Subject != "quant" || principal.Kind != platformsecurity.IdentityService {
		t.Fatalf("unexpected Quant principal: %+v", principal)
	}
}

func TestQuantServicePrincipalIsOptionalUntilQuantStageIsEnabled(t *testing.T) {
	principal, err := quantServicePrincipal(nil)
	if err != nil || principal != nil {
		t.Fatalf("missing Quant credential should remain optional while no Quant Stage is registered: principal=%+v err=%v", principal, err)
	}
}

func TestQuantServicePrincipalFailsClosedOnInsufficientCredential(t *testing.T) {
	secret := platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "0123456789abcdef0123456789abcdef", true
	}).Secret("TOKEN")
	for _, credentials := range [][]platformsecurity.ServiceCredential{
		{{Name: "quant", Secret: secret, Scopes: []string{platformsecurity.ScopeResearchRun}}},
		{{Name: "quant", Secret: secret, Scopes: []string{platformsecurity.ScopePlatformAdmin}}},
	} {
		if _, err := quantServicePrincipal(credentials); err == nil {
			t.Fatal("missing or insufficient Quant service credential was accepted")
		}
	}
}

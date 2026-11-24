package bootstrap

import (
	"errors"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

var errQuantServiceCredentialUnavailable = errors.New("Quant service credential is unavailable")

// quantServicePrincipal resolves the explicitly configured Quant service credential.
// The resulting identity is used only by Quant stages that access Data-authorized inputs.
func quantServicePrincipal(credentials []platformsecurity.ServiceCredential) (*platformsecurity.Principal, error) {
	var selected *platformsecurity.ServiceCredential
	for index := range credentials {
		if credentials[index].Name != "quant" {
			continue
		}
		if selected != nil {
			return nil, errQuantServiceCredentialUnavailable
		}
		selected = &credentials[index]
	}
	if selected == nil {
		return nil, nil
	}
	principal, err := platformsecurity.PrincipalFromServiceCredential(*selected)
	if err != nil {
		return nil, errQuantServiceCredentialUnavailable
	}
	if !containsServiceScope(principal.Scopes, platformsecurity.ScopeDataRead) {
		return nil, errQuantServiceCredentialUnavailable
	}
	return &principal, nil
}

func containsServiceScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if scope == expected {
			return true
		}
	}
	return false
}

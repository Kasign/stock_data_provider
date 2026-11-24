package bootstrap

import (
	"errors"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

var errResearchflowWorkerCredentialUnavailable = errors.New("Quant service credential is unavailable")

// quantServicePrincipal resolves one explicitly configured service credential.
// The credential name is a stable identity contract consumed by Data authorization policy.
func researchflowWorkerPrincipal(credentials []platformsecurity.ServiceCredential) (platformsecurity.Principal, error) {
	var selected *platformsecurity.ServiceCredential
	for index := range credentials {
		if credentials[index].Name != "quant" {
			continue
		}
		if selected != nil {
			return platformsecurity.Principal{}, errResearchflowWorkerCredentialUnavailable
		}
		selected = &credentials[index]
	}
	if selected == nil {
		return platformsecurity.Principal{}, errResearchflowWorkerCredentialUnavailable
	}
	principal, err := platformsecurity.PrincipalFromServiceCredential(*selected)
	if err != nil {
		return platformsecurity.Principal{}, errResearchflowWorkerCredentialUnavailable
	}
	if !containsServiceScope(principal.Scopes, platformsecurity.ScopeDataRead) {
		return platformsecurity.Principal{}, errResearchflowWorkerCredentialUnavailable
	}
	return principal, nil
}

func containsServiceScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if scope == expected {
			return true
		}
	}
	return false
}

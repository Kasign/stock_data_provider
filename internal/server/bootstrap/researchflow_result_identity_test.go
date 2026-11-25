package bootstrap

import (
	"context"
	"errors"
	"testing"
)

func TestResearchflowWorkerPrincipalResolverRequiresTrustedWorkerMarker(t *testing.T) {
	resolver := researchflowWorkerPrincipalResolver{}
	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, errResearchflowWorkerPrincipalUnavailable) {
		t.Fatalf("unmarked context resolved a Worker principal: %v", err)
	}
	principal, err := resolver.Resolve(withResearchflowWorkerPrincipal(context.Background()))
	if err != nil || principal.SubjectRef != "service:researchflow" {
		t.Fatalf("trusted Worker context did not resolve the Researchflow principal: %+v %v", principal, err)
	}
}

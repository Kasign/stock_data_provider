package bootstrap

import (
	"context"
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
)

func TestResearchflowWorkerPrincipalRequiresExplicitDataReadCredential(t *testing.T) {
	secret := platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "0123456789abcdef0123456789abcdef", true
	}).Secret("TOKEN")
	principal, err := researchflowWorkerPrincipal([]platformsecurity.ServiceCredential{{
		Name: "researchflow-worker", Secret: secret,
		Scopes: []string{platformsecurity.ScopeDataRead},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if principal.Subject != "researchflow-worker" || principal.Kind != platformsecurity.IdentityService {
		t.Fatalf("unexpected Worker principal: %+v", principal)
	}
}

func TestResearchflowWorkerPrincipalFailsClosed(t *testing.T) {
	secret := platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "0123456789abcdef0123456789abcdef", true
	}).Secret("TOKEN")
	for _, credentials := range [][]platformsecurity.ServiceCredential{
		nil,
		{{Name: "other-service", Secret: secret, Scopes: []string{platformsecurity.ScopeDataRead}}},
		{{Name: "researchflow-worker", Secret: secret, Scopes: []string{platformsecurity.ScopeResearchRun}}},
	} {
		if _, err := researchflowWorkerPrincipal(credentials); err == nil {
			t.Fatal("missing or insufficient service credential was accepted")
		}
	}
}

type stageExecutorFunc func(context.Context, researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error)

func (f stageExecutorFunc) ExecuteStage(ctx context.Context, request researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
	return f(ctx, request)
}

func TestPrincipalStageExecutorInjectsTrustedServiceIdentity(t *testing.T) {
	principal, err := platformsecurity.NewServicePrincipal(platformsecurity.ServiceCredential{
		Name: "researchflow-worker",
		Secret: platformconfig.NewEnvWithLookup(func(string) (string, bool) {
			return "0123456789abcdef0123456789abcdef", true
		}).Secret("TOKEN"),
		Scopes: []string{platformsecurity.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	executor := principalStageExecutor{
		principal: principal,
		next: stageExecutorFunc(func(ctx context.Context, _ researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
			called = true
			got, ok := platformsecurity.PrincipalFromContextValue(ctx)
			if !ok || got.Subject != "researchflow-worker" || got.AuthMethod != platformsecurity.AuthMethodServiceBearer {
				t.Fatalf("trusted Worker identity missing from Stage context: %+v", got)
			}
			return researchflowapplication.StageExecutionResult{}, nil
		}),
	}
	if _, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("Stage executor was not called")
	}
}

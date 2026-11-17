package bootstrap

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/platform/observability"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	researchflowapp "example.com/stock-analysis-flow/internal/researchflow/application"
)

func TestOwnerSessionResearchflowIdentityAcceptsOnlyOwnerSession(t *testing.T) {
	identity := ownerSessionResearchflowIdentity{}
	ctx, err := platformsecurity.WithPrincipal(context.Background(), platformsecurity.Principal{
		Kind: platformsecurity.IdentityOwner, AuthMethod: platformsecurity.AuthMethodOwnerSession, Subject: "owner",
	})
	if err != nil { t.Fatal(err) }
	caller, err := identity.AuthenticatedCallerRef(ctx)
	if err != nil || caller != "owner" {
		t.Fatalf("Owner Session identity caller=%q err=%v", caller, err)
	}

	ownerBearer, err := platformsecurity.WithPrincipal(context.Background(), platformsecurity.Principal{
		Kind: platformsecurity.IdentityOwner, AuthMethod: platformsecurity.AuthMethodOwnerBearer, Subject: "owner",
		Scopes: []string{platformsecurity.ScopePlatformAdmin},
	})
	if err != nil { t.Fatal(err) }
	if _, err := identity.AuthenticatedCallerRef(ownerBearer); !errors.Is(err, researchflowapp.ErrUnauthenticated) {
		t.Fatalf("Owner Bearer must not be treated as Owner Session: %v", err)
	}

	serviceBearer, err := platformsecurity.WithPrincipal(context.Background(), platformsecurity.Principal{
		Kind: platformsecurity.IdentityService, AuthMethod: platformsecurity.AuthMethodServiceBearer,
		Subject: "service:researchflow", Scopes: []string{platformsecurity.ScopeResearchRun},
	})
	if err != nil { t.Fatal(err) }
	if _, err := identity.AuthenticatedCallerRef(serviceBearer); !errors.Is(err, researchflowapp.ErrUnauthenticated) {
		t.Fatalf("service Bearer must not be treated as Owner Session: %v", err)
	}
	if _, err := identity.AuthenticatedCallerRef(context.Background()); !errors.Is(err, researchflowapp.ErrUnauthenticated) {
		t.Fatalf("missing Principal must be rejected: %v", err)
	}
}

func TestOwnerSessionResearchflowIdentityPropagatesRequestID(t *testing.T) {
	ctx := observability.WithRequestID(context.Background(), "request-123")
	requestID := (ownerSessionResearchflowIdentity{}).OriginatingRequestID(ctx)
	if requestID == nil || *requestID != "request-123" {
		t.Fatalf("OriginatingRequestID = %v, want request-123", requestID)
	}
	if got := (ownerSessionResearchflowIdentity{}).CorrelationID(ctx); got != "" {
		t.Fatalf("CorrelationID = %q, want empty for CreateRun generator", got)
	}
}

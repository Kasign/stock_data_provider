package bootstrap

import (
	"context"

	"example.com/stock-analysis-flow/internal/platform/observability"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	researchflowapp "example.com/stock-analysis-flow/internal/researchflow/application"
)

// ownerSessionResearchflowIdentity adapts only a verified Owner Session
// Principal. Owner Bearer and service credentials deliberately remain distinct.
type ownerSessionResearchflowIdentity struct{}

var _ researchflowapp.RequestIdentity = ownerSessionResearchflowIdentity{}

func (ownerSessionResearchflowIdentity) AuthenticatedCallerRef(ctx context.Context) (string, error) {
	principal, ok := platformsecurity.PrincipalFromContextValue(ctx)
	if !ok || principal.Kind != platformsecurity.IdentityOwner ||
		principal.AuthMethod != platformsecurity.AuthMethodOwnerSession || principal.Subject != "owner" {
		return "", researchflowapp.ErrUnauthenticated
	}
	return principal.Subject, nil
}

func (ownerSessionResearchflowIdentity) OriginatingRequestID(ctx context.Context) *string {
	requestID := observability.FromContext(ctx)
	if requestID == "" {
		return nil
	}
	return &requestID
}

func (ownerSessionResearchflowIdentity) CorrelationID(context.Context) string {
	// CreateRun generates a correlation ID when the trusted request context has none.
	return ""
}

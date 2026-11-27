package bootstrap

import (
	"context"
	"errors"
	"strings"
	"time"

	researchapplication "example.com/stock-analysis-flow/internal/research/application"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

var errResearchflowContextAccessDenied = errors.New("researchflow context access denied")

// researchflowContextAuthorizer rechecks every included News EvidenceRef of a
// Research Context through the same Owner authorization used on ingestion.
// It never treats the stored Context itself as an access grant, and it refuses
// non-News source sections until their Owner-specific reviewer is wired.
type researchflowContextAuthorizer struct {
	callerRef          string
	evidence           researchapplication.EvidenceRepository
	evidenceAuthorizer researchapplication.EvidenceAuthorizer
}

var _ researchapplication.ContextAuthorizer = (*researchflowContextAuthorizer)(nil)

func (a *researchflowContextAuthorizer) AuthorizeContext(
	ctx context.Context,
	callerRef string,
	purpose researchevidence.PurposeCode,
	value researchcontext.Context,
) error {
	if a == nil || a.evidence == nil || a.evidenceAuthorizer == nil || ctx == nil ||
		strings.TrimSpace(callerRef) == "" || callerRef != a.callerRef ||
		!validResearchflowContextPurpose(purpose) ||
		value.SchemaVersion != researchcontext.SchemaVersion ||
		value.SystemAsOf.IsZero() || value.SystemAsOf.Location() != time.UTC {
		return researchapplication.NewAuthorizationError(researchapplication.AuthorizationDenied, errResearchflowContextAccessDenied)
	}
	if err := ctx.Err(); err != nil {
		return researchapplication.NewAuthorizationError(researchapplication.AuthorizationTimeout, err)
	}
	for _, section := range value.SectionRefs {
		if !section.Included {
			continue
		}
		if section.SourceOwner != researchcontext.SourceOwnerNews ||
			!section.SystemAsOf.Equal(value.SystemAsOf) || len(section.EvidenceRefs) == 0 {
			return researchapplication.NewAuthorizationError(researchapplication.AuthorizationDenied, errResearchflowContextAccessDenied)
		}
		for _, ref := range section.EvidenceRefs {
			record, err := a.evidence.Get(ctx, ref)
			if err != nil {
				return researchapplication.NewAuthorizationError(researchapplication.AuthorizationUnavailable, err)
			}
			if record.EvidenceID != ref || !record.SystemAsOf.Equal(value.SystemAsOf) {
				return researchapplication.NewAuthorizationError(researchapplication.AuthorizationDenied, errResearchflowContextAccessDenied)
			}
			if err := a.evidenceAuthorizer.AuthorizeEvidence(ctx, callerRef, purpose, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func validResearchflowContextPurpose(purpose researchevidence.PurposeCode) bool {
	return purpose == researchevidence.PurposeResearchAnalysis || purpose == researchevidence.PurposeRiskReview
}

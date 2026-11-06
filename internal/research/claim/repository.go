package claim

import stdctx "context"

// Repository stores append-only Claim/Hypothesis versions and supports
// exact-version reads for historical research. Append must enforce contiguous
// versions and immutable claim identity.
type Repository interface {
	Append(stdctx.Context, Claim) error
	GetVersion(stdctx.Context, string, int) (Claim, error)
}

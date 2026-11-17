package claim

import stdctx "context"

// Repository stores append-only Claim/Hypothesis versions and supports
// exact-version historical reads. Append must validate the Claim and
// atomically enforce latest_version+1; replaying the same key and complete
// payload is idempotent, while conflicting payloads and version gaps fail.
type Repository interface {
	Append(stdctx.Context, Claim) error
	GetVersion(stdctx.Context, string, int) (Claim, error)
}

package context

import stdctx "context"

// Repository stores immutable Context versions for Researchflow assembly and
// exact-version historical replay. Append must validate the Context and
// atomically enforce latest_version+1. Replaying the same key with the same
// complete payload is idempotent; a conflicting payload or version gap fails.
type Repository interface {
	Append(stdctx.Context, Context) error
	GetVersion(stdctx.Context, string, int64) (Context, error)
}

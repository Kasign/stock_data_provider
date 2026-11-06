package context

import stdctx "context"

// Repository stores immutable Context versions for Researchflow assembly and
// exact-version historical replay. Append must be idempotent for identical
// (context_id, context_version, payload) and reject conflicting replays.
type Repository interface {
	Append(stdctx.Context, Context) error
	GetVersion(stdctx.Context, string, int64) (Context, error)
}

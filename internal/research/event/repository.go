package event

import stdctx "context"

// Repository stores append-only Event versions and supports exact-version
// historical reads. Append must validate the Event and atomically enforce
// latest_version+1; replaying the same key and complete payload is idempotent,
// while conflicting payloads and version gaps fail.
type Repository interface {
	Append(stdctx.Context, ResearchEvent) error
	GetVersion(stdctx.Context, string, int) (ResearchEvent, error)
}

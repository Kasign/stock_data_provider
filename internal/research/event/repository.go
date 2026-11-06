package event

import stdctx "context"

// Repository stores append-only Event versions and supports exact-version
// reads for historical research. Append must enforce the aggregate's current
// version and the domain's ValidateNextVersion rules.
type Repository interface {
	Append(stdctx.Context, ResearchEvent) error
	GetVersion(stdctx.Context, string, int) (ResearchEvent, error)
}

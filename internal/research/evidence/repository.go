package evidence

import stdctx "context"

// Repository stores immutable Evidence records. RSC-CTR-001 excludes
// authorization_ref from content_fingerprint, so idempotency must compare the
// complete serialized payload, not content_fingerprint alone. Reusing an
// evidence_id with any changed payload is a conflict until the project
// contract defines an explicit authorization-history/version model.
type Repository interface {
	Append(stdctx.Context, Evidence) error
	Get(stdctx.Context, string) (Evidence, error)
}

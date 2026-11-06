package evidence

import stdctx "context"

// Repository stores immutable Evidence records. RSC-CTR-001 has no
// Research-owned evidence_version field, so evidence_id is the current
// immutable key. Reusing an ID with a different content fingerprint must be
// rejected; the adapter must not synthesize replacement IDs.
type Repository interface {
	Append(stdctx.Context, Evidence) error
	Get(stdctx.Context, string) (Evidence, error)
}

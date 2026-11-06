package evidence

import stdctx "context"

// Repository stores immutable Evidence records. RSC-CTR-001 has no
// Research-owned evidence_version field, so evidence_id is the current
// immutable key. An identical complete payload replay is idempotent; a
// different payload under the same ID conflicts. The adapter must not
// synthesize replacement IDs. Authorization renewal semantics remain an
// explicit contract question because authorization_ref is excluded from the
// content fingerprint but included in the stored DTO payload.
type Repository interface {
	Append(stdctx.Context, Evidence) error
	Get(stdctx.Context, string) (Evidence, error)
}

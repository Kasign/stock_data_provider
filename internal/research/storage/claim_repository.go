package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
)

const claimTable = "research_claim_versions"

type ClaimRepository struct { base repositoryBase }

func NewClaimRepository(db *sql.DB, clock Clock) (*ClaimRepository, error) {
	base, err := newRepositoryBase(db, clock)
	if err != nil { return nil, err }
	return &ClaimRepository{base: base}, nil
}

var _ researchclaim.Repository = (*ClaimRepository)(nil)

func (r *ClaimRepository) Append(ctx context.Context, value researchclaim.Claim) error {
	if r == nil { return ErrInvalidRepository }
	if err := value.Validate(); err != nil { return fmt.Errorf("validate Claim: %w", err) }
	payload, digest, err := encodePayload(value)
	if err != nil { return err }
	now := r.base.now().UTC()
	insert := "INSERT INTO research_claim_versions (claim_id, claim_version, epistemic_type, status, system_as_of, payload_sha256, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
	return r.base.appendVersion(ctx, "claim", value.ClaimID, int64(value.ClaimVersion),
		claimTable, "claim_id", "claim_version", payload, now, insert,
		[]any{value.ClaimID, value.ClaimVersion, string(value.EpistemicType), string(value.Status), formatPIT(value.SystemAsOf), digest, string(payload), now},
		func(previous []byte) error {
			var prior researchclaim.Claim
			if err := json.Unmarshal(previous, &prior); err != nil { return fmt.Errorf("decode previous Claim: %w", err) }
			return value.ValidateNextVersion(prior)
		})
}

func (r *ClaimRepository) GetVersion(ctx context.Context, id string, version int) (researchclaim.Claim, error) {
	if r == nil { return researchclaim.Claim{}, ErrInvalidRepository }
	var value researchclaim.Claim
	payload, err := r.base.getVersion(ctx, claimTable, "claim_id", "claim_version", id, int64(version))
	if err != nil { return value, err }
	if err := json.Unmarshal(payload, &value); err != nil { return value, corruptStoredRecord("Claim", "decode failed", err) }
	if err := value.Validate(); err != nil { return value, corruptStoredRecord("Claim", "validation failed", err) }
	if value.ClaimID != id || value.ClaimVersion != version { return value, corruptStoredIdentity("Claim") }
	return value, nil
}

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	researchcontext "example.com/stock-analysis-flow/internal/research/context"
)

const contextTable = "research_context_versions"

type ContextRepository struct { base repositoryBase }

func NewContextRepository(db *sql.DB, clock Clock) (*ContextRepository, error) {
	base, err := newRepositoryBase(db, clock)
	if err != nil { return nil, err }
	return &ContextRepository{base: base}, nil
}

var _ researchcontext.Repository = (*ContextRepository)(nil)

func (r *ContextRepository) Append(ctx context.Context, value researchcontext.Context) error {
	if r == nil { return ErrInvalidRepository }
	if err := value.Validate(); err != nil { return fmt.Errorf("validate Context: %w", err) }
	payload, digest, err := encodePayload(value)
	if err != nil { return err }
	now := r.base.now().UTC()
	insert := "INSERT INTO research_context_versions (context_id, context_version, system_as_of, payload_sha256, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)"
	return r.base.appendVersion(ctx, "context", value.ContextID, value.ContextVersion,
		contextTable, "context_id", "context_version", payload, insert,
		[]any{value.ContextID, value.ContextVersion, formatPIT(value.SystemAsOf), digest, string(payload), now},
		func(previous []byte) error {
			var prior researchcontext.Context
			if err := json.Unmarshal(previous, &prior); err != nil { return fmt.Errorf("decode previous Context: %w", err) }
			return value.ValidateNextVersion(prior)
		})
}

func (r *ContextRepository) GetVersion(ctx context.Context, id string, version int64) (researchcontext.Context, error) {
	if r == nil { return researchcontext.Context{}, ErrInvalidRepository }
	var value researchcontext.Context
	payload, err := r.base.getVersion(ctx, contextTable, "context_id", "context_version", id, version)
	if err != nil { return value, err }
	if err := json.Unmarshal(payload, &value); err != nil { return value, fmt.Errorf("decode Context version: %w", err) }
	if err := value.Validate(); err != nil { return value, fmt.Errorf("stored Context is invalid: %w", err) }
	if value.ContextID != id || value.ContextVersion != version { return value, fmt.Errorf("%w: stored Context identity mismatch", ErrConflict) }
	return value, nil
}

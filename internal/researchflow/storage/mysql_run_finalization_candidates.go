package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/application"
)

// ListFinalizableRunRefs discovers normal, deadline-expired and user-cancelled
// Runs only after every Stage is terminal and no Attempt remains running.
// The finalizer re-reads the full snapshot and performs the authoritative CAS.
func (r *MySQLRunRepository) ListFinalizableRunRefs(ctx context.Context, afterRunRef string, limit int) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(afterRunRef) != afterRunRef || len(afterRunRef) > 255 || limit < 1 || limit > 1000 {
		return nil, errors.New("Researchflow finalization candidate cursor or limit is invalid")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT r.run_ref
		FROM researchflow_runs r
		WHERE r.run_ref > ?
			AND (
				(r.status = 'running' AND (r.termination_reason IS NULL OR r.termination_reason = ''))
				OR (r.termination_reason = 'deadline_exceeded' AND r.status IN ('created', 'queued', 'running'))
				OR (r.status = 'cancelling' AND r.termination_reason = 'user_cancelled')
			)
			AND EXISTS (
				SELECT 1 FROM researchflow_stages s WHERE s.run_ref = r.run_ref
			)
			AND NOT EXISTS (
				SELECT 1 FROM researchflow_stages s
				WHERE s.run_ref = r.run_ref
					AND s.status NOT IN ('succeeded', 'degraded', 'skipped', 'failed', 'timed_out', 'cancelled', 'blocked')
			)
			AND NOT EXISTS (
				SELECT 1 FROM researchflow_attempts a
				WHERE a.run_ref = r.run_ref AND a.status = 'running'
			)
		ORDER BY r.run_ref LIMIT ?`, afterRunRef, limit)
	if err != nil {
		return nil, fmt.Errorf("query finalizable Researchflow Runs: %w", err)
	}
	defer rows.Close()
	refs := make([]string, 0, limit)
	previous := afterRunRef
	for rows.Next() {
		var runRef string
		if err := rows.Scan(&runRef); err != nil {
			return nil, fmt.Errorf("scan finalizable Researchflow Run: %w", err)
		}
		if strings.TrimSpace(runRef) == "" || runRef <= previous {
			return nil, errors.New("finalizable Researchflow Run query returned an invalid or unordered reference")
		}
		refs = append(refs, runRef)
		previous = runRef
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate finalizable Researchflow Runs: %w", err)
	}
	return refs, nil
}

var _ application.FinalizableRunCandidateReader = (*MySQLRunRepository)(nil)

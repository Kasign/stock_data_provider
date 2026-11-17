package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// ListSchedulableRunRefs returns a bounded keyset page. Scheduler re-reads each
// aggregate and ClaimAttempt revalidates all mutable state under database locks.
func (r *MySQLRunRepository) ListSchedulableRunRefs(ctx context.Context, afterRunRef string, limit int) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(afterRunRef) != afterRunRef || len(afterRunRef) > 255 || limit < 1 || limit > 1000 {
		return nil, errors.New("Researchflow schedulable Run cursor or limit is invalid")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT run_ref FROM researchflow_runs
		WHERE run_ref > ?
			AND status IN (?, ?, ?)
			AND (termination_reason IS NULL OR termination_reason = '')
			AND deadline_at > DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')
			AND EXISTS (
				SELECT 1 FROM researchflow_stages s
				WHERE s.run_ref = researchflow_runs.run_ref
					AND s.status IN (?, ?, ?)
			)
		ORDER BY run_ref LIMIT ?`,
		afterRunRef,
		string(domain.RunCreated), string(domain.RunQueued), string(domain.RunRunning),
		string(domain.StagePending), string(domain.StageReady), string(domain.StageRetryWait),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query schedulable Researchflow Runs: %w", err)
	}
	defer rows.Close()
	refs := make([]string, 0, limit)
	previous := afterRunRef
	for rows.Next() {
		var runRef string
		if err := rows.Scan(&runRef); err != nil {
			return nil, fmt.Errorf("scan schedulable Researchflow Run: %w", err)
		}
		if strings.TrimSpace(runRef) == "" || runRef <= previous {
			return nil, errors.New("schedulable Researchflow Run query returned an invalid or unordered reference")
		}
		refs = append(refs, runRef)
		previous = runRef
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedulable Researchflow Runs: %w", err)
	}
	return refs, nil
}

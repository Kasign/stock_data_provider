package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"

	"example.com/stock-analysis-flow/internal/researchflow/application"
)

// ListExpiredRunCandidates returns a bounded, non-locking snapshot. Each
// candidate is revalidated under the Run row lock by ExpireRunDeadline.
func (r *MySQLRunRepository) ListExpiredRunCandidates(ctx context.Context, limit int) ([]application.DeadlineCandidate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("researchflow SQL database is not configured")
	}
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("Researchflow deadline candidate limit is invalid")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT run_ref, version FROM researchflow_runs
		WHERE status IN (?, ?, ?) AND (termination_reason IS NULL OR termination_reason = '')
		AND deadline_at <= DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')
		ORDER BY deadline_at, run_ref LIMIT ?`,
		string(domain.RunCreated), string(domain.RunQueued), string(domain.RunRunning), limit)
	if err != nil {
		return nil, fmt.Errorf("query expired Researchflow Runs: %w", err)
	}
	defer rows.Close()
	candidates := make([]application.DeadlineCandidate, 0, limit)
	for rows.Next() {
		var candidate application.DeadlineCandidate
		if err := rows.Scan(&candidate.RunRef, &candidate.Version); err != nil {
			return nil, fmt.Errorf("scan expired Researchflow Run: %w", err)
		}
		if strings.TrimSpace(candidate.RunRef) == "" {
			return nil, errors.New("expired Researchflow Run has an empty reference")
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired Researchflow Runs: %w", err)
	}
	return candidates, nil
}

package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/application"
)

// ListExpiredAttemptCandidates returns running Stages whose current Attempt
// Lease has expired. It deliberately does not mutate state: Worker.Execute
// calls ClaimAttempt, which rechecks the candidate and fences the old Attempt.
func (r *MySQLRunRepository) ListExpiredAttemptCandidates(ctx context.Context, afterRunRef, afterStageRef string, limit int) ([]application.ExpiredAttemptCandidate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(afterRunRef) != afterRunRef || strings.TrimSpace(afterStageRef) != afterStageRef ||
		len(afterRunRef) > 255 || len(afterStageRef) > 255 || (afterRunRef == "" && afterStageRef != "") ||
		limit < 1 || limit > 1000 {
		return nil, errors.New("Researchflow expired Attempt cursor or limit is invalid")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT r.run_ref, s.stage_ref, a.attempt_ref
		FROM researchflow_runs r
		INNER JOIN researchflow_stages s ON s.run_ref = r.run_ref
		INNER JOIN researchflow_attempts a
			ON a.run_ref = s.run_ref AND a.stage_ref = s.stage_ref
			AND a.attempt_no = s.attempt_no
		WHERE (r.run_ref, s.stage_ref) > (?, ?)
			AND r.status = ?
			AND (r.termination_reason IS NULL OR r.termination_reason = '')
			AND r.deadline_at > DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')
			AND s.status = ?
			AND a.status = ?
			AND a.lease_expires_at IS NOT NULL
			AND a.lease_expires_at <= DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')
		ORDER BY r.run_ref, s.stage_ref
		LIMIT ?`,
		afterRunRef, afterStageRef,
		"running", "running", "running", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query expired Researchflow Attempt candidates: %w", err)
	}
	defer rows.Close()
	items := make([]application.ExpiredAttemptCandidate, 0, limit)
	previousRun, previousStage := afterRunRef, afterStageRef
	for rows.Next() {
		var item application.ExpiredAttemptCandidate
		if err := rows.Scan(&item.RunRef, &item.StageRef, &item.AttemptRef); err != nil {
			return nil, fmt.Errorf("scan expired Researchflow Attempt candidate: %w", err)
		}
		if strings.TrimSpace(item.RunRef) == "" ||
			!isAfterExpiredAttemptCursor(item.RunRef, item.StageRef, previousRun, previousStage) ||
			strings.TrimSpace(item.StageRef) == "" || strings.TrimSpace(item.AttemptRef) == "" {
			return nil, errors.New("expired Researchflow Attempt query returned an invalid or unordered candidate")
		}
		items = append(items, item)
		previousRun, previousStage = item.RunRef, item.StageRef
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired Researchflow Attempt candidates: %w", err)
	}
	return items, nil
}


// isAfterExpiredAttemptCursor compares the composite (RunRef, StageRef) key.
// Multiple Stages belonging to the same Run must remain valid across pages.
func isAfterExpiredAttemptCursor(runRef, stageRef, afterRunRef, afterStageRef string) bool {
	return runRef > afterRunRef || (runRef == afterRunRef && stageRef > afterStageRef)
}

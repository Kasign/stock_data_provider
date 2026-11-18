package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/application"
)

// ListExpiredCancellationAttemptCandidates finds in-flight Attempts belonging
// to user-cancelling Runs after their leases expire. The fencing transaction
// rechecks Run, Stage, Attempt and database time under row locks.
func (r *MySQLRunRepository) ListExpiredCancellationAttemptCandidates(
	ctx context.Context,
	afterRunRef, afterStageRef string,
	limit int,
) ([]application.ExpiredCancellationAttemptCandidate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(afterRunRef) != afterRunRef || strings.TrimSpace(afterStageRef) != afterStageRef ||
		len(afterRunRef) > 255 || len(afterStageRef) > 255 ||
		(afterRunRef == "" && afterStageRef != "") || limit < 1 || limit > 1000 {
		return nil, errors.New("Researchflow cancellation recovery cursor or limit is invalid")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT r.run_ref, s.stage_ref, a.attempt_ref, s.version
		FROM researchflow_runs r
		INNER JOIN researchflow_stages s ON s.run_ref = r.run_ref
		INNER JOIN researchflow_attempts a
			ON a.run_ref = s.run_ref AND a.stage_ref = s.stage_ref
			AND a.attempt_no = s.attempt_no
		WHERE (r.run_ref, s.stage_ref) > (?, ?)
			AND r.status = 'cancelling'
			AND r.termination_reason = 'user_cancelled'
			AND s.status = 'running'
			AND a.status = 'running'
			AND a.lease_expires_at IS NOT NULL
			AND a.lease_expires_at <= DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')
		ORDER BY r.run_ref, s.stage_ref LIMIT ?`,
		afterRunRef, afterStageRef, limit)
	if err != nil {
		return nil, fmt.Errorf("query expired cancellation Attempts: %w", err)
	}
	defer rows.Close()
	items := make([]application.ExpiredCancellationAttemptCandidate, 0, limit)
	previousRun, previousStage := afterRunRef, afterStageRef
	for rows.Next() {
		var item application.ExpiredCancellationAttemptCandidate
		if err := rows.Scan(&item.RunRef, &item.StageRef, &item.AttemptRef, &item.ExpectedStageVersion); err != nil {
			return nil, fmt.Errorf("scan expired cancellation Attempt: %w", err)
		}
		if strings.TrimSpace(item.RunRef) == "" ||
			!isAfterExpiredAttemptCursor(item.RunRef, item.StageRef, previousRun, previousStage) ||
			strings.TrimSpace(item.StageRef) == "" || strings.TrimSpace(item.AttemptRef) == "" ||
			item.ExpectedStageVersion == 0 {
			return nil, errors.New("expired cancellation Attempt query returned an invalid or unordered candidate")
		}
		items = append(items, item)
		previousRun, previousStage = item.RunRef, item.StageRef
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired cancellation Attempts: %w", err)
	}
	return items, nil
}

var _ application.CancellationAttemptRecoveryStore = (*MySQLRunRepository)(nil)

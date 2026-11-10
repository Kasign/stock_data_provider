package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// RenewAttemptLease extends a live lease without changing its fencing token.
// The new expiry is capped by the immutable Run deadline.
func (r *MySQLRunRepository) RenewAttemptLease(
	ctx context.Context,
	runRef, stageRef, attemptRef, workerID string,
	fencingToken uint64,
	leaseDuration time.Duration,
) (time.Time, error) {
	if r == nil || r.db == nil {
		return time.Time{}, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(runRef) == "" || strings.TrimSpace(stageRef) == "" ||
		strings.TrimSpace(attemptRef) == "" || strings.TrimSpace(workerID) == "" ||
		len(workerID) > 128 || fencingToken == 0 || leaseDuration <= 0 {
		return time.Time{}, errors.New("Researchflow lease renewal arguments are invalid")
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return time.Time{}, fmt.Errorf("begin Researchflow lease renewal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runStatus, deadlineValue string
	if err := tx.QueryRowContext(ctx, `SELECT status, deadline_at
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, runRef,
	).Scan(&runStatus, &deadlineValue); err != nil {
		return time.Time{}, fmt.Errorf("lock Researchflow lease run %q: %w", runRef, err)
	}
	if domain.RunStatus(runStatus) != domain.RunRunning {
		return time.Time{}, ErrClaimRunUnavailable
	}

	var stageStatus string
	var stageAttemptNo int
	if err := tx.QueryRowContext(ctx, `SELECT status, attempt_no
		FROM researchflow_stages WHERE stage_ref = ? AND run_ref = ? FOR UPDATE`,
		stageRef, runRef,
	).Scan(&stageStatus, &stageAttemptNo); err != nil {
		return time.Time{}, fmt.Errorf("lock Researchflow lease stage %q: %w", stageRef, err)
	}
	if domain.StageStatus(stageStatus) != domain.StageRunning {
		return time.Time{}, ErrClaimStageUnavailable
	}

	var attemptStatus, owner string
	var leaseValue sql.NullString
	var token uint64
	var attemptNo int
	if err := tx.QueryRowContext(ctx, `SELECT status, lease_owner, lease_expires_at, fencing_token, attempt_no
		FROM researchflow_attempts
		WHERE attempt_ref = ? AND stage_ref = ? AND run_ref = ? FOR UPDATE`,
		attemptRef, stageRef, runRef,
	).Scan(&attemptStatus, &owner, &leaseValue, &token, &attemptNo); err != nil {
		return time.Time{}, fmt.Errorf("lock Researchflow lease attempt %q: %w", attemptRef, err)
	}
	if domain.AttemptStatus(attemptStatus) != domain.AttemptRunning ||
		owner != workerID || token != fencingToken || attemptNo != stageAttemptNo {
		return time.Time{}, ErrAttemptLeaseHeld
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return time.Time{}, err
	}
	deadline, err := parseResearchflowTime(deadlineValue)
	if err != nil {
		return time.Time{}, err
	}
	if !deadline.After(now) {
		return time.Time{}, ErrClaimRunUnavailable
	}
	currentExpiry, err := parseNullableResearchflowTime(leaseValue)
	if err != nil {
		return time.Time{}, err
	}
	if currentExpiry == nil || !currentExpiry.After(now) {
		return time.Time{}, ErrAttemptLeaseHeld
	}

	nextExpiry, err := calculateResearchflowLeaseExpiry(now, leaseDuration, deadline)
	if err != nil {
		return time.Time{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE researchflow_attempts SET lease_expires_at = ?
		WHERE attempt_ref = ? AND run_ref = ? AND stage_ref = ? AND attempt_no = ?
			AND status = ? AND lease_owner = ? AND fencing_token = ?
			AND lease_expires_at > ?`,
		formatResearchflowTimeValue(nextExpiry), attemptRef, runRef, stageRef, attemptNo,
		string(domain.AttemptRunning), workerID, fencingToken, formatResearchflowTimeValue(now))
	if err != nil {
		return time.Time{}, fmt.Errorf("extend Researchflow attempt lease: %w", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return time.Time{}, fmt.Errorf("verify Researchflow lease renewal: %w", rowsErr)
		}
		return time.Time{}, ErrAttemptLeaseHeld
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, fmt.Errorf("commit Researchflow lease renewal: %w", err)
	}
	return nextExpiry, nil
}

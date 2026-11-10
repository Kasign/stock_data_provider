package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// ConfirmAttemptCancellation persists a worker's cooperative stop confirmation.
// It is fenced by the current lease owner/generation and updates Attempt + Stage
// atomically. Expired leases must use the separate recovery/fencing path.
func (r *MySQLRunRepository) ConfirmAttemptCancellation(
	ctx context.Context,
	runRef, stageRef, attemptRef, workerID string,
	fencingToken, expectedStageVersion uint64,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(runRef) == "" || len(runRef) > 255 ||
		strings.TrimSpace(stageRef) == "" || len(stageRef) > 255 ||
		strings.TrimSpace(attemptRef) == "" || len(attemptRef) > 255 ||
		strings.TrimSpace(workerID) == "" || len(workerID) > 128 ||
		fencingToken == 0 || expectedStageVersion == ^uint64(0) {
		return false, errors.New("Researchflow Attempt cancellation confirmation arguments are invalid")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, fmt.Errorf("begin Researchflow Attempt cancellation confirmation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM researchflow_runs
		WHERE run_ref = ? FOR UPDATE`, runRef).Scan(&runStatus); err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation Run: %w", err)
	}
	if domain.RunStatus(runStatus) != domain.RunCancelling {
		return false, ErrClaimRunUnavailable
	}

	var stageStatus string
	var stageVersion uint64
	var attemptNo uint32
	var reasonJSON string
	if err := tx.QueryRowContext(ctx, `SELECT status, version, attempt_no, reason_codes
		FROM researchflow_stages WHERE stage_ref = ? AND run_ref = ? FOR UPDATE`,
		stageRef, runRef).Scan(&stageStatus, &stageVersion, &attemptNo, &reasonJSON); err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation Stage: %w", err)
	}
	if domain.StageStatus(stageStatus) != domain.StageRunning || stageVersion != expectedStageVersion {
		return false, ErrClaimStageUnavailable
	}

	var attemptStatus string
	var leaseOwner, leaseExpiresAt sql.NullString
	var attemptNoFromRow uint32
	var currentFencingToken uint64
	if err := tx.QueryRowContext(ctx, `SELECT status, lease_owner, lease_expires_at,
		attempt_no, fencing_token FROM researchflow_attempts
		WHERE attempt_ref = ? AND stage_ref = ? AND run_ref = ? FOR UPDATE`,
		attemptRef, stageRef, runRef).Scan(&attemptStatus, &leaseOwner,
		&leaseExpiresAt, &attemptNoFromRow, &currentFencingToken); err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation Attempt: %w", err)
	}
	if domain.AttemptStatus(attemptStatus) != domain.AttemptRunning ||
		!leaseOwner.Valid || leaseOwner.String != workerID ||
		currentFencingToken != fencingToken || attemptNoFromRow != attemptNo {
		return false, ErrAttemptLeaseHeld
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return false, err
	}
	expiresAt, err := parseNullableResearchflowTime(leaseExpiresAt)
	if err != nil {
		return false, err
	}
	if expiresAt == nil || !expiresAt.After(now) {
		return false, ErrAttemptLeaseHeld
	}
	var reasons []string
	if err := json.Unmarshal([]byte(reasonJSON), &reasons); err != nil {
		return false, fmt.Errorf("decode Researchflow cancellation Stage reasons: %w", err)
	}
	foundReason := false
	for _, reason := range reasons {
		if reason == "user_cancelled" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		reasons = append(reasons, "user_cancelled")
	}
	encodedReasons, err := json.Marshal(nonNilStrings(reasons))
	if err != nil {
		return false, fmt.Errorf("encode Researchflow cancellation Stage reasons: %w", err)
	}
	finishedAt := formatResearchflowTimeValue(now)

	attemptResult, err := tx.ExecContext(ctx, `UPDATE researchflow_attempts SET
		status = ?, finished_at = ?, lease_owner = NULL, lease_expires_at = NULL
		WHERE attempt_ref = ? AND run_ref = ? AND stage_ref = ? AND status = ?
			AND lease_owner = ? AND fencing_token = ? AND attempt_no = ?
			AND lease_expires_at > DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')`,
		string(domain.AttemptCancelled), finishedAt, attemptRef, runRef, stageRef,
		string(domain.AttemptRunning), workerID, fencingToken, attemptNo)
	if err != nil {
		return false, fmt.Errorf("persist Researchflow Attempt cancellation: %w", err)
	}
	if affected, rowsErr := attemptResult.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return false, fmt.Errorf("verify Researchflow Attempt cancellation: %w", rowsErr)
		}
		return false, ErrAttemptLeaseHeld
	}

	stageResult, err := tx.ExecContext(ctx, `UPDATE researchflow_stages SET
		status = ?, version = version + 1, retry_at = NULL,
		reason_codes = ?, finished_at = ?
		WHERE stage_ref = ? AND run_ref = ? AND status = ? AND version = ?`,
		string(domain.StageCancelled), string(encodedReasons), finishedAt,
		stageRef, runRef, string(domain.StageRunning), expectedStageVersion)
	if err != nil {
		return false, fmt.Errorf("persist Researchflow Stage cancellation: %w", err)
	}
	if affected, rowsErr := stageResult.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return false, fmt.Errorf("verify Researchflow Stage cancellation: %w", rowsErr)
		}
		return false, errors.New("Researchflow Stage changed during cancellation confirmation")
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit Researchflow Attempt cancellation confirmation: %w", err)
	}
	return true, nil
}

// FenceExpiredCancellationAttempt closes an in-flight Attempt after its lease
// expires while the Run is cancelling. The Attempt is recorded timed_out,
// while its Stage is cancelled by the user request; the fencing generation is
// advanced so a late worker cannot regain local write authority.
func (r *MySQLRunRepository) FenceExpiredCancellationAttempt(
	ctx context.Context,
	runRef, stageRef, attemptRef string,
	expectedStageVersion uint64,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(runRef) == "" || len(runRef) > 255 ||
		strings.TrimSpace(stageRef) == "" || len(stageRef) > 255 ||
		strings.TrimSpace(attemptRef) == "" || len(attemptRef) > 255 ||
		expectedStageVersion == ^uint64(0) {
		return false, errors.New("Researchflow expired cancellation fencing arguments are invalid")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, fmt.Errorf("begin Researchflow expired cancellation fencing: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM researchflow_runs
		WHERE run_ref = ? FOR UPDATE`, runRef).Scan(&runStatus); err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation Run: %w", err)
	}
	if domain.RunStatus(runStatus) != domain.RunCancelling {
		return false, ErrClaimRunUnavailable
	}
	var stageStatus string
	var stageVersion uint64
	var attemptNo uint32
	var reasonJSON string
	if err := tx.QueryRowContext(ctx, `SELECT status, version, attempt_no, reason_codes
		FROM researchflow_stages WHERE stage_ref = ? AND run_ref = ? FOR UPDATE`,
		stageRef, runRef).Scan(&stageStatus, &stageVersion, &attemptNo, &reasonJSON); err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation Stage: %w", err)
	}
	if domain.StageStatus(stageStatus) != domain.StageRunning || stageVersion != expectedStageVersion {
		return false, ErrClaimStageUnavailable
	}
	var attemptStatus string
	var leaseExpiresAt sql.NullString
	var attemptNoFromRow uint32
	var fencingToken uint64
	if err := tx.QueryRowContext(ctx, `SELECT status, lease_expires_at, attempt_no, fencing_token
		FROM researchflow_attempts WHERE attempt_ref = ? AND stage_ref = ? AND run_ref = ? FOR UPDATE`,
		attemptRef, stageRef, runRef).Scan(&attemptStatus, &leaseExpiresAt, &attemptNoFromRow, &fencingToken); err != nil {
		return false, fmt.Errorf("lock expired Researchflow cancellation Attempt: %w", err)
	}
	if domain.AttemptStatus(attemptStatus) != domain.AttemptRunning ||
		attemptNoFromRow != attemptNo {
		return false, ErrAttemptLeaseHeld
	}
	if fencingToken == ^uint64(0) {
		return false, errors.New("Researchflow Attempt fencing token exhausted")
	}
	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return false, err
	}
	expiresAt, err := parseNullableResearchflowTime(leaseExpiresAt)
	if err != nil {
		return false, err
	}
	if expiresAt == nil || expiresAt.After(now) {
		return false, ErrAttemptLeaseHeld
	}
	var reasons []string
	if err := json.Unmarshal([]byte(reasonJSON), &reasons); err != nil {
		return false, fmt.Errorf("decode Researchflow cancellation Stage reasons: %w", err)
	}
	foundReason := false
	for _, reason := range reasons {
		if reason == "user_cancelled" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		reasons = append(reasons, "user_cancelled")
	}
	encodedReasons, err := json.Marshal(nonNilStrings(reasons))
	if err != nil {
		return false, fmt.Errorf("encode Researchflow cancellation Stage reasons: %w", err)
	}
	finishedAt := formatResearchflowTimeValue(now)
	attemptResult, err := tx.ExecContext(ctx, `UPDATE researchflow_attempts SET
		status = ?, fencing_token = fencing_token + 1, finished_at = ?,
		lease_owner = NULL, lease_expires_at = NULL
		WHERE attempt_ref = ? AND run_ref = ? AND stage_ref = ? AND status = ?
			AND attempt_no = ? AND fencing_token = ?
			AND lease_expires_at <= DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')`,
		string(domain.AttemptTimedOut), finishedAt, attemptRef, runRef, stageRef,
		string(domain.AttemptRunning), attemptNo, fencingToken)
	if err != nil {
		return false, fmt.Errorf("fence expired Researchflow Attempt: %w", err)
	}
	if affected, rowsErr := attemptResult.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return false, fmt.Errorf("verify expired Researchflow Attempt fencing: %w", rowsErr)
		}
		return false, ErrAttemptLeaseHeld
	}
	stageResult, err := tx.ExecContext(ctx, `UPDATE researchflow_stages SET
		status = ?, version = version + 1, retry_at = NULL,
		reason_codes = ?, finished_at = ?
		WHERE stage_ref = ? AND run_ref = ? AND status = ? AND version = ?`,
		string(domain.StageCancelled), string(encodedReasons), finishedAt,
		stageRef, runRef, string(domain.StageRunning), expectedStageVersion)
	if err != nil {
		return false, fmt.Errorf("cancel Stage after expired Attempt fencing: %w", err)
	}
	if affected, rowsErr := stageResult.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return false, fmt.Errorf("verify Stage after expired Attempt fencing: %w", rowsErr)
		}
		return false, errors.New("Researchflow Stage changed during expired Attempt fencing")
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit expired Researchflow cancellation fencing: %w", err)
	}
	return true, nil
}

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

var ErrRunDeadlineNotReached = errors.New("Researchflow Run deadline has not been reached")

// ExpireRunDeadline closes every non-terminal Stage once the persisted Run
// deadline has elapsed. It fences all running Attempts immediately, regardless
// of lease expiry, but leaves Run finalization to the Application aggregator
// so ResultSummary and Outbox remain atomically committed by FinalizeRun.
func (r *MySQLRunRepository) ExpireRunDeadline(
	ctx context.Context,
	runRef string,
	expectedVersion uint64,
) (bool, uint64, error) {
	if r == nil || r.db == nil {
		return false, 0, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(runRef) == "" || len(runRef) > 255 || expectedVersion == ^uint64(0) {
		return false, 0, errors.New("Researchflow deadline expiration arguments are invalid")
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, 0, fmt.Errorf("begin Researchflow deadline expiration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	var version uint64
	var deadlineAt string
	var terminationReason sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, version, deadline_at, termination_reason
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, runRef,
	).Scan(&status, &version, &deadlineAt, &terminationReason); err != nil {
		return false, 0, fmt.Errorf("lock Researchflow deadline Run: %w", err)
	}
	runStatus := domain.RunStatus(status)
	if runStatus.Terminal() {
		return false, version, ErrRunAlreadyFinalized
	}
	if runStatus == domain.RunCancelling ||
		(runStatus != domain.RunCreated && runStatus != domain.RunQueued && runStatus != domain.RunRunning) {
		return false, version, ErrClaimRunUnavailable
	}
	if version != expectedVersion {
		return false, version, nil
	}
	if version == ^uint64(0) {
		return false, version, errors.New("Researchflow Run version exhausted")
	}
	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return false, version, err
	}
	persistedDeadline, err := parseResearchflowTime(deadlineAt)
	if err != nil {
		return false, version, fmt.Errorf("parse Researchflow Run deadline: %w", err)
	}
	if persistedDeadline.After(now) {
		return false, version, ErrRunDeadlineNotReached
	}
	if terminationReason.Valid && terminationReason.String != "" && terminationReason.String != "deadline_exceeded" {
		return false, version, errors.New("Researchflow Run has a conflicting termination reason")
	}

	type stageRecord struct {
		ref string
		status domain.StageStatus
		version uint64
		attemptNo uint32
		reasons []string
	}
	stageRows, err := tx.QueryContext(ctx, `SELECT stage_ref, status, version, attempt_no, reason_codes
		FROM researchflow_stages WHERE run_ref = ? ORDER BY stage_ref FOR UPDATE`, runRef)
	if err != nil {
		return false, version, fmt.Errorf("lock Researchflow deadline Stages: %w", err)
	}
	stages := make([]stageRecord, 0)
	stageByRef := make(map[string]*stageRecord)
	for stageRows.Next() {
		var item stageRecord
		var stageStatus, reasonJSON string
		if err := stageRows.Scan(&item.ref, &stageStatus, &item.version, &item.attemptNo, &reasonJSON); err != nil {
			_ = stageRows.Close()
			return false, version, fmt.Errorf("scan Researchflow deadline Stage: %w", err)
		}
		item.status = domain.StageStatus(stageStatus)
		if err := json.Unmarshal([]byte(reasonJSON), &item.reasons); err != nil {
			_ = stageRows.Close()
			return false, version, fmt.Errorf("decode Researchflow deadline Stage reasons: %w", err)
		}
		stages = append(stages, item)
	}
	if err := stageRows.Err(); err != nil {
		_ = stageRows.Close()
		return false, version, fmt.Errorf("iterate Researchflow deadline Stages: %w", err)
	}
	if err := stageRows.Close(); err != nil {
		return false, version, fmt.Errorf("close Researchflow deadline Stages: %w", err)
	}
	if len(stages) == 0 {
		return false, version, errors.New("Researchflow deadline Run has no Stages")
	}
	for i := range stages {
		stageByRef[stages[i].ref] = &stages[i]
	}

	type attemptRecord struct {
		ref string
		stageRef string
		attemptNo uint32
		status domain.AttemptStatus
		fencingToken uint64
	}
	attemptRows, err := tx.QueryContext(ctx, `SELECT attempt_ref, stage_ref, attempt_no, status, fencing_token
		FROM researchflow_attempts WHERE run_ref = ? ORDER BY stage_ref, attempt_no FOR UPDATE`, runRef)
	if err != nil {
		return false, version, fmt.Errorf("lock Researchflow deadline Attempts: %w", err)
	}
	attempts := make([]attemptRecord, 0)
	activeByStage := make(map[string]attemptRecord)
	for attemptRows.Next() {
		var item attemptRecord
		var attemptStatus string
		if err := attemptRows.Scan(&item.ref, &item.stageRef, &item.attemptNo, &attemptStatus, &item.fencingToken); err != nil {
			_ = attemptRows.Close()
			return false, version, fmt.Errorf("scan Researchflow deadline Attempt: %w", err)
		}
		item.status = domain.AttemptStatus(attemptStatus)
		if item.status == domain.AttemptRunning {
			stage, ok := stageByRef[item.stageRef]
			if !ok || stage.status != domain.StageRunning || uint32(stage.attemptNo) != item.attemptNo {
				_ = attemptRows.Close()
				return false, version, fmt.Errorf("Researchflow running Attempt %q does not match its Stage", item.ref)
			}
			if item.fencingToken == ^uint64(0) {
				_ = attemptRows.Close()
				return false, version, fmt.Errorf("Researchflow Attempt %q fencing token exhausted", item.ref)
			}
			if _, exists := activeByStage[item.stageRef]; exists {
				_ = attemptRows.Close()
				return false, version, fmt.Errorf("Researchflow Stage %q has multiple running Attempts", item.stageRef)
			}
			activeByStage[item.stageRef] = item
		}
		attempts = append(attempts, item)
	}
	if err := attemptRows.Err(); err != nil {
		_ = attemptRows.Close()
		return false, version, fmt.Errorf("iterate Researchflow deadline Attempts: %w", err)
	}
	if err := attemptRows.Close(); err != nil {
		return false, version, fmt.Errorf("close Researchflow deadline Attempts: %w", err)
	}
	for _, stage := range stages {
		if stage.status == domain.StageRunning {
			if _, ok := activeByStage[stage.ref]; !ok {
				return false, version, fmt.Errorf("Researchflow running Stage %q has no running Attempt", stage.ref)
			}
		}
	}

	finishedAt := formatResearchflowTimeValue(now)
	for _, attempt := range attempts {
		if attempt.status != domain.AttemptRunning {
			continue
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE researchflow_attempts SET
			status = ?, fencing_token = fencing_token + 1, finished_at = ?,
			lease_owner = NULL, lease_expires_at = NULL
			WHERE attempt_ref = ? AND run_ref = ? AND stage_ref = ?
				AND attempt_no = ? AND status = ? AND fencing_token = ?`,
			string(domain.AttemptTimedOut), finishedAt, attempt.ref, runRef,
			attempt.stageRef, attempt.attemptNo, string(domain.AttemptRunning), attempt.fencingToken)
		if updateErr != nil {
			return false, version, fmt.Errorf("fence Researchflow deadline Attempt %q: %w", attempt.ref, updateErr)
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
			if rowsErr != nil {
				return false, version, fmt.Errorf("verify Researchflow deadline Attempt fencing: %w", rowsErr)
			}
			return false, version, fmt.Errorf("Researchflow Attempt %q changed during deadline fencing", attempt.ref)
		}
	}

	for _, stage := range stages {
		if stage.status.Terminal() {
			continue
		}
		if stage.version == ^uint64(0) {
			return false, version, fmt.Errorf("Researchflow Stage %q version exhausted", stage.ref)
		}
		found := false
		for _, reason := range stage.reasons {
			if reason == "run_deadline_exceeded" {
				found = true
				break
			}
		}
		if !found {
			stage.reasons = append(stage.reasons, "run_deadline_exceeded")
		}
		encoded, marshalErr := json.Marshal(nonNilStrings(stage.reasons))
		if marshalErr != nil {
			return false, version, fmt.Errorf("encode Researchflow deadline Stage reasons: %w", marshalErr)
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE researchflow_stages SET
			status = ?, version = version + 1, retry_at = NULL,
			reason_codes = ?, finished_at = ?
			WHERE stage_ref = ? AND run_ref = ? AND status = ? AND version = ?`,
			string(domain.StageTimedOut), string(encoded), finishedAt,
			stage.ref, runRef, string(stage.status), stage.version)
		if updateErr != nil {
			return false, version, fmt.Errorf("expire Researchflow Stage %q: %w", stage.ref, updateErr)
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
			if rowsErr != nil {
				return false, version, fmt.Errorf("verify Researchflow Stage deadline expiration: %w", rowsErr)
			}
			return false, version, fmt.Errorf("Researchflow Stage %q changed during deadline expiration", stage.ref)
		}
	}

	result, err := tx.ExecContext(ctx, `UPDATE researchflow_runs SET
		version = version + 1, termination_reason = ?
		WHERE run_ref = ? AND version = ? AND status = ?`,
		"deadline_exceeded", runRef, expectedVersion, status)
	if err != nil {
		return false, version, fmt.Errorf("mark Researchflow Run deadline exceeded: %w", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return false, version, fmt.Errorf("verify Researchflow Run deadline expiration: %w", rowsErr)
		}
		return false, version, errors.New("Researchflow Run changed during deadline expiration")
	}
	if err := tx.Commit(); err != nil {
		return false, version, fmt.Errorf("commit Researchflow deadline expiration: %w", err)
	}
	return true, expectedVersion + 1, nil
}

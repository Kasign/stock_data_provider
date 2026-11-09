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

// AttemptResultCommit is the storage projection of an already validated
// Application/Domain transition. Storage enforces lease fencing and persists
// the Attempt and Stage outcome atomically.
type AttemptResultCommit struct {
	RunRef       string
	StageRef     string
	AttemptRef   string
	WorkerID     string
	FencingToken uint64
	AttemptStatus domain.AttemptStatus
	Stage        domain.Stage
	FaultRef     *string
	OutputRef    *string
}

// CommitAttemptResult persists a worker result only while its lease is current.
// Lock order is Run -> Stage -> Attempt; stale workers cannot overwrite a
// reclaimed attempt, even if their Stage version happens to be current.
func (r *MySQLRunRepository) CommitAttemptResult(ctx context.Context, input AttemptResultCommit) error {
	if r == nil || r.db == nil {
		return errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(input.RunRef) == "" || strings.TrimSpace(input.StageRef) == "" ||
		strings.TrimSpace(input.AttemptRef) == "" || strings.TrimSpace(input.WorkerID) == "" ||
		len(input.WorkerID) > 128 || input.FencingToken == 0 ||
		input.Stage.Ref != input.StageRef || input.Stage.RunRef != input.RunRef ||
		input.Stage.Status == "" || input.Stage.AttemptNo <= 0 ||
		(input.AttemptStatus != domain.AttemptSucceeded &&
			input.AttemptStatus != domain.AttemptDegraded &&
			input.AttemptStatus != domain.AttemptFailed &&
			input.AttemptStatus != domain.AttemptTimedOut &&
			input.AttemptStatus != domain.AttemptCancelled) {
		return errors.New("Researchflow attempt result arguments are invalid")
	}
	if input.Stage.Version == 0 {
		return errors.New("Researchflow result stage version must be positive")
	}
	diagnostics, err := jsonMarshalStrings(input.Stage.DiagnosticRefs)
	if err != nil {
		return fmt.Errorf("encode Researchflow result diagnostics: %w", err)
	}
	reasons, err := jsonMarshalStrings(input.Stage.ReasonCodes)
	if err != nil {
		return fmt.Errorf("encode Researchflow result reason codes: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin Researchflow result transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runStatus string
	var deadlineValue string
	if err := tx.QueryRowContext(ctx, `SELECT status, deadline_at
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, input.RunRef,
	).Scan(&runStatus, &deadlineValue); err != nil {
		return fmt.Errorf("lock Researchflow result run %q: %w", input.RunRef, err)
	}
	if domain.RunStatus(runStatus) != domain.RunRunning {
		return ErrClaimRunUnavailable
	}

	var stageStatus string
	var stageVersion uint64
	var stageAttemptNo int
	if err := tx.QueryRowContext(ctx, `SELECT status, version, attempt_no
		FROM researchflow_stages WHERE stage_ref = ? AND run_ref = ? FOR UPDATE`,
		input.StageRef, input.RunRef,
	).Scan(&stageStatus, &stageVersion, &stageAttemptNo); err != nil {
		return fmt.Errorf("lock Researchflow result stage %q: %w", input.StageRef, err)
	}
	if domain.StageStatus(stageStatus) != domain.StageRunning ||
		stageVersion == ^uint64(0) || input.Stage.Version != stageVersion+1 ||
		stageAttemptNo != input.Stage.AttemptNo {
		return ErrClaimStageUnavailable
	}

	var attemptStatus, owner string
	var leaseValue sql.NullString
	var fencingToken uint64
	var attemptNo int
	if err := tx.QueryRowContext(ctx, `SELECT status, lease_owner, lease_expires_at, fencing_token, attempt_no
		FROM researchflow_attempts
		WHERE attempt_ref = ? AND stage_ref = ? AND run_ref = ? FOR UPDATE`,
		input.AttemptRef, input.StageRef, input.RunRef,
	).Scan(&attemptStatus, &owner, &leaseValue, &fencingToken, &attemptNo); err != nil {
		return fmt.Errorf("lock Researchflow result attempt %q: %w", input.AttemptRef, err)
	}
	if domain.AttemptStatus(attemptStatus) != domain.AttemptRunning ||
		owner != input.WorkerID || fencingToken != input.FencingToken ||
		attemptNo != stageAttemptNo {
		return ErrAttemptLeaseHeld
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return err
	}
	deadline, err := parseResearchflowTime(deadlineValue)
	if err != nil {
		return err
	}
	if !deadline.After(now) {
		return ErrClaimRunUnavailable
	}
	leaseExpires, err := parseNullableResearchflowTime(leaseValue)
	if err != nil {
		return err
	}
	if leaseExpires == nil || !leaseExpires.After(now) {
		return ErrAttemptLeaseHeld
	}

	finishedAt := formatResearchflowTimeValue(now)
	attemptResult, err := tx.ExecContext(ctx, `UPDATE researchflow_attempts SET
		status = ?, finished_at = ?, fault_ref = ?, output_ref = ?
		WHERE attempt_ref = ? AND status = ? AND lease_owner = ? AND fencing_token = ?`,
		string(input.AttemptStatus), finishedAt, nullableString(input.FaultRef),
		nullableString(input.OutputRef), input.AttemptRef, string(domain.AttemptRunning),
		input.WorkerID, input.FencingToken)
	if err != nil {
		return fmt.Errorf("update Researchflow result attempt: %w", err)
	}
	if affected, rowsErr := attemptResult.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return fmt.Errorf("verify Researchflow result attempt update: %w", rowsErr)
		}
		return ErrAttemptLeaseHeld
	}

	stageResult, err := tx.ExecContext(ctx, `UPDATE researchflow_stages SET
		status = ?, version = ?, retry_at = ?, output_ref = ?, quality_summary_ref = ?,
		diagnostic_refs = ?, quality_status = ?, reason_codes = ?, risk_decision = ?,
		started_at = ?, finished_at = ?
		WHERE stage_ref = ? AND run_ref = ? AND version = ? AND status = ? AND attempt_no = ?`,
		string(input.Stage.Status), input.Stage.Version, nullableTime(input.Stage.RetryAt),
		nullableString(input.Stage.OutputRef), nullableString(input.Stage.QualitySummaryRef),
		diagnostics, nullableText(input.Stage.QualityStatus), reasons,
		nullableText(input.Stage.RiskDecision), nullableTime(input.Stage.StartedAt),
		nullableTime(input.Stage.FinishedAt), input.StageRef, input.RunRef,
		stageVersion, string(domain.StageRunning), stageAttemptNo)
	if err != nil {
		return fmt.Errorf("update Researchflow result stage: %w", err)
	}
	if affected, rowsErr := stageResult.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return fmt.Errorf("verify Researchflow result stage update: %w", rowsErr)
		}
		return ErrClaimStageUnavailable
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Researchflow attempt result: %w", err)
	}
	return nil
}

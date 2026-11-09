package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

var ErrRunAlreadyFinalized = errors.New("Researchflow run is already finalized")

const (
	runFinalizedEventType   = "researchflow.run.finalized"
	runFinalizedEventSchema = "1.0.0"
)

// RunFinalization is the persistence boundary for an already domain-aggregated
// terminal result. The Application layer must calculate and validate Summary
// and TerminalStatus; this method enforces the cross-row invariants atomically.
type RunFinalization struct {
	RunRef               string
	ExpectedVersion      uint64
	ResultSummaryRef     string
	SummarySchemaVersion string
	OutboxRef            string
	TerminalStatus       domain.RunStatus
	TerminationReason    string
	Summary              domain.ResultSummary
}

// FinalizeRun atomically fixes ResultSummary, terminal Run state and the
// run.finalized Outbox event. It serializes with Claim/Commit/Cancel by locking
// the Run row first, then Stage rows, then Attempt rows.
func (r *MySQLRunRepository) FinalizeRun(ctx context.Context, input RunFinalization) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(input.RunRef) == "" || len(input.RunRef) > 255 ||
		strings.TrimSpace(input.ResultSummaryRef) == "" || len(input.ResultSummaryRef) > 255 ||
		strings.TrimSpace(input.SummarySchemaVersion) == "" || len(input.SummarySchemaVersion) > 32 ||
		strings.TrimSpace(input.OutboxRef) == "" || len(input.OutboxRef) > 255 ||
		input.ExpectedVersion == ^uint64(0) {
		return false, errors.New("Researchflow finalization identity or version is invalid")
	}
	if !input.TerminalStatus.Terminal() || input.Summary.StageOutcomes == nil ||
		input.Summary.RiskGateDecisions == nil || input.Summary.MissingStageCodes == nil ||
		input.Summary.DegradedStageCodes == nil || input.Summary.BlockedStageCodes == nil ||
		input.Summary.ReasonCodes == nil || input.Summary.ExcludedStages == nil {
		return false, errors.New("Researchflow finalization requires a complete terminal status and summary")
	}
	summaryPayload, err := json.Marshal(input.Summary)
	if err != nil {
		return false, fmt.Errorf("encode Researchflow result summary: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, fmt.Errorf("begin Researchflow finalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runStatus string
	var runVersion uint64
	var correlationID string
	var existingSummaryRef sql.NullString
	var existingReason sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, version, correlation_id,
		result_summary_ref, termination_reason
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, input.RunRef,
	).Scan(&runStatus, &runVersion, &correlationID, &existingSummaryRef, &existingReason); err != nil {
		return false, fmt.Errorf("lock Researchflow finalization Run: %w", err)
	}
	if domain.RunStatus(runStatus).Terminal() {
		return false, ErrRunAlreadyFinalized
	}
	if existingSummaryRef.Valid {
		return false, errors.New("non-terminal Researchflow Run already references a ResultSummary")
	}
	if existingReason.Valid && existingReason.String != input.TerminationReason {
		return false, errors.New("Researchflow finalization termination reason does not match the persisted Run")
	}
	if existingReason.Valid && existingReason.String == "user_cancelled" && input.TerminalStatus != domain.RunCancelled {
		return false, errors.New("user-cancelled Researchflow Run must finalize as cancelled")
	}
	if runVersion != input.ExpectedVersion {
		return false, nil
	}
	currentStatus := domain.RunStatus(runStatus)
	switch currentStatus {
	case domain.RunRunning:
		if existingReason.Valid && existingReason.String == "deadline_exceeded" {
			if input.TerminalStatus != domain.RunFailed && input.TerminalStatus != domain.RunPartial {
				return false, errors.New("deadline-expired Researchflow Run cannot finalize as succeeded")
			}
		} else if input.TerminalStatus != domain.RunSucceeded && input.TerminalStatus != domain.RunPartial && input.TerminalStatus != domain.RunFailed {
			return false, errors.New("running Researchflow Run cannot finalize to the requested status")
		}
	case domain.RunCancelling:
		if input.TerminalStatus != domain.RunCancelled && input.TerminalStatus != domain.RunPartial && input.TerminalStatus != domain.RunFailed {
			return false, errors.New("cancelling Researchflow Run cannot finalize to the requested status")
		}
	default:
		return false, ErrClaimRunUnavailable
	}

	// Lock every Stage and require terminal state before reading Attempts.
	stageRows, err := tx.QueryContext(ctx, `SELECT stage_ref, status FROM researchflow_stages
		WHERE run_ref = ? ORDER BY stage_ref FOR UPDATE`, input.RunRef)
	if err != nil {
		return false, fmt.Errorf("lock Researchflow finalization Stages: %w", err)
	}
	stageCount := 0
	for stageRows.Next() {
		var ref, status string
		if err := stageRows.Scan(&ref, &status); err != nil {
			_ = stageRows.Close()
			return false, fmt.Errorf("scan Researchflow finalization Stage: %w", err)
		}
		stageCount++
		if !domain.StageStatus(status).Terminal() {
			_ = stageRows.Close()
			return false, fmt.Errorf("Researchflow Stage %q is not terminal", ref)
		}
	}
	if err := stageRows.Err(); err != nil {
		_ = stageRows.Close()
		return false, fmt.Errorf("iterate Researchflow finalization Stages: %w", err)
	}
	if err := stageRows.Close(); err != nil {
		return false, fmt.Errorf("close Researchflow finalization Stages: %w", err)
	}
	if stageCount == 0 {
		return false, errors.New("Researchflow Run has no persisted Stages")
	}

	attemptRows, err := tx.QueryContext(ctx, `SELECT attempt_ref, status FROM researchflow_attempts
		WHERE run_ref = ? ORDER BY stage_ref, attempt_no FOR UPDATE`, input.RunRef)
	if err != nil {
		return false, fmt.Errorf("lock Researchflow finalization Attempts: %w", err)
	}
	for attemptRows.Next() {
		var ref, status string
		if err := attemptRows.Scan(&ref, &status); err != nil {
			_ = attemptRows.Close()
			return false, fmt.Errorf("scan Researchflow finalization Attempt: %w", err)
		}
		if domain.AttemptStatus(status) == domain.AttemptRunning {
			_ = attemptRows.Close()
			return false, fmt.Errorf("Researchflow Attempt %q is still running", ref)
		}
		if !domain.AttemptStatus(status).Terminal() {
			_ = attemptRows.Close()
			return false, fmt.Errorf("Researchflow Attempt %q has an invalid status", ref)
		}
	}
	if err := attemptRows.Err(); err != nil {
		_ = attemptRows.Close()
		return false, fmt.Errorf("iterate Researchflow finalization Attempts: %w", err)
	}
	if err := attemptRows.Close(); err != nil {
		return false, fmt.Errorf("close Researchflow finalization Attempts: %w", err)
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return false, err
	}
	finishedAt := formatResearchflowTimeValue(now)
	eventPayload, err := json.Marshal(struct {
		EventType         string           `json:"event_type"`
		SchemaVersion     string           `json:"schema_version"`
		RunRef            string           `json:"run_ref"`
		RunStatus         domain.RunStatus `json:"run_status"`
		ResultSummaryRef  string           `json:"result_summary_ref"`
		TerminationReason string           `json:"termination_reason,omitempty"`
		CorrelationID     string           `json:"correlation_id"`
		FinishedAt        string           `json:"finished_at"`
	}{
		EventType: runFinalizedEventType, SchemaVersion: runFinalizedEventSchema,
		RunRef: input.RunRef, RunStatus: input.TerminalStatus,
		ResultSummaryRef: input.ResultSummaryRef, TerminationReason: input.TerminationReason,
		CorrelationID: correlationID, FinishedAt: finishedAt,
	})
	if err != nil {
		return false, fmt.Errorf("encode Researchflow finalized event: %w", err)
	}
	fingerprint := sha256.Sum256(eventPayload)
	fingerprintHex := hex.EncodeToString(fingerprint[:])
	idempotencyFingerprint := sha256.Sum256([]byte(input.RunRef + ":" + runFinalizedEventType))
	idempotencyKey := "researchflow:" + hex.EncodeToString(idempotencyFingerprint[:])

	if _, err := tx.ExecContext(ctx, `INSERT INTO researchflow_result_summaries
		(result_summary_ref, run_ref, schema_version, payload, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		input.ResultSummaryRef, input.RunRef, input.SummarySchemaVersion, string(summaryPayload), finishedAt); err != nil {
		return false, fmt.Errorf("insert Researchflow ResultSummary: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO researchflow_outbox
		(outbox_ref, run_ref, event_type, schema_version, idempotency_key, correlation_id,
		payload_fingerprint, payload, created_at, published_at, attempt_count, last_error_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, 0, NULL)`,
		input.OutboxRef, input.RunRef, runFinalizedEventType, runFinalizedEventSchema,
		idempotencyKey, correlationID, fingerprintHex,
		string(eventPayload), finishedAt); err != nil {
		return false, fmt.Errorf("insert Researchflow finalized Outbox event: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE researchflow_runs SET
		status = ?, version = version + 1, result_summary_ref = ?,
		termination_reason = ?, finished_at = ?
		WHERE run_ref = ? AND version = ? AND status = ?`,
		string(input.TerminalStatus), input.ResultSummaryRef,
		nullableText(input.TerminationReason), finishedAt,
		input.RunRef, input.ExpectedVersion, runStatus)
	if err != nil {
		return false, fmt.Errorf("finalize Researchflow Run: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("verify Researchflow Run finalization: %w", err)
	}
	if affected != 1 {
		return false, errors.New("Researchflow Run changed during finalization")
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit Researchflow finalization: %w", err)
	}
	return true, nil
}

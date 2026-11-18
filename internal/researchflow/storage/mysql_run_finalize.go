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

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

var ErrRunAlreadyFinalized = errors.New("Researchflow run is already finalized")

const (
	runFinalizedEventType   = "researchflow.run.terminal"
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
// run.terminal Outbox event. It serializes with Claim/Commit/Cancel by locking
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
	var correlationID, systemAsOf, templateRef, templateVersion string
	var existingSummaryRef sql.NullString
	var existingReason sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, version, correlation_id,
		system_as_of, template_ref, template_version, result_summary_ref, termination_reason
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, input.RunRef,
	).Scan(&runStatus, &runVersion, &correlationID, &systemAsOf, &templateRef, &templateVersion, &existingSummaryRef, &existingReason); err != nil {
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
	case domain.RunCreated, domain.RunQueued, domain.RunRunning:
		if existingReason.Valid {
			if existingReason.String != "deadline_exceeded" || input.TerminationReason != "deadline_exceeded" ||
				(input.TerminalStatus != domain.RunFailed && input.TerminalStatus != domain.RunPartial) {
				return false, errors.New("terminated Researchflow Run cannot use the requested finalization path")
			}
		} else if currentStatus != domain.RunRunning || input.TerminationReason != "" ||
			(input.TerminalStatus != domain.RunSucceeded && input.TerminalStatus != domain.RunPartial && input.TerminalStatus != domain.RunFailed) {
			return false, errors.New("Researchflow Run cannot finalize from its current state")
		}
	case domain.RunCancelling:
		if !existingReason.Valid || existingReason.String != "user_cancelled" ||
			input.TerminationReason != "user_cancelled" || input.TerminalStatus != domain.RunCancelled {
			return false, errors.New("cancelling Researchflow Run requires user-cancelled terminal finalization")
		}
	default:
		return false, ErrClaimRunUnavailable
	}

	// Lock every Stage and require the summary's Stage projection to match
	// the same transactional snapshot used for finalization.
	stageRows, err := tx.QueryContext(ctx, `SELECT stage_ref, stage_code, status, attempt_no
		FROM researchflow_stages WHERE run_ref = ? ORDER BY stage_ref FOR UPDATE`, input.RunRef)
	if err != nil {
		return false, fmt.Errorf("lock Researchflow finalization Stages: %w", err)
	}
	stageStatuses := make(map[string]domain.StageStatus, len(input.Summary.StageOutcomes))
	stageAttemptNumbers := make(map[string]int, len(input.Summary.StageOutcomes))
	for stageRows.Next() {
		var ref, code, status string
		var attemptNo int
		if err := stageRows.Scan(&ref, &code, &status, &attemptNo); err != nil {
			_ = stageRows.Close()
			return false, fmt.Errorf("scan Researchflow finalization Stage: %w", err)
		}
		stageStatus := domain.StageStatus(status)
		if strings.TrimSpace(code) == "" || !stageStatus.Terminal() {
			_ = stageRows.Close()
			return false, fmt.Errorf("Researchflow Stage %q has an empty code or is not terminal", ref)
		}
		if _, exists := stageStatuses[code]; exists {
			_ = stageRows.Close()
			return false, fmt.Errorf("Researchflow Run contains duplicate Stage code %q", code)
		}
		if attemptNo < 0 {
			_ = stageRows.Close()
			return false, fmt.Errorf("Researchflow Stage %q has a negative attempt number", ref)
		}
		stageStatuses[code] = stageStatus
		stageAttemptNumbers[ref] = attemptNo
	}
	if err := stageRows.Err(); err != nil {
		_ = stageRows.Close()
		return false, fmt.Errorf("iterate Researchflow finalization Stages: %w", err)
	}
	if err := stageRows.Close(); err != nil {
		return false, fmt.Errorf("close Researchflow finalization Stages: %w", err)
	}
	if len(stageStatuses) == 0 {
		return false, errors.New("Researchflow Run has no persisted Stages")
	}
	if len(stageStatuses) != len(input.Summary.StageOutcomes) {
		return false, errors.New("Researchflow ResultSummary Stage outcomes do not match persisted Stage set")
	}
	for code, persistedStatus := range stageStatuses {
		if summarizedStatus, ok := input.Summary.StageOutcomes[code]; !ok || summarizedStatus != persistedStatus {
			return false, fmt.Errorf("Researchflow ResultSummary Stage outcome %q does not match persisted Stage status", code)
		}
	}

	// Lock Attempts only after every Stage. Verify complete attempt numbering
	// against each Stage counter so missing history cannot be hidden by a terminal Stage.
	attemptCounts := make(map[string]int, len(stageAttemptNumbers))
	attemptMaxNumbers := make(map[string]int, len(stageAttemptNumbers))
	attemptRows, err := tx.QueryContext(ctx, `SELECT attempt_ref, stage_ref, attempt_no, status
		FROM researchflow_attempts WHERE run_ref = ? ORDER BY stage_ref, attempt_no FOR UPDATE`, input.RunRef)
	if err != nil {
		return false, fmt.Errorf("lock Researchflow finalization Attempts: %w", err)
	}
	for attemptRows.Next() {
		var ref, stageRef, status string
		var attemptNo int
		if err := attemptRows.Scan(&ref, &stageRef, &attemptNo, &status); err != nil {
			_ = attemptRows.Close()
			return false, fmt.Errorf("scan Researchflow finalization Attempt: %w", err)
		}
		if _, exists := stageAttemptNumbers[stageRef]; !exists {
			_ = attemptRows.Close()
			return false, fmt.Errorf("Researchflow Attempt %q references a Stage outside its Run", ref)
		}
		if attemptNo <= 0 || attemptNo != attemptMaxNumbers[stageRef]+1 {
			_ = attemptRows.Close()
			return false, fmt.Errorf("Researchflow Attempt %q history is not contiguous", ref)
		}
		attemptCounts[stageRef]++
		attemptMaxNumbers[stageRef] = attemptNo
		attemptStatus := domain.AttemptStatus(status)
		if !attemptStatus.Terminal() {
			_ = attemptRows.Close()
			return false, fmt.Errorf("Researchflow Attempt %q is not terminal", ref)
		}
	}
	if err := attemptRows.Err(); err != nil {
		_ = attemptRows.Close()
		return false, fmt.Errorf("iterate Researchflow finalization Attempts: %w", err)
	}
	if err := attemptRows.Close(); err != nil {
		return false, fmt.Errorf("close Researchflow finalization Attempts: %w", err)
	}
	for stageRef, expectedAttemptNo := range stageAttemptNumbers {
		if attemptCounts[stageRef] != expectedAttemptNo || attemptMaxNumbers[stageRef] != expectedAttemptNo {
			return false, fmt.Errorf("Researchflow Stage %q attempt_no does not match persisted Attempt history", stageRef)
		}
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return false, err
	}
	finishedAt := formatResearchflowTimeValue(now)
	systemAsOfTime, err := parseResearchflowTime(systemAsOf)
	if err != nil {
		return false, fmt.Errorf("parse Researchflow Run system_as_of for terminal event: %w", err)
	}
	var terminationReasonValue *string
	if input.TerminationReason != "" {
		value := input.TerminationReason
		terminationReasonValue = &value
	}
	var resultSummaryRefValue *string
	if input.TerminalStatus != domain.RunCancelled {
		value := input.ResultSummaryRef
		resultSummaryRefValue = &value
	}
	eventPayload, err := json.Marshal(struct {
		SourceEventRef    string           `json:"source_event_ref"`
		RunRef            string           `json:"run_ref"`
		Status            domain.RunStatus `json:"status"`
		TerminationReason *string          `json:"termination_reason"`
		SystemAsOf        string           `json:"system_as_of"`
		FinishedAt        string           `json:"finished_at"`
		ResultSummaryRef  *string          `json:"result_summary_ref"`
		TemplateRef       string           `json:"template_ref"`
		TemplateVersion   string           `json:"template_version"`
	}{
		SourceEventRef: input.RunRef, RunRef: input.RunRef, Status: input.TerminalStatus,
		TerminationReason: terminationReasonValue,
		SystemAsOf:        systemAsOfTime.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		FinishedAt:        finishedAt, ResultSummaryRef: resultSummaryRefValue,
		TemplateRef: templateRef, TemplateVersion: templateVersion,
	})
	if err != nil {
		return false, fmt.Errorf("encode Researchflow terminal event: %w", err)
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
		return false, fmt.Errorf("insert Researchflow terminal Outbox event: %w", err)
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

// FinalizeDeadlineRun adapts the Application deadline-finalization record to
// the existing atomic Run + ResultSummary + Outbox transaction.
func (r *MySQLRunRepository) FinalizeDeadlineRun(ctx context.Context, record application.DeadlineFinalizationRecord) (bool, error) {
	if record.TerminationReason != "deadline_exceeded" ||
		(record.TerminalStatus != domain.RunFailed && record.TerminalStatus != domain.RunPartial) {
		return false, errors.New("Researchflow deadline finalization record has an invalid termination reason or terminal status")
	}
	return r.FinalizeRun(ctx, RunFinalization{
		RunRef: record.RunRef, ExpectedVersion: record.ExpectedVersion,
		ResultSummaryRef: record.ResultSummaryRef, SummarySchemaVersion: record.SummarySchemaVersion,
		OutboxRef: record.OutboxRef, TerminalStatus: record.TerminalStatus,
		TerminationReason: record.TerminationReason, Summary: record.Summary,
	})
}

// LoadRunFinalizationSnapshot shares the immutable snapshot reconstruction used
// by deadline finalization; normal finalization applies a stricter Run-state gate.
func (r *MySQLRunRepository) LoadRunFinalizationSnapshot(ctx context.Context, runRef string) (application.DeadlineFinalizationSnapshot, error) {
	return r.LoadDeadlineFinalizationSnapshot(ctx, runRef)
}

// FinalizeRunResult adapts the normal Application aggregation record to the
// existing atomic Run + ResultSummary + Outbox transaction.
func (r *MySQLRunRepository) FinalizeRunResult(ctx context.Context, record application.RunFinalizationRecord) (bool, error) {
	if record.TerminationReason != "" ||
		(record.TerminalStatus != domain.RunSucceeded && record.TerminalStatus != domain.RunPartial && record.TerminalStatus != domain.RunFailed) {
		return false, errors.New("Researchflow normal finalization record has an invalid termination reason or terminal status")
	}
	return r.FinalizeRun(ctx, RunFinalization{
		RunRef: record.RunRef, ExpectedVersion: record.ExpectedVersion,
		ResultSummaryRef: record.ResultSummaryRef, SummarySchemaVersion: record.SummarySchemaVersion,
		OutboxRef: record.OutboxRef, TerminalStatus: record.TerminalStatus,
		TerminationReason: record.TerminationReason, Summary: record.Summary,
	})
}

// FinalizeCancellationRun adapts the Application cancellation record to the
// existing atomic Run + ResultSummary + Outbox transaction.
func (r *MySQLRunRepository) FinalizeCancellationRun(ctx context.Context, record application.CancellationFinalizationRecord) (bool, error) {
	if record.TerminationReason != "user_cancelled" || record.TerminalStatus != domain.RunCancelled {
		return false, errors.New("Researchflow cancellation finalization record has an invalid termination reason or terminal status")
	}
	return r.FinalizeRun(ctx, RunFinalization{
		RunRef: record.RunRef, ExpectedVersion: record.ExpectedVersion,
		ResultSummaryRef: record.ResultSummaryRef, SummarySchemaVersion: record.SummarySchemaVersion,
		OutboxRef: record.OutboxRef, TerminalStatus: record.TerminalStatus,
		TerminationReason: record.TerminationReason, Summary: record.Summary,
	})
}

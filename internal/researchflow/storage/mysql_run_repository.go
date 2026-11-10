package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	"example.com/stock-analysis-flow/internal/researchflow/state"
)

const researchflowTimestampLayout = "2006-01-02T15:04:05.000000000Z"

var _ application.RunRepository = (*MySQLRunRepository)(nil)

// MySQLRunRepository stores Researchflow runs in the module-owned MariaDB tables.
// The caller owns db lifecycle and migration execution.
type MySQLRunRepository struct {
	db *sql.DB
}

func NewMySQLRunRepository(db *sql.DB) (*MySQLRunRepository, error) {
	if db == nil {
		return nil, errors.New("researchflow SQL database is required")
	}
	return &MySQLRunRepository{db: db}, nil
}

func (r *MySQLRunRepository) FindIdempotentRun(
	ctx context.Context,
	scope application.IdempotencyScope,
) (application.CreateRunResult, string, bool, error) {
	if err := r.validateScope(scope); err != nil {
		return application.CreateRunResult{}, "", false, err
	}
	var runRef, fingerprint string
	err := r.db.QueryRowContext(ctx, `SELECT run_ref, request_fingerprint
		FROM researchflow_run_idempotency
		WHERE caller_ref = ? AND operation = ? AND idempotency_key = ?`,
		scope.CallerRef, scope.Operation, scope.Key,
	).Scan(&runRef, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return application.CreateRunResult{}, "", false, nil
	}
	if err != nil {
		return application.CreateRunResult{}, "", false, fmt.Errorf("query Researchflow idempotency record: %w", err)
	}
	result, err := r.loadRun(ctx, runRef)
	if err != nil {
		return application.CreateRunResult{}, "", false, err
	}
	return result, fingerprint, true, nil
}

func (r *MySQLRunRepository) CreateRunAtomic(
	ctx context.Context,
	record application.CreateRunRecord,
) (application.CreateRunResult, error) {
	if err := r.validateRecord(record); err != nil {
		return application.CreateRunResult{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("begin Researchflow create-run transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	dbNow, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return application.CreateRunResult{}, err
	}
	if !record.Run.DeadlineAt.After(dbNow) {
		return application.CreateRunResult{}, fmt.Errorf("%w: deadline elapsed before persistence", domain.ErrInvalidCommand)
	}
	record.Stages = append([]domain.Stage(nil), record.Stages...)
	record.Run.CreatedAt = dbNow
	for index := range record.Stages {
		record.Stages[index].CreatedAt = dbNow
		if record.Stages[index].Status == domain.StageSkipped {
			finished := dbNow
			record.Stages[index].FinishedAt = &finished
		}
	}

	resolvedSnapshot, err := json.Marshal(record.ResolvedTemplate)
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("encode resolved template snapshot: %w", err)
	}
	inputRefs, err := json.Marshal(nonNilStrings(record.Run.InputSnapshotRefs))
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("encode input snapshot references: %w", err)
	}
	systemAsOf, err := formatResearchflowTime(record.Run.SystemAsOf)
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("encode system_as_of: %w", err)
	}
	deadlineAt, err := formatResearchflowTime(record.Run.DeadlineAt)
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("encode deadline_at: %w", err)
	}
	createdAt := formatResearchflowTimeValue(dbNow)
	_, err = tx.ExecContext(ctx, `INSERT INTO researchflow_runs (
		run_ref, schema_version, status, version, system_as_of, deadline_at,
		template_ref, template_version, template_fingerprint, input_snapshot_refs,
		resolved_template_snapshot, parameters_snapshot, originating_request_id,
		correlation_id, result_summary_ref, termination_reason, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Run.Ref, record.Run.SchemaVersion, string(record.Run.Status), record.Run.Version,
		systemAsOf, deadlineAt, record.Run.TemplateRef, record.Run.TemplateVersion,
		record.Run.TemplateFingerprint, string(inputRefs), string(resolvedSnapshot),
		string(record.Parameters), nullableString(record.Run.OriginatingRequestID),
		record.Run.CorrelationID, nullableString(record.Run.ResultSummaryRef),
		nullableText(record.Run.TerminationReason), createdAt,
		nullableTime(record.Run.StartedAt), nullableTime(record.Run.FinishedAt),
	)
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("insert Researchflow run: %w", err)
	}

	for _, stage := range record.Stages {
		if stage.RunRef != record.Run.Ref {
			return application.CreateRunResult{}, fmt.Errorf("stage %q belongs to a different run", stage.Ref)
		}
		if err := insertResearchflowStage(ctx, tx, stage, dbNow); err != nil {
			return application.CreateRunResult{}, err
		}
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO researchflow_run_idempotency (
		caller_ref, operation, idempotency_key, request_fingerprint, run_ref, created_at
	) VALUES (?, ?, ?, ?, ?, ?)`,
		record.Scope.CallerRef, record.Scope.Operation, record.Scope.Key,
		record.Fingerprint, record.Run.Ref, createdAt,
	)
	if err != nil {
		if isMySQLDuplicateKey(err) {
			_ = tx.Rollback()
			existing, existingFingerprint, found, lookupErr := r.FindIdempotentRun(ctx, record.Scope)
			if lookupErr != nil {
				return application.CreateRunResult{}, fmt.Errorf("resolve concurrent Researchflow idempotency request: %w", lookupErr)
			}
			if found {
				if replayErr := domain.ValidateIdempotentReplay(existingFingerprint, record.Fingerprint); replayErr != nil {
					return application.CreateRunResult{}, replayErr
				}
				existing.Replayed = true
				return existing, nil
			}
		}
		return application.CreateRunResult{}, fmt.Errorf("insert Researchflow idempotency record: %w", err)
	}

	// Recheck after all writes so a request that crossed its deadline while
	// waiting on database locks cannot commit a newly created run.
	dbNow, err = readDatabaseUTC(tx, ctx)
	if err != nil {
		return application.CreateRunResult{}, err
	}
	if !record.Run.DeadlineAt.After(dbNow) {
		return application.CreateRunResult{}, fmt.Errorf("%w: deadline elapsed before transaction commit", domain.ErrInvalidCommand)
	}
	if err := tx.Commit(); err != nil {
		return application.CreateRunResult{}, fmt.Errorf("commit Researchflow create-run transaction: %w", err)
	}
	return application.CreateRunResult{
		Run: record.Run, Stages: append([]domain.Stage(nil), record.Stages...),
	}, nil
}

// CompareAndSwapRun updates Run state only; started_at is assigned from the database clock on entry to running.
func (r *MySQLRunRepository) CompareAndSwapRun(
	ctx context.Context,
	expectedVersion uint64,
	next domain.Run,
) (bool, error) {
	if err := r.validateRunRef(next.Ref); err != nil {
		return false, err
	}
	if expectedVersion == ^uint64(0) || next.Version != expectedVersion+1 || next.Status == "" {
		return false, errors.New("Researchflow Run CAS version or status is invalid")
	}
	if next.ResultSummary != nil || next.ResultSummaryRef != nil || next.FinishedAt != nil {
		return false, errors.New("Researchflow Run CAS cannot write terminal summary or finish time; use finalization")
	}
	var currentStatus string
	var currentReason, currentSummaryRef sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT status, termination_reason, result_summary_ref
		FROM researchflow_runs WHERE run_ref = ? AND version = ?`,
		next.Ref, expectedVersion,
	).Scan(&currentStatus, &currentReason, &currentSummaryRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("load current Researchflow Run state for CAS: %w", err)
	}
	if currentSummaryRef.Valid {
		return false, errors.New("Researchflow Run CAS cannot modify a Run with a ResultSummary; use finalization")
	}
	current := domain.Run{Status: domain.RunStatus(currentStatus)}
	if _, err := state.TransitionRun(current, next.Status, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("reject invalid Researchflow Run CAS transition %s -> %s: %w",
			currentStatus, next.Status, err)
	}
	currentReasonValue := ""
	if currentReason.Valid {
		currentReasonValue = currentReason.String
	}
	if next.TerminationReason != currentReasonValue {
		return false, errors.New("Researchflow Run CAS cannot change termination_reason; use cancellation or deadline transition")
	}
	if next.StartedAt != nil {
		return false, errors.New("Researchflow Run CAS cannot supply started_at; the database sets it when entering running")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE researchflow_runs SET
		status = ?, version = ?,
		started_at = CASE WHEN ? = ? THEN COALESCE(started_at, UTC_TIMESTAMP(6)) ELSE started_at END
		WHERE run_ref = ? AND version = ? AND status = ?
		AND COALESCE(termination_reason, '') = ? AND result_summary_ref IS NULL`,
		string(next.Status), next.Version, string(next.Status), string(domain.RunRunning),
		next.Ref, expectedVersion, currentStatus, currentReasonValue,
	)
	if err != nil {
		return false, fmt.Errorf("compare-and-swap Researchflow run %q: %w", next.Ref, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read Researchflow Run CAS affected rows: %w", err)
	}
	return affected == 1, nil
}

// CompareAndSwapStage is restricted to Scheduler-owned Pending -> Ready/Blocked transitions.
func (r *MySQLRunRepository) CompareAndSwapStage(
	ctx context.Context,
	expectedVersion uint64,
	next domain.Stage,
) (bool, error) {
	if err := r.validateRunRef(next.Ref); err != nil {
		return false, err
	}
	if strings.TrimSpace(next.RunRef) == "" || expectedVersion == ^uint64(0) ||
		next.Version != expectedVersion+1 || next.Status == "" || next.AttemptNo < 0 {
		return false, errors.New("Researchflow Stage CAS version or state is invalid")
	}
	var currentStatus, currentReasonsJSON string
	if err := r.db.QueryRowContext(ctx, `SELECT status, reason_codes FROM researchflow_stages
		WHERE stage_ref = ? AND run_ref = ? AND version = ?`,
		next.Ref, next.RunRef, expectedVersion,
	).Scan(&currentStatus, &currentReasonsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("load current Researchflow Stage status for CAS: %w", err)
	}
	if domain.StageStatus(currentStatus) != domain.StagePending ||
		(next.Status != domain.StageReady && next.Status != domain.StageBlocked) {
		return false, errors.New("Researchflow Stage CAS is reserved for Pending -> Ready/Blocked scheduler transitions")
	}
	if next.AttemptNo != 0 || next.RetryAt != nil || next.OutputRef != nil ||
		next.QualitySummaryRef != nil || len(next.DiagnosticRefs) != 0 ||
		strings.TrimSpace(next.QualityStatus) != "" || strings.TrimSpace(next.RiskDecision) != "" || next.StartedAt != nil {
		return false, errors.New("Researchflow Stage CAS cannot modify attempt, retry, result, quality or start fields")
	}
	var currentReasons []string
	if err := json.Unmarshal([]byte(currentReasonsJSON), &currentReasons); err != nil {
		return false, fmt.Errorf("decode current Researchflow Stage reason codes: %w", err)
	}
	expectedReasons := append([]string(nil), currentReasons...)
	if next.Status == domain.StageReady {
		if next.FinishedAt != nil || !equalResearchflowStrings(next.ReasonCodes, currentReasons) {
			return false, errors.New("Researchflow Ready transition cannot change Stage reasons or finish time")
		}
	} else {
		if next.FinishedAt == nil || next.FinishedAt.IsZero() {
			return false, errors.New("Researchflow Blocked transition requires a non-zero finish time")
		}
		if !containsResearchflowString(expectedReasons, "dependency_not_satisfied") {
			expectedReasons = append(expectedReasons, "dependency_not_satisfied")
		}
		if !equalResearchflowStrings(next.ReasonCodes, expectedReasons) {
			return false, errors.New("Researchflow Blocked transition may only append dependency_not_satisfied")
		}
		// The database clock, not the scheduler snapshot, owns the terminal timestamp.
	}
	if _, err := state.TransitionStage(domain.Stage{Status: domain.StageStatus(currentStatus)},
		next.Status, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("reject invalid Researchflow Stage CAS transition %s -> %s: %w",
			currentStatus, next.Status, err)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE researchflow_stages SET
		status = ?, version = ?, reason_codes = ?,
		finished_at = CASE WHEN ? = ? THEN UTC_TIMESTAMP(6) ELSE finished_at END
		WHERE stage_ref = ? AND run_ref = ? AND version = ? AND status = ?`,
		string(next.Status), next.Version, mustMarshalStrings(expectedReasons),
		string(next.Status), string(domain.StageBlocked),
		next.Ref, next.RunRef, expectedVersion, currentStatus,
	)
	if err != nil {
		return false, fmt.Errorf("compare-and-swap Researchflow stage %q: %w", next.Ref, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read Researchflow Stage CAS affected rows: %w", err)
	}
	return affected == 1, nil
}

func equalResearchflowStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func containsResearchflowString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mustMarshalStrings(values []string) string {
	encoded, err := json.Marshal(nonNilStrings(values))
	if err != nil {
		// A string slice cannot fail JSON encoding; retain a valid empty value
		// rather than allowing an impossible encoding error to alter the write.
		return "[]"
	}
	return string(encoded)
}

// GetRun loads a persisted run and its stages by immutable run reference.
func (r *MySQLRunRepository) GetRun(ctx context.Context, runRef string) (application.CreateRunResult, error) {
	if err := r.validateRunRef(runRef); err != nil {
		return application.CreateRunResult{}, err
	}
	return r.loadRun(ctx, runRef)
}

// GetStage loads one stage by immutable stage reference.
func (r *MySQLRunRepository) GetStage(ctx context.Context, stageRef string) (domain.Stage, error) {
	if err := r.validateRunRef(stageRef); err != nil {
		return domain.Stage{}, err
	}
	var stage domain.Stage
	var status, dependencies, diagnostics, reasonCodes, created string
	var retryAt, outputRef, qualitySummaryRef, qualityStatus, riskDecision, started, finished sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT
		stage_ref, run_ref, stage_code, status, version, required,
		dependency_stage_refs, retry_policy_ref, attempt_no, retry_at,
		output_ref, quality_summary_ref, diagnostic_refs, quality_status,
		reason_codes, risk_decision, created_at, started_at, finished_at
		FROM researchflow_stages WHERE stage_ref = ?`, stageRef,
	).Scan(
		&stage.Ref, &stage.RunRef, &stage.Code, &status, &stage.Version, &stage.Required,
		&dependencies, &stage.RetryPolicyRef, &stage.AttemptNo, &retryAt,
		&outputRef, &qualitySummaryRef, &diagnostics, &qualityStatus,
		&reasonCodes, &riskDecision, &created, &started, &finished,
	)
	if err != nil {
		return domain.Stage{}, fmt.Errorf("load Researchflow stage %q: %w", stageRef, err)
	}
	stage.Status = domain.StageStatus(status)
	if err := json.Unmarshal([]byte(dependencies), &stage.Dependencies); err != nil {
		return domain.Stage{}, fmt.Errorf("decode stage dependencies: %w", err)
	}
	if err := json.Unmarshal([]byte(diagnostics), &stage.DiagnosticRefs); err != nil {
		return domain.Stage{}, fmt.Errorf("decode stage diagnostics: %w", err)
	}
	if err := json.Unmarshal([]byte(reasonCodes), &stage.ReasonCodes); err != nil {
		return domain.Stage{}, fmt.Errorf("decode stage reason codes: %w", err)
	}
	if stage.Dependencies == nil {
		stage.Dependencies = []string{}
	}
	if stage.DiagnosticRefs == nil {
		stage.DiagnosticRefs = []string{}
	}
	if stage.ReasonCodes == nil {
		stage.ReasonCodes = []string{}
	}
	if stage.RetryAt, err = parseNullableResearchflowTime(retryAt); err != nil {
		return domain.Stage{}, err
	}
	if stage.CreatedAt, err = parseResearchflowTime(created); err != nil {
		return domain.Stage{}, err
	}
	if stage.StartedAt, err = parseNullableResearchflowTime(started); err != nil {
		return domain.Stage{}, err
	}
	if stage.FinishedAt, err = parseNullableResearchflowTime(finished); err != nil {
		return domain.Stage{}, err
	}
	stage.OutputRef = stringPointer(outputRef)
	stage.QualitySummaryRef = stringPointer(qualitySummaryRef)
	if qualityStatus.Valid {
		stage.QualityStatus = qualityStatus.String
	}
	if riskDecision.Valid {
		stage.RiskDecision = riskDecision.String
	}
	return stage, nil
}

// GetAttempt loads one immutable attempt record by reference.
func (r *MySQLRunRepository) GetAttempt(ctx context.Context, attemptRef string) (domain.Attempt, error) {
	if err := r.validateRunRef(attemptRef); err != nil {
		return domain.Attempt{}, err
	}
	var attempt domain.Attempt
	var status string
	var leaseOwner, leaseExpiresAt, finishedAt, faultRef, outputRef sql.NullString
	var startedAt string
	err := r.db.QueryRowContext(ctx, `SELECT
		attempt_ref, stage_ref, attempt_no, status, lease_owner, lease_expires_at,
		fencing_token, started_at, finished_at, fault_ref, output_ref
		FROM researchflow_attempts WHERE attempt_ref = ?`, attemptRef,
	).Scan(
		&attempt.Ref, &attempt.StageRef, &attempt.Number, &status, &leaseOwner,
		&leaseExpiresAt, &attempt.FencingToken, &startedAt, &finishedAt,
		&faultRef, &outputRef,
	)
	if err != nil {
		return domain.Attempt{}, fmt.Errorf("load Researchflow attempt %q: %w", attemptRef, err)
	}
	attempt.Status = domain.AttemptStatus(status)
	if leaseOwner.Valid {
		attempt.LeaseOwner = leaseOwner.String
	}
	if attempt.LeaseExpiresAt, err = parseNullableResearchflowTime(leaseExpiresAt); err != nil {
		return domain.Attempt{}, err
	}
	if attempt.StartedAt, err = parseResearchflowTime(startedAt); err != nil {
		return domain.Attempt{}, err
	}
	if attempt.FinishedAt, err = parseNullableResearchflowTime(finishedAt); err != nil {
		return domain.Attempt{}, err
	}
	attempt.FaultRef = stringPointer(faultRef)
	attempt.OutputRef = stringPointer(outputRef)
	return attempt, nil
}

func (r *MySQLRunRepository) validateRunRef(ref string) error {
	if r == nil || r.db == nil {
		return errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(ref) == "" || len(ref) > 255 {
		return errors.New("Researchflow reference is invalid")
	}
	return nil
}

func (r *MySQLRunRepository) loadRun(ctx context.Context, runRef string) (application.CreateRunResult, error) {
	var run domain.Run
	var status string
	var systemAsOf, deadlineAt, createdAt string
	var inputRefs, resolvedSnapshot string
	var requestID, resultSummaryRef, terminationReason, startedAt, finishedAt sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT
		run_ref, schema_version, status, version, system_as_of, deadline_at,
		template_ref, template_version, template_fingerprint, input_snapshot_refs,
		resolved_template_snapshot, originating_request_id, correlation_id,
		result_summary_ref, termination_reason, created_at, started_at, finished_at
		FROM researchflow_runs WHERE run_ref = ?`, runRef,
	).Scan(
		&run.Ref, &run.SchemaVersion, &status, &run.Version, &systemAsOf, &deadlineAt,
		&run.TemplateRef, &run.TemplateVersion, &run.TemplateFingerprint, &inputRefs,
		&resolvedSnapshot, &requestID, &run.CorrelationID, &resultSummaryRef,
		&terminationReason, &createdAt, &startedAt, &finishedAt,
	)
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("load Researchflow run %q: %w", runRef, err)
	}
	run.Status = domain.RunStatus(status)
	if run.SystemAsOf, err = parseResearchflowTime(systemAsOf); err != nil {
		return application.CreateRunResult{}, err
	}
	if run.DeadlineAt, err = parseResearchflowTime(deadlineAt); err != nil {
		return application.CreateRunResult{}, err
	}
	if run.CreatedAt, err = parseResearchflowTime(createdAt); err != nil {
		return application.CreateRunResult{}, err
	}
	if run.StartedAt, err = parseNullableResearchflowTime(startedAt); err != nil {
		return application.CreateRunResult{}, err
	}
	if run.FinishedAt, err = parseNullableResearchflowTime(finishedAt); err != nil {
		return application.CreateRunResult{}, err
	}
	run.OriginatingRequestID = stringPointer(requestID)
	run.ResultSummaryRef = stringPointer(resultSummaryRef)
	if terminationReason.Valid {
		run.TerminationReason = terminationReason.String
	}
	if err := json.Unmarshal([]byte(inputRefs), &run.InputSnapshotRefs); err != nil {
		return application.CreateRunResult{}, fmt.Errorf("decode Researchflow input snapshot references: %w", err)
	}
	if run.InputSnapshotRefs == nil {
		run.InputSnapshotRefs = []string{}
	}
	var resolved template.ResolvedTemplate
	if err := json.Unmarshal([]byte(resolvedSnapshot), &resolved); err != nil {
		return application.CreateRunResult{}, fmt.Errorf("decode Researchflow resolved template snapshot: %w", err)
	}
	run.ExcludedStages = append([]domain.ExcludedStage{}, resolved.Excluded...)

	run.ResultSummary, err = r.loadResultSummary(ctx, run.Ref, run.ResultSummaryRef, run.Status)
	if err != nil {
		return application.CreateRunResult{}, err
	}

	rows, err := r.db.QueryContext(ctx, `SELECT
		stage_ref, run_ref, stage_code, status, version, required,
		dependency_stage_refs, retry_policy_ref, attempt_no, retry_at,
		output_ref, quality_summary_ref, diagnostic_refs, quality_status,
		reason_codes, risk_decision, created_at, started_at, finished_at
		FROM researchflow_stages WHERE run_ref = ?`, runRef)
	if err != nil {
		return application.CreateRunResult{}, fmt.Errorf("query Researchflow stages: %w", err)
	}
	defer rows.Close()
	stages := make([]domain.Stage, 0)
	for rows.Next() {
		var stage domain.Stage
		var stageStatus, dependencies, diagnostics, reasonCodes, created string
		var required bool
		var retryAt, outputRef, qualitySummaryRef, qualityStatus, riskDecision, started, finished sql.NullString
		if err := rows.Scan(
			&stage.Ref, &stage.RunRef, &stage.Code, &stageStatus, &stage.Version, &required,
			&dependencies, &stage.RetryPolicyRef, &stage.AttemptNo, &retryAt,
			&outputRef, &qualitySummaryRef, &diagnostics, &qualityStatus,
			&reasonCodes, &riskDecision, &created, &started, &finished,
		); err != nil {
			return application.CreateRunResult{}, fmt.Errorf("scan Researchflow stage: %w", err)
		}
		stage.Status = domain.StageStatus(stageStatus)
		stage.Required = required
		if err := json.Unmarshal([]byte(dependencies), &stage.Dependencies); err != nil {
			return application.CreateRunResult{}, fmt.Errorf("decode stage dependencies: %w", err)
		}
		if err := json.Unmarshal([]byte(diagnostics), &stage.DiagnosticRefs); err != nil {
			return application.CreateRunResult{}, fmt.Errorf("decode stage diagnostics: %w", err)
		}
		if err := json.Unmarshal([]byte(reasonCodes), &stage.ReasonCodes); err != nil {
			return application.CreateRunResult{}, fmt.Errorf("decode stage reason codes: %w", err)
		}
		if stage.Dependencies == nil {
			stage.Dependencies = []string{}
		}
		if stage.DiagnosticRefs == nil {
			stage.DiagnosticRefs = []string{}
		}
		if stage.ReasonCodes == nil {
			stage.ReasonCodes = []string{}
		}
		if stage.RetryAt, err = parseNullableResearchflowTime(retryAt); err != nil {
			return application.CreateRunResult{}, err
		}
		if stage.CreatedAt, err = parseResearchflowTime(created); err != nil {
			return application.CreateRunResult{}, err
		}
		if stage.StartedAt, err = parseNullableResearchflowTime(started); err != nil {
			return application.CreateRunResult{}, err
		}
		if stage.FinishedAt, err = parseNullableResearchflowTime(finished); err != nil {
			return application.CreateRunResult{}, err
		}
		stage.OutputRef = stringPointer(outputRef)
		stage.QualitySummaryRef = stringPointer(qualitySummaryRef)
		if qualityStatus.Valid {
			stage.QualityStatus = qualityStatus.String
		}
		if riskDecision.Valid {
			stage.RiskDecision = riskDecision.String
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return application.CreateRunResult{}, fmt.Errorf("iterate Researchflow stages: %w", err)
	}
	order := make(map[string]int, len(resolved.Stages))
	for index, stage := range resolved.Stages {
		order[stage.Definition.Code] = index
	}
	sortResearchflowStages(stages, order)
	return application.CreateRunResult{Run: run, Stages: stages}, nil
}

// loadResultSummary restores the immutable summary referenced by a terminal Run.
func (r *MySQLRunRepository) loadResultSummary(
	ctx context.Context,
	runRef string,
	summaryRef *string,
	status domain.RunStatus,
) (*domain.ResultSummary, error) {
	if status.Terminal() && (summaryRef == nil || strings.TrimSpace(*summaryRef) == "") {
		return nil, fmt.Errorf("terminal Researchflow Run %q has no ResultSummary reference", runRef)
	}
	if !status.Terminal() && summaryRef != nil {
		return nil, fmt.Errorf("non-terminal Researchflow Run %q unexpectedly references a ResultSummary", runRef)
	}
	if summaryRef == nil {
		return nil, nil
	}
	var schemaVersion, payload string
	err := r.db.QueryRowContext(ctx, `SELECT schema_version, payload
		FROM researchflow_result_summaries
		WHERE result_summary_ref = ? AND run_ref = ?`, *summaryRef, runRef,
	).Scan(&schemaVersion, &payload)
	if err != nil {
		return nil, fmt.Errorf("load Researchflow ResultSummary %q: %w", *summaryRef, err)
	}
	if strings.TrimSpace(schemaVersion) == "" {
		return nil, fmt.Errorf("Researchflow ResultSummary %q has no schema version", *summaryRef)
	}
	summary, err := decodeResultSummary([]byte(payload))
	if err != nil {
		return nil, fmt.Errorf("decode Researchflow ResultSummary %q: %w", *summaryRef, err)
	}
	return &summary, nil
}

func decodeResultSummary(payload []byte) (domain.ResultSummary, error) {
	var summary domain.ResultSummary
	if len(bytes.TrimSpace(payload)) == 0 {
		return summary, errors.New("ResultSummary payload is empty")
	}
	if err := json.Unmarshal(payload, &summary); err != nil {
		return summary, fmt.Errorf("invalid ResultSummary JSON: %w", err)
	}
	if summary.StageOutcomes == nil || summary.RiskGateDecisions == nil ||
		summary.MissingStageCodes == nil || summary.DegradedStageCodes == nil ||
		summary.BlockedStageCodes == nil || summary.ReasonCodes == nil ||
		summary.ExcludedStages == nil {
		return summary, errors.New("ResultSummary payload is missing required collections")
	}
	if summary.Coverage < 0 || summary.Coverage > 1 {
		return summary, errors.New("ResultSummary coverage is outside [0,1]")
	}
	if strings.TrimSpace(summary.QualityStatus) == "" ||
		strings.TrimSpace(summary.ActionabilityStatus) == "" {
		return summary, errors.New("ResultSummary quality or actionability status is missing")
	}
	return summary, nil
}

func (r *MySQLRunRepository) validateScope(scope application.IdempotencyScope) error {
	if r == nil || r.db == nil {
		return errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(scope.CallerRef) == "" || strings.TrimSpace(scope.Operation) == "" ||
		strings.TrimSpace(scope.Key) == "" || len(scope.Key) > 128 {
		return errors.New("Researchflow idempotency scope is invalid")
	}
	return nil
}

func (r *MySQLRunRepository) validateRecord(record application.CreateRunRecord) error {
	if err := r.validateScope(record.Scope); err != nil {
		return err
	}
	if !validSHA256Fingerprint(record.Fingerprint) ||
		record.Run.Ref == "" || record.Run.SchemaVersion == "" || record.Run.TemplateRef == "" ||
		record.Run.TemplateVersion == "" || !validSHA256Fingerprint(record.Run.TemplateFingerprint) ||
		record.Run.CorrelationID == "" || record.Run.Version != 1 || record.Run.Status != domain.RunCreated {
		return errors.New("Researchflow create-run record is incomplete")
	}
	if record.Run.StartedAt != nil || record.Run.FinishedAt != nil ||
		record.Run.ResultSummary != nil || record.Run.ResultSummaryRef != nil ||
		strings.TrimSpace(record.Run.TerminationReason) != "" {
		return errors.New("Researchflow CreateRun cannot initialize lifecycle or terminal Run fields")
	}
	parameters := bytes.TrimSpace(record.Parameters)
	if len(parameters) == 0 || !json.Valid(parameters) || parameters[0] != '{' ||
		record.ResolvedTemplate.Ref != record.Run.TemplateRef ||
		record.ResolvedTemplate.Version != record.Run.TemplateVersion ||
		record.ResolvedTemplate.Fingerprint != record.Run.TemplateFingerprint {
		return errors.New("Researchflow create-run snapshots must be valid and match the run identity")
	}
	if len(record.Stages) == 0 {
		return errors.New("Researchflow create-run requires at least one stage")
	}
	for _, stage := range record.Stages {
		if stage.Version != 1 || stage.AttemptNo != 0 ||
			(stage.Status != domain.StagePending && stage.Status != domain.StageSkipped) ||
			stage.RetryAt != nil || stage.OutputRef != nil || stage.QualitySummaryRef != nil ||
			len(stage.DiagnosticRefs) != 0 || strings.TrimSpace(stage.QualityStatus) != "" ||
			strings.TrimSpace(stage.RiskDecision) != "" || stage.StartedAt != nil || stage.FinishedAt != nil {
			return fmt.Errorf("Researchflow CreateRun Stage %q contains fields owned by later lifecycle transitions", stage.Ref)
		}
	}
	return nil
}

func insertResearchflowStage(ctx context.Context, tx *sql.Tx, stage domain.Stage, created time.Time) error {
	dependencies, err := json.Marshal(nonNilStrings(stage.Dependencies))
	if err != nil {
		return fmt.Errorf("encode stage %q dependencies: %w", stage.Ref, err)
	}
	diagnostics, err := json.Marshal(nonNilStrings(stage.DiagnosticRefs))
	if err != nil {
		return fmt.Errorf("encode stage %q diagnostics: %w", stage.Ref, err)
	}
	reasons, err := json.Marshal(nonNilStrings(stage.ReasonCodes))
	if err != nil {
		return fmt.Errorf("encode stage %q reason codes: %w", stage.Ref, err)
	}
	createdAt := formatResearchflowTimeValue(created)
	_, err = tx.ExecContext(ctx, `INSERT INTO researchflow_stages (
		stage_ref, run_ref, stage_code, status, version, required,
		dependency_stage_refs, retry_policy_ref, attempt_no, retry_at,
		output_ref, quality_summary_ref, diagnostic_refs, quality_status,
		reason_codes, risk_decision, created_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		stage.Ref, stage.RunRef, stage.Code, string(stage.Status), stage.Version, stage.Required,
		string(dependencies), stage.RetryPolicyRef, stage.AttemptNo, nullableTime(stage.RetryAt),
		nullableString(stage.OutputRef), nullableString(stage.QualitySummaryRef),
		string(diagnostics), nullableText(stage.QualityStatus), string(reasons),
		nullableText(stage.RiskDecision), createdAt, nullableTime(stage.StartedAt),
		nullableTime(stage.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("insert Researchflow stage %q: %w", stage.Code, err)
	}
	return nil
}

func readDatabaseUTC(tx *sql.Tx, ctx context.Context) (time.Time, error) {
	var value string
	if err := tx.QueryRowContext(ctx, "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')").Scan(&value); err != nil {
		return time.Time{}, fmt.Errorf("read MariaDB UTC clock: %w", err)
	}
	parsed, err := parseResearchflowTime(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse MariaDB UTC clock: %w", err)
	}
	return parsed, nil
}

func formatResearchflowTime(value time.Time) (string, error) {
	if value.IsZero() {
		return "", errors.New("timestamp must not be zero")
	}
	return formatResearchflowTimeValue(value.UTC()), nil
}

// calculateResearchflowLeaseExpiry calculates a lease against the database clock
// and caps it at the immutable Run deadline. Researchflow timestamps are stored
// as fixed-width CHAR(30) values with nanosecond precision, not DATETIME(6).
func calculateResearchflowLeaseExpiry(now time.Time, duration time.Duration, deadline time.Time) (time.Time, error) {
	if duration <= 0 {
		return time.Time{}, errors.New("Researchflow lease duration must be positive")
	}
	expiresAt := now.Add(duration)
	if expiresAt.After(deadline) {
		expiresAt = deadline
	}
	if !expiresAt.After(now) {
		return time.Time{}, errors.New("Researchflow lease expiry must be after database time and within Run deadline")
	}
	// Keep the persisted representation fixed-width so lexical SQL comparisons
	// preserve chronological ordering for every accepted timestamp.
	if len(formatResearchflowTimeValue(expiresAt)) != len(researchflowTimestampLayout) {
		return time.Time{}, errors.New("Researchflow lease expiry is outside the supported timestamp range")
	}
	return expiresAt, nil
}

func formatResearchflowTimeValue(value time.Time) string {
	return value.UTC().Format(researchflowTimestampLayout)
}

func parseResearchflowTime(value string) (time.Time, error) {
	parsed, err := time.Parse(researchflowTimestampLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid Researchflow timestamp %q: %w", value, err)
	}
	return parsed.UTC(), nil
}

func parseNullableResearchflowTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseResearchflowTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatResearchflowTimeValue(*value)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func isMySQLDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func sortResearchflowStages(stages []domain.Stage, order map[string]int) {
	sort.SliceStable(stages, func(i, j int) bool {
		left, leftOK := order[stages[i].Code]
		right, rightOK := order[stages[j].Code]
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK {
			return left < right
		}
		return stages[i].Code < stages[j].Code
	})
}

func validSHA256Fingerprint(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

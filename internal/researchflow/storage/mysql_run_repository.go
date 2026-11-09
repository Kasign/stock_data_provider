package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
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
	run.ExcludedStages = append([]domain.ExcludedStage(nil), resolved.Excluded...)

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
	if strings.TrimSpace(record.Fingerprint) == "" || len(record.Fingerprint) != 64 ||
		record.Run.Ref == "" || record.Run.SchemaVersion == "" || record.Run.TemplateRef == "" ||
		record.Run.TemplateVersion == "" || record.Run.TemplateFingerprint == "" ||
		record.Run.CorrelationID == "" || record.Run.Version == 0 || record.Run.Status != domain.RunCreated {
		return errors.New("Researchflow create-run record is incomplete")
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

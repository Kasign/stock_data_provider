package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var _ application.WorkerStore = (*MySQLRunRepository)(nil)

// LoadStageExecutionSnapshot reads all immutable execution inputs from one
// InnoDB repeatable-read snapshot. Retry policy comes from the persisted
// resolved-template snapshot, never from the mutable template registry.
func (r *MySQLRunRepository) LoadStageExecutionSnapshot(
	ctx context.Context, runRef, stageRef string,
) (application.StageExecutionSnapshot, error) {
	if r == nil || r.db == nil {
		return application.StageExecutionSnapshot{}, errors.New("researchflow SQL database is not configured")
	}
	if err := r.validateRunRef(runRef); err != nil {
		return application.StageExecutionSnapshot{}, err
	}
	if err := r.validateRunRef(stageRef); err != nil {
		return application.StageExecutionSnapshot{}, err
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("begin Researchflow execution snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var run domain.Run
	var runStatus, systemAsOf, deadlineAt string
	var inputRefsJSON, resolvedJSON, parametersJSON string
	err = tx.QueryRowContext(ctx, `SELECT
		run_ref, schema_version, status, version, system_as_of, deadline_at,
		template_ref, template_version, template_fingerprint, input_snapshot_refs,
		resolved_template_snapshot, parameters_snapshot, correlation_id
		FROM researchflow_runs WHERE run_ref = ?`, runRef,
	).Scan(
		&run.Ref, &run.SchemaVersion, &runStatus, &run.Version, &systemAsOf, &deadlineAt,
		&run.TemplateRef, &run.TemplateVersion, &run.TemplateFingerprint,
		&inputRefsJSON, &resolvedJSON, &parametersJSON, &run.CorrelationID,
	)
	if err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("load Researchflow execution Run: %w", err)
	}
	run.Status = domain.RunStatus(runStatus)
	if run.SystemAsOf, err = parseResearchflowTime(systemAsOf); err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("decode Researchflow system_as_of: %w", err)
	}
	if run.DeadlineAt, err = parseResearchflowTime(deadlineAt); err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("decode Researchflow deadline_at: %w", err)
	}
	if err := json.Unmarshal([]byte(inputRefsJSON), &run.InputSnapshotRefs); err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("decode Researchflow input snapshot references: %w", err)
	}
	if run.InputSnapshotRefs == nil {
		run.InputSnapshotRefs = []string{}
	}
	parameters := json.RawMessage(append([]byte(nil), parametersJSON...))
	if !json.Valid(parameters) {
		return application.StageExecutionSnapshot{}, errors.New("Researchflow persisted parameters snapshot is invalid JSON")
	}

	stage, err := scanWorkerStage(tx.QueryRowContext(ctx, `SELECT
		stage_ref, run_ref, stage_code, status, version, required,
		dependency_stage_refs, retry_policy_ref, attempt_no, retry_at,
		output_ref, quality_summary_ref, diagnostic_refs, quality_status,
		reason_codes, risk_decision, created_at, started_at, finished_at
		FROM researchflow_stages WHERE stage_ref = ? AND run_ref = ?`, stageRef, runRef))
	if err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("load Researchflow execution Stage: %w", err)
	}

	var resolved template.ResolvedTemplate
	if err := json.Unmarshal([]byte(resolvedJSON), &resolved); err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("decode Researchflow resolved template snapshot: %w", err)
	}
	if resolved.Ref != run.TemplateRef || resolved.Version != run.TemplateVersion ||
		resolved.Fingerprint != run.TemplateFingerprint {
		return application.StageExecutionSnapshot{}, errors.New("Researchflow resolved template identity differs from Run")
	}
	var retryPolicy *template.RetryPolicy
	for i := range resolved.Stages {
		candidate := &resolved.Stages[i]
		if candidate.Definition.Code != stage.Code {
			continue
		}
		if retryPolicy != nil {
			return application.StageExecutionSnapshot{}, errors.New("Researchflow resolved template contains duplicate Stage codes")
		}
		if candidate.Definition.Retry.Ref != stage.RetryPolicyRef {
			return application.StageExecutionSnapshot{}, errors.New("Researchflow Stage retry policy reference differs from resolved template")
		}
		policy := candidate.Definition.Retry
		policy.RetryableReasonCodes = append([]string(nil), policy.RetryableReasonCodes...)
		retryPolicy = &policy
	}
	if retryPolicy == nil || retryPolicy.MaxAttempts <= 0 || retryPolicy.AttemptTimeoutMS <= 0 {
		return application.StageExecutionSnapshot{}, errors.New("Researchflow Stage retry policy is missing or invalid in resolved template")
	}
	if stage.RunRef != run.Ref || stage.Ref != stageRef {
		return application.StageExecutionSnapshot{}, errors.New("Researchflow execution snapshot references are inconsistent")
	}
	if err := tx.Commit(); err != nil {
		return application.StageExecutionSnapshot{}, fmt.Errorf("finish Researchflow execution snapshot: %w", err)
	}
	return application.StageExecutionSnapshot{
		Run: run, Stage: stage, Parameters: parameters,
		InputSnapshotRefs: append([]string(nil), run.InputSnapshotRefs...),
		RetryPolicy: *retryPolicy,
	}, nil
}

type workerStageScanner interface {
	Scan(dest ...any) error
}

func scanWorkerStage(row workerStageScanner) (domain.Stage, error) {
	var stage domain.Stage
	var status, dependencies, diagnostics, reasons, created string
	var retryAt, outputRef, qualitySummaryRef, qualityStatus, riskDecision, started, finished sql.NullString
	if err := row.Scan(
		&stage.Ref, &stage.RunRef, &stage.Code, &status, &stage.Version, &stage.Required,
		&dependencies, &stage.RetryPolicyRef, &stage.AttemptNo, &retryAt,
		&outputRef, &qualitySummaryRef, &diagnostics, &qualityStatus,
		&reasons, &riskDecision, &created, &started, &finished,
	); err != nil {
		return domain.Stage{}, err
	}
	stage.Status = domain.StageStatus(status)
	if err := json.Unmarshal([]byte(dependencies), &stage.Dependencies); err != nil {
		return domain.Stage{}, fmt.Errorf("decode dependencies: %w", err)
	}
	if err := json.Unmarshal([]byte(diagnostics), &stage.DiagnosticRefs); err != nil {
		return domain.Stage{}, fmt.Errorf("decode diagnostics: %w", err)
	}
	if err := json.Unmarshal([]byte(reasons), &stage.ReasonCodes); err != nil {
		return domain.Stage{}, fmt.Errorf("decode reason codes: %w", err)
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
	var err error
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

// CommitWorkerResult is a thin Application-to-Storage mapping. Validation and
// the atomic lease/fencing transaction remain owned by CommitAttemptResult.
func (r *MySQLRunRepository) CommitWorkerResult(ctx context.Context, input application.WorkerResultCommit) error {
	commit, err := workerResultCommitToStorage(input)
	if err != nil {
		return err
	}
	return r.CommitAttemptResult(ctx, commit)
}

func workerResultCommitToStorage(input application.WorkerResultCommit) (AttemptResultCommit, error) {
	if input.RunRef == "" || input.StageRef == "" || input.AttemptRef == "" ||
		input.WorkerID == "" || input.FencingToken == 0 ||
		input.Stage.Ref != input.StageRef || input.Stage.RunRef != input.RunRef {
		return AttemptResultCommit{}, errors.New("Researchflow worker result mapping arguments are invalid")
	}
	if (input.OutputRef == nil) != (input.Stage.OutputRef == nil) ||
		(input.OutputRef != nil && *input.OutputRef != *input.Stage.OutputRef) {
		return AttemptResultCommit{}, errors.New("Researchflow worker result output reference differs from Stage")
	}
	return AttemptResultCommit{
		RunRef: input.RunRef, StageRef: input.StageRef, AttemptRef: input.AttemptRef,
		WorkerID: input.WorkerID, FencingToken: input.FencingToken,
		AttemptStatus: input.AttemptStatus, Stage: input.Stage,
		FaultRef: cloneWorkerString(input.FaultRef), OutputRef: cloneWorkerString(input.OutputRef),
	}, nil
}

func cloneWorkerString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

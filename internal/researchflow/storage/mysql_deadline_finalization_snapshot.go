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

// LoadDeadlineFinalizationSnapshot reconstructs the finalization inputs from
// the immutable Run snapshot and persisted Stage/Attempt history. It never
// resolves a live Template Registry version.
func (r *MySQLRunRepository) LoadDeadlineFinalizationSnapshot(ctx context.Context, runRef string) (application.DeadlineFinalizationSnapshot, error) {
	if err := r.validateRunRef(runRef); err != nil {
		return application.DeadlineFinalizationSnapshot{}, err
	}
	created, err := r.loadRun(ctx, runRef)
	if err != nil {
		return application.DeadlineFinalizationSnapshot{}, err
	}
	var resolvedJSON string
	if err := r.db.QueryRowContext(ctx, `SELECT resolved_template_snapshot
		FROM researchflow_runs WHERE run_ref = ?`, runRef).Scan(&resolvedJSON); err != nil {
		return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("load Researchflow resolved template for deadline finalization: %w", err)
	}
	var resolved template.ResolvedTemplate
	if err := json.Unmarshal([]byte(resolvedJSON), &resolved); err != nil {
		return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("decode Researchflow resolved template for deadline finalization: %w", err)
	}
	if resolved.Ref != created.Run.TemplateRef || resolved.Version != created.Run.TemplateVersion ||
		resolved.Fingerprint != created.Run.TemplateFingerprint {
		return application.DeadlineFinalizationSnapshot{}, errors.New("Researchflow resolved template snapshot does not match Run identity")
	}

	policies := make(map[string]template.PropagationPolicy, len(resolved.Stages))
	hardRiskStages := make(map[string]bool)
	for _, stage := range resolved.Stages {
		code := stage.Definition.Code
		if code == "" {
			return application.DeadlineFinalizationSnapshot{}, errors.New("Researchflow resolved template contains an empty Stage code")
		}
		if _, exists := policies[code]; exists {
			return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("Researchflow resolved template contains duplicate Stage %q", code)
		}
		policies[code] = stage.Definition.Propagation
		if stage.Definition.GateType == "hard_risk" {
			hardRiskStages[code] = true
		}
	}

	rows, err := r.db.QueryContext(ctx, `SELECT
		attempt_ref, stage_ref, attempt_no, status, lease_owner, lease_expires_at,
		fencing_token, started_at, finished_at, fault_ref, output_ref
		FROM researchflow_attempts WHERE run_ref = ?
		ORDER BY stage_ref, attempt_no`, runRef)
	if err != nil {
		return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("query Researchflow deadline Attempts: %w", err)
	}
	defer rows.Close()
	attempts := make([]domain.Attempt, 0)
	for rows.Next() {
		var attempt domain.Attempt
		var status, startedAt string
		var leaseOwner, leaseExpiresAt, finishedAt, faultRef, outputRef sql.NullString
		if err := rows.Scan(
			&attempt.Ref, &attempt.StageRef, &attempt.Number, &status, &leaseOwner,
			&leaseExpiresAt, &attempt.FencingToken, &startedAt, &finishedAt, &faultRef, &outputRef,
		); err != nil {
			return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("scan Researchflow deadline Attempt: %w", err)
		}
		attempt.Status = domain.AttemptStatus(status)
		if attempt.StartedAt, err = parseResearchflowTime(startedAt); err != nil {
			return application.DeadlineFinalizationSnapshot{}, err
		}
		if attempt.LeaseExpiresAt, err = parseNullableResearchflowTime(leaseExpiresAt); err != nil {
			return application.DeadlineFinalizationSnapshot{}, err
		}
		if attempt.FinishedAt, err = parseNullableResearchflowTime(finishedAt); err != nil {
			return application.DeadlineFinalizationSnapshot{}, err
		}
		attempt.FaultRef = stringPointer(faultRef)
		attempt.OutputRef = stringPointer(outputRef)
		if leaseOwner.Valid {
			attempt.LeaseOwner = leaseOwner.String
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("iterate Researchflow deadline Attempts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return application.DeadlineFinalizationSnapshot{}, fmt.Errorf("close Researchflow deadline Attempts: %w", err)
	}
	return application.DeadlineFinalizationSnapshot{
		Run: created.Run, Stages: created.Stages, Attempts: attempts,
		PropagationPolicies: policies, HardRiskStageCodes: hardRiskStages,
		RequiresHardRiskGate: resolved.RequiresHardRiskGate,
	}, nil
}

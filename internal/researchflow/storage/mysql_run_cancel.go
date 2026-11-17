package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

var ErrRunAlreadyTerminal = application.ErrRunAlreadyTerminal

// BeginRunCancellation establishes the cancellation linearization point.
// It serializes with ClaimAttempt and CommitAttemptResult through the Run row
// lock, prevents further claims/results, and cancels stages that have not
// started. Running stages remain running until their worker stop is confirmed
// or their lease is fenced by the recovery path.
func (r *MySQLRunRepository) BeginRunCancellation(
	ctx context.Context,
	runRef string,
	expectedVersion uint64,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(runRef) == "" || len(runRef) > 255 || expectedVersion == ^uint64(0) {
		return false, errors.New("Researchflow cancellation arguments are invalid")
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, fmt.Errorf("begin Researchflow cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status, deadlineAt string
	var version uint64
	var terminationReason sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, version, termination_reason, deadline_at
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, runRef,
	).Scan(&status, &version, &terminationReason, &deadlineAt); err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation run %q: %w", runRef, err)
	}
	runStatus := domain.RunStatus(status)
	if runStatus.Terminal() {
		return false, ErrRunAlreadyTerminal
	}
	if terminationReason.Valid && terminationReason.String == "deadline_exceeded" {
		return false, errors.Join(ErrClaimRunUnavailable, application.ErrRunCancellationUnavailable)
	}
	if runStatus == domain.RunCancelling {
		if terminationReason.Valid && terminationReason.String == "user_cancelled" {
			if err := tx.Commit(); err != nil {
				return false, fmt.Errorf("commit idempotent Researchflow cancellation: %w", err)
			}
			return false, nil
		}
		return false, errors.Join(ErrClaimRunUnavailable, application.ErrRunCancellationUnavailable)
	}
	if runStatus != domain.RunCreated && runStatus != domain.RunQueued && runStatus != domain.RunRunning {
		return false, errors.Join(ErrClaimRunUnavailable, application.ErrRunCancellationUnavailable)
	}
	if version != expectedVersion {
		return false, nil
	}
	if version == ^uint64(0) {
		return false, errors.New("Researchflow run version exhausted")
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return false, err
	}
	deadline, err := parseResearchflowTime(deadlineAt)
	if err != nil {
		return false, fmt.Errorf("parse Researchflow cancellation deadline: %w", err)
	}
	if !deadline.After(now) {
		return false, errors.Join(ErrClaimRunUnavailable, application.ErrRunCancellationUnavailable)
	}
	rows, err := tx.QueryContext(ctx, `SELECT stage_ref, status, version, reason_codes
		FROM researchflow_stages WHERE run_ref = ? ORDER BY stage_ref FOR UPDATE`, runRef)
	if err != nil {
		return false, fmt.Errorf("lock Researchflow cancellation stages: %w", err)
	}
	type stageCancellation struct {
		ref     string
		status  domain.StageStatus
		version uint64
		reasons []string
	}
	stages := make([]stageCancellation, 0)
	for rows.Next() {
		var item stageCancellation
		var stageStatus, reasonJSON string
		if err := rows.Scan(&item.ref, &stageStatus, &item.version, &reasonJSON); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("scan Researchflow cancellation stage: %w", err)
		}
		item.status = domain.StageStatus(stageStatus)
		if err := json.Unmarshal([]byte(reasonJSON), &item.reasons); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("decode Researchflow cancellation stage reasons: %w", err)
		}
		stages = append(stages, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, fmt.Errorf("iterate Researchflow cancellation stages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("close Researchflow cancellation stages: %w", err)
	}

	for _, stage := range stages {
		switch stage.status {
		case domain.StagePending, domain.StageReady, domain.StageRetryWait:
			if stage.version == ^uint64(0) {
				return false, fmt.Errorf("Researchflow stage %q version exhausted", stage.ref)
			}
			reasons := append([]string(nil), stage.reasons...)
			reasons = append(reasons, "user_cancelled")
			encoded, marshalErr := json.Marshal(nonNilStrings(reasons))
			if marshalErr != nil {
				return false, fmt.Errorf("encode Researchflow cancellation reasons: %w", marshalErr)
			}
			result, updateErr := tx.ExecContext(ctx, `UPDATE researchflow_stages SET
				status = ?, version = version + 1, retry_at = NULL,
				reason_codes = ?, finished_at = ?
				WHERE stage_ref = ? AND run_ref = ? AND version = ? AND status = ?`,
				string(domain.StageCancelled), string(encoded), formatResearchflowTimeValue(now),
				stage.ref, runRef, stage.version, string(stage.status))
			if updateErr != nil {
				return false, fmt.Errorf("cancel Researchflow stage %q: %w", stage.ref, updateErr)
			}
			if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
				if rowsErr != nil {
					return false, fmt.Errorf("verify Researchflow stage cancellation: %w", rowsErr)
				}
				return false, fmt.Errorf("Researchflow stage %q changed while cancellation was locked", stage.ref)
			}
		}
	}

	result, err := tx.ExecContext(ctx, `UPDATE researchflow_runs SET
		status = ?, version = version + 1, termination_reason = ?
		WHERE run_ref = ? AND version = ? AND status = ?`,
		string(domain.RunCancelling), "user_cancelled", runRef, expectedVersion, string(runStatus))
	if err != nil {
		return false, fmt.Errorf("mark Researchflow run cancelling: %w", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return false, fmt.Errorf("verify Researchflow run cancellation: %w", rowsErr)
		}
		return false, errors.New("Researchflow run changed while cancellation was locked")
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit Researchflow cancellation: %w", err)
	}
	return true, nil
}

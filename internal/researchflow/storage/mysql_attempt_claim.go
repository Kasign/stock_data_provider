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

var (
	ErrClaimRunUnavailable   = errors.New("Researchflow run is not claimable")
	ErrClaimStageUnavailable = errors.New("Researchflow stage is not claimable")
	ErrAttemptLeaseHeld      = errors.New("Researchflow attempt lease is held by another worker")
	ErrAttemptLimitReached   = errors.New("Researchflow stage attempt limit reached")
	ErrRunConcurrencyLimit   = errors.New("Researchflow run parallel stage limit reached")
)

// ClaimAttempt atomically claims a ready stage and creates its next attempt.
// Lock order is fixed as Run -> Stage -> latest Attempt. attemptRef is used only
// when a new attempt is created; a repeated claim by the current lease owner
// returns the existing attempt without consuming another attempt number.
func (r *MySQLRunRepository) ClaimAttempt(
	ctx context.Context,
	runRef, stageRef, attemptRef, workerID string,
	leaseDuration time.Duration,
	maxAttempts int,
	maxParallel int,
) (domain.Attempt, bool, error) {
	if r == nil || r.db == nil {
		return domain.Attempt{}, false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(runRef) == "" || strings.TrimSpace(stageRef) == "" ||
		strings.TrimSpace(attemptRef) == "" || strings.TrimSpace(workerID) == "" ||
		len(workerID) > 128 || leaseDuration <= 0 || maxAttempts <= 0 || maxParallel <= 0 {
		return domain.Attempt{}, false, errors.New("Researchflow attempt claim arguments are invalid")
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("begin Researchflow attempt claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runStatus string
	var runVersion uint64
	var deadlineAt string
	err = tx.QueryRowContext(ctx, `SELECT status, version, deadline_at
		FROM researchflow_runs WHERE run_ref = ? FOR UPDATE`, runRef,
	).Scan(&runStatus, &runVersion, &deadlineAt)
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("lock Researchflow run %q: %w", runRef, err)
	}
	if domain.RunStatus(runStatus).Terminal() || (domain.RunStatus(runStatus) != domain.RunCreated &&
		domain.RunStatus(runStatus) != domain.RunQueued && domain.RunStatus(runStatus) != domain.RunRunning) {
		return domain.Attempt{}, false, ErrClaimRunUnavailable
	}

	var stage domain.Stage
	var stageStatus string
	var retryAt, startedAt sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT stage_ref, run_ref, status, version, attempt_no, retry_at, started_at
		FROM researchflow_stages WHERE stage_ref = ? AND run_ref = ? FOR UPDATE`,
		stageRef, runRef,
	).Scan(&stage.Ref, &stage.RunRef, &stageStatus, &stage.Version, &stage.AttemptNo, &retryAt, &startedAt)
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("lock Researchflow stage %q: %w", stageRef, err)
	}

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return domain.Attempt{}, false, err
	}
	deadline, err := parseResearchflowTime(deadlineAt)
	if err != nil {
		return domain.Attempt{}, false, err
	}
	if !deadline.After(now) {
		return domain.Attempt{}, false, ErrClaimRunUnavailable
	}

	// Lock and inspect the most recent attempt only after Run and Stage locks.
	var current domain.Attempt
	var currentStatus string
	var currentOwner, currentLease, currentStarted sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT attempt_ref, attempt_no, status, lease_owner,
		lease_expires_at, fencing_token, started_at
		FROM researchflow_attempts WHERE stage_ref = ?
		ORDER BY attempt_no DESC LIMIT 1 FOR UPDATE`, stageRef,
	).Scan(&current.Ref, &current.Number, &currentStatus, &currentOwner,
		&currentLease, &current.FencingToken, &currentStarted)
	hasAttempt := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Attempt{}, false, fmt.Errorf("lock latest Researchflow attempt for stage %q: %w", stageRef, err)
	}

	stageState := domain.StageStatus(stageStatus)
	if hasAttempt {
		attemptState := domain.AttemptStatus(currentStatus)
		if current.Number != stage.AttemptNo {
			return domain.Attempt{}, false, errors.New("Researchflow stage attempt number does not match latest attempt")
		}
		if attemptState == domain.AttemptRunning {
			if stageState != domain.StageRunning {
				return domain.Attempt{}, false, ErrClaimStageUnavailable
			}
			expiresAt, parseErr := parseNullableResearchflowTime(currentLease)
			if parseErr != nil {
				return domain.Attempt{}, false, parseErr
			}
			if expiresAt != nil && expiresAt.After(now) {
				if currentOwner.Valid && currentOwner.String == workerID {
					attempt, loadErr := r.getAttemptTx(ctx, tx, current.Ref)
					if loadErr != nil {
						return domain.Attempt{}, false, loadErr
					}
					if err := tx.Commit(); err != nil {
						return domain.Attempt{}, false, fmt.Errorf("commit idempotent Researchflow claim: %w", err)
					}
					return attempt, true, nil
				}
				return domain.Attempt{}, false, ErrAttemptLeaseHeld
			}
			if current.FencingToken == ^uint64(0) {
				return domain.Attempt{}, false, errors.New("Researchflow fencing token exhausted while reclaiming expired attempt")
			}
			result, updateErr := tx.ExecContext(ctx, `UPDATE researchflow_attempts SET
				status = ?, finished_at = ?, fencing_token = fencing_token + 1,
				lease_owner = NULL, lease_expires_at = NULL
				WHERE attempt_ref = ? AND status = ? AND fencing_token = ?`,
				string(domain.AttemptTimedOut), formatResearchflowTimeValue(now),
				current.Ref, string(domain.AttemptRunning), current.FencingToken)
			if updateErr != nil {
				return domain.Attempt{}, false, fmt.Errorf("expire previous Researchflow attempt %q: %w", current.Ref, updateErr)
			}
			if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
				if affectedErr != nil {
					return domain.Attempt{}, false, fmt.Errorf("verify expired Researchflow attempt update: %w", affectedErr)
				}
				return domain.Attempt{}, false, errors.New("Researchflow attempt changed while locked")
			}
		} else if stageState == domain.StageRunning {
			return domain.Attempt{}, false, ErrClaimStageUnavailable
		}
	} else if stageState == domain.StageRunning {
		return domain.Attempt{}, false, ErrClaimStageUnavailable
	}

	if stageState != domain.StageReady && stageState != domain.StageRetryWait &&
		stageState != domain.StageRunning {
		return domain.Attempt{}, false, ErrClaimStageUnavailable
	}
	if stageState == domain.StageReady {
		// ready is only the initial, never-attempted state. Retry promotion is
		// persisted atomically as retry_wait -> running with a new Attempt.
		if hasAttempt || stage.AttemptNo != 0 {
			return domain.Attempt{}, false, ErrClaimStageUnavailable
		}
	}
	if stageState == domain.StageRetryWait {
		// A retry is only legal on an active Run and must follow a failed or
		// timed-out Attempt. Missing retry_at/history is corrupt state, not due work.
		if domain.RunStatus(runStatus) != domain.RunRunning || !retryAt.Valid ||
			!hasAttempt || (domain.AttemptStatus(currentStatus) != domain.AttemptFailed &&
			domain.AttemptStatus(currentStatus) != domain.AttemptTimedOut) {
			return domain.Attempt{}, false, ErrClaimStageUnavailable
		}
		retryTime, parseErr := parseNullableResearchflowTime(retryAt)
		if parseErr != nil {
			return domain.Attempt{}, false, parseErr
		}
		if retryTime == nil || retryTime.After(now) {
			return domain.Attempt{}, false, ErrClaimStageUnavailable
		}
	}
	if stage.AttemptNo >= maxAttempts {
		return domain.Attempt{}, false, ErrAttemptLimitReached
	}

	// Run is locked above, so every ClaimAttempt for this Run serializes before
	// reading the active Stage count. This is the cross-worker concurrency gate.
	var invalidRunningStages, activeLeases int
	// An expired lease is a recoverable running Stage, not corruption and not
	// an active concurrency slot. A running Stage without a matching running
	// Attempt is inconsistent and must fail closed.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_stages s
		WHERE s.run_ref = ? AND s.status = ? AND s.stage_ref <> ?
			AND NOT EXISTS (
				SELECT 1 FROM researchflow_attempts a
				WHERE a.stage_ref = s.stage_ref AND a.run_ref = s.run_ref
					AND a.status = ? AND a.attempt_no = s.attempt_no
					AND a.lease_expires_at IS NOT NULL
			)`,
		runRef, string(domain.StageRunning), stageRef, string(domain.AttemptRunning),
	).Scan(&invalidRunningStages); err != nil {
		return domain.Attempt{}, false, fmt.Errorf("validate running Researchflow stages: %w", err)
	}
	if invalidRunningStages != 0 {
		return domain.Attempt{}, false, errors.New("Researchflow running Stage has no matching running Attempt")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_stages s
		WHERE s.run_ref = ? AND s.status = ? AND s.stage_ref <> ?
			AND EXISTS (
				SELECT 1 FROM researchflow_attempts a
				WHERE a.stage_ref = s.stage_ref AND a.run_ref = s.run_ref
					AND a.status = ? AND a.lease_expires_at > ?
					AND a.attempt_no = s.attempt_no
			)`,
		runRef, string(domain.StageRunning), stageRef, string(domain.AttemptRunning),
		formatResearchflowTimeValue(now)).Scan(&activeLeases); err != nil {
		return domain.Attempt{}, false, fmt.Errorf("count active Researchflow Attempt leases: %w", err)
	}
	if activeLeases >= maxParallel {
		return domain.Attempt{}, false, ErrRunConcurrencyLimit
	}

	var maxToken uint64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(fencing_token), 0)
		FROM researchflow_attempts WHERE stage_ref = ?`, stageRef).Scan(&maxToken); err != nil {
		return domain.Attempt{}, false, fmt.Errorf("read Researchflow fencing token: %w", err)
	}
	if maxToken == ^uint64(0) || stage.Version == ^uint64(0) || runVersion == ^uint64(0) {
		return domain.Attempt{}, false, errors.New("Researchflow version or fencing token exhausted")
	}
	token := maxToken + 1
	attemptNo := stage.AttemptNo + 1
	expiresAt := now.Add(leaseDuration)
	_, err = tx.ExecContext(ctx, `INSERT INTO researchflow_attempts (
		attempt_ref, run_ref, stage_ref, attempt_no, status, lease_owner,
		lease_expires_at, fencing_token, started_at, finished_at, fault_ref, output_ref
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL)`,
		attemptRef, runRef, stageRef, attemptNo, string(domain.AttemptRunning),
		workerID, formatResearchflowTimeValue(expiresAt), token, formatResearchflowTimeValue(now))
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("insert Researchflow attempt: %w", err)
	}

	var stageStarted any
	if startedAt.Valid {
		stageStarted = startedAt.String
	} else {
		stageStarted = formatResearchflowTimeValue(now)
	}
	result, err := tx.ExecContext(ctx, `UPDATE researchflow_stages SET
		status = ?, version = version + 1, attempt_no = ?, retry_at = NULL,
		started_at = ?, finished_at = NULL
		WHERE stage_ref = ? AND run_ref = ? AND version = ?`,
		string(domain.StageRunning), attemptNo, stageStarted, stageRef, runRef, stage.Version)
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("update claimed Researchflow stage: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		if err != nil {
			return domain.Attempt{}, false, fmt.Errorf("verify claimed Researchflow stage update: %w", err)
		}
		return domain.Attempt{}, false, errors.New("Researchflow stage changed while locked")
	}

	if domain.RunStatus(runStatus) != domain.RunRunning {
		result, updateErr := tx.ExecContext(ctx, `UPDATE researchflow_runs SET
			status = ?, version = version + 1, started_at = COALESCE(started_at, ?)
			WHERE run_ref = ? AND version = ?`,
			string(domain.RunRunning), formatResearchflowTimeValue(now), runRef, runVersion)
		if updateErr != nil {
			return domain.Attempt{}, false, fmt.Errorf("mark Researchflow run running: %w", updateErr)
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
			if affectedErr != nil {
				return domain.Attempt{}, false, fmt.Errorf("verify Researchflow run transition: %w", affectedErr)
			}
			return domain.Attempt{}, false, errors.New("Researchflow run changed while locked")
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Attempt{}, false, fmt.Errorf("commit Researchflow attempt claim: %w", err)
	}
	return domain.Attempt{
		Ref: attemptRef, StageRef: stageRef, Number: attemptNo,
		Status: domain.AttemptRunning, LeaseOwner: workerID,
		LeaseExpiresAt: &expiresAt, FencingToken: token, StartedAt: now,
	}, false, nil
}

func (r *MySQLRunRepository) getAttemptTx(ctx context.Context, tx *sql.Tx, attemptRef string) (domain.Attempt, error) {
	var attempt domain.Attempt
	var status string
	var owner, expires, finished, fault, output sql.NullString
	var started string
	err := tx.QueryRowContext(ctx, `SELECT attempt_ref, stage_ref, attempt_no, status,
		lease_owner, lease_expires_at, fencing_token, started_at, finished_at, fault_ref, output_ref
		FROM researchflow_attempts WHERE attempt_ref = ?`, attemptRef,
	).Scan(&attempt.Ref, &attempt.StageRef, &attempt.Number, &status, &owner,
		&expires, &attempt.FencingToken, &started, &finished, &fault, &output)
	if err != nil {
		return domain.Attempt{}, fmt.Errorf("load claimed Researchflow attempt %q: %w", attemptRef, err)
	}
	attempt.Status = domain.AttemptStatus(status)
	if owner.Valid {
		attempt.LeaseOwner = owner.String
	}
	var parseErr error
	if attempt.LeaseExpiresAt, parseErr = parseNullableResearchflowTime(expires); parseErr != nil {
		return domain.Attempt{}, parseErr
	}
	if attempt.StartedAt, parseErr = parseResearchflowTime(started); parseErr != nil {
		return domain.Attempt{}, parseErr
	}
	if attempt.FinishedAt, parseErr = parseNullableResearchflowTime(finished); parseErr != nil {
		return domain.Attempt{}, parseErr
	}
	attempt.FaultRef = stringPointer(fault)
	attempt.OutputRef = stringPointer(output)
	return attempt, nil
}

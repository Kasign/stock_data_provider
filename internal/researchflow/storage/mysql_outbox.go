package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OutboxMessage is an immutable event payload plus the current delivery attempt
// metadata. LeaseGeneration fences workers whose lease has been superseded.
type OutboxMessage struct {
	Ref               string
	RunRef             string
	EventType          string
	SchemaVersion      string
	IdempotencyKey     string
	CorrelationID      string
	PayloadFingerprint string
	Payload            string
	CreatedAt          time.Time
	AttemptCount       uint32
	LeaseGeneration    uint64
}

// ClaimOutbox atomically leases due unpublished messages to one worker.
// MariaDB 10.6+ SKIP LOCKED allows independent workers to claim disjoint rows.
func (r *MySQLRunRepository) ClaimOutbox(
	ctx context.Context,
	workerID, runRef string,
	limit int,
	leaseDuration time.Duration,
) ([]OutboxMessage, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(workerID) == "" || len(workerID) > 128 || len(runRef) > 255 ||
		limit <= 0 || limit > 500 || leaseDuration <= 0 {
		return nil, errors.New("Researchflow Outbox claim arguments are invalid")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin Researchflow Outbox claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return nil, err
	}
	nowValue := formatResearchflowTimeValue(now)
	query := `SELECT outbox_ref, run_ref, event_type,
		schema_version, idempotency_key, correlation_id, payload_fingerprint,
		payload, created_at, attempt_count, lease_generation
		FROM researchflow_outbox
		WHERE published_at IS NULL
			AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
			AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`
	args := []any{nowValue, nowValue}
	if runRef != "" {
		query += ` AND run_ref = ?`
		args = append(args, runRef)
	}
	query += ` ORDER BY created_at, outbox_ref LIMIT ? FOR UPDATE SKIP LOCKED`
	args = append(args, limit)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("lock due Researchflow Outbox messages: %w", err)
	}
	messages := make([]OutboxMessage, 0, limit)
	for rows.Next() {
		var item OutboxMessage
		var createdAt string
		if err := rows.Scan(&item.Ref, &item.RunRef, &item.EventType,
			&item.SchemaVersion, &item.IdempotencyKey, &item.CorrelationID,
			&item.PayloadFingerprint, &item.Payload, &createdAt,
			&item.AttemptCount, &item.LeaseGeneration); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan Researchflow Outbox message: %w", err)
		}
		item.CreatedAt, err = parseResearchflowTime(createdAt)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("parse Researchflow Outbox created_at: %w", err)
		}
		messages = append(messages, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate Researchflow Outbox messages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close Researchflow Outbox messages: %w", err)
	}

	leaseExpiry := formatResearchflowTimeValue(now.Add(leaseDuration))
	for i := range messages {
		if messages[i].LeaseGeneration == ^uint64(0) {
			return nil, fmt.Errorf("Researchflow Outbox %q lease generation exhausted", messages[i].Ref)
		}
		result, err := tx.ExecContext(ctx, `UPDATE researchflow_outbox SET
			lease_owner = ?, lease_expires_at = ?, lease_generation = lease_generation + 1,
			attempt_count = attempt_count + 1
			WHERE outbox_ref = ? AND published_at IS NULL
				AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`,
			workerID, leaseExpiry, messages[i].Ref, nowValue)
		if err != nil {
			return nil, fmt.Errorf("lease Researchflow Outbox %q: %w", messages[i].Ref, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("verify Researchflow Outbox lease: %w", err)
		}
		if affected != 1 {
			return nil, fmt.Errorf("Researchflow Outbox %q changed while locked", messages[i].Ref)
		}
		messages[i].LeaseGeneration++
		messages[i].AttemptCount++
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit Researchflow Outbox claim: %w", err)
	}
	return messages, nil
}

// AcknowledgeOutbox marks delivery complete only for the current, unexpired
// lease. A stale worker cannot acknowledge a message reclaimed by another.
func (r *MySQLRunRepository) AcknowledgeOutbox(
	ctx context.Context,
	outboxRef, workerID string,
	leaseGeneration uint64,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(outboxRef) == "" || len(outboxRef) > 255 ||
		strings.TrimSpace(workerID) == "" || len(workerID) > 128 || leaseGeneration == 0 {
		return false, errors.New("Researchflow Outbox acknowledgement arguments are invalid")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE researchflow_outbox SET
		published_at = DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z'),
		lease_owner = NULL, lease_expires_at = NULL,
		next_attempt_at = NULL, last_error_code = NULL
		WHERE outbox_ref = ? AND published_at IS NULL AND lease_owner = ?
			AND lease_generation = ? AND lease_expires_at > DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')`,
		outboxRef, workerID, leaseGeneration)
	if err != nil {
		return false, fmt.Errorf("acknowledge Researchflow Outbox %q: %w", outboxRef, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("verify Researchflow Outbox acknowledgement: %w", err)
	}
	return affected == 1, nil
}

// RetryOutbox releases the current lease and schedules another delivery.
// Backoff is chosen by the caller; this method only persists the schedule.
func (r *MySQLRunRepository) RetryOutbox(
	ctx context.Context,
	outboxRef, workerID string,
	leaseGeneration uint64,
	nextAttemptAt time.Time,
	errorCode string,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("researchflow SQL database is not configured")
	}
	if strings.TrimSpace(outboxRef) == "" || len(outboxRef) > 255 ||
		strings.TrimSpace(workerID) == "" || len(workerID) > 128 ||
		leaseGeneration == 0 || strings.TrimSpace(errorCode) == "" || len(errorCode) > 128 ||
		nextAttemptAt.IsZero() {
		return false, errors.New("Researchflow Outbox retry arguments are invalid")
	}
	nextAttemptAt = nextAttemptAt.UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE researchflow_outbox SET
		lease_owner = NULL, lease_expires_at = NULL, next_attempt_at = ?,
		last_error_code = ?
		WHERE outbox_ref = ? AND published_at IS NULL AND lease_owner = ?
			AND lease_generation = ? AND lease_expires_at > DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')`,
		formatResearchflowTimeValue(nextAttemptAt), errorCode,
		outboxRef, workerID, leaseGeneration)
	if err != nil {
		return false, fmt.Errorf("schedule Researchflow Outbox retry %q: %w", outboxRef, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("verify Researchflow Outbox retry: %w", err)
	}
	return affected == 1, nil
}

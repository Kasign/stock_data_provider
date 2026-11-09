package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// MySQLDeliveryAttemptRepository 持久化追加式 Attempt 历史。
// 它复用 Server 注入的连接池，不管理连接池生命周期。
type MySQLDeliveryAttemptRepository struct {
	db *sql.DB
}

var _ application.DeliveryAttemptRepository = (*MySQLDeliveryAttemptRepository)(nil)

func NewMySQLDeliveryAttemptRepository(db *sql.DB) (*MySQLDeliveryAttemptRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLDeliveryAttemptRepository{db: db}, nil
}

type lockedAttemptDelivery struct {
	version      int64
	status       domain.DeliveryStatus
	attemptCount int64
	maxAttempts  int64
	workerID     sql.NullString
	leaseExpiry  []byte
	fencingToken int64
	deadlineAt   []byte
	updatedAt    []byte
}

func lockAttemptDelivery(ctx context.Context, tx *sql.Tx, deliveryID string) (lockedAttemptDelivery, error) {
	var current lockedAttemptDelivery
	err := tx.QueryRowContext(ctx, `SELECT version, status, attempt_count, max_attempts,
		lease_worker_id, DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		fencing_token, DATE_FORMAT(deadline_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f')
		FROM notification_deliveries WHERE delivery_id = ? FOR UPDATE`, deliveryID).Scan(
		&current.version, &current.status, &current.attemptCount, &current.maxAttempts,
		&current.workerID, &current.leaseExpiry, &current.fencingToken,
		&current.deadlineAt, &current.updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return lockedAttemptDelivery{}, application.ErrDeliveryNotFound
	}
	if err != nil {
		return lockedAttemptDelivery{}, fmt.Errorf("notification storage: lock delivery for attempt: %w", err)
	}
	return current, nil
}

func normalizeAttemptDatabaseTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func nullableAttemptDatabaseTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return normalizeAttemptDatabaseTime(*value)
}

func attemptDatabaseNow(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var value []byte
	if err := tx.QueryRowContext(ctx, `SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%d %H:%i:%s.%f')`).Scan(&value); err != nil {
		return time.Time{}, fmt.Errorf("notification storage: read database UTC clock: %w", err)
	}
	return parseDatabaseTime(value)
}

func validateAttemptLease(
	current lockedAttemptDelivery,
	now time.Time,
	workerID string,
	fencingToken int64,
	leaseExpiresAt time.Time,
) (time.Time, error) {
	if !current.workerID.Valid || current.workerID.String != workerID ||
		current.fencingToken != fencingToken || len(current.leaseExpiry) == 0 {
		return time.Time{}, application.ErrDeliveryLeaseConflict
	}
	expiry, err := parseDatabaseTime(current.leaseExpiry)
	if err != nil {
		return time.Time{}, err
	}
	if !expiry.Equal(normalizeAttemptDatabaseTime(leaseExpiresAt)) || !now.Before(expiry) {
		return time.Time{}, application.ErrDeliveryLeaseConflict
	}
	return expiry, nil
}

func (r *MySQLDeliveryAttemptRepository) StartWithLease(
	ctx context.Context,
	expectedDeliveryVersion, fencingToken int64,
	attempt domain.DeliveryAttempt,
) error {
	if r == nil || r.db == nil || ctx == nil {
		return errors.New("notification storage: repository is not configured")
	}
	if err := attempt.Validate(); err != nil || attempt.FinishedAt != nil {
		return domain.ErrInvalidDeliveryAttempt
	}
	if expectedDeliveryVersion < 1 || fencingToken < 1 || attempt.FencingToken != fencingToken {
		return application.ErrDeliveryLeaseConflict
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("notification storage: begin attempt start: %w", err)
	}
	defer tx.Rollback()

	current, err := lockAttemptDelivery(ctx, tx, attempt.DeliveryID)
	if err != nil {
		return err
	}
	now, err := attemptDatabaseNow(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := validateAttemptLease(current, now, attempt.WorkerID, fencingToken, attempt.LeaseExpiresAt); err != nil {
		return err
	}
	if current.version != expectedDeliveryVersion {
		return application.ErrDeliveryVersionConflict
	}
	if current.status != domain.DeliverySending || current.attemptCount >= current.maxAttempts ||
		int64(attempt.AttemptNo) != current.attemptCount+1 {
		return application.ErrDeliveryAttemptConflict
	}
	deadline, err := parseDatabaseTime(current.deadlineAt)
	if err != nil {
		return err
	}
	updatedAt, err := parseDatabaseTime(current.updatedAt)
	if err != nil {
		return err
	}
	if attempt.StartedAt.After(now) || attempt.StartedAt.Before(updatedAt) ||
		!attempt.StartedAt.Before(deadline) || !attempt.StartedAt.Before(attempt.LeaseExpiresAt) {
		return domain.ErrInvalidDeliveryAttempt
	}

	// Delivery 行锁串行化同一 Delivery 的 Start；唯一索引仍是跨事务的最终防线。
	var running int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_delivery_attempts
		WHERE delivery_id = ? AND finished_at IS NULL`, attempt.DeliveryID).Scan(&running); err != nil {
		return fmt.Errorf("notification storage: check running attempt: %w", err)
	}
	if running != 0 {
		return application.ErrDeliveryAttemptConflict
	}
	var duplicateID, duplicateNumber, duplicateToken int
	if err := tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM notification_delivery_attempts WHERE attempt_id = ?),
		EXISTS(SELECT 1 FROM notification_delivery_attempts WHERE delivery_id = ? AND attempt_no = ?),
		EXISTS(SELECT 1 FROM notification_delivery_attempts WHERE delivery_id = ? AND fencing_token = ?)`,
		attempt.AttemptID, attempt.DeliveryID, attempt.AttemptNo, attempt.DeliveryID, fencingToken,
	).Scan(&duplicateID, &duplicateNumber, &duplicateToken); err != nil {
		return fmt.Errorf("notification storage: check attempt uniqueness: %w", err)
	}
	if duplicateID != 0 || duplicateNumber != 0 {
		return application.ErrDeliveryAttemptAlreadyExists
	}
	if duplicateToken != 0 {
		return application.ErrDeliveryAttemptConflict
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO notification_delivery_attempts (
		attempt_id, delivery_id, attempt_no, worker_id, fencing_token, lease_expires_at, started_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		attempt.AttemptID, attempt.DeliveryID, attempt.AttemptNo, attempt.WorkerID,
		attempt.FencingToken, normalizeAttemptDatabaseTime(attempt.LeaseExpiresAt), normalizeAttemptDatabaseTime(attempt.StartedAt),
	)
	if err != nil {
		if isMySQLDuplicateKey(err) {
			return application.ErrDeliveryAttemptAlreadyExists
		}
		return fmt.Errorf("notification storage: insert delivery attempt: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE notification_deliveries
		SET attempt_count = ?
		WHERE delivery_id = ? AND version = ? AND status = 'sending'
		AND fencing_token = ? AND lease_worker_id = ?
		AND lease_expires_at > UTC_TIMESTAMP(6) AND deadline_at > UTC_TIMESTAMP(6)`,
		attempt.AttemptNo, attempt.DeliveryID, expectedDeliveryVersion, fencingToken, attempt.WorkerID,
	)
	if err != nil {
		return fmt.Errorf("notification storage: update delivery attempt count: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("notification storage: verify delivery attempt count: %w", err)
	}
	if affected != 1 {
		return application.ErrDeliveryLeaseConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit attempt start: %w", err)
	}
	return nil
}

func (r *MySQLDeliveryAttemptRepository) CompleteWithLease(
	ctx context.Context,
	expectedDeliveryVersion, fencingToken int64,
	completed domain.DeliveryAttempt,
) error {
	if r == nil || r.db == nil || ctx == nil {
		return errors.New("notification storage: repository is not configured")
	}
	if err := completed.Validate(); err != nil || completed.FinishedAt == nil {
		return domain.ErrInvalidDeliveryAttempt
	}
	if expectedDeliveryVersion < 1 || fencingToken < 1 || completed.FencingToken != fencingToken {
		return application.ErrDeliveryLeaseConflict
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("notification storage: begin attempt completion: %w", err)
	}
	defer tx.Rollback()

	current, err := lockAttemptDelivery(ctx, tx, completed.DeliveryID)
	if err != nil {
		return err
	}
	now, err := attemptDatabaseNow(ctx, tx)
	if err != nil {
		return err
	}
	expiry, err := validateAttemptLease(current, now, completed.WorkerID, fencingToken, completed.LeaseExpiresAt)
	if err != nil {
		return err
	}
	if current.version != expectedDeliveryVersion {
		return application.ErrDeliveryVersionConflict
	}
	if current.status != domain.DeliverySending {
		return application.ErrDeliveryAttemptConflict
	}
	if completed.FinishedAt.After(now) || !completed.FinishedAt.Before(expiry) {
		return domain.ErrInvalidDeliveryAttempt
	}

	var existingDeliveryID, existingWorkerID string
	var existingAttemptNo int
	var existingFencingToken int64
	var existingLeaseExpiry, existingStartedAt, finishedAt []byte
	err = tx.QueryRowContext(ctx, `SELECT delivery_id, attempt_no, worker_id, fencing_token,
		DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(started_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(finished_at, '%Y-%m-%d %H:%i:%s.%f')
		FROM notification_delivery_attempts WHERE attempt_id = ? FOR UPDATE`, completed.AttemptID).Scan(
		&existingDeliveryID, &existingAttemptNo, &existingWorkerID, &existingFencingToken,
		&existingLeaseExpiry, &existingStartedAt, &finishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return application.ErrDeliveryAttemptNotFound
	}
	if err != nil {
		return fmt.Errorf("notification storage: lock delivery attempt: %w", err)
	}
	leaseExpiry, err := parseDatabaseTime(existingLeaseExpiry)
	if err != nil {
		return err
	}
	startedAt, err := parseDatabaseTime(existingStartedAt)
	if err != nil {
		return err
	}
	if existingDeliveryID != completed.DeliveryID || existingAttemptNo != completed.AttemptNo ||
		existingWorkerID != completed.WorkerID || existingFencingToken != fencingToken ||
		!leaseExpiry.Equal(normalizeAttemptDatabaseTime(completed.LeaseExpiresAt)) || !startedAt.Equal(normalizeAttemptDatabaseTime(completed.StartedAt)) {
		return application.ErrDeliveryAttemptConflict
	}
	if len(finishedAt) != 0 {
		return application.ErrDeliveryAttemptConflict
	}

	finishedAtForDB := normalizeAttemptDatabaseTime(*completed.FinishedAt)
	startedAtForDB := normalizeAttemptDatabaseTime(completed.StartedAt)
	durationForDB := finishedAtForDB.Sub(startedAtForDB).Milliseconds()
	result, err := tx.ExecContext(ctx, `UPDATE notification_delivery_attempts SET
		finished_at = ?, outcome = ?, provider_message_ref = ?, error_code = ?,
		retryable = ?, retry_after_at = ?, duration_ms = ?, evidence_ref = ?
		WHERE attempt_id = ? AND delivery_id = ? AND fencing_token = ? AND finished_at IS NULL`,
		finishedAtForDB, string(*completed.Outcome), nullableText(completed.ProviderMessageRef),
		nullableText(completed.ErrorCode), completed.Retryable, nullableAttemptDatabaseTime(completed.RetryAfterAt),
		durationForDB, nullableText(completed.EvidenceRef),
		completed.AttemptID, completed.DeliveryID, fencingToken,
	)
	if err != nil {
		return fmt.Errorf("notification storage: complete delivery attempt: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("notification storage: verify attempt completion: %w", err)
	}
	if affected != 1 {
		return application.ErrDeliveryAttemptConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit attempt completion: %w", err)
	}
	return nil
}

func (r *MySQLDeliveryAttemptRepository) ListByDelivery(
	ctx context.Context,
	deliveryID string,
) ([]domain.DeliveryAttempt, error) {
	if r == nil || r.db == nil || ctx == nil {
		return nil, errors.New("notification storage: repository is not configured")
	}
	var exists int
	if err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM notification_deliveries WHERE delivery_id = ?)", deliveryID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("notification storage: check delivery for attempt listing: %w", err)
	}
	if exists == 0 {
		return nil, application.ErrDeliveryNotFound
	}
	rows, err := r.db.QueryContext(ctx, `SELECT attempt_id, delivery_id, attempt_no, worker_id,
		fencing_token, DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(started_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(finished_at, '%Y-%m-%d %H:%i:%s.%f'),
		outcome, provider_message_ref, error_code, retryable,
		DATE_FORMAT(retry_after_at, '%Y-%m-%d %H:%i:%s.%f'),
		duration_ms, evidence_ref
		FROM notification_delivery_attempts WHERE delivery_id = ? ORDER BY attempt_no ASC`, deliveryID)
	if err != nil {
		return nil, fmt.Errorf("notification storage: list delivery attempts: %w", err)
	}
	defer rows.Close()

	attempts := make([]domain.DeliveryAttempt, 0)
	for rows.Next() {
		var attempt domain.DeliveryAttempt
		var leaseExpiry, startedAt, finishedAt, retryAfterAt []byte
		var outcome, providerRef, errorCode, evidenceRef sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(
			&attempt.AttemptID, &attempt.DeliveryID, &attempt.AttemptNo, &attempt.WorkerID,
			&attempt.FencingToken, &leaseExpiry, &startedAt, &finishedAt,
			&outcome, &providerRef, &errorCode, &attempt.Retryable, &retryAfterAt,
			&duration, &evidenceRef,
		); err != nil {
			return nil, fmt.Errorf("notification storage: scan delivery attempt: %w", err)
		}
		if attempt.LeaseExpiresAt, err = parseDatabaseTime(leaseExpiry); err != nil {
			return nil, err
		}
		if attempt.StartedAt, err = parseDatabaseTime(startedAt); err != nil {
			return nil, err
		}
		if len(finishedAt) != 0 {
			value, parseErr := parseDatabaseTime(finishedAt)
			if parseErr != nil {
				return nil, parseErr
			}
			attempt.FinishedAt = &value
		}
		if outcome.Valid {
			value := domain.DeliveryAttemptOutcome(outcome.String)
			attempt.Outcome = &value
		}
		attempt.ProviderMessageRef = nullableString(providerRef)
		attempt.ErrorCode = nullableString(errorCode)
		if len(retryAfterAt) != 0 {
			value, parseErr := parseDatabaseTime(retryAfterAt)
			if parseErr != nil {
				return nil, parseErr
			}
			attempt.RetryAfterAt = &value
		}
		if duration.Valid {
			value := duration.Int64
			attempt.DurationMS = &value
		}
		attempt.EvidenceRef = nullableString(evidenceRef)
		if err := attempt.Validate(); err != nil {
			return nil, fmt.Errorf("notification storage: invalid persisted delivery attempt: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notification storage: iterate delivery attempts: %w", err)
	}
	return attempts, nil
}

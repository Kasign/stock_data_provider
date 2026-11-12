package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// MySQLFanoutWorkItemRepository 持久化单事件展开任务，并复用 Inbox 接收事务。
type MySQLFanoutWorkItemRepository struct {
	db *sql.DB
}

var _ application.FanoutWorkItemRepository = (*MySQLFanoutWorkItemRepository)(nil)
var _ InboxFanoutWriter = (*MySQLFanoutWorkItemRepository)(nil)

func NewMySQLFanoutWorkItemRepository(db *sql.DB) (*MySQLFanoutWorkItemRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLFanoutWorkItemRepository{db: db}, nil
}

// InsertForAcceptedEvent 与 Inbox INSERT 共用事务；重复事件不会走到该方法。
func (r *MySQLFanoutWorkItemRepository) InsertForAcceptedEvent(
	ctx context.Context,
	tx *sql.Tx,
	record domain.InboxRecord,
) error {
	if r == nil || r.db == nil || tx == nil || ctx == nil {
		return errors.New("notification storage: fanout writer is not configured")
	}
	if err := record.Event.VerifyPayloadFingerprint(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_fanout_work_items (
		work_item_id, source_module, event_id, event_type, payload_fingerprint,
		snapshot_revision, cursor_subscription_id, status, attempt_count,
		lease_worker_id, lease_expires_at, fencing_token, next_attempt_at,
		last_error_code, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, NULL, '', 'pending', 0, NULL, NULL, 0, NULL, NULL, ?, ?)`,
		domain.FanoutWorkItemID(record.Event.SourceModule, record.Event.EventID),
		record.Event.SourceModule, record.Event.EventID, record.Event.EventType,
		record.Event.PayloadFingerprint, record.ReceivedAt, record.ReceivedAt,
	)
	if err != nil {
		return fmt.Errorf("notification storage: insert fanout work item: %w", err)
	}
	return nil
}

// ClaimDue 使用数据库时钟和事务行锁领取一个任务；首次 Claim 固定 Subscription 修订快照。
func (r *MySQLFanoutWorkItemRepository) ClaimDue(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (application.FanoutClaim, bool, error) {
	var result application.FanoutClaim
	if r == nil || r.db == nil || ctx == nil || workerID == "" || len(workerID) > 128 ||
		leaseDuration <= 0 || leaseDuration.Microseconds() < 1 {
		return result, false, errors.New("notification storage: invalid fanout claim input")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, false, fmt.Errorf("notification storage: begin fanout claim: %w", err)
	}
	defer tx.Rollback()

	var work domain.FanoutWorkItem
	var snapshotRevision sql.NullInt64
	var leaseWorkerID, lastErrorCode sql.NullString
	var leaseExpiresAt, nextAttemptAt, createdAt, updatedAt []byte
	var event domain.EventEnvelope
	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT
		w.work_item_id, w.source_module, w.event_id, w.event_type, w.payload_fingerprint,
		w.snapshot_revision, w.cursor_subscription_id, w.status, w.attempt_count,
		w.lease_worker_id, DATE_FORMAT(w.lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		w.fencing_token, DATE_FORMAT(w.next_attempt_at, '%Y-%m-%d %H:%i:%s.%f'),
		w.last_error_code, DATE_FORMAT(w.created_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(w.updated_at, '%Y-%m-%d %H:%i:%s.%f'),
		i.source_module, i.event_id, i.idempotency_key, i.event_type, i.schema_version,
		i.subject_ref, i.occurred_at, i.published_at, i.correlation_id,
		i.payload_fingerprint, i.payload
		FROM notification_fanout_work_items w
		INNER JOIN notification_inbox i
			ON i.source_module = w.source_module AND i.event_id = w.event_id
		WHERE (
			(w.status IN ('pending', 'retry_wait') AND (w.next_attempt_at IS NULL OR w.next_attempt_at <= UTC_TIMESTAMP(6)))
			OR (w.status = 'processing' AND w.lease_expires_at <= UTC_TIMESTAMP(6))
		)
		ORDER BY w.created_at, w.work_item_id
		LIMIT 1 FOR UPDATE`).Scan(
		&work.WorkItemID, &work.SourceModule, &work.EventID, &work.EventType, &work.PayloadFingerprint,
		&snapshotRevision, &work.CursorSubscriptionID, &work.Status, &work.AttemptCount,
		&leaseWorkerID, &leaseExpiresAt, &work.FencingToken, &nextAttemptAt,
		&lastErrorCode, &createdAt, &updatedAt,
		&event.SourceModule, &event.EventID, &event.IdempotencyKey, &event.EventType, &event.SchemaVersion,
		&event.SubjectRef, &event.OccurredAt, &event.PublishedAt, &event.CorrelationID,
		&event.PayloadFingerprint, &payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, fmt.Errorf("notification storage: select due fanout work item: %w", err)
	}
	work.LeaseWorkerID = leaseWorkerID.String
	work.LastErrorCode = lastErrorCode.String
	if snapshotRevision.Valid {
		work.SnapshotRevision = snapshotRevision.Int64
		work.HasSnapshot = true
	}
	if len(leaseExpiresAt) > 0 {
		if work.LeaseExpiresAt, err = parseDatabaseTime(leaseExpiresAt); err != nil {
			return result, false, err
		}
	}
	if len(nextAttemptAt) > 0 {
		if work.NextAttemptAt, err = parseDatabaseTime(nextAttemptAt); err != nil {
			return result, false, err
		}
	}
	if work.CreatedAt, err = parseDatabaseTime(createdAt); err != nil {
		return result, false, err
	}
	if work.UpdatedAt, err = parseDatabaseTime(updatedAt); err != nil {
		return result, false, err
	}
	if err := work.Validate(); err != nil {
		return result, false, fmt.Errorf(
			"notification storage: invalid fanout work item id=%q status=%q snapshot=%t revision=%d cursor=%q attempts=%d worker=%q lease_expires=%s fencing=%d next_attempt=%s created=%s updated=%s: %w",
			work.WorkItemID, work.Status, work.HasSnapshot, work.SnapshotRevision,
			work.CursorSubscriptionID, work.AttemptCount, work.LeaseWorkerID,
			work.LeaseExpiresAt.Format(time.RFC3339Nano), work.FencingToken,
			work.NextAttemptAt.Format(time.RFC3339Nano), work.CreatedAt.Format(time.RFC3339Nano),
			work.UpdatedAt.Format(time.RFC3339Nano), err,
		)
	}
	if !snapshotRevision.Valid {
		// 只有从未领取的初始 pending 项允许首次分配快照。
		// retry_wait/过期 processing 丢失快照时必须 fail closed，不能重建为新高水位。
		if work.Status != domain.FanoutPending || work.AttemptCount != 0 ||
			work.FencingToken != 0 || work.CursorSubscriptionID != "" {
			return result, false, domain.ErrInvalidFanoutWorkItem
		}
		if err := tx.QueryRowContext(ctx, `SELECT current_revision
			FROM notification_subscription_revision WHERE singleton_id = 1`).Scan(&work.SnapshotRevision); err != nil {
			return result, false, fmt.Errorf("notification storage: read subscription snapshot revision: %w", err)
		}
		work.HasSnapshot = true
	}
	event.Payload = append([]byte(nil), payload...)
	if err := event.VerifyPayloadFingerprint(); err != nil {
		return result, false, fmt.Errorf("notification storage: inbox event for fanout is invalid: %w", err)
	}
	if event.SourceModule != work.SourceModule || event.EventID != work.EventID ||
		event.EventType != work.EventType || event.PayloadFingerprint != work.PayloadFingerprint {
		return result, false, domain.ErrInvalidFanoutWorkItem
	}

	update, err := tx.ExecContext(ctx, `UPDATE notification_fanout_work_items
		SET status = 'processing', attempt_count = attempt_count + 1,
			next_attempt_at = NULL,
			lease_worker_id = ?, lease_expires_at = TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6)),
			fencing_token = fencing_token + 1, snapshot_revision = ?, updated_at = UTC_TIMESTAMP(6)
		WHERE work_item_id = ?`,
		workerID, leaseDuration.Microseconds(), work.SnapshotRevision, work.WorkItemID,
	)
	if err != nil {
		return result, false, fmt.Errorf("notification storage: update fanout claim: %w", err)
	}
	affected, err := update.RowsAffected()
	if err != nil {
		return result, false, fmt.Errorf("notification storage: read fanout claim affected rows: %w", err)
	}
	if affected != 1 {
		return result, false, errors.New("notification storage: fanout claim did not update exactly one row")
	}
	var expiresAt, changedAt []byte
	if err := tx.QueryRowContext(ctx, `SELECT fencing_token, attempt_count,
		DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f')
		FROM notification_fanout_work_items WHERE work_item_id = ?`, work.WorkItemID,
	).Scan(&work.FencingToken, &work.AttemptCount, &expiresAt, &changedAt); err != nil {
		return result, false, fmt.Errorf("notification storage: read claimed fanout lease: %w", err)
	}
	if work.LeaseExpiresAt, err = parseDatabaseTime(expiresAt); err != nil {
		return result, false, err
	}
	if work.UpdatedAt, err = parseDatabaseTime(changedAt); err != nil {
		return result, false, err
	}
	work.Status = domain.FanoutProcessing
	work.LeaseWorkerID = workerID
	work.NextAttemptAt = time.Time{}
	if err := work.Validate(); err != nil {
		return result, false, err
	}
	if err := tx.Commit(); err != nil {
		return result, false, fmt.Errorf("%w: notification storage: commit fanout claim: %w", application.ErrPersistenceOutcomeUnknown, err)
	}
	result = application.FanoutClaim{WorkItem: work, Event: event}
	return result, true, nil
}

func (r *MySQLFanoutWorkItemRepository) AdvanceWithLease(
	ctx context.Context,
	claim application.FanoutClaim,
	nextCursor string,
	completed bool,
) error {
	if r == nil || r.db == nil || ctx == nil || claim.WorkItem.WorkItemID == "" ||
		claim.WorkItem.LeaseWorkerID == "" || claim.WorkItem.FencingToken < 1 ||
		(nextCursor != "" && nextCursor < claim.WorkItem.CursorSubscriptionID) {
		return application.ErrFanoutWorkItemLeaseConflict
	}
	status := string(domain.FanoutPending)
	if completed {
		status = string(domain.FanoutCompleted)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE notification_fanout_work_items
		SET status = ?, cursor_subscription_id = ?, lease_worker_id = NULL,
			lease_expires_at = NULL, next_attempt_at = NULL, last_error_code = NULL,
			updated_at = UTC_TIMESTAMP(6)
		WHERE work_item_id = ? AND status = 'processing' AND lease_worker_id = ?
			AND fencing_token = ? AND lease_expires_at > UTC_TIMESTAMP(6)`,
		status, nextCursor, claim.WorkItem.WorkItemID, claim.WorkItem.LeaseWorkerID, claim.WorkItem.FencingToken,
	)
	if err != nil {
		return fmt.Errorf("%w: notification storage: advance fanout page: %w", application.ErrPersistenceOutcomeUnknown, err)
	}
	return requireFanoutLeaseUpdate(result)
}

func (r *MySQLFanoutWorkItemRepository) RetryWithLease(
	ctx context.Context,
	claim application.FanoutClaim,
	nextAttemptAt time.Time,
	reasonCode string,
	terminal bool,
) error {
	if r == nil || r.db == nil || ctx == nil || claim.WorkItem.WorkItemID == "" ||
		claim.WorkItem.LeaseWorkerID == "" || claim.WorkItem.FencingToken < 1 ||
		reasonCode == "" || len(reasonCode) > 128 || strings.TrimSpace(reasonCode) != reasonCode {
		return application.ErrFanoutWorkItemLeaseConflict
	}
	status := string(domain.FanoutRetryWait)
	var next any
	if terminal {
		status = string(domain.FanoutFailed)
		next = nil
	} else {
		if nextAttemptAt.IsZero() || nextAttemptAt.Location() != time.UTC {
			return errors.New("notification storage: fanout retry time must be UTC")
		}
		next = nextAttemptAt.Truncate(time.Microsecond)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE notification_fanout_work_items
		SET status = ?, next_attempt_at = ?, last_error_code = ?, lease_worker_id = NULL,
			lease_expires_at = NULL, updated_at = UTC_TIMESTAMP(6)
		WHERE work_item_id = ? AND status = 'processing' AND lease_worker_id = ?
			AND fencing_token = ? AND lease_expires_at > UTC_TIMESTAMP(6)`,
		status, next, reasonCode, claim.WorkItem.WorkItemID, claim.WorkItem.LeaseWorkerID, claim.WorkItem.FencingToken,
	)
	if err != nil {
		return fmt.Errorf("%w: notification storage: schedule fanout retry: %w", application.ErrPersistenceOutcomeUnknown, err)
	}
	return requireFanoutLeaseUpdate(result)
}

func requireFanoutLeaseUpdate(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%w: notification storage: read fanout affected rows: %w", application.ErrPersistenceOutcomeUnknown, err)
	}
	if affected != 1 {
		return application.ErrFanoutWorkItemLeaseConflict
	}
	return nil
}

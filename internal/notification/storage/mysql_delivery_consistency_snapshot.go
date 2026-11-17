package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

type consistencyQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// MySQLDeliveryConsistencySnapshotReader reads all three projections in one InnoDB
// REPEATABLE READ transaction. The first SELECT establishes the consistent-read view.
type MySQLDeliveryConsistencySnapshotReader struct {
	db *sql.DB
}

var _ application.DeliveryConsistencySnapshotReader = (*MySQLDeliveryConsistencySnapshotReader)(nil)

func NewMySQLDeliveryConsistencySnapshotReader(db *sql.DB) (*MySQLDeliveryConsistencySnapshotReader, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLDeliveryConsistencySnapshotReader{db: db}, nil
}

func (r *MySQLDeliveryConsistencySnapshotReader) ReadDeliveryConsistencySnapshot(
	ctx context.Context, deliveryID string,
) (application.DeliveryConsistencySnapshot, bool, error) {
	if r == nil || r.db == nil || ctx == nil || deliveryID == "" {
		return application.DeliveryConsistencySnapshot{}, false, errors.New("notification storage: repository or delivery id is invalid")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return application.DeliveryConsistencySnapshot{}, false, fmt.Errorf("notification storage: begin consistency snapshot: %w", err)
	}
	defer tx.Rollback()

	delivery, err := scanDelivery(tx.QueryRowContext(ctx, deliverySelect+" WHERE delivery_id = ?", deliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return application.DeliveryConsistencySnapshot{}, false, nil
	}
	if err != nil {
		return application.DeliveryConsistencySnapshot{}, false, fmt.Errorf("notification storage: read consistency delivery: %w", err)
	}
	attempts, err := listDeliveryAttempts(ctx, tx, deliveryID)
	if err != nil {
		return application.DeliveryConsistencySnapshot{}, false, err
	}
	events, err := listDeliveryStateEvents(ctx, tx, deliveryID)
	if err != nil {
		return application.DeliveryConsistencySnapshot{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return application.DeliveryConsistencySnapshot{}, false, fmt.Errorf("notification storage: commit consistency snapshot: %w", err)
	}
	return application.DeliveryConsistencySnapshot{Delivery: delivery, Attempts: attempts, StateEvents: events}, true, nil
}

func listDeliveryAttempts(ctx context.Context, q consistencyQueryer, deliveryID string) ([]domain.DeliveryAttempt, error) {
	rows, err := q.QueryContext(ctx, `SELECT attempt_id, delivery_id, attempt_no, worker_id,
		fencing_token, DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(started_at, '%Y-%m-%d %H:%i:%s.%f'),
		DATE_FORMAT(finished_at, '%Y-%m-%d %H:%i:%s.%f'),
		outcome, provider_message_ref, error_code, retryable,
		DATE_FORMAT(retry_after_at, '%Y-%m-%d %H:%i:%s.%f'),
		duration_ms, evidence_ref
		FROM notification_delivery_attempts WHERE delivery_id = ? ORDER BY attempt_no ASC`, deliveryID)
	if err != nil {
		return nil, fmt.Errorf("notification storage: list consistency attempts: %w", err)
	}
	defer rows.Close()
	attempts := make([]domain.DeliveryAttempt, 0)
	for rows.Next() {
		var attempt domain.DeliveryAttempt
		var leaseExpiry, startedAt, finishedAt, retryAfterAt []byte
		var outcome, providerRef, errorCode, evidenceRef sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(&attempt.AttemptID, &attempt.DeliveryID, &attempt.AttemptNo, &attempt.WorkerID,
			&attempt.FencingToken, &leaseExpiry, &startedAt, &finishedAt, &outcome, &providerRef,
			&errorCode, &attempt.Retryable, &retryAfterAt, &duration, &evidenceRef); err != nil {
			return nil, fmt.Errorf("notification storage: scan consistency attempt: %w", err)
		}
		if attempt.LeaseExpiresAt, err = parseDatabaseTime(leaseExpiry); err != nil {
			return nil, err
		}
		if attempt.StartedAt, err = parseDatabaseTime(startedAt); err != nil {
			return nil, err
		}
		if len(finishedAt) > 0 {
			value, e := parseDatabaseTime(finishedAt)
			if e != nil {
				return nil, e
			}
			attempt.FinishedAt = &value
		}
		if outcome.Valid {
			value := domain.DeliveryAttemptOutcome(outcome.String)
			attempt.Outcome = &value
		}
		attempt.ProviderMessageRef, attempt.ErrorCode = nullableString(providerRef), nullableString(errorCode)
		if len(retryAfterAt) > 0 {
			value, e := parseDatabaseTime(retryAfterAt)
			if e != nil {
				return nil, e
			}
			attempt.RetryAfterAt = &value
		}
		if duration.Valid {
			value := duration.Int64
			attempt.DurationMS = &value
		}
		attempt.EvidenceRef = nullableString(evidenceRef)
		if err := attempt.Validate(); err != nil {
			return nil, fmt.Errorf("notification storage: invalid consistency attempt: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notification storage: iterate consistency attempts: %w", err)
	}
	return attempts, nil
}

func listDeliveryStateEvents(ctx context.Context, q consistencyQueryer, deliveryID string) ([]domain.DeliveryStateEvent, error) {
	rows, err := q.QueryContext(ctx, `SELECT delivery_id, version, from_status, to_status,
		DATE_FORMAT(changed_at, '%Y-%m-%d %H:%i:%s.%f'),
		reason_code, evidence_ref, actor_ref, request_id, actor_reason
		FROM notification_delivery_state_events WHERE delivery_id = ? ORDER BY version ASC`, deliveryID)
	if err != nil {
		return nil, fmt.Errorf("notification storage: list consistency state events: %w", err)
	}
	defer rows.Close()
	events := make([]domain.DeliveryStateEvent, 0)
	for rows.Next() {
		var event domain.DeliveryStateEvent
		var fromStatus, toStatus string
		var changedAt []byte
		var reason, evidence, actor, request, actorReason sql.NullString
		if err := rows.Scan(&event.DeliveryID, &event.Version, &fromStatus, &toStatus, &changedAt,
			&reason, &evidence, &actor, &request, &actorReason); err != nil {
			return nil, fmt.Errorf("notification storage: scan consistency state event: %w", err)
		}
		event.From, event.To = domain.DeliveryStatus(fromStatus), domain.DeliveryStatus(toStatus)
		if event.ChangedAt, err = parseDatabaseTime(changedAt); err != nil {
			return nil, err
		}
		event.ReasonCode, event.EvidenceRef = nullableString(reason), nullableString(evidence)
		event.ActorRef, event.RequestID, event.ActorReason = nullableString(actor), nullableString(request), nullableString(actorReason)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notification storage: iterate consistency state events: %w", err)
	}
	return events, nil
}

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

// MySQLDeliveryRepository implements the expired-lease recovery transaction.
var _ application.DeliveryLeaseRecoveryRepository = (*MySQLDeliveryRepository)(nil)

// RecoverExpiredLease persists an already-assessed recovery result.
// The database clock is authoritative: an expired lease, current fencing token,
// expected Delivery version, snapshot and append-only state event are checked
// and committed in one transaction.
func (r *MySQLDeliveryRepository) RecoverExpiredLease(
	ctx context.Context,
	expectedVersion, fencingToken int64,
	next domain.DeliveryRecord,
	event domain.DeliveryStateEvent,
	now time.Time,
) error {
	if r == nil || r.db == nil || ctx == nil || !isUTCTime(now) ||
		expectedVersion < 1 || fencingToken < 1 || next.UpdatedAt.After(now) {
		return application.ErrDeliveryLeaseConflict
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if event.DeliveryID != next.DeliveryID || event.Version != next.Version ||
		event.To != next.Status || !event.ChangedAt.Equal(next.UpdatedAt) {
		return domain.ErrInvalidDeliveryChange
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("notification storage: begin expired lease recovery: %w", err)
	}
	defer tx.Rollback()

	current, err := scanDelivery(tx.QueryRowContext(ctx, deliverySelect+`
		WHERE delivery_id = ? FOR UPDATE`, next.DeliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return application.ErrDeliveryNotFound
	}
	if err != nil {
		return fmt.Errorf("notification storage: lock delivery for recovery: %w", err)
	}
	if current.Status != domain.DeliverySending || current.Version != expectedVersion ||
		next.Version != expectedVersion+1 || event.From != domain.DeliverySending ||
		!sameDeliveryIdentity(current, next) || next.UpdatedAt.Before(current.UpdatedAt) {
		return application.ErrDeliveryVersionConflict
	}

	var workerID sql.NullString
	var leaseExpiry []byte
	var currentToken int64
	if err := tx.QueryRowContext(ctx, `SELECT lease_worker_id,
		DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'), fencing_token
		FROM notification_deliveries WHERE delivery_id = ?`, next.DeliveryID).
		Scan(&workerID, &leaseExpiry, &currentToken); err != nil {
		return fmt.Errorf("notification storage: read expired delivery lease: %w", err)
	}
	if !workerID.Valid || len(leaseExpiry) == 0 || currentToken != fencingToken {
		return application.ErrDeliveryLeaseConflict
	}
	expiresAt, err := parseDatabaseTime(leaseExpiry)
	if err != nil {
		return err
	}
	if now.Before(expiresAt) {
		return application.ErrDeliveryLeaseConflict
	}

	result, err := tx.ExecContext(ctx, `UPDATE notification_deliveries SET
		status = ?, version = ?, attempt_count = ?, max_attempts = ?, next_attempt_at = ?,
		deadline_at = ?, reason_code = ?, last_error_code = ?, provider_message_ref = ?,
		updated_at = ?, lease_worker_id = NULL, lease_expires_at = NULL
		WHERE delivery_id = ? AND status = ? AND version = ? AND fencing_token = ?
		AND lease_expires_at <= UTC_TIMESTAMP(6)`,
		string(next.Status), next.Version, next.AttemptCount, next.MaxAttempts,
		nullableTime(next.NextAttemptAt), next.DeadlineAt.UTC(), nullableText(next.ReasonCode),
		nullableText(next.LastErrorCode), nullableText(next.ProviderMessageRef), next.UpdatedAt.UTC(),
		next.DeliveryID, string(domain.DeliverySending), expectedVersion, fencingToken)
	if err != nil {
		return fmt.Errorf("notification storage: update delivery after expired lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("notification storage: verify expired lease recovery: %w", err)
	}
	if affected != 1 {
		return application.ErrDeliveryLeaseConflict
	}
	if err := insertDeliveryStateEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit expired lease recovery: %w", err)
	}
	return nil
}

// sameDeliveryIdentity prevents recovery from changing the immutable plan snapshot.
func sameDeliveryIdentity(left, right domain.DeliveryRecord) bool {
	return left.DeliveryID == right.DeliveryID &&
		left.SourceModule == right.SourceModule &&
		left.EventID == right.EventID &&
		left.SubscriptionID == right.SubscriptionID &&
		left.SubscriptionVersion == right.SubscriptionVersion &&
		left.ChannelRef == right.ChannelRef &&
		left.ChannelConfigVersion == right.ChannelConfigVersion &&
		left.TemplateID == right.TemplateID &&
		left.TemplateVersion == right.TemplateVersion &&
		left.RecipientRef == right.RecipientRef &&
		left.PayloadFingerprint == right.PayloadFingerprint &&
		left.PlanFingerprint == right.PlanFingerprint &&
		left.DeliveryIdempotencyKey == right.DeliveryIdempotencyKey
}

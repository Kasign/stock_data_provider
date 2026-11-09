package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// ReconcileUnknown 将已完成证据核验的 unknown 结果以独立 CAS 原子落库。
// 对账不持有原 Worker 租约，因此不复用 UpdateWithLease。
func (r *MySQLDeliveryRepository) ReconcileUnknown(
	ctx context.Context,
	expectedVersion int64,
	next domain.DeliveryRecord,
	event domain.DeliveryStateEvent,
) error {
	if r == nil || r.db == nil || ctx == nil || expectedVersion < 1 {
		return errors.New("notification storage: invalid unknown reconciliation request")
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Version != expectedVersion+1 ||
		(next.Status != domain.DeliverySent && next.Status != domain.DeliveryFailed && next.Status != domain.DeliveryRetryWait) ||
		event.DeliveryID != next.DeliveryID || event.Version != next.Version ||
		event.From != domain.DeliveryUnknown || event.To != next.Status ||
		event.EvidenceRef == nil || !validUnknownEvidenceRef(*event.EvidenceRef) ||
		!event.ChangedAt.Equal(next.UpdatedAt) {
		return domain.ErrInvalidDeliveryChange
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("notification storage: begin unknown reconciliation: %w", err)
	}
	defer tx.Rollback()

	current, err := scanDelivery(tx.QueryRowContext(ctx, deliverySelect+
		" WHERE delivery_id = ? FOR UPDATE", next.DeliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return application.ErrDeliveryNotFound
	}
	if err != nil {
		return fmt.Errorf("notification storage: lock unknown delivery: %w", err)
	}
	if current.Status != domain.DeliveryUnknown || current.Version != expectedVersion ||
		!sameDeliveryIdentity(current, next) ||
		current.AttemptCount != next.AttemptCount ||
		current.MaxAttempts != next.MaxAttempts ||
		!current.DeadlineAt.Equal(next.DeadlineAt) ||
		!current.CreatedAt.Equal(next.CreatedAt) ||
		next.UpdatedAt.Before(current.UpdatedAt) {
		return application.ErrUnknownReconciliationConflict
	}

	result, err := tx.ExecContext(ctx, `UPDATE notification_deliveries SET
		status = ?, version = ?, attempt_count = ?, max_attempts = ?, next_attempt_at = ?,
		deadline_at = ?, reason_code = ?, last_error_code = ?, provider_message_ref = ?,
		updated_at = ?, lease_worker_id = NULL, lease_expires_at = NULL
		WHERE delivery_id = ? AND status = ? AND version = ?`,
		string(next.Status), next.Version, next.AttemptCount, next.MaxAttempts,
		nullableTime(next.NextAttemptAt), next.DeadlineAt.UTC(), nullableText(next.ReasonCode),
		nullableText(next.LastErrorCode), nullableText(next.ProviderMessageRef), next.UpdatedAt.UTC(),
		next.DeliveryID, string(domain.DeliveryUnknown), expectedVersion)
	if err != nil {
		return fmt.Errorf("notification storage: update reconciled delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("notification storage: verify unknown reconciliation CAS: %w", err)
	}
	if affected != 1 {
		return application.ErrUnknownReconciliationConflict
	}
	if err := insertDeliveryStateEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit unknown reconciliation: %w", err)
	}
	return nil
}

func validUnknownEvidenceRef(value string) bool {
	if value == "" || len(value) > 256 || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			character != '-' && character != '_' && character != '.' && character != ':' {
			return false
		}
	}
	return true
}

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

type MySQLDeliveryRepository struct {
	db *sql.DB
}

var _ application.DeliveryRepository = (*MySQLDeliveryRepository)(nil)

func NewMySQLDeliveryRepository(db *sql.DB) (*MySQLDeliveryRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLDeliveryRepository{db: db}, nil
}

const deliverySelect = `SELECT
	delivery_id, source_module, event_id, subscription_id,
	subscription_version, channel_ref, channel_config_version, template_id,
	template_version, recipient_ref, payload_fingerprint, plan_fingerprint,
	delivery_idempotency_key, status, version, attempt_count, max_attempts,
	DATE_FORMAT(next_attempt_at, '%Y-%m-%d %H:%i:%s.%f'),
	DATE_FORMAT(deadline_at, '%Y-%m-%d %H:%i:%s.%f'),
	reason_code, last_error_code, provider_message_ref,
	DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s.%f'),
	DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f')
	FROM notification_deliveries`

type deliveryScanner interface {
	Scan(dest ...any) error
}

func scanDelivery(row deliveryScanner) (domain.DeliveryRecord, error) {
	var record domain.DeliveryRecord
	var status string
	var nextAttempt, deadline, created, updated []byte
	var reason, lastError, providerMessage sql.NullString
	err := row.Scan(
		&record.DeliveryID, &record.SourceModule, &record.EventID, &record.SubscriptionID,
		&record.SubscriptionVersion, &record.ChannelRef, &record.ChannelConfigVersion,
		&record.TemplateID, &record.TemplateVersion, &record.RecipientRef,
		&record.PayloadFingerprint, &record.PlanFingerprint, &record.DeliveryIdempotencyKey,
		&status, &record.Version, &record.AttemptCount, &record.MaxAttempts,
		&nextAttempt, &deadline, &reason, &lastError, &providerMessage, &created, &updated,
	)
	if err != nil {
		return domain.DeliveryRecord{}, err
	}
	record.Status = domain.DeliveryStatus(status)
	if len(nextAttempt) > 0 {
		value, err := parseDatabaseTime(nextAttempt)
		if err != nil {
			return domain.DeliveryRecord{}, err
		}
		record.NextAttemptAt = &value
	}
	if record.DeadlineAt, err = parseDatabaseTime(deadline); err != nil {
		return domain.DeliveryRecord{}, err
	}
	if record.CreatedAt, err = parseDatabaseTime(created); err != nil {
		return domain.DeliveryRecord{}, err
	}
	if record.UpdatedAt, err = parseDatabaseTime(updated); err != nil {
		return domain.DeliveryRecord{}, err
	}
	record.ReasonCode = nullableString(reason)
	record.LastErrorCode = nullableString(lastError)
	record.ProviderMessageRef = nullableString(providerMessage)
	if err := record.Validate(); err != nil {
		return domain.DeliveryRecord{}, fmt.Errorf("notification storage: invalid persisted delivery: %w", err)
	}
	return record, nil
}

func parseDatabaseTime(value []byte) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05.000000", string(value), time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("notification storage: parse UTC database time: %w", err)
	}
	return parsed.UTC(), nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func nullableText(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func (r *MySQLDeliveryRepository) Create(ctx context.Context, delivery domain.DeliveryRecord) error {
	if r == nil || r.db == nil || ctx == nil {
		return errors.New("notification storage: repository is not configured")
	}
	if err := delivery.Validate(); err != nil {
		return err
	}
	if delivery.Status != domain.DeliveryQueued || delivery.Version != 1 ||
		delivery.AttemptCount != 0 || delivery.NextAttemptAt != nil {
		return domain.ErrInvalidDelivery
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("notification storage: begin delivery create: %w", err)
	}
	defer tx.Rollback()

	var matchingPlans int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_delivery_plans
		WHERE delivery_id = ? AND source_module = ? AND event_id = ?
		AND subscription_id = ? AND subscription_version = ?
		AND channel_ref = ? AND channel_config_version = ?
		AND template_id = ? AND template_version = ? AND recipient_ref = ?
		AND payload_fingerprint = ? AND plan_fingerprint = ?
		AND delivery_idempotency_key = ?`,
		delivery.DeliveryID, delivery.SourceModule, delivery.EventID,
		delivery.SubscriptionID, delivery.SubscriptionVersion,
		delivery.ChannelRef, delivery.ChannelConfigVersion,
		delivery.TemplateID, delivery.TemplateVersion, delivery.RecipientRef,
		delivery.PayloadFingerprint, delivery.PlanFingerprint, delivery.DeliveryIdempotencyKey,
	).Scan(&matchingPlans); err != nil {
		return fmt.Errorf("notification storage: verify immutable delivery plan: %w", err)
	}
	if matchingPlans != 1 {
		return errors.New("notification storage: delivery snapshot does not match its immutable plan")
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO notification_deliveries (
		delivery_id, source_module, event_id, subscription_id, subscription_version,
		channel_ref, channel_config_version, template_id, template_version, recipient_ref,
		payload_fingerprint, plan_fingerprint, delivery_idempotency_key, status, version,
		attempt_count, max_attempts, next_attempt_at, deadline_at, reason_code,
		last_error_code, provider_message_ref, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.DeliveryID, delivery.SourceModule, delivery.EventID, delivery.SubscriptionID,
		delivery.SubscriptionVersion, delivery.ChannelRef, delivery.ChannelConfigVersion,
		delivery.TemplateID, delivery.TemplateVersion, delivery.RecipientRef,
		delivery.PayloadFingerprint, delivery.PlanFingerprint, delivery.DeliveryIdempotencyKey,
		string(delivery.Status), delivery.Version, delivery.AttemptCount, delivery.MaxAttempts,
		nullableTime(delivery.NextAttemptAt), delivery.DeadlineAt.UTC(), nullableText(delivery.ReasonCode),
		nullableText(delivery.LastErrorCode), nullableText(delivery.ProviderMessageRef),
		delivery.CreatedAt.UTC(), delivery.UpdatedAt.UTC(),
	)
	if isMySQLDuplicateKey(err) {
		return application.ErrDeliveryAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("notification storage: create delivery: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit delivery create: %w", err)
	}
	return nil
}

func (r *MySQLDeliveryRepository) Get(ctx context.Context, deliveryID string) (domain.DeliveryRecord, bool, error) {
	if r == nil || r.db == nil || ctx == nil || deliveryID == "" {
		return domain.DeliveryRecord{}, false, errors.New("notification storage: repository or delivery id is invalid")
	}
	record, err := scanDelivery(r.db.QueryRowContext(ctx, deliverySelect+" WHERE delivery_id = ?", deliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeliveryRecord{}, false, nil
	}
	if err != nil {
		return domain.DeliveryRecord{}, false, fmt.Errorf("notification storage: get delivery: %w", err)
	}
	return record, true, nil
}

func (r *MySQLDeliveryRepository) ClaimDue(
	ctx context.Context,
	workerID string,
	now, leaseExpiresAt time.Time,
) (application.DeliveryClaim, bool, error) {
	if r == nil || r.db == nil || ctx == nil || !validDeliveryWorkerID(workerID) ||
		!isUTCTime(now) || !isUTCTime(leaseExpiresAt) || !leaseExpiresAt.After(now) {
		return application.DeliveryClaim{}, false, domain.ErrInvalidDeliveryChange
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: begin claim: %w", err)
	}
	defer tx.Rollback()

	record, err := scanDelivery(tx.QueryRowContext(ctx, deliverySelect+`
		WHERE status IN (?, ?)
		AND deadline_at > ?
		AND attempt_count < max_attempts
		AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		ORDER BY created_at, delivery_id
		LIMIT 1 FOR UPDATE`, string(domain.DeliveryQueued), string(domain.DeliveryRetryWait), now.UTC(), now.UTC()))
	if errors.Is(err, sql.ErrNoRows) {
		return application.DeliveryClaim{}, false, nil
	}
	if err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: select due delivery: %w", err)
	}
	var currentToken int64
	if err := tx.QueryRowContext(ctx, `SELECT fencing_token FROM notification_deliveries WHERE delivery_id = ?`, record.DeliveryID).Scan(&currentToken); err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: read fencing token: %w", err)
	}
	if currentToken == math.MaxInt64 {
		return application.DeliveryClaim{}, false, application.ErrDeliveryLeaseConflict
	}
	effectiveExpiry := leaseExpiresAt.UTC()
	if effectiveExpiry.After(record.DeadlineAt) {
		effectiveExpiry = record.DeadlineAt
	}
	if !effectiveExpiry.After(now) {
		return application.DeliveryClaim{}, false, nil
	}
	next, event, err := record.Transition(domain.DeliveryTransition{To: domain.DeliverySending, At: now.UTC()})
	if err != nil {
		return application.DeliveryClaim{}, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE notification_deliveries
		SET status = ?, version = ?, updated_at = ?, lease_worker_id = ?,
		lease_expires_at = ?, fencing_token = fencing_token + 1
		WHERE delivery_id = ? AND version = ? AND fencing_token = ?`,
		string(next.Status), next.Version, next.UpdatedAt.UTC(), workerID, effectiveExpiry,
		next.DeliveryID, record.Version, currentToken)
	if err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: claim delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: verify delivery claim: %w", err)
	}
	if affected != 1 {
		return application.DeliveryClaim{}, false, application.ErrDeliveryVersionConflict
	}
	if err := insertDeliveryStateEvent(ctx, tx, event); err != nil {
		return application.DeliveryClaim{}, false, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT fencing_token FROM notification_deliveries WHERE delivery_id = ?`, next.DeliveryID).Scan(&currentToken); err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: read claimed fencing token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return application.DeliveryClaim{}, false, fmt.Errorf("notification storage: commit delivery claim: %w", err)
	}
	return application.DeliveryClaim{
		Delivery: next,
		Lease:    application.DeliveryLease{WorkerID: workerID, FencingToken: currentToken, ExpiresAt: effectiveExpiry},
	}, true, nil
}

func (r *MySQLDeliveryRepository) UpdateWithLease(
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
		return fmt.Errorf("notification storage: begin delivery update: %w", err)
	}
	defer tx.Rollback()

	var workerID sql.NullString
	var leaseExpiry []byte
	var currentToken int64
	current, err := scanDelivery(tx.QueryRowContext(ctx, deliverySelect+`
		WHERE delivery_id = ? FOR UPDATE`, next.DeliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return application.ErrDeliveryNotFound
	}
	if err != nil {
		return fmt.Errorf("notification storage: lock delivery: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT lease_worker_id,
		DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'), fencing_token
		FROM notification_deliveries WHERE delivery_id = ?`, next.DeliveryID).
		Scan(&workerID, &leaseExpiry, &currentToken); err != nil {
		return fmt.Errorf("notification storage: read delivery lease: %w", err)
	}
	if !workerID.Valid || currentToken != fencingToken || len(leaseExpiry) == 0 {
		return application.ErrDeliveryLeaseConflict
	}
	expiry, err := parseDatabaseTime(leaseExpiry)
	if err != nil {
		return err
	}
	if !now.Before(expiry) {
		return application.ErrDeliveryLeaseConflict
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return application.ErrDeliveryVersionConflict
	}
	if event.From != current.Status {
		return domain.ErrInvalidDeliveryChange
	}
	if next.UpdatedAt.Before(current.UpdatedAt) || !next.UpdatedAt.Before(expiry) {
		return application.ErrDeliveryLeaseConflict
	}

	var result sql.Result
	if next.Status == domain.DeliverySending {
		result, err = tx.ExecContext(ctx, `UPDATE notification_deliveries SET
			status = ?, version = ?, attempt_count = ?, max_attempts = ?, next_attempt_at = ?,
			deadline_at = ?, reason_code = ?, last_error_code = ?, provider_message_ref = ?, updated_at = ?
			WHERE delivery_id = ? AND version = ? AND fencing_token = ?
			AND lease_expires_at > UTC_TIMESTAMP(6)`,
			string(next.Status), next.Version, next.AttemptCount, next.MaxAttempts, nullableTime(next.NextAttemptAt),
			next.DeadlineAt.UTC(), nullableText(next.ReasonCode), nullableText(next.LastErrorCode),
			nullableText(next.ProviderMessageRef), next.UpdatedAt.UTC(), next.DeliveryID, expectedVersion, fencingToken)
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE notification_deliveries SET
			status = ?, version = ?, attempt_count = ?, max_attempts = ?, next_attempt_at = ?,
			deadline_at = ?, reason_code = ?, last_error_code = ?, provider_message_ref = ?, updated_at = ?,
			lease_worker_id = NULL, lease_expires_at = NULL
			WHERE delivery_id = ? AND version = ? AND fencing_token = ?
			AND lease_expires_at > UTC_TIMESTAMP(6)`,
			string(next.Status), next.Version, next.AttemptCount, next.MaxAttempts, nullableTime(next.NextAttemptAt),
			next.DeadlineAt.UTC(), nullableText(next.ReasonCode), nullableText(next.LastErrorCode),
			nullableText(next.ProviderMessageRef), next.UpdatedAt.UTC(), next.DeliveryID, expectedVersion, fencingToken)
	}
	if err != nil {
		return fmt.Errorf("notification storage: update delivery with lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("notification storage: verify delivery update: %w", err)
	}
	if affected != 1 {
		return application.ErrDeliveryLeaseConflict
	}
	if err := insertDeliveryStateEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit delivery update: %w", err)
	}
	return nil
}

func insertDeliveryStateEvent(ctx context.Context, tx *sql.Tx, event domain.DeliveryStateEvent) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_delivery_state_events (
		delivery_id, version, from_status, to_status, changed_at, reason_code,
		evidence_ref, actor_ref, request_id, actor_reason
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.DeliveryID, event.Version, string(event.From), string(event.To), event.ChangedAt.UTC(),
		nullableText(event.ReasonCode), nullableText(event.EvidenceRef), nullableText(event.ActorRef),
		nullableText(event.RequestID), nullableText(event.ActorReason))
	if err != nil {
		return fmt.Errorf("notification storage: append delivery state event: %w", err)
	}
	return nil
}

func validDeliveryWorkerID(value string) bool {
	if value == "" || value != trimASCIIWhitespace(value) || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '-' && character != '_' &&
			character != '.' && character != ':' {
			return false
		}
	}
	return true
}

func trimASCIIWhitespace(value string) string {
	start, end := 0, len(value)
	for start < end && value[start] <= ' ' {
		start++
	}
	for end > start && value[end-1] <= ' ' {
		end--
	}
	return value[start:end]
}

func isUTCTime(value time.Time) bool {
	if value.IsZero() {
		return false
	}
	_, offset := value.Zone()
	return value.Location() == time.UTC && offset == 0
}

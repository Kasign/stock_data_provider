package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// MySQLSubscriptionRepository 保存不可变 Subscription 版本，并借助行锁实现版本 CAS。
type MySQLSubscriptionRepository struct {
	db              *sql.DB
	eventTypes      domain.EventTypeLookup
	filterValidator domain.FilterSchemaValidator
}

var _ application.SubscriptionRepository = (*MySQLSubscriptionRepository)(nil)

// NewMySQLSubscriptionRepository 接收 Server 管理的共享连接池及领域校验依赖。
func NewMySQLSubscriptionRepository(
	db *sql.DB,
	eventTypes domain.EventTypeLookup,
	filterValidator domain.FilterSchemaValidator,
) (*MySQLSubscriptionRepository, error) {
	if db == nil || eventTypes == nil {
		return nil, errors.New("notification storage: database and event type lookup are required")
	}
	return &MySQLSubscriptionRepository{db: db, eventTypes: eventTypes, filterValidator: filterValidator}, nil
}

const subscriptionSelect = `SELECT
	subscription_id, owner_ref, version, status, event_type,
	filter_schema_version, filters, recipient_scope, recipient_ref,
	channel_ref, template_id,
	DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s.%f'),
	DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f'),
	DATE_FORMAT(effective_at, '%Y-%m-%d %H:%i:%s.%f')
	FROM notification_subscriptions`

type subscriptionScanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row subscriptionScanner) (domain.Subscription, error) {
	var result domain.Subscription
	var status, recipientScope string
	var filters []byte
	var createdAt, updatedAt, effectiveAt []byte
	var filterSchemaVersion, recipientRef sql.NullString
	if err := row.Scan(
		&result.SubscriptionID, &result.OwnerRef, &result.Version, &status, &result.EventType,
		&filterSchemaVersion, &filters, &recipientScope, &recipientRef,
		&result.ChannelRef, &result.TemplateID, &createdAt, &updatedAt, &effectiveAt,
	); err != nil {
		return domain.Subscription{}, err
	}
	result.Status = domain.SubscriptionStatus(status)
	result.RecipientScope = domain.RecipientScope(recipientScope)
	result.FilterSchemaVersion = filterSchemaVersion.String
	result.RecipientRef = recipientRef.String
	if len(filters) > 0 && string(filters) != "null" {
		if !json.Valid(filters) {
			return domain.Subscription{}, errors.New("notification storage: persisted subscription filters are invalid JSON")
		}
		result.Filters = append(json.RawMessage(nil), filters...)
	}
	var err error
	if result.CreatedAt, err = parseDatabaseTime(createdAt); err != nil {
		return domain.Subscription{}, err
	}
	if result.UpdatedAt, err = parseDatabaseTime(updatedAt); err != nil {
		return domain.Subscription{}, err
	}
	if result.EffectiveAt, err = parseDatabaseTime(effectiveAt); err != nil {
		return domain.Subscription{}, err
	}
	return result, nil
}

func (r *MySQLSubscriptionRepository) validate(subscription domain.Subscription) error {
	if err := subscription.Validate(r.eventTypes, r.filterValidator); err != nil {
		return err
	}
	return nil
}

func (r *MySQLSubscriptionRepository) Create(ctx context.Context, subscription domain.Subscription) error {
	if r == nil || r.db == nil || ctx == nil {
		return errors.New("notification storage: repository is not configured")
	}
	if subscription.Version != 1 {
		return domain.ErrSubscriptionVersionConflict
	}
	if err := r.validate(subscription); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO notification_subscriptions (
		subscription_id, version, owner_ref, status, event_type, filter_schema_version,
		filters, recipient_scope, recipient_ref, channel_ref, template_id,
		created_at, updated_at, effective_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		subscription.SubscriptionID, subscription.Version, subscription.OwnerRef,
		string(subscription.Status), subscription.EventType, subscription.FilterSchemaVersion,
		nullableJSON(subscription.Filters), string(subscription.RecipientScope), subscription.RecipientRef,
		subscription.ChannelRef, subscription.TemplateID, subscription.CreatedAt.UTC(),
		subscription.UpdatedAt.UTC(), subscription.EffectiveAt.UTC(),
	)
	if isMySQLDuplicateKey(err) {
		return domain.ErrSubscriptionVersionConflict
	}
	if err != nil {
		return fmt.Errorf("notification storage: create subscription: %w", err)
	}
	return nil
}

func (r *MySQLSubscriptionRepository) GetLatest(
	ctx context.Context, subscriptionID string,
) (domain.Subscription, bool, error) {
	if r == nil || r.db == nil || ctx == nil || subscriptionID == "" {
		return domain.Subscription{}, false, errors.New("notification storage: repository or subscription id is invalid")
	}
	result, err := scanSubscription(r.db.QueryRowContext(ctx, subscriptionSelect+`
		WHERE subscription_id = ? ORDER BY version DESC LIMIT 1`, subscriptionID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Subscription{}, false, nil
	}
	if err != nil {
		return domain.Subscription{}, false, fmt.Errorf("notification storage: get latest subscription: %w", err)
	}
	if err := r.validate(result); err != nil {
		return domain.Subscription{}, false, fmt.Errorf("notification storage: persisted subscription is invalid: %w", err)
	}
	return cloneStoredSubscription(result), true, nil
}

func (r *MySQLSubscriptionRepository) AppendVersion(
	ctx context.Context, expectedVersion int64, next domain.Subscription,
) error {
	if r == nil || r.db == nil || ctx == nil {
		return errors.New("notification storage: repository is not configured")
	}
	if expectedVersion < 1 {
		return domain.ErrSubscriptionVersionConflict
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("notification storage: begin subscription version: %w", err)
	}
	defer tx.Rollback()

	previous, err := scanSubscription(tx.QueryRowContext(ctx, subscriptionSelect+`
		WHERE subscription_id = ? ORDER BY version DESC LIMIT 1 FOR UPDATE`, next.SubscriptionID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrSubscriptionVersionConflict
	}
	if err != nil {
		return fmt.Errorf("notification storage: lock latest subscription: %w", err)
	}
	if previous.Version != expectedVersion {
		return domain.ErrSubscriptionVersionConflict
	}
	if err := r.validate(previous); err != nil {
		return fmt.Errorf("notification storage: previous subscription version is invalid: %w", err)
	}
	if err := domain.ValidateSubscriptionVersion(previous, next); err != nil {
		return err
	}
	if err := r.validate(next); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notification_subscriptions (
		subscription_id, version, owner_ref, status, event_type, filter_schema_version,
		filters, recipient_scope, recipient_ref, channel_ref, template_id,
		created_at, updated_at, effective_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		next.SubscriptionID, next.Version, next.OwnerRef, string(next.Status), next.EventType,
		next.FilterSchemaVersion, nullableJSON(next.Filters), string(next.RecipientScope),
		next.RecipientRef, next.ChannelRef, next.TemplateID, next.CreatedAt.UTC(),
		next.UpdatedAt.UTC(), next.EffectiveAt.UTC(),
	)
	if isMySQLDuplicateKey(err) {
		return domain.ErrSubscriptionVersionConflict
	}
	if err != nil {
		return fmt.Errorf("notification storage: append subscription version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("notification storage: commit subscription version: %w", err)
	}
	return nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return []byte(value)
}

func cloneStoredSubscription(value domain.Subscription) domain.Subscription {
	value.Filters = append(json.RawMessage(nil), value.Filters...)
	return value
}


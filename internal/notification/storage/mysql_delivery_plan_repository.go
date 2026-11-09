package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	mysql "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// MySQLDeliveryPlanRepository persists immutable DeliveryPlan snapshots.
// It borrows the Server-owned database pool and never manages its lifecycle.
type MySQLDeliveryPlanRepository struct {
	db *sql.DB
}

var _ application.DeliveryPlanRepository = (*MySQLDeliveryPlanRepository)(nil)

// NewMySQLDeliveryPlanRepository wraps the shared MariaDB connection pool.
func NewMySQLDeliveryPlanRepository(db *sql.DB) (*MySQLDeliveryPlanRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLDeliveryPlanRepository{db: db}, nil
}

// CreateOrGet relies on the database unique identity tuple for cross-process
// idempotency. A duplicate is accepted only after loading and verifying the
// original immutable snapshot and comparing its complete fingerprint.
func (r *MySQLDeliveryPlanRepository) CreateOrGet(
	ctx context.Context,
	plan domain.DeliveryPlan,
) (domain.DeliveryPlan, bool, error) {
	if r == nil || r.db == nil || ctx == nil {
		return domain.DeliveryPlan{}, false, errors.New("notification storage: repository is not configured")
	}
	if err := plan.Validate(); err != nil {
		return domain.DeliveryPlan{}, false, err
	}
	if err := plan.VerifyFingerprint(); err != nil {
		return domain.DeliveryPlan{}, false, err
	}

	policyVersions, err := json.Marshal(plan.AuthorizationPolicyVersions)
	if err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: encode policy versions: %w", err)
	}
	authorizationRefs, err := json.Marshal(plan.AuthorizationRefs)
	if err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: encode authorization refs: %w", err)
	}
	snapshot, err := json.Marshal(plan)
	if err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: encode delivery plan: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `INSERT INTO notification_delivery_plans (
		delivery_id, source_module, event_id, subscription_id, subscription_version,
		channel_ref, channel_config_version, template_id, template_version, recipient_ref,
		payload_fingerprint, authorization_policy_versions, authorization_refs,
		plan_fingerprint, delivery_idempotency_key, plan_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.DeliveryID, plan.SourceModule, plan.EventID, plan.SubscriptionID, plan.SubscriptionVersion,
		plan.ChannelRef, plan.ChannelConfigVersion, plan.TemplateID, plan.TemplateVersion, plan.RecipientRef,
		plan.PayloadFingerprint, policyVersions, authorizationRefs,
		plan.PlanFingerprint, plan.DeliveryIdempotencyKey, snapshot,
	)
	if err == nil {
		return cloneDeliveryPlan(plan), false, nil
	}
	if !isMySQLDuplicateKey(err) {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: insert delivery plan: %w", err)
	}

	existing, found, loadErr := r.getByIdentity(ctx, plan)
	if loadErr != nil {
		return domain.DeliveryPlan{}, false, loadErr
	}
	if !found {
		// The duplicate may have been caused by a delivery_id collision rather
		// than the business identity tuple. Do not misreport it as an idempotent replay.
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: delivery plan unique constraint conflict: %w", err)
	}
	if err := domain.ValidateDeliveryPlanReplay(existing, plan); err != nil {
		return domain.DeliveryPlan{}, false, err
	}
	return existing, true, nil
}

func (r *MySQLDeliveryPlanRepository) getByIdentity(
	ctx context.Context,
	plan domain.DeliveryPlan,
) (domain.DeliveryPlan, bool, error) {
	var snapshot []byte
	err := r.db.QueryRowContext(ctx, `SELECT plan_json FROM notification_delivery_plans
		WHERE source_module = ? AND event_id = ? AND subscription_id = ? AND subscription_version = ?
		AND channel_ref = ? AND channel_config_version = ? AND template_id = ? AND template_version = ?
		AND recipient_ref = ?`,
		plan.SourceModule, plan.EventID, plan.SubscriptionID, plan.SubscriptionVersion,
		plan.ChannelRef, plan.ChannelConfigVersion, plan.TemplateID, plan.TemplateVersion,
		plan.RecipientRef,
	).Scan(&snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeliveryPlan{}, false, nil
	}
	if err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: load existing delivery plan: %w", err)
	}
	var existing domain.DeliveryPlan
	if err := json.Unmarshal(snapshot, &existing); err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: decode stored delivery plan: %w", err)
	}
	if err := existing.Validate(); err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: stored delivery plan is invalid: %w", err)
	}
	if err := existing.VerifyFingerprint(); err != nil {
		return domain.DeliveryPlan{}, false, fmt.Errorf("notification storage: stored delivery plan fingerprint is invalid: %w", err)
	}
	return existing, true, nil
}

func isMySQLDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func cloneDeliveryPlan(plan domain.DeliveryPlan) domain.DeliveryPlan {
	plan.AuthorizationPolicyVersions = append([]domain.AuthorizationPolicyVersion(nil), plan.AuthorizationPolicyVersions...)
	plan.AuthorizationRefs = append([]string(nil), plan.AuthorizationRefs...)
	return plan
}

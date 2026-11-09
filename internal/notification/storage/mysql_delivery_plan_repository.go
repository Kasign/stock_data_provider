package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

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

var errDeliveryPlanSnapshotColumnsMismatch = errors.New("notification storage: delivery plan snapshot columns mismatch")

// NewMySQLDeliveryPlanRepository wraps the shared MariaDB connection pool.
func NewMySQLDeliveryPlanRepository(db *sql.DB) (*MySQLDeliveryPlanRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLDeliveryPlanRepository{db: db}, nil
}

// CreateOrGet relies on the database unique identity tuple for cross-process
// idempotency. A duplicate is accepted only after checking both the immutable
// JSON snapshot and every denormalized column against the same persisted row.
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
	var columns deliveryPlanColumns
	err := r.db.QueryRowContext(ctx, `SELECT
		delivery_id, source_module, event_id, subscription_id, subscription_version,
		channel_ref, channel_config_version, template_id, template_version, recipient_ref,
		payload_fingerprint, authorization_policy_versions, authorization_refs,
		plan_fingerprint, delivery_idempotency_key, plan_json
		FROM notification_delivery_plans
		WHERE source_module = ? AND event_id = ? AND subscription_id = ? AND subscription_version = ?
		AND channel_ref = ? AND channel_config_version = ? AND template_id = ? AND template_version = ?
		AND recipient_ref = ?`,
		plan.SourceModule, plan.EventID, plan.SubscriptionID, plan.SubscriptionVersion,
		plan.ChannelRef, plan.ChannelConfigVersion, plan.TemplateID, plan.TemplateVersion,
		plan.RecipientRef,
	).Scan(
		&columns.deliveryID, &columns.sourceModule, &columns.eventID, &columns.subscriptionID,
		&columns.subscriptionVersion, &columns.channelRef, &columns.channelConfigVersion,
		&columns.templateID, &columns.templateVersion, &columns.recipientRef,
		&columns.payloadFingerprint, &columns.policyVersionsJSON, &columns.authorizationRefsJSON,
		&columns.planFingerprint, &columns.deliveryIdempotencyKey, &snapshot,
	)
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
	if err := validateDeliveryPlanColumns(existing, columns); err != nil {
		return domain.DeliveryPlan{}, false, err
	}
	return existing, true, nil
}

type deliveryPlanColumns struct {
	deliveryID             string
	sourceModule           string
	eventID                string
	subscriptionID         string
	subscriptionVersion    int64
	channelRef             string
	channelConfigVersion   int64
	templateID             string
	templateVersion        int64
	recipientRef           string
	payloadFingerprint     string
	policyVersionsJSON     []byte
	authorizationRefsJSON  []byte
	planFingerprint        string
	deliveryIdempotencyKey string
}

func validateDeliveryPlanColumns(plan domain.DeliveryPlan, columns deliveryPlanColumns) error {
	var policyVersions []domain.AuthorizationPolicyVersion
	if err := json.Unmarshal(columns.policyVersionsJSON, &policyVersions); err != nil {
		return fmt.Errorf("%w: invalid authorization policy versions JSON", errDeliveryPlanSnapshotColumnsMismatch)
	}
	var authorizationRefs []string
	if err := json.Unmarshal(columns.authorizationRefsJSON, &authorizationRefs); err != nil {
		return fmt.Errorf("%w: invalid authorization references JSON", errDeliveryPlanSnapshotColumnsMismatch)
	}
	if columns.deliveryID != plan.DeliveryID ||
		columns.sourceModule != plan.SourceModule ||
		columns.eventID != plan.EventID ||
		columns.subscriptionID != plan.SubscriptionID ||
		columns.subscriptionVersion != plan.SubscriptionVersion ||
		columns.channelRef != plan.ChannelRef ||
		columns.channelConfigVersion != plan.ChannelConfigVersion ||
		columns.templateID != plan.TemplateID ||
		columns.templateVersion != plan.TemplateVersion ||
		columns.recipientRef != plan.RecipientRef ||
		columns.payloadFingerprint != plan.PayloadFingerprint ||
		!reflect.DeepEqual(policyVersions, plan.AuthorizationPolicyVersions) ||
		!reflect.DeepEqual(authorizationRefs, plan.AuthorizationRefs) ||
		columns.planFingerprint != plan.PlanFingerprint ||
		columns.deliveryIdempotencyKey != plan.DeliveryIdempotencyKey {
		return errDeliveryPlanSnapshotColumnsMismatch
	}
	return nil
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

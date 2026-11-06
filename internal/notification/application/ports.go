package application

import (
	"context"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

// SubscriptionRepository stores immutable Subscription versions.
// Create must require version=1 and enforce subscription_id uniqueness.
// AppendVersion must atomically compare expectedVersion, require next.Version=expectedVersion+1,
// and insert a new immutable version without updating prior versions.
type SubscriptionRepository interface {
	Create(ctx context.Context, subscription domain.Subscription) error
	GetLatest(ctx context.Context, subscriptionID string) (subscription domain.Subscription, found bool, err error)
	AppendVersion(ctx context.Context, expectedVersion int64, next domain.Subscription) error
}

// DeliveryPlanRepository enforces the contract Delivery unique tuple atomically.
// Same tuple + same plan_fingerprint returns the original plan as duplicate;
// same tuple + different plan_fingerprint returns domain.ErrDeliveryPlanConflict.
type DeliveryPlanRepository interface {
	CreateOrGet(ctx context.Context, plan domain.DeliveryPlan) (existing domain.DeliveryPlan, duplicate bool, err error)
}

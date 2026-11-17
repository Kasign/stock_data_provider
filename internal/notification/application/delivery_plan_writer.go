package application

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryPlanRepositoryUnavailable = errors.New("notification: delivery plan repository unavailable")

// DeliveryPlanWriter is the application boundary for persisting immutable plans.
// It validates structure and fingerprint independently before invoking persistence.
type DeliveryPlanWriter struct {
	repository DeliveryPlanRepository
}

func NewDeliveryPlanWriter(repository DeliveryPlanRepository) (*DeliveryPlanWriter, error) {
	if repository == nil {
		return nil, ErrDeliveryPlanRepositoryUnavailable
	}
	return &DeliveryPlanWriter{repository: repository}, nil
}

func (w *DeliveryPlanWriter) CreateOrGet(ctx context.Context, plan domain.DeliveryPlan) (domain.DeliveryPlan, bool, error) {
	if w == nil || w.repository == nil {
		return domain.DeliveryPlan{}, false, ErrDeliveryPlanRepositoryUnavailable
	}
	if err := plan.Validate(); err != nil {
		return domain.DeliveryPlan{}, false, err
	}
	if err := plan.VerifyFingerprint(); err != nil {
		return domain.DeliveryPlan{}, false, err
	}
	return w.repository.CreateOrGet(ctx, plan)
}

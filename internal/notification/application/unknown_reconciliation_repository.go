package application

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrUnknownReconciliationConflict = errors.New("notification: unknown reconciliation conflict")

// UnknownDeliveryReconciliationRepository 在不依赖 Worker 租约的前提下，
// 以 Delivery 版本 CAS 持久化已核验的 unknown 对账结果。
// Delivery 快照和状态事件必须在同一事务中提交。
type UnknownDeliveryReconciliationRepository interface {
	ReconcileUnknown(ctx context.Context, expectedVersion int64, next domain.DeliveryRecord, event domain.DeliveryStateEvent) error
}

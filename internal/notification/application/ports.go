package application

import (
	"context"
	"errors"
	"time"

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

// DeliveryLease 表示当前 Delivery 的独占执行租约。
// 每次成功领取同一 Delivery 时，FencingToken 必须单调递增。
type DeliveryLease struct {
	WorkerID     string
	FencingToken int64
	ExpiresAt    time.Time
}

// DeliveryClaim 返回原子领取时取得的 Delivery 快照与租约。
type DeliveryClaim struct {
	Delivery domain.DeliveryRecord
	Lease    DeliveryLease
}

var (
	ErrDeliveryAlreadyExists        = errors.New("notification: delivery already exists")
	ErrDeliveryNotFound             = errors.New("notification: delivery not found")
	ErrDeliveryVersionConflict      = errors.New("notification: delivery version conflict")
	ErrDeliveryLeaseConflict        = errors.New("notification: delivery lease conflict")
	ErrDeliveryAttemptAlreadyExists = errors.New("notification: delivery attempt already exists")
	ErrDeliveryAttemptNotFound      = errors.New("notification: delivery attempt not found")
	ErrDeliveryAttemptConflict      = errors.New("notification: delivery attempt conflict")
)

// DeliveryRepository 负责 Delivery 当前快照与追加式状态事件。
// ClaimDue 原子领取一条到期的 queued/retry_wait 记录，迁移为 sending，追加状态事件并分配新 fencing token。
// UpdateWithLease 必须在同一原子操作中 CAS 更新快照并追加事件；过期租约、旧 token、旧版本或不匹配事件均须拒绝；next.Status 不再是 sending 时必须同时释放租约。
type DeliveryRepository interface {
	Create(ctx context.Context, delivery domain.DeliveryRecord) error
	Get(ctx context.Context, deliveryID string) (delivery domain.DeliveryRecord, found bool, err error)
	ClaimDue(ctx context.Context, workerID string, now, leaseExpiresAt time.Time) (claim DeliveryClaim, claimed bool, err error)
	UpdateWithLease(ctx context.Context, expectedVersion, fencingToken int64, next domain.DeliveryRecord, event domain.DeliveryStateEvent, now time.Time) error
}

// ExpiredDeliveryLeaseScanner 以有界批次返回数据库确认已过期的 sending 租约。
// 返回项只是候选快照，恢复写入仍必须重新执行版本/token/数据库时钟 CAS。
type ExpiredDeliveryLeaseScanner interface {
	ListExpiredLeases(ctx context.Context, limit int) ([]DeliveryClaim, error)
}

// DeliveryLeaseRecoveryRepository 将已过期租约的恢复结果与状态事件原子持久化。
// 评估逻辑必须先通过完整 Attempt 历史调用 AssessExpiredDeliveryLease；
// 本接口只负责在数据库权威时钟确认租约过期后，以版本和 fencing token CAS 收敛状态。
type DeliveryLeaseRecoveryRepository interface {
	RecoverExpiredLease(ctx context.Context, expectedVersion, fencingToken int64, next domain.DeliveryRecord, event domain.DeliveryStateEvent, now time.Time) error
}

// DeliveryAttemptRepository 管理单次渠道调用的追加式历史。
// StartWithLease 必须在原子写入时使用 Repository 权威时钟校验租约仍有效、StartedAt 不在未来；同一 fencing token 最多追加一个 Attempt。
// attempt_count 是 Attempt 历史的计数投影，随 StartWithLease 原子更新，不改变 Delivery 状态版本。
// CompleteWithLease 必须在原子写入时使用 Repository 权威时钟校验租约，并只允许当前有效租约将运行中 Attempt 完成一次；完成记录不可再次覆盖。
type DeliveryAttemptRepository interface {
	StartWithLease(ctx context.Context, expectedDeliveryVersion, fencingToken int64, attempt domain.DeliveryAttempt) error
	CompleteWithLease(ctx context.Context, expectedDeliveryVersion, fencingToken int64, completed domain.DeliveryAttempt) error
	ListByDelivery(ctx context.Context, deliveryID string) (attempts []domain.DeliveryAttempt, err error)
}

// TemplateRenderer renders one immutable template version against a validated event.
// Implementations must expose only declared scalar variables and enforce output limits.
type TemplateRenderer interface {
	Render(ctx context.Context, event domain.EventEnvelope, templateID string, version int64) (domain.RenderedContent, error)
}

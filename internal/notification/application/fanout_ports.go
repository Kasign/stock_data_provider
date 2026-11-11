package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrFanoutWorkItemLeaseConflict = errors.New("notification: fanout work item lease conflict")
	ErrFanoutConfigurationInvalid  = errors.New("notification: fanout configuration is invalid")
)

// FanoutClaim 是数据库成功领取后的不可变工作快照及其 fencing 租约。
type FanoutClaim struct {
	WorkItem domain.FanoutWorkItem
	Event    domain.EventEnvelope
}

// FanoutWorkItemRepository 的状态更新必须校验租约和 fencing token。
type FanoutWorkItemRepository interface {
	ClaimDue(ctx context.Context, workerID string, leaseDuration time.Duration) (claim FanoutClaim, claimed bool, err error)
	AdvanceWithLease(ctx context.Context, claim FanoutClaim, nextCursor string, completed bool) error
	RetryWithLease(ctx context.Context, claim FanoutClaim, nextAttemptAt time.Time, reasonCode string, terminal bool) error
}

// SubscriptionPageReader 只返回固定修订快照内每个 Subscription 的最新 active 版本。
type SubscriptionPageReader interface {
	ListActiveAtRevision(ctx context.Context, eventType string, snapshotRevision int64, afterSubscriptionID string, limit int) ([]domain.Subscription, error)
}

// FanoutPlanDecision 明确区分不匹配与生成计划；不匹配不是错误，也不产生 Delivery。
type FanoutPlanDecision struct {
	Matched bool
	Plan    domain.DeliveryPlan
}

// FanoutPlanFactory 仅在 Fanout 展开时调用；负责过滤和解析不可变计划输入，不得调用渠道 Send。
type FanoutPlanFactory interface {
	Build(ctx context.Context, event domain.EventEnvelope, subscription domain.Subscription) (FanoutPlanDecision, error)
}

type FanoutRetryPolicy struct {
	MaxAttempts int64
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

type FanoutDispatcherConfig struct {
	PageSize      int
	LeaseDuration time.Duration
	Retry         FanoutRetryPolicy
}

type FanoutRunReport struct {
	Claimed        bool
	WorkItemID     string
	Processed      int
	PlansCreated   int
	PlansReplayed  int
	Completed      bool
	RetryScheduled bool
	Failed         bool
}

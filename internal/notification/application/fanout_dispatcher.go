package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type FanoutDispatcher struct {
	workItems FanoutWorkItemRepository
	subs      SubscriptionPageReader
	factory   FanoutPlanFactory
	plans     *DeliveryPlanWriter
	clock     Clock
	config    FanoutDispatcherConfig
}

func NewFanoutDispatcher(
	workItems FanoutWorkItemRepository,
	subs SubscriptionPageReader,
	factory FanoutPlanFactory,
	plans *DeliveryPlanWriter,
	clock Clock,
	config FanoutDispatcherConfig,
) (*FanoutDispatcher, error) {
	if workItems == nil || subs == nil || factory == nil || plans == nil || clock == nil ||
		config.PageSize < 1 || config.PageSize > 200 || config.LeaseDuration <= 0 ||
		config.Retry.MaxAttempts < 1 || config.Retry.BaseDelay <= 0 ||
		config.Retry.MaxDelay < config.Retry.BaseDelay {
		return nil, ErrFanoutConfigurationInvalid
	}
	return &FanoutDispatcher{
		workItems: workItems,
		subs:      subs,
		factory:   factory,
		plans:     plans,
		clock:     clock,
		config:    config,
	}, nil
}

// RunOnce 处理最多一个 WorkItem 的一个页面；Worker 生命周期和循环由 Server 管理。
func (d *FanoutDispatcher) RunOnce(ctx context.Context, workerID string) (FanoutRunReport, error) {
	var report FanoutRunReport
	if d == nil || d.workItems == nil || d.subs == nil || d.factory == nil || d.plans == nil || d.clock == nil ||
		ctx == nil || workerID == "" || len(workerID) > 128 {
		return report, ErrFanoutConfigurationInvalid
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	claim, claimed, err := d.workItems.ClaimDue(ctx, workerID, d.config.LeaseDuration)
	if err != nil || !claimed {
		return report, err
	}
	report.Claimed = true
	report.WorkItemID = claim.WorkItem.WorkItemID
	if claim.WorkItem.AttemptCount > d.config.Retry.MaxAttempts {
		if err := d.workItems.RetryWithLease(ctx, claim, time.Time{}, "notification.fanout.attempts_exhausted", true); err != nil {
			return report, err
		}
		report.Failed = true
		return report, nil
	}
	if err := claim.WorkItem.Validate(); err != nil {
		return d.retryOrFail(ctx, claim, report, err)
	}
	if err := claim.Event.VerifyPayloadFingerprint(); err != nil {
		return d.retryOrFail(ctx, claim, report, err)
	}
	if claim.Event.SourceModule != claim.WorkItem.SourceModule ||
		claim.Event.EventID != claim.WorkItem.EventID ||
		claim.Event.EventType != claim.WorkItem.EventType ||
		claim.Event.PayloadFingerprint != claim.WorkItem.PayloadFingerprint ||
		!claim.WorkItem.HasSnapshot {
		return d.retryOrFail(ctx, claim, report, domain.ErrInvalidFanoutWorkItem)
	}

	page, err := d.subs.ListActiveAtRevision(
		ctx,
		claim.Event.EventType,
		claim.WorkItem.SnapshotRevision,
		claim.WorkItem.CursorSubscriptionID,
		d.config.PageSize+1,
	)
	if err != nil {
		return d.retryOrFail(ctx, claim, report, err)
	}
	if len(page) > d.config.PageSize+1 {
		return d.retryOrFail(ctx, claim, report, domain.ErrInvalidFanoutWorkItem)
	}
	hasMore := len(page) > d.config.PageSize
	if hasMore {
		page = page[:d.config.PageSize]
	}
	nextCursor := claim.WorkItem.CursorSubscriptionID
	for _, subscription := range page {
		if err := ctx.Err(); err != nil {
			return d.retryOrFail(ctx, claim, report, err)
		}
		if subscription.SubscriptionID <= nextCursor {
			return d.retryOrFail(ctx, claim, report, domain.ErrInvalidFanoutWorkItem)
		}
		decision, buildErr := d.factory.Build(ctx, claim.Event, subscription)
		if buildErr != nil {
			return d.retryOrFail(ctx, claim, report, buildErr)
		}
		if decision.Matched {
			_, duplicate, writeErr := d.plans.CreateOrGet(ctx, decision.Plan)
			if writeErr != nil {
				return d.retryOrFail(ctx, claim, report, writeErr)
			}
			if duplicate {
				report.PlansReplayed++
			} else {
				report.PlansCreated++
			}
		}
		nextCursor = subscription.SubscriptionID
		report.Processed++
	}
	if err := d.workItems.AdvanceWithLease(ctx, claim, nextCursor, !hasMore); err != nil {
		return report, err
	}
	report.Completed = !hasMore
	return report, nil
}

func (d *FanoutDispatcher) retryOrFail(
	ctx context.Context,
	claim FanoutClaim,
	report FanoutRunReport,
	cause error,
) (FanoutRunReport, error) {
	now := d.clock.Now()
	if now.IsZero() || now.Location() != time.UTC {
		return report, ErrFanoutConfigurationInvalid
	}
	reasonCode := "notification.fanout.processing_failed"
	retryable := true
	if errors.Is(cause, domain.ErrInvalidDeliveryPlan) ||
		errors.Is(cause, domain.ErrDeliveryPlanConflict) ||
		errors.Is(cause, domain.ErrInvalidFanoutWorkItem) {
		retryable = false
		reasonCode = "notification.fanout.contract_violation"
	}
	terminal := !retryable || claim.WorkItem.AttemptCount >= d.config.Retry.MaxAttempts
	if terminal {
		if err := d.workItems.RetryWithLease(ctx, claim, time.Time{}, reasonCode, true); err != nil {
			return report, err
		}
		report.Failed = true
		return report, nil
	}
	delay := d.config.Retry.BaseDelay
	for attempt := int64(1); attempt < claim.WorkItem.AttemptCount && delay < d.config.Retry.MaxDelay; attempt++ {
		if delay > d.config.Retry.MaxDelay/2 {
			delay = d.config.Retry.MaxDelay
			break
		}
		delay *= 2
	}
	if delay > d.config.Retry.MaxDelay {
		delay = d.config.Retry.MaxDelay
	}
	nextAttemptAt := now.Add(delay).Truncate(time.Microsecond)
	if err := d.workItems.RetryWithLease(ctx, claim, nextAttemptAt, reasonCode, false); err != nil {
		return report, err
	}
	report.RetryScheduled = true
	return report, nil
}

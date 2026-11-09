package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryExecutionUnavailable = errors.New("notification: delivery execution unavailable")

// DeliveryExecutionResult 汇总一次 Worker 对单条 Delivery 的执行结果。
// 最终状态以 Delivery 与 Attempt Repository 中的持久化事实为准。
type DeliveryExecutionResult struct {
	Action        DeliveryPreflightAction
	ReasonCode    string
	AttemptID     string
	ChannelResult *domain.ChannelSendResult
}

// DeliveryExecutionService 串联单条 Delivery 的 Preflight、渲染、Attempt 与渠道调用。
// 队列扫描和 Claim 由上层 Worker 负责，避免本服务隐式拥有调度生命周期。
type DeliveryExecutionService struct {
	preflight           *DeliveryPreflight
	preflightCompletion *DeliveryPreflightCompletionService
	renderer            TemplateRenderer
	attempts            DeliveryAttemptRepository
	completion          *DeliveryCompletionService
}

func NewDeliveryExecutionService(
	preflight *DeliveryPreflight,
	preflightCompletion *DeliveryPreflightCompletionService,
	renderer TemplateRenderer,
	attempts DeliveryAttemptRepository,
	completion *DeliveryCompletionService,
) (*DeliveryExecutionService, error) {
	if preflight == nil || preflightCompletion == nil || renderer == nil || attempts == nil || completion == nil {
		return nil, ErrDeliveryExecutionUnavailable
	}
	return &DeliveryExecutionService{
		preflight: preflight, preflightCompletion: preflightCompletion,
		renderer: renderer, attempts: attempts, completion: completion,
	}, nil
}

// Execute 执行一条已领取的 Delivery。attemptID、startedAt、finishedAt 和 evidenceRef
// 由 Worker 提供，以复用 Server 的 ID、Clock 与诊断证据生成能力。
// Preflight/渲染/请求构建失败会 fail closed；Attempt 启动失败时绝不调用渠道。
func (s *DeliveryExecutionService) Execute(
	ctx context.Context,
	claim DeliveryClaim,
	event domain.EventEnvelope,
	attemptID string,
	startedAt time.Time,
	finishedAt time.Time,
	randomUnit float64,
	evidenceRef string,
) (DeliveryExecutionResult, error) {
	var output DeliveryExecutionResult
	if s == nil || s.preflight == nil || s.preflightCompletion == nil ||
		s.renderer == nil || s.attempts == nil || s.completion == nil {
		return output, ErrDeliveryExecutionUnavailable
	}
	if ctx == nil {
		return output, ErrDeliveryExecutionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return output, err
	}

	preflight, preflightErr := s.preflight.Check(ctx, claim.Delivery)
	output.Action, output.ReasonCode = preflight.Action, preflight.ReasonCode
	if preflight.Action != DeliveryPreflightReady {
		if preflight.Action == "" {
			return output, preflightErr
		}
		if applyErr := s.preflightCompletion.Apply(ctx, claim, preflight, startedAt, evidenceRef); applyErr != nil {
			return output, errors.Join(preflightErr, applyErr)
		}
		return output, preflightErr
	}
	if preflightErr != nil {
		return output, preflightErr
	}

	content, err := s.renderer.Render(ctx, event, claim.Delivery.TemplateID, claim.Delivery.TemplateVersion)
	if err != nil {
		block := DeliveryPreflightResult{Action: DeliveryPreflightBlock, ReasonCode: "template_render_failed"}
		if applyErr := s.preflightCompletion.Apply(ctx, claim, block, startedAt, evidenceRef); applyErr != nil {
			return output, errors.Join(err, applyErr)
		}
		output.Action, output.ReasonCode = block.Action, block.ReasonCode
		return output, err
	}

	request, err := BuildChannelSendRequest(
		claim.Delivery, preflight.ChannelSnapshot, preflight.Recipient, content,
	)
	if err != nil {
		block := DeliveryPreflightResult{Action: DeliveryPreflightBlock, ReasonCode: "channel_request_invalid"}
		if applyErr := s.preflightCompletion.Apply(ctx, claim, block, startedAt, evidenceRef); applyErr != nil {
			return output, errors.Join(err, applyErr)
		}
		output.Action, output.ReasonCode = block.Action, block.ReasonCode
		return output, err
	}

	attempt := domain.DeliveryAttempt{
		AttemptID: attemptID, DeliveryID: claim.Delivery.DeliveryID,
		AttemptNo: int(claim.Delivery.AttemptCount + 1),
		WorkerID: claim.Lease.WorkerID, FencingToken: claim.Lease.FencingToken,
		LeaseExpiresAt: claim.Lease.ExpiresAt, StartedAt: startedAt,
	}
	if err := attempt.Validate(); err != nil {
		return output, err
	}
	if err := s.attempts.StartWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		return output, err
	}

	result, sendErr := preflight.Channel.Send(ctx, request)
	output.AttemptID = attemptID
	output.ChannelResult = &result
	if err := s.completion.Complete(
		ctx, claim, attempt, preflight.ChannelSnapshot, result,
		finishedAt, randomUnit, evidenceRef,
	); err != nil {
		return output, errors.Join(sendErr, err)
	}
	output.Action = DeliveryPreflightReady
	return output, sendErr
}

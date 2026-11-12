package application

import (
	"context"
	"errors"
	"math"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrDeliveryExecutionUnavailable   = errors.New("notification: delivery execution unavailable")
	ErrDeliveryExecutionEventMismatch = errors.New("notification: execution event does not match Delivery snapshot")
)

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
	clock               Clock
}

func NewDeliveryExecutionService(
	preflight *DeliveryPreflight,
	preflightCompletion *DeliveryPreflightCompletionService,
	renderer TemplateRenderer,
	attempts DeliveryAttemptRepository,
	completion *DeliveryCompletionService,
	clock Clock,
) (*DeliveryExecutionService, error) {
	if preflight == nil || preflightCompletion == nil || renderer == nil ||
		attempts == nil || completion == nil || clock == nil {
		return nil, ErrDeliveryExecutionUnavailable
	}
	return &DeliveryExecutionService{
		preflight: preflight, preflightCompletion: preflightCompletion,
		renderer: renderer, attempts: attempts, completion: completion, clock: clock,
	}, nil
}

func (s *DeliveryExecutionService) executionTime() (time.Time, error) {
	now := s.clock.Now()
	if !isRecoveryUTC(now) {
		return time.Time{}, errors.New("notification: execution clock must return UTC time")
	}
	return now.Truncate(time.Microsecond), nil
}

// Execute 执行一条已领取的 Delivery。Attempt ID、RetryPolicy 随机采样值和
// 诊断 evidence_ref 由 Worker 提供；开始/完成时间由注入的 Clock 在真实边界采样。
// Attempt.StartWithLease 失败时绝不调用渠道；发送结果不确定时沿用 Unknown 对账流程。
func (s *DeliveryExecutionService) Execute(
	ctx context.Context,
	claim DeliveryClaim,
	event domain.EventEnvelope,
	attemptID string,
	randomUnit float64,
	evidenceRef string,
) (DeliveryExecutionResult, error) {
	var output DeliveryExecutionResult
	if s == nil || s.preflight == nil || s.preflightCompletion == nil ||
		s.renderer == nil || s.attempts == nil || s.completion == nil || s.clock == nil {
		return output, ErrDeliveryExecutionUnavailable
	}
	if ctx == nil {
		return output, ErrDeliveryExecutionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return output, err
	}
	if math.IsNaN(randomUnit) || math.IsInf(randomUnit, 0) || randomUnit < 0 || randomUnit >= 1 {
		return output, ErrRetryNotApplicable
	}
	if event.EventID != claim.Delivery.EventID ||
		event.SourceModule != claim.Delivery.SourceModule ||
		event.PayloadFingerprint != claim.Delivery.PayloadFingerprint ||
		event.VerifyPayloadFingerprint() != nil {
		return output, ErrDeliveryExecutionEventMismatch
	}

	preflight, preflightErr := s.preflight.Check(ctx, claim.Delivery)
	output.Action, output.ReasonCode = preflight.Action, preflight.ReasonCode
	if preflight.Action != DeliveryPreflightReady {
		if preflight.Action == "" {
			return output, preflightErr
		}
		at, timeErr := s.executionTime()
		if timeErr != nil {
			return output, errors.Join(preflightErr, timeErr)
		}
		if applyErr := s.preflightCompletion.Apply(ctx, claim, preflight, at, evidenceRef); applyErr != nil {
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
		at, timeErr := s.executionTime()
		if timeErr != nil {
			return output, errors.Join(err, timeErr)
		}
		if applyErr := s.preflightCompletion.Apply(ctx, claim, block, at, evidenceRef); applyErr != nil {
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
		at, timeErr := s.executionTime()
		if timeErr != nil {
			return output, errors.Join(err, timeErr)
		}
		if applyErr := s.preflightCompletion.Apply(ctx, claim, block, at, evidenceRef); applyErr != nil {
			return output, errors.Join(err, applyErr)
		}
		output.Action, output.ReasonCode = block.Action, block.ReasonCode
		return output, err
	}

	history, err := s.attempts.ListByDelivery(ctx, claim.Delivery.DeliveryID)
	if err != nil {
		return output, err
	}
	if err := ValidateDeliveryAttemptHistoryForDelivery(claim.Delivery, history); err != nil {
		return output, ErrLeaseRecoveryHistory
	}
	if len(history) != claim.Delivery.AttemptCount {
		return output, ErrLeaseRecoveryHistory
	}
	for i := range history {
		previous := history[i]
		if err := previous.Validate(); err != nil ||
			previous.DeliveryID != claim.Delivery.DeliveryID ||
			previous.AttemptNo != i+1 || previous.FinishedAt == nil || previous.Outcome == nil {
			return output, ErrLeaseRecoveryHistory
		}
		if i == 0 {
			firstAttemptAt = previous.StartedAt
		} else {
			prior := history[i-1]
			if previous.FencingToken <= prior.FencingToken ||
				previous.StartedAt.Before(*prior.FinishedAt) {
				return output, ErrLeaseRecoveryHistory
			}
			switch *prior.Outcome {
			case domain.DeliveryAttemptRejected:
				if !prior.Retryable {
					return output, ErrLeaseRecoveryHistory
				}
			case domain.DeliveryAttemptNotSent:
			default:
				return output, ErrLeaseRecoveryHistory
			}
		}
	}
	// Attempt 开始时间在历史校验完成后采样，避免历史读取耗时占用尚未开始的单次预算。
	startedAt, err := s.executionTime()
	if err != nil {
		return output, err
	}
	firstAttemptAt := startedAt
	if len(history) > 0 {
		firstAttemptAt = history[0].StartedAt
	}
	attemptDeadline, err := s.completion.retry.AttemptDeadline(
		startedAt, firstAttemptAt, claim.Delivery.DeadlineAt,
	)
	if err != nil {
		// 总体预算已耗尽：不追加 Attempt、不触发渠道；由租约恢复路径收敛状态。
		return output, err
	}
	// 渠道调用不得越过当前 Worker 的租约边界；租约到期后由恢复流程接管。
	if !isRecoveryUTC(claim.Lease.ExpiresAt) {
		return output, ErrRetryNotApplicable
	}
	attemptDeadline = earlierTime(attemptDeadline, claim.Lease.ExpiresAt)
	if !attemptDeadline.After(startedAt) {
		return output, ErrRetryNotApplicable
	}
	attempt := domain.DeliveryAttempt{
		AttemptID: attemptID, DeliveryID: claim.Delivery.DeliveryID,
		AttemptNo: int(claim.Delivery.AttemptCount + 1),
		WorkerID:  claim.Lease.WorkerID, FencingToken: claim.Lease.FencingToken,
		LeaseExpiresAt: claim.Lease.ExpiresAt, StartedAt: startedAt,
	}
	if err := attempt.Validate(); err != nil {
		return output, err
	}
	if err := s.attempts.StartWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		return output, err
	}

	// StartWithLease 可能消耗时间；必须在持久化成功后重新采样注入时钟，
	// 只把此刻到绝对截止时间的剩余预算交给使用真实时钟的 Go Context。
	sendStartedAt, err := s.executionTime()
	if err != nil {
		return output, err
	}
	sendTimeout := attemptDeadline.Sub(sendStartedAt)
	if sendTimeout <= 0 {
		// Attempt 已持久化，但渠道调用尚未开始；用明确 not_sent 事实收敛，
		// 避免把已知未发送误留成运行中 Attempt 并在恢复时升级为 unknown。
		notSent := domain.ChannelSendResult{
			Outcome:   domain.ChannelOutcomeNotSent,
			ErrorCode: "attempt_deadline_elapsed_before_send",
		}
		finishedAt, timeErr := s.executionTime()
		if timeErr != nil {
			return output, timeErr
		}
		if completeErr := s.completion.Complete(
			ctx, claim, attempt, preflight.ChannelSnapshot, notSent,
			finishedAt, randomUnit, evidenceRef,
		); completeErr != nil {
			return output, errors.Join(ErrRetryNotApplicable, completeErr)
		}
		output.Action = DeliveryPreflightReady
		output.ReasonCode = "attempt_deadline_elapsed_before_send"
		output.AttemptID = attemptID
		output.ChannelResult = &notSent
		return output, nil
	}
	sendCtx, cancelSend := context.WithTimeout(ctx, sendTimeout)
	result, sendErr := preflight.Channel.Send(sendCtx, request)
	cancelSend()
	output.AttemptID = attemptID
	output.ChannelResult = &result
	finishedAt, timeErr := s.executionTime()
	if timeErr != nil {
		return output, errors.Join(sendErr, timeErr)
	}
	if err := s.completion.Complete(
		ctx, claim, attempt, preflight.ChannelSnapshot, result,
		finishedAt, randomUnit, evidenceRef,
	); err != nil {
		return output, errors.Join(sendErr, err)
	}
	output.Action = DeliveryPreflightReady
	return output, sendErr
}

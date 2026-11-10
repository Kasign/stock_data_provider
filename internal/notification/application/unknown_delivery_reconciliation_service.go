package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrUnknownDeliverySnapshotUnavailable = errors.New("notification: unknown delivery snapshot unavailable")
	ErrUnknownDeliverySnapshotNotConsistent = errors.New("notification: unknown delivery snapshot is not consistent")
)

// verifiedUnknownDeliveryEvidence 只能由本包的一致性快照编排在 Registry 查询成功后构造。
type verifiedUnknownDeliveryEvidence struct {
	evidence        UnknownDeliveryEvidence
	deliveryVersion int64
	attemptID       string
	channel         domain.ChannelConfigSnapshot
}

func (v verifiedUnknownDeliveryEvidence) matches(
	delivery domain.DeliveryRecord,
	channel domain.ChannelConfigSnapshot,
	attempts []domain.DeliveryAttempt,
) bool {
	if v.deliveryVersion != delivery.Version ||
		v.attemptID == "" ||
		!equalChannelSnapshot(v.channel, channel) ||
		v.evidence.DeliveryID != delivery.DeliveryID ||
		v.evidence.AttemptID != v.attemptID ||
		len(attempts) != delivery.AttemptCount ||
		len(attempts) == 0 {
		return false
	}
	latest := attempts[len(attempts)-1]
	return latest.AttemptID == v.attemptID &&
		latest.Outcome != nil && *latest.Outcome == domain.DeliveryAttemptAmbiguous &&
		latest.FinishedAt != nil
}

// UnknownDeliveryReconciliationService 将一致性读取、Registry 查询、规则评估和 CAS 收敛固定为单一入口。
type UnknownDeliveryReconciliationService struct {
	reader     DeliveryConsistencySnapshotReader
	registry   *ChannelRegistry
	repository UnknownDeliveryReconciliationRepository
	policy     RetryPolicy
}

func NewUnknownDeliveryReconciliationService(
	reader DeliveryConsistencySnapshotReader,
	registry *ChannelRegistry,
	repository UnknownDeliveryReconciliationRepository,
	policy RetryPolicy,
) (*UnknownDeliveryReconciliationService, error) {
	if reader == nil || registry == nil || repository == nil {
		return nil, ErrUnknownDeliverySnapshotUnavailable
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &UnknownDeliveryReconciliationService{
		reader: reader, registry: registry, repository: repository, policy: policy,
	}, nil
}

// Reconcile 必须从数据库单次一致性读取开始；查询失败或快照不可比较时不执行写操作。
func (s *UnknownDeliveryReconciliationService) Reconcile(
	ctx context.Context,
	deliveryID string,
	now time.Time,
	randomUnit float64,
) (UnknownReconciliationAssessment, *RetryDecision, error) {
	if s == nil || s.reader == nil || s.registry == nil || s.repository == nil ||
		ctx == nil || deliveryID == "" || !isRecoveryUTC(now) {
		return UnknownReconciliationAssessment{}, nil, ErrUnknownDeliverySnapshotUnavailable
	}
	if err := ctx.Err(); err != nil {
		return UnknownReconciliationAssessment{}, nil, err
	}
	snapshot, found, err := s.reader.ReadDeliveryConsistencySnapshot(ctx, deliveryID)
	if err != nil {
		return UnknownReconciliationAssessment{}, nil, err
	}
	if !found || snapshot.Delivery.DeliveryID != deliveryID {
		return UnknownReconciliationAssessment{}, nil, ErrUnknownReconciliationSnapshotUnavailable
	}
	consistency := AssessDeliveryConsistency(DeliveryConsistencyInput{
		Delivery: snapshot.Delivery, Attempts: snapshot.Attempts, AttemptsComparable: true,
		StateEvents: snapshot.StateEvents, StateEventsComparable: true,
	})
	if consistency.Status != DeliveryConsistencyPass ||
		snapshot.Delivery.Status != domain.DeliveryUnknown {
		return UnknownReconciliationAssessment{}, nil, ErrUnknownDeliverySnapshotNotConsistent
	}

	channel, err := s.registry.snapshotForVersion(snapshot.Delivery.ChannelRef, snapshot.Delivery.ChannelConfigVersion)
	if err != nil {
		return UnknownReconciliationAssessment{}, nil, err
	}
	if len(snapshot.Attempts) == 0 {
		return UnknownReconciliationAssessment{}, nil, ErrUnknownReconciliationNotApplicable
	}
	latest := snapshot.Attempts[len(snapshot.Attempts)-1]
	evidence, err := s.registry.LookupUnknownDelivery(ctx, snapshot.Delivery, channel, latest)
	if err != nil {
		return UnknownReconciliationAssessment{}, nil, err
	}
	verified := verifiedUnknownDeliveryEvidence{
		evidence: evidence, deliveryVersion: snapshot.Delivery.Version,
		attemptID: latest.AttemptID, channel: cloneChannelSnapshot(channel),
	}
	if err := ctx.Err(); err != nil {
		return UnknownReconciliationAssessment{}, nil, err
	}
	return reconcileUnknownDelivery(
		ctx, s.repository, s.policy, snapshot.Delivery, channel,
		snapshot.Attempts, verified, now, randomUnit,
	)
}

func (r *ChannelRegistry) snapshotForVersion(channelRef string, configVersion int64) (domain.ChannelConfigSnapshot, error) {
	if r == nil {
		return domain.ChannelConfigSnapshot{}, ErrChannelAdapterUnavailable
	}
	r.mu.RLock()
	registration, exists := r.registrations[channelKey{ref: channelRef, version: configVersion}]
	r.mu.RUnlock()
	if !exists {
		return domain.ChannelConfigSnapshot{}, ErrChannelVersionNotFound
	}
	return cloneChannelSnapshot(registration.snapshot), nil
}

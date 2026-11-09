package app

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrNewsAuthorizationFingerprintInvalid  = errors.New("news.authorization.fingerprint_invalid")
	ErrNewsAuthorizationVerifierUnavailable = errors.New("news.authorization.verifier_unavailable")
	ErrNewsCurrentPolicyUnavailable         = errors.New("news.authorization.current_policy_unavailable")
	ErrNewsCurrentPolicyDenied              = errors.New("news.authorization.current_policy_denied")
)

// NewsAuthorizationFingerprintVerifier 是 GOV-CTR-006 的 News 侧窄端口。
// 实现必须委托项目唯一 JCS Runtime Verifier；News 不实现或复制 JCS。
type NewsAuthorizationFingerprintVerifier interface {
	VerifyNewsAuthorizationFingerprint(context.Context, domain.NewsAuthorizationBinding) error
}

// CurrentNewsAuthorizationPolicyResolver 必须从可信策略 Owner 读取当前完整策略集合，
// 并针对 Binding 中的 purpose/content level 计算每项 Allows。
type CurrentNewsAuthorizationPolicyResolver interface {
	ResolveCurrentNewsAuthorizationPolicies(context.Context, domain.NewsAuthorizationBinding) (domain.CurrentAuthorizationPolicySet, error)
}

// EvaluateNewsEvidenceReadAuthorization 执行内容披露前的授权预检。
// 它不读取或返回内容；最终查询必须在同一授权一致性边界内再次执行该检查并读取内容。
type EvaluateNewsEvidenceReadAuthorization struct {
	verifier NewsAuthorizationFingerprintVerifier
	policies CurrentNewsAuthorizationPolicyResolver
	now      func() time.Time
}

func NewEvaluateNewsEvidenceReadAuthorization(
	verifier NewsAuthorizationFingerprintVerifier,
	policies CurrentNewsAuthorizationPolicyResolver,
) (*EvaluateNewsEvidenceReadAuthorization, error) {
	if verifier == nil {
		return nil, ErrNewsAuthorizationVerifierUnavailable
	}
	if policies == nil {
		return nil, ErrNewsCurrentPolicyUnavailable
	}
	return &EvaluateNewsEvidenceReadAuthorization{
		verifier: verifier,
		policies: policies,
		now:      time.Now,
	}, nil
}

func (u *EvaluateNewsEvidenceReadAuthorization) Execute(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
	observation domain.Observation,
	version domain.NewsItemVersion,
	callerRef string,
	purposeCode string,
	contentLevel domain.AuthorizationContentLevel,
) error {
	if u == nil || u.verifier == nil || u.policies == nil || u.now == nil || ctx == nil ||
		domain.ValidateNewsAuthorizationBindingForEvidence(binding, observation, version) != nil ||
		binding.CallerRef != callerRef || binding.PurposeCode != purposeCode ||
		binding.RequestedContentLevel != contentLevel {
		return ErrInvalidAuthorizationSnapshot
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := u.now().UTC()
	snapshotDecision := domain.EvaluateNewsAuthorizationUse(binding, now)
	if !snapshotDecision.Allowed {
		return errors.Join(ErrNewsCurrentPolicyDenied, errors.New(snapshotDecision.ReasonCode))
	}
	if err := u.verifier.VerifyNewsAuthorizationFingerprint(ctx, binding); err != nil {
		return ErrNewsAuthorizationFingerprintInvalid
	}
	current, err := u.policies.ResolveCurrentNewsAuthorizationPolicies(ctx, binding)
	if err != nil {
		return ErrNewsCurrentPolicyUnavailable
	}
	decision := domain.EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, current, now)
	if !decision.Allowed {
		return errors.Join(ErrNewsCurrentPolicyDenied, errors.New(decision.ReasonCode))
	}
	return nil
}

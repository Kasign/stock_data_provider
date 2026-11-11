package domain

import "time"

// CurrentAuthorizationPolicy 是可信策略 Owner 对当前策略版本的实时解析结果。
// Allows 必须针对 Binding 中的用途与内容级别计算，不可仅由策略版本存在推导。
type CurrentAuthorizationPolicy struct {
	PolicyType    string
	PolicyRef     string
	PolicyVersion string
	Status        AuthorizationStatus
	EffectiveAt   time.Time
	ExpiresAt     *time.Time
	Allows        bool
}

// CurrentAuthorizationPolicySet 是一次读取时取得的完整当前策略集合。
type CurrentAuthorizationPolicySet struct {
	EvaluatedAt time.Time
	Policies    []CurrentAuthorizationPolicy
}

// EvaluateNewsAuthorizationAgainstCurrentPolicies 将不可变授权快照与当前策略版本逐项核对。
// 策略版本变化、撤销、失效、缺失或当前用途/内容级别不再允许时一律拒绝。
func EvaluateNewsAuthorizationAgainstCurrentPolicies(
	binding NewsAuthorizationBinding,
	current CurrentAuthorizationPolicySet,
	now time.Time,
) AuthorizationUseDecision {
	snapshotDecision := EvaluateNewsAuthorizationUse(binding, now)
	if !snapshotDecision.Allowed {
		return snapshotDecision
	}
	if now.IsZero() || current.EvaluatedAt.IsZero() || current.EvaluatedAt.After(now) ||
		current.Policies == nil || len(current.Policies) != len(binding.PolicySnapshot.Policies) {
		return AuthorizationUseDecision{ReasonCode: "news.authorization.current_policy_unavailable"}
	}

	byIdentity := make(map[string]CurrentAuthorizationPolicy, len(current.Policies))
	for _, policy := range current.Policies {
		if !canonicalOpaqueRef(policy.PolicyType) || !canonicalOpaqueRef(policy.PolicyRef) ||
			!canonicalOpaqueRef(policy.PolicyVersion) {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.current_policy_invalid"}
		}
		key := policy.PolicyType + "\x00" + policy.PolicyRef
		if _, exists := byIdentity[key]; exists {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.current_policy_invalid"}
		}
		byIdentity[key] = policy
	}

	for _, snapshotPolicy := range binding.PolicySnapshot.Policies {
		key := snapshotPolicy.PolicyType + "\x00" + snapshotPolicy.PolicyRef
		currentPolicy, exists := byIdentity[key]
		if !exists {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.current_policy_missing"}
		}
		if currentPolicy.PolicyVersion != snapshotPolicy.PolicyVersion {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.policy_version_changed"}
		}
		switch currentPolicy.Status {
		case AuthorizationRevoked:
			return AuthorizationUseDecision{ReasonCode: "news.authorization.policy_revoked"}
		case AuthorizationPending:
			return AuthorizationUseDecision{ReasonCode: "news.authorization.policy_pending"}
		case AuthorizationApproved, AuthorizationRestricted:
		default:
			return AuthorizationUseDecision{ReasonCode: "news.authorization.current_policy_status_unknown"}
		}
		if !currentPolicy.EffectiveAt.IsZero() && now.Before(currentPolicy.EffectiveAt) {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.policy_not_yet_effective"}
		}
		if currentPolicy.ExpiresAt != nil && !now.Before(*currentPolicy.ExpiresAt) {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.policy_expired"}
		}
		if !currentPolicy.Allows {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.current_policy_denied"}
		}
	}
	return AuthorizationUseDecision{Allowed: true}
}

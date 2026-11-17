package storage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var _ app.CurrentNewsAuthorizationPolicyResolver = (*MySQLRepository)(nil)

// ResolveCurrentNewsAuthorizationPolicies 读取 Binding 对应的完整当前策略集合。
// 该方法用于预检；最终正文读取必须使用带当前指针行锁的原子读取事务。
func (r *MySQLRepository) ResolveCurrentNewsAuthorizationPolicies(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
) (domain.CurrentAuthorizationPolicySet, error) {
	if r == nil || r.db == nil || ctx == nil ||
		domain.ValidateNewsAuthorizationBinding(binding) != nil {
		return domain.CurrentAuthorizationPolicySet{}, app.ErrNewsCurrentPolicyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.CurrentAuthorizationPolicySet{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.CurrentAuthorizationPolicySet{}, errors.Join(app.ErrNewsCurrentPolicyUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	loaded, err := loadCurrentNewsPolicyVersions(ctx, tx, binding, false)
	if err != nil {
		return domain.CurrentAuthorizationPolicySet{}, err
	}
	result, err := buildCurrentNewsAuthorizationPolicySet(binding, loaded, time.Now().UTC())
	if err != nil {
		return domain.CurrentAuthorizationPolicySet{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.CurrentAuthorizationPolicySet{}, errors.Join(app.ErrNewsCurrentPolicyUnavailable, err)
	}
	return result, nil
}

// loadCurrentNewsPolicyVersions 按 Binding 已验证的字典序逐行读取当前指针。
// lockPointers=true 时只锁当前指针行；不可变历史版本行不需要写锁。
func loadCurrentNewsPolicyVersions(
	ctx context.Context,
	tx *sql.Tx,
	binding domain.NewsAuthorizationBinding,
	lockPointers bool,
) (map[string]domain.NewsPolicyVersion, error) {
	loaded := make(map[string]domain.NewsPolicyVersion, len(binding.PolicySnapshot.Policies))
	for _, expected := range binding.PolicySnapshot.Policies {
		query := `SELECT policy_version FROM news_current_policy_versions
			WHERE policy_type = ? AND policy_ref = ?`
		if lockPointers {
			query += " FOR UPDATE"
		}
		var currentVersion string
		if err := tx.QueryRowContext(ctx, query, expected.PolicyType, expected.PolicyRef).Scan(&currentVersion); err != nil {
			return nil, errors.Join(app.ErrNewsCurrentPolicyUnavailable, err)
		}
		if currentVersion != expected.PolicyVersion {
			return nil, app.ErrNewsCurrentPolicyUnavailable
		}

		var value domain.NewsPolicyVersion
		var payload []byte
		var createdAt time.Time
		if err := tx.QueryRowContext(ctx, `SELECT policy_type, policy_ref, policy_version,
			payload_json, payload_hash, created_at
			FROM news_policy_versions
			WHERE policy_type = ? AND policy_ref = ? AND policy_version = ?`,
			expected.PolicyType, expected.PolicyRef, currentVersion,
		).Scan(&value.PolicyType, &value.PolicyRef, &value.PolicyVersion, &payload, &value.PayloadHash, &createdAt); err != nil {
			return nil, errors.Join(app.ErrNewsCurrentPolicyUnavailable, err)
		}
		value.Payload = append([]byte(nil), payload...)
		value.CreatedAt = createdAt.UTC()
		if value.PolicyType != expected.PolicyType || value.PolicyRef != expected.PolicyRef ||
			value.PolicyVersion != expected.PolicyVersion || domain.ValidateNewsPolicyVersion(value) != nil {
			return nil, app.ErrNewsCurrentPolicyUnavailable
		}
		sum := sha256.Sum256(value.Payload)
		actualHash := hex.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(actualHash), []byte(value.PayloadHash)) != 1 {
			return nil, app.ErrNewsCurrentPolicyUnavailable
		}
		loaded[value.PolicyType+"\x00"+value.PolicyRef] = value
	}
	if len(loaded) != len(binding.PolicySnapshot.Policies) {
		return nil, app.ErrNewsCurrentPolicyUnavailable
	}
	return loaded, nil
}

func buildCurrentNewsAuthorizationPolicySet(
	binding domain.NewsAuthorizationBinding,
	loaded map[string]domain.NewsPolicyVersion,
	evaluatedAt time.Time,
) (domain.CurrentAuthorizationPolicySet, error) {
	result := domain.CurrentAuthorizationPolicySet{
		EvaluatedAt: evaluatedAt.UTC(),
		Policies:    make([]domain.CurrentAuthorizationPolicy, 0, len(binding.PolicySnapshot.Policies)),
	}
	var source domain.SourcePolicy
	var endpoint *domain.EndpointPolicy
	sourceValue, ok := loaded[domain.NewsPolicyTypeSource+"\x00"+binding.SourceRef]
	if !ok || decodeCurrentPolicy(sourceValue.Payload, &source) != nil {
		return domain.CurrentAuthorizationPolicySet{}, app.ErrNewsCurrentPolicyUnavailable
	}
	if binding.EndpointRef.Value != nil {
		value, ok := loaded[domain.NewsPolicyTypeEndpoint+"\x00"+*binding.EndpointRef.Value]
		if !ok {
			return domain.CurrentAuthorizationPolicySet{}, app.ErrNewsCurrentPolicyUnavailable
		}
		endpoint = new(domain.EndpointPolicy)
		if decodeCurrentPolicy(value.Payload, endpoint) != nil {
			return domain.CurrentAuthorizationPolicySet{}, app.ErrNewsCurrentPolicyUnavailable
		}
	}

	for _, expected := range binding.PolicySnapshot.Policies {
		value := loaded[expected.PolicyType+"\x00"+expected.PolicyRef]
		current := domain.CurrentAuthorizationPolicy{
			PolicyType: value.PolicyType, PolicyRef: value.PolicyRef, PolicyVersion: value.PolicyVersion,
		}
		switch value.PolicyType {
		case domain.NewsPolicyTypeSource:
			current.Status = source.Status
			current.EffectiveAt = source.EffectiveAt
			current.ExpiresAt = clonePolicyTime(source.ExpiresAt)
			current.Allows = policyAllowsPurposeAndContent(source.AllowedPurposes, source.AllowedContentKinds, binding)
		case domain.NewsPolicyTypeEndpoint:
			current.Status = endpoint.Status
			if current.Status == "" {
				current.Status = source.Status
			}
			current.EffectiveAt = endpoint.EffectiveAt
			current.ExpiresAt = clonePolicyTime(endpoint.ExpiresAt)
			current.Allows = endpointAllowsPurposeAndContent(*endpoint, binding)
		case domain.NewsPolicyTypeProviderOutput:
			var policy domain.ProviderOutputPolicy
			if decodeCurrentPolicy(value.Payload, &policy) != nil {
				return domain.CurrentAuthorizationPolicySet{}, app.ErrNewsCurrentPolicyUnavailable
			}
			current.Status = policy.Status
			current.EffectiveAt = policy.EffectiveAt
			current.ExpiresAt = clonePolicyTime(policy.ExpiresAt)
			current.Allows = policy.AllowRequestProcessing &&
				policyAllowsPurposeAndContent(policy.AllowedPurposes, policy.AllowedContentKinds, binding)
		default:
			return domain.CurrentAuthorizationPolicySet{}, app.ErrNewsCurrentPolicyUnavailable
		}
		result.Policies = append(result.Policies, current)
	}
	return result, nil
}

func decodeCurrentPolicy(payload []byte, target any) error {
	return json.Unmarshal(payload, target)
}

func clonePolicyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func policyAllowsPurposeAndContent(
	purposes []domain.Purpose,
	contentKinds []domain.ContentKind,
	binding domain.NewsAuthorizationBinding,
) bool {
	purpose, ok := authorizationPurposeForCode(binding.PurposeCode)
	if !ok || !containsNewsPurpose(purposes, purpose) {
		return false
	}
	for _, required := range authorizationContentKinds(binding.RequestedContentLevel) {
		if containsNewsContentKind(contentKinds, required) {
			return true
		}
	}
	return false
}

func endpointAllowsPurposeAndContent(policy domain.EndpointPolicy, binding domain.NewsAuthorizationBinding) bool {
	if policy.AllowedPurposes != nil || policy.AllowedContentKinds != nil {
		purpose, ok := authorizationPurposeForCode(binding.PurposeCode)
		if !ok {
			return false
		}
		if policy.AllowedPurposes != nil && !containsNewsPurpose(policy.AllowedPurposes, purpose) {
			return false
		}
		if policy.AllowedContentKinds != nil {
			matched := false
			for _, required := range authorizationContentKinds(binding.RequestedContentLevel) {
				if containsNewsContentKind(policy.AllowedContentKinds, required) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

func authorizationPurposeForCode(value string) (domain.Purpose, bool) {
	switch value {
	case "research_analysis":
		return domain.PurposeInternalPersonalResearch, true
	case "risk_review":
		// risk_review 是受控调用场景显式选择的用途，对应领域内部业务用途；
		// 不得据此把通用 internal_business 自动推断为 risk_review。
		return domain.PurposeInternalBusiness, true
	case "user_display":
		return domain.PurposePublicDisplay, true
	default:
		return "", false
	}
}

func authorizationContentKinds(level domain.AuthorizationContentLevel) []domain.ContentKind {
	switch level {
	case domain.AuthorizationContentMetadata:
		return []domain.ContentKind{domain.ContentMetadata, domain.ContentTitle}
	case domain.AuthorizationContentSummary:
		return []domain.ContentKind{domain.ContentAbstract}
	case domain.AuthorizationContentSnippet:
		return []domain.ContentKind{domain.ContentSnippet}
	case domain.AuthorizationContentFull:
		return []domain.ContentKind{domain.ContentFullText}
	default:
		return nil
	}
}

func containsNewsPurpose(values []domain.Purpose, target domain.Purpose) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsNewsContentKind(values []domain.ContentKind, target domain.ContentKind) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

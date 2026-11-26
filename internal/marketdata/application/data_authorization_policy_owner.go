package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/platform/observability"
	"example.com/stock-analysis-flow/internal/platform/security"
)

var (
	ErrDataPolicyOwnerUnavailable = errors.New("data authorization policy owner unavailable")
	ErrDataPolicyNotPublished     = errors.New("data authorization policy not published")
	ErrDataPolicyOwnerDenied      = errors.New("data authorization policy owner denied")
	ErrDataPolicyVersionConflict  = errors.New("data authorization policy version conflict")
	ErrDataPolicyInvalidChange    = errors.New("data authorization policy change invalid")
)

// DataAuthorizationPolicyOwner 是 Data 策略的唯一发布边界。
// 策略内容由 Owner Session 明确提交；调用方不能指定策略 Owner 或绕过版本比较。
type DataAuthorizationPolicyOwner struct {
	repository app.DataAuthorizationPolicyRepository
	now        func() time.Time
}

func NewDataAuthorizationPolicyOwner(repository app.DataAuthorizationPolicyRepository) (*DataAuthorizationPolicyOwner, error) {
	if repository == nil {
		return nil, ErrDataPolicyOwnerUnavailable
	}
	return &DataAuthorizationPolicyOwner{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

// Current 返回日线数据类的当前已发布策略。
func (owner *DataAuthorizationPolicyOwner) Current(ctx context.Context) (authorization.CurrentPolicy, error) {
	return owner.CurrentFor(ctx, authorization.DailyBarsV2PolicyRef)
}

// CurrentFor 返回指定数据类的当前已发布策略。未发布或存储不可用时均返回错误，不合成默认策略。
func (owner *DataAuthorizationPolicyOwner) CurrentFor(ctx context.Context, policyRef string) (authorization.CurrentPolicy, error) {
	if owner == nil || owner.repository == nil || ctx == nil || !authorization.ValidDataPolicyRef(policyRef) {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	if _, err := requireDataPolicyOwnerSession(ctx); err != nil {
		return authorization.CurrentPolicy{}, err
	}
	current, err := owner.resolveCurrent(ctx, policyRef)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return authorization.CurrentPolicy{}, ctxErr
		}
		if errors.Is(err, app.ErrDataAuthorizationPolicyNotPublished) {
			return authorization.CurrentPolicy{}, ErrDataPolicyNotPublished
		}
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	if _, err := authorization.DecodePolicyVersionFor(current.Version, policyRef); err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	if err := authorization.ValidatePolicy(current.Policy); err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	return cloneCurrentDataPolicy(current), nil
}

// Publish 发布日线数据类的一个新的不可变策略版本。
func (owner *DataAuthorizationPolicyOwner) Publish(
	ctx context.Context,
	expectedCurrentVersion, newVersion string,
	policy authorization.DataAccessPolicy,
) (authorization.CurrentPolicy, error) {
	return owner.PublishFor(ctx, authorization.DailyBarsV2PolicyRef, expectedCurrentVersion, newVersion, policy)
}

// PublishFor 发布指定数据类的一个新的不可变策略版本。
// expectedCurrentVersion 为空仅允许首次发布；非空时必须精确匹配当前版本。
//
// 不同数据类的版本序列彼此独立：日线的版本号不能用于交易事实，反之亦然。
func (owner *DataAuthorizationPolicyOwner) PublishFor(
	ctx context.Context,
	policyRef, expectedCurrentVersion, newVersion string,
	policy authorization.DataAccessPolicy,
) (authorization.CurrentPolicy, error) {
	if owner == nil || owner.repository == nil || ctx == nil || !authorization.ValidDataPolicyRef(policyRef) ||
		!validDataPolicyVersion(newVersion) ||
		(expectedCurrentVersion != "" && !validDataPolicyVersion(expectedCurrentVersion)) ||
		newVersion == expectedCurrentVersion {
		return authorization.CurrentPolicy{}, ErrDataPolicyInvalidChange
	}
	principal, err := requireDataPolicyOwnerSession(ctx)
	if err != nil {
		return authorization.CurrentPolicy{}, err
	}
	if err := ctx.Err(); err != nil {
		return authorization.CurrentPolicy{}, err
	}
	if err := authorization.ValidatePolicy(policy); err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyInvalidChange
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyInvalidChange
	}
	createdAt := owner.now()
	if createdAt.IsZero() || createdAt.Location() != time.UTC {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	sum := sha256.Sum256(payload)
	version := authorization.PolicyVersion{
		PolicyType:    authorization.DataAccessPolicyType,
		PolicyRef:     policyRef,
		PolicyVersion: newVersion,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(sum[:]),
		CreatedAt:     createdAt,
	}
	publishedPolicy, err := authorization.DecodePolicyVersionFor(version, policyRef)
	if err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyInvalidChange
	}
	requestID := observability.FromContext(ctx)
	if !observability.ValidRequestID(requestID) {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	audit := app.DataAuthorizationPolicyPublicationAudit{
		ActorRef:   "principal:" + principal.Subject,
		RequestID:  requestID,
		OccurredAt: createdAt,
	}
	if err := owner.repository.PublishDataAuthorizationPolicyVersion(ctx, expectedCurrentVersion, version, audit); err != nil {
		if errors.Is(err, app.ErrDataAuthorizationPolicyVersionConflict) {
			return authorization.CurrentPolicy{}, ErrDataPolicyVersionConflict
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return authorization.CurrentPolicy{}, ctxErr
		}
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	// 返回本次 CAS 实际发布的不可变版本；不能重新读取 current pointer，
	// 否则并发发布下一版本时会把已成功的本次发布误报为失败。
	return cloneCurrentDataPolicy(authorization.CurrentPolicy{
		Version: version,
		Policy:  publishedPolicy,
	}), nil
}

// resolveCurrent 按数据类解析当前策略：日线走既有方法以保持实现兼容，
// 其他数据类必须由仓储显式支持，否则失败而不是回落到日线策略。
func (owner *DataAuthorizationPolicyOwner) resolveCurrent(ctx context.Context, policyRef string) (authorization.CurrentPolicy, error) {
	if policyRef == authorization.DailyBarsV2PolicyRef {
		return owner.repository.ResolveCurrentDataAuthorizationPolicy(ctx)
	}
	resolver, ok := owner.repository.(app.DataAuthorizationPolicyRefResolver)
	if !ok {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	return resolver.ResolveCurrentDataAuthorizationPolicyByRef(ctx, policyRef)
}

func requireDataPolicyOwnerSession(ctx context.Context) (security.Principal, error) {
	principal, ok := security.PrincipalFromContextValue(ctx)
	if !ok || principal.Kind != security.IdentityOwner ||
		principal.AuthMethod != security.AuthMethodOwnerSession || principal.Subject != "owner" {
		return security.Principal{}, ErrDataPolicyOwnerDenied
	}
	return principal, nil
}

func validDataPolicyVersion(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

func cloneCurrentDataPolicy(value authorization.CurrentPolicy) authorization.CurrentPolicy {
	value.Version.Payload = append([]byte(nil), value.Version.Payload...)
	value.Policy.AllowedSubjects = append([]string(nil), value.Policy.AllowedSubjects...)
	value.Policy.AllowedPurposes = append([]string(nil), value.Policy.AllowedPurposes...)
	value.Policy.RequiredScopes = append([]string(nil), value.Policy.RequiredScopes...)
	if value.Policy.ExpiresAt != nil {
		expiresAt := *value.Policy.ExpiresAt
		value.Policy.ExpiresAt = &expiresAt
	}
	return value
}

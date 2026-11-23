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

// Current 返回当前已发布策略。未发布或存储不可用时均返回错误，不合成默认策略。
func (owner *DataAuthorizationPolicyOwner) Current(ctx context.Context) (authorization.CurrentPolicy, error) {
	if owner == nil || owner.repository == nil || ctx == nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	if err := requireDataPolicyOwnerSession(ctx); err != nil {
		return authorization.CurrentPolicy{}, err
	}
	current, err := owner.repository.ResolveCurrentDataAuthorizationPolicy(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return authorization.CurrentPolicy{}, ctxErr
		}
		if errors.Is(err, app.ErrDataAuthorizationPolicyNotPublished) {
			return authorization.CurrentPolicy{}, ErrDataPolicyNotPublished
		}
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	if _, err := authorization.DecodePolicyVersion(current.Version); err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	if err := authorization.ValidatePolicy(current.Policy); err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyOwnerUnavailable
	}
	return cloneCurrentDataPolicy(current), nil
}

// Publish 发布一个新的不可变策略版本。
// expectedCurrentVersion 为空仅允许首次发布；非空时必须精确匹配当前版本。
func (owner *DataAuthorizationPolicyOwner) Publish(
	ctx context.Context,
	expectedCurrentVersion, newVersion string,
	policy authorization.DataAccessPolicy,
) (authorization.CurrentPolicy, error) {
	if owner == nil || owner.repository == nil || ctx == nil ||
		!validDataPolicyVersion(newVersion) ||
		(expectedCurrentVersion != "" && !validDataPolicyVersion(expectedCurrentVersion)) ||
		newVersion == expectedCurrentVersion {
		return authorization.CurrentPolicy{}, ErrDataPolicyInvalidChange
	}
	if err := requireDataPolicyOwnerSession(ctx); err != nil {
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
		PolicyRef:     authorization.DailyBarsV2PolicyRef,
		PolicyVersion: newVersion,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(sum[:]),
		CreatedAt:     createdAt,
	}
	publishedPolicy, err := authorization.DecodePolicyVersion(version)
	if err != nil {
		return authorization.CurrentPolicy{}, ErrDataPolicyInvalidChange
	}
	if err := owner.repository.PublishDataAuthorizationPolicyVersion(ctx, expectedCurrentVersion, version); err != nil {
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

func requireDataPolicyOwnerSession(ctx context.Context) error {
	principal, ok := security.PrincipalFromContextValue(ctx)
	if !ok || principal.Kind != security.IdentityOwner ||
		principal.AuthMethod != security.AuthMethodOwnerSession || principal.Subject != "owner" {
		return ErrDataPolicyOwnerDenied
	}
	return nil
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

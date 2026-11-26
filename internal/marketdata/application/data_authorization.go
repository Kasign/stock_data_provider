package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	platformauthorization "example.com/stock-analysis-flow/internal/platform/authorization"
	"example.com/stock-analysis-flow/internal/platform/security"
)

var (
	ErrDataAuthorizationUnavailable = errors.New("data authorization application unavailable")
	ErrDataAuthorizationDenied      = errors.New("data authorization denied")
	ErrDataAuthorizationInvalid     = errors.New("data authorization reference invalid")
)

// Field order is part of the persisted contract: the repository re-marshals
// these structures and requires byte-for-byte canonical agreement before an
// authorization decision may be stored. Keep this order aligned with
// storedDataAuthorizationRef and GOV-CTR-006-data-allowed.golden.json.
type dataAuthorizationRef struct {
	AuthorizationRef    string   `json:"authorization_ref"`
	PolicyOwner         string   `json:"policy_owner"`
	PolicyVersion       string   `json:"policy_version"`
	Decision            string   `json:"decision"`
	PurposeCode         string   `json:"purpose_code"`
	ContentLevel        string   `json:"content_level"`
	SubjectRef          string   `json:"subject_ref"`
	ResourceRef         string   `json:"resource_ref"`
	DecidedAt           string   `json:"decided_at"`
	ExpiresAt           *string  `json:"expires_at"`
	Conditions          []string `json:"conditions"`
	DecisionFingerprint string   `json:"decision_fingerprint"`
	SchemaVersion       string   `json:"schema_version"`
}

// The nested policy snapshot is re-marshalled from a generic map by the
// repository, so its keys must already be in lexicographic order.
type dataAuthorizationPolicySnapshot struct {
	Policies      []dataAuthorizationPolicyRecord `json:"policies"`
	SchemaVersion string                          `json:"schema_version"`
	SnapshotRef   string                          `json:"snapshot_ref"`
}

type dataAuthorizationPolicyRecord struct {
	PolicyRef     string `json:"policy_ref"`
	PolicyType    string `json:"policy_type"`
	PolicyVersion string `json:"policy_version"`
}

// DataAuthorizationOwner 是 Data Owner 唯一的授权签发与生产读取边界。
// Principal 只从已认证的 context 解析；调用方不能提交 subject 或 scopes。
type DataAuthorizationOwner struct {
	policies  app.DataAuthorizationPolicyRepository
	decisions app.DataAuthorizationDecisionStore
	now       func() time.Time
}

func NewDataAuthorizationOwner(
	policies app.DataAuthorizationPolicyRepository,
	decisions app.DataAuthorizationDecisionStore,
) (*DataAuthorizationOwner, error) {
	if policies == nil || decisions == nil {
		return nil, ErrDataAuthorizationUnavailable
	}
	return &DataAuthorizationOwner{policies: policies, decisions: decisions, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Issue 为一个已持久化 Snapshot 签发日线数据类授权。策略缺失、主体缺失或权限不足均拒绝。
func (o *DataAuthorizationOwner) Issue(ctx context.Context, snapshotRef, purpose string) (string, error) {
	return o.IssueFor(ctx, snapshotRef, purpose, authorization.DailyBarsV2PolicyRef)
}

// IssueFor 为指定数据类的已持久化 Snapshot 签发授权。
//
// 不同数据类必须各自发布策略：该数据类策略缺失或仓储不支持按数据类解析时直接失败，
// 绝不回落到日线策略——否则日线访问权限会被静默扩展成其他数据类的访问权限。
func (o *DataAuthorizationOwner) IssueFor(ctx context.Context, snapshotRef, purpose, policyRef string) (string, error) {
	if o == nil || o.policies == nil || o.decisions == nil || ctx == nil ||
		!authorization.ValidDataPolicyRef(policyRef) {
		return "", ErrDataAuthorizationUnavailable
	}
	if !validDataSnapshotRef(snapshotRef) {
		return "", ErrDataAuthorizationInvalid
	}
	principal, subjectRef, scopes, err := trustedDataPrincipal(ctx)
	if err != nil {
		return "", err
	}
	_ = principal
	current, err := o.resolveCurrentPolicy(ctx, policyRef)
	if err != nil {
		return "", fmt.Errorf("%w: current policy unavailable: %v", ErrDataAuthorizationUnavailable, err)
	}
	// 仓储返回的策略必须真的是所请求的数据类：否则决策会把日线策略绑到别的数据类上，
	// 虽然读取端还会再拒绝，但决策行本身已经是错的，必须在签发阶段就失败。
	if current.Version.PolicyRef != policyRef {
		return "", fmt.Errorf("%w: resolved policy class does not match the requested data class", ErrDataAuthorizationUnavailable)
	}
	now := o.now().UTC()
	if err := authorization.Evaluate(current.Policy, authorization.AccessRequest{
		SubjectRef: subjectRef, PurposeCode: purpose, Scopes: scopes, Now: now,
	}); err != nil {
		return "", fmt.Errorf("%w: %v", ErrDataAuthorizationDenied, err)
	}

	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("%w: authorization reference generation failed", ErrDataAuthorizationUnavailable)
	}
	authorizationRef := "data_authorization:" + base64.RawURLEncoding.EncodeToString(random)
	snapshot := dataAuthorizationPolicySnapshot{
		SnapshotRef: current.Version.PolicyVersion,
		Policies: []dataAuthorizationPolicyRecord{{
			PolicyType: current.Version.PolicyType, PolicyRef: current.Version.PolicyRef,
			PolicyVersion: current.Version.PolicyVersion,
		}},
		SchemaVersion: "1.0.0",
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return "", ErrDataAuthorizationUnavailable
	}
	ref := dataAuthorizationRef{
		SchemaVersion: "1.0.0", AuthorizationRef: authorizationRef,
		PolicyOwner: "data", PolicyVersion: current.Version.PolicyVersion,
		Decision: "allowed", PurposeCode: purpose, ContentLevel: "full",
		SubjectRef: subjectRef, ResourceRef: snapshotRef,
		DecidedAt: now.Format(time.RFC3339Nano), ExpiresAt: nil, Conditions: []string{},
	}
	if current.Policy.ExpiresAt != nil {
		expiry := current.Policy.ExpiresAt.UTC().Format(time.RFC3339Nano)
		ref.ExpiresAt = &expiry
	}
	unsigned, err := json.Marshal(ref)
	if err != nil {
		return "", ErrDataAuthorizationUnavailable
	}
	ref.DecisionFingerprint, err = platformauthorization.Fingerprint(unsigned, snapshotJSON)
	if err != nil {
		return "", fmt.Errorf("%w: fingerprint creation failed: %v", ErrDataAuthorizationUnavailable, err)
	}
	authJSON, err := json.Marshal(ref)
	if err != nil {
		return "", ErrDataAuthorizationUnavailable
	}
	if err := o.decisions.AppendDataAuthorizationDecision(ctx, app.DataAuthorizationDecision{
		AuthorizationRef: authJSON, PolicySnapshot: snapshotJSON,
	}); err != nil {
		return "", fmt.Errorf("%w: persist decision: %v", ErrDataAuthorizationUnavailable, err)
	}
	return authorizationRef, nil
}

// AuthorizeRead 在返回数据前重新检查当前策略、调用主体、资源、用途、有效期和撤销状态。
// Payload 读取由调用方在成功后执行；后续 Snapshot Reader 应将这两个步骤放入同一事务边界。
func (o *DataAuthorizationOwner) AuthorizeRead(ctx context.Context, authorizationRef, snapshotRef, purpose string) error {
	return o.AuthorizeReadFor(ctx, authorizationRef, snapshotRef, purpose, authorization.DailyBarsV2PolicyRef)
}

// AuthorizeReadFor 按指定数据类授权复核读取权限。
func (o *DataAuthorizationOwner) AuthorizeReadFor(ctx context.Context, authorizationRef, snapshotRef, purpose, policyRef string) error {
	if o == nil || o.policies == nil || o.decisions == nil || ctx == nil ||
		!authorization.ValidDataPolicyRef(policyRef) {
		return ErrDataAuthorizationUnavailable
	}
	if strings.TrimSpace(authorizationRef) == "" || !validDataSnapshotRef(snapshotRef) {
		return ErrDataAuthorizationInvalid
	}
	_, subjectRef, scopes, err := trustedDataPrincipal(ctx)
	if err != nil {
		return err
	}
	decision, err := o.decisions.GetDataAuthorizationDecision(ctx, authorizationRef)
	if err != nil {
		return fmt.Errorf("%w: decision lookup failed: %v", ErrDataAuthorizationDenied, err)
	}
	var ref dataAuthorizationRef
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return ErrDataAuthorizationInvalid
	}
	if ref.SchemaVersion != "1.0.0" || ref.AuthorizationRef != authorizationRef ||
		ref.PolicyOwner != "data" || ref.Decision != "allowed" || ref.ContentLevel != "full" ||
		ref.SubjectRef != subjectRef || ref.ResourceRef != snapshotRef || ref.PurposeCode != purpose ||
		len(ref.Conditions) != 0 {
		return ErrDataAuthorizationDenied
	}
	now := o.now().UTC()
	if ref.ExpiresAt != nil {
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, *ref.ExpiresAt)
		if parseErr != nil || !now.Before(expiresAt) {
			return ErrDataAuthorizationDenied
		}
	}
	current, err := o.resolveCurrentPolicy(ctx, policyRef)
	if err != nil {
		return fmt.Errorf("%w: current policy unavailable: %v", ErrDataAuthorizationUnavailable, err)
	}
	if current.Version.PolicyRef != policyRef || current.Version.PolicyVersion != ref.PolicyVersion {
		return ErrDataAuthorizationDenied
	}
	if err := authorization.Evaluate(current.Policy, authorization.AccessRequest{
		SubjectRef: subjectRef, PurposeCode: purpose, Scopes: scopes, Now: now,
	}); err != nil {
		return fmt.Errorf("%w: current policy denied: %v", ErrDataAuthorizationDenied, err)
	}
	return nil
}

// resolveCurrentPolicy 按数据类解析当前策略。
//
// 日线数据类继续走既有方法，保持所有现有仓储实现的行为不变；
// 其他数据类必须由仓储显式实现按引用解析，否则失败——不允许回落到日线策略。
func (o *DataAuthorizationOwner) resolveCurrentPolicy(ctx context.Context, policyRef string) (authorization.CurrentPolicy, error) {
	if policyRef == authorization.DailyBarsV2PolicyRef {
		return o.policies.ResolveCurrentDataAuthorizationPolicy(ctx)
	}
	resolver, ok := o.policies.(app.DataAuthorizationPolicyRefResolver)
	if !ok {
		return authorization.CurrentPolicy{}, ErrDataAuthorizationUnavailable
	}
	return resolver.ResolveCurrentDataAuthorizationPolicyByRef(ctx, policyRef)
}

func trustedDataPrincipal(ctx context.Context) (security.Principal, string, []string, error) {
	principal, ok := security.PrincipalFromContextValue(ctx)
	if !ok || principal.Subject == "" {
		return security.Principal{}, "", nil, ErrDataAuthorizationDenied
	}
	var subjectRef string
	switch principal.Kind {
	case security.IdentityOwner:
		if principal.Subject != "owner" ||
			(principal.AuthMethod != security.AuthMethodOwnerBearer && principal.AuthMethod != security.AuthMethodOwnerSession) {
			return security.Principal{}, "", nil, ErrDataAuthorizationDenied
		}
		subjectRef = "principal:owner"
	case security.IdentityService:
		if principal.AuthMethod != security.AuthMethodServiceBearer {
			return security.Principal{}, "", nil, ErrDataAuthorizationDenied
		}
		subjectRef = "principal:service:" + principal.Subject
	default:
		return security.Principal{}, "", nil, ErrDataAuthorizationDenied
	}
	scopes := append([]string(nil), principal.Scopes...)
	sort.Strings(scopes)
	for index := 1; index < len(scopes); index++ {
		if scopes[index] == scopes[index-1] {
			return security.Principal{}, "", nil, ErrDataAuthorizationDenied
		}
	}
	return principal, subjectRef, scopes, nil
}

func validDataSnapshotRef(value string) bool {
	const prefix = "data_snapshot:sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	for _, char := range value[len(prefix):] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

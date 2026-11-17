// Package application exposes Quant use cases to the Server and Researchflow.
package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
)

const (
	quantPurposeCode  = "quant_analysis"
	quantContentLevel = "full"
)

var (
	ErrInvalidGetResultRequest  = errors.New("invalid quant get-result request")
	ErrResultUnavailable        = errors.New("quant result unavailable")
	ErrInvalidAuthorization     = errors.New("quant authorization proof is invalid")
	ErrInvalidStoredRecord      = errors.New("quant stored result is invalid")
	ErrAuditUnavailable         = errors.New("quant result access audit unavailable")
	ErrAccessDenied             = errors.New("quant result access denied")
	ErrAuthorizationUnavailable = errors.New("quant authorization unavailable")
)

// Principal 由 Server 的认证上下文解析器产生；不得从 GetResult 请求体或普通 Header 构造。
type Principal struct {
	SubjectRef string
}

// PrincipalResolver 从可信调用上下文解析服务主体。
type PrincipalResolver interface {
	Resolve(ctx context.Context) (Principal, error)
}

// PrincipalResolverFunc 便于 Server 组合根注入可信主体解析逻辑。
type PrincipalResolverFunc func(context.Context) (Principal, error)

func (resolve PrincipalResolverFunc) Resolve(ctx context.Context) (Principal, error) {
	return resolve(ctx)
}

// ReadRequest 是授权读取端口的内部命令，不是跨模块公开 DTO。
type ReadRequest struct {
	SubjectRef   string
	ResourceRef  string
	PurposeCode  string
	ContentLevel string
	At           time.Time
}

// AuthorizationRef 是 GOV-CTR-006 的 Quant Owner 授权证明。
type AuthorizationRef struct {
	SchemaVersion       string   `json:"schema_version"`
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
	ExpiresAtPresent    bool     `json:"-"`
	Conditions          []string `json:"conditions"`
	DecisionFingerprint string   `json:"decision_fingerprint"`
}

// PolicyVersion 是策略快照中一条实际生效的策略版本。
// UnmarshalJSON 保留 expires_at 必填但可为 null 的存在性语义，避免把字段缺失误读为永久授权。
func (ref *AuthorizationRef) UnmarshalJSON(data []byte) error {
	if _, err := manifest.FingerprintPayload(data); err != nil {
		return ErrInvalidAuthorization
	}
	type authorizationRefAlias AuthorizationRef
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded authorizationRefAlias
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidAuthorization
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, exists := fields["expires_at"]; !exists {
		return ErrInvalidAuthorization
	}
	*ref = AuthorizationRef(decoded)
	ref.ExpiresAtPresent = true
	return nil
}

type PolicyVersion struct {
	PolicyType    string `json:"policy_type"`
	PolicyRef     string `json:"policy_ref"`
	PolicyVersion string `json:"policy_version"`
}

// PolicySnapshot 是 GOV-CTR-006 的不可变策略组合快照。
type PolicySnapshot struct {
	SchemaVersion string          `json:"schema_version"`
	SnapshotRef   string          `json:"snapshot_ref"`
	Policies      []PolicyVersion `json:"policies"`
}

// UnmarshalJSON 拒绝策略快照中的未知字段，避免忽略额外策略元数据。
func (snapshot *PolicySnapshot) UnmarshalJSON(data []byte) error {
	if _, err := manifest.FingerprintPayload(data); err != nil {
		return ErrInvalidAuthorization
	}
	type policySnapshotAlias PolicySnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded policySnapshotAlias
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidAuthorization
	}
	*snapshot = PolicySnapshot(decoded)
	return nil
}

// AuthorizedRecord 必须由结果 Owner 的授权读取端口在同一一致性边界内返回。
type AuthorizedRecord struct {
	Record           result.Record
	AuthorizationRef AuthorizationRef
	PolicySnapshot   PolicySnapshot
}

// AccessAuditEvent 只记录授权读取元数据，不包含结果 Payload 或受限内容。
type AccessAuditEvent struct {
	SubjectRef       string
	ResourceRef      string
	PurposeCode      string
	Decision         string
	ReasonCode       string
	AuthorizationRef string
	OccurredAt       time.Time
}

// AccessAuditor 持久化每次已识别主体的结果读取决策；写入失败时 Application 必须拒绝释放结果。
type AccessAuditor interface {
	RecordResultAccess(ctx context.Context, event AccessAuditEvent) error
}

// AuthorizedResultReader 负责当前授权复核和结果读取。
// 实现必须在一个可证明的线性化/一致性边界内完成：读取当前策略快照、判断主体/资源/用途/级别、读取不可变记录。
// 若授权策略在检查与读取间变化，或无法证明二者使用同一策略版本，必须拒绝且不得返回记录。
// 该接口不得以历史 allowed 缓存、authorization_ref 字符串或 expires_at=null 代替当前授权判断。
type AuthorizedResultReader interface {
	ReadAuthorized(ctx context.Context, request ReadRequest) (AuthorizedRecord, error)
}

// GetResult 是 Quant 的授权读取用例。它不接受客户端自报主体，也不暴露 Repository。
type GetResult struct {
	reader     AuthorizedResultReader
	principals PrincipalResolver
	auditor    AccessAuditor
	now        func() time.Time
}

// NewGetResult 创建授权读取用例；now 必须由 Server 注入可信 UTC Clock。
func NewGetResult(reader AuthorizedResultReader, principals PrincipalResolver, auditor AccessAuditor, now func() time.Time) (*GetResult, error) {
	if reader == nil || principals == nil || auditor == nil || now == nil {
		return nil, ErrInvalidGetResultRequest
	}
	return &GetResult{reader: reader, principals: principals, auditor: auditor, now: now}, nil
}

// Execute 只有在记录指纹、当前授权证明和强类型 Payload 全部验证后才返回完整 Envelope。
func (usecase *GetResult) Execute(ctx context.Context, resultID string) (result.Envelope, error) {
	if usecase == nil || ctx == nil || !validResultID(resultID) {
		return result.Envelope{}, ErrInvalidGetResultRequest
	}
	if err := ctx.Err(); err != nil {
		return result.Envelope{}, err
	}
	principal, err := usecase.principals.Resolve(ctx)
	if err != nil || !validText(principal.SubjectRef) {
		return result.Envelope{}, ErrResultUnavailable
	}
	now := usecase.now()
	if now.IsZero() || now.Location() != time.UTC {
		return result.Envelope{}, ErrResultUnavailable
	}
	request := ReadRequest{
		SubjectRef: principal.SubjectRef, ResourceRef: resultID,
		PurposeCode: quantPurposeCode, ContentLevel: quantContentLevel, At: now,
	}
	authorized, err := usecase.reader.ReadAuthorized(ctx, request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result.Envelope{}, ctxErr
		}
		decision, reasonCode := "unknown", "authorization_unavailable"
		if errors.Is(err, ErrAccessDenied) {
			decision, reasonCode = "denied", "authorization_denied"
		}
		if auditErr := usecase.recordAudit(ctx, request, decision, reasonCode, ""); auditErr != nil {
			return result.Envelope{}, auditErr
		}
		return result.Envelope{}, ErrResultUnavailable
	}
	if err := ctx.Err(); err != nil {
		return result.Envelope{}, err
	}
	record, err := authorized.Record.ValidatedSnapshot()
	if err != nil || record.Manifest.ResultID != resultID {
		if auditErr := usecase.recordAudit(ctx, request, "unknown", "stored_record_invalid", ""); auditErr != nil {
			return result.Envelope{}, auditErr
		}
		return result.Envelope{}, ErrInvalidStoredRecord
	}
	if err := validateAuthorization(authorized.AuthorizationRef, authorized.PolicySnapshot, request); err != nil {
		if auditErr := usecase.recordAudit(ctx, request, "denied", "authorization_proof_invalid", authorized.AuthorizationRef.AuthorizationRef); auditErr != nil {
			return result.Envelope{}, auditErr
		}
		return result.Envelope{}, err
	}
	payload, err := result.DecodePayload(record.Manifest.ResultType, record.Manifest.PayloadSchemaVersion, record.Payload)
	if err != nil {
		if auditErr := usecase.recordAudit(ctx, request, "unknown", "payload_type_unsupported", authorized.AuthorizationRef.AuthorizationRef); auditErr != nil {
			return result.Envelope{}, auditErr
		}
		return result.Envelope{}, result.ErrUnsupportedPayload
	}
	envelope, err := result.BuildEnvelope(record.Manifest, authorized.AuthorizationRef.AuthorizationRef, payload)
	if err != nil {
		if auditErr := usecase.recordAudit(ctx, request, "unknown", "envelope_invalid", authorized.AuthorizationRef.AuthorizationRef); auditErr != nil {
			return result.Envelope{}, auditErr
		}
		return result.Envelope{}, ErrInvalidStoredRecord
	}
	if err := envelope.ValidateAgainstManifest(record.Manifest); err != nil {
		if auditErr := usecase.recordAudit(ctx, request, "unknown", "manifest_mismatch", authorized.AuthorizationRef.AuthorizationRef); auditErr != nil {
			return result.Envelope{}, auditErr
		}
		return result.Envelope{}, ErrInvalidStoredRecord
	}
	if err := ctx.Err(); err != nil {
		return result.Envelope{}, err
	}
	if err := usecase.recordAudit(ctx, request, "allowed", "result_released", authorized.AuthorizationRef.AuthorizationRef); err != nil {
		return result.Envelope{}, err
	}
	return envelope, nil
}

func (usecase *GetResult) recordAudit(ctx context.Context, request ReadRequest, decision, reasonCode, authorizationRef string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	event := AccessAuditEvent{
		SubjectRef: request.SubjectRef, ResourceRef: request.ResourceRef,
		PurposeCode: request.PurposeCode, Decision: decision, ReasonCode: reasonCode,
		AuthorizationRef: authorizationRef, OccurredAt: request.At,
	}
	if err := usecase.auditor.RecordResultAccess(ctx, event); err != nil {
		return ErrAuditUnavailable
	}
	return nil
}

type authorizationFingerprintInput struct {
	SchemaVersion           string          `json:"schema_version"`
	PolicyOwner             string          `json:"policy_owner"`
	PolicyVersion           string          `json:"policy_version"`
	Decision                string          `json:"decision"`
	PurposeCode             string          `json:"purpose_code"`
	ContentLevel            string          `json:"content_level"`
	SubjectRef              string          `json:"subject_ref"`
	ResourceRef             string          `json:"resource_ref"`
	DecidedAt               string          `json:"decided_at"`
	ExpiresAt               *string         `json:"expires_at"`
	Conditions              []string        `json:"conditions"`
	EvaluatedPolicyVersions []PolicyVersion `json:"evaluated_policy_versions"`
}

func validateAuthorization(ref AuthorizationRef, snapshot PolicySnapshot, request ReadRequest) error {
	if ref.SchemaVersion != "1.0.0" || !validText(ref.AuthorizationRef) ||
		ref.PolicyOwner != "quant" || ref.Decision != "allowed" ||
		ref.PurposeCode != request.PurposeCode || ref.ContentLevel != request.ContentLevel ||
		ref.SubjectRef != request.SubjectRef || ref.ResourceRef != request.ResourceRef ||
		ref.PolicyVersion != snapshot.SnapshotRef || !ref.ExpiresAtPresent || ref.Conditions == nil || len(ref.Conditions) != 0 {
		return ErrInvalidAuthorization
	}
	decidedAt, ok := parseCanonicalUTC(ref.DecidedAt)
	if !ok || decidedAt.After(request.At) {
		return ErrInvalidAuthorization
	}
	if ref.ExpiresAt != nil {
		expiresAt, valid := parseCanonicalUTC(*ref.ExpiresAt)
		if !valid || !expiresAt.After(request.At) || !expiresAt.After(decidedAt) {
			return ErrInvalidAuthorization
		}
	}
	if snapshot.SchemaVersion != "1.0.0" || !validText(snapshot.SnapshotRef) ||
		snapshot.Policies == nil || len(snapshot.Policies) == 0 {
		return ErrInvalidAuthorization
	}
	policies := append([]PolicyVersion{}, snapshot.Policies...)
	for i, policy := range policies {
		if !validText(policy.PolicyType) || !validText(policy.PolicyRef) || !validText(policy.PolicyVersion) {
			return ErrInvalidAuthorization
		}
		if i > 0 && comparePolicyVersion(policies[i-1], policy) >= 0 {
			return ErrInvalidAuthorization
		}
	}
	input := authorizationFingerprintInput{
		SchemaVersion: ref.SchemaVersion, PolicyOwner: ref.PolicyOwner,
		PolicyVersion: ref.PolicyVersion, Decision: ref.Decision,
		PurposeCode: ref.PurposeCode, ContentLevel: ref.ContentLevel,
		SubjectRef: ref.SubjectRef, ResourceRef: ref.ResourceRef,
		DecidedAt: ref.DecidedAt, ExpiresAt: ref.ExpiresAt,
		Conditions:              append([]string{}, ref.Conditions...),
		EvaluatedPolicyVersions: policies,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return ErrInvalidAuthorization
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return ErrInvalidAuthorization
	}
	fingerprint, err := manifest.FingerprintPayload(canonical)
	if err != nil || fingerprint != ref.DecisionFingerprint {
		return ErrInvalidAuthorization
	}
	return nil
}

func comparePolicyVersion(left, right PolicyVersion) int {
	if value := strings.Compare(left.PolicyType, right.PolicyType); value != 0 {
		return value
	}
	if value := strings.Compare(left.PolicyRef, right.PolicyRef); value != 0 {
		return value
	}
	return strings.Compare(left.PolicyVersion, right.PolicyVersion)
}

func parseCanonicalUTC(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || !strings.HasSuffix(value, "Z") ||
		parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, false
	}
	return parsed, true
}

func validResultID(value string) bool {
	const prefix = "quant_result:sha256:"
	digest := strings.TrimPrefix(value, prefix)
	if digest == value || len(digest) != 64 {
		return false
	}
	for _, r := range digest {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	return utf8.ValidString(value) && value != "" && strings.TrimSpace(value) == value
}

// SortPolicyVersions 返回按 GOV-CTR-006 tuple 顺序排序的副本，供 Owner 构造快照时使用。
func SortPolicyVersions(values []PolicyVersion) []PolicyVersion {
	result := append([]PolicyVersion{}, values...)
	sort.Slice(result, func(i, j int) bool { return comparePolicyVersion(result[i], result[j]) < 0 })
	return result
}

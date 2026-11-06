package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

type AuthorizationDecision string

const (
	AuthorizationDecisionAllowed     AuthorizationDecision = "allowed"
	AuthorizationDecisionDenied      AuthorizationDecision = "denied"
	AuthorizationDecisionConditional AuthorizationDecision = "conditional"
	AuthorizationDecisionUnknown     AuthorizationDecision = "unknown"
)

type AuthorizationContentLevel string

const (
	AuthorizationContentMetadata AuthorizationContentLevel = "metadata"
	AuthorizationContentSummary  AuthorizationContentLevel = "summary"
	AuthorizationContentSnippet  AuthorizationContentLevel = "snippet"
	AuthorizationContentFull     AuthorizationContentLevel = "full"
)

type AuthorizationPolicyVersion struct {
	PolicyType    string `json:"policy_type"`
	PolicyRef     string `json:"policy_ref"`
	PolicyVersion string `json:"policy_version"`
}

type AuthorizationPolicySnapshot struct {
	SnapshotRef string                       `json:"snapshot_ref"`
	Policies    []AuthorizationPolicyVersion `json:"policies"`
}

// NullableDateTime distinguishes an explicit JSON null from an omitted field.
type NullableDateTime struct {
	Present bool
	Value   *string
}

func (v *NullableDateTime) UnmarshalJSON(data []byte) error {
	v.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		v.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	v.Value = &value
	return nil
}

func (v NullableDateTime) MarshalJSON() ([]byte, error) {
	if !v.Present {
		return nil, ErrAuthorizationNullableFieldMissing
	}
	if v.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*v.Value)
}

// NullableOpaqueRef distinguishes explicit JSON null from a missing required field.
type NullableOpaqueRef struct {
	Present bool
	Value   *string
}

func (v *NullableOpaqueRef) UnmarshalJSON(data []byte) error {
	v.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		v.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	v.Value = &value
	return nil
}

func (v NullableOpaqueRef) MarshalJSON() ([]byte, error) {
	if !v.Present {
		return nil, ErrAuthorizationNullableFieldMissing
	}
	if v.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*v.Value)
}

type NewsAuthorizationRef struct {
	AuthorizationRef    string                    `json:"authorization_ref"`
	PolicyOwner         string                    `json:"policy_owner"`
	PolicyVersion       string                    `json:"policy_version"`
	Decision            AuthorizationDecision     `json:"decision"`
	PurposeCode         string                    `json:"purpose_code"`
	ContentLevel        AuthorizationContentLevel `json:"content_level"`
	SubjectRef          string                    `json:"subject_ref"`
	ResourceRef         string                    `json:"resource_ref"`
	DecidedAt           string                    `json:"decided_at"`
	ExpiresAt           NullableDateTime          `json:"expires_at"`
	Conditions          []string                  `json:"conditions"`
	DecisionFingerprint string                    `json:"decision_fingerprint"`
}

// NewsAuthorizationBinding is the NEWS-CTR-002 cross-object binding DTO.
// CallerRef must be supplied by a trusted in-process caller context, never by
// an external request field. This value object does not itself authorize reads.
type NewsAuthorizationBinding struct {
	CallerRef             string                      `json:"caller_ref"`
	ResourceRef           string                      `json:"resource_ref"`
	PurposeCode           string                      `json:"purpose_code"`
	RequestedContentLevel AuthorizationContentLevel   `json:"requested_content_level"`
	ProviderRef           NullableOpaqueRef           `json:"provider_ref"`
	SourceRef             string                      `json:"source_ref"`
	EndpointRef           NullableOpaqueRef           `json:"endpoint_ref"`
	AuthorizationRef      NewsAuthorizationRef        `json:"authorization_ref"`
	PolicySnapshot        AuthorizationPolicySnapshot `json:"policy_snapshot"`
}

var (
	ErrAuthorizationBindingInvalid       = errors.New("news.authorization.binding_invalid")
	ErrAuthorizationNullableFieldMissing = errors.New("news.authorization.nullable_field_missing")
	// Fingerprint syntax is checked here; JCS verification waits for the shared GOV-CTR-006 verifier in the common baseline.
	authorizationFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func ValidateNewsAuthorizationBinding(binding NewsAuthorizationBinding) error {
	if !canonicalOpaqueRef(binding.CallerRef) ||
		!canonicalOpaqueRef(binding.ResourceRef) ||
		!canonicalOpaqueRef(binding.SourceRef) ||
		!validAuthorizationPurpose(binding.PurposeCode) ||
		!validAuthorizationContentLevel(binding.RequestedContentLevel) {
		return ErrAuthorizationBindingInvalid
	}
	if !binding.ProviderRef.Present || !binding.EndpointRef.Present {
		return ErrAuthorizationBindingInvalid
	}
	if binding.ProviderRef.Value != nil && !canonicalOpaqueRef(*binding.ProviderRef.Value) {
		return ErrAuthorizationBindingInvalid
	}
	if binding.EndpointRef.Value != nil && !canonicalOpaqueRef(*binding.EndpointRef.Value) {
		return ErrAuthorizationBindingInvalid
	}

	ref := binding.AuthorizationRef
	snapshot := binding.PolicySnapshot
	if !canonicalOpaqueRef(ref.AuthorizationRef) ||
		ref.PolicyOwner != "news" ||
		!canonicalOpaqueRef(ref.PolicyVersion) ||
		ref.PolicyVersion != snapshot.SnapshotRef ||
		ref.SubjectRef != binding.CallerRef ||
		ref.ResourceRef != binding.ResourceRef ||
		ref.PurposeCode != binding.PurposeCode ||
		!validAuthorizationContentLevel(ref.ContentLevel) ||
		!validAuthorizationDecision(ref.Decision) ||
		!validUTCDateTime(ref.DecidedAt) ||
		!ref.ExpiresAt.Present ||
		!authorizationFingerprintPattern.MatchString(ref.DecisionFingerprint) {
		return ErrAuthorizationBindingInvalid
	}
	if ref.ExpiresAt.Value != nil {
		expiresAt, ok := parseUTCDateTime(*ref.ExpiresAt.Value)
		decidedAt, decidedOK := parseUTCDateTime(ref.DecidedAt)
		if !ok || !decidedOK || !expiresAt.After(decidedAt) {
			return ErrAuthorizationBindingInvalid
		}
	}
	if ref.Conditions == nil || !strictlySortedNonEmpty(ref.Conditions) {
		return ErrAuthorizationBindingInvalid
	}
	if ref.Decision == AuthorizationDecisionAllowed && len(ref.Conditions) != 0 {
		return ErrAuthorizationBindingInvalid
	}
	if ref.Decision == AuthorizationDecisionConditional && len(ref.Conditions) == 0 {
		return ErrAuthorizationBindingInvalid
	}
	if ref.Decision == AuthorizationDecisionAllowed &&
		authorizationContentRank(ref.ContentLevel) < authorizationContentRank(binding.RequestedContentLevel) {
		return ErrAuthorizationBindingInvalid
	}
	if !canonicalOpaqueRef(snapshot.SnapshotRef) || len(snapshot.Policies) == 0 {
		return ErrAuthorizationBindingInvalid
	}
	if !validNewsPolicySnapshot(binding, snapshot) {
		return ErrAuthorizationBindingInvalid
	}
	return nil
}

type AuthorizationUseDecision struct {
	Allowed    bool
	ReasonCode string
}

func EvaluateNewsAuthorizationUse(binding NewsAuthorizationBinding, now time.Time) AuthorizationUseDecision {
	if err := ValidateNewsAuthorizationBinding(binding); err != nil {
		return AuthorizationUseDecision{ReasonCode: "news.authorization.binding_invalid"}
	}
	if now.IsZero() {
		return AuthorizationUseDecision{ReasonCode: "news.authorization.clock_invalid"}
	}
	ref := binding.AuthorizationRef
	decidedAt, ok := parseUTCDateTime(ref.DecidedAt)
	if !ok || now.Before(decidedAt) {
		return AuthorizationUseDecision{ReasonCode: "news.authorization.not_yet_effective"}
	}
	if ref.ExpiresAt.Value != nil {
		expiresAt, valid := parseUTCDateTime(*ref.ExpiresAt.Value)
		if !valid || !now.Before(expiresAt) {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.expired"}
		}
	}
	switch ref.Decision {
	case AuthorizationDecisionAllowed:
		if len(ref.Conditions) != 0 {
			return AuthorizationUseDecision{ReasonCode: "news.authorization.allowed_with_conditions"}
		}
		return AuthorizationUseDecision{Allowed: true}
	case AuthorizationDecisionConditional:
		return AuthorizationUseDecision{ReasonCode: "news.authorization.conditional_not_satisfied"}
	case AuthorizationDecisionDenied:
		return AuthorizationUseDecision{ReasonCode: "news.authorization.denied"}
	default:
		return AuthorizationUseDecision{ReasonCode: "news.authorization.decision_unknown"}
	}
}

func validNewsPolicySnapshot(binding NewsAuthorizationBinding, snapshot AuthorizationPolicySnapshot) bool {
	hasSource, hasProvider, hasEndpoint := false, false, false
	var previous AuthorizationPolicyVersion
	for i, policy := range snapshot.Policies {
		if !canonicalOpaqueRef(policy.PolicyType) ||
			!canonicalOpaqueRef(policy.PolicyRef) ||
			!canonicalOpaqueRef(policy.PolicyVersion) {
			return false
		}
		if i > 0 && comparePolicyVersion(previous, policy) >= 0 {
			return false
		}
		switch policy.PolicyType {
		case "source":
			if hasSource || policy.PolicyRef != binding.SourceRef {
				return false
			}
			hasSource = true
		case "provider_output":
			if hasProvider || binding.ProviderRef.Value == nil || policy.PolicyRef != *binding.ProviderRef.Value {
				return false
			}
			hasProvider = true
		case "endpoint":
			if hasEndpoint || binding.EndpointRef.Value == nil || policy.PolicyRef != *binding.EndpointRef.Value {
				return false
			}
			hasEndpoint = true
		default:
			return false
		}
		previous = policy
	}
	return hasSource &&
		(binding.ProviderRef.Value == nil || hasProvider) &&
		(binding.EndpointRef.Value == nil || hasEndpoint)
}

func comparePolicyVersion(a, b AuthorizationPolicyVersion) int {
	for _, pair := range [][2]string{{a.PolicyType, b.PolicyType}, {a.PolicyRef, b.PolicyRef}, {a.PolicyVersion, b.PolicyVersion}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func canonicalOpaqueRef(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func strictlySortedNonEmpty(values []string) bool {
	for i, value := range values {
		if !canonicalOpaqueRef(value) || (i > 0 && values[i-1] >= value) {
			return false
		}
	}
	return true
}

func validAuthorizationPurpose(value string) bool {
	switch value {
	case "research_analysis", "risk_review", "user_display":
		return true
	default:
		return false
	}
}

// AuthorizationPurposeCodeForPurpose maps only domain purposes with an
// unambiguous NEWS-CTR-002 equivalent. Unsupported/ambiguous purposes fail closed.
func AuthorizationPurposeCodeForPurpose(purpose Purpose) (string, bool) {
	switch purpose {
	case PurposeInternalPersonalResearch:
		return "research_analysis", true
	case PurposePublicDisplay:
		return "user_display", true
	default:
		return "", false
	}
}

// AuthorizationContentLevelForKind maps textual content kinds to the contract
// disclosure hierarchy. Attachments are not implicitly equivalent to full text.
func AuthorizationContentLevelForKind(kind ContentKind) (AuthorizationContentLevel, bool) {
	switch kind {
	case ContentMetadata, ContentTitle:
		return AuthorizationContentMetadata, true
	case ContentAbstract:
		return AuthorizationContentSummary, true
	case ContentSnippet:
		return AuthorizationContentSnippet, true
	case ContentFullText:
		return AuthorizationContentFull, true
	default:
		return "", false
	}
}

func validAuthorizationContentLevel(value AuthorizationContentLevel) bool {
	switch value {
	case AuthorizationContentMetadata, AuthorizationContentSummary, AuthorizationContentSnippet, AuthorizationContentFull:
		return true
	default:
		return false
	}
}

func authorizationContentRank(value AuthorizationContentLevel) int {
	switch value {
	case AuthorizationContentMetadata:
		return 1
	case AuthorizationContentSummary:
		return 2
	case AuthorizationContentSnippet:
		return 3
	case AuthorizationContentFull:
		return 4
	default:
		return 0
	}
}

func validAuthorizationDecision(value AuthorizationDecision) bool {
	switch value {
	case AuthorizationDecisionAllowed, AuthorizationDecisionDenied, AuthorizationDecisionConditional, AuthorizationDecisionUnknown:
		return true
	default:
		return false
	}
}

func validUTCDateTime(value string) bool {
	_, ok := parseUTCDateTime(value)
	return ok
}

func parseUTCDateTime(value string) (time.Time, bool) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil && parsed.Location() == time.UTC
}

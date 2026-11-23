package authorization

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	DataAccessPolicyType          = "data_access"
	DailyBarsV2PolicyRef          = "dailybars-v2-access"
	DataAccessPolicySchemaVersion = "1.0.0"
)

type PolicyStatus string

const (
	PolicyApproved   PolicyStatus = "approved"
	PolicyRestricted PolicyStatus = "restricted"
	PolicyPending    PolicyStatus = "pending"
	PolicyRevoked    PolicyStatus = "revoked"
)

var (
	ErrPolicyInvalid = errors.New("market-data authorization policy invalid")
	ErrPolicyDenied  = errors.New("market-data authorization policy denied")
)

// DataAccessPolicy 是 Data Owner 对日线 Snapshot 访问的最小策略对象。
// 空 AllowedSubjects 或 AllowedPurposes 均不授予访问权限；不存在默认允许。
type DataAccessPolicy struct {
	SchemaVersion   string       `json:"schema_version"`
	Status          PolicyStatus `json:"status"`
	AllowedSubjects []string     `json:"allowed_subjects"`
	AllowedPurposes []string     `json:"allowed_purposes"`
	RequiredScopes  []string     `json:"required_scopes"`
	EffectiveAt     time.Time    `json:"effective_at"`
	ExpiresAt       *time.Time   `json:"expires_at"`
}

type PolicyVersion struct {
	PolicyType    string
	PolicyRef     string
	PolicyVersion string
	Payload       []byte
	PayloadHash   string
	CreatedAt     time.Time
}

type CurrentPolicy struct {
	Version PolicyVersion
	Policy  DataAccessPolicy
}

type AccessRequest struct {
	SubjectRef  string
	PurposeCode string
	Scopes      []string
	Now         time.Time
}

func DecodePolicyVersion(value PolicyVersion) (DataAccessPolicy, error) {
	if value.PolicyType != DataAccessPolicyType || value.PolicyRef != DailyBarsV2PolicyRef ||
		!validOpaque(value.PolicyVersion) || value.CreatedAt.IsZero() || value.CreatedAt.Location() != time.UTC ||
		len(value.Payload) == 0 || len(value.Payload) > 64*1024 || len(value.PayloadHash) != 64 {
		return DataAccessPolicy{}, ErrPolicyInvalid
	}
	var policy DataAccessPolicy
	decoder := json.NewDecoder(bytes.NewReader(value.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return DataAccessPolicy{}, ErrPolicyInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || ValidatePolicy(policy) != nil {
		return DataAccessPolicy{}, ErrPolicyInvalid
	}
	return policy, nil
}

func ValidatePolicy(policy DataAccessPolicy) error {
	if policy.SchemaVersion != DataAccessPolicySchemaVersion ||
		(policy.Status != PolicyApproved && policy.Status != PolicyRestricted &&
			policy.Status != PolicyPending && policy.Status != PolicyRevoked) ||
		policy.EffectiveAt.IsZero() || policy.EffectiveAt.Location() != time.UTC {
		return ErrPolicyInvalid
	}
	if policy.ExpiresAt != nil &&
		(policy.ExpiresAt.IsZero() || policy.ExpiresAt.Location() != time.UTC ||
			!policy.ExpiresAt.After(policy.EffectiveAt)) {
		return ErrPolicyInvalid
	}
	if !sortedUniqueNonEmpty(policy.AllowedSubjects, validPrincipalRef) ||
		!sortedUniqueNonEmpty(policy.AllowedPurposes, validPurpose) ||
		!sortedUnique(policy.RequiredScopes, validScope) {
		return ErrPolicyInvalid
	}
	return nil
}

func Evaluate(policy DataAccessPolicy, request AccessRequest) error {
	if ValidatePolicy(policy) != nil || !validPrincipalRef(request.SubjectRef) ||
		!validPurpose(request.PurposeCode) || request.Now.IsZero() || request.Now.Location() != time.UTC ||
		!sortedUnique(request.Scopes, validScope) {
		return ErrPolicyInvalid
	}
	if policy.Status != PolicyApproved && policy.Status != PolicyRestricted {
		return ErrPolicyDenied
	}
	if request.Now.Before(policy.EffectiveAt) ||
		(policy.ExpiresAt != nil && !request.Now.Before(*policy.ExpiresAt)) {
		return ErrPolicyDenied
	}
	if !contains(policy.AllowedSubjects, request.SubjectRef) ||
		!contains(policy.AllowedPurposes, request.PurposeCode) {
		return ErrPolicyDenied
	}
	for _, required := range policy.RequiredScopes {
		if !contains(request.Scopes, required) {
			return ErrPolicyDenied
		}
	}
	return nil
}

func validPurpose(value string) bool {
	switch value {
	case "quant_analysis", "research_analysis", "risk_review":
		return true
	default:
		return false
	}
}

func validPrincipalRef(value string) bool {
	if !strings.HasPrefix(value, "principal:") || len(value) > 256 ||
		strings.TrimSpace(value) != value {
		return false
	}
	return validOpaque(value)
}

func validScope(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') &&
			r != ':' && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

func validOpaque(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func sortedUniqueNonEmpty(values []string, valid func(string) bool) bool {
	return len(values) > 0 && sortedUnique(values, valid)
}

func sortedUnique(values []string, valid func(string) bool) bool {
	copyValues := append([]string(nil), values...)
	if !sort.StringsAreSorted(copyValues) {
		return false
	}
	for index, value := range copyValues {
		if !valid(value) || (index > 0 && copyValues[index-1] == value) {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

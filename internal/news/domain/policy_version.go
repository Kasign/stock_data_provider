package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	NewsPolicyTypeSource         = "source"
	NewsPolicyTypeEndpoint       = "endpoint"
	NewsPolicyTypeProviderOutput = "provider_output"
)

var ErrNewsPolicyVersionInvalid = errors.New("news.policy.version_invalid")

// NewsPolicyVersion 是可持久化的不可变策略版本。Payload 保存策略领域对象的 JSON。
type NewsPolicyVersion struct {
	PolicyType    string
	PolicyRef     string
	PolicyVersion string
	Payload       []byte
	PayloadHash   string
	CreatedAt     time.Time
}

// ValidateNewsPolicyVersion 校验策略身份、版本载荷及其时间/状态字段。
// PayloadHash 由调用方按原始 Payload 字节计算；持久层会再次核验。
func ValidateNewsPolicyVersion(value NewsPolicyVersion) error {
	if !canonicalOpaqueRef(value.PolicyType) ||
		!canonicalOpaqueRef(value.PolicyRef) ||
		!canonicalOpaqueRef(value.PolicyVersion) ||
		value.CreatedAt.IsZero() || value.CreatedAt.Location() != time.UTC ||
		len(value.Payload) == 0 || len(value.Payload) > 256*1024 ||
		len(value.PayloadHash) != 64 {
		return ErrNewsPolicyVersionInvalid
	}
	switch value.PolicyType {
	case NewsPolicyTypeSource:
		var policy SourcePolicy
		if err := decodeNewsPolicyPayload(value.Payload, &policy); err != nil ||
			!validAuthorizationStatus(policy.Status, false) ||
			strings.TrimSpace(policy.EvidenceRef) == "" ||
			policy.MaxRetention < 0 ||
			!validNewsPolicyTimeRange(policy.EffectiveAt, policy.ExpiresAt) {
			return ErrNewsPolicyVersionInvalid
		}
	case NewsPolicyTypeEndpoint:
		var policy EndpointPolicy
		if err := decodeNewsPolicyPayload(value.Payload, &policy); err != nil ||
			!validAuthorizationStatus(policy.Status, true) ||
			(policy.HasOverrides() && strings.TrimSpace(policy.EvidenceRef) == "") ||
			(policy.MaxRetention != nil && *policy.MaxRetention < 0) ||
			!validNewsPolicyTimeRange(policy.EffectiveAt, policy.ExpiresAt) {
			return ErrNewsPolicyVersionInvalid
		}
	case NewsPolicyTypeProviderOutput:
		var policy ProviderOutputPolicy
		if err := decodeNewsPolicyPayload(value.Payload, &policy); err != nil ||
			!validAuthorizationStatus(policy.Status, false) ||
			strings.TrimSpace(policy.EvidenceRef) == "" ||
			policy.MaxRequestRetention < 0 || policy.MaxCacheRetention < 0 ||
			policy.MaxPersistenceRetention < 0 ||
			!validNewsPolicyTimeRange(policy.EffectiveAt, policy.ExpiresAt) {
			return ErrNewsPolicyVersionInvalid
		}
	default:
		return ErrNewsPolicyVersionInvalid
	}
	return nil
}

func decodeNewsPolicyPayload(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("news policy payload: trailing JSON")
	}
	return nil
}

func validAuthorizationStatus(status AuthorizationStatus, allowEmpty bool) bool {
	if allowEmpty && status == "" {
		return true
	}
	switch status {
	case AuthorizationApproved, AuthorizationRestricted, AuthorizationPending, AuthorizationRevoked:
		return true
	default:
		return false
	}
}

func validNewsPolicyTimeRange(effectiveAt time.Time, expiresAt *time.Time) bool {
	return expiresAt == nil || (effectiveAt.IsZero() || expiresAt.After(effectiveAt))
}

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

const (
	ProviderOutputPoliciesEnv = "NEWS_PROVIDER_OUTPUT_POLICIES_JSON"
	maximumProviderPolicies   = 32
)

// ProviderOutputPolicyRegistration 是经过启动校验的 Provider 输出策略版本绑定。
// ExpectedCurrentPolicyVersion 为空允许首次发布；前进版本必须显式给出当前版本。
type ProviderOutputPolicyRegistration struct {
	ProviderCode                 string
	PolicyRef                    string
	PolicyVersion                string
	ExpectedCurrentPolicyVersion string
	Policy                       domain.ProviderOutputPolicy
}

type providerOutputPolicyJSON struct {
	ProviderCode                 string                       `json:"provider_code"`
	PolicyRef                    string                       `json:"policy_ref"`
	PolicyVersion                string                       `json:"policy_version"`
	ExpectedCurrentPolicyVersion string                       `json:"expected_current_policy_version"`
	Policy                       providerOutputPolicyBodyJSON `json:"policy"`
}

type providerOutputPolicyBodyJSON struct {
	Status                        domain.AuthorizationStatus `json:"status"`
	AllowedPurposes               []domain.Purpose           `json:"allowed_purposes"`
	AllowedContentKinds           []domain.ContentKind       `json:"allowed_content_kinds"`
	AllowRequestProcessing        bool                       `json:"allow_request_processing"`
	AllowCrossRequestCache        bool                       `json:"allow_cross_request_cache"`
	MaxRequestRetentionSeconds    int64                      `json:"max_request_retention_seconds"`
	MaxCacheRetentionSeconds      int64                      `json:"max_cache_retention_seconds"`
	AllowedStorageModes           []domain.StorageMode       `json:"allowed_storage_modes"`
	AllowedMetadataFields         []string                   `json:"allowed_metadata_fields"`
	MaxPersistenceRetentionSecond int64                      `json:"max_persistence_retention_seconds"`
	EffectiveAt                   time.Time                  `json:"effective_at"`
	ExpiresAt                     *time.Time                 `json:"expires_at"`
	EvidenceRef                   string                     `json:"evidence_ref"`
}

func (c Config) ProviderOutputPolicies() []ProviderOutputPolicyRegistration {
	out := make([]ProviderOutputPolicyRegistration, len(c.providerOutputPolicies))
	for i, value := range c.providerOutputPolicies {
		out[i] = cloneProviderOutputPolicyRegistration(value)
	}
	return out
}

func loadProviderOutputPolicies(env platformconfig.Env) ([]ProviderOutputPolicyRegistration, error) {
	raw := strings.TrimSpace(env.String(ProviderOutputPoliciesEnv, ""))
	if raw == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var entries []providerOutputPolicyJSON
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("news config %s must be a valid JSON array", ProviderOutputPoliciesEnv)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("news config %s must contain exactly one JSON value", ProviderOutputPoliciesEnv)
	}
	if len(entries) == 0 || len(entries) > maximumProviderPolicies {
		return nil, fmt.Errorf("news config %s must contain 1..%d provider policies", ProviderOutputPoliciesEnv, maximumProviderPolicies)
	}
	result := make([]ProviderOutputPolicyRegistration, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		value, err := convertProviderOutputPolicy(entry)
		if err != nil {
			return nil, fmt.Errorf("news config %s contains an invalid provider policy", ProviderOutputPoliciesEnv)
		}
		if _, exists := seen[value.ProviderCode]; exists {
			return nil, fmt.Errorf("news config %s contains duplicate provider_code", ProviderOutputPoliciesEnv)
		}
		seen[value.ProviderCode] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func convertProviderOutputPolicy(entry providerOutputPolicyJSON) (ProviderOutputPolicyRegistration, error) {
	value := ProviderOutputPolicyRegistration{
		ProviderCode:                 strings.TrimSpace(entry.ProviderCode),
		PolicyRef:                    strings.TrimSpace(entry.PolicyRef),
		PolicyVersion:                strings.TrimSpace(entry.PolicyVersion),
		ExpectedCurrentPolicyVersion: strings.TrimSpace(entry.ExpectedCurrentPolicyVersion),
	}
	if !feedIdentifierPattern.MatchString(value.ProviderCode) ||
		!feedIdentifierPattern.MatchString(value.PolicyRef) ||
		!feedIdentifierPattern.MatchString(value.PolicyVersion) ||
		(value.ExpectedCurrentPolicyVersion != "" && !feedIdentifierPattern.MatchString(value.ExpectedCurrentPolicyVersion)) {
		return ProviderOutputPolicyRegistration{}, errors.New("invalid provider policy identity")
	}
	limits := []int64{
		entry.Policy.MaxRequestRetentionSeconds,
		entry.Policy.MaxCacheRetentionSeconds,
		entry.Policy.MaxPersistenceRetentionSecond,
	}
	for _, seconds := range limits {
		if seconds < 0 || seconds > math.MaxInt64/int64(time.Second) {
			return ProviderOutputPolicyRegistration{}, errors.New("invalid provider policy retention")
		}
	}
	value.Policy = domain.ProviderOutputPolicy{
		Status:                  entry.Policy.Status,
		AllowedPurposes:         append([]domain.Purpose(nil), entry.Policy.AllowedPurposes...),
		AllowedContentKinds:     append([]domain.ContentKind(nil), entry.Policy.AllowedContentKinds...),
		AllowRequestProcessing:  entry.Policy.AllowRequestProcessing,
		AllowCrossRequestCache:  entry.Policy.AllowCrossRequestCache,
		MaxRequestRetention:     time.Duration(entry.Policy.MaxRequestRetentionSeconds) * time.Second,
		MaxCacheRetention:       time.Duration(entry.Policy.MaxCacheRetentionSeconds) * time.Second,
		AllowedStorageModes:     append([]domain.StorageMode(nil), entry.Policy.AllowedStorageModes...),
		AllowedMetadataFields:   append([]string(nil), entry.Policy.AllowedMetadataFields...),
		MaxPersistenceRetention: time.Duration(entry.Policy.MaxPersistenceRetentionSecond) * time.Second,
		EffectiveAt:             entry.Policy.EffectiveAt.UTC(),
		ExpiresAt:               cloneTime(entry.Policy.ExpiresAt),
		EvidenceRef:             strings.TrimSpace(entry.Policy.EvidenceRef),
	}
	payload, err := json.Marshal(value.Policy)
	if err != nil {
		return ProviderOutputPolicyRegistration{}, err
	}
	sum := sha256.Sum256(payload)
	version := domain.NewsPolicyVersion{
		PolicyType: domain.NewsPolicyTypeProviderOutput, PolicyRef: value.PolicyRef,
		PolicyVersion: value.PolicyVersion, Payload: payload,
		PayloadHash: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC(),
	}
	if err := domain.ValidateNewsPolicyVersion(version); err != nil {
		return ProviderOutputPolicyRegistration{}, err
	}

	now := time.Now().UTC()
	requests := []domain.CollectionPolicyRequest{
		{
			Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentAbstract,
			StorageMode: domain.StorageTransient, Retention: time.Hour,
		},
		{
			Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentAbstract,
			StorageMode: domain.StoragePersistentExcerpt, Retention: time.Hour,
		},
	}
	for _, fieldPath := range value.Policy.AllowedMetadataFields {
		requests = append(requests, domain.CollectionPolicyRequest{
			Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentMetadata,
			StorageMode: domain.StoragePersistentMetadata, Retention: time.Hour, FieldPath: fieldPath,
		})
	}
	for _, request := range requests {
		decision := domain.EvaluateProviderOutputPolicy(value.Policy, request, now)
		if !decision.Allowed {
			return ProviderOutputPolicyRegistration{}, errors.New("provider policy does not authorize required output paths")
		}
	}
	return value, nil
}

func validateProviderPolicyBindings(
	sources []FeedSource,
	policies []ProviderOutputPolicyRegistration,
) error {
	byCode := make(map[string]ProviderOutputPolicyRegistration, len(policies))
	for _, policy := range policies {
		byCode[policy.ProviderCode] = policy
	}
	for _, source := range sources {
		if source.ProviderCode == "" {
			continue
		}
		policy, exists := byCode[source.ProviderCode]
		if !exists || policy.PolicyRef != source.ProviderOutputPolicyRef ||
			policy.PolicyVersion != source.ProviderOutputPolicyVersionID {
			return fmt.Errorf("news config %s binding for source %q has no matching provider policy", FeedSourcesEnv, source.SourceID)
		}
	}
	return nil
}

func cloneProviderOutputPolicyRegistration(value ProviderOutputPolicyRegistration) ProviderOutputPolicyRegistration {
	value.Policy.AllowedPurposes = append([]domain.Purpose(nil), value.Policy.AllowedPurposes...)
	value.Policy.AllowedContentKinds = append([]domain.ContentKind(nil), value.Policy.AllowedContentKinds...)
	value.Policy.AllowedStorageModes = append([]domain.StorageMode(nil), value.Policy.AllowedStorageModes...)
	value.Policy.AllowedMetadataFields = append([]string(nil), value.Policy.AllowedMetadataFields...)
	value.Policy.ExpiresAt = cloneTime(value.Policy.ExpiresAt)
	return value
}

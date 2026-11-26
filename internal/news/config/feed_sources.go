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
	"regexp"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

const (
	FeedSourcesEnv = "NEWS_FEED_SOURCES_JSON"
	maximumFeedSources = 32
	defaultFeedIngestRetention = 24 * time.Hour
)

var feedIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// FeedSource 是经过启动配置校验的只读 Source/Endpoint 注册快照。
// ExpectedCurrentSourcePolicyVersionID 是显式版本前置条件：空值允许首次初始化；
// 当前已是相同目标版本且载荷一致时幂等成功；更新到新版本时必须填写期望替换的当前版本，避免旧配置回退。
type FeedSource struct {
	SourceID string
	EndpointID string
	URL string
	PublisherName string
	PublisherDomain string
	AccessMethod domain.AccessMethod
	SourcePolicy domain.SourcePolicy
	SourcePolicyVersionID string
	ExpectedCurrentSourcePolicyVersionID string
	URLPolicy domain.SourceURLPolicy
	MaxResponseBytes int64
}

// FeedSources returns a defensive copy of the trusted startup registrations.
func (c Config) FeedSources() []FeedSource {
	out := make([]FeedSource, len(c.feedSources))
	for i, value := range c.feedSources {
		out[i] = cloneFeedSource(value)
	}
	return out
}

type feedSourceJSON struct {
	SourceID string `json:"source_id"`
	EndpointID string `json:"endpoint_id"`
	URL string `json:"url"`
	PublisherName string `json:"publisher_name"`
	PublisherDomain string `json:"publisher_domain"`
	AccessMethod domain.AccessMethod `json:"access_method"`
	SourcePolicyVersionID string `json:"source_policy_version_id"`
	ExpectedCurrentSourcePolicyVersionID string `json:"expected_current_source_policy_version_id"`
	SourcePolicy feedSourcePolicyJSON `json:"source_policy"`
	URLPolicy feedURLPolicyJSON `json:"url_policy"`
	MaxResponseBytes int64 `json:"max_response_bytes"`
}

type feedSourcePolicyJSON struct {
	Status domain.AuthorizationStatus `json:"status"`
	AllowedAccessMethods []domain.AccessMethod `json:"allowed_access_methods"`
	AllowedPurposes []domain.Purpose `json:"allowed_purposes"`
	AllowedContentKinds []domain.ContentKind `json:"allowed_content_kinds"`
	AllowedStorageModes []domain.StorageMode `json:"allowed_storage_modes"`
	AllowedMetadataFields []string `json:"allowed_metadata_fields"`
	MaxRetentionSeconds int64 `json:"max_retention_seconds"`
	EffectiveAt time.Time `json:"effective_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	EvidenceRef string `json:"evidence_ref"`
}

type feedURLPolicyJSON struct {
	AllowedSchemes []string `json:"allowed_schemes"`
	AllowedHosts []string `json:"allowed_hosts"`
	AllowedPorts []uint16 `json:"allowed_ports"`
}

func loadFeedSources(env platformconfig.Env) ([]FeedSource, error) {
	raw := strings.TrimSpace(env.String(FeedSourcesEnv, ""))
	if raw == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var entries []feedSourceJSON
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("news config %s must be a valid JSON array", FeedSourcesEnv)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("news config %s must contain exactly one JSON value", FeedSourcesEnv)
	}
	if len(entries) == 0 || len(entries) > maximumFeedSources {
		return nil, fmt.Errorf("news config %s must contain 1..%d feed sources", FeedSourcesEnv, maximumFeedSources)
	}

	result := make([]FeedSource, 0, len(entries))
	endpoints := make(map[string]struct{}, len(entries))
	sourcePolicies := make(map[string]string, len(entries))
	for _, entry := range entries {
		value, err := convertFeedSource(entry)
		if err != nil {
			return nil, fmt.Errorf("news config %s contains an invalid source registration", FeedSourcesEnv)
		}
		key := value.SourceID + "\x00" + value.EndpointID
		if _, exists := endpoints[key]; exists {
			return nil, fmt.Errorf("news config %s contains duplicate Source/Endpoint identity", FeedSourcesEnv)
		}
		endpoints[key] = struct{}{}

		payload, err := json.Marshal(value.SourcePolicy)
		if err != nil {
			return nil, fmt.Errorf("news config %s contains an invalid source policy", FeedSourcesEnv)
		}
		sum := sha256.Sum256(payload)
		policyFingerprint := hex.EncodeToString(sum[:])
		if previous, exists := sourcePolicies[value.SourceID]; exists && previous != value.SourcePolicyVersionID+"\x00"+value.ExpectedCurrentSourcePolicyVersionID+"\x00"+policyFingerprint {
			return nil, fmt.Errorf("news config %s contains conflicting policy snapshots for one source", FeedSourcesEnv)
		}
		sourcePolicies[value.SourceID] = value.SourcePolicyVersionID + "\x00" + value.ExpectedCurrentSourcePolicyVersionID + "\x00" + policyFingerprint
		result = append(result, value)
	}
	return result, nil
}

func convertFeedSource(entry feedSourceJSON) (FeedSource, error) {
	value := FeedSource{
		SourceID: strings.TrimSpace(entry.SourceID),
		EndpointID: strings.TrimSpace(entry.EndpointID),
		URL: strings.TrimSpace(entry.URL),
		PublisherName: strings.TrimSpace(entry.PublisherName),
		PublisherDomain: strings.TrimSpace(entry.PublisherDomain),
		AccessMethod: entry.AccessMethod,
		SourcePolicyVersionID: strings.TrimSpace(entry.SourcePolicyVersionID),
		ExpectedCurrentSourcePolicyVersionID: strings.TrimSpace(entry.ExpectedCurrentSourcePolicyVersionID),
		MaxResponseBytes: entry.MaxResponseBytes,
	}
	if !feedIdentifierPattern.MatchString(value.SourceID) ||
		!feedIdentifierPattern.MatchString(value.EndpointID) ||
		!feedIdentifierPattern.MatchString(value.SourcePolicyVersionID) ||
		(value.ExpectedCurrentSourcePolicyVersionID != "" && !feedIdentifierPattern.MatchString(value.ExpectedCurrentSourcePolicyVersionID)) ||
		value.URL == "" || value.URL != entry.URL || value.PublisherName == "" || value.PublisherDomain == "" ||
		(value.AccessMethod != domain.AccessRSS && value.AccessMethod != domain.AccessLicensedFeed) ||
		value.MaxResponseBytes < 0 || value.MaxResponseBytes > 16<<20 {
		return FeedSource{}, errors.New("invalid Feed source identity or endpoint")
	}

	if entry.SourcePolicy.MaxRetentionSeconds <= 0 ||
		entry.SourcePolicy.MaxRetentionSeconds > math.MaxInt64/int64(time.Second) {
		return FeedSource{}, errors.New("invalid Feed source policy retention")
	}
	value.SourcePolicy = domain.SourcePolicy{
		Status: entry.SourcePolicy.Status,
		AllowedAccessMethods: append([]domain.AccessMethod(nil), entry.SourcePolicy.AllowedAccessMethods...),
		AllowedPurposes: append([]domain.Purpose(nil), entry.SourcePolicy.AllowedPurposes...),
		AllowedContentKinds: append([]domain.ContentKind(nil), entry.SourcePolicy.AllowedContentKinds...),
		AllowedStorageModes: append([]domain.StorageMode(nil), entry.SourcePolicy.AllowedStorageModes...),
		AllowedMetadataFields: append([]string(nil), entry.SourcePolicy.AllowedMetadataFields...),
		MaxRetention: time.Duration(entry.SourcePolicy.MaxRetentionSeconds) * time.Second,
		EffectiveAt: entry.SourcePolicy.EffectiveAt.UTC(),
		ExpiresAt: cloneTime(entry.SourcePolicy.ExpiresAt),
		EvidenceRef: strings.TrimSpace(entry.SourcePolicy.EvidenceRef),
	}
	value.URLPolicy = domain.SourceURLPolicy{
		AllowedSchemes: append([]string(nil), entry.URLPolicy.AllowedSchemes...),
		AllowedHosts: append([]string(nil), entry.URLPolicy.AllowedHosts...),
		AllowedPorts: append([]uint16(nil), entry.URLPolicy.AllowedPorts...),
	}
	if !domain.EvaluateSourceURL(value.URL, value.URLPolicy).Allowed {
		return FeedSource{}, errors.New("Feed endpoint URL is outside its allowlist")
	}

	payload, err := json.Marshal(value.SourcePolicy)
	if err != nil {
		return FeedSource{}, err
	}
	sum := sha256.Sum256(payload)
	version := domain.NewsPolicyVersion{
		PolicyType: domain.NewsPolicyTypeSource, PolicyRef: value.SourceID,
		PolicyVersion: value.SourcePolicyVersionID, Payload: payload,
		PayloadHash: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC(),
	}
	if err := domain.ValidateNewsPolicyVersion(version); err != nil {
		return FeedSource{}, err
	}

	retention := defaultFeedIngestRetention
	if value.SourcePolicy.MaxRetention < retention {
		retention = value.SourcePolicy.MaxRetention
	}
	now := time.Now().UTC()
	for _, storageMode := range []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentExcerpt} {
		decision := domain.EvaluateSourceCollectionPolicy(value.SourcePolicy, nil, domain.CollectionPolicyRequest{
			AccessMethod: value.AccessMethod, Purpose: domain.PurposeInternalPersonalResearch,
			ContentKind: domain.ContentAbstract, StorageMode: storageMode, Retention: retention,
		}, now)
		if !decision.Allowed {
			return FeedSource{}, errors.New("Feed source policy does not authorize the required collection/storage path")
		}
	}
	return value, nil
}

func cloneFeedSource(value FeedSource) FeedSource {
	value.SourcePolicy.AllowedAccessMethods = append([]domain.AccessMethod(nil), value.SourcePolicy.AllowedAccessMethods...)
	value.SourcePolicy.AllowedPurposes = append([]domain.Purpose(nil), value.SourcePolicy.AllowedPurposes...)
	value.SourcePolicy.AllowedContentKinds = append([]domain.ContentKind(nil), value.SourcePolicy.AllowedContentKinds...)
	value.SourcePolicy.AllowedStorageModes = append([]domain.StorageMode(nil), value.SourcePolicy.AllowedStorageModes...)
	value.SourcePolicy.AllowedMetadataFields = append([]string(nil), value.SourcePolicy.AllowedMetadataFields...)
	value.SourcePolicy.ExpiresAt = cloneTime(value.SourcePolicy.ExpiresAt)
	value.URLPolicy.AllowedSchemes = append([]string(nil), value.URLPolicy.AllowedSchemes...)
	value.URLPolicy.AllowedHosts = append([]string(nil), value.URLPolicy.AllowedHosts...)
	value.URLPolicy.AllowedPorts = append([]uint16(nil), value.URLPolicy.AllowedPorts...)
	return value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

package domain

import "time"

// AuthorizationStatus 表示授权核验状态；restricted 仍可在明确限制范围内使用。
type AuthorizationStatus string

const (
	AuthorizationApproved   AuthorizationStatus = "approved"
	AuthorizationRestricted AuthorizationStatus = "restricted"
	AuthorizationPending    AuthorizationStatus = "pending"
	AuthorizationRevoked    AuthorizationStatus = "revoked"
)

// AccessMethod 表示来源访问方式。
type AccessMethod string

const (
	AccessManual       AccessMethod = "manual"
	AccessOfficialAPI  AccessMethod = "official_api"
	AccessRSS          AccessMethod = "rss"
	AccessScriptedWeb  AccessMethod = "scripted_web"
	AccessLicensedFeed AccessMethod = "licensed_feed"
)

// ContentKind 表示拟采集的内容类型。
type ContentKind string

const (
	ContentMetadata   ContentKind = "metadata"
	ContentTitle      ContentKind = "title"
	ContentSnippet    ContentKind = "snippet"
	ContentAbstract   ContentKind = "abstract"
	ContentFullText   ContentKind = "full_text"
	ContentAttachment ContentKind = "attachment"
)

// StorageMode 表示内容的处理/保存方式。
type StorageMode string

const (
	StorageNone               StorageMode = "none"
	StorageTransient          StorageMode = "transient"
	StoragePersistentMetadata StorageMode = "persistent_metadata"
	StoragePersistentExcerpt  StorageMode = "persistent_excerpt"
	StoragePersistentFullText StorageMode = "persistent_full_text"
)

// Purpose 表示内容使用目的。
type Purpose string

const (
	PurposeInternalPersonalResearch Purpose = "internal_personal_research"
	PurposeInternalBusiness         Purpose = "internal_business"
	PurposePublicDisplay            Purpose = "public_display"
	PurposeRedistribution           Purpose = "redistribution"
	PurposeModelTraining            Purpose = "model_training"
)

// SourcePolicy 是发布来源级授权规则。空白许可集合表示不授予该维度权限。
type SourcePolicy struct {
	Status               AuthorizationStatus
	AllowedAccessMethods []AccessMethod
	AllowedPurposes      []Purpose
	AllowedContentKinds  []ContentKind
	AllowedStorageModes  []StorageMode
	MaxRetention         time.Duration
	EffectiveAt          time.Time
	ExpiresAt            *time.Time
	EvidenceRef          string
}

// EndpointPolicy 只表达入口相对 Source 默认规则的差异。
// nil 集合/MaxRetention 表示继承；非 nil 空集合表示明确禁止该维度。
type EndpointPolicy struct {
	Status               AuthorizationStatus
	AllowedAccessMethods []AccessMethod
	AllowedPurposes      []Purpose
	AllowedContentKinds  []ContentKind
	AllowedStorageModes  []StorageMode
	MaxRetention         *time.Duration
	EffectiveAt          time.Time
	ExpiresAt            *time.Time
	EvidenceRef          string
}

// ProviderOutputPolicy 描述搜索 Provider 对自身输出授予的用途和留存权限。
type ProviderOutputPolicy struct {
	Status                   AuthorizationStatus
	AllowedPurposes          []Purpose
	AllowedContentKinds      []ContentKind
	AllowRequestProcessing   bool
	AllowCrossRequestCache   bool
	MaxRequestRetention      time.Duration
	MaxCacheRetention       time.Duration
	AllowedStorageModes      []StorageMode
	MaxPersistenceRetention time.Duration
	EffectiveAt               time.Time
	ExpiresAt                *time.Time
	EvidenceRef               string
}

// CollectionPolicyRequest 描述一次具体采集/留存意图。
type CollectionPolicyRequest struct {
	AccessMethod      AccessMethod
	Purpose           Purpose
	ContentKind       ContentKind
	StorageMode       StorageMode
	Retention         time.Duration
	CrossRequestCache bool
}

// PolicyDecision 是新闻领域授权评估结果，不是平台 Diagnostics 状态。
type PolicyDecision struct {
	Allowed      bool
	ReasonCode   string
	MaxRetention time.Duration
}

// EvaluateSourceCollectionPolicy 评估 Source 与可选 Endpoint 的权限交集。
// Provider 输出权利必须由调用方另行评估；本函数只负责发布来源及其入口。
func EvaluateSourceCollectionPolicy(source SourcePolicy, endpoint *EndpointPolicy, request CollectionPolicyRequest, now time.Time) PolicyDecision {
	if now.IsZero() || request.AccessMethod == "" || request.Purpose == "" || request.ContentKind == "" || request.StorageMode == "" || request.Retention < 0 || (request.CrossRequestCache && request.StorageMode != StorageTransient) {
		return denied("news.policy.invalid_request")
	}
	if source.Status != AuthorizationApproved && source.Status != AuthorizationRestricted {
		if source.Status == AuthorizationPending {
			return denied("news.policy.source_pending")
		}
		if source.Status == AuthorizationRevoked {
			return denied("news.policy.source_revoked")
		}
		return denied("news.policy.source_status_unknown")
	}
	if source.EvidenceRef == "" {
		return denied("news.policy.source_evidence_missing")
	}
	if !policyTimeActive(source.EffectiveAt, source.ExpiresAt, now) {
		return denied(policyTimeReason(source.EffectiveAt, source.ExpiresAt, now, "source"))
	}
	if source.MaxRetention < 0 {
		return denied("news.policy.source_policy_invalid")
	}
	if !containsPolicyValue(source.AllowedAccessMethods, request.AccessMethod) {
		return denied("news.policy.access_method_denied")
	}
	if !containsPolicyValue(source.AllowedPurposes, request.Purpose) {
		return denied("news.policy.purpose_denied")
	}
	if !containsPolicyValue(source.AllowedContentKinds, request.ContentKind) {
		return denied("news.policy.content_denied")
	}
	if !containsPolicyValue(source.AllowedStorageModes, request.StorageMode) {
		return denied("news.policy.storage_denied")
	}
	if request.StorageMode == StorageNone {
		if request.Retention != 0 || request.CrossRequestCache {
			return denied("news.policy.retention_invalid")
		}
	} else if source.MaxRetention == 0 || request.Retention == 0 || request.Retention > source.MaxRetention {
		return denied("news.policy.retention_exceeded")
	}

	maxRetention := source.MaxRetention
	if endpoint == nil {
		if request.StorageMode == StorageNone {
			return allowed(0)
		}
		return allowed(maxRetention)
	}
	if endpoint.Status != "" && endpoint.Status != AuthorizationApproved && endpoint.Status != AuthorizationRestricted {
		if endpoint.Status == AuthorizationPending {
			return denied("news.policy.endpoint_pending")
		}
		if endpoint.Status == AuthorizationRevoked {
			return denied("news.policy.endpoint_revoked")
		}
		return denied("news.policy.endpoint_status_unknown")
	}
	if endpoint.HasOverrides() && endpoint.EvidenceRef == "" {
		return denied("news.policy.endpoint_evidence_missing")
	}
	if !policyTimeActive(endpoint.EffectiveAt, endpoint.ExpiresAt, now) {
		return denied(policyTimeReason(endpoint.EffectiveAt, endpoint.ExpiresAt, now, "endpoint"))
	}

	if endpoint.AllowedAccessMethods != nil {
		if !isPolicySubset(endpoint.AllowedAccessMethods, source.AllowedAccessMethods) {
			return denied("news.policy.endpoint_expansion_requires_evidence")
		}
		if !containsPolicyValue(endpoint.AllowedAccessMethods, request.AccessMethod) {
			return denied("news.policy.endpoint_access_method_denied")
		}
	}
	if endpoint.AllowedPurposes != nil {
		if !isPolicySubset(endpoint.AllowedPurposes, source.AllowedPurposes) {
			return denied("news.policy.endpoint_expansion_requires_evidence")
		}
		if !containsPolicyValue(endpoint.AllowedPurposes, request.Purpose) {
			return denied("news.policy.endpoint_purpose_denied")
		}
	}
	if endpoint.AllowedContentKinds != nil {
		if !isPolicySubset(endpoint.AllowedContentKinds, source.AllowedContentKinds) {
			return denied("news.policy.endpoint_expansion_requires_evidence")
		}
		if !containsPolicyValue(endpoint.AllowedContentKinds, request.ContentKind) {
			return denied("news.policy.endpoint_content_denied")
		}
	}
	if endpoint.AllowedStorageModes != nil {
		if !isPolicySubset(endpoint.AllowedStorageModes, source.AllowedStorageModes) {
			return denied("news.policy.endpoint_expansion_requires_evidence")
		}
		if !containsPolicyValue(endpoint.AllowedStorageModes, request.StorageMode) {
			return denied("news.policy.endpoint_storage_denied")
		}
	}
	if endpoint.MaxRetention != nil {
		if *endpoint.MaxRetention < 0 {
			return denied("news.policy.endpoint_policy_invalid")
		}
		if *endpoint.MaxRetention > source.MaxRetention {
			return denied("news.policy.endpoint_expansion_requires_evidence")
		}
		maxRetention = *endpoint.MaxRetention
		if request.StorageMode != StorageNone && (maxRetention == 0 || request.Retention > maxRetention) {
			return denied("news.policy.endpoint_retention_exceeded")
		}
	}
	if request.StorageMode == StorageNone {
		return allowed(0)
	}
	return allowed(maxRetention)
}

// EvaluateProviderOutputPolicy evaluates the separate rights granted by a search Provider.
// It does not imply that the original publisher permits fetching or retaining the article.
func EvaluateProviderOutputPolicy(policy ProviderOutputPolicy, request CollectionPolicyRequest, now time.Time) PolicyDecision {
	if now.IsZero() || request.Purpose == "" || request.StorageMode == "" || request.Retention < 0 || (request.CrossRequestCache && request.StorageMode != StorageTransient) {
		return denied("news.policy.invalid_request")
	}
	if policy.Status != AuthorizationApproved && policy.Status != AuthorizationRestricted {
		if policy.Status == AuthorizationPending {
			return denied("news.policy.provider_pending")
		}
		if policy.Status == AuthorizationRevoked {
			return denied("news.policy.provider_revoked")
		}
		return denied("news.policy.provider_status_unknown")
	}
	if policy.EvidenceRef == "" {
		return denied("news.policy.provider_evidence_missing")
	}
	if !policyTimeActive(policy.EffectiveAt, policy.ExpiresAt, now) {
		return denied(policyTimeReason(policy.EffectiveAt, policy.ExpiresAt, now, "provider"))
	}
	if policy.MaxRequestRetention < 0 || policy.MaxCacheRetention < 0 || policy.MaxPersistenceRetention < 0 {
		return denied("news.policy.provider_policy_invalid")
	}
	if !containsPolicyValue(policy.AllowedPurposes, request.Purpose) {
		return denied("news.policy.provider_purpose_denied")
	}
	if !containsPolicyValue(policy.AllowedContentKinds, request.ContentKind) {
		return denied("news.policy.provider_content_denied")
	}
	if !policy.AllowRequestProcessing {
		return denied("news.policy.provider_processing_denied")
	}
	if request.StorageMode == StorageNone {
		if request.Retention != 0 || request.CrossRequestCache {
			return denied("news.policy.retention_invalid")
		}
		return allowed(0)
	}
	if request.Retention == 0 {
		return denied("news.policy.retention_invalid")
	}
	if request.StorageMode == StorageTransient {
		if request.CrossRequestCache {
			if !policy.AllowCrossRequestCache || policy.MaxCacheRetention == 0 || request.Retention > policy.MaxCacheRetention {
				return denied("news.policy.provider_cache_denied")
			}
			return allowed(policy.MaxCacheRetention)
		}
		if policy.MaxRequestRetention == 0 || request.Retention > policy.MaxRequestRetention {
			return denied("news.policy.provider_request_retention_exceeded")
		}
		return allowed(policy.MaxRequestRetention)
	}
	if request.CrossRequestCache {
		return denied("news.policy.invalid_request")
	}
	if !isPersistentStorageMode(request.StorageMode) || !containsPolicyValue(policy.AllowedStorageModes, request.StorageMode) {
		return denied("news.policy.provider_storage_denied")
	}
	if policy.MaxPersistenceRetention == 0 || request.Retention > policy.MaxPersistenceRetention {
		return denied("news.policy.provider_persistence_retention_exceeded")
	}
	for _, mode := range policy.AllowedStorageModes {
		if !isPersistentStorageMode(mode) {
			return denied("news.policy.provider_policy_invalid")
		}
	}
	return allowed(policy.MaxPersistenceRetention)
}

// EvaluateCandidateCollectionPolicy intersects Provider output rights with Source and Endpoint rights.
// A nil Provider policy is reserved for candidates obtained directly from a registered Source.
func EvaluateCandidateCollectionPolicy(provider *ProviderOutputPolicy, source SourcePolicy, endpoint *EndpointPolicy, request CollectionPolicyRequest, now time.Time) PolicyDecision {
	sourceDecision := EvaluateSourceCollectionPolicy(source, endpoint, request, now)
	if !sourceDecision.Allowed {
		return sourceDecision
	}
	if provider == nil {
		if request.CrossRequestCache {
			return denied("news.policy.provider_policy_missing")
		}
		return sourceDecision
	}
	providerDecision := EvaluateProviderOutputPolicy(*provider, request, now)
	if !providerDecision.Allowed {
		return providerDecision
	}
	if request.StorageMode == StorageNone {
		return allowed(0)
	}
	maxRetention := sourceDecision.MaxRetention
	if providerDecision.MaxRetention < maxRetention {
		maxRetention = providerDecision.MaxRetention
	}
	if request.Retention > maxRetention {
		return denied("news.policy.retention_intersection_exceeded")
	}
	return allowed(maxRetention)
}

func isPersistentStorageMode(mode StorageMode) bool {
	return mode == StoragePersistentMetadata || mode == StoragePersistentExcerpt || mode == StoragePersistentFullText
}

// HasOverrides reports whether an endpoint policy contains any independent restriction.
func (p EndpointPolicy) HasOverrides() bool {
	return p.Status != "" ||
		p.AllowedAccessMethods != nil ||
		p.AllowedPurposes != nil ||
		p.AllowedContentKinds != nil ||
		p.AllowedStorageModes != nil ||
		p.MaxRetention != nil ||
		!p.EffectiveAt.IsZero() ||
		p.ExpiresAt != nil
}

func denied(reason string) PolicyDecision {
	return PolicyDecision{Allowed: false, ReasonCode: reason}
}

func allowed(maxRetention time.Duration) PolicyDecision {
	return PolicyDecision{Allowed: true, MaxRetention: maxRetention}
}

func policyTimeActive(effectiveAt time.Time, expiresAt *time.Time, now time.Time) bool {
	if !effectiveAt.IsZero() && now.Before(effectiveAt) {
		return false
	}
	return expiresAt == nil || now.Before(*expiresAt)
}

func policyTimeReason(effectiveAt time.Time, expiresAt *time.Time, now time.Time, scope string) string {
	if !effectiveAt.IsZero() && now.Before(effectiveAt) {
		return "news.policy." + scope + "_not_yet_effective"
	}
	return "news.policy." + scope + "_expired"
}

func containsPolicyValue[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isPolicySubset[T comparable](candidate, parent []T) bool {
	for _, value := range candidate {
		if !containsPolicyValue(parent, value) {
			return false
		}
	}
	return true
}

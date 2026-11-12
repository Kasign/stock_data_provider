package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrEvidenceAuthorizationUnavailable = errors.New("news.evidence.authorization_unavailable")

// EvidenceReadAuthorizer 必须执行 NEWS-CTR-002 快照指纹验证及当前策略复核。
// 实际组合根应注入委托公共 GOV-CTR-006 Verifier 的实现；News 不复制 JCS。
type EvidenceReadAuthorizer interface {
	ExecuteWithCurrentPolicies(context.Context, domain.NewsAuthorizationBinding, domain.Observation, domain.NewsItemVersion, string, string, domain.AuthorizationContentLevel, domain.CurrentAuthorizationPolicySet) error
}

// EvidenceAtomicContentRepository 在当前策略指针行锁保护下完成授权回调和精确正文读取。
// 只有事务提交成功后才允许返回正文。
type EvidenceAtomicContentRepository interface {
	GetEvidenceContentWithCurrentAuthorization(
		context.Context,
		domain.NewsAuthorizationBinding,
		domain.Observation,
		domain.NewsItemVersion,
		time.Time,
		func(context.Context, domain.CurrentAuthorizationPolicySet) error,
	) (domain.ContentLayers, error)
}

// GetEvidence 组合单版本 PIT、最新授权快照、当前授权复核和内容级别裁剪。
// 在授权通过前，不构造任何可向跨模块返回的内容 DTO。
type GetEvidence struct {
	repository EvidencePITRepository
	atomicContent EvidenceAtomicContentRepository
	snapshots     AuthorizationSnapshotRepository
	authorizer EvidenceReadAuthorizer
	now        func() time.Time
}

// NewGetEvidence 创建单条 Evidence 查询 Facade。
func NewGetEvidence(
	repository EvidencePITRepository,
	snapshots AuthorizationSnapshotRepository,
	authorizer EvidenceReadAuthorizer,
) (*GetEvidence, error) {
	if repository == nil || snapshots == nil || authorizer == nil {
		return nil, ErrEvidenceAuthorizationUnavailable
	}
	atomicContent, ok := repository.(EvidenceAtomicContentRepository)
	if !ok {
		return nil, ErrEvidenceAuthorizationUnavailable
	}
	return &GetEvidence{repository: repository, atomicContent: atomicContent, snapshots: snapshots, authorizer: authorizer, now: time.Now}, nil
}

// Execute 只返回 PIT 可见且当前授权仍有效的证据；授权失败时返回空 DTO。
func (u *GetEvidence) Execute(ctx context.Context, request GetEvidenceRequest) (ResearchEvidenceRecord, error) {
	if u == nil || u.repository == nil || u.atomicContent == nil || u.snapshots == nil || u.authorizer == nil || u.now == nil || ctx == nil {
		return ResearchEvidenceRecord{}, ErrInvalidEvidenceQuery
	}
	if err := ctx.Err(); err != nil {
		return ResearchEvidenceRecord{}, err
	}
	now := u.now().UTC()
	if request.Validate(now) != nil {
		return ResearchEvidenceRecord{}, ErrInvalidEvidenceQuery
	}
	pit, err := u.repository.GetEvidenceVersionAt(ctx, request)
	if err != nil {
		return ResearchEvidenceRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return ResearchEvidenceRecord{}, err
	}
	if !validEvidencePITRecord(pit, request) {
		return ResearchEvidenceRecord{}, ErrInvalidEvidenceQuery
	}
	// 到期或历史期限未知的候选载荷不得通过读取路径继续披露，即使清理任务尚未执行。
	if pit.Observation.PayloadPurgedAt != nil || pit.Observation.RetentionUntil.IsZero() ||
		!pit.Observation.RetentionUntil.After(now) {
		pit.Observation.Candidate = domain.Candidate{}
	}
	resourceRef, err := domain.NewsEvidenceResourceRef(pit.Version.NewsID, pit.Version.VersionNo)
	if err != nil {
		return ResearchEvidenceRecord{}, ErrInvalidAuthorizationSnapshot
	}
	contentLevel := domain.AuthorizationContentLevel(request.RequestedContentLevel)
	binding, err := u.snapshots.GetLatestAuthorizationSnapshotForEvidence(
		ctx, request.CallerRef, resourceRef, string(request.PurposeCode), contentLevel,
	)
	if err != nil {
		return ResearchEvidenceRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return ResearchEvidenceRecord{}, err
	}
	if binding.CallerRef != request.CallerRef ||
		binding.ResourceRef != resourceRef ||
		binding.PurposeCode != string(request.PurposeCode) ||
		binding.RequestedContentLevel != contentLevel ||
		binding.SourceRef != pit.Observation.SourceID ||
		domain.ValidateNewsAuthorizationBindingForEvidence(binding, pit.Observation, pit.Version) != nil {
		return ResearchEvidenceRecord{}, ErrInvalidAuthorizationSnapshot
	}
	if err := ctx.Err(); err != nil {
		return ResearchEvidenceRecord{}, err
	}
	// 当前策略锁定、授权复核和正文读取必须共享一个 MariaDB 事务。
	content, err := u.atomicContent.GetEvidenceContentWithCurrentAuthorization(
		ctx, binding, pit.Observation, pit.Version, request.SystemAsOf,
		func(readCtx context.Context, current domain.CurrentAuthorizationPolicySet) error {
			return u.authorizer.ExecuteWithCurrentPolicies(
				readCtx, binding, pit.Observation, pit.Version, request.CallerRef,
				string(request.PurposeCode), contentLevel, current,
			)
		},
	)
	if err != nil {
		return ResearchEvidenceRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return ResearchEvidenceRecord{}, err
	}
	pit.Version.Content = content
	return researchEvidenceRecordFromPIT(pit, binding, request), nil
}

func validEvidencePITRecord(pit EvidencePITRecord, request GetEvidenceRequest) bool {
	observation, version := pit.Observation, pit.Version
	if observation.ID == "" || observation.NewsID != request.NewsID ||
		version.ID == "" || version.NewsID != request.NewsID ||
		(request.VersionID != "" && version.ID != request.VersionID) ||
		version.VersionNo < 1 || version.ObservationID != observation.ID ||
		pit.SourceRef == "" || pit.SourceRef != observation.SourceID ||
		pit.ProviderRef != observation.ProviderCode ||
		observation.ObservedAt.IsZero() || version.ObservedAt.IsZero() ||
		!validEvidenceUTC(observation.ObservedAt) || !validEvidenceUTC(version.ObservedAt) ||
		observation.ObservedAt.After(request.SystemAsOf) || version.ObservedAt.After(request.SystemAsOf) {
		return false
	}
	if !observation.RetentionUntil.IsZero() &&
		(!validEvidenceUTC(observation.RetentionUntil) || !observation.RetentionUntil.After(observation.ObservedAt)) {
		return false
	}
	if observation.PayloadPurgedAt != nil {
		if observation.PayloadPurgedAt.IsZero() || !validEvidenceUTC(*observation.PayloadPurgedAt) ||
			observation.RetentionUntil.IsZero() || observation.PayloadPurgedAt.Before(observation.RetentionUntil) ||
			!reflect.DeepEqual(observation.Candidate, domain.Candidate{}) {
			return false
		}
	}
	return true
}

func researchEvidenceRecordFromPIT(
	pit EvidencePITRecord,
	binding domain.NewsAuthorizationBinding,
	request GetEvidenceRequest,
) ResearchEvidenceRecord {
	kind := evidenceContentKind(request.RequestedContentLevel)
	layers := domain.RestrictContentLayers(pit.Version.Content, kind)
	record := ResearchEvidenceRecord{
		NewsID:                pit.Version.NewsID,
		ObservationID:         pit.Observation.ID,
		VersionID:             pit.Version.ID,
		VersionNo:             pit.Version.VersionNo,
		ContentFingerprint:    pit.Version.Fingerprint,
		SourceRef:             pit.Observation.SourceID,
		IdentityStatus:        string(pit.Observation.IdentityStatus),
		BusinessQualityStatus: EvidenceBusinessQualityUnknown,
		QualityStatus:         EvidenceQualityUnknown,
		PublishedAt:           cloneEvidenceTime(pit.Version.PublishedAt),
		SystemObservedAt:      pit.Version.ObservedAt.UTC(),
		ContentLevel:          actualEvidenceContentLevel(layers),
		AuthorizationRef:      binding.AuthorizationRef.AuthorizationRef,
		AuthorizationStatus:   string(binding.AuthorizationRef.Decision),
		SystemAsOf:            request.SystemAsOf.UTC(),
		RelationStatus:        EvidenceRelationNotProvided,
		RelationRefs:          []string{},
	}
	if pit.ProviderRef != "" {
		provider := pit.ProviderRef
		record.ProviderRef = &provider
	}
	if pit.Version.Title != "" {
		value := pit.Version.Title
		record.Title = &value
	}
	if pit.Version.PublisherName != "" {
		value := pit.Version.PublisherName
		record.PublisherName = &value
	}
	if pit.Version.PublisherDomain != "" {
		value := pit.Version.PublisherDomain
		record.PublisherDomain = &value
	}
	if pit.Observation.Candidate.PublishedAtRaw != "" {
		value := pit.Observation.Candidate.PublishedAtRaw
		record.PublishedAtRaw = &value
	}
	record.FetchedAt = cloneEvidenceTime(pit.Observation.Candidate.FetchedAt)
	record.NormalizedAt = cloneEvidenceTime(pit.Observation.Candidate.NormalizedAt)
	if excerpt := evidenceExcerpt(layers); excerpt != "" {
		record.ContentExcerpt = &excerpt
	}
	// available_at requires independent, source-backed provenance; observed_at is
	// knowledge time and must never be substituted for source availability time.
	return record
}

func evidenceContentKind(level EvidenceContentLevel) domain.ContentKind {
	switch level {
	case EvidenceContentSummary:
		return domain.ContentAbstract
	case EvidenceContentSnippet:
		return domain.ContentSnippet
	case EvidenceContentFull:
		return domain.ContentFullText
	default:
		return domain.ContentMetadata
	}
}

func actualEvidenceContentLevel(layers domain.ContentLayers) EvidenceContentLevel {
	level := EvidenceContentMetadata
	if layers.SourceSummary != "" {
		level = EvidenceContentSummary
	}
	if layers.Body != nil {
		if bodyLevel, ok := domain.AuthorizationContentLevelForKind(layers.Body.Kind); ok {
			switch bodyLevel {
			case domain.AuthorizationContentSummary:
				if level == EvidenceContentMetadata {
					level = EvidenceContentSummary
				}
			case domain.AuthorizationContentSnippet:
				level = EvidenceContentSnippet
			case domain.AuthorizationContentFull:
				level = EvidenceContentFull
			}
		}
	}
	return level
}

func evidenceExcerpt(layers domain.ContentLayers) string {
	parts := make([]string, 0, 2)
	if value := strings.TrimSpace(layers.SourceSummary); value != "" {
		parts = append(parts, value)
	}
	if layers.Body != nil {
		if value := strings.TrimSpace(layers.Body.Text); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n\n")
}

func cloneEvidenceTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
}

package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrProviderOutputPolicyUnavailable = errors.New("news.provider.output_policy_unavailable")
	ErrProviderOutputPolicyDenied      = errors.New("news.provider.output_policy_denied")
	ErrSourceCatalogUnavailable        = errors.New("news.source.catalog_unavailable")
)

// ProviderOutputPolicyBinding 是可信策略目录解析后的不可变版本绑定。
// Resolver 必须由 Server 组合根注入可信 Catalog 实现，不得由 Provider 响应或请求参数构造。
type ProviderOutputPolicyBinding struct {
	ProviderCode  string
	PolicyRef     string
	PolicyVersion string
	Policy        domain.ProviderOutputPolicy
}

// ProviderOutputPolicyResolver 只解析当前生效的 Provider 输出策略。
// 返回值必须来自受控配置/策略目录，并保证 Ref + Version 指向不可变版本。
type ProviderOutputPolicyResolver interface {
	ResolveProviderOutputPolicy(context.Context, string) (ProviderOutputPolicyBinding, error)
}

// IngestProviderCandidateRequest 描述将 Provider 搜索候选写入 News 的授权意图。
type IngestProviderCandidateRequest struct {
	SourceID          string
	EndpointID        string
	IdempotencyKey    string
	Candidate         domain.Candidate
	Purpose           domain.Purpose
	ContentKind       domain.ContentKind
	StorageMode       domain.StorageMode
	Retention         time.Duration
	CrossRequestCache bool
}

// IngestProviderCandidate 在写入身份/Observation 前，重新从可信 SourceCatalog 和
// ProviderPolicyResolver 读取策略，并执行 Provider ∩ Source ∩ Endpoint 权限评估。
// 任一策略缺失、版本缺失或权限不满足时 fail-closed，不触碰 Repository。
type IngestProviderCandidate struct {
	ingest   *candidateIngest
	sources  SourceCatalog
	policies ProviderOutputPolicyResolver
	now      func() time.Time
}

// NewIngestProviderCandidate 是可由 Server 组合根调用的公开构造器。底层候选写入器
// 始终在 app 包内创建，Provider 候选仍必须经过可信 Provider/Source/Endpoint 策略评估。
func NewIngestProviderCandidate(
	repository IdentityRepository,
	newID func() string,
	sources SourceCatalog,
	policies ProviderOutputPolicyResolver,
) (*IngestProviderCandidate, error) {
	if repository == nil || newID == nil || sources == nil || policies == nil {
		return nil, ErrProviderOutputPolicyUnavailable
	}
	return &IngestProviderCandidate{
		ingest: newCandidateIngest(repository, newID), sources: sources, policies: policies, now: time.Now,
	}, nil
}

func (u *IngestProviderCandidate) Execute(ctx context.Context, request IngestProviderCandidateRequest) (domain.Observation, error) {
	if u == nil || u.ingest == nil || u.sources == nil || u.policies == nil || ctx == nil {
		return domain.Observation{}, ErrInvalidRequest
	}
	request.SourceID = strings.TrimSpace(request.SourceID)
	request.EndpointID = strings.TrimSpace(request.EndpointID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.Candidate.ProviderCode = strings.TrimSpace(request.Candidate.ProviderCode)
	if request.SourceID == "" || request.EndpointID == "" || request.IdempotencyKey == "" ||
		request.Candidate.ProviderCode == "" || request.Purpose == "" || request.ContentKind == "" ||
		request.StorageMode == "" || request.Retention <= 0 {
		return domain.Observation{}, ErrInvalidRequest
	}

	registration, err := u.sources.LookupEndpoint(ctx, request.SourceID, request.EndpointID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return domain.Observation{}, ctxErr
	}
	if err != nil {
		if errors.Is(err, ErrSourceEndpointNotFound) {
			return domain.Observation{}, ErrSourceEndpointNotFound
		}
		return domain.Observation{}, errors.Join(ErrSourceCatalogUnavailable, err)
	}
	if registration.SourceID != request.SourceID ||
		registration.EndpointID != request.EndpointID || !registration.Enabled {
		return domain.Observation{}, ErrSourceEndpointNotFound
	}
	if registration.SourcePolicyVersionID == "" ||
		(registration.EndpointPolicy != nil && registration.EndpointPolicyVersionID == "") ||
		(registration.EndpointPolicy == nil && registration.EndpointPolicyVersionID != "") {
		return domain.Observation{}, ErrProviderOutputPolicyUnavailable
	}

	providerBinding, err := u.policies.ResolveProviderOutputPolicy(ctx, request.Candidate.ProviderCode)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return domain.Observation{}, ctxErr
	}
	if err != nil || strings.TrimSpace(providerBinding.ProviderCode) != request.Candidate.ProviderCode ||
		strings.TrimSpace(providerBinding.PolicyRef) == "" || strings.TrimSpace(providerBinding.PolicyVersion) == "" {
		return domain.Observation{}, ErrProviderOutputPolicyUnavailable
	}

	evaluatedAt := u.now().UTC()
	policyRequest := domain.CollectionPolicyRequest{
		AccessMethod:      registration.AccessMethod,
		Purpose:           request.Purpose,
		ContentKind:       request.ContentKind,
		StorageMode:       request.StorageMode,
		Retention:         request.Retention,
		CrossRequestCache: request.CrossRequestCache,
	}
	trace := ObservationPolicyTrace{
		SourcePolicyVersionID:         registration.SourcePolicyVersionID,
		ProviderOutputPolicyRef:       strings.TrimSpace(providerBinding.PolicyRef),
		ProviderOutputPolicyVersionID: strings.TrimSpace(providerBinding.PolicyVersion),
		PolicyEvaluatedAt:             evaluatedAt,
		Retention:                     request.Retention,
	}
	if registration.EndpointPolicy != nil {
		trace.EndpointID = registration.EndpointID
		trace.EndpointPolicyVersionID = registration.EndpointPolicyVersionID
	}
	return u.ingest.executeWithPolicyTraceAndCheck(
		ctx, request.SourceID, request.IdempotencyKey, request.Candidate, registration.URLPolicy, trace,
		func(tx IdentityTransaction) error {
			policyTx, ok := tx.(CurrentIngestPolicyIdentityTransaction)
			if !ok {
				return ErrProviderOutputPolicyUnavailable
			}
			sourcePolicy, endpointPolicy, currentProviderPolicy, policyErr := policyTx.LockCurrentIngestPolicies(
				ctx,
				request.SourceID, registration.SourcePolicyVersionID,
				registration.EndpointID, registration.EndpointPolicyVersionID,
				providerBinding.PolicyRef, providerBinding.PolicyVersion,
			)
			if policyErr != nil || currentProviderPolicy == nil {
				return errors.Join(ErrProviderOutputPolicyUnavailable, policyErr)
			}
			decision := domain.EvaluateCandidateCollectionPolicy(
				currentProviderPolicy, sourcePolicy, endpointPolicy, policyRequest, u.now().UTC(),
			)
			if !decision.Allowed {
				return errors.Join(ErrProviderOutputPolicyDenied, errors.New(decision.ReasonCode))
			}
			return nil
		},
	)
}

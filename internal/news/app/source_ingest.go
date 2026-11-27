package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

// CurrentIngestPolicyIdentityTransaction 在 Identity 写事务内锁定并读取当前策略组合。
// 生产实现必须使用与 Observation 写入相同的数据库事务。
type CurrentIngestPolicyIdentityTransaction interface {
	LockCurrentIngestPolicies(
		context.Context,
		string, string,
		string, string,
		string, string,
	) (domain.SourcePolicy, *domain.EndpointPolicy, *domain.ProviderOutputPolicy, error)
}

var (
	ErrSourcePersistencePolicyUnavailable = errors.New("news: source persistence policy unavailable")
	ErrSourcePersistencePolicyDenied      = errors.New("news: source persistence policy denied")
)

// IngestCollectedSourceRequest 将一次已完成的 Feed 采集结果转为 Observation 元数据。
// Observation 仅保存候选元数据；Feed 正文/摘要不在本用例中持久化。
type IngestCollectedSourceRequest struct {
	Collection        CollectSourceResult
	IdempotencyPrefix string
	Purpose           domain.Purpose
	Retention         time.Duration
}

// IngestCollectedSource 在每条 Observation 写事务内锁定并复核当前 Source/Endpoint 策略，
// 再评估元数据持久化权限，避免授权预检与持久化之间发生撤销竞态。
type IngestCollectedSource struct {
	ingest    *candidateIngest
	sources   SourceCatalog
	providers ProviderOutputPolicyResolver
	now       func() time.Time
}

// NewIngestCollectedSource 是可由 Server 组合根调用的公开构造器。底层候选写入器
// 始终在 app 包内创建，不向调用方暴露绕过 Source 授权的写入入口。
func NewIngestCollectedSource(
	repository IdentityRepository,
	newID func() string,
	sources SourceCatalog,
) (*IngestCollectedSource, error) {
	return NewIngestCollectedSourceWithProviderPolicies(repository, newID, sources, nil)
}

// NewIngestCollectedSourceWithProviderPolicies 为 direct source 注册增加自身
// 输出策略绑定。provider resolver 为空时，任何带 Provider 绑定的登记都会
// fail-closed；无 Provider 绑定的 RSS/Licensed 登记保持既有行为。
func NewIngestCollectedSourceWithProviderPolicies(
	repository IdentityRepository,
	newID func() string,
	sources SourceCatalog,
	providers ProviderOutputPolicyResolver,
) (*IngestCollectedSource, error) {
	if repository == nil || newID == nil || sources == nil {
		return nil, ErrSourcePersistencePolicyUnavailable
	}
	return &IngestCollectedSource{
		ingest: newCandidateIngest(repository, newID), sources: sources, providers: providers, now: time.Now,
	}, nil
}

func (u *IngestCollectedSource) Execute(ctx context.Context, request IngestCollectedSourceRequest) ([]domain.Observation, error) {
	if u == nil || u.ingest == nil || u.sources == nil || ctx == nil {
		return nil, ErrInvalidRequest
	}
	result := request.Collection
	sourceID := strings.TrimSpace(result.SourceID)
	endpointID := strings.TrimSpace(result.EndpointID)
	prefix := strings.TrimSpace(request.IdempotencyPrefix)
	if sourceID == "" || endpointID == "" || prefix == "" ||
		request.Purpose == "" || result.CollectionPurpose == "" ||
		request.Purpose != result.CollectionPurpose || request.Retention <= 0 ||
		result.PolicyEvaluatedAt.IsZero() || strings.TrimSpace(result.SourcePolicyVersionID) == "" ||
		!result.hasValidCollectionSeal() {
		return nil, ErrInvalidRequest
	}

	registration, err := u.sources.LookupEndpoint(ctx, sourceID, endpointID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, errors.Join(ErrSourcePersistencePolicyUnavailable, err)
	}
	if registration.SourceID != sourceID || registration.EndpointID != endpointID ||
		!registration.Enabled || registration.SourcePolicyVersionID != result.SourcePolicyVersionID ||
		registration.EndpointPolicyVersionID != result.EndpointPolicyVersionID ||
		(registration.EndpointPolicy == nil && result.EndpointPolicyVersionID != "") ||
		(registration.EndpointPolicy != nil && result.EndpointPolicyVersionID == "") {
		return nil, ErrSourcePersistencePolicyUnavailable
	}

	providerCode := strings.TrimSpace(registration.ProviderCode)
	providerRef := strings.TrimSpace(registration.ProviderOutputPolicyRef)
	providerVersion := strings.TrimSpace(registration.ProviderOutputPolicyVersionID)
	if providerCode == "" {
		if providerRef != "" || providerVersion != "" {
			return nil, ErrProviderOutputPolicyUnavailable
		}
	} else {
		if providerRef == "" || providerVersion == "" || u.providers == nil {
			return nil, ErrProviderOutputPolicyUnavailable
		}
		binding, err := u.providers.ResolveProviderOutputPolicy(ctx, providerCode)
		if err != nil || strings.TrimSpace(binding.ProviderCode) != providerCode ||
			strings.TrimSpace(binding.PolicyRef) != providerRef ||
			strings.TrimSpace(binding.PolicyVersion) != providerVersion {
			return nil, ErrProviderOutputPolicyUnavailable
		}
	}

	now := u.now().UTC()
	if result.PolicyEvaluatedAt.After(now) {
		return nil, ErrSourcePersistencePolicyUnavailable
	}
	policyRequest := domain.CollectionPolicyRequest{
		AccessMethod: registration.AccessMethod,
		Purpose:      request.Purpose,
		ContentKind:  domain.ContentAbstract,
		StorageMode:  domain.StoragePersistentExcerpt,
		Retention:    request.Retention,
	}

	trace := ObservationPolicyTrace{
		SourcePolicyVersionID: result.SourcePolicyVersionID,
		PolicyEvaluatedAt:     result.PolicyEvaluatedAt.UTC(),
		Retention:             request.Retention,
	}
	if registration.EndpointPolicy != nil {
		trace.EndpointID = endpointID
		trace.EndpointPolicyVersionID = result.EndpointPolicyVersionID
	}
	trace.ProviderOutputPolicyRef = providerRef
	trace.ProviderOutputPolicyVersionID = providerVersion

	// 先完整预检批次，避免前几条已写入后才发现后续条目混入其他 Provider 候选。
	for _, entry := range result.Entries {
		entryProviderCode := strings.TrimSpace(entry.Candidate.ProviderCode)
		if providerCode == "" {
			if entryProviderCode != "" {
				return nil, ErrProviderOutputPolicyRequired
			}
			continue
		}
		if entryProviderCode != "" && entryProviderCode != providerCode {
			return nil, ErrProviderOutputPolicyRequired
		}
	}

	checkPersistencePolicy := func(tx IdentityTransaction) error {
		policyTx, ok := tx.(CurrentIngestPolicyIdentityTransaction)
		if !ok {
			return ErrSourcePersistencePolicyUnavailable
		}
		sourcePolicy, endpointPolicy, providerPolicy, policyErr := policyTx.LockCurrentIngestPolicies(
			ctx, sourceID, result.SourcePolicyVersionID, endpointID, result.EndpointPolicyVersionID, providerRef, providerVersion,
		)
		if policyErr != nil {
			return errors.Join(ErrSourcePersistencePolicyUnavailable, policyErr)
		}
		if (providerCode == "") != (providerPolicy == nil) {
			return errors.Join(ErrProviderOutputPolicyUnavailable, ErrSourcePersistencePolicyUnavailable)
		}
		decision := domain.EvaluateCandidateCollectionPolicy(
			providerPolicy, sourcePolicy, endpointPolicy, policyRequest, u.now().UTC(),
		)
		if !decision.Allowed {
			return errors.Join(ErrSourcePersistencePolicyDenied, errors.New(decision.ReasonCode))
		}
		return nil
	}
	if len(result.Entries) == 0 {
		err := u.ingest.repository.WithinIdentityTransaction(ctx, checkPersistencePolicy)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err != nil {
			return nil, errors.Join(ErrIdentityTransaction, err)
		}
		return []domain.Observation{}, nil
	}

	observations := make([]domain.Observation, 0, len(result.Entries))
	for _, entry := range result.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate := entry.Candidate
		candidate.ProviderCode = providerCode
		// Observation 只保存候选元数据；来源摘要按授权层级保存，未识别正文不升级为全文。
		candidate.ProviderSummary = ""
		content := domain.RestrictContentLayers(entry.Content, policyRequest.ContentKind)
		contentFingerprint := domain.BuildVersionFingerprint(candidate, content)
		canonicalURL := ""
		if candidate.RawURL != "" {
			if decision := domain.EvaluateSourceURL(candidate.RawURL, registration.URLPolicy); decision.Allowed {
				canonicalURL = decision.CanonicalURL
			}
		}
		identityDigest := sha256.Sum256([]byte(
			sourceID + "\x00" + candidate.SourceItemID + "\x00" + candidate.RawURL + "\x00" +
				canonicalURL + "\x00" + contentFingerprint,
		))
		idempotencyKey := prefix + "/" + hex.EncodeToString(identityDigest[:])
		observation, ingestErr := u.ingest.executeWithPolicyTraceAndCheckAndContent(
			ctx, sourceID, idempotencyKey, candidate, content,
			registration.URLPolicy, trace, checkPersistencePolicy,
		)
		if ingestErr != nil {
			return nil, ingestErr
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

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

// SourcePolicyIdentityTransaction 在 Identity 写事务内锁定并读取当前 Source/Endpoint 策略版本。
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
	ingest  *candidateIngest
	sources SourceCatalog
	now     func() time.Time
}

// NewIngestCollectedSource 是可由 Server 组合根调用的公开构造器。底层候选写入器
// 始终在 app 包内创建，不向调用方暴露绕过 Source 授权的写入入口。
func NewIngestCollectedSource(
	repository IdentityRepository,
	newID func() string,
	sources SourceCatalog,
) (*IngestCollectedSource, error) {
	if repository == nil || newID == nil || sources == nil {
		return nil, ErrSourcePersistencePolicyUnavailable
	}
	return &IngestCollectedSource{
		ingest: newCandidateIngest(repository, newID), sources: sources, now: time.Now,
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

	now := u.now().UTC()
	if result.PolicyEvaluatedAt.After(now) {
		return nil, ErrSourcePersistencePolicyUnavailable
	}
	policyRequest := domain.CollectionPolicyRequest{
		AccessMethod: registration.AccessMethod,
		Purpose:      request.Purpose,
		ContentKind:  domain.ContentMetadata,
		StorageMode:  domain.StoragePersistentMetadata,
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

	// 先完整预检批次，避免前几条已写入后才发现后续条目混入 Provider 候选。
	for _, entry := range result.Entries {
		if strings.TrimSpace(entry.Candidate.ProviderCode) != "" {
			return nil, ErrProviderOutputPolicyRequired
		}
	}

	checkPersistencePolicy := func(tx IdentityTransaction) error {
		policyTx, ok := tx.(CurrentIngestPolicyIdentityTransaction)
		if !ok {
			return ErrSourcePersistencePolicyUnavailable
		}
		sourcePolicy, endpointPolicy, _, policyErr := policyTx.LockCurrentIngestPolicies(
			ctx, sourceID, result.SourcePolicyVersionID, endpointID, result.EndpointPolicyVersionID, "", "",
		)
		if policyErr != nil {
			return errors.Join(ErrSourcePersistencePolicyUnavailable, policyErr)
		}
		decision := domain.EvaluateSourceCollectionPolicy(sourcePolicy, endpointPolicy, policyRequest, u.now().UTC())
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
		// Observation 当前只持久化元数据；不得将 Provider 摘要作为来源摘要保存。
		candidate.ProviderSummary = ""
		fingerprint := sha256.Sum256([]byte(
			candidate.SourceItemID + "\x00" + candidate.RawURL + "\x00" +
				candidate.Title + "\x00" + candidate.PublishedAtRaw,
		))
		idempotencyKey := prefix + "/" + hex.EncodeToString(fingerprint[:])
		observation, ingestErr := u.ingest.executeWithPolicyTraceAndCheck(
			ctx, sourceID, idempotencyKey, candidate, registration.URLPolicy, trace,
checkPersistencePolicy,
		)
		if ingestErr != nil {
			return nil, ingestErr
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

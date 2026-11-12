package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrIdentityTransaction               = errors.New("news: identity transaction failed")
	ErrObservationIdempotencyConflict    = errors.New("news: observation idempotency key reused with different payload")
	ErrObservationPayloadPurged          = errors.New("news: observation payload has been purged")
	ErrObservationRetentionResultInvalid = errors.New("news: invalid observation retention purge result")
	ErrProviderOutputPolicyRequired      = errors.New("news: provider output policy evaluation required")
)

// IdentityRepository 要求在单一事务中读取身份、创建主体/凭据并追加 Observation。
// 生产实现必须在解析身份前取得按 SourceID 协调的跨实例写锁，并持有至事务提交；
// 否则两个事务可能同时查无凭据并创建重复主体。数据库还必须约束
// (source_id, identifier_type, identifier_hash) 唯一、observation.id 唯一，
// 并对全局 idempotency_key 建立唯一约束。唯一约束冲突后需重跑完整事务并重新解析身份。
type IdentityRepository interface {
	WithinIdentityTransaction(context.Context, func(IdentityTransaction) error) error
}

// IdentityTransaction 是一次身份处理事务中的最小持久化端口。
type IdentityTransaction interface {
	FindIdentityCredentials(context.Context, string, []domain.IdentityCredential) ([]domain.IdentityCredential, error)
	CreateNewsItem(context.Context, string) (string, error)
	SaveIdentityCredentials(context.Context, []domain.IdentityCredential) error
	// AppendObservation 对全局唯一的 IdempotencyKey：相同候选载荷返回既有记录，载荷不同返回冲突。
	AppendObservation(context.Context, domain.Observation) (domain.Observation, error)
}

// candidateIngest 负责将一个已发现候选写入身份处理流程；不负责真实抓取或跨来源去重。
type candidateIngest struct {
	repository IdentityRepository
	now        func() time.Time
	newID      func() string
}

// newCandidateIngest 创建仅供 News Application 包内使用的候选身份处理器。
func newCandidateIngest(repository IdentityRepository, newID func() string) *candidateIngest {
	return &candidateIngest{repository: repository, now: time.Now, newID: newID}
}

// candidateIngest 是 News Application 包内的底层身份处理器，不作为跨模块入口。
// 生产调用必须经 IngestCollectedSource 或 IngestProviderCandidate 完成可信授权与保留期评估。
type ObservationPolicyTrace struct {
	SourcePolicyVersionID         string
	EndpointID                    string
	EndpointPolicyVersionID       string
	ProviderOutputPolicyRef       string
	ProviderOutputPolicyVersionID string
	PolicyEvaluatedAt             time.Time
	Retention                     time.Duration
}

// executeUnscoped 仅供包内底层测试使用；生产调用不得绕过受控 Ingest 用例。
func (u *candidateIngest) executeUnscoped(ctx context.Context, sourceID, idempotencyKey string, candidate domain.Candidate, policy domain.SourceURLPolicy) (domain.Observation, error) {
	if strings.TrimSpace(candidate.ProviderCode) != "" {
		return domain.Observation{}, ErrProviderOutputPolicyRequired
	}
	return u.executeCore(ctx, sourceID, idempotencyKey, candidate, policy, ObservationPolicyTrace{}, nil)
}

// executeWithPolicyTrace 接收 Source/Endpoint 可信授权结果，并要求实际保留期限为正数。
// Provider 候选必须经 IngestProviderCandidate 的独立策略 Resolver 评估。
func (u *candidateIngest) executeWithPolicyTrace(ctx context.Context, sourceID, idempotencyKey string, candidate domain.Candidate, policy domain.SourceURLPolicy, trace ObservationPolicyTrace) (domain.Observation, error) {
	return u.executeWithPolicyTraceAndCheck(ctx, sourceID, idempotencyKey, candidate, policy, trace, nil)
}

// executeWithPolicyTraceAndCheck 在身份写入事务内执行额外的授权校验。
// 校验回调与 Observation/Identity 写入共享同一个数据库事务。
func (u *candidateIngest) executeWithPolicyTraceAndCheck(
	ctx context.Context,
	sourceID, idempotencyKey string,
	candidate domain.Candidate,
	policy domain.SourceURLPolicy,
	trace ObservationPolicyTrace,
	beforePersist func(IdentityTransaction) error,
) (domain.Observation, error) {
	trace.SourcePolicyVersionID = strings.TrimSpace(trace.SourcePolicyVersionID)
	trace.EndpointID = strings.TrimSpace(trace.EndpointID)
	trace.EndpointPolicyVersionID = strings.TrimSpace(trace.EndpointPolicyVersionID)
	trace.ProviderOutputPolicyRef = strings.TrimSpace(trace.ProviderOutputPolicyRef)
	trace.ProviderOutputPolicyVersionID = strings.TrimSpace(trace.ProviderOutputPolicyVersionID)
	if trace.SourcePolicyVersionID == "" || trace.PolicyEvaluatedAt.IsZero() || trace.Retention <= 0 {
		return domain.Observation{}, ErrInvalidRequest
	}
	if (strings.TrimSpace(candidate.ProviderCode) != "" ||
		trace.ProviderOutputPolicyRef != "" || trace.ProviderOutputPolicyVersionID != "") && beforePersist == nil {
		return domain.Observation{}, ErrProviderOutputPolicyRequired
	}
	trace.PolicyEvaluatedAt = trace.PolicyEvaluatedAt.UTC()
	return u.executeCore(ctx, sourceID, idempotencyKey, candidate, policy, trace, beforePersist)
}

func (u *candidateIngest) executeCore(ctx context.Context, sourceID, idempotencyKey string, candidate domain.Candidate, policy domain.SourceURLPolicy, trace ObservationPolicyTrace, beforePersist func(IdentityTransaction) error) (domain.Observation, error) {
	if u == nil || u.repository == nil || u.newID == nil || ctx == nil {
		return domain.Observation{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return domain.Observation{}, err
	}
	sourceID = strings.TrimSpace(sourceID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if sourceID == "" || idempotencyKey == "" {
		return domain.Observation{}, ErrInvalidRequest
	}

	observedAt := u.now().UTC()
	candidate = domain.NormalizeCandidate(candidate, observedAt, candidate.QueryDimension)
	// CanonicalURL 是策略决策产物，不能信任调用方预填值。
	candidate.CanonicalURL = ""
	if candidate.RawURL != "" {
		decision := domain.EvaluateSourceURL(candidate.RawURL, policy)
		if decision.Allowed {
			candidate.CanonicalURL = decision.CanonicalURL
		}
	}
	observation := domain.Observation{
		ID:                            strings.TrimSpace(u.newID()),
		IdempotencyKey:                idempotencyKey,
		SourceID:                      sourceID,
		ProviderCode:                  candidate.ProviderCode,
		QueryDimension:                candidate.QueryDimension,
		EndpointID:                    trace.EndpointID,
		SourcePolicyVersionID:         trace.SourcePolicyVersionID,
		EndpointPolicyVersionID:       trace.EndpointPolicyVersionID,
		ProviderOutputPolicyRef:       trace.ProviderOutputPolicyRef,
		ProviderOutputPolicyVersionID: trace.ProviderOutputPolicyVersionID,
		PolicyEvaluatedAt:             trace.PolicyEvaluatedAt,
		Candidate:                     candidate,
		ObservedAt:                    observedAt,
	}
	if trace.Retention > 0 {
		observation.RetentionUntil = observedAt.Add(trace.Retention).UTC()
	}
	if observation.ID == "" {
		return domain.Observation{}, ErrInvalidRequest
	}

	credentials := []domain.IdentityCredential(nil)
	if strings.TrimSpace(candidate.SourceItemID) == "" && strings.TrimSpace(candidate.RawURL) == "" {
		observation.IdentityStatus = domain.ObservationUnidentified
		observation.ReasonCode = "news.identity.credential_required"
	} else {
		built, err := domain.BuildIdentityCredentials(sourceID, candidate.SourceItemID, candidate.RawURL, policy)
		if err != nil {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = "news.identity.credential_rejected"
		} else {
			credentials = built
			for i := range credentials {
				credentials[i].ObservedAt = observedAt
			}
		}
	}

	var persisted domain.Observation
	err := u.repository.WithinIdentityTransaction(ctx, func(tx IdentityTransaction) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if beforePersist != nil {
			if err := beforePersist(tx); err != nil {
				return err
			}
		}
		if len(credentials) == 0 {
			var appendErr error
			persisted, appendErr = tx.AppendObservation(ctx, observation)
			return appendErr
		}

		existing, err := tx.FindIdentityCredentials(ctx, sourceID, credentials)
		if err != nil {
			return err
		}
		resolution := domain.ResolveIdentity(sourceID, credentials, existing)
		switch resolution.Status {
		case domain.IdentityConflict:
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = resolution.Reason
		case domain.IdentityMatched:
			observation.IdentityStatus = domain.ObservationMatched
			observation.NewsID = resolution.NewsID
		case domain.IdentityNew:
			newsID, createErr := tx.CreateNewsItem(ctx, sourceID)
			if createErr != nil {
				return createErr
			}
			if strings.TrimSpace(newsID) == "" {
				return errors.New("news.identity.created_item_id_missing")
			}
			observation.IdentityStatus = domain.ObservationNew
			observation.NewsID = newsID
		default:
			return errors.New("news.identity.resolution_unknown")
		}

		if observation.IdentityStatus == domain.ObservationNew || observation.IdentityStatus == domain.ObservationMatched {
			toSave := credentialsNotYetStored(credentials, existing)
			for i := range toSave {
				toSave[i].NewsID = observation.NewsID
			}
			if len(toSave) > 0 {
				if err := tx.SaveIdentityCredentials(ctx, toSave); err != nil {
					return err
				}
			}
		}
		var appendErr error
		persisted, appendErr = tx.AppendObservation(ctx, observation)
		return appendErr
	})
	// Repository 可能在事务提交后才观察到取消，或其实现未在返回前传播取消。
	// 此處只保證呼叫方不會收到成功結果；若提交已完成，不代表資料庫寫入已回滾。
	if ctxErr := ctx.Err(); ctxErr != nil {
		return domain.Observation{}, ctxErr
	}
	if err != nil {
		return domain.Observation{}, errors.Join(ErrIdentityTransaction, err)
	}
	return persisted, nil
}

func credentialsNotYetStored(candidates, existing []domain.IdentityCredential) []domain.IdentityCredential {
	result := make([]domain.IdentityCredential, 0, len(candidates))
	for _, candidate := range candidates {
		found := false
		for _, stored := range existing {
			if stored.SourceID == candidate.SourceID && stored.Type == candidate.Type &&
				stored.Hash == candidate.Hash && stored.Value == candidate.Value &&
				(stored.Status == domain.IdentifierActive || stored.Status == domain.IdentifierHistorical) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return result
}

// effectiveObservationRetention 以实际保留时长比较，避免重试时间变化导致相同授权期限冲突。
func effectiveObservationRetention(observation domain.Observation) time.Duration {
	if observation.RetentionUntil.IsZero() || observation.ObservedAt.IsZero() || observation.RetentionUntil.Before(observation.ObservedAt) {
		return 0
	}
	return observation.RetentionUntil.Sub(observation.ObservedAt)
}

// SameObservationPayload 判断同一幂等键的候选载荷是否一致；不比较生成 ID、采集时间和解析结果。
func SameObservationPayload(a, b domain.Observation) bool {
	aCandidate, bCandidate := a.Candidate, b.Candidate
	aCandidate.ObservedAt = time.Time{}
	bCandidate.ObservedAt = time.Time{}
	aCandidate.FetchedAt = nil
	bCandidate.FetchedAt = nil
	aCandidate.NormalizedAt = nil
	bCandidate.NormalizedAt = nil
	return a.IdempotencyKey == b.IdempotencyKey &&
		a.SourceID == b.SourceID &&
		a.ProviderCode == b.ProviderCode &&
		a.QueryDimension == b.QueryDimension &&
		a.EndpointID == b.EndpointID &&
		a.SourcePolicyVersionID == b.SourcePolicyVersionID &&
		a.EndpointPolicyVersionID == b.EndpointPolicyVersionID &&
		a.ProviderOutputPolicyRef == b.ProviderOutputPolicyRef &&
		a.ProviderOutputPolicyVersionID == b.ProviderOutputPolicyVersionID &&
		reflect.DeepEqual(aCandidate, bCandidate) &&
		effectiveObservationRetention(a) == effectiveObservationRetention(b)
}

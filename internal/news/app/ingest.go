package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrIdentityTransaction               = errors.New("news: formal ingest transaction failed")
	ErrObservationIdempotencyConflict    = errors.New("news: observation idempotency key reused with different payload")
	ErrObservationPayloadPurged          = errors.New("news: observation payload has been purged")
	ErrObservationRetentionResultInvalid = errors.New("news: invalid observation retention purge result")
	ErrProviderOutputPolicyRequired      = errors.New("news: provider output policy evaluation required")
	ErrNewsIdentityConflict              = errors.New("news: URL and source-native identity conflict")
	ErrCanonicalURLHashCollision         = errors.New("news: canonical URL hash collision")
)

// IdentityRepository 在一个 MariaDB 事务中完成身份解析、新闻主体、来源身份、Observation 和内容版本写入。
type IdentityRepository interface {
	WithinIdentityTransaction(context.Context, func(IdentityTransaction) error) error
}

// IdentityTransaction 的调用顺序固定为：策略锁、全局 URL 锁、来源原生 ID 锁、主体/版本/Observation 写入。
type IdentityTransaction interface {
	FindNewsItemByCanonicalURL(context.Context, string, string) (domain.NewsItem, bool, error)
	FindNewsItemByID(context.Context, string) (domain.NewsItem, bool, error)
	FindIdentityCredentials(context.Context, string, []domain.IdentityCredential) ([]domain.IdentityCredential, error)
	CreateNewsItem(context.Context, domain.NewsItem) error
	UpdateNewsItem(context.Context, domain.NewsItem) error
	SaveIdentityCredentials(context.Context, []domain.IdentityCredential) error
	AppendObservation(context.Context, domain.Observation) (domain.Observation, error)
	LatestNewsItemVersion(context.Context, string) (domain.NewsItemVersion, bool, error)
	AppendNewsItemVersion(context.Context, domain.NewsItemVersion) error
}

// candidateIngest 是正式新闻入库的唯一底层用例。
type candidateIngest struct {
	repository IdentityRepository
	now        func() time.Time
	newID      func() string
}

func newCandidateIngest(repository IdentityRepository, newID func() string) *candidateIngest {
	return &candidateIngest{repository: repository, now: time.Now, newID: newID}
}

type ObservationPolicyTrace struct {
	SourcePolicyVersionID         string
	EndpointID                    string
	EndpointPolicyVersionID       string
	ProviderOutputPolicyRef       string
	ProviderOutputPolicyVersionID string
	PolicyEvaluatedAt             time.Time
	Retention                     time.Duration
}

func (u *candidateIngest) executeUnscoped(ctx context.Context, sourceID, idempotencyKey string, candidate domain.Candidate, policy domain.SourceURLPolicy) (domain.Observation, error) {
	if strings.TrimSpace(candidate.ProviderCode) != "" {
		return domain.Observation{}, ErrProviderOutputPolicyRequired
	}
	return u.executeCore(ctx, sourceID, idempotencyKey, candidate, domain.ContentLayers{}, policy, ObservationPolicyTrace{}, nil)
}

func (u *candidateIngest) executeWithPolicyTrace(ctx context.Context, sourceID, idempotencyKey string, candidate domain.Candidate, policy domain.SourceURLPolicy, trace ObservationPolicyTrace) (domain.Observation, error) {
	return u.executeWithPolicyTraceAndCheck(ctx, sourceID, idempotencyKey, candidate, policy, trace, nil)
}

func (u *candidateIngest) executeWithPolicyTraceAndCheck(
	ctx context.Context,
	sourceID, idempotencyKey string,
	candidate domain.Candidate,
	policy domain.SourceURLPolicy,
	trace ObservationPolicyTrace,
	beforePersist func(IdentityTransaction) error,
) (domain.Observation, error) {
	return u.executeWithPolicyTraceAndCheckAndContent(
		ctx, sourceID, idempotencyKey, candidate, domain.ContentLayers{},
		policy, trace, beforePersist,
	)
}

func (u *candidateIngest) executeWithPolicyTraceAndCheckAndContent(
	ctx context.Context,
	sourceID, idempotencyKey string,
	candidate domain.Candidate,
	content domain.ContentLayers,
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
	return u.executeCore(ctx, sourceID, idempotencyKey, candidate, content, policy, trace, beforePersist)
}

func (u *candidateIngest) executeCore(
	ctx context.Context,
	sourceID, idempotencyKey string,
	candidate domain.Candidate,
	content domain.ContentLayers,
	policy domain.SourceURLPolicy,
	trace ObservationPolicyTrace,
	beforePersist func(IdentityTransaction) error,
) (domain.Observation, error) {
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
	content = domain.NormalizeContentLayers(content)
	candidate.CanonicalURL = ""
	if candidate.RawURL != "" {
		decision := domain.EvaluateSourceURL(candidate.RawURL, policy)
		if decision.Allowed {
			candidate.CanonicalURL = decision.CanonicalURL
		}
	}
	canonicalURLHash := ""
	if candidate.CanonicalURL != "" {
		sum := sha256.Sum256([]byte(candidate.CanonicalURL))
		canonicalURLHash = hex.EncodeToString(sum[:])
	}
	fingerprint := domain.BuildVersionFingerprint(candidate, content)
	observation := domain.Observation{
		ID: strings.TrimSpace(u.newID()), IdempotencyKey: idempotencyKey,
		SourceID: sourceID, ProviderCode: candidate.ProviderCode, QueryDimension: candidate.QueryDimension,
		EndpointID: trace.EndpointID, SourcePolicyVersionID: trace.SourcePolicyVersionID,
		EndpointPolicyVersionID:       trace.EndpointPolicyVersionID,
		ProviderOutputPolicyRef:       trace.ProviderOutputPolicyRef,
		ProviderOutputPolicyVersionID: trace.ProviderOutputPolicyVersionID,
		PolicyEvaluatedAt:             trace.PolicyEvaluatedAt, Candidate: candidate, ObservedAt: observedAt,
	}
	if trace.Retention > 0 {
		observation.RetentionUntil = observedAt.Add(trace.Retention).UTC()
	}
	if observation.ID == "" {
		return domain.Observation{}, ErrInvalidRequest
	}

	credentials, err := domain.BuildIdentityCredentials(sourceID, candidate.SourceItemID)
	if err != nil {
		return domain.Observation{}, err
	}
	for i := range credentials {
		credentials[i].ObservedAt = observedAt
	}
	if candidate.CanonicalURL == "" && len(credentials) == 0 {
		if candidate.RawURL != "" {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = "news.identity.url_rejected"
		} else {
			observation.IdentityStatus = domain.ObservationUnidentified
			observation.ReasonCode = "news.identity.credential_required"
		}
	}

	var persisted domain.Observation
	err = u.repository.WithinIdentityTransaction(ctx, func(tx IdentityTransaction) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if beforePersist != nil {
			if err := beforePersist(tx); err != nil {
				return err
			}
		}
		if candidate.CanonicalURL == "" && len(credentials) == 0 {
			persisted, err = tx.AppendObservation(ctx, observation)
			return err
		}

		var urlItem domain.NewsItem
		urlMatched := false
		if candidate.CanonicalURL != "" {
			var findErr error
			urlItem, urlMatched, findErr = tx.FindNewsItemByCanonicalURL(ctx, candidate.CanonicalURL, canonicalURLHash)
			if errors.Is(findErr, ErrCanonicalURLHashCollision) {
				observation.IdentityStatus = domain.ObservationConflict
				observation.ReasonCode = "news.identity.canonical_url_hash_collision"
				observation.NewsID = ""
				persisted, findErr = tx.AppendObservation(ctx, observation)
				return findErr
			}
			if findErr != nil {
				return findErr
			}
		}

		var nativeResolution domain.IdentityResolution
		var existingCredentials []domain.IdentityCredential
		if len(credentials) > 0 {
			existingCredentials, err = tx.FindIdentityCredentials(ctx, sourceID, credentials)
			if err != nil {
				return err
			}
			nativeResolution = domain.ResolveIdentity(sourceID, credentials, existingCredentials)
		} else {
			nativeResolution = domain.IdentityResolution{Status: domain.IdentityNew}
		}

		if nativeResolution.Status == domain.IdentityConflict {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = nativeResolution.Reason
			persisted, err = tx.AppendObservation(ctx, observation)
			return err
		}
		if urlMatched && nativeResolution.Status == domain.IdentityMatched && urlItem.ID != nativeResolution.NewsID {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = "news.identity.multiple_items"
			persisted, err = tx.AppendObservation(ctx, observation)
			return err
		}

		newItem := false
		var item domain.NewsItem
		switch {
		case urlMatched:
			item = urlItem
		case nativeResolution.Status == domain.IdentityMatched:
			var urlErr error
			item, _, urlErr = tx.FindNewsItemByID(ctx, nativeResolution.NewsID)
			if urlErr != nil {
				return urlErr
			}
			if item.ID == "" {
				return errors.New("news.identity.native_item_missing")
			}
			if candidate.CanonicalURL != "" && item.CanonicalURL != "" && item.CanonicalURL != candidate.CanonicalURL {
				observation.IdentityStatus = domain.ObservationConflict
				observation.ReasonCode = "news.identity.canonical_url_mismatch"
				persisted, err = tx.AppendObservation(ctx, observation)
				return err
			}
		default:
			item = domain.NewsItem{ID: strings.TrimSpace(u.newID()) + "-news"}
			if item.ID == "-news" {
				return ErrInvalidRequest
			}
			newItem = true
		}

		if item.ID == "" {
			return errors.New("news.identity.resolved_item_id_missing")
		}
		if candidate.CanonicalURL != "" {
			if item.CanonicalURL != "" && item.CanonicalURL != candidate.CanonicalURL {
				observation.IdentityStatus = domain.ObservationConflict
				observation.ReasonCode = "news.identity.canonical_url_mismatch"
				persisted, err = tx.AppendObservation(ctx, observation)
				return err
			}
			item.CanonicalURL = candidate.CanonicalURL
			item.CanonicalURLHash = canonicalURLHash
		}

		if newItem {
			observation.IdentityStatus = domain.ObservationNew
		} else {
			observation.IdentityStatus = domain.ObservationMatched
		}
		observation.NewsID = item.ID

		// 首次主体创建必须先于 Observation，以满足 Observation.news_id 外键。
		if newItem {
			item.CreatedAt = observedAt
		}
		item.Title = candidate.Title
		item.PublisherName = candidate.PublisherName
		item.PublisherDomain = candidate.PublisherDomain
		item.Author = candidate.Author
		item.PublishedAt = candidate.PublishedAt
		item.Content = content
		item.ContentFingerprint = fingerprint
		item.UpdatedAt = observedAt
		if newItem {
			if err := tx.CreateNewsItem(ctx, item); err != nil {
				return err
			}
		}

		if len(credentials) > 0 {
			toSave := credentialsNotYetStored(credentials, existingCredentials)
			for i := range toSave {
				toSave[i].NewsID = item.ID
			}
			if len(toSave) > 0 {
				if err := tx.SaveIdentityCredentials(ctx, toSave); err != nil {
					return err
				}
			}
		}

		persisted, err = tx.AppendObservation(ctx, observation)
		if err != nil {
			return err
		}
		// 幂等重放只返回第一次提交的 Observation，不重复改写主体或追加版本。
		if persisted.ID != observation.ID {
			return nil
		}

		current, exists, err := tx.LatestNewsItemVersion(ctx, item.ID)
		if err != nil {
			return err
		}
		if !exists || current.Fingerprint != fingerprint {
			versionID := strings.TrimSpace(u.newID()) + "-version"
			if versionID == "-version" {
				return ErrInvalidRequest
			}
			versionNo := 1
			if exists {
				versionNo = current.VersionNo + 1
			}
			version := domain.NewsItemVersion{
				ID: versionID, NewsID: item.ID, VersionNo: versionNo,
				Fingerprint: fingerprint, Title: candidate.Title,
				PublisherName: candidate.PublisherName, PublisherDomain: candidate.PublisherDomain,
				Author: candidate.Author, PublishedAt: candidate.PublishedAt,
				Content: content, ObservedAt: observedAt, ObservationID: observation.ID,
			}
			if err := tx.AppendNewsItemVersion(ctx, version); err != nil {
				return err
			}
			if !newItem {
				if err := tx.UpdateNewsItem(ctx, item); err != nil {
					return err
				}
			}
		}
		return nil
	})
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
			if stored.SourceID == candidate.SourceID && stored.Hash == candidate.Hash && stored.Value == candidate.Value {
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

func effectiveObservationRetention(observation domain.Observation) time.Duration {
	if observation.RetentionUntil.IsZero() || observation.ObservedAt.IsZero() || observation.RetentionUntil.Before(observation.ObservedAt) {
		return 0
	}
	return observation.RetentionUntil.Sub(observation.ObservedAt)
}

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

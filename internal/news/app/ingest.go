package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrIdentityTransaction = errors.New("news: identity transaction failed")
	ErrObservationAppend   = errors.New("news: append observation failed")
)

// IdentityRepository 要求在单一事务中读取身份、创建主体/凭据并追加 Observation。
// 实现必须依靠事务隔离与唯一约束处理并发竞争，不允许先查后写的非原子实现。
type IdentityRepository interface {
	WithinIdentityTransaction(context.Context, func(IdentityTransaction) error) error
}

// IdentityTransaction 是一次身份处理事务中的最小持久化端口。
type IdentityTransaction interface {
	FindIdentityCredentials(context.Context, string, []domain.IdentityCredential) ([]domain.IdentityCredential, error)
	CreateNewsItem(context.Context, string) (string, error)
	SaveIdentityCredentials(context.Context, []domain.IdentityCredential) error
	AppendObservation(context.Context, domain.Observation) error
}

// IngestCandidate 负责将一个已发现候选写入身份处理流程；不负责真实抓取或跨来源去重。
type IngestCandidate struct {
	repository IdentityRepository
	now        func() time.Time
	newID      func() string
}

// NewIngestCandidate 创建候选身份处理用例。
func NewIngestCandidate(repository IdentityRepository, newID func() string) *IngestCandidate {
	return &IngestCandidate{repository: repository, now: time.Now, newID: newID}
}

// Execute 在同一事务内解析身份、必要时创建主体、保存身份凭据并追加 Observation。
// 无身份凭据的候选只形成 Observation；冲突候选只追加冲突 Observation，绝不创建主体。
func (u *IngestCandidate) Execute(ctx context.Context, sourceID string, candidate domain.Candidate, policy domain.SourceURLPolicy) (domain.Observation, error) {
	if u == nil || u.repository == nil || u.newID == nil || ctx == nil {
		return domain.Observation{}, ErrInvalidRequest
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return domain.Observation{}, ErrInvalidRequest
	}

	observedAt := u.now().UTC()
	candidate = domain.NormalizeCandidate(candidate, observedAt, candidate.QueryDimension)
	observation := domain.Observation{
		ID:             u.newID(),
		SourceID:       sourceID,
		ProviderCode:   candidate.ProviderCode,
		QueryDimension: candidate.QueryDimension,
		Candidate:      candidate,
		ObservedAt:     observedAt,
	}

	credentials := []domain.IdentityCredential(nil)
	if strings.TrimSpace(candidate.ProviderItemID) == "" && strings.TrimSpace(candidate.RawURL) == "" {
		observation.IdentityStatus = domain.ObservationUnidentified
		observation.ReasonCode = "news.identity.credential_required"
	} else {
		built, err := domain.BuildIdentityCredentials(sourceID, candidate.ProviderItemID, candidate.RawURL, policy)
		if err != nil {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = "news.identity.credential_rejected"
		} else {
			credentials = built
		}
	}

	err := u.repository.WithinIdentityTransaction(ctx, func(tx IdentityTransaction) error {
		if len(credentials) == 0 {
			return tx.AppendObservation(ctx, observation)
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
			for i := range credentials {
				credentials[i].NewsID = observation.NewsID
			}
			if err := tx.SaveIdentityCredentials(ctx, credentials); err != nil {
				return err
			}
		}
		return tx.AppendObservation(ctx, observation)
	})
	if err != nil {
		return domain.Observation{}, errors.Join(ErrIdentityTransaction, err)
	}
	return observation, nil
}

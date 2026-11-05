package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrResearchUpdateTransaction = errors.New("news: research update transaction failed")
	ErrVersionHistoryInvalid     = errors.New("news: version history invalid")
)

// ResearchUpdateRepository 将版本追加、关系读取和关系证据追加放在同一事务内。
type ResearchUpdateRepository interface {
	WithinResearchUpdateTransaction(context.Context, func(ResearchUpdateTransaction) error) error
}

// ResearchUpdateTransaction 只暴露新闻版本和关联断言所需的最小存储能力。
type ResearchUpdateTransaction interface {
	LatestNewsItemVersion(context.Context, string) (domain.NewsItemVersion, bool, error)
	AppendNewsItemVersion(context.Context, domain.NewsItemVersion) error
	ListRelationAssertions(context.Context, string) ([]domain.RelationAssertion, error)
	AppendRelationAssertion(context.Context, domain.RelationAssertion, []domain.RelationEvidence) error
}

// ResearchUpdateRequest 描述一次对既有新闻主体的内容修订和关系断言更新。
type ResearchUpdateRequest struct {
	NewsID        string
	Candidate     domain.Candidate
	Content       domain.ContentLayers
	ObservationID string
	Relations     []domain.RelationAssertion
	Evidence      map[string][]domain.RelationEvidence
}

// ResearchUpdateResult 返回版本是否追加及当前有效关系。
type ResearchUpdateResult struct {
	Version        domain.NewsItemVersion
	VersionCreated bool
	CurrentRelations []domain.RelationAssertion
}

// ApplyResearchUpdate 将内容版本和关系断言作为一个原子业务更新执行。
// 内容指纹未变化时复用当前版本；关系修正/撤销必须通过新断言 supersede 历史断言。
type ApplyResearchUpdate struct {
	repository ResearchUpdateRepository
	now        func() time.Time
	newID      func() string
}

func NewApplyResearchUpdate(repository ResearchUpdateRepository, newID func() string) *ApplyResearchUpdate {
	return &ApplyResearchUpdate{repository: repository, now: time.Now, newID: newID}
}

func (u *ApplyResearchUpdate) Execute(ctx context.Context, request ResearchUpdateRequest) (ResearchUpdateResult, error) {
	if u == nil || u.repository == nil || u.newID == nil || ctx == nil ||
		strings.TrimSpace(request.NewsID) == "" || strings.TrimSpace(request.ObservationID) == "" {
		return ResearchUpdateResult{}, ErrInvalidRequest
	}
	request.NewsID = strings.TrimSpace(request.NewsID)
	request.ObservationID = strings.TrimSpace(request.ObservationID)
	observedAt := u.now().UTC()
	candidate := domain.NormalizeCandidate(request.Candidate, observedAt, request.Candidate.QueryDimension)
	content := domain.NormalizeContentLayers(request.Content)
	fingerprint := domain.BuildVersionFingerprint(candidate, content)

	var result ResearchUpdateResult
	err := u.repository.WithinResearchUpdateTransaction(ctx, func(tx ResearchUpdateTransaction) error {
		current, exists, err := tx.LatestNewsItemVersion(ctx, request.NewsID)
		if err != nil {
			return err
		}
		if exists && (current.NewsID != request.NewsID || current.VersionNo < 1 || strings.TrimSpace(current.Fingerprint) == "") {
			return ErrVersionHistoryInvalid
		}

		if exists && current.Fingerprint == fingerprint {
			result.Version = current
		} else {
			versionNo := 1
			if exists {
				versionNo = current.VersionNo + 1
			}
			versionID := strings.TrimSpace(u.newID())
			if versionID == "" {
				return ErrInvalidRequest
			}
			version := domain.NewsItemVersion{
				ID: versionID, NewsID: request.NewsID, VersionNo: versionNo,
				Fingerprint: fingerprint, Title: candidate.Title,
				PublisherName: candidate.PublisherName, PublisherDomain: candidate.PublisherDomain,
				PublishedAt: candidate.PublishedAt, Content: content,
				ObservedAt: observedAt, ObservationID: request.ObservationID,
			}
			if err := tx.AppendNewsItemVersion(ctx, version); err != nil {
				return err
			}
			result.Version = version
			result.VersionCreated = true
		}

		existingRelations, err := tx.ListRelationAssertions(ctx, request.NewsID)
		if err != nil {
			return err
		}
		if len(request.Relations) > 0 && !result.VersionCreated && !exists {
			return ErrVersionHistoryInvalid
		}
		allRelations := append([]domain.RelationAssertion(nil), existingRelations...)
		for _, assertion := range request.Relations {
			assertion.ID = strings.TrimSpace(assertion.ID)
			assertion.NewsID = request.NewsID
			if assertion.ObservedAt.IsZero() {
				assertion.ObservedAt = observedAt
			}
			if err := domain.ValidateRelationAssertion(assertion); err != nil {
				return err
			}
			evidence := append([]domain.RelationEvidence(nil), request.Evidence[assertion.ID]...)
			if len(evidence) == 0 {
				return domain.ErrRelationEvidenceInvalid
			}
			for i := range evidence {
				if evidence[i].RelationID == "" {
					evidence[i].RelationID = assertion.ID
				}
				if evidence[i].Type == domain.RelationEvidenceContentSpan && evidence[i].SourceVersionID == "" {
					evidence[i].SourceVersionID = result.Version.ID
				}
				if evidence[i].ObservationID == "" && evidence[i].SourceVersionID == "" {
					evidence[i].ObservationID = request.ObservationID
				}
				if err := domain.ValidateRelationEvidence(assertion, evidence[i]); err != nil {
					return err
				}
			}
			allRelations = append(allRelations, assertion)
			if _, err := domain.ResolveCurrentRelations(allRelations); err != nil {
				return err
			}
			if err := tx.AppendRelationAssertion(ctx, assertion, evidence); err != nil {
				return err
			}
		}
		currentRelations, err := domain.ResolveCurrentRelations(allRelations)
		if err != nil {
			return err
		}
		result.CurrentRelations = currentRelations
		return nil
	})
	if err != nil {
		return ResearchUpdateResult{}, errors.Join(ErrResearchUpdateTransaction, err)
	}
	return result, nil
}

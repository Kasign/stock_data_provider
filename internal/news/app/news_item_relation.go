package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrNewsItemRelationTransaction = errors.New("news: item relation transaction failed")
	ErrNewsItemRelationEvidence    = errors.New("news: item relation evidence ownership invalid")
	ErrNewsItemRelationHistory     = errors.New("news: item relation history conflict")
)

// NewsItemRelationRepository 必须在单一事务内读取双方证据、完整历史并追加断言。
type NewsItemRelationRepository interface {
	WithinNewsItemRelationTransaction(context.Context, func(NewsItemRelationTransaction) error) error
}

// NewsItemRelationTransaction 是转载断言 Application 所需的最小事务端口。
type NewsItemRelationTransaction interface {
	ObservationByID(context.Context, string) (domain.Observation, bool, error)
	NewsItemVersionByID(context.Context, string) (domain.NewsItemVersion, bool, error)
	LatestNewsItemVersion(context.Context, string) (domain.NewsItemVersion, bool, error)
	ListAllNewsItemRelations(context.Context) ([]domain.NewsItemRelation, error)
	AppendNewsItemRelation(context.Context, domain.NewsItemRelation) error
}

// ApplyNewsItemRelation 只追加关系断言，不覆盖既有历史。
type ApplyNewsItemRelation struct {
	repository NewsItemRelationRepository
	now        func() time.Time
	newID      func() string
}

func NewApplyNewsItemRelation(repository NewsItemRelationRepository, newID func() string) *ApplyNewsItemRelation {
	return &ApplyNewsItemRelation{repository: repository, now: time.Now, newID: newID}
}

// NewsItemRelationRequest 的 Observation 必须属于转载方；版本 ID 必须同时提供或同时留空。
type NewsItemRelationRequest struct {
	Relation domain.NewsItemRelation
}

// Apply 在同一事务内验证证据归属、双方版本和完整关系图，再追加断言。
func (u *ApplyNewsItemRelation) Execute(ctx context.Context, request NewsItemRelationRequest) (domain.NewsItemRelation, error) {
	if u == nil || u.repository == nil || u.newID == nil || ctx == nil {
		return domain.NewsItemRelation{}, ErrInvalidRequest
	}
	relation := request.Relation
	relation.ID = strings.TrimSpace(relation.ID)
	if relation.ID == "" {
		relation.ID = strings.TrimSpace(u.newID())
	}
	relation.FromNewsID = strings.TrimSpace(relation.FromNewsID)
	relation.ToNewsID = strings.TrimSpace(relation.ToNewsID)
	relation.FromSourceID = strings.TrimSpace(relation.FromSourceID)
	relation.ToSourceID = strings.TrimSpace(relation.ToSourceID)
	relation.EvidenceObservation = strings.TrimSpace(relation.EvidenceObservation)
	relation.FromVersionID = strings.TrimSpace(relation.FromVersionID)
	relation.ToVersionID = strings.TrimSpace(relation.ToVersionID)
	relation.SupersedesRelationID = strings.TrimSpace(relation.SupersedesRelationID)
	if relation.AssertedAt.IsZero() {
		relation.AssertedAt = u.now().UTC()
	}
	if relation.ObservedAt.IsZero() {
		relation.ObservedAt = u.now().UTC()
	}
	if err := domain.ValidateNewsItemRelation(relation); err != nil {
		return domain.NewsItemRelation{}, err
	}

	err := u.repository.WithinNewsItemRelationTransaction(ctx, func(tx NewsItemRelationTransaction) error {
		observation, exists, err := tx.ObservationByID(ctx, relation.EvidenceObservation)
		if err != nil {
			return err
		}
		if !exists || observation.ID != relation.EvidenceObservation ||
			observation.NewsID != relation.FromNewsID ||
			observation.SourceID != relation.FromSourceID {
			return ErrNewsItemRelationEvidence
		}
		if err := validateRelationVersionOwnership(ctx, tx, relation.FromVersionID, relation.FromNewsID, relation.FromSourceID); err != nil {
			return err
		}
		if err := validateRelationVersionOwnership(ctx, tx, relation.ToVersionID, relation.ToNewsID, relation.ToSourceID); err != nil {
			return err
		}
		if relation.FromVersionID != "" {
			latest, ok, err := tx.LatestNewsItemVersion(ctx, relation.FromNewsID)
			if err != nil {
				return err
			}
			if !ok || latest.ID != relation.FromVersionID {
				return ErrNewsItemRelationEvidence
			}
		}
		if relation.ToVersionID != "" {
			latest, ok, err := tx.LatestNewsItemVersion(ctx, relation.ToNewsID)
			if err != nil {
				return err
			}
			if !ok || latest.ID != relation.ToVersionID {
				return ErrNewsItemRelationEvidence
			}
		}

		history, err := tx.ListAllNewsItemRelations(ctx)
		if err != nil {
			return err
		}
		for _, existing := range history {
			if existing.ID == relation.ID {
				return ErrNewsItemRelationHistory
			}
		}
		all := append(append([]domain.NewsItemRelation(nil), history...), relation)
		if err := domain.ValidateNewsItemRelationHistory(all); err != nil {
			return errors.Join(ErrNewsItemRelationHistory, err)
		}
		return tx.AppendNewsItemRelation(ctx, relation)
	})
	if err != nil {
		return domain.NewsItemRelation{}, errors.Join(ErrNewsItemRelationTransaction, err)
	}
	return relation, nil
}

func validateRelationVersionOwnership(
	ctx context.Context,
	tx NewsItemRelationTransaction,
	versionID, newsID, sourceID string,
) error {
	if versionID == "" {
		return nil
	}
	version, exists, err := tx.NewsItemVersionByID(ctx, versionID)
	if err != nil {
		return err
	}
	if !exists || version.ID != versionID || version.NewsID != newsID || version.ObservationID == "" {
		return ErrNewsItemRelationEvidence
	}
	observation, exists, err := tx.ObservationByID(ctx, version.ObservationID)
	if err != nil {
		return err
	}
	if !exists || observation.NewsID != newsID || observation.SourceID != sourceID {
		return ErrNewsItemRelationEvidence
	}
	return nil
}

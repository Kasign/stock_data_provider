package domain

import (
	"errors"
	"strings"
)

type NewsItemRelationKind string

const (
	NewsItemRelationRepostOf NewsItemRelationKind = "repost_of"
)

// NewsItemRelation 表示不同发布来源之间的转载/再发布关系，不合并两条新闻主体。
// FromVersionID/ToVersionID 固定关系证据所依据的双方内容版本，不随最新版本自动漂移。
type NewsItemRelation struct {
	ID                  string
	FromNewsID          string
	FromSourceID        string
	ToNewsID            string
	ToSourceID          string
	Kind                NewsItemRelationKind
	EvidenceObservation string
	FromVersionID       string
	ToVersionID         string
}

type NewsItemRelationEvidenceStatus string

const (
	NewsItemRelationEvidenceUnversioned NewsItemRelationEvidenceStatus = "unversioned"
	NewsItemRelationEvidenceCurrent     NewsItemRelationEvidenceStatus = "current"
	NewsItemRelationEvidenceStale       NewsItemRelationEvidenceStatus = "stale"
)

var ErrNewsItemRelationInvalid = errors.New("news.item_relation.invalid")

func ValidateNewsItemRelation(relation NewsItemRelation) error {
	fromVersionID := strings.TrimSpace(relation.FromVersionID)
	toVersionID := strings.TrimSpace(relation.ToVersionID)
	if strings.TrimSpace(relation.ID) == "" ||
		strings.TrimSpace(relation.FromNewsID) == "" ||
		strings.TrimSpace(relation.ToNewsID) == "" ||
		relation.FromNewsID == relation.ToNewsID ||
		strings.TrimSpace(relation.FromSourceID) == "" ||
		strings.TrimSpace(relation.ToSourceID) == "" ||
		relation.FromSourceID == relation.ToSourceID ||
		relation.Kind != NewsItemRelationRepostOf ||
		(strings.TrimSpace(relation.EvidenceObservation) == "" && (fromVersionID == "" || toVersionID == "")) ||
		((fromVersionID == "") != (toVersionID == "")) {
		return ErrNewsItemRelationInvalid
	}
	return nil
}

// AssessNewsItemRelationEvidence 比较关系证据绑定的双方版本与各自最新版本。
// 内容任一侧发生修订即返回 stale，要求重新评估；不会自动把历史证据改绑到新版本。
func AssessNewsItemRelationEvidence(
	relation NewsItemRelation,
	fromCurrentVersionID, toCurrentVersionID string,
) (NewsItemRelationEvidenceStatus, error) {
	if err := ValidateNewsItemRelation(relation); err != nil {
		return "", err
	}
	if strings.TrimSpace(relation.FromVersionID) == "" || strings.TrimSpace(relation.ToVersionID) == "" {
		return NewsItemRelationEvidenceUnversioned, nil
	}
	fromCurrentVersionID = strings.TrimSpace(fromCurrentVersionID)
	toCurrentVersionID = strings.TrimSpace(toCurrentVersionID)
	if fromCurrentVersionID == "" || toCurrentVersionID == "" {
		return "", ErrNewsItemRelationInvalid
	}
	if relation.FromVersionID != fromCurrentVersionID || relation.ToVersionID != toCurrentVersionID {
		return NewsItemRelationEvidenceStale, nil
	}
	return NewsItemRelationEvidenceCurrent, nil
}

// ValidateNewsItemRelationGraph 拒绝重复边和循环转载链；FromNewsID 指转载方，ToNewsID 指原始来源项。
func ValidateNewsItemRelationGraph(relations []NewsItemRelation) error {
	byID := make(map[string]NewsItemRelation, len(relations))
	edges := make(map[string]string, len(relations))
	for _, relation := range relations {
		if err := ValidateNewsItemRelation(relation); err != nil {
			return err
		}
		if _, exists := byID[relation.ID]; exists {
			return ErrNewsItemRelationInvalid
		}
		byID[relation.ID] = relation
		edgeKey := relation.FromNewsID + "\x00" + relation.ToNewsID
		if _, exists := edges[edgeKey]; exists {
			return ErrNewsItemRelationInvalid
		}
		edges[edgeKey] = relation.ID
	}
	parents := make(map[string]string, len(relations))
	for _, relation := range relations {
		if existing, exists := parents[relation.FromNewsID]; exists && existing != relation.ToNewsID {
			return ErrNewsItemRelationInvalid
		}
		parents[relation.FromNewsID] = relation.ToNewsID
	}
	for itemID := range parents {
		seen := make(map[string]struct{})
		current := itemID
		for current != "" {
			if _, exists := seen[current]; exists {
				return ErrNewsItemRelationInvalid
			}
			seen[current] = struct{}{}
			current = parents[current]
		}
	}
	return nil
}

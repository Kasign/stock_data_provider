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
type NewsItemRelation struct {
	ID                  string
	FromNewsID          string
	FromSourceID        string
	ToNewsID            string
	ToSourceID          string
	Kind                NewsItemRelationKind
	EvidenceObservation string
	EvidenceVersion     string
}

var ErrNewsItemRelationInvalid = errors.New("news.item_relation.invalid")

func ValidateNewsItemRelation(relation NewsItemRelation) error {
	if strings.TrimSpace(relation.ID) == "" ||
		strings.TrimSpace(relation.FromNewsID) == "" ||
		strings.TrimSpace(relation.ToNewsID) == "" ||
		relation.FromNewsID == relation.ToNewsID ||
		strings.TrimSpace(relation.FromSourceID) == "" ||
		strings.TrimSpace(relation.ToSourceID) == "" ||
		relation.FromSourceID == relation.ToSourceID ||
		relation.Kind != NewsItemRelationRepostOf ||
		(relation.EvidenceObservation == "" && relation.EvidenceVersion == "") {
		return ErrNewsItemRelationInvalid
	}
	return nil
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

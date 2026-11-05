package domain

import (
	"errors"
	"strings"
	"time"
)

type NewsItemRelationKind string

const (
	NewsItemRelationRepostOf NewsItemRelationKind = "repost_of"
)

type NewsItemRelationStatus string

const (
	NewsItemRelationProposed  NewsItemRelationStatus = "proposed"
	NewsItemRelationConfirmed NewsItemRelationStatus = "confirmed"
	NewsItemRelationRetracted NewsItemRelationStatus = "retracted"
)

// NewsItemRelation 是不可变的转载关系断言。修订或撤销必须追加新断言并指向被替代断言。
type NewsItemRelation struct {
	ID                   string
	FromNewsID           string
	FromSourceID         string
	ToNewsID             string
	ToSourceID           string
	Kind                 NewsItemRelationKind
	Status               NewsItemRelationStatus
	SupersedesRelationID string
	EvidenceObservation  string
	FromVersionID        string
	ToVersionID          string
	AssertedAt           time.Time
	ObservedAt           time.Time
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
		(relation.Status != NewsItemRelationProposed && relation.Status != NewsItemRelationConfirmed && relation.Status != NewsItemRelationRetracted) ||
		relation.AssertedAt.IsZero() || relation.ObservedAt.IsZero() ||
		(relation.Status == NewsItemRelationRetracted && strings.TrimSpace(relation.SupersedesRelationID) == "") ||
		(strings.TrimSpace(relation.EvidenceObservation) == "" && (fromVersionID == "" || toVersionID == "")) ||
		((fromVersionID == "") != (toVersionID == "")) {
		return ErrNewsItemRelationInvalid
	}
	if strings.TrimSpace(relation.SupersedesRelationID) == relation.ID {
		return ErrNewsItemRelationInvalid
	}
	return nil
}

// ValidateNewsItemRelationHistory 校验追加式断言链和当前确认关系构成的 DAG。
// proposed 替代断言尚未生效；只有 confirmed/retracted 后继才改变当前关系图。
func ValidateNewsItemRelationHistory(relations []NewsItemRelation) error {
	byID := make(map[string]NewsItemRelation, len(relations))
	successor := make(map[string]string, len(relations))
	for _, relation := range relations {
		if err := ValidateNewsItemRelation(relation); err != nil {
			return err
		}
		if _, exists := byID[relation.ID]; exists {
			return ErrNewsItemRelationInvalid
		}
		byID[relation.ID] = relation
	}
	for _, relation := range relations {
		parentID := strings.TrimSpace(relation.SupersedesRelationID)
		if parentID == "" {
			continue
		}
		parent, exists := byID[parentID]
		if !exists || parent.ID == relation.ID ||
			parent.FromNewsID != relation.FromNewsID ||
			parent.FromSourceID != relation.FromSourceID ||
			parent.Kind != relation.Kind ||
			parent.Status == NewsItemRelationRetracted ||
			(relation.Status == NewsItemRelationRetracted &&
				(parent.ToNewsID != relation.ToNewsID || parent.ToSourceID != relation.ToSourceID)) {
			return ErrNewsItemRelationInvalid
		}
		if _, exists := successor[parentID]; exists {
			return ErrNewsItemRelationInvalid
		}
		successor[parentID] = relation.ID
	}
	for id := range byID {
		seen := make(map[string]struct{})
		current := id
		for current != "" {
			if _, exists := seen[current]; exists {
				return ErrNewsItemRelationInvalid
			}
			seen[current] = struct{}{}
			current = successor[current]
		}
	}

	// 沿替代链跳过 proposed 中间态：只要后续链上出现 confirmed/retracted，
	// 所有祖先断言都应退出当前图，不能因直接后继仍是 proposed 而复活旧边。
	effectivelyReplaced := func(id string) bool {
		seen := make(map[string]struct{})
		for nextID := successor[id]; nextID != ""; nextID = successor[nextID] {
			if _, exists := seen[nextID]; exists {
				return true
			}
			seen[nextID] = struct{}{}
			next := byID[nextID]
			if next.Status == NewsItemRelationConfirmed || next.Status == NewsItemRelationRetracted {
				return true
			}
		}
		return false
	}
	edges := make(map[string]struct{})
	adjacency := make(map[string][]string)
	for id, relation := range byID {
		if effectivelyReplaced(id) || relation.Status != NewsItemRelationConfirmed {
			continue
		}
		key := relation.FromNewsID + "\x00" + relation.ToNewsID
		if _, exists := edges[key]; exists {
			return ErrNewsItemRelationInvalid
		}
		edges[key] = struct{}{}
		adjacency[relation.FromNewsID] = append(adjacency[relation.FromNewsID], relation.ToNewsID)
	}
	state := make(map[string]uint8)
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return false
		}
		if state[node] == 2 {
			return true
		}
		state[node] = 1
		for _, next := range adjacency[node] {
			if !visit(next) {
				return false
			}
		}
		state[node] = 2
		return true
	}
	for node := range adjacency {
		if !visit(node) {
			return ErrNewsItemRelationInvalid
		}
	}
	return nil
}

// ValidateNewsItemRelationGraph 保留原有调用名，按完整断言历史规则校验。
func ValidateNewsItemRelationGraph(relations []NewsItemRelation) error {
	return ValidateNewsItemRelationHistory(relations)
}

// AssessNewsItemRelationEvidence 比较关系证据绑定的双方版本与各自最新版本。
// 内容任一侧发生修订即返回 stale；不会自动改写历史证据引用。
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

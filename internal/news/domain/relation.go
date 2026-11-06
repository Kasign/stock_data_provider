package domain

import (
	"errors"
	"strings"
	"time"
)

type RelationSubjectType string

const (
	RelationSubjectSecurity RelationSubjectType = "security"
	RelationSubjectIndustry RelationSubjectType = "industry"
)

type RelationKind string

const (
	RelationMentions       RelationKind = "mentions"
	RelationPrimarySubject RelationKind = "primary_subject"
)

type RelationMethod string

const (
	RelationSourceExplicit RelationMethod = "source_explicit"
	RelationTitleMatch     RelationMethod = "title_match"
	RelationContentMatch   RelationMethod = "content_match"
	RelationRule           RelationMethod = "rule"
	RelationManual         RelationMethod = "manual"
	RelationModel          RelationMethod = "model"
)

type RelationStatus string

const (
	RelationProposed  RelationStatus = "proposed"
	RelationConfirmed RelationStatus = "confirmed"
	RelationRejected  RelationStatus = "rejected"
	RelationRetracted RelationStatus = "retracted"
)

type RelationEvidenceType string

const (
	RelationEvidenceSourceField RelationEvidenceType = "source_field"
	RelationEvidenceTitleSpan   RelationEvidenceType = "title_span"
	RelationEvidenceContentSpan RelationEvidenceType = "content_span"
	RelationEvidenceRuleMatch   RelationEvidenceType = "rule_match"
	RelationEvidenceManual      RelationEvidenceType = "manual_review"
	RelationEvidenceModelOutput RelationEvidenceType = "model_output"
)

// SecurityRelationSubject 使用市场/交易所与证券代码组成身份，不允许仅凭代码跨市场匹配。
type SecurityRelationSubject struct {
	Market string
	Symbol string
	Name   string
}

// IndustryRelationSubject 保留分类体系和行业代码/名称快照，不依赖尚未统一的 industry_id。
type IndustryRelationSubject struct {
	ClassificationSystem  string
	ClassificationVersion string
	IndustryCode          string
	IndustryName          string
}

// RelationAssertion 是追加式关系断言；修正/撤销通过新断言 supersedes 旧断言。
type RelationAssertion struct {
	ID                   string
	NewsID               string
	SubjectType          RelationSubjectType
	Security             *SecurityRelationSubject
	Industry             *IndustryRelationSubject
	Kind                 RelationKind
	Method               RelationMethod
	Status               RelationStatus
	SupersedesRelationID string
	ValidFrom            *time.Time
	ValidTo              *time.Time
	ObservedAt           time.Time
}

// RelationEvidence 只保存可定位证据引用和摘要指纹，不复制正文或大段 snippet。
type RelationEvidence struct {
	RelationID      string
	Type            RelationEvidenceType
	SourceVersionID string
	ObservationID   string
	RuleVersion     string
	ModelVersion    string
	FieldPath       string
	EvidenceHash    string
}

var (
	ErrRelationInvalid         = errors.New("news.relation.invalid")
	ErrRelationEvidenceInvalid = errors.New("news.relation.evidence_invalid")
	ErrRelationHistoryConflict = errors.New("news.relation.history_conflict")
)

func ValidateRelationAssertion(assertion RelationAssertion) error {
	if assertion.ID == "" || assertion.ID != strings.TrimSpace(assertion.ID) ||
		assertion.NewsID == "" || assertion.NewsID != strings.TrimSpace(assertion.NewsID) ||
		assertion.Kind != RelationMentions && assertion.Kind != RelationPrimarySubject ||
		assertion.ObservedAt.IsZero() {
		return ErrRelationInvalid
	}
	if assertion.ValidFrom != nil && assertion.ValidTo != nil && !assertion.ValidTo.After(*assertion.ValidFrom) {
		return ErrRelationInvalid
	}

	switch assertion.SubjectType {
	case RelationSubjectSecurity:
		if assertion.Security == nil || assertion.Industry != nil ||
			assertion.Security.Market == "" || assertion.Security.Market != strings.TrimSpace(assertion.Security.Market) ||
			assertion.Security.Symbol == "" || assertion.Security.Symbol != strings.TrimSpace(assertion.Security.Symbol) {
			return ErrRelationInvalid
		}
	case RelationSubjectIndustry:
		if assertion.Industry == nil || assertion.Security != nil ||
			assertion.Industry.ClassificationSystem == "" ||
			assertion.Industry.ClassificationSystem != strings.TrimSpace(assertion.Industry.ClassificationSystem) ||
			assertion.Industry.ClassificationVersion == "" ||
			assertion.Industry.ClassificationVersion != strings.TrimSpace(assertion.Industry.ClassificationVersion) ||
			(assertion.Industry.IndustryCode == "" && assertion.Industry.IndustryName == "") ||
			assertion.Industry.IndustryCode != strings.TrimSpace(assertion.Industry.IndustryCode) ||
			assertion.Industry.IndustryName != strings.TrimSpace(assertion.Industry.IndustryName) {
			return ErrRelationInvalid
		}
	default:
		return ErrRelationInvalid
	}

	switch assertion.Method {
	case RelationSourceExplicit, RelationTitleMatch, RelationContentMatch, RelationRule, RelationManual, RelationModel:
	default:
		return ErrRelationInvalid
	}
	switch assertion.Status {
	case RelationProposed, RelationConfirmed, RelationRejected, RelationRetracted:
	default:
		return ErrRelationInvalid
	}
	if assertion.Method == RelationModel && assertion.Status != RelationProposed {
		return ErrRelationInvalid
	}
	if assertion.SupersedesRelationID != "" && assertion.SupersedesRelationID != strings.TrimSpace(assertion.SupersedesRelationID) {
		return ErrRelationInvalid
	}
	if assertion.Status == RelationProposed && assertion.SupersedesRelationID != "" {
		return ErrRelationInvalid
	}
	if assertion.SupersedesRelationID == assertion.ID && assertion.SupersedesRelationID != "" {
		return ErrRelationInvalid
	}
	return nil
}

func ValidateRelationEvidence(assertion RelationAssertion, evidence RelationEvidence) error {
	if ValidateRelationAssertion(assertion) != nil ||
		evidence.RelationID == "" || evidence.RelationID != assertion.ID ||
		evidence.SourceVersionID != strings.TrimSpace(evidence.SourceVersionID) ||
		evidence.ObservationID != strings.TrimSpace(evidence.ObservationID) ||
		evidence.RuleVersion != strings.TrimSpace(evidence.RuleVersion) ||
		evidence.ModelVersion != strings.TrimSpace(evidence.ModelVersion) ||
		evidence.FieldPath != strings.TrimSpace(evidence.FieldPath) {
		return ErrRelationEvidenceInvalid
	}
	if evidence.SourceVersionID == "" && evidence.ObservationID == "" {
		return ErrRelationEvidenceInvalid
	}
	switch assertion.Method {
	case RelationSourceExplicit:
		if evidence.Type != RelationEvidenceSourceField || strings.TrimSpace(evidence.FieldPath) == "" {
			return ErrRelationEvidenceInvalid
		}
	case RelationTitleMatch:
		if evidence.Type != RelationEvidenceTitleSpan || strings.TrimSpace(evidence.FieldPath) != "title" {
			return ErrRelationEvidenceInvalid
		}
	case RelationContentMatch:
		if evidence.Type != RelationEvidenceContentSpan || evidence.SourceVersionID == "" {
			return ErrRelationEvidenceInvalid
		}
	case RelationRule:
		if evidence.Type != RelationEvidenceRuleMatch || strings.TrimSpace(evidence.RuleVersion) == "" {
			return ErrRelationEvidenceInvalid
		}
	case RelationManual:
		if evidence.Type != RelationEvidenceManual {
			return ErrRelationEvidenceInvalid
		}
	case RelationModel:
		if evidence.Type != RelationEvidenceModelOutput || strings.TrimSpace(evidence.ModelVersion) == "" {
			return ErrRelationEvidenceInvalid
		}
	}
	return nil
}

// ResolveCurrentRelations 沿 supersedes 链计算当前已确认关系；终态 rejected/retracted 不作为当前关系。
func ResolveCurrentRelations(assertions []RelationAssertion) ([]RelationAssertion, error) {
	byID := make(map[string]RelationAssertion, len(assertions))
	superseded := make(map[string]string, len(assertions))
	for _, assertion := range assertions {
		if err := ValidateRelationAssertion(assertion); err != nil {
			return nil, err
		}
		if _, exists := byID[assertion.ID]; exists {
			return nil, ErrRelationHistoryConflict
		}
		byID[assertion.ID] = assertion
	}
	for _, assertion := range assertions {
		parentID := strings.TrimSpace(assertion.SupersedesRelationID)
		if parentID == "" {
			continue
		}
		parent, exists := byID[parentID]
		if !exists || parent.NewsID != assertion.NewsID || parent.Kind != assertion.Kind || parent.Status == RelationRetracted {
			return nil, ErrRelationHistoryConflict
		}
		if _, exists := superseded[parentID]; exists {
			return nil, ErrRelationHistoryConflict
		}
		superseded[parentID] = assertion.ID
	}

	for _, assertion := range assertions {
		seen := make(map[string]struct{})
		current := assertion
		for current.SupersedesRelationID != "" {
			if _, exists := seen[current.ID]; exists {
				return nil, ErrRelationHistoryConflict
			}
			seen[current.ID] = struct{}{}
			parent, exists := byID[current.SupersedesRelationID]
			if !exists {
				return nil, ErrRelationHistoryConflict
			}
			current = parent
		}
	}

	result := make([]RelationAssertion, 0, len(assertions))
	primarySubjects := make(map[string]string)
	for _, assertion := range assertions {
		if _, hasSuccessor := superseded[assertion.ID]; hasSuccessor {
			continue
		}
		if assertion.Status != RelationConfirmed {
			continue
		}
		if assertion.Kind == RelationPrimarySubject {
			if _, exists := primarySubjects[assertion.NewsID]; exists {
				return nil, ErrRelationHistoryConflict
			}
			primarySubjects[assertion.NewsID] = assertion.ID
		}
		result = append(result, assertion)
	}
	return result, nil
}

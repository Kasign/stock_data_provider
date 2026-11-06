package domain

import (
	"errors"
	"strings"
	"time"
)

// IndustryClassificationStatus 表示行业分类代码在该分类版本中的状态。
type IndustryClassificationStatus string

const (
	IndustryClassificationActive       IndustryClassificationStatus = "active"
	IndustryClassificationDiscontinued IndustryClassificationStatus = "discontinued"
)

// IndustryClassificationRecord 是带业务有效时间和来源证据的行业分类快照。
// 分类体系与版本共同定义代码空间；不同版本的相同代码不代表同一行业。
type IndustryClassificationRecord struct {
	ClassificationSystem  string
	ClassificationVersion string
	IndustryCode          string
	IndustryName          string
	ParentCode            string
	ValidFrom             time.Time
	ValidTo               *time.Time
	ObservedAt            time.Time
	Source                string
	EvidenceRef           string
	Status                IndustryClassificationStatus
}

type IndustryClassificationResolutionStatus string

const (
	IndustryClassificationResolved  IndustryClassificationResolutionStatus = "resolved"
	IndustryClassificationNotFound  IndustryClassificationResolutionStatus = "not_found"
	IndustryClassificationAmbiguous IndustryClassificationResolutionStatus = "ambiguous"
)

type IndustryClassificationResolution struct {
	Status IndustryClassificationResolutionStatus
	Record IndustryClassificationRecord
}

var ErrIndustryClassificationInvalid = errors.New("news.industry_classification.invalid")

// ValidateIndustryClassificationRecord 校验分类快照的身份、有效期和来源证据。
func ValidateIndustryClassificationRecord(record IndustryClassificationRecord) error {
	if strings.TrimSpace(record.ClassificationSystem) == "" ||
		strings.TrimSpace(record.ClassificationVersion) == "" ||
		strings.TrimSpace(record.IndustryCode) == "" ||
		strings.TrimSpace(record.IndustryName) == "" ||
		record.ValidFrom.IsZero() ||
		record.ObservedAt.IsZero() ||
		strings.TrimSpace(record.Source) == "" ||
		strings.TrimSpace(record.EvidenceRef) == "" {
		return ErrIndustryClassificationInvalid
	}
	if record.ValidTo != nil && !record.ValidTo.After(record.ValidFrom) {
		return ErrIndustryClassificationInvalid
	}
	if record.Status != IndustryClassificationActive && record.Status != IndustryClassificationDiscontinued {
		return ErrIndustryClassificationInvalid
	}
	if strings.TrimSpace(record.ParentCode) == record.IndustryCode {
		return ErrIndustryClassificationInvalid
	}
	return nil
}

// ResolveIndustryClassification 严格按分类体系、版本、代码和业务有效时间解析。
// 不跨版本推断映射；同一时点多条记录命中时返回 ambiguous。
func ResolveIndustryClassification(
	records []IndustryClassificationRecord,
	classificationSystem, classificationVersion, industryCode string,
	asOf time.Time,
) (IndustryClassificationResolution, error) {
	classificationSystem = strings.TrimSpace(classificationSystem)
	classificationVersion = strings.TrimSpace(classificationVersion)
	industryCode = strings.TrimSpace(industryCode)
	if classificationSystem == "" || classificationVersion == "" || industryCode == "" || asOf.IsZero() {
		return IndustryClassificationResolution{}, ErrIndustryClassificationInvalid
	}

	var matched *IndustryClassificationRecord
	for i := range records {
		record := records[i]
		if err := ValidateIndustryClassificationRecord(record); err != nil {
			return IndustryClassificationResolution{}, err
		}
		if record.ClassificationSystem != classificationSystem ||
			record.ClassificationVersion != classificationVersion ||
			record.IndustryCode != industryCode ||
			asOf.Before(record.ValidFrom) ||
			(record.ValidTo != nil && !asOf.Before(*record.ValidTo)) {
			continue
		}
		if matched != nil {
			return IndustryClassificationResolution{Status: IndustryClassificationAmbiguous}, nil
		}
		copy := record
		matched = &copy
	}
	if matched == nil {
		return IndustryClassificationResolution{Status: IndustryClassificationNotFound}, nil
	}
	return IndustryClassificationResolution{Status: IndustryClassificationResolved, Record: *matched}, nil
}

package domain

// BusinessQualityStatus 描述新闻业务质量，不表达通用诊断执行状态。
type BusinessQualityStatus string

const (
	BusinessQualityValid    BusinessQualityStatus = "valid"
	BusinessQualityDegraded BusinessQualityStatus = "degraded"
	BusinessQualityInvalid  BusinessQualityStatus = "invalid"
	BusinessQualityUnknown  BusinessQualityStatus = "unknown"
)

type QualityDimension string

const (
	QualityDimensionMetadata QualityDimension = "metadata"
	QualityDimensionContent  QualityDimension = "content"
	QualityDimensionSource   QualityDimension = "source"
)

type SourceIdentityState string

const (
	SourceIdentityUnknown  SourceIdentityState = "unknown"
	SourceIdentityResolved SourceIdentityState = "resolved"
	SourceIdentityConflict SourceIdentityState = "conflict"
)

type QualityIssue struct {
	Code      string
	Dimension QualityDimension
}

type QualityDimensionResult struct {
	Dimension QualityDimension
	Status    BusinessQualityStatus
	Issues    []QualityIssue
}

type BusinessQualityAssessment struct {
	Status     BusinessQualityStatus
	Dimensions []QualityDimensionResult
	Issues     []QualityIssue
}

// AssessBusinessQuality 依据已确认的业务证据评估候选质量。
// SourceIdentityState 必须由身份识别流程提供；不能仅凭 PublisherName 或 URL 推断已解析来源。
func AssessBusinessQuality(candidate Candidate, content ContentLayers, sourceState SourceIdentityState) BusinessQualityAssessment {
	metadata := QualityDimensionResult{Dimension: QualityDimensionMetadata, Status: BusinessQualityValid}
	if candidate.Title == "" {
		metadata.Status = BusinessQualityInvalid
		metadata.Issues = append(metadata.Issues, QualityIssue{Code: "title_missing", Dimension: metadata.Dimension})
	}
	if candidate.RawURL == "" {
		metadata.Status = degradeQuality(metadata.Status)
		metadata.Issues = append(metadata.Issues, QualityIssue{Code: "url_missing", Dimension: metadata.Dimension})
	}
	if candidate.PublishedAt == nil {
		metadata.Status = degradeQuality(metadata.Status)
		metadata.Issues = append(metadata.Issues, QualityIssue{Code: "published_at_unknown", Dimension: metadata.Dimension})
	}

	content = NormalizeContentLayers(content)
	contentResult := QualityDimensionResult{Dimension: QualityDimensionContent, Status: BusinessQualityValid}
	switch {
	case content.Body != nil && content.Body.Text != "":
		// 正文载荷存在，但其授权状态由独立策略评估，不在此重复判断。
	case content.SourceSummary != "":
		contentResult.Status = BusinessQualityDegraded
		contentResult.Issues = append(contentResult.Issues, QualityIssue{Code: "body_missing_summary_only", Dimension: contentResult.Dimension})
	case content.ProviderSummary != "":
		contentResult.Status = BusinessQualityDegraded
		contentResult.Issues = append(contentResult.Issues, QualityIssue{Code: "source_content_missing_provider_summary_only", Dimension: contentResult.Dimension})
	default:
		contentResult.Status = BusinessQualityDegraded
		if len(content.Attachments) > 0 {
			contentResult.Issues = append(contentResult.Issues, QualityIssue{Code: "attachment_only_no_body", Dimension: contentResult.Dimension})
		} else {
			contentResult.Issues = append(contentResult.Issues, QualityIssue{Code: "content_missing", Dimension: contentResult.Dimension})
		}
	}
	for _, attachment := range content.Attachments {
		for _, issue := range attachment.ValidationIssues {
			contentResult.Status = degradeQuality(contentResult.Status)
			contentResult.Issues = append(contentResult.Issues, QualityIssue{Code: "attachment_" + issue, Dimension: contentResult.Dimension})
		}
	}

	source := QualityDimensionResult{Dimension: QualityDimensionSource, Status: BusinessQualityUnknown}
	switch sourceState {
	case SourceIdentityResolved:
		source.Status = BusinessQualityValid
	case SourceIdentityConflict:
		source.Status = BusinessQualityInvalid
		source.Issues = append(source.Issues, QualityIssue{Code: "source_identity_conflict", Dimension: source.Dimension})
	case SourceIdentityUnknown, "":
		source.Issues = append(source.Issues, QualityIssue{Code: "source_identity_unresolved", Dimension: source.Dimension})
	default:
		source.Issues = append(source.Issues, QualityIssue{Code: "source_identity_state_unsupported", Dimension: source.Dimension})
	}

	result := BusinessQualityAssessment{
		Dimensions: []QualityDimensionResult{metadata, contentResult, source},
	}
	result.Status = BusinessQualityValid
	for _, dimension := range result.Dimensions {
		result.Issues = append(result.Issues, dimension.Issues...)
		result.Status = combineQuality(result.Status, dimension.Status)
	}
	return result
}

func degradeQuality(status BusinessQualityStatus) BusinessQualityStatus {
	if status == BusinessQualityInvalid {
		return status
	}
	return BusinessQualityDegraded
}

func combineQuality(current, next BusinessQualityStatus) BusinessQualityStatus {
	if current == BusinessQualityInvalid || next == BusinessQualityInvalid {
		return BusinessQualityInvalid
	}
	if current == BusinessQualityDegraded || next == BusinessQualityDegraded {
		return BusinessQualityDegraded
	}
	if current == BusinessQualityUnknown || next == BusinessQualityUnknown {
		return BusinessQualityUnknown
	}
	return BusinessQualityValid
}

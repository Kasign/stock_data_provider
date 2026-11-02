package validation

import "fmt"

type Stage string

const (
	StageParse     Stage = "parse"
	StageNormalize Stage = "normalize"
	StageValidate  Stage = "validate"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type FieldIssue struct {
	Stage         Stage    `json:"stage"`
	Field         string   `json:"field"`
	ProviderField string   `json:"provider_field,omitempty"`
	Code          string   `json:"code"`
	Expected      string   `json:"expected"`
	ActualType    string   `json:"actual_type,omitempty"`
	ActualPreview string   `json:"actual_preview,omitempty"`
	Severity      Severity `json:"severity"`
	Message       string   `json:"message"`
	RowIndex      *int     `json:"row_index,omitempty"`
	Symbol        string   `json:"symbol,omitempty"`
	TradeDate     string   `json:"trade_date,omitempty"`
}

type Report struct {
	TotalCount   int          `json:"total_count"`
	ErrorCount   int          `json:"error_count"`
	WarningCount int          `json:"warning_count"`
	InfoCount    int          `json:"info_count"`
	Issues       []FieldIssue `json:"issues"`
	Truncated    bool         `json:"truncated"`
	maxIssues    int
}

const DefaultMaxIssues = 100

func NewReport(maxIssues int) Report {
	if maxIssues <= 0 {
		maxIssues = DefaultMaxIssues
	}
	return Report{Issues: make([]FieldIssue, 0), maxIssues: maxIssues}
}

func (r *Report) Add(item FieldIssue) {
	if r == nil {
		return
	}
	r.TotalCount++
	switch item.Severity {
	case SeverityError:
		r.ErrorCount++
	case SeverityWarning:
		r.WarningCount++
	case SeverityInfo:
		r.InfoCount++
	}
	if r.maxIssues <= 0 {
		r.maxIssues = DefaultMaxIssues
	}
	if len(r.Issues) >= r.maxIssues {
		r.Truncated = true
		return
	}
	r.Issues = append(r.Issues, item)
}

func (r Report) HasErrors() bool {
	if r.ErrorCount > 0 {
		return true
	}
	// Compatibility for reports assembled as struct literals by older callers.
	for _, item := range r.Issues {
		if item.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r Report) Merge(other Report) Report {
	limit := r.maxIssues
	if limit <= 0 {
		limit = DefaultMaxIssues
	}
	merged := NewReport(limit)
	for _, item := range r.Issues {
		merged.Add(item)
	}
	for _, item := range other.Issues {
		merged.Add(item)
	}
	leftErrors, leftWarnings, leftInfo := r.severityCounts()
	rightErrors, rightWarnings, rightInfo := other.severityCounts()
	merged.TotalCount = r.TotalCount + other.TotalCount
	merged.ErrorCount = leftErrors + rightErrors
	merged.WarningCount = leftWarnings + rightWarnings
	merged.InfoCount = leftInfo + rightInfo
	merged.Truncated = merged.Truncated || r.Truncated || other.Truncated || merged.TotalCount > len(merged.Issues)
	return merged
}

func (r Report) severityCounts() (errors, warnings, info int) {
	if r.ErrorCount+r.WarningCount+r.InfoCount == r.TotalCount {
		return r.ErrorCount, r.WarningCount, r.InfoCount
	}
	for _, item := range r.Issues {
		switch item.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			info++
		}
	}
	return errors, warnings, info
}

func issue(stage Stage, field, code, expected, message string, severity Severity) FieldIssue {
	return FieldIssue{Stage: stage, Field: field, Code: code, Expected: expected, Message: message, Severity: severity}
}

func (r Report) String() string {
	return fmt.Sprintf("validation issues: %d (error=%d warning=%d info=%d returned=%d truncated=%t)",
		r.TotalCount, r.ErrorCount, r.WarningCount, r.InfoCount, len(r.Issues), r.Truncated)
}

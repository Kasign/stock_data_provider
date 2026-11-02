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
	TotalCount int          `json:"total_count"`
	Issues     []FieldIssue `json:"issues"`
	Truncated  bool         `json:"truncated"`
	maxIssues  int
}

const DefaultMaxIssues = 100

func NewReport(maxIssues int) Report {
	if maxIssues <= 0 {
		maxIssues = DefaultMaxIssues
	}
	return Report{Issues: make([]FieldIssue, 0), maxIssues: maxIssues}
}

func (r *Report) Add(issue FieldIssue) {
	if r == nil {
		return
	}
	r.TotalCount++
	if r.maxIssues <= 0 {
		r.maxIssues = DefaultMaxIssues
	}
	if len(r.Issues) >= r.maxIssues {
		r.Truncated = true
		return
	}
	r.Issues = append(r.Issues, issue)
}

func (r Report) HasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
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
	for _, issue := range r.Issues {
		merged.Add(issue)
	}
	for _, issue := range other.Issues {
		merged.Add(issue)
	}
	merged.TotalCount = r.TotalCount + other.TotalCount
	merged.Truncated = merged.Truncated || r.Truncated || other.Truncated || merged.TotalCount > len(merged.Issues)
	return merged
}

func issue(stage Stage, field, code, expected, message string, severity Severity) FieldIssue {
	return FieldIssue{Stage: stage, Field: field, Code: code, Expected: expected, Message: message, Severity: severity}
}

func (r Report) String() string {
	return fmt.Sprintf("validation issues: %d (returned %d, truncated=%t)", r.TotalCount, len(r.Issues), r.Truncated)
}

package validation

import (
	"context"
	"fmt"
	"sync"
)

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
	Count         int      `json:"count,omitempty"`
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
	count := issueCount(item)
	r.TotalCount += count
	switch item.Severity {
	case SeverityError:
		r.ErrorCount += count
	case SeverityWarning:
		r.WarningCount += count
	case SeverityInfo:
		r.InfoCount += count
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

func (r Report) HasStage(stage Stage) bool {
	for _, item := range r.Issues {
		if item.Stage == stage {
			return true
		}
	}
	return false
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
	merged.Truncated = merged.Truncated || r.Truncated || other.Truncated
	return merged
}

func (r Report) severityCounts() (errors, warnings, info int) {
	if r.ErrorCount+r.WarningCount+r.InfoCount == r.TotalCount {
		return r.ErrorCount, r.WarningCount, r.InfoCount
	}
	for _, item := range r.Issues {
		count := issueCount(item)
		switch item.Severity {
		case SeverityError:
			errors += count
		case SeverityWarning:
			warnings += count
		case SeverityInfo:
			info += count
		}
	}
	return errors, warnings, info
}

func issueCount(item FieldIssue) int {
	if item.Count > 0 {
		return item.Count
	}
	return 1
}

func issue(stage Stage, field, code, expected, message string, severity Severity) FieldIssue {
	return FieldIssue{Stage: stage, Field: field, Code: code, Expected: expected, Message: message, Severity: severity}
}

func (r Report) String() string {
	return fmt.Sprintf("validation issues: %d (error=%d warning=%d info=%d returned=%d truncated=%t)",
		r.TotalCount, r.ErrorCount, r.WarningCount, r.InfoCount, len(r.Issues), r.Truncated)
}

type reporterContextKey struct{}

// Reporter collects validation issues for one request. It is safe for concurrent
// provider work and must not be reused across independent requests.
type Reporter struct {
	mu     sync.Mutex
	report Report
}

func NewReporter(maxIssues int) *Reporter {
	return &Reporter{report: NewReport(maxIssues)}
}

func WithReporter(ctx context.Context, reporter *Reporter) context.Context {
	if ctx == nil || reporter == nil {
		return ctx
	}
	return context.WithValue(ctx, reporterContextKey{}, reporter)
}

func AddContextReport(ctx context.Context, report Report) {
	if ctx == nil {
		return
	}
	reporter, _ := ctx.Value(reporterContextKey{}).(*Reporter)
	if reporter != nil {
		reporter.AddReport(report)
	}
}

func ReporterFromContext(ctx context.Context) *Reporter {
	if ctx == nil {
		return nil
	}
	reporter, _ := ctx.Value(reporterContextKey{}).(*Reporter)
	return reporter
}

func (r *Reporter) AddReport(report Report) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.report = r.report.Merge(report)
}

func (r *Reporter) Snapshot() Report {
	if r == nil {
		return NewReport(DefaultMaxIssues)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.report
}

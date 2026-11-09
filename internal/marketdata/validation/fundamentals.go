package validation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

// ValidateFundamentalsSnapshot reports field-level issues without changing provider data.
func ValidateFundamentalsSnapshot(snapshot *model.FundamentalsSnapshot, requestedSymbol string, year, quarter int) Report {
	report := NewReport(DefaultMaxIssues)
	add := func(field, providerField, code, expected, message string, severity Severity, row int) {
		item := FieldIssue{Stage: StageValidate, Field: field, ProviderField: providerField, Code: code,
			Expected: expected, Severity: severity, Message: message}
		if row >= 0 {
			item.RowIndex = &row
		}
		if snapshot != nil {
			item.Symbol = snapshot.Symbol
		}
		report.Add(item)
	}
	if snapshot == nil {
		add("snapshot", "", "required", "non-nil fundamentals snapshot", "fundamentals snapshot is nil", SeverityError, -1)
		return report
	}
	if snapshot.Symbol != requestedSymbol {
		add("symbol", "symbol", "identity_mismatch", requestedSymbol, "fundamentals symbol differs from requested security", SeverityError, -1)
	}
	if snapshot.Year != year || snapshot.Quarter != quarter {
		add("period", "year/quarter", "period_mismatch", fmt.Sprintf("%d-Q%d", year, quarter), "fundamentals report period differs from requested period", SeverityError, -1)
	}
	if strings.TrimSpace(snapshot.Source) == "" {
		add("source", "source", "required", "non-empty provider identifier", "fundamentals source is missing", SeverityError, -1)
	}
	if strings.TrimSpace(snapshot.SourceVersion) == "" {
		add("source_version", "source_version", "required", "identifiable provider schema/version", "fundamentals source version is missing", SeverityError, -1)
	}
	if snapshot.FetchedAt.IsZero() || snapshot.FetchedAt.After(time.Now().UTC().Add(5*time.Minute)) {
		add("fetched_at", "fetched_at", "invalid_timestamp", "non-zero timestamp not more than five minutes in the future", "fundamentals fetched_at is missing or in the future", SeverityError, -1)
	}
	if quarter < 1 || quarter > 4 || year < 2007 {
		add("period", "year/quarter", "invalid_period", "year >= 2007 and quarter in 1..4", "fundamentals requested period is invalid", SeverityError, -1)
		return report
	}
	periodEnd := time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	allowed := map[string]bool{"profitability": true, "operating": true, "growth": true, "solvency": true, "cash_flow": true, "dupont": true,
		"income_statement": true, "balance_sheet": true, "cash_flow_statement": true}
	required := []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"}
	seen := make(map[string]bool, len(snapshot.Sections))
	hasData := false
	allValid := true
	today := time.Now().UTC().Format("2006-01-02")
	for index, section := range snapshot.Sections {
		row := index
		if !allowed[section.Category] || seen[section.Category] {
			add("sections.category", "category", "unsupported_or_duplicate", "unique supported section category", "fundamentals section category is unsupported or duplicated", SeverityError, row)
		}
		seen[section.Category] = true
		if section.HasData != (len(section.Metrics) > 0) {
			add("sections.has_data", "has_data/metrics", "inconsistent_presence", "has_data must match metrics presence", "fundamentals section has inconsistent data flag and metrics", SeverityError, row)
		}
		switch section.QualityStatus {
		case model.QualityValid, model.QualityDegraded, model.QualityUnknown:
		default:
			add("sections.quality_status", "quality_status", "invalid_enum", "valid|degraded|unknown", "fundamentals section quality status is unsupported", SeverityError, row)
		}
		if section.HasData {
			hasData = true
			if section.ReportDate != periodEnd {
				add("sections.report_date", "report_date", "period_date_mismatch", periodEnd, "fundamentals report date does not match requested quarter end", SeverityError, row)
			}
			if section.ReportDate > today {
				add("sections.report_date", "report_date", "future_date", "not later than today", "fundamentals report date is in the future", SeverityError, row)
			}
			if section.PublicationDate == "" {
				if section.AvailableAt == nil && section.QualityStatus == model.QualityValid {
					add("sections.publication_date", "publication_date", "publication_evidence_missing", "publication_date or available_at", "valid fundamentals section has no publication evidence", SeverityError, row)
				}
			} else {
				parsed, err := time.Parse("2006-01-02", section.PublicationDate)
				if err != nil || parsed.Format("2006-01-02") != section.PublicationDate {
					add("sections.publication_date", "publication_date", "invalid_date", "YYYY-MM-DD", "fundamentals publication date is invalid", SeverityError, row)
				} else if section.PublicationDate < section.ReportDate || section.PublicationDate > today {
					add("sections.publication_date", "publication_date", "date_out_of_range", "report_date <= publication_date <= today", "fundamentals publication date is outside the allowed range", SeverityError, row)
				}
			}
			if section.AvailableAt != nil {
				availableDate := section.AvailableAt.In(time.UTC).Format("2006-01-02")
				if section.AvailableAt.IsZero() || section.AvailableAt.After(time.Now().UTC()) || availableDate < section.ReportDate ||
					(section.PublicationDate != "" && availableDate != section.PublicationDate) {
					add("sections.available_at", "available_at", "invalid_availability_time", "non-future timestamp consistent with report/publication date", "fundamentals availability timestamp is invalid or conflicts with publication date", SeverityError, row)
				}
			}
			if section.QualityStatus != model.QualityValid {
				allValid = false
			}
		} else {
			allValid = false
			add("sections.metrics", "metrics", "section_without_data", "at least one metric for a populated section", "fundamentals section contains no data", SeverityWarning, row)
		}
		metricKeys := make(map[string]bool, len(section.Metrics))
		for metricIndex, metric := range section.Metrics {
			if strings.TrimSpace(metric.Key) == "" || strings.TrimSpace(metric.Name) == "" || strings.TrimSpace(metric.Unit) == "" ||
				strings.TrimSpace(metric.SourceField) == "" || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
				add(fmt.Sprintf("sections.metrics[%d]", metricIndex), "metric", "invalid_metric", "key/name/unit/source_field and finite value", "fundamentals metric metadata or value is invalid", SeverityError, row)
			}
			if metricKeys[metric.Key] {
				add(fmt.Sprintf("sections.metrics[%d].key", metricIndex), "key", "duplicate_metric", "unique metric key within section", "fundamentals section contains duplicate metric keys", SeverityError, row)
			}
			metricKeys[metric.Key] = true
		}
	}
	for _, category := range required {
		if !seen[category] {
			add("sections", "category", "required_section_missing", category, "required fundamentals indicator section is missing", SeverityError, -1)
		}
	}
	if snapshot.HasData != hasData {
		add("has_data", "has_data", "inconsistent_presence", "matches populated sections", "fundamentals snapshot data flag is inconsistent", SeverityError, -1)
	}
	if snapshot.QualityStatus == model.QualityValid && (!hasData || !allValid) {
		add("quality_status", "quality_status", "invalid_quality_claim", "degraded or unknown when sections are incomplete", "fundamentals snapshot cannot be valid with missing or incomplete sections", SeverityError, -1)
	}
	return report
}

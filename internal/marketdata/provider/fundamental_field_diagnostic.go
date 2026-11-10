package provider

import (
	"context"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

// addProviderFieldParseIssue 将 Provider 原始字段问题写入本次请求的统一字段 Reporter。
// 字段诊断属于 Data 领域质量证据；Platform Observation 只承载领域无关的检查结果。
func addProviderFieldParseIssue(ctx context.Context, symbol, field, providerField, code, expected, actualType, preview string, rowIndex int) {
	report := validation.NewReport(1)
	row := rowIndex
	report.Add(validation.FieldIssue{
		Stage: validation.StageParse, Field: field, ProviderField: providerField,
		Code: code, Expected: expected, ActualType: actualType,
		ActualPreview: providerFieldIssuePreview(preview), Severity: validation.SeverityWarning,
		Message: "fundamentals provider field is missing or invalid",
		RowIndex: &row, Symbol: symbol,
	})
	validation.AddContextReport(ctx, report)
}

func providerFieldIssuePreview(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)
	return truncateDiagnosticPreview(value, 160)
	return value
}

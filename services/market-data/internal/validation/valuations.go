package validation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

// ValidateValuationSnapshot reports structural and field-level valuation issues.
// Nullable metrics are informational: a null value is not converted to zero and
// does not imply that the provider explicitly reported an unavailable value.
func ValidateValuationSnapshot(snapshot *model.ValuationSnapshot, requested []string) Report {
	report := NewReport(DefaultMaxIssues)
	add := func(symbol, field, providerField, code, expected, message string, severity Severity, row int) {
		item := FieldIssue{Stage: StageValidate, Field: field, ProviderField: providerField, Code: code,
			Expected: expected, Severity: severity, Message: message, Symbol: symbol}
		if row >= 0 {
			item.RowIndex = &row
		}
		report.Add(item)
	}
	if snapshot == nil {
		add("", "snapshot", "", "required", "non-nil valuation snapshot", "valuation snapshot is nil", SeverityError, -1)
		return report
	}
	if strings.TrimSpace(snapshot.Source) == "" {
		add("", "source", "source", "required", "non-empty provider identifier", "valuation source is missing", SeverityError, -1)
	}
	if strings.TrimSpace(snapshot.SourceVersion) == "" {
		add("", "source_version", "source_version", "required", "identifiable provider schema/version", "valuation source version is missing", SeverityError, -1)
	}
	now := time.Now()
	if snapshot.FetchedAt.IsZero() || snapshot.FetchedAt.After(now.Add(5*time.Minute)) {
		add("", "fetched_at", "fetched_at", "invalid_timestamp", "non-zero timestamp not more than five minutes in the future", "valuation fetched_at is missing or in the future", SeverityError, -1)
	}
	if snapshot.ProviderReportDate != "" {
		date, err := time.Parse("2006-01-02", snapshot.ProviderReportDate)
		if err != nil || date.Format("2006-01-02") != snapshot.ProviderReportDate {
			add("", "provider_report_date", "provider_report_date", "invalid_date", "YYYY-MM-DD", "valuation provider report date is invalid", SeverityError, -1)
		} else if snapshot.ProviderReportDate > now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02") {
			add("", "provider_report_date", "provider_report_date", "future_date", "not later than China market date", "valuation provider report date is in the future", SeverityError, -1)
		}
	} else {
		add("", "provider_report_date", "provider_report_date", "date_evidence_missing", "provider report date when supplied by source", "valuation report date was not supplied; freshness cannot be inferred from fetch time", SeverityWarning, -1)
	}
	if snapshot.ProviderTimestamp == nil {
		add("", "provider_timestamp", "provider_timestamp", "timestamp_evidence_missing", "exact provider timestamp when supplied by source", "valuation provider timestamp is absent; report date and fetch time are not substitutes", SeverityWarning, -1)
	} else if snapshot.ProviderTimestamp.IsZero() || snapshot.ProviderTimestamp.After(now.Add(time.Minute)) {
		add("", "provider_timestamp", "provider_timestamp", "invalid_timestamp", "non-zero timestamp not more than one minute in the future", "valuation provider timestamp is invalid", SeverityError, -1)
	}
	switch snapshot.QualityStatus {
	case model.QualityValid:
	case model.QualityDegraded:
		add("", "quality_status", "quality_status", "provider_degraded", "valid only when all required contract evidence is present", "provider marks valuation snapshot as degraded", SeverityWarning, -1)
	case model.QualityUnknown:
		add("", "quality_status", "quality_status", "quality_unknown", "valid|degraded|unknown", "provider valuation quality is unknown", SeverityWarning, -1)
	default:
		add("", "quality_status", "quality_status", "invalid_enum", "valid|degraded|unknown", "valuation snapshot quality status is unsupported", SeverityError, -1)
	}
	if snapshot.Total != len(snapshot.Items) {
		add("", "total", "total", "count_mismatch", "total equals returned item count", "valuation total does not match returned items", SeverityError, -1)
	}
	if len(snapshot.RequestedSymbols) != len(requested) {
		add("", "requested_symbols", "requested_symbols", "count_mismatch", "same symbols and order as request", "valuation requested-symbol count differs from probe request", SeverityError, -1)
	} else {
		for i, symbol := range requested {
			if snapshot.RequestedSymbols[i] != symbol {
				add("", fmt.Sprintf("requested_symbols[%d]", i), "requested_symbols", "identity_mismatch", symbol, "valuation requested-symbol identity or order differs from request", SeverityError, i)
			}
		}
	}
	expected := make(map[string]struct{}, len(requested))
	for _, symbol := range requested {
		expected[symbol] = struct{}{}
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	itemDegraded := false
	for i, item := range snapshot.Items {
		symbol := item.Symbol
		if !isCanonicalValuationSymbol(symbol) {
			add(item.Symbol, "symbol", "symbol", "invalid_symbol", "six digits plus .SH/.SZ/.BJ", "valuation item symbol is invalid or noncanonical", SeverityError, i)
		}
		if _, ok := expected[symbol]; !ok {
			add(item.Symbol, "symbol", "symbol", "unrequested_symbol", "symbol included in request", "valuation provider returned an unrequested symbol", SeverityError, i)
		}
		if _, duplicate := seen[symbol]; duplicate {
			add(item.Symbol, "symbol", "symbol", "duplicate_symbol", "unique symbol per snapshot", "valuation provider returned a duplicate symbol", SeverityError, i)
		}
		seen[symbol] = struct{}{}
		if item.Code != strings.Split(symbol, ".")[0] {
			add(symbol, "code", "code", "identity_mismatch", strings.Split(symbol, ".")[0], "valuation code does not match canonical symbol", SeverityError, i)
		}
		if strings.TrimSpace(item.Source) == "" || item.Source != snapshot.Source {
			add(symbol, "source", "source", "source_mismatch", snapshot.Source, "valuation item source differs from snapshot source", SeverityError, i)
		}
		if item.SourceVersion != snapshot.SourceVersion {
			add(symbol, "source_version", "source_version", "version_mismatch", snapshot.SourceVersion, "valuation item source version differs from snapshot version", SeverityError, i)
		}
		if item.FetchedAt.IsZero() || item.FetchedAt.After(now.Add(5*time.Minute)) {
			add(symbol, "fetched_at", "fetched_at", "invalid_timestamp", "non-zero timestamp not more than five minutes in the future", "valuation item fetched_at is missing or in the future", SeverityError, i)
		}
		switch item.QualityStatus {
		case model.QualityValid:
		case model.QualityDegraded:
			itemDegraded = true
			add(symbol, "quality_status", "quality_status", "item_degraded", "valid or degraded", "valuation item is marked degraded", SeverityWarning, i)
		default:
			add(symbol, "quality_status", "quality_status", "invalid_enum", "valid|degraded", "valuation item quality status is unsupported", SeverityError, i)
		}
		metrics := []struct {
			field string
			value *float64
		}{
			{"pe_ttm", item.PETTM}, {"pe_mrq", item.PEMRQ}, {"pe_static", item.PEStatic},
			{"pb_mrq", item.PBMRQ}, {"ps_ttm", item.PSTTM}, {"pcf_ttm", item.PCFTTM},
			{"turnover_rate_percent", item.TurnoverRatePercent}, {"float_turnover_rate_percent", item.FloatTurnoverRatePercent},
			{"volume_ratio", item.VolumeRatio}, {"dividend_yield_percent", item.DividendYieldPercent},
			{"dividend_yield_ttm_percent", item.DividendYieldTTMPercent},
		}
		for _, metric := range metrics {
			if metric.value == nil {
				add(symbol, metric.field, metric.field, "value_not_provided", "nullable numeric value; null is not zero", "provider did not supply this optional valuation metric; availability reason is not inferred", SeverityInfo, i)
				continue
			}
			if math.IsNaN(*metric.value) || math.IsInf(*metric.value, 0) {
				add(symbol, metric.field, metric.field, "non_finite_value", "finite number or null", "valuation metric is not a finite number", SeverityError, i)
			}
		}
		// Valuation multiples may legitimately be negative. Ratio/percentage fields
		// follow the existing service contract and must not be negative.
		for _, metric := range []struct {
			field string
			value *float64
		}{
			{"turnover_rate_percent", item.TurnoverRatePercent},
			{"float_turnover_rate_percent", item.FloatTurnoverRatePercent},
			{"volume_ratio", item.VolumeRatio},
			{"dividend_yield_percent", item.DividendYieldPercent},
			{"dividend_yield_ttm_percent", item.DividendYieldTTMPercent},
		} {
			if metric.value != nil && !math.IsNaN(*metric.value) && !math.IsInf(*metric.value, 0) && *metric.value < 0 {
				add(symbol, metric.field, metric.field, "negative_ratio", "non-negative value or null", "valuation ratio/percentage field is negative", SeverityError, i)
			}
		}
	}
	missing := make(map[string]struct{}, len(snapshot.MissingSymbols))
	for i, symbol := range snapshot.MissingSymbols {
		if _, ok := expected[symbol]; !ok {
			add(symbol, fmt.Sprintf("missing_symbols[%d]", i), "missing_symbols", "unrequested_symbol", "requested symbol", "valuation snapshot marks an unrequested symbol as missing", SeverityError, i)
		}
		if _, returned := seen[symbol]; returned {
			add(symbol, "missing_symbols", "missing_symbols", "returned_and_missing", "symbol either returned or missing, never both", "valuation symbol is both returned and marked missing", SeverityError, i)
		}
		if _, duplicate := missing[symbol]; duplicate {
			add(symbol, "missing_symbols", "missing_symbols", "duplicate_missing_symbol", "unique missing symbol", "valuation missing-symbol list contains a duplicate", SeverityError, i)
		}
		missing[symbol] = struct{}{}
		add(symbol, "symbol", "missing_symbols", "requested_symbol_missing", "one returned item or explicit missing-symbol entry", "provider did not return a requested security", SeverityWarning, i)
	}
	if len(seen)+len(missing) != len(requested) {
		add("", "items", "items", "coverage_mismatch", "every requested symbol is returned or explicitly missing", "valuation returned/missing symbol coverage does not match request", SeverityError, -1)
	}
	if snapshot.QualityStatus == model.QualityValid &&
		(len(snapshot.MissingSymbols) > 0 || snapshot.ProviderTimestamp == nil || snapshot.ProviderReportDate == "" || itemDegraded) {
		add("", "quality_status", "quality_status", "invalid_quality_claim", "degraded or unknown when required evidence is incomplete", "valuation snapshot claims valid quality despite missing source-time, coverage, or degraded item evidence", SeverityError, -1)
	}
	return report
}

func isCanonicalValuationSymbol(symbol string) bool {
	if len(symbol) != 9 || symbol[6] != '.' {
		return false
	}
	switch symbol[7:] {
	case "SH", "SZ", "BJ":
	default:
		return false
	}
	for _, digit := range symbol[:6] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
